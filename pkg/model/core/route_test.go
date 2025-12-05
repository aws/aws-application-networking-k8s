package core

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func TestHasAllParentRefsRejected(t *testing.T) {
	tests := []struct {
		name               string
		routeStatusParents []gwv1.RouteParentStatus
		expected           bool
		description        string
	}{
		{
			name:               "empty_parents",
			routeStatusParents: []gwv1.RouteParentStatus{},
			expected:           true,
			description:        "No parents should be considered fully rejected",
		},
		{
			name: "all_rejected",
			routeStatusParents: []gwv1.RouteParentStatus{
				{
					Conditions: []metav1.Condition{
						{
							Type:   string(gwv1.RouteConditionAccepted),
							Status: metav1.ConditionFalse,
						},
					},
				},
				{
					Conditions: []metav1.Condition{
						{
							Type:   string(gwv1.RouteConditionAccepted),
							Status: metav1.ConditionFalse,
						},
					},
				},
			},
			expected:    true,
			description: "All rejected parentRefs should return true",
		},
		{
			name: "some_accepted",
			routeStatusParents: []gwv1.RouteParentStatus{
				{
					Conditions: []metav1.Condition{
						{
							Type:   string(gwv1.RouteConditionAccepted),
							Status: metav1.ConditionTrue,
						},
					},
				},
				{
					Conditions: []metav1.Condition{
						{
							Type:   string(gwv1.RouteConditionAccepted),
							Status: metav1.ConditionFalse,
						},
					},
				},
			},
			expected:    false,
			description: "Some accepted parentRefs should return false",
		},
		{
			name: "all_accepted",
			routeStatusParents: []gwv1.RouteParentStatus{
				{
					Conditions: []metav1.Condition{
						{
							Type:   string(gwv1.RouteConditionAccepted),
							Status: metav1.ConditionTrue,
						},
					},
				},
			},
			expected:    false,
			description: "All accepted parentRefs should return false",
		},
		{
			name: "no_accepted_condition",
			routeStatusParents: []gwv1.RouteParentStatus{
				{
					Conditions: []metav1.Condition{
						{
							Type:   "SomeOtherCondition",
							Status: metav1.ConditionTrue,
						},
					},
				},
			},
			expected:    true,
			description: "ParentRefs without Accepted condition should be considered rejected",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			route := &HTTPRoute{
				r: gwv1.HTTPRoute{
					Status: gwv1.HTTPRouteStatus{
						RouteStatus: gwv1.RouteStatus{
							Parents: test.routeStatusParents,
						},
					},
				},
			}

			result := HasAllParentRefsRejected(route)
			assert.Equal(t, test.expected, result, test.description)
		})
	}
}

func TestIsRouteKindAllowedByListener(t *testing.T) {
	tests := []struct {
		name        string
		routeKind   string
		protocol    gwv1.ProtocolType
		kinds       []gwv1.RouteGroupKind
		expected    bool
		description string
	}{
		{
			name:        "http_route_http_listener_default",
			routeKind:   "HTTPRoute",
			protocol:    gwv1.HTTPProtocolType,
			kinds:       nil,
			expected:    true,
			description: "HTTPRoute should be allowed by HTTP listener with default kinds",
		},
		{
			name:        "http_route_https_listener_default",
			routeKind:   "HTTPRoute",
			protocol:    gwv1.HTTPSProtocolType,
			kinds:       nil,
			expected:    true,
			description: "HTTPRoute should be allowed by HTTPS listener with default kinds",
		},
		{
			name:        "http_route_tls_listener_default",
			routeKind:   "HTTPRoute",
			protocol:    gwv1.TLSProtocolType,
			kinds:       nil,
			expected:    false,
			description: "HTTPRoute should not be allowed by TLS listener with default kinds",
		},
		{
			name:        "grpc_route_http_listener_default",
			routeKind:   "GRPCRoute",
			protocol:    gwv1.HTTPProtocolType,
			kinds:       nil,
			expected:    false,
			description: "GRPCRoute should not be allowed by HTTP listener with default kinds",
		},
		{
			name:        "grpc_route_https_listener_default",
			routeKind:   "GRPCRoute",
			protocol:    gwv1.HTTPSProtocolType,
			kinds:       nil,
			expected:    true,
			description: "GRPCRoute should be allowed by HTTPS listener with default kinds",
		},
		{
			name:        "tls_route_tls_listener_default",
			routeKind:   "TLSRoute",
			protocol:    gwv1.TLSProtocolType,
			kinds:       nil,
			expected:    true,
			description: "TLSRoute should be allowed by TLS listener with default kinds",
		},
		{
			name:        "tls_route_http_listener_default",
			routeKind:   "TLSRoute",
			protocol:    gwv1.HTTPProtocolType,
			kinds:       nil,
			expected:    false,
			description: "TLSRoute should not be allowed by HTTP listener with default kinds",
		},
		{
			name:      "http_route_https_listener_explicit_grpc_only",
			routeKind: "HTTPRoute",
			protocol:  gwv1.HTTPSProtocolType,
			kinds: []gwv1.RouteGroupKind{
				{Kind: "GRPCRoute"},
			},
			expected:    false,
			description: "HTTPRoute should not be allowed by HTTPS listener configured for GRPCRoute only",
		},
		{
			name:      "grpc_route_https_listener_explicit_grpc_only",
			routeKind: "GRPCRoute",
			protocol:  gwv1.HTTPSProtocolType,
			kinds: []gwv1.RouteGroupKind{
				{Kind: "GRPCRoute"},
			},
			expected:    true,
			description: "GRPCRoute should be allowed by HTTPS listener configured for GRPCRoute only",
		},
		{
			name:      "http_route_tls_listener_explicit_http_allowed",
			routeKind: "HTTPRoute",
			protocol:  gwv1.TLSProtocolType,
			kinds: []gwv1.RouteGroupKind{
				{Kind: "HTTPRoute"},
			},
			expected:    true,
			description: "HTTPRoute should be allowed by TLS listener with explicit HTTPRoute kinds",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var route Route
			switch test.routeKind {
			case "HTTPRoute":
				route = NewHTTPRoute(gwv1.HTTPRoute{})
			case "GRPCRoute":
				route = NewGRPCRoute(gwv1.GRPCRoute{})
			case "TLSRoute":
				route = NewTLSRoute(gwv1alpha2.TLSRoute{})
			}

			listener := gwv1.Listener{
				Protocol: test.protocol,
			}
			if test.kinds != nil {
				listener.AllowedRoutes = &gwv1.AllowedRoutes{
					Kinds: test.kinds,
				}
			}

			result := isRouteKindAllowedByListener(route, listener)
			assert.Equal(t, test.expected, result, test.description)
		})
	}
}

func TestIsRouteAllowedByListener(t *testing.T) {
	tests := []struct {
		name           string
		routeNamespace string
		gwNamespace    string
		fromPolicy     *gwv1.FromNamespaces
		selector       *metav1.LabelSelector
		nsLabels       map[string]string
		routeKind      string
		protocol       gwv1.ProtocolType
		expected       bool
		expectError    bool
		description    string
	}{
		{
			name:           "same_namespace_policy_same_ns",
			routeNamespace: "test-ns",
			gwNamespace:    "test-ns",
			fromPolicy:     &[]gwv1.FromNamespaces{gwv1.NamespacesFromSame}[0],
			routeKind:      "HTTPRoute",
			protocol:       gwv1.HTTPProtocolType,
			expected:       true,
			expectError:    false,
			description:    "Route from same namespace should be allowed with Same policy",
		},
		{
			name:           "same_namespace_policy_diff_ns",
			routeNamespace: "route-ns",
			gwNamespace:    "gw-ns",
			fromPolicy:     &[]gwv1.FromNamespaces{gwv1.NamespacesFromSame}[0],
			routeKind:      "HTTPRoute",
			protocol:       gwv1.HTTPProtocolType,
			expected:       false,
			expectError:    false,
			description:    "Route from different namespace should not be allowed with Same policy",
		},
		{
			name:           "all_namespace_policy_same_ns",
			routeNamespace: "test-ns",
			gwNamespace:    "test-ns",
			fromPolicy:     &[]gwv1.FromNamespaces{gwv1.NamespacesFromAll}[0],
			routeKind:      "HTTPRoute",
			protocol:       gwv1.HTTPProtocolType,
			expected:       true,
			expectError:    false,
			description:    "Route from same namespace should be allowed with All policy",
		},
		{
			name:           "all_namespace_policy_diff_ns",
			routeNamespace: "route-ns",
			gwNamespace:    "gw-ns",
			fromPolicy:     &[]gwv1.FromNamespaces{gwv1.NamespacesFromAll}[0],
			routeKind:      "HTTPRoute",
			protocol:       gwv1.HTTPProtocolType,
			expected:       true,
			expectError:    false,
			description:    "Route from different namespace should be allowed with All policy",
		},
		{
			name:           "selector_policy_matching_label",
			routeNamespace: "route-ns",
			gwNamespace:    "gw-ns",
			fromPolicy:     &[]gwv1.FromNamespaces{gwv1.NamespacesFromSelector}[0],
			selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"env": "prod"},
			},
			nsLabels:    map[string]string{"env": "prod"},
			routeKind:   "HTTPRoute",
			protocol:    gwv1.HTTPProtocolType,
			expected:    true,
			expectError: false,
			description: "Route from namespace with matching label should be allowed",
		},
		{
			name:           "selector_policy_non_matching_label",
			routeNamespace: "route-ns",
			gwNamespace:    "gw-ns",
			fromPolicy:     &[]gwv1.FromNamespaces{gwv1.NamespacesFromSelector}[0],
			selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"env": "prod"},
			},
			nsLabels:    map[string]string{"env": "dev"},
			routeKind:   "HTTPRoute",
			protocol:    gwv1.HTTPProtocolType,
			expected:    false,
			expectError: false,
			description: "Route from namespace with non-matching label should not be allowed",
		},
		{
			name:           "no_allowed_routes_defaults_to_same",
			routeNamespace: "route-ns",
			gwNamespace:    "gw-ns",
			fromPolicy:     nil,
			routeKind:      "HTTPRoute",
			protocol:       gwv1.HTTPProtocolType,
			expected:       false,
			expectError:    false,
			description:    "No allowedRoutes should default to Same namespace behavior",
		},
		{
			name:           "incompatible_route_kind",
			routeNamespace: "test-ns",
			gwNamespace:    "test-ns",
			fromPolicy:     &[]gwv1.FromNamespaces{gwv1.NamespacesFromAll}[0],
			routeKind:      "TLSRoute",
			protocol:       gwv1.HTTPProtocolType,
			expected:       false,
			expectError:    false,
			description:    "TLSRoute should not be allowed by HTTP listener even with All namespace policy",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			_ = corev1.AddToScheme(scheme)

			objs := []client.Object{}
			if test.nsLabels != nil {
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name:   test.routeNamespace,
						Labels: test.nsLabels,
					},
				}
				objs = append(objs, ns)
			}

			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()

			var route Route
			switch test.routeKind {
			case "HTTPRoute":
				route = NewHTTPRoute(gwv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-route",
						Namespace: test.routeNamespace,
					},
				})
			case "GRPCRoute":
				route = NewGRPCRoute(gwv1.GRPCRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-route",
						Namespace: test.routeNamespace,
					},
				})
			case "TLSRoute":
				route = NewTLSRoute(gwv1alpha2.TLSRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-route",
						Namespace: test.routeNamespace,
					},
				})
			}

			gw := &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway",
					Namespace: test.gwNamespace,
				},
			}

			listener := gwv1.Listener{
				Name:     "test-listener",
				Protocol: test.protocol,
			}

			if test.fromPolicy != nil || test.selector != nil {
				listener.AllowedRoutes = &gwv1.AllowedRoutes{}
				if test.fromPolicy != nil || test.selector != nil {
					listener.AllowedRoutes.Namespaces = &gwv1.RouteNamespaces{}
					if test.fromPolicy != nil {
						listener.AllowedRoutes.Namespaces.From = test.fromPolicy
					}
					if test.selector != nil {
						listener.AllowedRoutes.Namespaces.Selector = test.selector
					}
				}
			}
			result, err := IsRouteAllowedByListener(context.Background(), k8sClient, route, gw, listener)

			if test.expectError {
				assert.Error(t, err, test.description)
			} else {
				assert.NoError(t, err, test.description)
				assert.Equal(t, test.expected, result, test.description)
			}
		})
	}
}

func TestIsParentRefAccepted(t *testing.T) {
	parentRef1 := gwv1.ParentReference{
		Name:      "gateway1",
		Namespace: &[]gwv1.Namespace{"default"}[0],
	}
	parentRef2 := gwv1.ParentReference{
		Name:      "gateway2",
		Namespace: &[]gwv1.Namespace{"default"}[0],
	}
	sectionName := gwv1.SectionName("http")
	parentRefWithSection := gwv1.ParentReference{
		Name:        "gateway1",
		Namespace:   &[]gwv1.Namespace{"default"}[0],
		SectionName: &sectionName,
	}

	tests := []struct {
		name               string
		routeStatusParents []gwv1.RouteParentStatus
		checkParentRef     gwv1.ParentReference
		expected           bool
		description        string
	}{
		{
			name:               "no_parents_in_status",
			routeStatusParents: []gwv1.RouteParentStatus{},
			checkParentRef:     parentRef1,
			expected:           false,
			description:        "No parents in status should return false",
		},
		{
			name: "matching_parentref_accepted",
			routeStatusParents: []gwv1.RouteParentStatus{
				{
					ParentRef: parentRef1,
					Conditions: []metav1.Condition{
						{
							Type:   string(gwv1.RouteConditionAccepted),
							Status: metav1.ConditionTrue,
						},
					},
				},
			},
			checkParentRef: parentRef1,
			expected:       true,
			description:    "Matching parentRef with accepted condition should return true",
		},
		{
			name: "matching_parentref_rejected",
			routeStatusParents: []gwv1.RouteParentStatus{
				{
					ParentRef: parentRef1,
					Conditions: []metav1.Condition{
						{
							Type:   string(gwv1.RouteConditionAccepted),
							Status: metav1.ConditionFalse,
						},
					},
				},
			},
			checkParentRef: parentRef1,
			expected:       false,
			description:    "Matching parentRef with rejected condition should return false",
		},
		{
			name: "non_matching_parentref",
			routeStatusParents: []gwv1.RouteParentStatus{
				{
					ParentRef: parentRef1,
					Conditions: []metav1.Condition{
						{
							Type:   string(gwv1.RouteConditionAccepted),
							Status: metav1.ConditionTrue,
						},
					},
				},
			},
			checkParentRef: parentRef2,
			expected:       false,
			description:    "Non-matching parentRef should return false",
		},
		{
			name: "no_accepted_condition",
			routeStatusParents: []gwv1.RouteParentStatus{
				{
					ParentRef: parentRef1,
					Conditions: []metav1.Condition{
						{
							Type:   "SomeOtherCondition",
							Status: metav1.ConditionTrue,
						},
					},
				},
			},
			checkParentRef: parentRef1,
			expected:       false,
			description:    "Matching parentRef without accepted condition should return false",
		},
		{
			name: "parentref_with_section_name",
			routeStatusParents: []gwv1.RouteParentStatus{
				{
					ParentRef: parentRefWithSection,
					Conditions: []metav1.Condition{
						{
							Type:   string(gwv1.RouteConditionAccepted),
							Status: metav1.ConditionTrue,
						},
					},
				},
			},
			checkParentRef: parentRefWithSection,
			expected:       true,
			description:    "Matching parentRef with sectionName should work correctly",
		},
		{
			name: "multiple_parents_check_specific",
			routeStatusParents: []gwv1.RouteParentStatus{
				{
					ParentRef: parentRef1,
					Conditions: []metav1.Condition{
						{
							Type:   string(gwv1.RouteConditionAccepted),
							Status: metav1.ConditionFalse,
						},
					},
				},
				{
					ParentRef: parentRef2,
					Conditions: []metav1.Condition{
						{
							Type:   string(gwv1.RouteConditionAccepted),
							Status: metav1.ConditionTrue,
						},
					},
				},
			},
			checkParentRef: parentRef2,
			expected:       true,
			description:    "Should find correct parentRef among multiple parents",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			route := &HTTPRoute{
				r: gwv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-route",
						Namespace: "test-namespace",
					},
					Status: gwv1.HTTPRouteStatus{
						RouteStatus: gwv1.RouteStatus{
							Parents: test.routeStatusParents,
						},
					},
				},
			}

			result := IsParentRefAccepted(route, test.checkParentRef)
			assert.Equal(t, test.expected, result, test.description)
		})
	}
}

func TestIsRouteAllowedByListener_ErrorCases(t *testing.T) {
	tests := []struct {
		name        string
		setupClient func() client.Client
		description string
	}{
		{
			name: "invalid_label_selector",
			setupClient: func() client.Client {
				scheme := runtime.NewScheme()
				_ = corev1.AddToScheme(scheme)
				return fake.NewClientBuilder().WithScheme(scheme).Build()
			},
			description: "Invalid label selector should return error",
		},
		{
			name: "namespace_not_found",
			setupClient: func() client.Client {
				scheme := runtime.NewScheme()
				_ = corev1.AddToScheme(scheme)
				return fake.NewClientBuilder().WithScheme(scheme).Build()
			},
			description: "Missing namespace should return error",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			k8sClient := test.setupClient()

			route := NewHTTPRoute(gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "missing-ns",
				},
			})

			gw := &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway",
					Namespace: "gw-ns",
				},
			}

			var listener gwv1.Listener
			if test.name == "invalid_label_selector" {
				listener = gwv1.Listener{
					Name:     "test-listener",
					Protocol: gwv1.HTTPProtocolType,
					AllowedRoutes: &gwv1.AllowedRoutes{
						Namespaces: &gwv1.RouteNamespaces{
							From: &[]gwv1.FromNamespaces{gwv1.NamespacesFromSelector}[0],
							Selector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "", // Invalid empty key
										Operator: metav1.LabelSelectorOpIn,
										Values:   []string{},
									},
								},
							},
						},
					},
				}
			} else {
				listener = gwv1.Listener{
					Name:     "test-listener",
					Protocol: gwv1.HTTPProtocolType,
					AllowedRoutes: &gwv1.AllowedRoutes{
						Namespaces: &gwv1.RouteNamespaces{
							From: &[]gwv1.FromNamespaces{gwv1.NamespacesFromSelector}[0],
							Selector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"env": "prod"},
							},
						},
					},
				}
			}

			_, err := IsRouteAllowedByListener(context.Background(), k8sClient, route, gw, listener)
			assert.Error(t, err, test.description)
		})
	}
}
