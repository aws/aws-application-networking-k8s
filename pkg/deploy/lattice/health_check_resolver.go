package lattice

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/service/vpclattice"
	"sigs.k8s.io/controller-runtime/pkg/client"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s/policyhelper"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

// HealthCheckConfigResolver provides centralized logic for resolving health check configuration
// from TargetGroupPolicy resources for target groups.
type HealthCheckConfigResolver struct {
	log    gwlog.Logger
	client client.Client
}

// NewHealthCheckConfigResolver creates a new health check configuration resolver.
func NewHealthCheckConfigResolver(log gwlog.Logger, client client.Client) *HealthCheckConfigResolver {
	return &HealthCheckConfigResolver{
		log:    log,
		client: client,
	}
}

// ResolveHealthCheckConfig takes a target group model and returns the appropriate health check configuration
// by checking for applicable TargetGroupPolicy resources. It implements proper error handling and fallback
// to default configuration when policy resolution fails, ensuring existing behavior is preserved when no
// policies are applicable.
//
// This function:
// 1. Checks if the target group is eligible for policy-based health check configuration
// 2. Attempts to resolve applicable TargetGroupPolicy resources for the target group's service
// 3. Converts TargetGroupPolicy health check specification to VPC Lattice health check configuration format
// 4. Falls back gracefully to nil (allowing caller to apply defaults) when policy resolution fails
// 5. Preserves existing behavior when no policies are applicable
func (r *HealthCheckConfigResolver) ResolveHealthCheckConfig(ctx context.Context, targetGroup *model.TargetGroup) (*vpclattice.HealthCheckConfig, error) {
	if r.client == nil {
		r.log.Debugf(ctx, "No client available for policy resolution, skipping health check config resolution")
		return nil, nil
	}

	// Only resolve health check configuration from policies for ServiceExport target groups
	// This maintains backwards compatibility and follows the existing pattern where only
	// ServiceExport target groups need cross-cluster health check configuration synchronization
	if targetGroup.Spec.K8SSourceType != model.SourceTypeSvcExport {
		r.log.Debugf(ctx, "Target group source type is %s, skipping policy-based health check resolution", targetGroup.Spec.K8SSourceType)
		return nil, nil
	}

	// Create policy handler for TargetGroupPolicy
	tgpHandler := policyhelper.NewTargetGroupPolicyHandler(r.log, r.client)

	// Try to resolve TargetGroupPolicy for the service
	// This will check both direct Service targets and ServiceExport targets with the same name/namespace
	tgp, err := tgpHandler.FindPolicyForService(ctx, targetGroup.Spec.K8SServiceName, targetGroup.Spec.K8SServiceNamespace)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve TargetGroupPolicy for service %s/%s: %w",
			targetGroup.Spec.K8SServiceNamespace, targetGroup.Spec.K8SServiceName, err)
	}

	if tgp == nil {
		r.log.Debugf(ctx, "No TargetGroupPolicy found for service %s/%s",
			targetGroup.Spec.K8SServiceNamespace, targetGroup.Spec.K8SServiceName)
		return nil, nil
	}

	// Convert TargetGroupPolicy health check specification to VPC Lattice health check configuration format
	healthCheckConfig := r.convertPolicyToHealthCheckConfig(tgp)
	if healthCheckConfig != nil {
		r.log.Debugf(ctx, "Resolved health check configuration from TargetGroupPolicy %s/%s for service %s/%s",
			tgp.Namespace, tgp.Name, targetGroup.Spec.K8SServiceNamespace, targetGroup.Spec.K8SServiceName)
	}

	return healthCheckConfig, nil
}

// convertPolicyToHealthCheckConfig converts TargetGroupPolicy health check specification to VPC Lattice health check configuration format.
// This function handles the conversion between the Kubernetes API types and the AWS SDK types, ensuring proper
// type casting and nil handling for optional fields.
//
// The conversion preserves all health check parameters from the policy:
// - Enabled flag
// - Interval and timeout settings
// - Healthy/unhealthy threshold counts
// - Status match patterns (converted to VPC Lattice Matcher format)
// - Health check path and port
// - Protocol and protocol version
func (r *HealthCheckConfigResolver) convertPolicyToHealthCheckConfig(tgp *anv1alpha1.TargetGroupPolicy) *vpclattice.HealthCheckConfig {
	if tgp == nil {
		return nil
	}

	hc := tgp.Spec.HealthCheck
	if hc == nil {
		return nil
	}

	// Convert status match pattern to VPC Lattice Matcher format
	var matcher *vpclattice.Matcher
	if hc.StatusMatch != nil {
		matcher = &vpclattice.Matcher{HttpCode: hc.StatusMatch}
	}

	// Convert TargetGroupPolicy health check types to VPC Lattice types
	// The TargetGroupPolicy uses custom enum types that need to be cast to string pointers
	var protocol *string
	if hc.Protocol != nil {
		protocolStr := string(*hc.Protocol)
		protocol = &protocolStr
	}

	var protocolVersion *string
	if hc.ProtocolVersion != nil {
		protocolVersionStr := string(*hc.ProtocolVersion)
		protocolVersion = &protocolVersionStr
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
		Protocol:                   protocol,
		ProtocolVersion:            protocolVersion,
	}
}
