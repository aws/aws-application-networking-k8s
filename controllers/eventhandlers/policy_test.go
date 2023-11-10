package eventhandlers

import (
	"context"
	mock_client "github.com/aws/aws-application-networking-k8s/mocks/controller-runtime/client"
	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	"testing"
)

func TestMapObjectToPolicy(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()

	mockClient := mock_client.NewMockClient(c)
	h := NewPolicyEventHandler(gwlog.FallbackLogger, mockClient, &anv1alpha1.VpcAssociationPolicy{})

	policy := anv1alpha1.VpcAssociationPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gw-1-policy",
			Namespace: "default",
		},
		Spec: anv1alpha1.VpcAssociationPolicySpec{
			TargetRef: &gwv1alpha2.PolicyTargetReference{
				Group: gwv1alpha2.GroupName,
				Name:  "gw-1",
				Kind:  "Gateway",
			},
		},
	}

	mockClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, list *anv1alpha1.VpcAssociationPolicyList, arg3 ...interface{}) error {
			list.Items = append(list.Items, policy)
			return nil
		})

	reqs := h.mapObjectToPolicy(&gwv1alpha2.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gw-1",
			Namespace: "default",
		},
	})
	assert.Len(t, reqs, 1)
	assert.Equal(t, "gw-1-policy", reqs[0].Name)
}
