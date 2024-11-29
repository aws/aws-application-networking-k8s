package eventhandlers

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
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

func TestServiceToRoutes(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()

	routes := []gwv1.HTTPRoute{
		createHTTPRoute("invalid-kind", "ns1", gwv1.BackendObjectReference{
			Kind: (*gwv1.Kind)(ptr.To("NotService")),
			Name: "test-service",
		}),
		createHTTPRoute("invalid-nil-kind", "ns1", gwv1.BackendObjectReference{
			Kind:      nil,
			Namespace: nil,
			Name:      "test-service",
		}),
		createHTTPRoute("valid-nil-group", "ns1", gwv1.BackendObjectReference{
			Group:     nil,
			Kind:      (*gwv1.Kind)(ptr.To("Service")),
			Namespace: nil,
			Name:      "test-service",
		}),
		createHTTPRoute("invalid-group", "ns1", gwv1.BackendObjectReference{
			Group:     (*gwv1.Group)(ptr.To("not-core")),
			Kind:      (*gwv1.Kind)(ptr.To("Service")),
			Namespace: nil,
			Name:      "test-service",
		}),
		createHTTPRoute("valid-inferred-namespace", "ns1", gwv1.BackendObjectReference{
			Group:     (*gwv1.Group)(ptr.To("")),
			Kind:      (*gwv1.Kind)(ptr.To("Service")),
			Namespace: nil,
			Name:      "test-service",
		}),
		createHTTPRoute("valid-explicit-namespace", "ns1", gwv1.BackendObjectReference{
			Group:     (*gwv1.Group)(ptr.To("")),
			Kind:      (*gwv1.Kind)(ptr.To("Service")),
			Namespace: (*gwv1.Namespace)(ptr.To("ns1")),
			Name:      "test-service",
		}),
		createHTTPRoute("invalid-different-namespace", "ns1", gwv1.BackendObjectReference{
			Kind:      (*gwv1.Kind)(ptr.To("Service")),
			Namespace: (*gwv1.Namespace)(ptr.To("ns2")),
			Name:      "test-service",
		}),
		createHTTPRoute("invalid-different-name", "ns1", gwv1.BackendObjectReference{
			Kind:      (*gwv1.Kind)(ptr.To("Service")),
			Namespace: (*gwv1.Namespace)(ptr.To("ns1")),
			Name:      "not-test-service",
		}),
	}
	validRoutes := []string{
		"valid-nil-group",
		"valid-inferred-namespace",
		"valid-explicit-namespace",
	}

	mockClient := mock_client.NewMockClient(c)
	mockClient.EXPECT().List(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, routeList *gwv1.HTTPRouteList, _ ...interface{}) error {
			routeList.Items = append(routeList.Items, routes...)
			return nil
		},
	)

	mapper := &resourceMapper{log: gwlog.FallbackLogger, client: mockClient}
	res := mapper.ServiceToRoutes(context.Background(), &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-service",
			Namespace: "ns1",
		},
	}, core.HttpRouteType)

	assert.Len(t, res, len(validRoutes))
	for i, r := range res {
		assert.Equal(t, validRoutes[i], r.Name())
	}
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
