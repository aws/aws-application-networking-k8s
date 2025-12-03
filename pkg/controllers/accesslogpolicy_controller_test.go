package controllers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
)

func TestAccessLogPolicyReconciler_targetRefToResourceName(t *testing.T) {
	r := &accessLogPolicyReconciler{}

	tests := []struct {
		name          string
		alp           *anv1alpha1.AccessLogPolicy
		targetObj     metav1.Object
		expected      string
		expectError   bool
		errorContains string
	}{
		{
			name: "Gateway targetRef returns gateway name",
			alp: &anv1alpha1.AccessLogPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
				},
				Spec: anv1alpha1.AccessLogPolicySpec{
					TargetRef: &gwv1alpha2.NamespacedPolicyTargetReference{
						Kind: "Gateway",
						Name: "test-gateway",
					},
				},
			},
			targetObj:   nil,
			expected:    "test-gateway",
			expectError: false,
		},
		{
			name: "HTTPRoute targetRef without namespace uses policy namespace",
			alp: &anv1alpha1.AccessLogPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "policy-namespace",
				},
				Spec: anv1alpha1.AccessLogPolicySpec{
					TargetRef: &gwv1alpha2.NamespacedPolicyTargetReference{
						Kind: "HTTPRoute",
						Name: "test-route",
					},
				},
			},
			targetObj: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "policy-namespace",
				},
			},
			expected:    "test-route-policy-namespace",
			expectError: false,
		},
		{
			name: "HTTPRoute targetRef with namespace uses specified namespace",
			alp: &anv1alpha1.AccessLogPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "policy-namespace",
				},
				Spec: anv1alpha1.AccessLogPolicySpec{
					TargetRef: &gwv1alpha2.NamespacedPolicyTargetReference{
						Kind:      "HTTPRoute",
						Name:      "test-route",
						Namespace: (*gwv1alpha2.Namespace)(&[]string{"route-namespace"}[0]),
					},
				},
			},
			targetObj: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "route-namespace",
				},
			},
			expected:    "test-route-route-namespace",
			expectError: false,
		},
		{
			name: "HTTPRoute targetRef with valid service name override",
			alp: &anv1alpha1.AccessLogPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
				},
				Spec: anv1alpha1.AccessLogPolicySpec{
					TargetRef: &gwv1alpha2.NamespacedPolicyTargetReference{
						Kind: "HTTPRoute",
						Name: "test-route",
					},
				},
			},
			targetObj: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "test-namespace",
					Annotations: map[string]string{
						k8s.ServiceNameOverrideAnnotation: "custom-service-name",
					},
				},
			},
			expected:    "custom-service-name",
			expectError: false,
		},
		{
			name: "HTTPRoute targetRef with invalid service name override",
			alp: &anv1alpha1.AccessLogPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
				},
				Spec: anv1alpha1.AccessLogPolicySpec{
					TargetRef: &gwv1alpha2.NamespacedPolicyTargetReference{
						Kind: "HTTPRoute",
						Name: "test-route",
					},
				},
			},
			targetObj: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "test-namespace",
					Annotations: map[string]string{
						k8s.ServiceNameOverrideAnnotation: "svc-invalid-name",
					},
				},
			},
			expected:      "",
			expectError:   true,
			errorContains: "invalid service name override",
		},
		{
			name: "GRPCRoute targetRef without namespace uses policy namespace",
			alp: &anv1alpha1.AccessLogPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "policy-namespace",
				},
				Spec: anv1alpha1.AccessLogPolicySpec{
					TargetRef: &gwv1alpha2.NamespacedPolicyTargetReference{
						Kind: "GRPCRoute",
						Name: "test-grpc-route",
					},
				},
			},
			targetObj: &gwv1.GRPCRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-grpc-route",
					Namespace: "policy-namespace",
				},
			},
			expected:    "test-grpc-route-policy-namespace",
			expectError: false,
		},
		{
			name: "GRPCRoute targetRef with namespace uses specified namespace",
			alp: &anv1alpha1.AccessLogPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "policy-namespace",
				},
				Spec: anv1alpha1.AccessLogPolicySpec{
					TargetRef: &gwv1alpha2.NamespacedPolicyTargetReference{
						Kind:      "GRPCRoute",
						Name:      "test-grpc-route",
						Namespace: (*gwv1alpha2.Namespace)(&[]string{"route-namespace"}[0]),
					},
				},
			},
			targetObj: &gwv1.GRPCRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-grpc-route",
					Namespace: "route-namespace",
				},
			},
			expected:    "test-grpc-route-route-namespace",
			expectError: false,
		},
		{
			name: "GRPCRoute targetRef with valid service name override",
			alp: &anv1alpha1.AccessLogPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
				},
				Spec: anv1alpha1.AccessLogPolicySpec{
					TargetRef: &gwv1alpha2.NamespacedPolicyTargetReference{
						Kind: "GRPCRoute",
						Name: "test-grpc-route",
					},
				},
			},
			targetObj: &gwv1.GRPCRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-grpc-route",
					Namespace: "test-namespace",
					Annotations: map[string]string{
						k8s.ServiceNameOverrideAnnotation: "custom-grpc-service",
					},
				},
			},
			expected:    "custom-grpc-service",
			expectError: false,
		},
		{
			name: "unsupported targetRef kind returns error",
			alp: &anv1alpha1.AccessLogPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace",
				},
				Spec: anv1alpha1.AccessLogPolicySpec{
					TargetRef: &gwv1alpha2.NamespacedPolicyTargetReference{
						Kind: "Service",
						Name: "test-service",
					},
				},
			},
			targetObj:     nil,
			expected:      "",
			expectError:   true,
			errorContains: "unsupported targetRef kind",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := r.targetRefToResourceName(tt.alp, tt.targetObj)

			if tt.expectError {
				assert.Error(t, err, "Expected error for test case: %s", tt.name)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains, "Error message should contain expected text")
				}
				assert.Equal(t, tt.expected, result, "Result should match expected value even on error")
			} else {
				assert.NoError(t, err, "Expected no error for test case: %s", tt.name)
				assert.Equal(t, tt.expected, result, "Result should match expected value")
			}
		})
	}
}
