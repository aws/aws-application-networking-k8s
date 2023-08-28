package eventhandlers

import (
	"context"
	mock_client "github.com/aws/aws-application-networking-k8s/mocks/controller-runtime/client"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	gateway_api "sigs.k8s.io/gateway-api/apis/v1beta1"
	mcs_api "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
	"testing"
)

func TestServiceImportEventHandler_MapToRoute(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()

	routes := []gateway_api.HTTPRoute{
		createHTTPRoute("valid-route", "ns1", gateway_api.BackendObjectReference{
			Group:     (*gateway_api.Group)(pointer.String("multicluster.x-k8s.io")),
			Kind:      (*gateway_api.Kind)(pointer.String("ServiceImport")),
			Namespace: (*gateway_api.Namespace)(pointer.String("ns1")),
			Name:      "test-service",
		}),
	}
	mockClient := mock_client.NewMockClient(c)
	h := NewServiceImportEventHandler(gwlog.FallbackLogger, mockClient)
	mockClient.EXPECT().List(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, routeList *gateway_api.HTTPRouteList, _ ...interface{}) error {
			for _, route := range routes {
				routeList.Items = append(routeList.Items, route)
			}
			return nil
		},
	).AnyTimes()

	reqs := h.mapToRoute(&mcs_api.ServiceImport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-service",
			Namespace: "ns1",
		},
	}, core.HttpRouteType)
	assert.Len(t, reqs, 1)
	assert.Equal(t, "valid-route", reqs[0].Name)
}
