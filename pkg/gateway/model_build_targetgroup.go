package gateway

import (
	"context"
	"errors"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/aws/aws-sdk-go/service/vpclattice"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s/policyhelper"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

type SvcExportTargetGroupModelBuilder interface {
	// used during standard model build
	Build(ctx context.Context, svcExport *anv1alpha1.ServiceExport) (core.Stack, error)

	// used for reconciliation of existing target groups against a service export object
	BuildTargetGroup(ctx context.Context, svcExport *anv1alpha1.ServiceExport) (*model.TargetGroup, error)
}

type SvcExportTargetGroupBuilder struct {
	log    gwlog.Logger
	client client.Client
}

func NewSvcExportTargetGroupBuilder(
	log gwlog.Logger,
	client client.Client,
) *SvcExportTargetGroupBuilder {
	return &SvcExportTargetGroupBuilder{
		log:    log,
		client: client,
	}
}

type svcExportTargetGroupModelBuildTask struct {
	log           gwlog.Logger
	client        client.Client
	serviceExport *anv1alpha1.ServiceExport
	stack         core.Stack
}

func (b *SvcExportTargetGroupBuilder) Build(
	ctx context.Context,
	svcExport *anv1alpha1.ServiceExport,
) (core.Stack, error) {
	stack := core.NewDefaultStack(core.StackID(k8s.NamespacedName(svcExport)))

	task := &svcExportTargetGroupModelBuildTask{
		log:           b.log,
		serviceExport: svcExport,
		stack:         stack,
		client:        b.client,
	}

	if err := task.run(ctx); err != nil {
		return nil, err
	}

	return task.stack, nil
}

func (b *SvcExportTargetGroupBuilder) BuildTargetGroup(ctx context.Context, svcExport *anv1alpha1.ServiceExport) (*model.TargetGroup, error) {
	stack := core.NewDefaultStack(core.StackID(k8s.NamespacedName(svcExport)))

	task := &svcExportTargetGroupModelBuildTask{
		log:           b.log,
		serviceExport: svcExport,
		stack:         stack,
		client:        b.client,
	}

	return task.buildTargetGroup(ctx)
}

func (t *svcExportTargetGroupModelBuildTask) run(ctx context.Context) error {
	tg, err := t.buildTargetGroup(ctx)
	if err != nil {
		return fmt.Errorf("failed to build target group for service export %s-%s due to %w",
			t.serviceExport.Name, t.serviceExport.Namespace, err)
	}

	if !tg.IsDeleted {
		err = t.buildTargets(ctx, tg.ID())
		if err != nil {
			t.log.Debugf("Failed to build targets for service export %s-%s due to %s",
				t.serviceExport.Name, t.serviceExport.Namespace, err)
			return err
		}
	}

	return nil
}

func (t *svcExportTargetGroupModelBuildTask) buildTargets(ctx context.Context, stackTgId string) error {
	targetsBuilder := NewTargetsBuilder(t.log, t.client, t.stack)
	_, err := targetsBuilder.BuildForServiceExport(ctx, t.serviceExport, stackTgId)
	if err != nil {
		return err
	}
	return nil
}

func (t *svcExportTargetGroupModelBuildTask) buildTargetGroup(ctx context.Context) (*model.TargetGroup, error) {
	svc := &corev1.Service{}
	noSvcFoundAndDeleting := false
	if err := t.client.Get(ctx, k8s.NamespacedName(t.serviceExport), svc); err != nil {
		if apierrors.IsNotFound(err) && !t.serviceExport.DeletionTimestamp.IsZero() {
			// if we're deleting, it's OK if the service isn't there
			noSvcFoundAndDeleting = true
		} else { // either it's some other error or we aren't deleting
			return nil, fmt.Errorf("Failed to find corresponding k8sService %s, error :%w ", k8s.NamespacedName(t.serviceExport), err)
		}
	}

	var ipAddressType string
	var err error
	if noSvcFoundAndDeleting {
		ipAddressType = "IPV4" // just pick a default
	} else {
		ipAddressType, err = buildTargetGroupIpAddressType(svc)
		if err != nil {
			return nil, err
		}
	}

	tgps, err := policyhelper.GetAttachedPolicies(ctx, t.client, k8s.NamespacedName(t.serviceExport), &anv1alpha1.TargetGroupPolicy{})
	if err != nil {
		return nil, err
	}

	protocol := "HTTP"
	protocolVersion := vpclattice.TargetGroupProtocolVersionHttp1
	var healthCheckConfig *vpclattice.HealthCheckConfig
	if len(tgps) > 0 {
		// TODO: TGP conflicts should be handled correctly w/ status update, for now just picking up one
		tgp := tgps[0]
		if tgp.Spec.Protocol != nil {
			protocol = *tgp.Spec.Protocol
		}
		if tgp.Spec.ProtocolVersion != nil {
			protocolVersion = *tgp.Spec.ProtocolVersion
		}
		healthCheckConfig = parseHealthCheckConfig(tgp)
	}

	spec := model.TargetGroupSpec{
		Type:              model.TargetGroupTypeIP,
		Port:              80,
		Protocol:          protocol,
		ProtocolVersion:   protocolVersion,
		IpAddressType:     ipAddressType,
		HealthCheckConfig: healthCheckConfig,
	}
	spec.VpcId = config.VpcID
	spec.K8SSourceType = model.SourceTypeSvcExport
	spec.K8SClusterName = config.ClusterName
	spec.K8SServiceName = t.serviceExport.Name
	spec.K8SServiceNamespace = t.serviceExport.Namespace

	stackTG, err := model.NewTargetGroup(t.stack, spec)
	if err != nil {
		return nil, err
	}

	stackTG.IsDeleted = !t.serviceExport.DeletionTimestamp.IsZero()
	return stackTG, nil
}

type BackendRefTargetGroupModelBuilder interface {
	Build(ctx context.Context, route core.Route, backendRef core.BackendRef, stack core.Stack) (core.Stack, *model.TargetGroup, error)
}

type BackendRefTargetGroupBuilder struct {
	log    gwlog.Logger
	client client.Client
}

func NewBackendRefTargetGroupBuilder(log gwlog.Logger, client client.Client) BackendRefTargetGroupModelBuilder {
	return &BackendRefTargetGroupBuilder{
		log:    log,
		client: client,
	}
}

type backendRefTargetGroupModelBuildTask struct {
	log        gwlog.Logger
	client     client.Client
	stack      core.Stack
	route      core.Route
	backendRef core.BackendRef
}

func (b *BackendRefTargetGroupBuilder) Build(
	ctx context.Context,
	route core.Route,
	backendRef core.BackendRef,
	stack core.Stack,
) (core.Stack, *model.TargetGroup, error) {
	if stack == nil {
		stack = core.NewDefaultStack(core.StackID(k8s.NamespacedName(route.K8sObject())))
		b.log.Debugf("Creating new stack for build task")
	}

	task := backendRefTargetGroupModelBuildTask{
		log:        b.log,
		client:     b.client,
		stack:      stack,
		route:      route,
		backendRef: backendRef,
	}

	stackTg, err := task.buildTargetGroup(ctx)
	if err != nil {
		return nil, nil, err
	}
	return task.stack, stackTg, nil
}

func (t *backendRefTargetGroupModelBuildTask) buildTargetGroup(ctx context.Context) (*model.TargetGroup, error) {
	if string(*t.backendRef.Kind()) == "ServiceImport" {
		return nil, errors.New("not supported for ServiceImport BackendRef")
	}

	tgSpec, err := t.buildTargetGroupSpec(ctx)
	if err != nil {
		return nil, fmt.Errorf("buildTargetGroupSpec err %w", err)
	}

	stackTG, err := model.NewTargetGroup(t.stack, tgSpec)
	if err != nil {
		return nil, err
	}
	t.log.Debugf("Added target group for backendRef %s to the stack %s", t.backendRef.Name(), stackTG.ID())

	stackTG.IsDeleted = !t.route.DeletionTimestamp().IsZero()
	if !stackTG.IsDeleted {
		t.buildTargets(ctx, stackTG.ID())
	}

	return stackTG, nil
}

func (t *backendRefTargetGroupModelBuildTask) buildTargets(ctx context.Context, stackTgId string) error {
	if string(*t.backendRef.Kind()) == "ServiceImport" {
		t.log.Debugf("Service import does not manage targets, returning")
		return nil
	}

	backendRefNsName := getBackendRefNsName(t.route, t.backendRef)
	svc := &corev1.Service{}
	if err := t.client.Get(ctx, backendRefNsName, svc); err != nil {
		if apierrors.IsNotFound(err) && !t.route.DeletionTimestamp().IsZero() {
			t.log.Infof("Ignoring not found error for service %s on deleted route %s",
				t.backendRef.Name(), t.route.Name())
		} else {
			return fmt.Errorf("error finding backend service %s due to %s", backendRefNsName, err)
		}
	}

	targetsBuilder := NewTargetsBuilder(t.log, t.client, t.stack)
	_, err := targetsBuilder.Build(ctx, svc, t.backendRef, stackTgId)
	if err != nil {
		return err
	}

	return nil
}

// Now, Only k8sService and serviceImport creation deletion use this function to build TargetGroupSpec, serviceExport does not use this function to create TargetGroupSpec
func (t *backendRefTargetGroupModelBuildTask) buildTargetGroupSpec(ctx context.Context) (model.TargetGroupSpec, error) {
	backendKind := string(*t.backendRef.Kind())
	t.log.Debugf("buildTargetGroupSpec, kind %s", backendKind)

	vpc := config.VpcID
	eksCluster := config.ClusterName
	routeIsDeleted := !t.route.DeletionTimestamp().IsZero()

	backendRefNsName := getBackendRefNsName(t.route, t.backendRef)
	svc := &corev1.Service{}
	if err := t.client.Get(ctx, backendRefNsName, svc); err != nil {
		if routeIsDeleted && apierrors.IsNotFound(err) {
			t.log.Infof("Ignoring not found error for service %s on deleted route %s",
				t.backendRef.Name(), t.route.Name())
		} else if !routeIsDeleted {
			return model.TargetGroupSpec{},
				fmt.Errorf("error finding backend service %s due to %s", backendRefNsName, err)
		}
	}

	ipAddressType := vpclattice.IpAddressTypeIpv4
	var err error
	if svc != nil {
		ipAddressType, err = buildTargetGroupIpAddressType(svc)
		if err != nil {
			if routeIsDeleted {
				// Ignore error for deletion request
				t.log.Debugf("Unable to determine IP address type for deleted route, using default")
				ipAddressType = vpclattice.IpAddressTypeIpv4
			} else {
				// we care that there's an error if we are not deleting
				return model.TargetGroupSpec{}, err
			}
		}
	}

	tgps, err := policyhelper.GetAttachedPolicies(ctx, t.client, backendRefNsName, &anv1alpha1.TargetGroupPolicy{})
	if err != nil {
		return model.TargetGroupSpec{}, err
	}

	protocol := "HTTP"
	protocolVersion := vpclattice.TargetGroupProtocolVersionHttp1
	var healthCheckConfig *vpclattice.HealthCheckConfig
	if len(tgps) > 0 {
		// TODO: TGP conflicts should be handled correctly w/ status update, for now just picking up one
		tgp := tgps[0]
		if tgp.Spec.Protocol != nil {
			protocol = *tgp.Spec.Protocol
		}

		if tgp.Spec.ProtocolVersion != nil {
			protocolVersion = *tgp.Spec.ProtocolVersion
		}
		healthCheckConfig = parseHealthCheckConfig(tgp)
	}

	// GRPC takes precedence over other protocolVersions.
	parentRefType := model.SourceTypeHTTPRoute
	if _, ok := t.route.(*core.GRPCRoute); ok {
		protocolVersion = vpclattice.TargetGroupProtocolVersionGrpc
		parentRefType = model.SourceTypeGRPCRoute
	}

	spec := model.TargetGroupSpec{
		Type:              model.TargetGroupTypeIP,
		Port:              80,
		Protocol:          protocol,
		ProtocolVersion:   protocolVersion,
		IpAddressType:     ipAddressType,
		HealthCheckConfig: healthCheckConfig,
	}
	spec.VpcId = vpc
	spec.K8SSourceType = parentRefType
	spec.K8SClusterName = eksCluster
	spec.K8SServiceName = backendRefNsName.Name
	spec.K8SServiceNamespace = backendRefNsName.Namespace
	spec.K8SRouteName = t.route.Name()
	spec.K8SRouteNamespace = t.route.Namespace()

	return spec, nil
}

func getBackendRefNsName(route core.Route, backendRef core.BackendRef) types.NamespacedName {
	var namespace = route.Namespace()
	if backendRef.Namespace() != nil {
		namespace = string(*backendRef.Namespace())
	}

	backendRefNsName := types.NamespacedName{
		Namespace: namespace,
		Name:      string(backendRef.Name()),
	}
	return backendRefNsName
}

func parseHealthCheckConfig(tgp *anv1alpha1.TargetGroupPolicy) *vpclattice.HealthCheckConfig {
	hc := tgp.Spec.HealthCheck
	if hc == nil {
		return nil
	}
	var matcher *vpclattice.Matcher
	if hc.StatusMatch != nil {
		matcher = &vpclattice.Matcher{HttpCode: hc.StatusMatch}
	}
	return &vpclattice.HealthCheckConfig{
		Enabled:                    hc.Enabled,
		HealthCheckIntervalSeconds: hc.IntervalSeconds,
		HealthCheckTimeoutSeconds:  hc.TimeoutSeconds,
		HealthyThresholdCount:      hc.HealthyThresholdCount,
		UnhealthyThresholdCount:    hc.UnhealthyThresholdCount,
		Matcher:                    matcher,
		Path:                       hc.Path,
		Port:                       hc.Port,
		Protocol:                   (*string)(hc.Protocol),
		ProtocolVersion:            (*string)(hc.ProtocolVersion),
	}
}

func buildTargetGroupIpAddressType(svc *corev1.Service) (string, error) {
	ipFamilies := svc.Spec.IPFamilies

	if len(ipFamilies) != 1 {
		return "", errors.New("Lattice Target Group only supports single stack IP addresses")
	}

	// IpFamilies will always have at least 1 element
	ipFamily := ipFamilies[0]

	switch ipFamily {
	case corev1.IPv4Protocol:
		return vpclattice.IpAddressTypeIpv4, nil
	case corev1.IPv6Protocol:
		return vpclattice.IpAddressTypeIpv6, nil
	default:
		return "", fmt.Errorf("unknown ipFamily: %s", ipFamily)
	}
}

func GetServiceForBackendRef(ctx context.Context, client client.Client, route core.Route, backendRef core.BackendRef) (*corev1.Service, error) {
	svc := &corev1.Service{}
	key := types.NamespacedName{
		Name: string(backendRef.Name()),
	}

	if backendRef.Namespace() != nil {
		key.Namespace = string(*backendRef.Namespace())
	} else {
		key.Namespace = route.Namespace()
	}

	if err := client.Get(ctx, key, svc); err != nil {
		return nil, err
	}

	return svc, nil
}
