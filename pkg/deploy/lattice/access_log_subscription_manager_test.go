package lattice

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/types"

	an_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

const (
	sourceName               = "test"
	serviceNetworkArn        = "arn:aws:vpc-lattice:us-west-2:123456789012:servicenetwork/sn-12345678901234567"
	serviceArn               = "arn:aws:vpc-lattice:us-west-2:123456789012:service/svc-12345678901234567"
	s3DestinationArn         = "arn:aws:s3:::test"
	cloudWatchDestinationArn = "arn:aws:logs:us-west-2:123456789012:log-group:test:*"
	firehoseDestinationArn   = "arn:aws:firehose:us-west-2:123456789012:deliverystream/test"
	accessLogSubscriptionArn = "arn:aws:vpc-lattice:us-west-2:123456789012:accesslogsubscription/als-12345678901234567"
)

var accessLogPolicyNamespacedName = types.NamespacedName{
	Namespace: "test-namespace",
	Name:      "test-name",
}

func simpleAccessLogSubscription(eventType core.EventType) *lattice.AccessLogSubscription {
	return &lattice.AccessLogSubscription{
		Spec: lattice.AccessLogSubscriptionSpec{
			SourceType:        lattice.ServiceNetworkSourceType,
			SourceName:        sourceName,
			DestinationArn:    s3DestinationArn,
			ALPNamespacedName: accessLogPolicyNamespacedName,
			EventType:         eventType,
		},
	}
}

func TestAccessLogSubscriptionManager(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := services.NewMockLattice(c)
	mockTagging := services.NewMockTagging(c)
	cloud := an_aws.NewDefaultCloudWithTagging(mockLattice, mockTagging, TestCloudConfig)
	expectedTags := cloud.DefaultTagsMergedWith(services.Tags{
		lattice.AccessLogPolicyTagKey: aws.String(accessLogPolicyNamespacedName.String()),
	})
	serviceNetworkInfo := &services.ServiceNetworkInfo{
		SvcNetwork: vpclattice.ServiceNetworkSummary{
			Arn:  aws.String(serviceNetworkArn),
			Name: aws.String(sourceName),
		},
	}
	createALSForSNInput := &vpclattice.CreateAccessLogSubscriptionInput{
		ResourceIdentifier: aws.String(serviceNetworkArn),
		DestinationArn:     aws.String(s3DestinationArn),
		Tags:               expectedTags,
	}
	createALSForSvcInput := &vpclattice.CreateAccessLogSubscriptionInput{
		ResourceIdentifier: aws.String(serviceArn),
		DestinationArn:     aws.String(s3DestinationArn),
		Tags:               expectedTags,
	}
	createALSOutput := &vpclattice.CreateAccessLogSubscriptionOutput{
		Arn: aws.String(accessLogSubscriptionArn),
	}
	getALSInput := &vpclattice.GetAccessLogSubscriptionInput{
		AccessLogSubscriptionIdentifier: aws.String(accessLogSubscriptionArn),
	}
	getALSOutput := &vpclattice.GetAccessLogSubscriptionOutput{
		Arn:            aws.String(accessLogSubscriptionArn),
		ResourceArn:    aws.String(serviceNetworkArn),
		DestinationArn: aws.String(s3DestinationArn),
	}
	updateALSInput := &vpclattice.UpdateAccessLogSubscriptionInput{
		AccessLogSubscriptionIdentifier: aws.String(accessLogSubscriptionArn),
		DestinationArn:                  aws.String(s3DestinationArn),
	}
	updateALSOutput := &vpclattice.UpdateAccessLogSubscriptionOutput{
		Arn: aws.String(accessLogSubscriptionArn),
	}
	deleteALSInput := &vpclattice.DeleteAccessLogSubscriptionInput{
		AccessLogSubscriptionIdentifier: aws.String(accessLogSubscriptionArn),
	}
	deleteALSOutput := &vpclattice.DeleteAccessLogSubscriptionOutput{}

	t.Run("Create_NewALSForServiceNetwork_ReturnsNewALSStatus", func(t *testing.T) {
		accessLogSubscription := simpleAccessLogSubscription(core.CreateEvent)

		mockLattice.EXPECT().FindServiceNetwork(ctx, sourceName).Return(serviceNetworkInfo, nil)
		mockLattice.EXPECT().CreateAccessLogSubscriptionWithContext(ctx, createALSForSNInput).Return(createALSOutput, nil)

		mgr := NewAccessLogSubscriptionManager(gwlog.FallbackLogger, cloud)
		resp, err := mgr.Create(ctx, accessLogSubscription)
		assert.Nil(t, err)
		assert.Equal(t, accessLogSubscriptionArn, resp.Arn)
	})

	t.Run("Create_NewALSForService_ReturnsNewALSStatus", func(t *testing.T) {
		accessLogSubscription := simpleAccessLogSubscription(core.CreateEvent)
		accessLogSubscription.Spec.SourceType = lattice.ServiceSourceType
		findServiceOutput := &vpclattice.ServiceSummary{
			Arn:  aws.String(serviceArn),
			Name: aws.String(sourceName),
		}

		mockLattice.EXPECT().FindService(ctx, sourceName).Return(findServiceOutput, nil)
		mockLattice.EXPECT().CreateAccessLogSubscriptionWithContext(ctx, createALSForSvcInput).Return(createALSOutput, nil)

		mgr := NewAccessLogSubscriptionManager(gwlog.FallbackLogger, cloud)
		resp, err := mgr.Create(ctx, accessLogSubscription)
		assert.Nil(t, err)
		assert.Equal(t, accessLogSubscriptionArn, resp.Arn)
	})

	t.Run("Create_NewALSForDeletedServiceNetwork_ReturnsNotFoundError", func(t *testing.T) {
		accessLogSubscription := simpleAccessLogSubscription(core.CreateEvent)
		createALSErr := &vpclattice.ResourceNotFoundException{
			ResourceType: aws.String("SERVICE_NETWORK"),
			ResourceId:   aws.String(serviceNetworkArn),
		}

		mockLattice.EXPECT().FindServiceNetwork(ctx, sourceName).Return(serviceNetworkInfo, nil)
		mockLattice.EXPECT().CreateAccessLogSubscriptionWithContext(ctx, createALSForSNInput).Return(nil, createALSErr)

		mgr := NewAccessLogSubscriptionManager(gwlog.FallbackLogger, cloud)
		resp, err := mgr.Create(ctx, accessLogSubscription)
		assert.Nil(t, resp)
		assert.True(t, services.IsNotFoundError(err))
	})

	t.Run("Create_NewALSForDeletedService_ReturnsNotFoundError", func(t *testing.T) {
		accessLogSubscription := simpleAccessLogSubscription(core.CreateEvent)
		accessLogSubscription.Spec.SourceType = lattice.ServiceSourceType
		findServiceOutput := &vpclattice.ServiceSummary{
			Arn:  aws.String(serviceArn),
			Name: aws.String(sourceName),
		}
		createALSErr := &vpclattice.ResourceNotFoundException{
			ResourceType: aws.String("SERVICE"),
			ResourceId:   aws.String(serviceArn),
		}

		mockLattice.EXPECT().FindService(ctx, sourceName).Return(findServiceOutput, nil)
		mockLattice.EXPECT().CreateAccessLogSubscriptionWithContext(ctx, createALSForSvcInput).Return(nil, createALSErr)

		mgr := NewAccessLogSubscriptionManager(gwlog.FallbackLogger, cloud)
		resp, err := mgr.Create(ctx, accessLogSubscription)
		assert.Nil(t, resp)
		assert.True(t, services.IsNotFoundError(err))
	})

	t.Run("Create_NewALSForMissingS3Destination_ReturnsInvalidError", func(t *testing.T) {
		accessLogSubscription := simpleAccessLogSubscription(core.CreateEvent)
		createALSErr := &vpclattice.ResourceNotFoundException{
			ResourceType: aws.String("BUCKET"),
			ResourceId:   aws.String(s3DestinationArn),
		}

		mockLattice.EXPECT().FindServiceNetwork(ctx, sourceName).Return(serviceNetworkInfo, nil)
		mockLattice.EXPECT().CreateAccessLogSubscriptionWithContext(ctx, createALSForSNInput).Return(nil, createALSErr)

		mgr := NewAccessLogSubscriptionManager(gwlog.FallbackLogger, cloud)
		resp, err := mgr.Create(ctx, accessLogSubscription)
		assert.Nil(t, resp)
		assert.True(t, services.IsInvalidError(err))
	})

	t.Run("Create_NewALSForMissingCloudWatchDestination_ReturnsInvalidError", func(t *testing.T) {
		accessLogSubscription := simpleAccessLogSubscription(core.CreateEvent)
		accessLogSubscription.Spec.DestinationArn = cloudWatchDestinationArn
		createALSInput := &vpclattice.CreateAccessLogSubscriptionInput{
			ResourceIdentifier: aws.String(serviceNetworkArn),
			DestinationArn:     aws.String(cloudWatchDestinationArn),
			Tags:               expectedTags,
		}
		createALSErr := &vpclattice.ResourceNotFoundException{
			ResourceType: aws.String("LOG_GROUP"),
			ResourceId:   aws.String(cloudWatchDestinationArn),
		}

		mockLattice.EXPECT().FindServiceNetwork(ctx, sourceName).Return(serviceNetworkInfo, nil)
		mockLattice.EXPECT().CreateAccessLogSubscriptionWithContext(ctx, createALSInput).Return(nil, createALSErr)

		mgr := NewAccessLogSubscriptionManager(gwlog.FallbackLogger, cloud)
		resp, err := mgr.Create(ctx, accessLogSubscription)
		assert.Nil(t, resp)
		assert.True(t, services.IsInvalidError(err))
	})

	t.Run("Create_NewALSForMissingFirehoseDestination_ReturnsInvalidError", func(t *testing.T) {
		accessLogSubscription := simpleAccessLogSubscription(core.CreateEvent)
		accessLogSubscription.Spec.DestinationArn = firehoseDestinationArn
		createALSInput := &vpclattice.CreateAccessLogSubscriptionInput{
			ResourceIdentifier: aws.String(serviceNetworkArn),
			DestinationArn:     aws.String(firehoseDestinationArn),
			Tags:               expectedTags,
		}
		createALSErr := &vpclattice.ResourceNotFoundException{
			ResourceType: aws.String("DELIVERY_STREAM"),
			ResourceId:   aws.String(firehoseDestinationArn),
		}

		mockLattice.EXPECT().FindServiceNetwork(ctx, sourceName).Return(serviceNetworkInfo, nil)
		mockLattice.EXPECT().CreateAccessLogSubscriptionWithContext(ctx, createALSInput).Return(nil, createALSErr)

		mgr := NewAccessLogSubscriptionManager(gwlog.FallbackLogger, cloud)
		resp, err := mgr.Create(ctx, accessLogSubscription)
		assert.Nil(t, resp)
		assert.True(t, services.IsInvalidError(err))
	})

	t.Run("Create_ConflictingALSForSameResourceFromDifferentPolicy_ReturnsConflictError", func(t *testing.T) {
		accessLogSubscription := simpleAccessLogSubscription(core.CreateEvent)
		createALSErr := &vpclattice.ConflictException{
			ResourceType: aws.String("ACCESS_LOG_SUBSCRIPTION"),
		}
		listALSInput := &vpclattice.ListAccessLogSubscriptionsInput{
			ResourceIdentifier: aws.String(serviceNetworkArn),
		}
		listALSOutput := &vpclattice.ListAccessLogSubscriptionsOutput{
			Items: []*vpclattice.AccessLogSubscriptionSummary{
				{
					Arn:            aws.String(accessLogSubscriptionArn),
					DestinationArn: aws.String(s3DestinationArn),
				},
			},
		}
		listTagsInput := &vpclattice.ListTagsForResourceInput{
			ResourceArn: aws.String(accessLogSubscriptionArn),
		}
		listTagsOutput := &vpclattice.ListTagsForResourceOutput{
			Tags: services.Tags{
				lattice.AccessLogPolicyTagKey: aws.String("other/policy"),
			},
		}

		mockLattice.EXPECT().FindServiceNetwork(ctx, sourceName).Return(serviceNetworkInfo, nil)
		mockLattice.EXPECT().CreateAccessLogSubscriptionWithContext(ctx, createALSForSNInput).Return(nil, createALSErr)
		mockLattice.EXPECT().ListAccessLogSubscriptionsWithContext(ctx, listALSInput).Return(listALSOutput, nil)
		mockLattice.EXPECT().ListTagsForResourceWithContext(ctx, listTagsInput).Return(listTagsOutput, nil)

		mgr := NewAccessLogSubscriptionManager(gwlog.FallbackLogger, cloud)
		resp, err := mgr.Create(ctx, accessLogSubscription)
		assert.Nil(t, resp)
		assert.True(t, services.IsConflictError(err))
	})

	t.Run("Create_ConflictingALSForSameResourceFromSamePolicy_ReturnsNewALSStatus", func(t *testing.T) {
		accessLogSubscription := simpleAccessLogSubscription(core.CreateEvent)
		createALSErr := &vpclattice.ConflictException{
			ResourceType: aws.String("ACCESS_LOG_SUBSCRIPTION"),
		}
		listALSInput := &vpclattice.ListAccessLogSubscriptionsInput{
			ResourceIdentifier: aws.String(serviceNetworkArn),
		}
		listALSOutput := &vpclattice.ListAccessLogSubscriptionsOutput{
			Items: []*vpclattice.AccessLogSubscriptionSummary{
				{
					Arn:            aws.String(accessLogSubscriptionArn),
					DestinationArn: aws.String(s3DestinationArn),
				},
			},
		}
		listTagsInput := &vpclattice.ListTagsForResourceInput{
			ResourceArn: aws.String(accessLogSubscriptionArn),
		}
		listTagsOutput := &vpclattice.ListTagsForResourceOutput{
			Tags: services.Tags{
				lattice.AccessLogPolicyTagKey: aws.String(accessLogPolicyNamespacedName.String()),
			},
		}

		mockLattice.EXPECT().FindServiceNetwork(ctx, sourceName).Return(serviceNetworkInfo, nil)
		mockLattice.EXPECT().CreateAccessLogSubscriptionWithContext(ctx, createALSForSNInput).Return(nil, createALSErr)
		mockLattice.EXPECT().ListAccessLogSubscriptionsWithContext(ctx, listALSInput).Return(listALSOutput, nil)
		mockLattice.EXPECT().ListTagsForResourceWithContext(ctx, listTagsInput).Return(listTagsOutput, nil)

		mgr := NewAccessLogSubscriptionManager(gwlog.FallbackLogger, cloud)
		resp, err := mgr.Create(ctx, accessLogSubscription)
		assert.Nil(t, err)
		assert.Equal(t, accessLogSubscriptionArn, resp.Arn)
	})

	t.Run("Create_NewAccessLogSubscriptionForMissingServiceNetwork_ReturnsNotFoundError", func(t *testing.T) {
		accessLogSubscription := simpleAccessLogSubscription(core.CreateEvent)
		notFoundErr := services.NewNotFoundError("", "")

		mockLattice.EXPECT().FindServiceNetwork(ctx, sourceName).Return(nil, notFoundErr)

		mgr := NewAccessLogSubscriptionManager(gwlog.FallbackLogger, cloud)
		resp, err := mgr.Create(ctx, accessLogSubscription)
		assert.Nil(t, resp)
		assert.True(t, services.IsNotFoundError(err))
	})

	t.Run("Create_NewAccessLogSubscriptionForMissingService_ReturnsNotFoundError", func(t *testing.T) {
		accessLogSubscription := simpleAccessLogSubscription(core.CreateEvent)
		accessLogSubscription.Spec.SourceType = lattice.ServiceSourceType
		notFoundErr := services.NewNotFoundError("", "")

		mockLattice.EXPECT().FindService(ctx, sourceName).Return(nil, notFoundErr)

		mgr := NewAccessLogSubscriptionManager(gwlog.FallbackLogger, cloud)
		resp, err := mgr.Create(ctx, accessLogSubscription)
		assert.Nil(t, resp)
		assert.True(t, services.IsNotFoundError(err))
	})

	t.Run("Update_ALSWithSameDestinationType_UpdatesALSAndReturnsSuccess", func(t *testing.T) {
		accessLogSubscription := simpleAccessLogSubscription(core.UpdateEvent)
		accessLogSubscription.Status = &lattice.AccessLogSubscriptionStatus{
			Arn: accessLogSubscriptionArn,
		}

		mockLattice.EXPECT().GetAccessLogSubscriptionWithContext(ctx, getALSInput).Return(getALSOutput, nil)
		mockLattice.EXPECT().FindServiceNetwork(ctx, sourceName).Return(serviceNetworkInfo, nil)
		mockLattice.EXPECT().UpdateAccessLogSubscriptionWithContext(ctx, updateALSInput).Return(updateALSOutput, nil)

		mockTagging.EXPECT().UpdateTags(ctx, accessLogSubscriptionArn, gomock.Any(), nil).Return(nil)

		mgr := NewAccessLogSubscriptionManager(gwlog.FallbackLogger, cloud)
		resp, err := mgr.Update(ctx, accessLogSubscription)
		assert.Nil(t, err)
		assert.Equal(t, accessLogSubscriptionArn, resp.Arn)
	})

	t.Run("Update_ALSWithDifferentDestinationType_CreatesNewALSThenDeletesOldALSAndReturnsNewALSStatus", func(t *testing.T) {
		newAccessLogSubscriptionArn := accessLogSubscriptionArn + "new"
		accessLogSubscription := simpleAccessLogSubscription(core.UpdateEvent)
		accessLogSubscription.Status = &lattice.AccessLogSubscriptionStatus{
			Arn: accessLogSubscriptionArn,
		}
		updateALSErr := &vpclattice.ConflictException{
			ResourceType: aws.String("ACCESS_LOG_SUBSCRIPTION"),
		}
		createALSOutput := &vpclattice.CreateAccessLogSubscriptionOutput{
			Arn: aws.String(newAccessLogSubscriptionArn),
		}

		mockLattice.EXPECT().GetAccessLogSubscriptionWithContext(ctx, getALSInput).Return(getALSOutput, nil)
		mockLattice.EXPECT().FindServiceNetwork(ctx, sourceName).Return(serviceNetworkInfo, nil)
		mockLattice.EXPECT().UpdateAccessLogSubscriptionWithContext(ctx, updateALSInput).Return(nil, updateALSErr)
		mockLattice.EXPECT().FindServiceNetwork(ctx, sourceName).Return(serviceNetworkInfo, nil)
		mockLattice.EXPECT().CreateAccessLogSubscriptionWithContext(ctx, createALSForSNInput).Return(createALSOutput, nil)
		mockLattice.EXPECT().DeleteAccessLogSubscriptionWithContext(ctx, deleteALSInput).Return(deleteALSOutput, nil)

		mgr := NewAccessLogSubscriptionManager(gwlog.FallbackLogger, cloud)
		resp, err := mgr.Update(ctx, accessLogSubscription)
		assert.Nil(t, err)
		assert.Equal(t, newAccessLogSubscriptionArn, resp.Arn)
	})

	t.Run("Update_ALSWithDifferentSource_CreatesNewALSThenDeletesOldALSAndReturnsNewALSStatus", func(t *testing.T) {
		newAccessLogSubscriptionArn := accessLogSubscriptionArn + "new"
		newSourceArn := serviceNetworkArn + "new"
		newSourceName := sourceName + "new"
		accessLogSubscription := simpleAccessLogSubscription(core.UpdateEvent)
		accessLogSubscription.Spec.SourceName = newSourceName
		accessLogSubscription.Status = &lattice.AccessLogSubscriptionStatus{
			Arn: accessLogSubscriptionArn,
		}
		serviceNetworkInfo := &services.ServiceNetworkInfo{
			SvcNetwork: vpclattice.ServiceNetworkSummary{
				Arn:  aws.String(newSourceArn),
				Name: aws.String(newSourceName),
			},
		}
		createALSInput := &vpclattice.CreateAccessLogSubscriptionInput{
			ResourceIdentifier: aws.String(newSourceArn),
			DestinationArn:     aws.String(s3DestinationArn),
			Tags:               expectedTags,
		}
		createALSOutput := &vpclattice.CreateAccessLogSubscriptionOutput{
			Arn: aws.String(newAccessLogSubscriptionArn),
		}

		mockLattice.EXPECT().GetAccessLogSubscriptionWithContext(ctx, getALSInput).Return(getALSOutput, nil)
		mockLattice.EXPECT().FindServiceNetwork(ctx, newSourceName).Return(serviceNetworkInfo, nil)
		mockLattice.EXPECT().FindServiceNetwork(ctx, newSourceName).Return(serviceNetworkInfo, nil)
		mockLattice.EXPECT().CreateAccessLogSubscriptionWithContext(ctx, createALSInput).Return(createALSOutput, nil)
		mockLattice.EXPECT().DeleteAccessLogSubscriptionWithContext(ctx, deleteALSInput).Return(deleteALSOutput, nil)

		mgr := NewAccessLogSubscriptionManager(gwlog.FallbackLogger, cloud)
		resp, err := mgr.Update(ctx, accessLogSubscription)
		assert.Nil(t, err)
		assert.Equal(t, newAccessLogSubscriptionArn, resp.Arn)
	})

	t.Run("Update_ALSDoesNotExistOnGet_CreatesNewALS", func(t *testing.T) {
		newAccessLogSubscriptionArn := accessLogSubscriptionArn + "new"
		accessLogSubscription := simpleAccessLogSubscription(core.UpdateEvent)
		accessLogSubscription.Status = &lattice.AccessLogSubscriptionStatus{
			Arn: accessLogSubscriptionArn,
		}
		getALSError := &vpclattice.ResourceNotFoundException{
			ResourceType: aws.String("ACCESS_LOG_SUBSCRIPTION"),
		}
		createALSOutput.Arn = aws.String(newAccessLogSubscriptionArn)

		mockLattice.EXPECT().GetAccessLogSubscriptionWithContext(ctx, getALSInput).Return(nil, getALSError)
		mockLattice.EXPECT().FindServiceNetwork(ctx, sourceName).Return(serviceNetworkInfo, nil)
		mockLattice.EXPECT().CreateAccessLogSubscriptionWithContext(ctx, createALSForSNInput).Return(createALSOutput, nil)

		mgr := NewAccessLogSubscriptionManager(gwlog.FallbackLogger, cloud)
		resp, err := mgr.Update(ctx, accessLogSubscription)
		assert.Nil(t, err)
		assert.Equal(t, newAccessLogSubscriptionArn, resp.Arn)
	})

	t.Run("Update_ALSDoesNotExistOnUpdate_CreatesNewALS", func(t *testing.T) {
		newAccessLogSubscriptionArn := accessLogSubscriptionArn + "new"
		accessLogSubscription := simpleAccessLogSubscription(core.UpdateEvent)
		accessLogSubscription.Status = &lattice.AccessLogSubscriptionStatus{
			Arn: accessLogSubscriptionArn,
		}
		updateALSError := &vpclattice.ResourceNotFoundException{
			ResourceType: aws.String("ACCESS_LOG_SUBSCRIPTION"),
		}
		createALSOutput.Arn = aws.String(newAccessLogSubscriptionArn)

		mockLattice.EXPECT().GetAccessLogSubscriptionWithContext(ctx, getALSInput).Return(getALSOutput, nil)
		mockLattice.EXPECT().FindServiceNetwork(ctx, sourceName).Return(serviceNetworkInfo, nil)
		mockLattice.EXPECT().UpdateAccessLogSubscriptionWithContext(ctx, updateALSInput).Return(nil, updateALSError)
		mockLattice.EXPECT().FindServiceNetwork(ctx, sourceName).Return(serviceNetworkInfo, nil)
		mockLattice.EXPECT().CreateAccessLogSubscriptionWithContext(ctx, createALSForSNInput).Return(createALSOutput, nil)

		mgr := NewAccessLogSubscriptionManager(gwlog.FallbackLogger, cloud)
		resp, err := mgr.Update(ctx, accessLogSubscription)
		assert.Nil(t, err)
		assert.Equal(t, newAccessLogSubscriptionArn, resp.Arn)
	})

	t.Run("Update_AccessDeniedExceptionReceivedOnGet_ReturnsInvalidError", func(t *testing.T) {
		accessLogSubscription := simpleAccessLogSubscription(core.UpdateEvent)
		accessLogSubscription.Status = &lattice.AccessLogSubscriptionStatus{
			Arn: accessLogSubscriptionArn,
		}
		getALSError := &vpclattice.AccessDeniedException{}

		mockLattice.EXPECT().GetAccessLogSubscriptionWithContext(ctx, getALSInput).Return(nil, getALSError)

		mgr := NewAccessLogSubscriptionManager(gwlog.FallbackLogger, cloud)
		resp, err := mgr.Update(ctx, accessLogSubscription)
		assert.Nil(t, resp)
		assert.True(t, services.IsInvalidError(err))
	})

	t.Run("Update_AccessDeniedExceptionReceivedOnUpdate_ReturnsInvalidError", func(t *testing.T) {
		accessLogSubscription := simpleAccessLogSubscription(core.UpdateEvent)
		accessLogSubscription.Status = &lattice.AccessLogSubscriptionStatus{
			Arn: accessLogSubscriptionArn,
		}
		updateALSError := &vpclattice.AccessDeniedException{}

		mockLattice.EXPECT().GetAccessLogSubscriptionWithContext(ctx, getALSInput).Return(getALSOutput, nil)
		mockLattice.EXPECT().FindServiceNetwork(ctx, sourceName).Return(serviceNetworkInfo, nil)
		mockLattice.EXPECT().UpdateAccessLogSubscriptionWithContext(ctx, updateALSInput).Return(nil, updateALSError)

		mgr := NewAccessLogSubscriptionManager(gwlog.FallbackLogger, cloud)
		resp, err := mgr.Update(ctx, accessLogSubscription)
		assert.Nil(t, resp)
		assert.True(t, services.IsInvalidError(err))
	})

	t.Run("Update_ServiceNetworkDoesNotExist_ReturnsNotFoundError", func(t *testing.T) {
		accessLogSubscription := simpleAccessLogSubscription(core.UpdateEvent)
		accessLogSubscription.Status = &lattice.AccessLogSubscriptionStatus{
			Arn: accessLogSubscriptionArn,
		}
		updateALSError := &vpclattice.ResourceNotFoundException{
			ResourceType: aws.String("SERVICE_NETWORK"),
		}

		mockLattice.EXPECT().GetAccessLogSubscriptionWithContext(ctx, getALSInput).Return(getALSOutput, nil)
		mockLattice.EXPECT().FindServiceNetwork(ctx, sourceName).Return(serviceNetworkInfo, nil)
		mockLattice.EXPECT().UpdateAccessLogSubscriptionWithContext(ctx, updateALSInput).Return(nil, updateALSError)

		mgr := NewAccessLogSubscriptionManager(gwlog.FallbackLogger, cloud)
		resp, err := mgr.Update(ctx, accessLogSubscription)
		assert.Nil(t, resp)
		assert.True(t, services.IsNotFoundError(err))
	})

	t.Run("Update_ServiceDoesNotExist_ReturnsNotFoundError", func(t *testing.T) {
		accessLogSubscription := simpleAccessLogSubscription(core.UpdateEvent)
		accessLogSubscription.Status = &lattice.AccessLogSubscriptionStatus{
			Arn: accessLogSubscriptionArn,
		}
		updateALSError := &vpclattice.ResourceNotFoundException{
			ResourceType: aws.String("SERVICE"),
		}

		mockLattice.EXPECT().GetAccessLogSubscriptionWithContext(ctx, getALSInput).Return(getALSOutput, nil)
		mockLattice.EXPECT().FindServiceNetwork(ctx, sourceName).Return(serviceNetworkInfo, nil)
		mockLattice.EXPECT().UpdateAccessLogSubscriptionWithContext(ctx, updateALSInput).Return(nil, updateALSError)

		mgr := NewAccessLogSubscriptionManager(gwlog.FallbackLogger, cloud)
		resp, err := mgr.Update(ctx, accessLogSubscription)
		assert.Nil(t, resp)
		assert.True(t, services.IsNotFoundError(err))
	})

	t.Run("Test_Delete_AccessLogSubscriptionExists_ReturnsSuccess", func(t *testing.T) {
		mockLattice.EXPECT().DeleteAccessLogSubscriptionWithContext(ctx, deleteALSInput).Return(deleteALSOutput, nil)

		mgr := NewAccessLogSubscriptionManager(gwlog.FallbackLogger, cloud)
		err := mgr.Delete(ctx, accessLogSubscriptionArn)
		assert.Nil(t, err)
	})

	t.Run("Delete_ALSDoesNotExist_ReturnsSuccess", func(t *testing.T) {
		deleteALSErr := &vpclattice.ResourceNotFoundException{
			ResourceType: aws.String("ACCESS_LOG_SUBSCRIPTION"),
		}

		mockLattice.EXPECT().DeleteAccessLogSubscriptionWithContext(ctx, deleteALSInput).Return(nil, deleteALSErr)

		mgr := NewAccessLogSubscriptionManager(gwlog.FallbackLogger, cloud)
		err := mgr.Delete(ctx, accessLogSubscriptionArn)
		assert.Nil(t, err)
	})
}

func Test_AccessLogSubscriptionManager_WithAdditionalTags_Create(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := services.NewMockLattice(c)
	mockTagging := services.NewMockTagging(c)
	cloud := an_aws.NewDefaultCloudWithTagging(mockLattice, mockTagging, TestCloudConfig)

	accessLogSubscription := &lattice.AccessLogSubscription{
		Spec: lattice.AccessLogSubscriptionSpec{
			SourceType:        lattice.ServiceNetworkSourceType,
			SourceName:        sourceName,
			DestinationArn:    s3DestinationArn,
			ALPNamespacedName: accessLogPolicyNamespacedName,
			EventType:         core.CreateEvent,
			AdditionalTags: services.Tags{
				"Environment": &[]string{"Test"}[0],
				"Project":     &[]string{"ALSManager"}[0],
			},
		},
	}

	serviceNetworkInfo := &services.ServiceNetworkInfo{
		SvcNetwork: vpclattice.ServiceNetworkSummary{
			Arn:  aws.String(serviceNetworkArn),
			Name: aws.String(sourceName),
		},
	}

	baseTags := cloud.DefaultTagsMergedWith(services.Tags{
		lattice.AccessLogPolicyTagKey: aws.String(accessLogPolicyNamespacedName.String()),
	})
	expectedTags := cloud.MergeTags(baseTags, accessLogSubscription.Spec.AdditionalTags)

	mockLattice.EXPECT().FindServiceNetwork(ctx, sourceName).Return(serviceNetworkInfo, nil)
	mockLattice.EXPECT().CreateAccessLogSubscriptionWithContext(ctx, gomock.Any()).DoAndReturn(
		func(ctx context.Context, input *vpclattice.CreateAccessLogSubscriptionInput, opts ...interface{}) (*vpclattice.CreateAccessLogSubscriptionOutput, error) {
			assert.Equal(t, expectedTags, input.Tags, "ALS tags should include additional tags")
			assert.Equal(t, serviceNetworkArn, *input.ResourceIdentifier)
			assert.Equal(t, s3DestinationArn, *input.DestinationArn)

			return &vpclattice.CreateAccessLogSubscriptionOutput{
				Arn: aws.String(accessLogSubscriptionArn),
			}, nil
		})

	mgr := NewAccessLogSubscriptionManager(gwlog.FallbackLogger, cloud)
	resp, err := mgr.Create(ctx, accessLogSubscription)
	assert.Nil(t, err)
	assert.Equal(t, accessLogSubscriptionArn, resp.Arn)
}

func Test_AccessLogSubscriptionManager_WithAdditionalTags_Update(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := services.NewMockLattice(c)
	mockTagging := services.NewMockTagging(c)
	cloud := an_aws.NewDefaultCloudWithTagging(mockLattice, mockTagging, TestCloudConfig)

	accessLogSubscription := &lattice.AccessLogSubscription{
		Spec: lattice.AccessLogSubscriptionSpec{
			SourceType:        lattice.ServiceNetworkSourceType,
			SourceName:        sourceName,
			DestinationArn:    s3DestinationArn,
			ALPNamespacedName: accessLogPolicyNamespacedName,
			EventType:         core.UpdateEvent,
			AdditionalTags: services.Tags{
				"Environment": &[]string{"Prod"}[0],
				"Project":     &[]string{"ALSUpdate"}[0],
			},
		},
		Status: &lattice.AccessLogSubscriptionStatus{
			Arn: accessLogSubscriptionArn,
		},
	}

	serviceNetworkInfo := &services.ServiceNetworkInfo{
		SvcNetwork: vpclattice.ServiceNetworkSummary{
			Arn:  aws.String(serviceNetworkArn),
			Name: aws.String(sourceName),
		},
	}

	getALSInput := &vpclattice.GetAccessLogSubscriptionInput{
		AccessLogSubscriptionIdentifier: aws.String(accessLogSubscriptionArn),
	}
	getALSOutput := &vpclattice.GetAccessLogSubscriptionOutput{
		Arn:            aws.String(accessLogSubscriptionArn),
		ResourceArn:    aws.String(serviceNetworkArn),
		DestinationArn: aws.String(s3DestinationArn),
	}
	updateALSInput := &vpclattice.UpdateAccessLogSubscriptionInput{
		AccessLogSubscriptionIdentifier: aws.String(accessLogSubscriptionArn),
		DestinationArn:                  aws.String(s3DestinationArn),
	}
	updateALSOutput := &vpclattice.UpdateAccessLogSubscriptionOutput{
		Arn: aws.String(accessLogSubscriptionArn),
	}

	mockLattice.EXPECT().GetAccessLogSubscriptionWithContext(ctx, getALSInput).Return(getALSOutput, nil)
	mockLattice.EXPECT().FindServiceNetwork(ctx, sourceName).Return(serviceNetworkInfo, nil)
	mockLattice.EXPECT().UpdateAccessLogSubscriptionWithContext(ctx, updateALSInput).Return(updateALSOutput, nil)

	mockTagging.EXPECT().UpdateTags(ctx, accessLogSubscriptionArn, accessLogSubscription.Spec.AdditionalTags, nil).Return(nil)

	mgr := NewAccessLogSubscriptionManager(gwlog.FallbackLogger, cloud)
	resp, err := mgr.Update(ctx, accessLogSubscription)
	assert.Nil(t, err)
	assert.Equal(t, accessLogSubscriptionArn, resp.Arn)
}
