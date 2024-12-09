package eventhandlers

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	mock_client "github.com/aws/aws-application-networking-k8s/mocks/controller-runtime/client"
	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

func TestServiceImportEventHandler_MapToRoute(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()

	routes := []gwv1.HTTPRoute{
		createHTTPRoute("valid-route", "ns1", gwv1.BackendObjectReference{
			Group:     (*gwv1.Group)(ptr.To("application-networking.k8s.aws")),
			Kind:      (*gwv1.Kind)(ptr.To("ServiceImport")),
			Namespace: (*gwv1.Namespace)(ptr.To("ns1")),
			Name:      "test-service",
		}),
	}
	mockClient := mock_client.NewMockClient(c)
	h := NewServiceImportEventHandler(gwlog.FallbackLogger, mockClient)
	mockClient.EXPECT().List(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, routeList *gwv1.HTTPRouteList, _ ...interface{}) error {
			routeList.Items = append(routeList.Items, routes...)
			return nil
		},
	).AnyTimes()

	reqs := h.mapToRoute(context.Background(), &anv1alpha1.ServiceImport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-service",
			Namespace: "ns1",
		},
	}, core.HttpRouteType)
	assert.Len(t, reqs, 1)
	assert.Equal(t, "valid-route", reqs[0].Name)
}
