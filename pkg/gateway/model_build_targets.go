package gateway

import (
	"context"
	"errors"
	"fmt"
	"github.com/golang/glog"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	lattice_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

const (
	resourceIDLatticeTargets = "LatticeTargets"
)

type LatticeTargetsBuilder interface {
	Build(ctx context.Context, service *corev1.Service, routename string) (core.Stack, *latticemodel.Targets, error)
}

type latticeTargetsModelBuilder struct {
	client.Client
	defaultTags map[string]string

	datastore *latticestore.LatticeDataStore

	cloud lattice_aws.Cloud
}

func NewTargetsBuilder(client client.Client, cloud lattice_aws.Cloud, datastore *latticestore.LatticeDataStore) *latticeTargetsModelBuilder {
	return &latticeTargetsModelBuilder{
		Client:    client,
		cloud:     cloud,
		datastore: datastore,
	}
}

func (b *latticeTargetsModelBuilder) Build(ctx context.Context, service *corev1.Service, routename string) (core.Stack, *latticemodel.Targets, error) {
	stack := core.NewDefaultStack(core.StackID(k8s.NamespacedName((service))))

	task := &latticeTargetsModelBuildTask{
		Client:      b.Client,
		tgName:      service.Name,
		tgNamespace: service.Namespace,
		routename:   routename,
		stack:       stack,
		datastore:   b.datastore,
	}

	if err := task.run(ctx); err != nil {
		return nil, nil, corev1.ErrIntOverflowGenerated
	}

	return task.stack, task.latticeTargets, nil
}

func (t *latticeTargetsModelBuildTask) run(ctx context.Context) error {
	err := t.buildModel(ctx)

	return err
}

func (t *latticeTargetsModelBuildTask) buildModel(ctx context.Context) error {
	err := t.buildLatticeTargets(ctx)

	if err != nil {
		glog.V(6).Infof("Failed on buildLatticeTargets %v \n", err)
		return err
	}

	return nil
}

func (t *latticeTargetsModelBuildTask) buildLatticeTargets(ctx context.Context) error {
	ds := t.datastore
	tgName := latticestore.TargetGroupName(t.tgName, t.tgNamespace)
	tg, err := ds.GetTargetGroup(tgName, t.routename, false) // isServiceImport= false

	if err != nil {
		errmsg := fmt.Sprintf("Build Targets failed because target group (name=%s, namespace=%s found not in datastore)", t.tgName, t.tgNamespace)
		glog.V(6).Infof("errmsg %s\n ", errmsg)
		return errors.New(errmsg)
	}

	if !tg.ByBackendRef && !tg.ByServiceExport {
		errmsg := fmt.Sprintf("Build Targets failed because its target Group name=%s, namespace=%s is no longer referenced", t.tgName, t.tgNamespace)
		glog.V(6).Infof("errmsg %s\n", errmsg)
		return errors.New(errmsg)
	}

	endPoints := &corev1.Endpoints{}

	svc := &corev1.Service{}
	namespacedName := types.NamespacedName{
		Namespace: t.tgNamespace,
		Name:      t.tgName,
	}

	if err := t.Client.Get(ctx, namespacedName, svc); err != nil {
		errmsg := fmt.Sprintf("Build Targets failed because K8S service %v does not exist", namespacedName)
		return errors.New(errmsg)
	}
	var targetList []latticemodel.Target

	if svc.DeletionTimestamp.IsZero() {
		if err := t.Client.Get(ctx, namespacedName, endPoints); err != nil {
			errmsg := fmt.Sprintf("Build Targets failed because K8S service %v does not exist", namespacedName)
			glog.V(6).Infof("errmsg: %v\n", errmsg)
			return errors.New(errmsg)
		}

		glog.V(6).Infof("Build Targets:  endPoints %v \n", endPoints)

		for _, endPoint := range endPoints.Subsets {

			for _, address := range endPoint.Addresses {
				for _, port := range endPoint.Ports {
					glog.V(6).Infof("serviceReconcile-endpoints: address %v, port %v\n", address, port)
					target := latticemodel.Target{
						TargetIP: address.IP,
						Port:     int64(port.Port),
					}
					targetList = append(targetList, target)
				}
			}
		}
	}

	glog.V(6).Infof("Build Targets--- targetIPList [%v]\n", targetList)

	spec := latticemodel.TargetsSpec{
		Name:         t.tgName,
		Namespace:    t.tgNamespace,
		RouteName:    t.routename,
		TargetIPList: targetList,
	}

	t.latticeTargets = latticemodel.NewTargets(t.stack, tgName, spec)

	return nil
}

type latticeTargetsModelBuildTask struct {
	client.Client
	tgName      string
	tgNamespace string
	routename   string

	latticeTargets *latticemodel.Targets
	stack          core.Stack

	datastore *latticestore.LatticeDataStore
}
