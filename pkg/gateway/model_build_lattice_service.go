package gateway

import (
	"context"
	"errors"
	"fmt"
	"github.com/golang/glog"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"

	lattice_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
)

const (
	resourceIDLatticeService = "LatticeService"
)

type LatticeServiceBuilder interface {
	Build(ctx context.Context, httpRoute core.Route) (core.Stack, *latticemodel.Service, error)
}

type latticeServiceModelBuilder struct {
	client.Client
	defaultTags map[string]string
	Datastore   *latticestore.LatticeDataStore

	cloud lattice_aws.Cloud
}

func NewLatticeServiceBuilder(client client.Client, datastore *latticestore.LatticeDataStore, cloud lattice_aws.Cloud) *latticeServiceModelBuilder {
	return &latticeServiceModelBuilder{
		Client:    client,
		Datastore: datastore,
		cloud:     cloud,
	}
}

func (b *latticeServiceModelBuilder) Build(ctx context.Context, route core.Route) (core.Stack, *latticemodel.Service, error) {
	stack := core.NewDefaultStack(core.StackID(k8s.NamespacedName(route.K8sObject())))

	task := &latticeServiceModelBuildTask{
		route:     route,
		stack:     stack,
		Client:    b.Client,
		tgByResID: make(map[string]*latticemodel.TargetGroup),
		Datastore: b.Datastore,
	}

	if err := task.run(ctx); err != nil {
		return stack, task.latticeService, errors.New("LATTICE_RETRY")
	}

	return task.stack, task.latticeService, nil
}

func (t *latticeServiceModelBuildTask) run(ctx context.Context) error {

	err := t.buildModel(ctx)

	return err
}

func (t *latticeServiceModelBuildTask) buildModel(ctx context.Context) error {
	err := t.buildLatticeService(ctx)

	if err != nil {
		return fmt.Errorf("latticeServiceModelBuildTask: Failed on buildLatticeService %s", err)
	}

	_, err = t.buildTargetGroup(ctx, t.Client)

	if err != nil {
		return fmt.Errorf("latticeServiceModelBuildTask: Failed on buildTargetGroup %+v", err)
	}

	if !t.route.DeletionTimestamp().IsZero() {
		glog.V(2).Infof("latticeServiceModelBuildTask: for delete ignore Targets, policy %v\n", t.route)
		return nil
	}

	err = t.buildTargets(ctx)

	if err != nil {
		glog.V(6).Infof("latticeServiceModelBuildTask: Failed on building targets, error = %v\n ", err)
	}
	// only build listener when it is NOT delete case
	err = t.buildListener(ctx)

	if err != nil {
		return fmt.Errorf("latticeServiceModelBuildTask: Failed on building listener %+v", err)
	}

	err = t.buildRules(ctx)

	if err != nil {
		return fmt.Errorf("latticeServiceModelBuildTask: Failed on building rule %+v", err)
	}

	return nil
}

func (t *latticeServiceModelBuildTask) buildLatticeService(ctx context.Context) error {
	pro := "HTTP"
	protocols := []*string{&pro}
	spec := latticemodel.ServiceSpec{
		Name:      t.route.Name(),
		Namespace: t.route.Namespace(),
		Protocols: protocols,
		//ServiceNetworkNames: string(t.route.Spec().ParentRefs()[0].Name),
	}

	for _, parentRef := range t.route.Spec().ParentRefs() {
		spec.ServiceNetworkNames = append(spec.ServiceNetworkNames, string(parentRef.Name))
	}
	defaultGateway, err := config.GetClusterLocalGateway()
	if err == nil {
		spec.ServiceNetworkNames = append(spec.ServiceNetworkNames, defaultGateway)
	}

	if len(t.route.Spec().Hostnames()) > 0 {
		// The 1st hostname will be used as lattice customer-domain-name
		spec.CustomerDomainName = string(t.route.Spec().Hostnames()[0])

		glog.V(2).Infof("Setting customer-domain-name: %v for httpRoute %v-%v",
			spec.CustomerDomainName, t.route.Name(), t.route.Namespace())
	} else {
		glog.V(2).Infof("No custom-domain-name for httproute :%v-%v",
			t.route.Name(), t.route.Namespace())
		spec.CustomerDomainName = ""
	}

	if t.route.DeletionTimestamp().IsZero() {
		spec.IsDeleted = false
	} else {
		spec.IsDeleted = true
	}

	serviceResourceName := fmt.Sprintf("%s-%s", t.route.Name(), t.route.Namespace())

	t.latticeService = latticemodel.NewLatticeService(t.stack, serviceResourceName, spec)

	return nil
}

type latticeServiceModelBuildTask struct {
	route core.Route
	client.Client

	latticeService  *latticemodel.Service
	tgByResID       map[string]*latticemodel.TargetGroup
	listenerByResID map[string]*latticemodel.Listener
	rulesByResID    map[string]*latticemodel.Rule
	stack           core.Stack

	Datastore *latticestore.LatticeDataStore
	cloud     lattice_aws.Cloud
}
