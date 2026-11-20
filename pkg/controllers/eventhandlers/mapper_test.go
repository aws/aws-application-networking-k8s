package eventhandlers

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	mock_client "github.com/aws/aws-application-networking-k8s/mocks/controller-runtime/client"
	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

func createHTTPRoute(name, namespace string, backendRef gwv1.BackendObjectReference) gwv1.HTTPRoute {
	return gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: gwv1.HTTPRouteSpec{
			Rules: []gwv1.HTTPRouteRule{
				{
					BackendRefs: []gwv1.HTTPBackendRef{
						{
							BackendRef: gwv1.BackendRef{
								BackendObjectReference: backendRef,
							},
						},
					},
				},
			},
		},
	}
}

func TestBackendRefToRoutes(t *testing.T) {
	svcRef := func(group *string, ns *string, name string) gwv1.BackendObjectReference {
		return gwv1.BackendObjectReference{
			Group:     (*gwv1.Group)(group),
			Kind:      (*gwv1.Kind)(ptr.To("Service")),
			Namespace: (*gwv1.Namespace)(ns),
			Name:      gwv1.ObjectName(name),
		}
	}
	importRef := func(group *string) gwv1.BackendObjectReference {
		return gwv1.BackendObjectReference{
			Group: (*gwv1.Group)(group),
			Kind:  (*gwv1.Kind)(ptr.To("ServiceImport")),
			Name:  "test-service",
		}
	}
	httpRoute := func(name, ns string, refs ...gwv1.BackendObjectReference) *gwv1.HTTPRoute {
		route := createHTTPRoute(name, ns, refs[0])
		for _, ref := range refs[1:] {
			route.Spec.Rules = append(route.Spec.Rules, gwv1.HTTPRouteRule{
				BackendRefs: []gwv1.HTTPBackendRef{{BackendRef: gwv1.BackendRef{BackendObjectReference: ref}}},
			})
		}
		return &route
	}

	objs := []client.Object{
		httpRoute("nil-group", "ns1", svcRef(nil, nil, "test-service")),
		httpRoute("empty-group", "ns1", svcRef(ptr.To(""), nil, "test-service")),
		httpRoute("non-core-group", "ns1", svcRef(ptr.To("not-core"), nil, "test-service")),
		httpRoute("nil-kind", "ns1", gwv1.BackendObjectReference{Name: "test-service"}),
		httpRoute("other-kind", "ns1", gwv1.BackendObjectReference{Kind: (*gwv1.Kind)(ptr.To("NotService")), Name: "test-service"}),
		httpRoute("other-name", "ns1", svcRef(nil, nil, "not-test-service")),
		httpRoute("cross-ns-in", "ns2", svcRef(nil, ptr.To("ns1"), "test-service")),
		httpRoute("cross-ns-out", "ns1", svcRef(nil, ptr.To("ns2"), "test-service")),
		httpRoute("duplicate", "ns1", svcRef(nil, nil, "test-service"), svcRef(ptr.To(""), ptr.To("ns1"), "test-service")),
		httpRoute("import-nil-group", "ns1", importRef(nil)),
		httpRoute("import-group", "ns1", importRef(ptr.To(anv1alpha1.GroupName))),
		&gwv1.GRPCRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "grpc", Namespace: "ns1"},
			Spec: gwv1.GRPCRouteSpec{Rules: []gwv1.GRPCRouteRule{{BackendRefs: []gwv1.GRPCBackendRef{
				{BackendRef: gwv1.BackendRef{BackendObjectReference: svcRef(nil, nil, "test-service")}},
			}}}},
		},
		&gwv1.TLSRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "tls", Namespace: "ns1"},
			Spec: gwv1.TLSRouteSpec{Rules: []gwv1.TLSRouteRule{{BackendRefs: []gwv1.BackendRef{
				{BackendObjectReference: svcRef(nil, nil, "test-service")},
			}}}},
		},
	}

	scheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(scheme)
	gwv1.Install(scheme)
	builder := testclient.NewClientBuilder().WithScheme(scheme).WithObjects(objs...)
	for _, obj := range []client.Object{&gwv1.HTTPRoute{}, &gwv1.GRPCRoute{}, &gwv1.TLSRoute{}} {
		builder = builder.WithIndex(obj, routeBackendRefIndex, routeBackendRefKeys)
	}
	mapper := &resourceMapper{log: gwlog.FallbackLogger, client: builder.Build()}
	objMeta := metav1.ObjectMeta{Name: "test-service", Namespace: "ns1"}

	names := func(routes []core.Route) []string {
		var res []string
		for _, r := range routes {
			res = append(res, r.Namespace()+"/"+r.Name())
		}
		return res
	}
	ctx := context.Background()
	svc := &corev1.Service{ObjectMeta: objMeta}
	svcImport := &anv1alpha1.ServiceImport{ObjectMeta: objMeta}
	assert.ElementsMatch(t, []string{"ns1/nil-group", "ns1/empty-group", "ns2/cross-ns-in", "ns1/duplicate"},
		names(mapper.ServiceToRoutes(ctx, svc, core.HttpRouteType)))
	assert.ElementsMatch(t, []string{"ns1/grpc"}, names(mapper.ServiceToRoutes(ctx, svc, core.GrpcRouteType)))
	assert.ElementsMatch(t, []string{"ns1/tls"}, names(mapper.ServiceToRoutes(ctx, svc, core.TlsRouteType)))
	assert.ElementsMatch(t, []string{"ns1/import-nil-group", "ns1/import-group"},
		names(mapper.ServiceImportToRoutes(ctx, svcImport, core.HttpRouteType)))
}

func TestBackendRefToRoutesListError(t *testing.T) {
	scheme := runtime.NewScheme()
	gwv1.Install(scheme)
	k8sRoute := createHTTPRoute("route", "ns1", gwv1.BackendObjectReference{Kind: (*gwv1.Kind)(ptr.To("Service")), Name: "test-service"})
	// No index registered, so the List by backendRef fails.
	mapper := &resourceMapper{log: gwlog.FallbackLogger, client: testclient.NewClientBuilder().WithScheme(scheme).WithObjects(&k8sRoute).Build()}
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "test-service", Namespace: "ns1"}}
	assert.Nil(t, mapper.ServiceToRoutes(context.Background(), svc, core.HttpRouteType))
}

func TestTargetGroupPolicyToService(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()

	ns1 := "default"
	ns2 := "non-default"

	testCases := []struct {
		namespace       string
		targetKind      gwv1.Kind
		targetNamespace *gwv1.Namespace
		serviceFound    bool
		success         bool
	}{
		{
			namespace:       ns1,
			targetKind:      "Service",
			targetNamespace: (*gwv1.Namespace)(&ns2),
			success:         false,
		},
		{
			namespace:       ns1,
			targetKind:      "NotService",
			targetNamespace: (*gwv1.Namespace)(&ns1),
			success:         false,
		},
		{
			namespace:       ns1,
			targetKind:      "Service",
			targetNamespace: (*gwv1.Namespace)(&ns1),
			serviceFound:    false,
			success:         false,
		},
		{
			namespace:       ns1,
			targetKind:      "Service",
			targetNamespace: (*gwv1.Namespace)(&ns1),
			serviceFound:    true,
			success:         true,
		},
		{
			namespace:       ns1,
			targetKind:      "Service",
			targetNamespace: nil,
			serviceFound:    true,
			success:         true,
		},
	}

	for i, tt := range testCases {
		t.Run(fmt.Sprintf("TGPolicyToService_%d", i), func(t *testing.T) {
			mockClient := mock_client.NewMockClient(c)
			mapper := &resourceMapper{log: gwlog.FallbackLogger, client: mockClient}
			if tt.serviceFound {
				mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			} else {
				mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("fail")).AnyTimes()
			}
			svc := mapper.TargetGroupPolicyToService(context.Background(), &anv1alpha1.TargetGroupPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-policy",
					Namespace: tt.namespace,
				},
				Spec: anv1alpha1.TargetGroupPolicySpec{
					TargetRef: &gwv1alpha2.NamespacedPolicyTargetReference{
						Group:     "",
						Kind:      tt.targetKind,
						Name:      "test-service",
						Namespace: tt.targetNamespace,
					},
				},
			})
			if tt.success {
				assert.NotNil(t, svc)
			} else {
				assert.Nil(t, svc)
			}
		})
	}
}

func TestVpcAssociationPolicyToGateway(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()

	ns1 := "default"
	ns2 := "non-default"

	testCases := []struct {
		testCaseName    string
		namespace       string
		targetKind      gwv1.Kind
		targetNamespace *gwv1.Namespace
		gatewayFound    bool
		expectSuccess   bool
	}{
		{
			testCaseName:    "namespace not match",
			namespace:       ns1,
			targetKind:      "Gateway",
			targetNamespace: (*gwv1.Namespace)(&ns2),
			expectSuccess:   false,
		},
		{
			testCaseName:    "targetKind not match scenario 1",
			namespace:       ns1,
			targetKind:      "NotGateway",
			targetNamespace: (*gwv1.Namespace)(&ns1),
			expectSuccess:   false,
		},
		{
			testCaseName:    "targetKind not match scenario 2",
			namespace:       ns1,
			targetKind:      "Service",
			targetNamespace: (*gwv1.Namespace)(&ns1),
			expectSuccess:   false,
		},
		{
			testCaseName:    "gateway not found",
			namespace:       ns1,
			targetKind:      "Gateway",
			targetNamespace: (*gwv1.Namespace)(&ns1),
			gatewayFound:    false,
			expectSuccess:   false,
		},
		{
			testCaseName:    "gateway found, targetRef namespace match",
			namespace:       ns1,
			targetKind:      "Gateway",
			targetNamespace: (*gwv1.Namespace)(&ns1),
			gatewayFound:    true,
			expectSuccess:   true,
		},
		{
			testCaseName:    "gateway found, targetRef namespace not defined",
			namespace:       ns1,
			targetKind:      "Gateway",
			targetNamespace: nil,
			gatewayFound:    true,
			expectSuccess:   true,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.testCaseName, func(t *testing.T) {
			mockClient := mock_client.NewMockClient(c)
			mapper := &resourceMapper{log: gwlog.FallbackLogger, client: mockClient}
			if tt.gatewayFound {
				mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			} else {
				mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("fail")).AnyTimes()
			}
			var targetRefGroupName string
			if tt.targetKind == "Gateway" {
				targetRefGroupName = gwv1.GroupName
			} else if tt.targetKind == "Service" {
				targetRefGroupName = corev1.GroupName
			}

			gw := mapper.VpcAssociationPolicyToGateway(context.Background(), &anv1alpha1.VpcAssociationPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test--vpc-association-policy",
					Namespace: tt.namespace,
				},

				Spec: anv1alpha1.VpcAssociationPolicySpec{
					TargetRef: &gwv1alpha2.NamespacedPolicyTargetReference{
						Group:     gwv1.Group(targetRefGroupName),
						Kind:      tt.targetKind,
						Name:      "test-gw",
						Namespace: tt.targetNamespace,
					},
				},
			})
			if tt.expectSuccess {
				assert.NotNil(t, gw)
			} else {
				assert.Nil(t, gw)
			}
		})
	}
}

func TestServiceToServiceExport(t *testing.T) {
	tests := []struct {
		name         string
		svc          *corev1.Service
		svcExports   []anv1alpha1.ServiceExport
		expectFound  bool
		expectedName string
	}{
		{
			name: "nil service returns nil",
			svc:  nil,
		},
		{
			name: "direct name match - ServiceExport name equals Service name",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "checkout",
					Namespace: "default",
				},
			},
			svcExports: []anv1alpha1.ServiceExport{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "checkout",
						Namespace: "default",
					},
				},
			},
			expectFound:  true,
			expectedName: "checkout",
		},
		{
			name: "annotation reverse lookup - ServiceExport has service-name annotation",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "checkout",
					Namespace: "default",
				},
			},
			svcExports: []anv1alpha1.ServiceExport{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "checkout-east",
						Namespace: "default",
						Annotations: map[string]string{
							"application-networking.k8s.aws/service-name": "checkout",
						},
					},
				},
			},
			expectFound:  true,
			expectedName: "checkout-east",
		},
		{
			name: "no match - no ServiceExport with matching name or annotation",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "checkout",
					Namespace: "default",
				},
			},
			svcExports: []anv1alpha1.ServiceExport{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "other-export",
						Namespace: "default",
						Annotations: map[string]string{
							"application-networking.k8s.aws/service-name": "other-service",
						},
					},
				},
			},
			expectFound: false,
		},
		{
			name: "direct match takes precedence over annotation match",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "checkout",
					Namespace: "default",
				},
			},
			svcExports: []anv1alpha1.ServiceExport{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "checkout",
						Namespace: "default",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "checkout-annotated",
						Namespace: "default",
						Annotations: map[string]string{
							"application-networking.k8s.aws/service-name": "checkout",
						},
					},
				},
			},
			expectFound:  true,
			expectedName: "checkout",
		},
		{
			name: "different namespace - no match",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "checkout",
					Namespace: "default",
				},
			},
			svcExports: []anv1alpha1.ServiceExport{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "checkout-east",
						Namespace: "other-ns",
						Annotations: map[string]string{
							"application-networking.k8s.aws/service-name": "checkout",
						},
					},
				},
			},
			expectFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			anv1alpha1.Install(scheme)
			clientgoscheme.AddToScheme(scheme)

			builder := testclient.NewClientBuilder().WithScheme(scheme)
			for i := range tt.svcExports {
				builder = builder.WithObjects(&tt.svcExports[i])
			}
			k8sClient := builder.Build()

			mapper := &resourceMapper{log: gwlog.FallbackLogger, client: k8sClient}
			result := mapper.ServiceToServiceExport(context.Background(), tt.svc)

			if tt.expectFound {
				assert.NotNil(t, result, "Expected ServiceExport to be found")
				assert.Equal(t, tt.expectedName, result.Name)
			} else {
				assert.Nil(t, result, "Expected no ServiceExport to be found")
			}
		})
	}
}
