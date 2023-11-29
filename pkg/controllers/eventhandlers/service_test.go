package eventhandlers

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	mock_client "github.com/aws/aws-application-networking-k8s/mocks/controller-runtime/client"
	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

func TestServiceEventHandler_MapToRoute(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()

	routes := []gwv1beta1.HTTPRoute{
		createHTTPRoute("valid-route", "ns1", gwv1beta1.BackendObjectReference{
			Group:     (*gwv1beta1.Group)(ptr.To("")),
			Kind:      (*gwv1beta1.Kind)(ptr.To("Service")),
			Namespace: (*gwv1beta1.Namespace)(ptr.To("ns1")),
			Name:      "test-service",
		}),
	}
	mockClient := mock_client.NewMockClient(c)
	h := NewServiceEventHandler(gwlog.FallbackLogger, mockClient)
	mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, svc client.Object, _ ...interface{}) error {
			svc.SetName("test-service")
			svc.SetNamespace("ns1")
			return nil
		},
	).AnyTimes()
	mockClient.EXPECT().List(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, routeList *gwv1beta1.HTTPRouteList, _ ...interface{}) error {
			routeList.Items = append(routeList.Items, routes...)
			return nil
		},
	).AnyTimes()

	objs := []client.Object{
		&anv1alpha1.TargetGroupPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-policy",
				Namespace: "ns1",
			},
			Spec: anv1alpha1.TargetGroupPolicySpec{
				TargetRef: &gwv1alpha2.PolicyTargetReference{
					Group: "",
					Kind:  "Service",
					Name:  "test-service",
				},
			},
		},
		&corev1.Endpoints{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-service",
				Namespace: "ns1",
			},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-service",
				Namespace: "ns1",
			},
		},
	}
	for _, obj := range objs {
		reqs := h.mapToRoute(context.Background(), obj, core.HttpRouteType)
		assert.Len(t, reqs, 1)
		assert.Equal(t, "valid-route", reqs[0].Name)
	}
}

func TestServiceEventHandler_MapToServiceExport(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()

	mockClient := mock_client.NewMockClient(c)
	h := NewServiceEventHandler(gwlog.FallbackLogger, mockClient)
	mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, svcOrSvcExport client.Object, _ ...interface{}) error {
			svcOrSvcExport.SetName("test-service")
			svcOrSvcExport.SetNamespace("ns1")
			return nil
		},
	).AnyTimes()

	objs := []client.Object{
		&anv1alpha1.TargetGroupPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-policy",
				Namespace: "ns1",
			},
			Spec: anv1alpha1.TargetGroupPolicySpec{
				TargetRef: &gwv1alpha2.PolicyTargetReference{
					Group: "",
					Kind:  "Service",
					Name:  "test-service",
				},
			},
		},
		&corev1.Endpoints{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-service",
				Namespace: "ns1",
			},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-service",
				Namespace: "ns1",
			},
		},
	}
	for _, obj := range objs {
		reqs := h.mapToServiceExport(context.Background(), obj)
		assert.Len(t, reqs, 1)
		assert.Equal(t, "test-service", reqs[0].Name)
	}
}
