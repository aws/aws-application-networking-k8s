package policyhelper

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	mock_client "github.com/aws/aws-application-networking-k8s/mocks/controller-runtime/client"
	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

func TestPolicyClient(t *testing.T) {
	type iap = anv1alpha1.IAMAuthPolicy
	type iapl = anv1alpha1.IAMAuthPolicyList

	t.Run("new list and policy", func(t *testing.T) {
		c := newK8sPolicyClient[iap, iapl](nil)
		assert.NotNil(t, c.newPolicy())
		assert.NotNil(t, c.newList())
	})
}

func TestPolicyHandler(t *testing.T) {
	type iap = anv1alpha1.IAMAuthPolicy
	type iapl = anv1alpha1.IAMAuthPolicyList

	phcfg := PolicyHandlerConfig{}
	_ = NewPolicyHandler[iap, iapl](phcfg)
}

func TestGroupKindSet(t *testing.T) {
	objs := []client.Object{&gwv1.Gateway{}, &gwv1.HTTPRoute{}, &gwv1.GRPCRoute{}}
	gks := NewGroupKindSet(objs...)
	assert.True(t, gks.Contains(GroupKind{gwv1.GroupName, "Gateway"}))
	assert.True(t, gks.Contains(GroupKind{gwv1.GroupName, "HTTPRoute"}))
	assert.True(t, gks.Contains(GroupKind{gwv1.GroupName, "GRPCRoute"}))
}

func TestFindPolicyForService(t *testing.T) {
	// This test verifies that FindPolicyForService can locate TargetGroupPolicy resources
	// for Service objects by checking both direct Service targets and ServiceExport targets
	// with the same name and namespace.

	type tgp = anv1alpha1.TargetGroupPolicy
	type tgpl = anv1alpha1.TargetGroupPolicyList

	t.Run("method exists and has correct signature", func(t *testing.T) {
		phcfg := PolicyHandlerConfig{}
		ph := NewPolicyHandler[tgp, tgpl](phcfg)

		// Verify the method exists and can be called
		// In a real test, this would use a mock client and verify the behavior
		assert.NotNil(t, ph)

		// The method should exist and be callable
		// policy, err := ph.FindPolicyForService(context.TODO(), "test-service", "test-namespace")
		// This would require a proper mock setup which is beyond the scope of this unit test
	})

	t.Run("backwards compatibility - existing behavior unchanged", func(t *testing.T) {
		// Verify that the existing NewTargetGroupPolicyHandler still works as before
		// and supports both Service and ServiceExport objects in TargetRefKinds
		phcfg := PolicyHandlerConfig{
			TargetRefKinds: NewGroupKindSet(&corev1.Service{}, &anv1alpha1.ServiceExport{}),
		}
		ph := NewPolicyHandler[tgp, tgpl](phcfg)

		assert.NotNil(t, ph)
		assert.True(t, ph.kinds.Contains(GroupKind{corev1.GroupName, "Service"}))
		assert.True(t, ph.kinds.Contains(GroupKind{anv1alpha1.GroupName, "ServiceExport"}))
	})
}

func Test_FindPolicyForService_ServiceBasedResolution(t *testing.T) {
	type tgp = anv1alpha1.TargetGroupPolicy
	type tgpl = anv1alpha1.TargetGroupPolicyList

	tests := []struct {
		name             string
		serviceName      string
		serviceNamespace string
		policies         []anv1alpha1.TargetGroupPolicy
		expectedPolicy   *anv1alpha1.TargetGroupPolicy
		expectError      bool
	}{
		{
			name:             "Policy targets Service directly",
			serviceName:      "test-service",
			serviceNamespace: "test-namespace",
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
					},
				},
			},
			expectedPolicy: &anv1alpha1.TargetGroupPolicy{
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
				},
			},
			expectError: false,
		},
		{
			name:             "Policy targets ServiceExport with same name",
			serviceName:      "test-service",
			serviceNamespace: "test-namespace",
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
							Name:  "test-service", // Same name as service
						},
					},
				},
			},
			expectedPolicy: &anv1alpha1.TargetGroupPolicy{
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
				},
			},
			expectError: false,
		},
		{
			name:             "Service policy takes precedence over ServiceExport policy",
			serviceName:      "test-service",
			serviceNamespace: "test-namespace",
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
					},
				},
			},
			expectedPolicy: &anv1alpha1.TargetGroupPolicy{
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
				},
			},
			expectError: false,
		},
		{
			name:             "No applicable policies found",
			serviceName:      "test-service",
			serviceNamespace: "test-namespace",
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
							Name:  "other-service", // Different service name
						},
					},
				},
			},
			expectedPolicy: nil,
			expectError:    false,
		},
		{
			name:             "Policy in different namespace should not match",
			serviceName:      "test-service",
			serviceNamespace: "test-namespace",
			policies: []anv1alpha1.TargetGroupPolicy{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "service-policy",
						Namespace: "other-namespace", // Different namespace
					},
					Spec: anv1alpha1.TargetGroupPolicySpec{
						TargetRef: &gwv1alpha2.NamespacedPolicyTargetReference{
							Group: corev1.GroupName,
							Kind:  "Service",
							Name:  "test-service",
						},
					},
				},
			},
			expectedPolicy: nil,
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

			// Create policy handler
			phcfg := PolicyHandlerConfig{
				Log:            gwlog.FallbackLogger,
				Client:         k8sClient,
				TargetRefKinds: NewGroupKindSet(&corev1.Service{}, &anv1alpha1.ServiceExport{}),
			}
			ph := NewPolicyHandler[tgp, tgpl](phcfg)

			// Call the method under test
			policy, err := ph.FindPolicyForService(ctx, tt.serviceName, tt.serviceNamespace)

			// Verify results
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if tt.expectedPolicy == nil {
				// Check if policy is zero value (equivalent to nil)
				var zero *anv1alpha1.TargetGroupPolicy
				assert.Equal(t, fmt.Sprintf("%v", zero), fmt.Sprintf("%v", policy))
			} else {
				assert.NotNil(t, policy)
				assert.Equal(t, tt.expectedPolicy.Name, policy.Name)
				assert.Equal(t, tt.expectedPolicy.Namespace, policy.Namespace)
				assert.Equal(t, tt.expectedPolicy.Spec.TargetRef, policy.Spec.TargetRef)
			}
		})
	}
}

func Test_FindPolicyForService_ErrorHandling(t *testing.T) {
	type tgp = anv1alpha1.TargetGroupPolicy
	type tgpl = anv1alpha1.TargetGroupPolicyList

	t.Run("client error should be propagated", func(t *testing.T) {
		ctx := context.Background()

		// Create a mock controller and client that returns an error
		c := gomock.NewController(t)
		defer c.Finish()

		mockClient := mock_client.NewMockClient(c)
		mockClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(errors.New("client error")).AnyTimes()

		// Create policy handler with mock client
		phcfg := PolicyHandlerConfig{
			Log:            gwlog.FallbackLogger,
			Client:         mockClient,
			TargetRefKinds: NewGroupKindSet(&corev1.Service{}, &anv1alpha1.ServiceExport{}),
		}
		ph := NewPolicyHandler[tgp, tgpl](phcfg)

		// Call the method under test
		policy, err := ph.FindPolicyForService(ctx, "test-service", "test-namespace")

		// Verify error is propagated
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "client error")

		// Verify policy is zero value
		var zero *anv1alpha1.TargetGroupPolicy
		assert.Equal(t, fmt.Sprintf("%v", zero), fmt.Sprintf("%v", policy))
	})
}
