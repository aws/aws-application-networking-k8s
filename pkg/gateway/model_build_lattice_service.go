package gateway

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/service/vpclattice"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"

	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

//go:generate mockgen -destination model_build_lattice_service_mock.go -package gateway github.com/aws/aws-application-networking-k8s/pkg/gateway LatticeServiceBuilder

type LatticeServiceBuilder interface {
	Build(ctx context.Context, httpRoute core.Route) (core.Stack, error)
}

type LatticeServiceModelBuilder struct {
	log         gwlog.Logger
	client      client.Client
	brTgBuilder BackendRefTargetGroupModelBuilder
}

func NewLatticeServiceBuilder(
	log gwlog.Logger,
	client client.Client,
	brTgBuilder BackendRefTargetGroupModelBuilder,
) *LatticeServiceModelBuilder {
	return &LatticeServiceModelBuilder{
		log:         log,
		client:      client,
		brTgBuilder: brTgBuilder,
	}
}

func (b *LatticeServiceModelBuilder) Build(
	ctx context.Context,
	route core.Route,
) (core.Stack, error) {
	stack := core.NewDefaultStack(core.StackID(k8s.NamespacedName(route.K8sObject())))

	task := &latticeServiceModelBuildTask{
		log:         b.log,
		route:       route,
		stack:       stack,
		client:      b.client,
		brTgBuilder: b.brTgBuilder,
	}

	if err := task.run(ctx); err != nil {
		return task.stack, err
	}

	return task.stack, nil
}

func (t *latticeServiceModelBuildTask) run(ctx context.Context) error {
	err := t.buildModel(ctx)
	return err
}

func (t *latticeServiceModelBuildTask) buildModel(ctx context.Context) error {
	modelSvc, err := t.buildLatticeService(ctx)
	if err != nil {
		return err
	}

	err = t.buildListeners(ctx, modelSvc.ID())
	if err != nil {
		return fmt.Errorf("failed to build listener due to %w", err)
	}

	var modelListeners []*model.Listener
	err = t.stack.ListResources(&modelListeners)
	if err != nil {
		return err
	}
	t.log.Debugf(ctx, "Building rules for %d listeners", len(modelListeners))
	for _, modelListener := range modelListeners {
		if modelListener.Spec.Protocol == vpclattice.ListenerProtocolTlsPassthrough {
			t.log.Debugf(ctx, "Skip building rules for TLS_PASSTHROUGH listener %s, since lattice TLS_PASSTHROUGH listener can only have listener defaultAction and without any other rule", modelListener.ID())
			continue
		}

		// building rules will also build target groups and targets as needed
		// even on delete we try to build everything we may then need to remove
		err = t.buildRules(ctx, modelListener.ID())
		if err != nil {
			return fmt.Errorf("failed to build rules due to %w", err)
		}
	}

	return nil
}

func (t *latticeServiceModelBuildTask) buildLatticeService(ctx context.Context) (*model.Service, error) {
	var routeType core.RouteType
	switch t.route.(type) {
	case *core.HTTPRoute:
		routeType = core.HttpRouteType
	case *core.GRPCRoute:
		routeType = core.GrpcRouteType
	case *core.TLSRoute:
		routeType = core.TlsRouteType
		// VPC Lattice requires a custom domain name for TLS listeners
		if len(t.route.Spec().Hostnames()) == 0 {
			return nil, fmt.Errorf("TLSRoute %s/%s must specify at least one hostname as VPC Lattice requires a custom domain name",
				t.route.Namespace(), t.route.Name())
		}
	default:
		return nil, fmt.Errorf("unsupported route type: %T", t.route)
	}

	spec := model.ServiceSpec{
		ServiceTagFields: model.ServiceTagFields{
			RouteName:      t.route.Name(),
			RouteNamespace: t.route.Namespace(),
			RouteType:      routeType,
		},
	}

	spec.AdditionalTags = k8s.GetAdditionalTagsFromAnnotations(ctx, t.route.K8sObject())

	// Check if standalone mode is enabled for this route
	standalone, err := t.isStandaloneMode(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to determine standalone mode: %w", err)
	}

	t.log.Infof(ctx, "Standalone mode determination for route %s/%s: %t",
		t.route.Namespace(), t.route.Name(), standalone)

	if !standalone {
		// Standard mode: populate ServiceNetworkNames from parent references
		for _, parentRef := range t.route.Spec().ParentRefs() {
			gw := &gwv1.Gateway{}
			parentNamespace := t.route.Namespace()
			if parentRef.Namespace != nil {
				parentNamespace = string(*parentRef.Namespace)
			}
			err := t.client.Get(ctx, client.ObjectKey{Name: string(parentRef.Name), Namespace: parentNamespace}, gw)
			if err != nil {
				t.log.Infof(ctx, "Ignoring route %s because failed to get gateway %s: %v", t.route.Name(), gw.Spec.GatewayClassName, err)
				continue
			}
			if k8s.IsControlledByLatticeGatewayController(ctx, t.client, gw) {
				spec.ServiceNetworkNames = append(spec.ServiceNetworkNames, string(parentRef.Name))
			} else {
				t.log.Infof(ctx, "Ignoring route %s because gateway %s is not managed by lattice gateway controller", t.route.Name(), gw.Name)
			}
		}
		if config.ServiceNetworkOverrideMode {
			spec.ServiceNetworkNames = []string{config.DefaultServiceNetwork}
		}

		t.log.Infof(ctx, "Creating service with service network association for route %s-%s (networks: %v)",
			t.route.Name(), t.route.Namespace(), spec.ServiceNetworkNames)
	} else {
		// Standalone mode: empty ServiceNetworkNames (no service network association)
		spec.ServiceNetworkNames = []string{}
		t.log.Infof(ctx, "Creating standalone service for route %s-%s (no service network association)",
			t.route.Name(), t.route.Namespace())
	}

	if len(t.route.Spec().Hostnames()) > 0 {
		// The 1st hostname will be used as lattice customer-domain-name
		spec.CustomerDomainName = string(t.route.Spec().Hostnames()[0])

		t.log.Infof(ctx, "Setting customer-domain-name: %s for route %s-%s",
			spec.CustomerDomainName, t.route.Name(), t.route.Namespace())
	} else {
		t.log.Infof(ctx, "No custom-domain-name for route %s-%s",
			t.route.Name(), t.route.Namespace())
		spec.CustomerDomainName = ""
	}

	certArn, err := t.getACMCertArn(ctx)
	if err != nil {
		return nil, err
	}
	spec.CustomerCertARN = certArn

	allowTakeoverFrom, err := t.getAllowTakeoverFrom(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get takeover annotation: %w", err)
	}
	spec.AllowTakeoverFrom = allowTakeoverFrom

	// Check for service name override on route first (highest precedence)
	serviceNameOverride, err := k8s.GetServiceNameOverrideWithValidation(t.route.K8sObject())
	if err != nil {
		return nil, err
	}

	// If not set on route, check the Gateway's lattice-service-name annotation
	if serviceNameOverride == "" {
		gw, gwErr := t.findGateway(ctx)
		if gwErr == nil && gw != nil {
			gwAnnotations := gw.GetAnnotations()
			if gwAnnotations != nil {
				// Check for the lattice-service-name annotation on the Gateway
				if latticeServiceName, exists := gwAnnotations[k8s.AnnotationPrefix+"lattice-service-name"]; exists && latticeServiceName != "" {
					if validateErr := k8s.ValidateVPCLatticeServiceName(latticeServiceName); validateErr != nil {
						return nil, k8s.NewInvalidServiceNameOverrideError(latticeServiceName, validateErr.Error())
					}
					serviceNameOverride = latticeServiceName
					t.log.Debugf(ctx, "Using lattice-service-name from Gateway %s/%s: %s",
						gw.GetNamespace(), gw.GetName(), latticeServiceName)
				}
			}
		}
	}

	spec.ServiceNameOverride = serviceNameOverride

	svc, err := model.NewLatticeService(t.stack, spec)
	if err != nil {
		return nil, err
	}

	t.log.Debugf(ctx, "Added service %s to the stack (ID %s)", svc.Spec.LatticeServiceName(), svc.ID())
	svc.IsDeleted = !t.route.DeletionTimestamp().IsZero()
	return svc, nil
}

// returns empty string if not found
func (t *latticeServiceModelBuildTask) getACMCertArn(ctx context.Context) (string, error) {
	// when a service is associate to multiple service network(s), all listener config MUST be same
	// so here we are only using the 1st gateway
	gw, err := t.findGateway(ctx)
	if err != nil {
		if apierrors.IsNotFound(err) && !t.route.DeletionTimestamp().IsZero() {
			return "", nil // ok if we're deleting the route
		}
		return "", err
	}

	for _, parentRef := range t.route.Spec().ParentRefs() {
		if string(parentRef.Name) != gw.Name {
			t.log.Debugf(ctx, "Ignore ParentRef of different gateway %s", parentRef.Name)
			continue
		}

		if parentRef.SectionName == nil {
			continue
		}

		for _, section := range gw.Spec.Listeners {
			if section.Name == *parentRef.SectionName && section.TLS != nil {
				if section.TLS.Mode != nil && *section.TLS.Mode == gwv1.TLSModeTerminate {
					curCertARN, ok := section.TLS.Options[awsCustomCertARN]
					if ok {
						t.log.Debugf(ctx, "Found certification %s under section %s", curCertARN, section.Name)
						return string(curCertARN), nil
					}
				}
				break
			}
		}
	}

	return "", nil
}

type latticeServiceModelBuildTask struct {
	log         gwlog.Logger
	route       core.Route
	client      client.Client
	stack       core.Stack
	brTgBuilder BackendRefTargetGroupModelBuilder
}

// isStandaloneMode determines if standalone mode should be enabled for the route.
// It uses enhanced validation and error handling to gracefully handle annotation
// parsing errors and gateway lookup failures.
func (t *latticeServiceModelBuildTask) isStandaloneMode(ctx context.Context) (bool, error) {
	// Use the enhanced validation function for better error reporting
	standalone, warnings, err := k8s.GetStandaloneModeForRouteWithValidation(ctx, t.client, t.route)

	// Log any validation warnings
	for _, warning := range warnings {
		t.log.Warnf(ctx, "Standalone mode validation warning for route %s/%s: %s",
			t.route.Namespace(), t.route.Name(), warning)
	}

	// Add debug logging for gateway lookup
	t.log.Debugf(ctx, "Checking standalone mode for route %s/%s with %d parent refs",
		t.route.Namespace(), t.route.Name(), len(t.route.Spec().ParentRefs()))

	if err != nil {
		// Log the error but check if we can continue with a safe default
		t.log.Errorf(ctx, "Failed to determine standalone mode for route %s/%s: %v",
			t.route.Namespace(), t.route.Name(), err)

		// For critical errors, we should fail the operation
		return false, fmt.Errorf("standalone mode determination failed: %w", err)
	}

	t.log.Debugf(ctx, "Standalone mode for route %s/%s: %t",
		t.route.Namespace(), t.route.Name(), standalone)
	return standalone, nil
}

func (t *latticeServiceModelBuildTask) getAllowTakeoverFrom(ctx context.Context) (string, error) {
	annotations := t.route.K8sObject().GetAnnotations()
	if annotations == nil {
		return "", nil
	}

	takeoverFrom := annotations[k8s.AllowTakeoverFromAnnotation]
	if takeoverFrom == "" {
		return "", nil
	}

	t.log.Debugf(ctx, "Found allow-takeover-from annotation: %s for route %s/%s",
		takeoverFrom, t.route.Namespace(), t.route.Name())

	return takeoverFrom, nil
}
