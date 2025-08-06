package lattice

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	mock_client "github.com/aws/aws-application-networking-k8s/mocks/controller-runtime/client"
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

func Test_ResolveHealthCheckConfig_WithPolicies(t *testing.T) {
	tests := []struct {
		name           string
		targetGroup    *model.TargetGroup
		policies       []anv1alpha1.TargetGroupPolicy
		expectedConfig *vpclattice.HealthCheckConfig
		expectError    bool
	}{
		{
			name: "ServiceExport target group with applicable Service policy",
			targetGroup: &model.TargetGroup{
				Spec: model.TargetGroupSpec{
					TargetGroupTagFields: model.TargetGroupTagFields{
						K8SSourceType:       model.SourceTypeSvcExport,
						K8SServiceName:      "test-service",
						K8SServiceNamespace: "test-namespace",
					},
				},
			},
			policies: []anv1alpha1.TargetGroupPolicy{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "service-policy",
						Namespace: "test-namespace",
					},
					Spec: anv1alpha1.TargetGroupPolicySpec{
						TargetRef: &gwv1alpha2.NamespacedPolicyTargetReference{
							Group: corev1.GroupName,
							Kind:  "Service",
							Name:  "test-service",
						},
						HealthCheck: &anv1alpha1.HealthCheckConfig{
							Enabled:                 aws.Bool(true),
							Path:                    aws.String("/api/health"),
							IntervalSeconds:         aws.Int64(15),
							TimeoutSeconds:          aws.Int64(10),
							HealthyThresholdCount:   aws.Int64(3),
							UnhealthyThresholdCount: aws.Int64(2),
							StatusMatch:             aws.String("200-299"),
							Protocol:                (*anv1alpha1.HealthCheckProtocol)(aws.String("HTTP")),
							ProtocolVersion:         (*anv1alpha1.HealthCheckProtocolVersion)(aws.String("HTTP1")),
						},
					},
				},
			},
			expectedConfig: &vpclattice.HealthCheckConfig{
				Enabled:                    aws.Bool(true),
				Path:                       aws.String("/api/health"),
				HealthCheckIntervalSeconds: aws.Int64(15),
				HealthCheckTimeoutSeconds:  aws.Int64(10),
				HealthyThresholdCount:      aws.Int64(3),
				UnhealthyThresholdCount:    aws.Int64(2),
				Matcher:                    &vpclattice.Matcher{HttpCode: aws.String("200-299")},
				Protocol:                   aws.String("HTTP"),
				ProtocolVersion:            aws.String("HTTP1"),
			},
			expectError: false,
		},
		{
			name: "ServiceExport target group with applicable ServiceExport policy",
			targetGroup: &model.TargetGroup{
				Spec: model.TargetGroupSpec{
					TargetGroupTagFields: model.TargetGroupTagFields{
						K8SSourceType:       model.SourceTypeSvcExport,
						K8SServiceName:      "test-service",
						K8SServiceNamespace: "test-namespace",
					},
				},
			},
			policies: []anv1alpha1.TargetGroupPolicy{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "serviceexport-policy",
						Namespace: "test-namespace",
					},
					Spec: anv1alpha1.TargetGroupPolicySpec{
						TargetRef: &gwv1alpha2.NamespacedPolicyTargetReference{
							Group: anv1alpha1.GroupName,
							Kind:  "ServiceExport",
							Name:  "test-service",
						},
						HealthCheck: &anv1alpha1.HealthCheckConfig{
							Enabled: aws.Bool(false),
							Path:    aws.String("/custom/health"),
							Port:    aws.Int64(9090),
						},
					},
				},
			},
			expectedConfig: &vpclattice.HealthCheckConfig{
				Enabled: aws.Bool(false),
				Path:    aws.String("/custom/health"),
				Port:    aws.Int64(9090),
			},
			expectError: false,
		},
		{
			name: "ServiceExport target group with multiple policies - Service takes precedence",
			targetGroup: &model.TargetGroup{
				Spec: model.TargetGroupSpec{
					TargetGroupTagFields: model.TargetGroupTagFields{
						K8SSourceType:       model.SourceTypeSvcExport,
						K8SServiceName:      "test-service",
						K8SServiceNamespace: "test-namespace",
					},
				},
			},
			policies: []anv1alpha1.TargetGroupPolicy{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "service-policy",
						Namespace: "test-namespace",
					},
					Spec: anv1alpha1.TargetGroupPolicySpec{
						TargetRef: &gwv1alpha2.NamespacedPolicyTargetReference{
							Group: corev1.GroupName,
							Kind:  "Service",
							Name:  "test-service",
						},
						HealthCheck: &anv1alpha1.HealthCheckConfig{
							Enabled: aws.Bool(true),
							Path:    aws.String("/service/health"),
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "serviceexport-policy",
						Namespace: "test-namespace",
					},
					Spec: anv1alpha1.TargetGroupPolicySpec{
						TargetRef: &gwv1alpha2.NamespacedPolicyTargetReference{
							Group: anv1alpha1.GroupName,
							Kind:  "ServiceExport",
							Name:  "test-service",
						},
						HealthCheck: &anv1alpha1.HealthCheckConfig{
							Enabled: aws.Bool(false),
							Path:    aws.String("/serviceexport/health"),
						},
					},
				},
			},
			expectedConfig: &vpclattice.HealthCheckConfig{
				Enabled: aws.Bool(true),
				Path:    aws.String("/service/health"),
			},
			expectError: false,
		},
		{
			name: "ServiceExport target group with no applicable policies",
			targetGroup: &model.TargetGroup{
				Spec: model.TargetGroupSpec{
					TargetGroupTagFields: model.TargetGroupTagFields{
						K8SSourceType:       model.SourceTypeSvcExport,
						K8SServiceName:      "test-service",
						K8SServiceNamespace: "test-namespace",
					},
				},
			},
			policies: []anv1alpha1.TargetGroupPolicy{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "other-policy",
						Namespace: "test-namespace",
					},
					Spec: anv1alpha1.TargetGroupPolicySpec{
						TargetRef: &gwv1alpha2.NamespacedPolicyTargetReference{
							Group: corev1.GroupName,
							Kind:  "Service",
							Name:  "other-service", // Different service
						},
						HealthCheck: &anv1alpha1.HealthCheckConfig{
							Enabled: aws.Bool(true),
							Path:    aws.String("/other/health"),
						},
					},
				},
			},
			expectedConfig: nil,
			expectError:    false,
		},
		{
			name: "Policy with nil health check should return nil",
			targetGroup: &model.TargetGroup{
				Spec: model.TargetGroupSpec{
					TargetGroupTagFields: model.TargetGroupTagFields{
						K8SSourceType:       model.SourceTypeSvcExport,
						K8SServiceName:      "test-service",
						K8SServiceNamespace: "test-namespace",
					},
				},
			},
			policies: []anv1alpha1.TargetGroupPolicy{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "service-policy",
						Namespace: "test-namespace",
					},
					Spec: anv1alpha1.TargetGroupPolicySpec{
						TargetRef: &gwv1alpha2.NamespacedPolicyTargetReference{
							Group: corev1.GroupName,
							Kind:  "Service",
							Name:  "test-service",
						},
						HealthCheck: nil, // No health check config
					},
				},
			},
			expectedConfig: nil,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Create scheme and add required types
			scheme := runtime.NewScheme()
			_ = anv1alpha1.AddToScheme(scheme)
			_ = corev1.AddToScheme(scheme)
			_ = gwv1alpha2.AddToScheme(scheme)

			// Convert policies to client.Object slice
			objects := make([]client.Object, len(tt.policies))
			for i, policy := range tt.policies {
				policyCopy := policy
				objects[i] = &policyCopy
			}

			// Create fake client with policies
			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(objects...).
				Build()

			log := gwlog.FallbackLogger
			resolver := NewHealthCheckConfigResolver(log, k8sClient)

			config, err := resolver.ResolveHealthCheckConfig(ctx, tt.targetGroup)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.expectedConfig, config)
		})
	}
}

func Test_ResolveHealthCheckConfig_ErrorHandling(t *testing.T) {
	tests := []struct {
		name        string
		targetGroup *model.TargetGroup
		expectError bool
	}{
		{
			name: "ServiceExport target group with policy resolution error",
			targetGroup: &model.TargetGroup{
				Spec: model.TargetGroupSpec{
					TargetGroupTagFields: model.TargetGroupTagFields{
						K8SSourceType:       model.SourceTypeSvcExport,
						K8SServiceName:      "test-service",
						K8SServiceNamespace: "test-namespace",
					},
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Create a mock controller and client that returns an error
			c := gomock.NewController(t)
			defer c.Finish()

			mockClient := mock_client.NewMockClient(c)
			mockClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(errors.New("policy resolution error")).AnyTimes()

			log := gwlog.FallbackLogger
			resolver := NewHealthCheckConfigResolver(log, mockClient)

			config, err := resolver.ResolveHealthCheckConfig(ctx, tt.targetGroup)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "failed to resolve TargetGroupPolicy")
			} else {
				assert.NoError(t, err)
			}

			assert.Nil(t, config)
		})
	}
}

func Test_ResolveHealthCheckConfig_ProtocolConversion(t *testing.T) {
	tests := []struct {
		name             string
		policyProtocol   *anv1alpha1.HealthCheckProtocol
		policyVersion    *anv1alpha1.HealthCheckProtocolVersion
		expectedProtocol *string
		expectedVersion  *string
	}{
		{
			name:             "HTTP protocol conversion",
			policyProtocol:   (*anv1alpha1.HealthCheckProtocol)(aws.String("HTTP")),
			policyVersion:    (*anv1alpha1.HealthCheckProtocolVersion)(aws.String("HTTP1")),
			expectedProtocol: aws.String("HTTP"),
			expectedVersion:  aws.String("HTTP1"),
		},
		{
			name:             "HTTPS protocol conversion",
			policyProtocol:   (*anv1alpha1.HealthCheckProtocol)(aws.String("HTTPS")),
			policyVersion:    (*anv1alpha1.HealthCheckProtocolVersion)(aws.String("HTTP2")),
			expectedProtocol: aws.String("HTTPS"),
			expectedVersion:  aws.String("HTTP2"),
		},
		{
			name:             "nil protocol should result in nil",
			policyProtocol:   nil,
			policyVersion:    nil,
			expectedProtocol: nil,
			expectedVersion:  nil,
		},
		{
			name:             "only protocol specified",
			policyProtocol:   (*anv1alpha1.HealthCheckProtocol)(aws.String("HTTP")),
			policyVersion:    nil,
			expectedProtocol: aws.String("HTTP"),
			expectedVersion:  nil,
		},
		{
			name:             "only version specified",
			policyProtocol:   nil,
			policyVersion:    (*anv1alpha1.HealthCheckProtocolVersion)(aws.String("HTTP1")),
			expectedProtocol: nil,
			expectedVersion:  aws.String("HTTP1"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := &anv1alpha1.TargetGroupPolicy{
				Spec: anv1alpha1.TargetGroupPolicySpec{
					HealthCheck: &anv1alpha1.HealthCheckConfig{
						Enabled:         aws.Bool(true),
						Protocol:        tt.policyProtocol,
						ProtocolVersion: tt.policyVersion,
					},
				},
			}

			log := gwlog.FallbackLogger
			resolver := NewHealthCheckConfigResolver(log, nil)

			config := resolver.convertPolicyToHealthCheckConfig(policy)

			assert.NotNil(t, config)
			assert.Equal(t, tt.expectedProtocol, config.Protocol)
			assert.Equal(t, tt.expectedVersion, config.ProtocolVersion)
		})
	}
}
