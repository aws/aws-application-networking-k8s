package lattice

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/gateway-api/apis/v1alpha2"

	mockclient "github.com/aws/aws-application-networking-k8s/mocks/controller-runtime/client"
	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/gateway"
	"github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

func TestSynthesizeAccessLogSubscription(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockManager := NewMockAccessLogSubscriptionManager(c)
	k8sClient := mockclient.NewMockClient(c)
	builder := gateway.NewAccessLogSubscriptionModelBuilder(gwlog.FallbackLogger, k8sClient)

	t.Run("SpecIsNotDeleted_CreatesAccessLogSubscription", func(t *testing.T) {
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

		mockManager.EXPECT().Create(ctx, accessLogSubscription).Return(&lattice.AccessLogSubscriptionStatus{}, nil).Times(1)

		synthesizer := NewAccessLogSubscriptionSynthesizer(gwlog.FallbackLogger, k8sClient, mockManager, stack)
		err := synthesizer.Synthesize(ctx)
		assert.Nil(t, err)
	})

	t.Run("SpecIsNotDeletedButErrorOccurs_ReturnsError", func(t *testing.T) {
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

		mockManager.EXPECT().Create(ctx, accessLogSubscription).Return(nil, errors.New("mock error")).Times(1)

		synthesizer := NewAccessLogSubscriptionSynthesizer(gwlog.FallbackLogger, k8sClient, mockManager, stack)
		err := synthesizer.Synthesize(ctx)
		assert.NotNil(t, err)
	})

	t.Run("SpecIsDeleted_DeletesAccessLogSubscription", func(t *testing.T) {
		input := &anv1alpha1.AccessLogPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					anv1alpha1.AccessLogSubscriptionAnnotationKey: "arn:aws:vpc-lattice:us-west-2:123456789012:accesslogsubscription/als-12345678901234567",
				},
				DeletionTimestamp: &metav1.Time{},
			},
			Spec: anv1alpha1.AccessLogPolicySpec{
				DestinationArn: aws.String(s3DestinationArn),
				TargetRef: &v1alpha2.PolicyTargetReference{
					Kind: "Gateway",
					Name: "TestName",
				},
			},
		}

		stack, accessLogSubscription, _ := builder.Build(context.Background(), input)

		mockManager.EXPECT().Delete(ctx, accessLogSubscription).Return(nil).Times(1)

		synthesizer := NewAccessLogSubscriptionSynthesizer(gwlog.FallbackLogger, k8sClient, mockManager, stack)
		err := synthesizer.Synthesize(ctx)
		assert.Nil(t, err)
	})

	t.Run("SpecIsDeletedButAnnotationIsMissing_IgnoresAccessLogSubscription", func(t *testing.T) {
		input := &anv1alpha1.AccessLogPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Annotations:       map[string]string{},
				DeletionTimestamp: &metav1.Time{},
			},
			Spec: anv1alpha1.AccessLogPolicySpec{
				DestinationArn: aws.String(s3DestinationArn),
				TargetRef: &v1alpha2.PolicyTargetReference{
					Kind: "Gateway",
					Name: "TestName",
				},
			},
		}

		stack, _, _ := builder.Build(context.Background(), input)

		mockManager.EXPECT().Delete(gomock.Any(), gomock.Any()).Return(nil).Times(0)

		synthesizer := NewAccessLogSubscriptionSynthesizer(gwlog.FallbackLogger, k8sClient, mockManager, stack)
		err := synthesizer.Synthesize(ctx)
		assert.Nil(t, err)
	})

	t.Run("SpecIsDeletedButErrorOccurs_ReturnsError", func(t *testing.T) {
		input := &anv1alpha1.AccessLogPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					anv1alpha1.AccessLogSubscriptionAnnotationKey: "arn:aws:vpc-lattice:us-west-2:123456789012:accesslogsubscription/als-12345678901234567",
				},
				DeletionTimestamp: &metav1.Time{},
			},
			Spec: anv1alpha1.AccessLogPolicySpec{
				DestinationArn: aws.String(s3DestinationArn),
				TargetRef: &v1alpha2.PolicyTargetReference{
					Kind: "Gateway",
					Name: "TestName",
				},
			},
		}

		stack, accessLogSubscription, _ := builder.Build(context.Background(), input)

		mockManager.EXPECT().Delete(ctx, accessLogSubscription).Return(errors.New("mock error")).Times(1)

		synthesizer := NewAccessLogSubscriptionSynthesizer(gwlog.FallbackLogger, k8sClient, mockManager, stack)
		err := synthesizer.Synthesize(ctx)
		assert.NotNil(t, err)
	})
}
