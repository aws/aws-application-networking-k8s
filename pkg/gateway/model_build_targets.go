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
	undefinedPort            = int32(0)
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
		return errors.New(errmsg)
	}

	if !tg.ByBackendRef && !tg.ByServiceExport {
		errmsg := fmt.Sprintf("Build Targets failed because its target Group name=%s, namespace=%s is no longer referenced", t.tgName, t.tgNamespace)
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

	definedPorts := make(map[int32]struct{})
	if tg.ByServiceExport {
		serviceExport := &mcs_api.ServiceExport{}
		err = t.Client.Get(ctx, namespacedName, serviceExport)
		if err != nil {
			glog.V(6).Infof("Failed to find Service export in the DS. Name:%s, Namespace:%s - err:%s", t.tgName, t.tgNamespace, err)
		} else {
			portsAnnotations := strings.Split(serviceExport.ObjectMeta.Annotations[portAnnotationsKey], ",")

			for _, portAnnotation := range portsAnnotations {
				definedPort, err := strconv.ParseInt(portAnnotation, 10, 32)
				if err != nil {
					glog.V(6).Infof("Failed to read Annotations/Port:%s, err:%s", serviceExport.ObjectMeta.Annotations[portAnnotationsKey], err)
				} else {
					definedPorts[int32(definedPort)] = struct{}{}
				}
			}
			glog.V(6).Infof("Build Targets - portAnnotations: %v", definedPorts)
		}
	} else if tg.ByBackendRef && t.port != undefinedPort {
		definedPorts[t.port] = struct{}{}
	}

	// A service port MUST have a name if there are multiple ports exposed from a service.
	// Therefore, if a port is named, endpoint port is only relevant if it has the same name.
	//
	// If a service port is unnamed, it MUST be the only port that is exposed from a service.
	// In this case, as long as the service port is matching with backendRef/annotations,
	// we can consider all endpoints valid.

	servicePortNames := make(map[string]struct{})
	skipMatch := false

	for _, port := range svc.Spec.Ports {
		if _, ok := definedPorts[port.Port]; ok {
			if port.Name != "" {
				servicePortNames[port.Name] = struct{}{}
			} else {
				// Unnamed, consider all endpoints valid
				skipMatch = true
			}
		}
	}

	// Having no backendRef port makes all endpoints valid - this is mainly for backwards compatibility.
	if len(definedPorts) == 0 {
		skipMatch = true
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
					glog.V(6).Infof("serviceReconcile-endpoints: address %v, port %v", address, port)
					target := latticemodel.Target{
						TargetIP: address.IP,
						Port:     int64(port.Port),
					}
					if _, ok := servicePortNames[port.Name]; ok || skipMatch {
						targetList = append(targetList, target)
					}
				}
			}
		}
	}

	glog.V(6).Infof("Build Targets--- targetIPList [%v]", targetList)

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
	tgName         string
	tgNamespace    string
	routename      string
	port           int32
	latticeTargets *latticemodel.Targets
	stack          core.Stack
	datastore      *latticestore.LatticeDataStore
	route          core.Route
}
