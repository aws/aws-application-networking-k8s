package eventhandlers

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	gateway_api_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gateway_api "sigs.k8s.io/gateway-api/apis/v1beta1"

	mock_client "github.com/aws/aws-application-networking-k8s/mocks/controller-runtime/client"
	"github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

func createHTTPRoute(name, namespace string, backendRef gateway_api.BackendObjectReference) gateway_api.HTTPRoute {
	return gateway_api.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: gateway_api.HTTPRouteSpec{
			Rules: []gateway_api.HTTPRouteRule{
				{
					BackendRefs: []gateway_api.HTTPBackendRef{
						{
							BackendRef: gateway_api.BackendRef{
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

	routes := []gateway_api.HTTPRoute{
		createHTTPRoute("invalid-kind", "ns1", gateway_api.BackendObjectReference{
			Kind: (*gateway_api.Kind)(pointer.String("NotService")),
			Name: "test-service",
		}),
		createHTTPRoute("invalid-nil-kind", "ns1", gateway_api.BackendObjectReference{
			Kind:      nil,
			Namespace: nil,
			Name:      "test-service",
		}),
		createHTTPRoute("invalid-nil-group", "ns1", gateway_api.BackendObjectReference{
			Kind:      (*gateway_api.Kind)(pointer.String("Service")),
			Namespace: nil,
			Name:      "test-service",
		}),
		createHTTPRoute("invalid-group", "ns1", gateway_api.BackendObjectReference{
			Group:     (*gateway_api.Group)(pointer.String("not-core")),
			Kind:      (*gateway_api.Kind)(pointer.String("Service")),
			Namespace: nil,
			Name:      "test-service",
		}),
		createHTTPRoute("valid-inferred-namespace", "ns1", gateway_api.BackendObjectReference{
			Group:     (*gateway_api.Group)(pointer.String("")),
			Kind:      (*gateway_api.Kind)(pointer.String("Service")),
			Namespace: nil,
			Name:      "test-service",
		}),
		createHTTPRoute("valid-explicit-namespace", "ns1", gateway_api.BackendObjectReference{
			Group:     (*gateway_api.Group)(pointer.String("")),
			Kind:      (*gateway_api.Kind)(pointer.String("Service")),
			Namespace: (*gateway_api.Namespace)(pointer.String("ns1")),
			Name:      "test-service",
		}),
		createHTTPRoute("invalid-different-namespace", "ns1", gateway_api.BackendObjectReference{
			Kind:      (*gateway_api.Kind)(pointer.String("Service")),
			Namespace: (*gateway_api.Namespace)(pointer.String("ns2")),
			Name:      "test-service",
		}),
		createHTTPRoute("invalid-different-name", "ns1", gateway_api.BackendObjectReference{
			Kind:      (*gateway_api.Kind)(pointer.String("Service")),
			Namespace: (*gateway_api.Namespace)(pointer.String("ns1")),
			Name:      "not-test-service",
		}),
	}
	validRoutes := []string{
		"valid-inferred-namespace",
		"valid-explicit-namespace",
	}

	mockClient := mock_client.NewMockClient(c)
	mockClient.EXPECT().List(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, routeList *gateway_api.HTTPRouteList, _ ...interface{}) error {
			for _, route := range routes {
				routeList.Items = append(routeList.Items, route)
			}
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
		targetKind      gateway_api.Kind
		targetNamespace *gateway_api.Namespace
		serviceFound    bool
		success         bool
	}{
		{
			namespace:       ns1,
			targetKind:      "Service",
			targetNamespace: (*gateway_api.Namespace)(&ns2),
			success:         false,
		},
		{
			namespace:       ns1,
			targetKind:      "NotService",
			targetNamespace: (*gateway_api.Namespace)(&ns1),
			success:         false,
		},
		{
			namespace:       ns1,
			targetKind:      "Service",
			targetNamespace: (*gateway_api.Namespace)(&ns1),
			serviceFound:    false,
			success:         false,
		},
		{
			namespace:       ns1,
			targetKind:      "Service",
			targetNamespace: (*gateway_api.Namespace)(&ns1),
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

	for _, tt := range testCases {
		mockClient := mock_client.NewMockClient(c)
		mapper := &resourceMapper{log: gwlog.FallbackLogger, client: mockClient}
		if tt.serviceFound {
			mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		} else {
			mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("fail")).AnyTimes()
		}
		svc := mapper.TargetGroupPolicyToService(context.Background(), &v1alpha1.TargetGroupPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-policy",
				Namespace: tt.namespace,
			},
			Spec: v1alpha1.TargetGroupPolicySpec{
				TargetRef: &gateway_api_v1alpha2.PolicyTargetReference{
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
		targetKind      gateway_api.Kind
		targetNamespace *gateway_api.Namespace
		gatewayFound    bool
		expectSuccess   bool
	}{
		{
			testCaseName:    "namespace not match",
			namespace:       ns1,
			targetKind:      "Gateway",
			targetNamespace: (*gateway_api.Namespace)(&ns2),
			expectSuccess:   false,
		},
		{
			testCaseName:    "targetKind not match scenario 1",
			namespace:       ns1,
			targetKind:      "NotGateway",
			targetNamespace: (*gateway_api.Namespace)(&ns1),
			expectSuccess:   false,
		},
		{
			testCaseName:    "targetKind not match scenario 2",
			namespace:       ns1,
			targetKind:      "Service",
			targetNamespace: (*gateway_api.Namespace)(&ns1),
			expectSuccess:   false,
		},
		{
			testCaseName:    "gateway not found",
			namespace:       ns1,
			targetKind:      "Gateway",
			targetNamespace: (*gateway_api.Namespace)(&ns1),
			gatewayFound:    false,
			expectSuccess:   false,
		},
		{
			testCaseName:    "gateway found, targetRef namespace match",
			namespace:       ns1,
			targetKind:      "Gateway",
			targetNamespace: (*gateway_api.Namespace)(&ns1),
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
		mockClient := mock_client.NewMockClient(c)
		mapper := &resourceMapper{log: gwlog.FallbackLogger, client: mockClient}
		if tt.gatewayFound {
			mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		} else {
			mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("fail")).AnyTimes()
		}
		var targetRefGroupName string
		if tt.targetKind == "Gateway" {
			targetRefGroupName = gateway_api.GroupName
		} else if tt.targetKind == "Service" {
			targetRefGroupName = corev1.GroupName
		}

		gw := mapper.VpcAssociationPolicyToGateway(context.Background(), &v1alpha1.VpcAssociationPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test--vpc-association-policy",
				Namespace: tt.namespace,
			},

			Spec: v1alpha1.VpcAssociationPolicySpec{
				TargetRef: &gateway_api_v1alpha2.PolicyTargetReference{
					Group:     gateway_api.Group(targetRefGroupName),
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
	}
}
