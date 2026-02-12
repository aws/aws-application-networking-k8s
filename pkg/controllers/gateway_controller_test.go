package controllers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func TestUpdateGWListenerStatus_RemovesStaleListeners(t *testing.T) {
	scheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(scheme)
	gwv1.Install(scheme)
	gwv1alpha2.Install(scheme)
	addOptionalCRDs(scheme)

	tests := []struct {
		name                  string
		gatewaySpec           gwv1.GatewaySpec
		initialStatus         gwv1.GatewayStatus
		expectedListenerCount int
		expectedListenerNames []gwv1.SectionName
	}{
		{
			name: "removes deleted listener from status",
			gatewaySpec: gwv1.GatewaySpec{
				GatewayClassName: "amazon-vpc-lattice",
				Listeners: []gwv1.Listener{
					{
						Name:     "http",
						Port:     80,
						Protocol: gwv1.HTTPProtocolType,
						AllowedRoutes: &gwv1.AllowedRoutes{
							Kinds: []gwv1.RouteGroupKind{{Kind: "HTTPRoute"}},
						},
					},
				},
			},
			initialStatus: gwv1.GatewayStatus{
				Listeners: []gwv1.ListenerStatus{
					{Name: "http", AttachedRoutes: 0},
					{Name: "https", AttachedRoutes: 0},
				},
			},
			expectedListenerCount: 1,
			expectedListenerNames: []gwv1.SectionName{"http"},
		},
		{
			name: "removes multiple deleted listeners",
			gatewaySpec: gwv1.GatewaySpec{
				GatewayClassName: "amazon-vpc-lattice",
				Listeners: []gwv1.Listener{
					{
						Name:     "http",
						Port:     80,
						Protocol: gwv1.HTTPProtocolType,
						AllowedRoutes: &gwv1.AllowedRoutes{
							Kinds: []gwv1.RouteGroupKind{{Kind: "HTTPRoute"}},
						},
					},
				},
			},
			initialStatus: gwv1.GatewayStatus{
				Listeners: []gwv1.ListenerStatus{
					{Name: "http", AttachedRoutes: 0},
					{Name: "https", AttachedRoutes: 0},
					{Name: "grpc", AttachedRoutes: 0},
				},
			},
			expectedListenerCount: 1,
			expectedListenerNames: []gwv1.SectionName{"http"},
		},
		{
			name: "preserves all listeners when none deleted",
			gatewaySpec: gwv1.GatewaySpec{
				GatewayClassName: "amazon-vpc-lattice",
				Listeners: []gwv1.Listener{
					{
						Name:     "http",
						Port:     80,
						Protocol: gwv1.HTTPProtocolType,
						AllowedRoutes: &gwv1.AllowedRoutes{
							Kinds: []gwv1.RouteGroupKind{{Kind: "HTTPRoute"}},
						},
					},
					{
						Name:     "https",
						Port:     443,
						Protocol: gwv1.HTTPSProtocolType,
						AllowedRoutes: &gwv1.AllowedRoutes{
							Kinds: []gwv1.RouteGroupKind{{Kind: "HTTPRoute"}},
						},
					},
				},
			},
			initialStatus: gwv1.GatewayStatus{
				Listeners: []gwv1.ListenerStatus{
					{Name: "http", AttachedRoutes: 2},
					{Name: "https", AttachedRoutes: 1},
				},
			},
			expectedListenerCount: 2,
			expectedListenerNames: []gwv1.SectionName{"http", "https"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			gw := &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway",
					Namespace: "default",
				},
				Spec:   tt.gatewaySpec,
				Status: tt.initialStatus,
			}

			k8sClient := testclient.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(gw).
				WithStatusSubresource(gw).
				Build()

			err := UpdateGWListenerStatus(ctx, k8sClient, gw)
			assert.NoError(t, err)

			updatedGw := &gwv1.Gateway{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "test-gateway",
				Namespace: "default",
			}, updatedGw)
			assert.NoError(t, err)

			assert.Equal(t, tt.expectedListenerCount, len(updatedGw.Status.Listeners))

			actualNames := make([]gwv1.SectionName, len(updatedGw.Status.Listeners))
			for i, listener := range updatedGw.Status.Listeners {
				actualNames[i] = listener.Name
			}

			for _, expectedName := range tt.expectedListenerNames {
				assert.Contains(t, actualNames, expectedName)
			}

			for _, actualName := range actualNames {
				assert.Contains(t, tt.expectedListenerNames, actualName)
			}
		})
	}
}

func TestUpdateGWListenerStatus_ResolvedRefsCondition(t *testing.T) {
	scheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(scheme)
	gwv1.Install(scheme)
	gwv1alpha2.Install(scheme)
	addOptionalCRDs(scheme)

	tests := []struct {
		name              string
		listeners         []gwv1.Listener
		expectAccepted    bool
		expectResolvedRef bool
	}{
		{
			name: "valid listener has both Accepted and ResolvedRefs conditions set to True",
			listeners: []gwv1.Listener{
				{
					Name:     "http",
					Port:     80,
					Protocol: gwv1.HTTPProtocolType,
					AllowedRoutes: &gwv1.AllowedRoutes{
						Kinds: []gwv1.RouteGroupKind{{Kind: "HTTPRoute"}},
					},
				},
			},
			expectAccepted:    true,
			expectResolvedRef: true,
		},
		{
			name: "invalid listener has ResolvedRefs False and no Accepted condition",
			listeners: []gwv1.Listener{
				{
					Name:     "bad",
					Port:     80,
					Protocol: gwv1.HTTPProtocolType,
					AllowedRoutes: &gwv1.AllowedRoutes{
						Kinds: []gwv1.RouteGroupKind{{Kind: "UnsupportedRoute"}},
					},
				},
			},
			expectAccepted:    false,
			expectResolvedRef: false,
		},
		{
			name: "multiple valid listeners each have both conditions",
			listeners: []gwv1.Listener{
				{
					Name:     "http",
					Port:     80,
					Protocol: gwv1.HTTPProtocolType,
					AllowedRoutes: &gwv1.AllowedRoutes{
						Kinds: []gwv1.RouteGroupKind{{Kind: "HTTPRoute"}},
					},
				},
				{
					Name:     "https",
					Port:     443,
					Protocol: gwv1.HTTPSProtocolType,
					AllowedRoutes: &gwv1.AllowedRoutes{
						Kinds: []gwv1.RouteGroupKind{{Kind: "HTTPRoute"}},
					},
				},
			},
			expectAccepted:    true,
			expectResolvedRef: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			gw := &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway",
					Namespace: "default",
				},
				Spec: gwv1.GatewaySpec{
					GatewayClassName: "amazon-vpc-lattice",
					Listeners:        tt.listeners,
				},
			}

			k8sClient := testclient.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(gw).
				WithStatusSubresource(gw).
				Build()

			_ = UpdateGWListenerStatus(ctx, k8sClient, gw)

			updatedGw := &gwv1.Gateway{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "test-gateway",
				Namespace: "default",
			}, updatedGw)
			assert.NoError(t, err)

			for _, ls := range updatedGw.Status.Listeners {
				var foundAccepted, foundResolvedRefs bool
				for _, cond := range ls.Conditions {
					if cond.Type == string(gwv1.ListenerConditionAccepted) {
						foundAccepted = true
						assert.Equal(t, metav1.ConditionTrue, cond.Status)
						assert.Equal(t, string(gwv1.ListenerReasonAccepted), cond.Reason)
					}
					if cond.Type == string(gwv1.ListenerConditionResolvedRefs) {
						foundResolvedRefs = true
						if tt.expectResolvedRef {
							assert.Equal(t, metav1.ConditionTrue, cond.Status)
							assert.Equal(t, string(gwv1.ListenerReasonResolvedRefs), cond.Reason)
						} else {
							assert.Equal(t, metav1.ConditionFalse, cond.Status)
							assert.Equal(t, string(gwv1.ListenerReasonInvalidRouteKinds), cond.Reason)
						}
					}
				}
				assert.Equal(t, tt.expectAccepted, foundAccepted, "Accepted condition presence mismatch for listener %s", ls.Name)
				assert.True(t, foundResolvedRefs, "ResolvedRefs condition must be present for listener %s", ls.Name)
			}
		})
	}
}
