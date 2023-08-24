package eventhandlers

import (
	"context"
	"errors"
	mock_client "github.com/aws/aws-application-networking-k8s/mocks/controller-runtime/client"
	"github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	gateway_api_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gateway_api "sigs.k8s.io/gateway-api/apis/v1beta1"
	mcs_api "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
	"testing"
)

func TestGetTargetRef(t *testing.T) {
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
		h := NewTargetGroupPolicyEventHandler(gwlog.NewLogger(true), mockClient)
		if tt.serviceFound {
			mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		} else {
			mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("fail")).AnyTimes()
		}
		svc := h.getTargetRef(&v1alpha1.TargetGroupPolicy{
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

func TestMapToHTTPRoute(t *testing.T) {
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
		createHTTPRoute("valid-inferred-namespace", "ns1", gateway_api.BackendObjectReference{
			Kind:      (*gateway_api.Kind)(pointer.String("Service")),
			Namespace: nil,
			Name:      "test-service",
		}),
		createHTTPRoute("valid-explicit-namespace", "ns1", gateway_api.BackendObjectReference{
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
	h := NewTargetGroupPolicyEventHandler(gwlog.NewLogger(true), mockClient)
	mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, svc *corev1.Service, _ ...interface{}) error {
			svc.SetName("test-service")
			svc.SetNamespace("ns1")
			return nil
		},
	)
	mockClient.EXPECT().List(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, routeList *gateway_api.HTTPRouteList, _ ...interface{}) error {
			for _, route := range routes {
				routeList.Items = append(routeList.Items, route)
			}
			return nil
		},
	)
	reqs := h.MapToHTTPRoute(&v1alpha1.TargetGroupPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-policy",
			Namespace: "ns1",
		},
		Spec: v1alpha1.TargetGroupPolicySpec{
			TargetRef: &gateway_api_v1alpha2.PolicyTargetReference{
				Group: "",
				Kind:  "Service",
				Name:  "test-service",
			},
		},
	})
	assert.Len(t, reqs, len(validRoutes))
	for i, req := range reqs {
		assert.Equal(t, validRoutes[i], req.Name)
	}
}

func TestMapToServiceExport(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()

	mockClient := mock_client.NewMockClient(c)
	h := NewTargetGroupPolicyEventHandler(gwlog.NewLogger(true), mockClient)
	mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, svc *corev1.Service, _ ...interface{}) error {
			svc.SetName("test-service")
			svc.SetNamespace("ns1")
			return nil
		},
	)
	mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, svc *mcs_api.ServiceExport, _ ...interface{}) error {
			svc.SetName("test-service")
			svc.SetNamespace("ns1")
			return nil
		},
	)

	reqs := h.MapToServiceExport(&v1alpha1.TargetGroupPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-policy",
			Namespace: "ns1",
		},
		Spec: v1alpha1.TargetGroupPolicySpec{
			TargetRef: &gateway_api_v1alpha2.PolicyTargetReference{
				Group: "",
				Kind:  "Service",
				Name:  "test-service",
			},
		},
	})
	assert.Len(t, reqs, 1)
	assert.Equal(t, "test-service", reqs[0].Name)
}
