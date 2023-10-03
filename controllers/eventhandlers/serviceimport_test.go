package eventhandlers

import (
	"context"
	mockclient "github.com/aws/aws-application-networking-k8s/mocks/controller-runtime/client"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
	"testing"
)

func TestServiceImportEventHandler_MapToRoute(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()

	routes := []gwv1beta1.HTTPRoute{
		createHTTPRoute("valid-route", "ns1", gwv1beta1.BackendObjectReference{
			Group:     (*gwv1beta1.Group)(pointer.String("multicluster.x-k8s.io")),
			Kind:      (*gwv1beta1.Kind)(pointer.String("ServiceImport")),
			Namespace: (*gwv1beta1.Namespace)(pointer.String("ns1")),
			Name:      "test-service",
		}),
	}
	mockClient := mockclient.NewMockClient(c)
	h := NewServiceImportEventHandler(gwlog.FallbackLogger, mockClient)
	mockClient.EXPECT().List(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, routeList *gwv1beta1.HTTPRouteList, _ ...interface{}) error {
			for _, route := range routes {
				routeList.Items = append(routeList.Items, route)
			}
			return nil
		},
	).AnyTimes()

	reqs := h.mapToRoute(&mcsv1alpha1.ServiceImport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-service",
			Namespace: "ns1",
		},
	}, core.HttpRouteType)
	assert.Len(t, reqs, 1)
	assert.Equal(t, "valid-route", reqs[0].Name)
}
