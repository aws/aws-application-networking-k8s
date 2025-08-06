package lattice

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

func TestHealthCheckConfigResolver_ResolveHealthCheckConfig(t *testing.T) {
	tests := []struct {
		name           string
		targetGroup    *model.TargetGroup
		expectedConfig *vpclattice.HealthCheckConfig
	}{
		{
			name: "ServiceExport target group with no applicable policy",
			targetGroup: &model.TargetGroup{
				Spec: model.TargetGroupSpec{
					TargetGroupTagFields: model.TargetGroupTagFields{
						K8SSourceType:       model.SourceTypeSvcExport,
						K8SServiceName:      "test-service",
						K8SServiceNamespace: "test-namespace",
					},
				},
			},
			expectedConfig: nil,
		},
		{
			name: "HTTPRoute target group should skip policy resolution",
			targetGroup: &model.TargetGroup{
				Spec: model.TargetGroupSpec{
					TargetGroupTagFields: model.TargetGroupTagFields{
						K8SSourceType:       model.SourceTypeHTTPRoute,
						K8SServiceName:      "test-service",
						K8SServiceNamespace: "test-namespace",
					},
				},
			},
			expectedConfig: nil,
		},
		{
			name: "GRPCRoute target group should skip policy resolution",
			targetGroup: &model.TargetGroup{
				Spec: model.TargetGroupSpec{
					TargetGroupTagFields: model.TargetGroupTagFields{
						K8SSourceType:       model.SourceTypeGRPCRoute,
						K8SServiceName:      "test-service",
						K8SServiceNamespace: "test-namespace",
					},
				},
			},
			expectedConfig: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Create scheme and add required types
			scheme := runtime.NewScheme()
			_ = anv1alpha1.AddToScheme(scheme)
			_ = corev1.AddToScheme(scheme)

			// Create fake client with no objects
			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				Build()

			log := gwlog.FallbackLogger
			resolver := NewHealthCheckConfigResolver(log, k8sClient)

			config, err := resolver.ResolveHealthCheckConfig(ctx, tt.targetGroup)

			assert.NoError(t, err)
			assert.Equal(t, tt.expectedConfig, config)
		})
	}
}

func TestHealthCheckConfigResolver_ResolveHealthCheckConfig_NoClient(t *testing.T) {
	ctx := context.Background()
	log := gwlog.FallbackLogger
	resolver := NewHealthCheckConfigResolver(log, nil)

	targetGroup := &model.TargetGroup{
		Spec: model.TargetGroupSpec{
			TargetGroupTagFields: model.TargetGroupTagFields{
				K8SSourceType:       model.SourceTypeSvcExport,
				K8SServiceName:      "test-service",
				K8SServiceNamespace: "test-namespace",
			},
		},
	}

	config, err := resolver.ResolveHealthCheckConfig(ctx, targetGroup)

	assert.NoError(t, err)
	assert.Nil(t, config)
}

func TestHealthCheckConfigResolver_convertPolicyToHealthCheckConfig(t *testing.T) {
	tests := []struct {
		name           string
		policy         *anv1alpha1.TargetGroupPolicy
		expectedConfig *vpclattice.HealthCheckConfig
	}{
		{
			name:           "nil policy should return nil",
			policy:         nil,
			expectedConfig: nil,
		},
		{
			name: "policy with nil health check should return nil",
			policy: &anv1alpha1.TargetGroupPolicy{
				Spec: anv1alpha1.TargetGroupPolicySpec{
					HealthCheck: nil,
				},
			},
			expectedConfig: nil,
		},
		{
			name: "policy with complete health check configuration",
			policy: &anv1alpha1.TargetGroupPolicy{
				Spec: anv1alpha1.TargetGroupPolicySpec{
					HealthCheck: &anv1alpha1.HealthCheckConfig{
						Enabled:                 aws.Bool(true),
						IntervalSeconds:         aws.Int64(15),
						TimeoutSeconds:          aws.Int64(10),
						HealthyThresholdCount:   aws.Int64(5),
						UnhealthyThresholdCount: aws.Int64(3),
						Path:                    aws.String("/api/health"),
						Port:                    aws.Int64(9090),
						StatusMatch:             aws.String("200"),
						Protocol:                (*anv1alpha1.HealthCheckProtocol)(aws.String("HTTPS")),
						ProtocolVersion:         (*anv1alpha1.HealthCheckProtocolVersion)(aws.String("HTTP2")),
					},
				},
			},
			expectedConfig: &vpclattice.HealthCheckConfig{
				Enabled:                    aws.Bool(true),
				HealthCheckIntervalSeconds: aws.Int64(15),
				HealthCheckTimeoutSeconds:  aws.Int64(10),
				HealthyThresholdCount:      aws.Int64(5),
				UnhealthyThresholdCount:    aws.Int64(3),
				Path:                       aws.String("/api/health"),
				Port:                       aws.Int64(9090),
				Matcher:                    &vpclattice.Matcher{HttpCode: aws.String("200")},
				Protocol:                   aws.String("HTTPS"),
				ProtocolVersion:            aws.String("HTTP2"),
			},
		},
		{
			name: "policy with partial health check configuration",
			policy: &anv1alpha1.TargetGroupPolicy{
				Spec: anv1alpha1.TargetGroupPolicySpec{
					HealthCheck: &anv1alpha1.HealthCheckConfig{
						Enabled: aws.Bool(false),
						Path:    aws.String("/status"),
						// Other fields are nil
					},
				},
			},
			expectedConfig: &vpclattice.HealthCheckConfig{
				Enabled: aws.Bool(false),
				Path:    aws.String("/status"),
				// Other fields should be nil
			},
		},
		{
			name: "policy with no status match should not create matcher",
			policy: &anv1alpha1.TargetGroupPolicy{
				Spec: anv1alpha1.TargetGroupPolicySpec{
					HealthCheck: &anv1alpha1.HealthCheckConfig{
						Enabled: aws.Bool(true),
						Path:    aws.String("/health"),
						// StatusMatch is nil
					},
				},
			},
			expectedConfig: &vpclattice.HealthCheckConfig{
				Enabled: aws.Bool(true),
				Path:    aws.String("/health"),
				Matcher: nil, // Should be nil when StatusMatch is not provided
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := gwlog.FallbackLogger
			resolver := NewHealthCheckConfigResolver(log, nil)

			config := resolver.convertPolicyToHealthCheckConfig(tt.policy)

			assert.Equal(t, tt.expectedConfig, config)
		})
	}
}
