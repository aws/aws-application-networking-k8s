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

	core "github.com/aws/aws-application-networking-k8s/pkg/model/core"
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

func TestUpdateGWListenerStatus_ProgrammedCondition(t *testing.T) {
	scheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(scheme)
	gwv1.Install(scheme)
	gwv1alpha2.Install(scheme)
	addOptionalCRDs(scheme)

	tests := []struct {
		name             string
		listeners        []gwv1.Listener
		expectProgrammed bool
	}{
		{
			name: "valid listener has Programmed condition set to True",
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
			expectProgrammed: true,
		},
		{
			name: "invalid listener does not have Programmed condition",
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
			expectProgrammed: false,
		},
		{
			name: "multiple valid listeners each have Programmed condition",
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
			expectProgrammed: true,
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
				var foundProgrammed bool
				for _, cond := range ls.Conditions {
					if cond.Type == string(gwv1.ListenerConditionProgrammed) {
						foundProgrammed = true
						assert.Equal(t, metav1.ConditionTrue, cond.Status)
						assert.Equal(t, string(gwv1.ListenerReasonProgrammed), cond.Reason)
					}
				}
				assert.Equal(t, tt.expectProgrammed, foundProgrammed, "Programmed condition presence mismatch for listener %s", ls.Name)
			}
		})
	}
}

func TestListenerRouteGroupKindSupported_TLSRoute(t *testing.T) {
	tests := []struct {
		name        string
		listener    gwv1.Listener
		expectValid bool
		expectKinds []gwv1.RouteGroupKind
	}{
		{
			name: "TLSRoute valid on TLS listener",
			listener: gwv1.Listener{
				Protocol: gwv1.TLSProtocolType,
				AllowedRoutes: &gwv1.AllowedRoutes{
					Kinds: []gwv1.RouteGroupKind{{Kind: "TLSRoute"}},
				},
			},
			expectValid: true,
			expectKinds: []gwv1.RouteGroupKind{{Kind: "TLSRoute"}},
		},
		{
			name: "TLSRoute invalid on HTTP listener",
			listener: gwv1.Listener{
				Protocol: gwv1.HTTPProtocolType,
				AllowedRoutes: &gwv1.AllowedRoutes{
					Kinds: []gwv1.RouteGroupKind{{Kind: "TLSRoute"}},
				},
			},
			expectValid: false,
			expectKinds: []gwv1.RouteGroupKind{},
		},
		{
			name: "TLSRoute invalid on HTTPS listener",
			listener: gwv1.Listener{
				Protocol: gwv1.HTTPSProtocolType,
				AllowedRoutes: &gwv1.AllowedRoutes{
					Kinds: []gwv1.RouteGroupKind{{Kind: "TLSRoute"}},
				},
			},
			expectValid: false,
			expectKinds: []gwv1.RouteGroupKind{},
		},
		{
			name: "GRPCRoute valid on HTTPS listener",
			listener: gwv1.Listener{
				Protocol: gwv1.HTTPSProtocolType,
				AllowedRoutes: &gwv1.AllowedRoutes{
					Kinds: []gwv1.RouteGroupKind{{Kind: "GRPCRoute"}},
				},
			},
			expectValid: true,
			expectKinds: []gwv1.RouteGroupKind{{Kind: "GRPCRoute"}},
		},
		{
			name: "GRPCRoute invalid on HTTP listener",
			listener: gwv1.Listener{
				Protocol: gwv1.HTTPProtocolType,
				AllowedRoutes: &gwv1.AllowedRoutes{
					Kinds: []gwv1.RouteGroupKind{{Kind: "GRPCRoute"}},
				},
			},
			expectValid: false,
			expectKinds: []gwv1.RouteGroupKind{},
		},
		{
			name: "nil AllowedRoutes on HTTP listener defaults to HTTPRoute",
			listener: gwv1.Listener{
				Protocol: gwv1.HTTPProtocolType,
			},
			expectValid: true,
			expectKinds: []gwv1.RouteGroupKind{{Kind: "HTTPRoute"}},
		},
		{
			name: "nil AllowedRoutes on HTTPS listener defaults to HTTPRoute and GRPCRoute",
			listener: gwv1.Listener{
				Protocol: gwv1.HTTPSProtocolType,
			},
			expectValid: true,
			expectKinds: []gwv1.RouteGroupKind{{Kind: "HTTPRoute"}, {Kind: "GRPCRoute"}},
		},
		{
			name: "nil AllowedRoutes on TLS listener defaults to TLSRoute",
			listener: gwv1.Listener{
				Protocol: gwv1.TLSProtocolType,
			},
			expectValid: true,
			expectKinds: []gwv1.RouteGroupKind{{Kind: "TLSRoute"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, kinds := listenerRouteGroupKindSupported(tt.listener)
			assert.Equal(t, tt.expectValid, valid)
			assert.Equal(t, tt.expectKinds, kinds)
		})
	}
}

func TestUpdateGWListenerStatus_AttachedRoutes(t *testing.T) {
	scheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(scheme)
	gwv1.Install(scheme)
	gwv1alpha2.Install(scheme)
	addOptionalCRDs(scheme)

	sectionHTTP := gwv1.SectionName("http")
	gwName := gwv1.ObjectName("test-gateway")

	tests := []struct {
		name                   string
		listeners              []gwv1.Listener
		httpRoutes             []*gwv1.HTTPRoute
		tlsRoutes              []*gwv1.TLSRoute
		expectedAttachedRoutes map[gwv1.SectionName]int32
	}{
		{
			name: "nil sectionName attaches HTTPRoute to all compatible listeners",
			listeners: []gwv1.Listener{
				{
					Name: "http", Port: 80, Protocol: gwv1.HTTPProtocolType,
					AllowedRoutes: &gwv1.AllowedRoutes{Kinds: []gwv1.RouteGroupKind{{Kind: "HTTPRoute"}}},
				},
				{
					Name: "https", Port: 443, Protocol: gwv1.HTTPSProtocolType,
					AllowedRoutes: &gwv1.AllowedRoutes{Kinds: []gwv1.RouteGroupKind{{Kind: "HTTPRoute"}}},
				},
			},
			httpRoutes: []*gwv1.HTTPRoute{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "route1", Namespace: "default"},
					Spec: gwv1.HTTPRouteSpec{
						CommonRouteSpec: gwv1.CommonRouteSpec{
							ParentRefs: []gwv1.ParentReference{{Name: gwName}},
						},
					},
				},
			},
			expectedAttachedRoutes: map[gwv1.SectionName]int32{
				"http":  1,
				"https": 1,
			},
		},
		{
			name: "explicit sectionName attaches only to matching listener",
			listeners: []gwv1.Listener{
				{
					Name: "http", Port: 80, Protocol: gwv1.HTTPProtocolType,
					AllowedRoutes: &gwv1.AllowedRoutes{Kinds: []gwv1.RouteGroupKind{{Kind: "HTTPRoute"}}},
				},
				{
					Name: "https", Port: 443, Protocol: gwv1.HTTPSProtocolType,
					AllowedRoutes: &gwv1.AllowedRoutes{Kinds: []gwv1.RouteGroupKind{{Kind: "HTTPRoute"}}},
				},
			},
			httpRoutes: []*gwv1.HTTPRoute{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "route1", Namespace: "default"},
					Spec: gwv1.HTTPRouteSpec{
						CommonRouteSpec: gwv1.CommonRouteSpec{
							ParentRefs: []gwv1.ParentReference{{Name: gwName, SectionName: &sectionHTTP}},
						},
					},
				},
			},
			expectedAttachedRoutes: map[gwv1.SectionName]int32{
				"http":  1,
				"https": 0,
			},
		},
		{
			name: "HTTPRoute does not attach to TLS listener",
			listeners: []gwv1.Listener{
				{
					Name: "http", Port: 80, Protocol: gwv1.HTTPProtocolType,
					AllowedRoutes: &gwv1.AllowedRoutes{Kinds: []gwv1.RouteGroupKind{{Kind: "HTTPRoute"}}},
				},
				{
					Name: "tls", Port: 8443, Protocol: gwv1.TLSProtocolType,
					AllowedRoutes: &gwv1.AllowedRoutes{Kinds: []gwv1.RouteGroupKind{{Kind: "TLSRoute"}}},
				},
			},
			httpRoutes: []*gwv1.HTTPRoute{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "route1", Namespace: "default"},
					Spec: gwv1.HTTPRouteSpec{
						CommonRouteSpec: gwv1.CommonRouteSpec{
							ParentRefs: []gwv1.ParentReference{{Name: gwName}},
						},
					},
				},
			},
			expectedAttachedRoutes: map[gwv1.SectionName]int32{
				"http": 1,
				"tls":  0,
			},
		},
		{
			name: "TLSRoute attaches only to TLS listener with nil sectionName",
			listeners: []gwv1.Listener{
				{
					Name: "http", Port: 80, Protocol: gwv1.HTTPProtocolType,
					AllowedRoutes: &gwv1.AllowedRoutes{Kinds: []gwv1.RouteGroupKind{{Kind: "HTTPRoute"}}},
				},
				{
					Name: "tls", Port: 8443, Protocol: gwv1.TLSProtocolType,
					AllowedRoutes: &gwv1.AllowedRoutes{Kinds: []gwv1.RouteGroupKind{{Kind: "TLSRoute"}}},
				},
			},
			tlsRoutes: []*gwv1.TLSRoute{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "tls-route1", Namespace: "default"},
					Spec: gwv1.TLSRouteSpec{
						CommonRouteSpec: gwv1.CommonRouteSpec{
							ParentRefs: []gwv1.ParentReference{{Name: gwName}},
						},
					},
				},
			},
			expectedAttachedRoutes: map[gwv1.SectionName]int32{
				"http": 0,
				"tls":  1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			gw := &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "test-gateway", Namespace: "default"},
				Spec: gwv1.GatewaySpec{
					GatewayClassName: "amazon-vpc-lattice",
					Listeners:        tt.listeners,
				},
			}

			builder := testclient.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(gw).
				WithStatusSubresource(gw)

			for _, r := range tt.httpRoutes {
				builder = builder.WithObjects(r)
			}
			for _, r := range tt.tlsRoutes {
				builder = builder.WithObjects(r)
			}

			k8sClient := builder.Build()

			err := UpdateGWListenerStatus(ctx, k8sClient, gw)
			assert.NoError(t, err)

			updatedGw := &gwv1.Gateway{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: "test-gateway", Namespace: "default"}, updatedGw)
			assert.NoError(t, err)

			for _, ls := range updatedGw.Status.Listeners {
				expected, ok := tt.expectedAttachedRoutes[ls.Name]
				assert.True(t, ok, "unexpected listener %s", ls.Name)
				assert.Equal(t, expected, ls.AttachedRoutes, "attachedRoutes mismatch for listener %s", ls.Name)
			}
		})
	}
}

func TestUpdateGWListenerStatus_SupportedKinds(t *testing.T) {
	scheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(scheme)
	gwv1.Install(scheme)
	gwv1alpha2.Install(scheme)
	addOptionalCRDs(scheme)

	tests := []struct {
		name          string
		listener      gwv1.Listener
		expectedKinds []gwv1.RouteGroupKind
	}{
		{
			name: "HTTP listener supports HTTPRoute",
			listener: gwv1.Listener{
				Name: "http", Port: 80, Protocol: gwv1.HTTPProtocolType,
				AllowedRoutes: &gwv1.AllowedRoutes{Kinds: []gwv1.RouteGroupKind{{Kind: "HTTPRoute"}}},
			},
			expectedKinds: []gwv1.RouteGroupKind{{Kind: "HTTPRoute"}},
		},
		{
			name: "HTTPS listener supports HTTPRoute and GRPCRoute",
			listener: gwv1.Listener{
				Name: "https", Port: 443, Protocol: gwv1.HTTPSProtocolType,
				AllowedRoutes: &gwv1.AllowedRoutes{Kinds: []gwv1.RouteGroupKind{{Kind: "HTTPRoute"}}},
			},
			expectedKinds: []gwv1.RouteGroupKind{{Kind: "HTTPRoute"}, {Kind: "GRPCRoute"}},
		},
		{
			name: "TLS listener supports TLSRoute",
			listener: gwv1.Listener{
				Name: "tls", Port: 8443, Protocol: gwv1.TLSProtocolType,
				AllowedRoutes: &gwv1.AllowedRoutes{Kinds: []gwv1.RouteGroupKind{{Kind: "TLSRoute"}}},
			},
			expectedKinds: []gwv1.RouteGroupKind{{Kind: "TLSRoute"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			gw := &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "test-gateway", Namespace: "default"},
				Spec: gwv1.GatewaySpec{
					GatewayClassName: "amazon-vpc-lattice",
					Listeners:        []gwv1.Listener{tt.listener},
				},
			}

			k8sClient := testclient.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(gw).
				WithStatusSubresource(gw).
				Build()

			err := UpdateGWListenerStatus(ctx, k8sClient, gw)
			assert.NoError(t, err)

			updatedGw := &gwv1.Gateway{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: "test-gateway", Namespace: "default"}, updatedGw)
			assert.NoError(t, err)

			assert.Equal(t, 1, len(updatedGw.Status.Listeners))
			assert.Equal(t, tt.expectedKinds, updatedGw.Status.Listeners[0].SupportedKinds)
		})
	}
}

func TestUpdateGWListenerStatus_OverrideRoute(t *testing.T) {
	scheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(scheme)
	gwv1.Install(scheme)
	gwv1alpha2.Install(scheme)
	addOptionalCRDs(scheme)

	gwName := gwv1.ObjectName("test-gateway")

	tests := []struct {
		name                string
		cachedAnnotations   map[string]string
		overrideAnnotations map[string]string
		useOverride         bool
		expectAddress       bool
		expectedDomain      string
	}{
		{
			name:              "without override, stale cache has no address",
			cachedAnnotations: map[string]string{},
			useOverride:       false,
			expectAddress:     false,
		},
		{
			name:                "override replaces stale cached route",
			cachedAnnotations:   map[string]string{},
			overrideAnnotations: map[string]string{LatticeAssignedDomainName: "my-service.lattice.aws"},
			useOverride:         true,
			expectAddress:       true,
			expectedDomain:      "my-service.lattice.aws",
		},
		{
			name:              "without override, cached route with annotation works",
			cachedAnnotations: map[string]string{LatticeAssignedDomainName: "cached.lattice.aws"},
			useOverride:       false,
			expectAddress:     true,
			expectedDomain:    "cached.lattice.aws",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			gw := &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "test-gateway", Namespace: "default"},
				Spec: gwv1.GatewaySpec{
					GatewayClassName: "amazon-vpc-lattice",
					Listeners: []gwv1.Listener{
						{Name: "http", Port: 80, Protocol: gwv1.HTTPProtocolType,
							AllowedRoutes: &gwv1.AllowedRoutes{Kinds: []gwv1.RouteGroupKind{{Kind: "HTTPRoute"}}}},
					},
				},
			}

			cachedRoute := &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "route1",
					Namespace:   "default",
					Annotations: tt.cachedAnnotations,
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{{Name: gwName}},
					},
				},
			}

			k8sClient := testclient.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(gw, cachedRoute).
				WithStatusSubresource(gw).
				Build()

			var overrides []core.Route
			if tt.useOverride {
				overrideHTTPRoute := &gwv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "route1",
						Namespace:   "default",
						Annotations: tt.overrideAnnotations,
					},
					Spec: gwv1.HTTPRouteSpec{
						CommonRouteSpec: gwv1.CommonRouteSpec{
							ParentRefs: []gwv1.ParentReference{{Name: gwName}},
						},
					},
				}
				overrides = append(overrides, core.NewHTTPRoute(*overrideHTTPRoute))
			}

			err := UpdateGWListenerStatus(ctx, k8sClient, gw, overrides...)
			assert.NoError(t, err)

			updatedGw := &gwv1.Gateway{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: "test-gateway", Namespace: "default"}, updatedGw)
			assert.NoError(t, err)

			if tt.expectAddress {
				if assert.Equal(t, 1, len(updatedGw.Status.Addresses)) {
					assert.Equal(t, tt.expectedDomain, updatedGw.Status.Addresses[0].Value)
				}
			} else {
				assert.Equal(t, 0, len(updatedGw.Status.Addresses))
			}
		})
	}
}
