package gateway

import (
	"context"
	"errors"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	"github.com/aws/aws-sdk-go/aws"
	discoveryv1 "k8s.io/api/discovery/v1"
)

const (
	portAnnotationsKey = "application-networking.k8s.aws/port"
	undefinedPort      = int32(0)
)

type LatticeTargetsBuilder interface {
	Build(ctx context.Context, service *corev1.Service, backendRef core.BackendRef, stackTgId string) (core.Stack, error)
	BuildForServiceExport(ctx context.Context, serviceExport *anv1alpha1.ServiceExport, stackTgId string) (core.Stack, error)
}

type LatticeTargetsModelBuilder struct {
	log    gwlog.Logger
	client client.Client
	stack  core.Stack
}

func NewTargetsBuilder(
	log gwlog.Logger,
	client client.Client,
	stack core.Stack,
) *LatticeTargetsModelBuilder {
	return &LatticeTargetsModelBuilder{
		log:    log,
		client: client,
		stack:  stack,
	}
}

func (b *LatticeTargetsModelBuilder) Build(ctx context.Context, service *corev1.Service,
	backendRef core.BackendRef, stackTgId string) (core.Stack, error) {
	return b.build(ctx, nil, service, backendRef, b.stack, stackTgId)
}

func (b *LatticeTargetsModelBuilder) BuildForServiceExport(ctx context.Context,
	serviceExport *anv1alpha1.ServiceExport, stackTgId string) (core.Stack, error) {

	return b.build(ctx, serviceExport, nil, nil, b.stack, stackTgId)
}

func (b *LatticeTargetsModelBuilder) build(ctx context.Context,
	serviceExport *anv1alpha1.ServiceExport,
	service *corev1.Service, backendRef core.BackendRef,
	stack core.Stack, stackTgId string,
) (core.Stack, error) {
	isServiceExport := serviceExport != nil
	isBackendRef := service != nil && backendRef != nil
	if !(isServiceExport || isBackendRef) {
		return nil, errors.New("either service export or route/service/backendRef must be specified")
	}
	if isServiceExport && isBackendRef {
		return nil, errors.New("either service export or route/service/backendRef must be specified, but not both")
	}

	if isServiceExport {
		b.log.Debugf(ctx, "Processing targets for service export %s-%s", serviceExport.Name, serviceExport.Namespace)

		serviceName := types.NamespacedName{
			Namespace: serviceExport.Namespace,
			Name:      serviceExport.Name,
		}

		tmpSvc := &corev1.Service{}
		if err := b.client.Get(ctx, serviceName, tmpSvc); err != nil {
			return nil, err
		}
		service = tmpSvc
	} else {
		b.log.Debugf(ctx, "Processing targets for service %s-%s", service.Name, service.Namespace)
	}

	if stack == nil {
		stack = core.NewDefaultStack(core.StackID(k8s.NamespacedName(service)))
	}

	if !service.DeletionTimestamp.IsZero() {
		b.log.Debugf(ctx, "service %s/%s is deleted, skipping target build", service.Name, service.Namespace)
		return stack, nil
	}

	task := &latticeTargetsModelBuildTask{
		log:           b.log,
		client:        b.client,
		serviceExport: serviceExport,
		service:       service,
		backendRef:    backendRef,
		stack:         stack,
		stackTgId:     stackTgId,
	}

	if err := task.run(ctx); err != nil {
		return nil, err
	}

	return task.stack, nil
}

func (t *latticeTargetsModelBuildTask) run(ctx context.Context) error {
	return t.buildLatticeTargets(ctx)
}

func (t *latticeTargetsModelBuildTask) buildLatticeTargets(ctx context.Context) error {
	definedPorts := t.getDefinedPorts()

	// A service port MUST have a name if there are multiple ports exposed from a service.
	// Therefore, if a port is named, endpoint port is only relevant if it has the same name.
	//
	// If a service port is unnamed, it MUST be the only port that is exposed from a service.
	// In this case, as long as the service port is matching with backendRef/annotations,
	// we can consider all endpoints valid.

	servicePortNames := make(map[string]struct{})
	skipMatch := false

	for _, port := range t.service.Spec.Ports {
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

	var targetList []model.Target
	if t.service.DeletionTimestamp.IsZero() {
		var err error
		targetList, err = t.getTargetListFromEndpoints(ctx, servicePortNames, skipMatch)
		if err != nil {
			return err
		}
	}

	spec := model.TargetsSpec{
		StackTargetGroupId: t.stackTgId,
		TargetList:         targetList,
	}

	_, err := model.NewTargets(t.stack, spec)
	if err != nil {
		return err
	}

	return nil
}

func (t *latticeTargetsModelBuildTask) getTargetListFromEndpoints(ctx context.Context, servicePortNames map[string]struct{}, skipMatch bool) ([]model.Target, error) {
	epSlices := &discoveryv1.EndpointSliceList{}
	if err := t.client.List(ctx, epSlices,
		client.InNamespace(t.service.Namespace),
		client.MatchingLabels{discoveryv1.LabelServiceName: t.service.Name}); err != nil {
		return nil, err
	}

	var targetList []model.Target
	for _, epSlice := range epSlices.Items {
		for _, port := range epSlice.Ports {
			// Note that the Endpoint's port name is from ServicePort, but the actual registered port
			// is from Pods(targets).
			if _, ok := servicePortNames[aws.StringValue(port.Name)]; ok || skipMatch {
				for _, ep := range epSlice.Endpoints {
					for _, address := range ep.Addresses {
						// Do not model terminating endpoints so that they can deregister.
						if aws.BoolValue(ep.Conditions.Terminating) {
							continue
						}
						target := model.Target{
							TargetIP: address,
							Port:     int64(aws.Int32Value(port.Port)),
							Ready:    aws.BoolValue(ep.Conditions.Ready),
						}
						if ep.TargetRef != nil && ep.TargetRef.Kind == "Pod" {
							target.TargetRef = types.NamespacedName{Namespace: ep.TargetRef.Namespace, Name: ep.TargetRef.Name}
						}
						targetList = append(targetList, target)
					}
				}
			}
		}
	}
	return targetList, nil
}

func (t *latticeTargetsModelBuildTask) getDefinedPorts() map[int32]struct{} {
	definedPorts := make(map[int32]struct{})

	isServiceExport := t.serviceExport != nil
	if isServiceExport {
		portsAnnotations := strings.Split(t.serviceExport.Annotations[portAnnotationsKey], ",")

		for _, portAnnotation := range portsAnnotations {
			if portAnnotation != "" {
				definedPort, err := strconv.ParseInt(portAnnotation, 10, 32)
				if err != nil {
					t.log.Infof(context.TODO(), "failed to read Annotations/Port: %s due to %s",
						t.serviceExport.Annotations[portAnnotationsKey], err)
				} else {
					definedPorts[int32(definedPort)] = struct{}{}
				}
			}
		}
	} else if t.backendRef.Port() != nil {
		backendRefPort := int32(*t.backendRef.Port())
		if backendRefPort != undefinedPort {
			definedPorts[backendRefPort] = struct{}{}
		}
	}
	return definedPorts
}

type latticeTargetsModelBuildTask struct {
	log           gwlog.Logger
	client        client.Client
	serviceExport *anv1alpha1.ServiceExport
	service       *corev1.Service
	backendRef    core.BackendRef
	stack         core.Stack
	stackTgId     string
}
