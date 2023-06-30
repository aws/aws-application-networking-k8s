package gateway

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/golang/glog"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gateway_api "sigs.k8s.io/gateway-api/apis/v1beta1"
	mcs_api "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	lattice_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

const (
	resourceIDLatticeTargets = "LatticeTargets"
	portAnnotationsKey       = "multicluster.x-k8s.io/port"
	undefinedPort            = int64(0)
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

	svc := &corev1.Service{}
	namespacedName := types.NamespacedName{
		Namespace: t.tgNamespace,
		Name:      t.tgName,
	}

	if err := t.Client.Get(ctx, namespacedName, svc); err != nil {
		errmsg := fmt.Sprintf("Build Targets failed because K8S service %v does not exist", namespacedName)
		return errors.New(errmsg)
	}

	definedPorts := []int64{undefinedPort}
	if tg.ByServiceExport {
		serviceExport := &mcs_api.ServiceExport{}
		err = t.Client.Get(ctx, namespacedName, serviceExport)
		if err != nil {
			glog.V(6).Infof("Failed to find Service export in the DS. Name:%v, Namespace:%v - err:%s\n ", t.tgName, t.tgNamespace, err)
		} else {
			// TODO: Change the code to support multiple comma separated ports instead of a single port
			//portsAnnotations := strings.Split(serviceExport.ObjectMeta.Annotations["multicluster.x-k8s.io/Ports"], ",")
			definedPorts[0], err = strconv.ParseInt(serviceExport.ObjectMeta.Annotations[portAnnotationsKey], 10, 64)
			if err != nil {
				glog.V(6).Infof("Failed to read Annotaions/Port:%v, err:%s\n ", serviceExport.ObjectMeta.Annotations[portAnnotationsKey], err)
			}
			glog.V(6).Infof("Build Targets - portAnnotations: %v \n", definedPorts)
		}
	} else if tg.ByBackendRef {
		if (nil != t.httpRoute) && (nil != t.httpRoute.Spec.Rules) && (0 < len(t.httpRoute.Spec.Rules)) {
			definedPorts = []int64{}
			for _, rule := range t.httpRoute.Spec.Rules {
				for _, ref := range rule.BackendRefs {
					definedPorts = append(definedPorts, int64(*ref.Port))
				}
			}
		}
	}

	var targetList []latticemodel.Target
	endPoints := &corev1.Endpoints{}

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
					if definedPorts[0] == undefinedPort || portdefined(definedPorts, int64(target.Port)) {
						targetList = append(targetList, target)
						glog.V(6).Infof("Found a port match, registering the target - port:%v, containerPort:%v, taerget:%v ***\n", int64(target.Port), definedPorts[0], target)
					} else {
						glog.V(6).Infof("Found port does not match the defined port. definedPort:%v, target.Port:%v\n", definedPorts[0], target.Port)
					}
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

func portdefined(definedPorts []int64, foundPort int64) bool {
	for _, port := range definedPorts {
		if foundPort == port {
			return true
		}
	}
	return false
}

type latticeTargetsModelBuildTask struct {
	client.Client
	tgName         string
	tgNamespace    string
	routename      string
	latticeTargets *latticemodel.Targets
	stack          core.Stack
	datastore      *latticestore.LatticeDataStore
	httpRoute      *gateway_api.HTTPRoute
}
