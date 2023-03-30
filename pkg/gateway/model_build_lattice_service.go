package gateway

import (
	"context"
	"errors"
	"fmt"
	"github.com/golang/glog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	gateway_api "sigs.k8s.io/gateway-api/apis/v1beta1"

	lattice_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
)

const (
	resourceIDLatticeService = "LatticeService"
)

type LatticeServiceBuilder interface {
	Build(ctx context.Context, httpRoute *gateway_api.HTTPRoute) (core.Stack, *latticemodel.Service, error)
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

// TODO  right now everything is around HTTPRoute,  future, this might need to refactor for TLSRoute
func (b *latticeServiceModelBuilder) Build(ctx context.Context, httpRoute *gateway_api.HTTPRoute) (core.Stack, *latticemodel.Service, error) {
	stack := core.NewDefaultStack(core.StackID(k8s.NamespacedName(httpRoute)))

	task := &latticeServiceModelBuildTask{
		httpRoute: httpRoute,
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
		glog.V(2).Infof("latticeServiceModelBuildTask: Failed on buildLatticeService %v\n ", err)
		return err
	}

	if !t.httpRoute.DeletionTimestamp.IsZero() {
		// in case of deleting HTTPRoute, we will let reconcile logic to delete
		// stated target group(s) at next reconcile interval
		glog.V(2).Infof("latticeServiceModuleBuildTask: for HTTPRouteDelete, reconcile tagetgroups/targets at reconcile interval")
		return nil
	}

	_, err = t.buildTargetGroup(ctx, t.Client)

	if err != nil {
		glog.V(2).Infof("latticeServiceModelBuildTask: Failed on buildTargetGroup, error=%v\n", err)
		return err
	}

	if !t.httpRoute.DeletionTimestamp.IsZero() {
		glog.V(2).Infof("latticeServiceModelBuildTask: for delete ignore Targets, policy %v\n", t.httpRoute)
		return nil
	}

	err = t.buildTargets(ctx)

	if err != nil {
		glog.V(6).Infof("latticeServiceModelBuildTask: Faild on building targets, error = %v\n ", err)
	}
	// only build listener when it is NOT delete case
	err = t.buildListener(ctx)

	if err != nil {
		glog.V(6).Infof("latticeServiceModelBuildTask: Faild on building listener, error = %v \n", err)
		return err
	}

	err = t.buildRules(ctx)

	if err != nil {
		glog.V(2).Infof("latticeServiceModelBuildTask: Failed on building rule, error = %v \n", err)
		return err
	}

	return nil
}

func (t *latticeServiceModelBuildTask) buildLatticeService(ctx context.Context) error {
	pro := "HTTP"
	protocols := []*string{&pro}
	spec := latticemodel.ServiceSpec{
		Name:      t.httpRoute.Name,
		Namespace: t.httpRoute.Namespace,
		Protocols: protocols,
		//ServiceNetworkNames: string(t.httpRoute.Spec.ParentRefs[0].Name),
	}

	for _, parentRef := range t.httpRoute.Spec.ParentRefs {
		spec.ServiceNetworkNames = append(spec.ServiceNetworkNames, string(parentRef.Name))

	}

	if len(t.httpRoute.Spec.Hostnames) > 0 {
		// The 1st hostname will be used as lattice customer-domain-name
		spec.CustomerDomainName = string(t.httpRoute.Spec.Hostnames[0])

		glog.V(2).Infof("Setting customer-domain-name: %v for httpRoute %v-%v",
			spec.CustomerDomainName, t.httpRoute.Name, t.httpRoute.Namespace)
	} else {
		glog.V(2).Infof("No customter-domain-name for httproute :%v-%v",
			t.httpRoute.Name, t.httpRoute.Namespace)
		spec.CustomerDomainName = ""
	}

	if t.httpRoute.DeletionTimestamp.IsZero() {
		spec.IsDeleted = false
	} else {
		spec.IsDeleted = true
	}

	serviceResourceName := fmt.Sprintf("%s-%s", t.httpRoute.Name, t.httpRoute.Namespace)

	t.latticeService = latticemodel.NewLatticeService(t.stack, serviceResourceName, spec)

	return nil
}

type latticeServiceModelBuildTask struct {
	httpRoute *gateway_api.HTTPRoute
	client.Client

	latticeService  *latticemodel.Service
	tgByResID       map[string]*latticemodel.TargetGroup
	listenerByResID map[string]*latticemodel.Listener
	rulesByResID    map[string]*latticemodel.Rule
	stack           core.Stack

	Datastore *latticestore.LatticeDataStore
	cloud     lattice_aws.Cloud
}
