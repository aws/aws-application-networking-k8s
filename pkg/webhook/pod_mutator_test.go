package webhook

import (
	"context"
	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"testing"
)

func Test_ReadinessGateInjection(t *testing.T) {
	var serviceKind gwv1.Kind = "Service"
	var gwNamespace = gwv1.Namespace("gw-namespace")
	var svcNamespace = gwv1.Namespace("test")

	tests := []struct {
		name                   string
		omitGatewayClass       bool
		performUpdate          bool
		pod                    corev1.Pod
		services               []corev1.Service
		httpRoutes             []gwv1.HTTPRoute
		v1HttpRoutes           []gwv1.HTTPRoute
		grpcRoutes             []gwv1.GRPCRoute
		gateways               []gwv1.Gateway
		svcExport              *anv1alpha1.ServiceExport
		expectedConditionTypes []corev1.PodConditionType
	}{
		{
			name: "HTTP route",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod-1",
					Namespace: "test",
					Labels: map[string]string{
						"env": "test",
					},
				},
			},
			services: []corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc-1",
						Namespace: "test",
					},
					Spec: corev1.ServiceSpec{
						Selector: map[string]string{
							"env": "test",
						},
					},
				},
			},
			httpRoutes: []gwv1.HTTPRoute{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "http-route-1",
						Namespace: "test",
					},
					Spec: gwv1.HTTPRouteSpec{
						CommonRouteSpec: gwv1.CommonRouteSpec{ParentRefs: []gwv1.ParentReference{
							{Name: "gw-1"},
						}},
						Rules: []gwv1.HTTPRouteRule{
							{
								BackendRefs: []gwv1.HTTPBackendRef{{BackendRef: gwv1.BackendRef{
									BackendObjectReference: gwv1.BackendObjectReference{
										Name: "svc-1",
										Kind: &serviceKind,
									},
								}}},
							},
						},
					},
				},
			},
			gateways: []gwv1.Gateway{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gw-1",
						Namespace: "test",
					},
					Spec: gwv1.GatewaySpec{
						GatewayClassName: "amazon-vpc-lattice",
					},
				},
			},
			expectedConditionTypes: []corev1.PodConditionType{
				corev1.PodConditionType(PodReadinessGateConditionType),
			},
		},
		{
			name: "GRPC Route",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod-1",
					Namespace: "test",
					Labels: map[string]string{
						"env": "test",
					},
				},
			},
			services: []corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc-1",
						Namespace: "test",
					},
					Spec: corev1.ServiceSpec{
						Selector: map[string]string{
							"env": "test",
						},
					},
				},
			},
			grpcRoutes: []gwv1.GRPCRoute{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "http-route-1",
						Namespace: "test",
					},
					Spec: gwv1.GRPCRouteSpec{
						CommonRouteSpec: gwv1.CommonRouteSpec{ParentRefs: []gwv1.ParentReference{
							{Name: "gw-1"},
						}},
						Rules: []gwv1.GRPCRouteRule{
							{
								BackendRefs: []gwv1.GRPCBackendRef{{BackendRef: gwv1.BackendRef{
									BackendObjectReference: gwv1.BackendObjectReference{
										Name: "svc-1",
										Kind: &serviceKind,
									},
								}}},
							},
						},
					},
				},
			},
			gateways: []gwv1.Gateway{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gw-1",
						Namespace: "test",
					},
					Spec: gwv1.GatewaySpec{
						GatewayClassName: "amazon-vpc-lattice",
					},
				},
			},
			expectedConditionTypes: []corev1.PodConditionType{
				corev1.PodConditionType(PodReadinessGateConditionType),
			},
		},
		{
			name: "service export",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod-1",
					Namespace: "test",
					Labels: map[string]string{
						"env": "test",
					},
				},
			},
			services: []corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc-1",
						Namespace: "test",
					},
					Spec: corev1.ServiceSpec{
						Selector: map[string]string{
							"env": "test",
						},
					},
				},
			},
			svcExport: &anv1alpha1.ServiceExport{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc-1",
					Namespace: "test",
					Annotations: map[string]string{
						"application-networking.k8s.aws/federation": "amazon-vpc-lattice",
					},
				},
			},
			expectedConditionTypes: []corev1.PodConditionType{
				corev1.PodConditionType(PodReadinessGateConditionType),
			},
		},
		{
			name: "service, route, gateway different namespaces, but referencing works",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod-1",
					Namespace: "test",
					Labels: map[string]string{
						"env": "test",
					},
				},
			},
			services: []corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc-1",
						Namespace: string(svcNamespace),
					},
					Spec: corev1.ServiceSpec{
						Selector: map[string]string{
							"env": "test",
						},
					},
				},
			},
			httpRoutes: []gwv1.HTTPRoute{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "http-route-1",
						Namespace: "route-namespace",
					},
					Spec: gwv1.HTTPRouteSpec{
						CommonRouteSpec: gwv1.CommonRouteSpec{ParentRefs: []gwv1.ParentReference{
							{
								Name:      "gw-1",
								Namespace: &gwNamespace,
							},
						}},
						Rules: []gwv1.HTTPRouteRule{
							{
								BackendRefs: []gwv1.HTTPBackendRef{{BackendRef: gwv1.BackendRef{
									BackendObjectReference: gwv1.BackendObjectReference{
										Name:      "svc-1",
										Namespace: &svcNamespace,
										Kind:      &serviceKind,
									},
								}}},
							},
						},
					},
				},
			},
			gateways: []gwv1.Gateway{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gw-1",
						Namespace: string(gwNamespace),
					},
					Spec: gwv1.GatewaySpec{
						GatewayClassName: "amazon-vpc-lattice",
					},
				},
			},
			expectedConditionTypes: []corev1.PodConditionType{
				corev1.PodConditionType(PodReadinessGateConditionType),
			},
		},
		{
			name: "service, route, gateway different namespaces, do not reference each other",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod-1",
					Namespace: "test",
					Labels: map[string]string{
						"env": "test",
					},
				},
			},
			services: []corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc-1",
						Namespace: string(svcNamespace),
					},
					Spec: corev1.ServiceSpec{
						Selector: map[string]string{
							"env": "test",
						},
					},
				},
			},
			httpRoutes: []gwv1.HTTPRoute{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "http-route-1",
						Namespace: "route-namespace",
					},
					Spec: gwv1.HTTPRouteSpec{
						CommonRouteSpec: gwv1.CommonRouteSpec{ParentRefs: []gwv1.ParentReference{
							{
								Name: "gw-1",
							},
						}},
						Rules: []gwv1.HTTPRouteRule{
							{
								BackendRefs: []gwv1.HTTPBackendRef{{BackendRef: gwv1.BackendRef{
									BackendObjectReference: gwv1.BackendObjectReference{
										Name: "svc-1",
										Kind: &serviceKind,
									},
								}}},
							},
						},
					},
				},
			},
			gateways: []gwv1.Gateway{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gw-1",
						Namespace: string(gwNamespace),
					},
					Spec: gwv1.GatewaySpec{
						GatewayClassName: "amazon-vpc-lattice",
					},
				},
			},
			expectedConditionTypes: []corev1.PodConditionType{},
		},
		{
			name: "no service",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod-1",
					Namespace: "test",
					Labels: map[string]string{
						"env": "test",
					},
				},
			},
			expectedConditionTypes: []corev1.PodConditionType{},
		},
		{
			name: "service labels do not match",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod-1",
					Namespace: "test",
					Labels: map[string]string{
						"env": "test",
					},
				},
			},
			services: []corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc-1",
						Namespace: "test",
					},
					Spec: corev1.ServiceSpec{
						Selector: map[string]string{
							"env": "PROD",
						},
					},
				},
			},
			expectedConditionTypes: []corev1.PodConditionType{},
		},
		{
			name: "no route",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod-1",
					Namespace: "test",
					Labels: map[string]string{
						"env": "test",
					},
				},
			},
			services: []corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc-1",
						Namespace: "test",
					},
					Spec: corev1.ServiceSpec{
						Selector: map[string]string{
							"env": "test",
						},
					},
				},
			},
			gateways: []gwv1.Gateway{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gw-1",
						Namespace: "test",
					},
					Spec: gwv1.GatewaySpec{
						GatewayClassName: "amazon-vpc-lattice",
					},
				},
			},
			expectedConditionTypes: []corev1.PodConditionType{},
		},
		{
			name: "no gateway",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod-1",
					Namespace: "test",
					Labels: map[string]string{
						"env": "test",
					},
				},
			},
			services: []corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc-1",
						Namespace: "test",
					},
					Spec: corev1.ServiceSpec{
						Selector: map[string]string{
							"env": "test",
						},
					},
				},
			},
			httpRoutes: []gwv1.HTTPRoute{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "http-route-1",
						Namespace: "test",
					},
					Spec: gwv1.HTTPRouteSpec{
						CommonRouteSpec: gwv1.CommonRouteSpec{ParentRefs: []gwv1.ParentReference{
							{Name: "gw-1"},
						}},
						Rules: []gwv1.HTTPRouteRule{
							{
								BackendRefs: []gwv1.HTTPBackendRef{{BackendRef: gwv1.BackendRef{
									BackendObjectReference: gwv1.BackendObjectReference{
										Name: "svc-1",
										Kind: &serviceKind,
									},
								}}},
							},
						},
					},
				},
			},
			expectedConditionTypes: []corev1.PodConditionType{},
		},
		{
			name: "HTTP route other gateway type",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod-1",
					Namespace: "test",
					Labels: map[string]string{
						"env": "test",
					},
				},
			},
			services: []corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc-1",
						Namespace: "test",
					},
					Spec: corev1.ServiceSpec{
						Selector: map[string]string{
							"env": "test",
						},
					},
				},
			},
			httpRoutes: []gwv1.HTTPRoute{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "http-route-1",
						Namespace: "test",
					},
					Spec: gwv1.HTTPRouteSpec{
						CommonRouteSpec: gwv1.CommonRouteSpec{ParentRefs: []gwv1.ParentReference{
							{Name: "gw-1"},
						}},
						Rules: []gwv1.HTTPRouteRule{
							{
								BackendRefs: []gwv1.HTTPBackendRef{{BackendRef: gwv1.BackendRef{
									BackendObjectReference: gwv1.BackendObjectReference{
										Name: "svc-1",
										Kind: &serviceKind,
									},
								}}},
							},
						},
					},
				},
			},
			gateways: []gwv1.Gateway{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gw-1",
						Namespace: "test",
					},
					Spec: gwv1.GatewaySpec{
						GatewayClassName: "some-other-gateway-type",
					},
				},
			},
			expectedConditionTypes: []corev1.PodConditionType{},
		},
		{
			name: "GRPC route other gateway type",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod-1",
					Namespace: "test",
					Labels: map[string]string{
						"env": "test",
					},
				},
			},
			services: []corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc-1",
						Namespace: "test",
					},
					Spec: corev1.ServiceSpec{
						Selector: map[string]string{
							"env": "test",
						},
					},
				},
			},
			grpcRoutes: []gwv1.GRPCRoute{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "http-route-1",
						Namespace: "test",
					},
					Spec: gwv1.GRPCRouteSpec{
						CommonRouteSpec: gwv1.CommonRouteSpec{ParentRefs: []gwv1.ParentReference{
							{Name: "gw-1"},
						}},
						Rules: []gwv1.GRPCRouteRule{
							{
								BackendRefs: []gwv1.GRPCBackendRef{{BackendRef: gwv1.BackendRef{
									BackendObjectReference: gwv1.BackendObjectReference{
										Name: "svc-1",
										Kind: &serviceKind,
									},
								}}},
							},
						},
					},
				},
			},
			gateways: []gwv1.Gateway{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gw-1",
						Namespace: "test",
					},
					Spec: gwv1.GatewaySpec{
						GatewayClassName: "other-gateway-type",
					},
				},
			},
			expectedConditionTypes: []corev1.PodConditionType{},
		},
		{
			name: "not modified - empty pod condition remains unchanged",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod-1",
					Namespace: "test",
					Labels: map[string]string{
						"env": "test",
					},
				},
			},
			expectedConditionTypes: []corev1.PodConditionType{},
		},
		{
			name: "not modified - existing pod condition remains unchanged",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod-1",
					Namespace: "test",
					Labels: map[string]string{
						"env": "test",
					},
				},
				Spec: corev1.PodSpec{ReadinessGates: []corev1.PodReadinessGate{
					{
						ConditionType: corev1.PodConditionType("some-condition"),
					},
				}},
			},
			expectedConditionTypes: []corev1.PodConditionType{
				corev1.PodConditionType("some-condition"),
			},
		},
		{
			name: "appends to existing pod condition",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod-1",
					Namespace: "test",
					Labels: map[string]string{
						"env": "test",
					},
				},
				Spec: corev1.PodSpec{ReadinessGates: []corev1.PodReadinessGate{
					{
						ConditionType: corev1.PodConditionType("some-condition"),
					},
				}},
			},
			services: []corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc-1",
						Namespace: "test",
					},
					Spec: corev1.ServiceSpec{
						Selector: map[string]string{
							"env": "test",
						},
					},
				},
			},
			httpRoutes: []gwv1.HTTPRoute{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "http-route-1",
						Namespace: "test",
					},
					Spec: gwv1.HTTPRouteSpec{
						CommonRouteSpec: gwv1.CommonRouteSpec{ParentRefs: []gwv1.ParentReference{
							{Name: "gw-1"},
						}},
						Rules: []gwv1.HTTPRouteRule{
							{
								BackendRefs: []gwv1.HTTPBackendRef{{BackendRef: gwv1.BackendRef{
									BackendObjectReference: gwv1.BackendObjectReference{
										Name: "svc-1",
										Kind: &serviceKind,
									},
								}}},
							},
						},
					},
				},
			},
			gateways: []gwv1.Gateway{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gw-1",
						Namespace: "test",
					},
					Spec: gwv1.GatewaySpec{
						GatewayClassName: "amazon-vpc-lattice",
					},
				},
			},
			expectedConditionTypes: []corev1.PodConditionType{
				corev1.PodConditionType("some-condition"),
				corev1.PodConditionType(PodReadinessGateConditionType),
			},
		},
		{
			name: "service in both GRPC route and HTTP route",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod-1",
					Namespace: "test",
					Labels: map[string]string{
						"env": "test",
					},
				},
			},
			services: []corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc-1",
						Namespace: "test",
					},
					Spec: corev1.ServiceSpec{
						Selector: map[string]string{
							"env": "test",
						},
					},
				},
			},
			httpRoutes: []gwv1.HTTPRoute{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "http-route-1",
						Namespace: "test",
					},
					Spec: gwv1.HTTPRouteSpec{
						CommonRouteSpec: gwv1.CommonRouteSpec{ParentRefs: []gwv1.ParentReference{
							{Name: "gw-1"},
						}},
						Rules: []gwv1.HTTPRouteRule{
							{
								BackendRefs: []gwv1.HTTPBackendRef{{BackendRef: gwv1.BackendRef{
									BackendObjectReference: gwv1.BackendObjectReference{
										Name: "svc-1",
										Kind: &serviceKind,
									},
								}}},
							},
						},
					},
				},
			},
			grpcRoutes: []gwv1.GRPCRoute{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "http-route-1",
						Namespace: "test",
					},
					Spec: gwv1.GRPCRouteSpec{
						CommonRouteSpec: gwv1.CommonRouteSpec{ParentRefs: []gwv1.ParentReference{
							{Name: "gw-1"},
						}},
						Rules: []gwv1.GRPCRouteRule{
							{
								BackendRefs: []gwv1.GRPCBackendRef{{BackendRef: gwv1.BackendRef{
									BackendObjectReference: gwv1.BackendObjectReference{
										Name: "svc-1",
										Kind: &serviceKind,
									},
								}}},
							},
						},
					},
				},
			},
			gateways: []gwv1.Gateway{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gw-1",
						Namespace: "test",
					},
					Spec: gwv1.GatewaySpec{
						GatewayClassName: "amazon-vpc-lattice",
					},
				},
			},
			expectedConditionTypes: []corev1.PodConditionType{
				corev1.PodConditionType(PodReadinessGateConditionType),
			},
		},
		{
			name: "multiple services multiple routes",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod-1",
					Namespace: "test",
					Labels: map[string]string{
						"env": "test",
					},
				},
			},
			services: []corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc-1",
						Namespace: "test",
					},
					Spec: corev1.ServiceSpec{
						Selector: map[string]string{
							"env": "test",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc-2",
						Namespace: "test",
					},
					Spec: corev1.ServiceSpec{
						Selector: map[string]string{
							"env": "test",
						},
					},
				},
			},
			httpRoutes: []gwv1.HTTPRoute{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "http-route-1",
						Namespace: "test",
					},
					Spec: gwv1.HTTPRouteSpec{
						CommonRouteSpec: gwv1.CommonRouteSpec{ParentRefs: []gwv1.ParentReference{
							{Name: "gw-1"},
						}},
						Rules: []gwv1.HTTPRouteRule{
							{
								BackendRefs: []gwv1.HTTPBackendRef{{BackendRef: gwv1.BackendRef{
									BackendObjectReference: gwv1.BackendObjectReference{
										Name: "svc-1",
										Kind: &serviceKind,
									},
								}}},
							},
						},
					},
				},
			},
			grpcRoutes: []gwv1.GRPCRoute{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "http-route-1",
						Namespace: "test",
					},
					Spec: gwv1.GRPCRouteSpec{
						CommonRouteSpec: gwv1.CommonRouteSpec{ParentRefs: []gwv1.ParentReference{
							{Name: "gw-1"},
						}},
						Rules: []gwv1.GRPCRouteRule{
							{
								BackendRefs: []gwv1.GRPCBackendRef{{BackendRef: gwv1.BackendRef{
									BackendObjectReference: gwv1.BackendObjectReference{
										Name: "svc-2",
										Kind: &serviceKind,
									},
								}}},
							},
						},
					},
				},
			},
			gateways: []gwv1.Gateway{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gw-1",
						Namespace: "test",
					},
					Spec: gwv1.GatewaySpec{
						GatewayClassName: "amazon-vpc-lattice",
					},
				},
			},
			expectedConditionTypes: []corev1.PodConditionType{
				corev1.PodConditionType(PodReadinessGateConditionType),
			},
		},
		{
			name: "lots of objects but nothing matches but service",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod-1",
					Namespace: "test",
					Labels: map[string]string{
						"env": "test",
					},
				},
			},
			services: []corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc-1",
						Namespace: "test",
					},
					Spec: corev1.ServiceSpec{
						Selector: map[string]string{
							"env": "test",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc-2",
						Namespace: "test",
					},
					Spec: corev1.ServiceSpec{
						Selector: map[string]string{
							"env": "test",
						},
					},
				},
			},
			httpRoutes: []gwv1.HTTPRoute{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "http-route-1",
						Namespace: "test",
					},
					Spec: gwv1.HTTPRouteSpec{
						CommonRouteSpec: gwv1.CommonRouteSpec{ParentRefs: []gwv1.ParentReference{
							{Name: "gw-1"},
						}},
						Rules: []gwv1.HTTPRouteRule{
							{
								BackendRefs: []gwv1.HTTPBackendRef{{BackendRef: gwv1.BackendRef{
									BackendObjectReference: gwv1.BackendObjectReference{
										Name: "not-a-real-service",
										Kind: &serviceKind,
									},
								}}},
							},
						},
					},
				},
			},
			grpcRoutes: []gwv1.GRPCRoute{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "http-route-1",
						Namespace: "test",
					},
					Spec: gwv1.GRPCRouteSpec{
						CommonRouteSpec: gwv1.CommonRouteSpec{ParentRefs: []gwv1.ParentReference{
							{Name: "gw-1"},
						}},
						Rules: []gwv1.GRPCRouteRule{
							{
								BackendRefs: []gwv1.GRPCBackendRef{{BackendRef: gwv1.BackendRef{
									BackendObjectReference: gwv1.BackendObjectReference{
										Name: "not-a-real-service-2",
										Kind: &serviceKind,
									},
								}}},
							},
						},
					},
				},
			},
			gateways: []gwv1.Gateway{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gw-1",
						Namespace: "test",
					},
					Spec: gwv1.GatewaySpec{
						GatewayClassName: "amazon-vpc-lattice",
					},
				},
			},
			expectedConditionTypes: []corev1.PodConditionType{},
		},
		{
			name:             "gateway class missing",
			omitGatewayClass: true,
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod-1",
					Namespace: "test",
					Labels: map[string]string{
						"env": "test",
					},
				},
			},
			services: []corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc-1",
						Namespace: "test",
					},
					Spec: corev1.ServiceSpec{
						Selector: map[string]string{
							"env": "test",
						},
					},
				},
			},
			httpRoutes: []gwv1.HTTPRoute{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "http-route-1",
						Namespace: "test",
					},
					Spec: gwv1.HTTPRouteSpec{
						CommonRouteSpec: gwv1.CommonRouteSpec{ParentRefs: []gwv1.ParentReference{
							{Name: "gw-1"},
						}},
						Rules: []gwv1.HTTPRouteRule{
							{
								BackendRefs: []gwv1.HTTPBackendRef{{BackendRef: gwv1.BackendRef{
									BackendObjectReference: gwv1.BackendObjectReference{
										Name: "svc-1",
										Kind: &serviceKind,
									},
								}}},
							},
						},
					},
				},
			},
			gateways: []gwv1.Gateway{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gw-1",
						Namespace: "test",
					},
					Spec: gwv1.GatewaySpec{
						GatewayClassName: "amazon-vpc-lattice",
					},
				},
			},
			expectedConditionTypes: []corev1.PodConditionType{},
		},
		{
			name:          "Update does nothing",
			performUpdate: true,
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod-1",
					Namespace: "test",
					Labels: map[string]string{
						"env": "test",
					},
				},
			},
			services: []corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "svc-1",
						Namespace: "test",
					},
					Spec: corev1.ServiceSpec{
						Selector: map[string]string{
							"env": "test",
						},
					},
				},
			},
			httpRoutes: []gwv1.HTTPRoute{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "http-route-1",
						Namespace: "test",
					},
					Spec: gwv1.HTTPRouteSpec{
						CommonRouteSpec: gwv1.CommonRouteSpec{ParentRefs: []gwv1.ParentReference{
							{Name: "gw-1"},
						}},
						Rules: []gwv1.HTTPRouteRule{
							{
								BackendRefs: []gwv1.HTTPBackendRef{{BackendRef: gwv1.BackendRef{
									BackendObjectReference: gwv1.BackendObjectReference{
										Name: "svc-1",
										Kind: &serviceKind,
									},
								}}},
							},
						},
					},
				},
			},
			gateways: []gwv1.Gateway{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gw-1",
						Namespace: "test",
					},
					Spec: gwv1.GatewaySpec{
						GatewayClassName: "amazon-vpc-lattice",
					},
				},
			},
			expectedConditionTypes: []corev1.PodConditionType{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.TODO()

			k8sScheme := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sScheme)
			gwv1.Install(k8sScheme)
			anv1alpha1.Install(k8sScheme)

			k8sClient := testclient.NewClientBuilder().WithScheme(k8sScheme).Build()

			gwClass := &gwv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "amazon-vpc-lattice",
					Namespace: "default",
				},
				Spec: gwv1.GatewayClassSpec{
					ControllerName: "application-networking.k8s.aws/gateway-api-controller",
				},
			}
			if !tt.omitGatewayClass {
				assert.NoError(t, k8sClient.Create(ctx, gwClass.DeepCopy()))
			}

			for _, svc := range tt.services {
				assert.NoError(t, k8sClient.Create(ctx, svc.DeepCopy()))
			}
			for _, httpRoute := range tt.httpRoutes {
				assert.NoError(t, k8sClient.Create(ctx, httpRoute.DeepCopy()))
			}
			for _, v1HttpRoute := range tt.v1HttpRoutes {
				assert.NoError(t, k8sClient.Create(ctx, v1HttpRoute.DeepCopy()))
			}
			for _, grpcRoute := range tt.grpcRoutes {
				assert.NoError(t, k8sClient.Create(ctx, grpcRoute.DeepCopy()))
			}
			for _, gw := range tt.gateways {
				assert.NoError(t, k8sClient.Create(ctx, gw.DeepCopy()))
			}
			if tt.svcExport != nil {
				assert.NoError(t, k8sClient.Create(ctx, tt.svcExport.DeepCopy()))
			}

			injector := NewPodReadinessGateInjector(k8sClient, gwlog.FallbackLogger)
			m := NewPodMutator(gwlog.FallbackLogger, k8sScheme, injector)

			var retPod runtime.Object
			var err error

			if tt.performUpdate {
				retPod, err = m.MutateUpdate(context.TODO(), &tt.pod, &tt.pod)
			} else {
				retPod, err = m.MutateCreate(context.TODO(), &tt.pod)
			}
			assert.Nil(t, err)

			expectedConditionsMap := make(map[string]corev1.PodConditionType)
			for _, conditionType := range tt.expectedConditionTypes {
				expectedConditionsMap[string(conditionType)] = conditionType
			}
			actualConditionsMap := make(map[string]corev1.PodConditionType)
			for _, gate := range retPod.(*corev1.Pod).Spec.ReadinessGates {
				actualConditionsMap[string(gate.ConditionType)] = gate.ConditionType
			}

			assert.Equal(t, len(expectedConditionsMap), len(actualConditionsMap))
			for k := range expectedConditionsMap {
				_, ok := actualConditionsMap[k]
				assert.Truef(t, ok, "expected pod condition type %s not found", k)
			}
		})
	}
}
