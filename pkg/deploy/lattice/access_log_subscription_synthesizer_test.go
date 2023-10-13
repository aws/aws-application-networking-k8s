package lattice

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/gateway-api/apis/v1alpha2"

	mockclient "github.com/aws/aws-application-networking-k8s/mocks/controller-runtime/client"
	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/gateway"
	"github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

func TestSynthesizeAccessLogSubscription(t *testing.T) {
	setup := func() (
		context.Context,
		*gomock.Controller,
		*MockAccessLogSubscriptionManager,
		*mockclient.MockClient,
		gateway.AccessLogSubscriptionModelBuilder,
	) {
		ctx := context.TODO()
		c := gomock.NewController(t)
		mockManager := NewMockAccessLogSubscriptionManager(c)
		k8sClient := mockclient.NewMockClient(c)
		builder := gateway.NewAccessLogSubscriptionModelBuilder(gwlog.FallbackLogger, k8sClient)
		return ctx, c, mockManager, k8sClient, builder
	}

	t.Run("SpecIsNotDeleted_CreatesAccessLogSubscription", func(t *testing.T) {
		ctx, c, mockManager, k8sClient, builder := setup()
		defer c.Finish()
		input := &anv1alpha1.AccessLogPolicy{
			Spec: anv1alpha1.AccessLogPolicySpec{
				DestinationArn: aws.String(s3DestinationArn),
				TargetRef: &v1alpha2.PolicyTargetReference{
					Kind: "Gateway",
					Name: "TestName",
				},
			},
		}

		stack, accessLogSubscription, _ := builder.Build(context.Background(), input)

		k8sClient.EXPECT().List(context.Background(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
		mockManager.EXPECT().Create(ctx, accessLogSubscription).Return(&lattice.AccessLogSubscriptionStatus{}, nil).AnyTimes()

		synthesizer := NewAccessLogSubscriptionSynthesizer(gwlog.FallbackLogger, k8sClient, mockManager, stack)
		err := synthesizer.Synthesize(ctx)
		assert.Nil(t, err)
	})

	t.Run("SpecIsNotDeletedButErrorOccurs_ReturnsError", func(t *testing.T) {
		ctx, c, mockManager, k8sClient, builder := setup()
		defer c.Finish()
		input := &anv1alpha1.AccessLogPolicy{
			Spec: anv1alpha1.AccessLogPolicySpec{
				DestinationArn: aws.String(s3DestinationArn),
				TargetRef: &v1alpha2.PolicyTargetReference{
					Kind: "Gateway",
					Name: "TestName",
				},
			},
		}

		stack, accessLogSubscription, _ := builder.Build(context.Background(), input)

		k8sClient.EXPECT().List(context.Background(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
		mockManager.EXPECT().Create(ctx, accessLogSubscription).Return(nil, errors.New("mock error")).AnyTimes()

		synthesizer := NewAccessLogSubscriptionSynthesizer(gwlog.FallbackLogger, k8sClient, mockManager, stack)
		err := synthesizer.Synthesize(ctx)
		assert.NotNil(t, err)
	})
}
