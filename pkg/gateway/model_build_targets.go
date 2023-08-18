package gateway

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/golang/glog"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
		glog.V(6).Infof("Failed on buildLatticeTargets %s", err)
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
		glog.V(6).Infof("errmsg %s", errmsg)
		return errors.New(errmsg)
	}

	if !tg.ByBackendRef && !tg.ByServiceExport {
		errmsg := fmt.Sprintf("Build Targets failed because its target Group name=%s, namespace=%s is no longer referenced", t.tgName, t.tgNamespace)
		glog.V(6).Infof("errmsg %s", errmsg)
		return errors.New(errmsg)
	}

	svc := &corev1.Service{}
	namespacedName := types.NamespacedName{
		Namespace: t.tgNamespace,
		Name:      t.tgName,
	}

	if err := t.Client.Get(ctx, namespacedName, svc); err != nil {
		errmsg := fmt.Sprintf("Build Targets failed because K8S service %s does not exist", namespacedName)
		return errors.New(errmsg)
	}

	definedPorts := make([]int64, 0)
	if tg.ByServiceExport {
		serviceExport := &mcs_api.ServiceExport{}
		err = t.Client.Get(ctx, namespacedName, serviceExport)
		if err != nil {
			glog.V(6).Infof("Failed to find Service export in the DS. Name:%s, Namespace:%s - err:%s", t.tgName, t.tgNamespace, err)
		} else {
			portsAnnotations := strings.Split(serviceExport.ObjectMeta.Annotations[portAnnotationsKey], ",")

			for _, portAnnotation := range portsAnnotations {
				definedPort, err := strconv.ParseInt(portAnnotation, 10, 64)
				if err != nil {
					glog.V(6).Infof("Failed to read Annotations/Port:%s, err:%s", serviceExport.ObjectMeta.Annotations[portAnnotationsKey], err)
				} else {
					definedPorts = append(definedPorts, definedPort)
				}
			}
			glog.V(6).Infof("Build Targets - portAnnotations: %s", definedPorts)
		}
	} else if tg.ByBackendRef {
		definedPorts = getDefinedPortsFromK8sRouteRules(t, definedPorts)
	}
	if len(definedPorts) == 0 {
		definedPorts = []int64{undefinedPort}
	}

	var targetList []latticemodel.Target
	endPoints := &corev1.Endpoints{}

	if svc.DeletionTimestamp.IsZero() {
		if err := t.Client.Get(ctx, namespacedName, endPoints); err != nil {
			errmsg := fmt.Sprintf("Build Targets failed because K8S service %s does not exist", namespacedName)
			glog.V(6).Infof("errmsg: %s", errmsg)
			return errors.New(errmsg)
		}

		glog.V(6).Infof("Build Targets:  endPoints %s", endPoints)

		for _, endPoint := range endPoints.Subsets {
			for _, address := range endPoint.Addresses {
				for _, port := range endPoint.Ports {
					glog.V(6).Infof("serviceReconcile-endpoints: address %s, port %s", address, port)
					target := latticemodel.Target{
						TargetIP: address.IP,
						Port:     int64(port.Port),
					}

					for _, definedPort := range definedPorts {
						if target.Port == definedPort {
							// target port matches service export port
							targetList = append(targetList, target)
							glog.V(6).Infof("portAnnotations:%s, target.Port:%s", definedPort, target.Port)
						} else if definedPort == undefinedPort || definedPort == target.Port {
							targetList = append(targetList, target)
							glog.V(6).Infof("Found a port match, registering = definedPort:%s, port.Port:%s", definedPort, port.Port)
						} else {
							glog.V(6).Infof("Port does not match the target - port:%s, definedPort:%s, target:%s ***", target.Port, definedPort, target)
						}
					}
				}
			}
		}
	}

	glog.V(6).Infof("Build Targets--- targetIPList [%s]", targetList)

	spec := latticemodel.TargetsSpec{
		Name:         t.tgName,
		Namespace:    t.tgNamespace,
		RouteName:    t.routename,
		TargetIPList: targetList,
	}

	t.latticeTargets = latticemodel.NewTargets(t.stack, tgName, spec)

	return nil
}

func getDefinedPortsFromK8sRouteRules(t *latticeTargetsModelBuildTask, definedPorts []int64) []int64 {
	if t.route != nil && t.route.Spec().Rules() != nil {
		definedPorts = []int64{}
		for _, rule := range t.route.Spec().Rules() {
			for _, ref := range rule.BackendRefs() {
				if ref.Port() != nil && string(ref.Name()) == t.tgName && string(*ref.Namespace()) == t.tgNamespace {
					definedPorts = append(definedPorts, int64(*ref.Port()))
				}
			}
		}
	}
	return definedPorts
}

type latticeTargetsModelBuildTask struct {
	client.Client
	tgName         string
	tgNamespace    string
	routename      string
	port           int64
	latticeTargets *latticemodel.Targets
	stack          core.Stack
	datastore      *latticestore.LatticeDataStore
	route          core.Route
}
