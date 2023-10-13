package lattice

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	an_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
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
	accessLogSubscriptionId  = "als-12345678901234567"
)

func Test_Create_NewAccessLogSubscriptionForServiceNetwork_ReturnsSuccess(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := services.NewMockLattice(c)
	cloud := an_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

	accessLogSubscription := &lattice.AccessLogSubscription{
		Spec: lattice.AccessLogSubscriptionSpec{
			SourceType:     lattice.ServiceNetworkSourceType,
			SourceName:     sourceName,
			DestinationArn: s3DestinationArn,
			EventType:      core.CreateEvent,
		},
	}
	serviceNetworkInfo := &services.ServiceNetworkInfo{
		SvcNetwork: vpclattice.ServiceNetworkSummary{
			Arn:  aws.String(serviceNetworkArn),
			Name: aws.String(sourceName),
		},
	}
	createALSInput := &vpclattice.CreateAccessLogSubscriptionInput{
		ResourceIdentifier: aws.String(serviceNetworkArn),
		DestinationArn:     aws.String(s3DestinationArn),
		Tags:               cloud.DefaultTags(),
	}
	createALSOutput := &vpclattice.CreateAccessLogSubscriptionOutput{
		Arn: aws.String(accessLogSubscriptionArn),
		Id:  aws.String(accessLogSubscriptionId),
	}

	mockLattice.EXPECT().FindServiceNetwork(ctx, sourceName, config.AccountID).Return(serviceNetworkInfo, nil)
	mockLattice.EXPECT().CreateAccessLogSubscriptionWithContext(ctx, createALSInput).Return(createALSOutput, nil)

	mgr := NewAccessLogSubscriptionManager(gwlog.FallbackLogger, cloud)
	resp, err := mgr.Create(ctx, accessLogSubscription)
	assert.Nil(t, err)
	assert.Equal(t, accessLogSubscriptionArn, *resp.Arn)
}

func Test_Create_NewAccessLogSubscriptionForService_ReturnsSuccess(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := services.NewMockLattice(c)
	cloud := an_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

	accessLogSubscription := &lattice.AccessLogSubscription{
		Spec: lattice.AccessLogSubscriptionSpec{
			SourceType:     lattice.ServiceSourceType,
			SourceName:     sourceName,
			DestinationArn: s3DestinationArn,
			EventType:      core.CreateEvent,
		},
	}
	serviceNameProvider := services.NewDefaultLatticeServiceNameProvider(sourceName)
	findServiceOutput := &vpclattice.ServiceSummary{
		Arn:  aws.String(serviceArn),
		Name: aws.String(sourceName),
	}
	createALSInput := &vpclattice.CreateAccessLogSubscriptionInput{
		ResourceIdentifier: aws.String(serviceArn),
		DestinationArn:     aws.String(s3DestinationArn),
		Tags:               cloud.DefaultTags(),
	}
	createALSOutput := &vpclattice.CreateAccessLogSubscriptionOutput{
		Arn: aws.String(accessLogSubscriptionArn),
		Id:  aws.String(accessLogSubscriptionId),
	}

	mockLattice.EXPECT().FindService(ctx, serviceNameProvider).Return(findServiceOutput, nil)
	mockLattice.EXPECT().CreateAccessLogSubscriptionWithContext(ctx, createALSInput).Return(createALSOutput, nil)

	mgr := NewAccessLogSubscriptionManager(gwlog.FallbackLogger, cloud)
	resp, err := mgr.Create(ctx, accessLogSubscription)
	assert.Nil(t, err)
	assert.Equal(t, accessLogSubscriptionArn, *resp.Arn)
}

func Test_Create_NewAccessLogSubscriptionForDeletedServiceNetwork_ReturnsNotFoundError(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := services.NewMockLattice(c)
	cloud := an_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

	accessLogSubscription := &lattice.AccessLogSubscription{
		Spec: lattice.AccessLogSubscriptionSpec{
			SourceType:     lattice.ServiceNetworkSourceType,
			SourceName:     sourceName,
			DestinationArn: s3DestinationArn,
			EventType:      core.CreateEvent,
		},
	}
	serviceNetworkInfo := &services.ServiceNetworkInfo{
		SvcNetwork: vpclattice.ServiceNetworkSummary{
			Arn:  aws.String(serviceNetworkArn),
			Name: aws.String(sourceName),
		},
	}
	createALSInput := &vpclattice.CreateAccessLogSubscriptionInput{
		ResourceIdentifier: aws.String(serviceNetworkArn),
		DestinationArn:     aws.String(s3DestinationArn),
		Tags:               cloud.DefaultTags(),
	}
	createALSErr := &vpclattice.ResourceNotFoundException{
		ResourceType: aws.String("SERVICE_NETWORK"),
		ResourceId:   aws.String(serviceNetworkArn),
	}

	mockLattice.EXPECT().FindServiceNetwork(ctx, sourceName, config.AccountID).Return(serviceNetworkInfo, nil)
	mockLattice.EXPECT().CreateAccessLogSubscriptionWithContext(ctx, createALSInput).Return(nil, createALSErr)

	mgr := NewAccessLogSubscriptionManager(gwlog.FallbackLogger, cloud)
	resp, err := mgr.Create(ctx, accessLogSubscription)
	assert.Nil(t, resp)
	assert.True(t, services.IsNotFoundError(err))
}

func Test_Create_NewAccessLogSubscriptionForDeletedService_ReturnsNotFoundError(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := services.NewMockLattice(c)
	cloud := an_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

	accessLogSubscription := &lattice.AccessLogSubscription{
		Spec: lattice.AccessLogSubscriptionSpec{
			SourceType:     lattice.ServiceSourceType,
			SourceName:     sourceName,
			DestinationArn: s3DestinationArn,
			EventType:      core.CreateEvent,
		},
	}
	serviceNameProvider := services.NewDefaultLatticeServiceNameProvider(sourceName)
	findServiceOutput := &vpclattice.ServiceSummary{
		Arn:  aws.String(serviceArn),
		Name: aws.String(sourceName),
	}
	createALSInput := &vpclattice.CreateAccessLogSubscriptionInput{
		ResourceIdentifier: aws.String(serviceArn),
		DestinationArn:     aws.String(s3DestinationArn),
		Tags:               cloud.DefaultTags(),
	}
	createALSErr := &vpclattice.ResourceNotFoundException{
		ResourceType: aws.String("SERVICE"),
		ResourceId:   aws.String(serviceArn),
	}

	mockLattice.EXPECT().FindService(ctx, serviceNameProvider).Return(findServiceOutput, nil)
	mockLattice.EXPECT().CreateAccessLogSubscriptionWithContext(ctx, createALSInput).Return(nil, createALSErr)

	mgr := NewAccessLogSubscriptionManager(gwlog.FallbackLogger, cloud)
	resp, err := mgr.Create(ctx, accessLogSubscription)
	assert.Nil(t, resp)
	assert.True(t, services.IsNotFoundError(err))
}

func Test_Create_NewAccessLogSubscriptionForMissingS3Destination_ReturnsInvalidError(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := services.NewMockLattice(c)
	cloud := an_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

	accessLogSubscription := &lattice.AccessLogSubscription{
		Spec: lattice.AccessLogSubscriptionSpec{
			SourceType:     lattice.ServiceNetworkSourceType,
			SourceName:     sourceName,
			DestinationArn: s3DestinationArn,
			EventType:      core.CreateEvent,
		},
	}
	serviceNetworkInfo := &services.ServiceNetworkInfo{
		SvcNetwork: vpclattice.ServiceNetworkSummary{
			Arn:  aws.String(serviceNetworkArn),
			Name: aws.String(sourceName),
		},
	}
	createALSInput := &vpclattice.CreateAccessLogSubscriptionInput{
		ResourceIdentifier: aws.String(serviceNetworkArn),
		DestinationArn:     aws.String(s3DestinationArn),
		Tags:               cloud.DefaultTags(),
	}
	createALSErr := &vpclattice.ResourceNotFoundException{
		ResourceType: aws.String("BUCKET"),
		ResourceId:   aws.String(s3DestinationArn),
	}

	mockLattice.EXPECT().FindServiceNetwork(ctx, sourceName, config.AccountID).Return(serviceNetworkInfo, nil)
	mockLattice.EXPECT().CreateAccessLogSubscriptionWithContext(ctx, createALSInput).Return(nil, createALSErr)

	mgr := NewAccessLogSubscriptionManager(gwlog.FallbackLogger, cloud)
	resp, err := mgr.Create(ctx, accessLogSubscription)
	assert.Nil(t, resp)
	assert.True(t, services.IsInvalidError(err))
}

func Test_Create_NewAccessLogSubscriptionForMissingCloudWatchDestination_ReturnsInvalidError(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := services.NewMockLattice(c)
	cloud := an_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

	accessLogSubscription := &lattice.AccessLogSubscription{
		Spec: lattice.AccessLogSubscriptionSpec{
			SourceType:     lattice.ServiceNetworkSourceType,
			SourceName:     sourceName,
			DestinationArn: cloudWatchDestinationArn,
			EventType:      core.CreateEvent,
		},
	}
	serviceNetworkInfo := &services.ServiceNetworkInfo{
		SvcNetwork: vpclattice.ServiceNetworkSummary{
			Arn:  aws.String(serviceNetworkArn),
			Name: aws.String(sourceName),
		},
	}
	createALSInput := &vpclattice.CreateAccessLogSubscriptionInput{
		ResourceIdentifier: aws.String(serviceNetworkArn),
		DestinationArn:     aws.String(cloudWatchDestinationArn),
		Tags:               cloud.DefaultTags(),
	}
	createALSErr := &vpclattice.ResourceNotFoundException{
		ResourceType: aws.String("LOG_GROUP"),
		ResourceId:   aws.String(cloudWatchDestinationArn),
	}

	mockLattice.EXPECT().FindServiceNetwork(ctx, sourceName, config.AccountID).Return(serviceNetworkInfo, nil)
	mockLattice.EXPECT().CreateAccessLogSubscriptionWithContext(ctx, createALSInput).Return(nil, createALSErr)

	mgr := NewAccessLogSubscriptionManager(gwlog.FallbackLogger, cloud)
	resp, err := mgr.Create(ctx, accessLogSubscription)
	assert.Nil(t, resp)
	assert.True(t, services.IsInvalidError(err))
}

func Test_Create_NewAccessLogSubscriptionForMissingFirehoseDestination_ReturnsInvalidError(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := services.NewMockLattice(c)
	cloud := an_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

	accessLogSubscription := &lattice.AccessLogSubscription{
		Spec: lattice.AccessLogSubscriptionSpec{
			SourceType:     lattice.ServiceNetworkSourceType,
			SourceName:     sourceName,
			DestinationArn: firehoseDestinationArn,
			EventType:      core.CreateEvent,
		},
	}
	serviceNetworkInfo := &services.ServiceNetworkInfo{
		SvcNetwork: vpclattice.ServiceNetworkSummary{
			Arn:  aws.String(serviceNetworkArn),
			Name: aws.String(sourceName),
		},
	}
	createALSInput := &vpclattice.CreateAccessLogSubscriptionInput{
		ResourceIdentifier: aws.String(serviceNetworkArn),
		DestinationArn:     aws.String(firehoseDestinationArn),
		Tags:               cloud.DefaultTags(),
	}
	createALSErr := &vpclattice.ResourceNotFoundException{
		ResourceType: aws.String("DELIVERY_STREAM"),
		ResourceId:   aws.String(firehoseDestinationArn),
	}

	mockLattice.EXPECT().FindServiceNetwork(ctx, sourceName, config.AccountID).Return(serviceNetworkInfo, nil)
	mockLattice.EXPECT().CreateAccessLogSubscriptionWithContext(ctx, createALSInput).Return(nil, createALSErr)

	mgr := NewAccessLogSubscriptionManager(gwlog.FallbackLogger, cloud)
	resp, err := mgr.Create(ctx, accessLogSubscription)
	assert.Nil(t, resp)
	assert.True(t, services.IsInvalidError(err))
}

func Test_Create_ConflictingAccessLogSubscriptionForSameResource_ReturnsConflictError(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := services.NewMockLattice(c)
	cloud := an_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

	accessLogSubscription := &lattice.AccessLogSubscription{
		Spec: lattice.AccessLogSubscriptionSpec{
			SourceType:     lattice.ServiceNetworkSourceType,
			SourceName:     sourceName,
			DestinationArn: s3DestinationArn,
			EventType:      core.CreateEvent,
		},
	}
	serviceNetworkInfo := &services.ServiceNetworkInfo{
		SvcNetwork: vpclattice.ServiceNetworkSummary{
			Arn:  aws.String(serviceNetworkArn),
			Name: aws.String(sourceName),
		},
	}
	createALSInput := &vpclattice.CreateAccessLogSubscriptionInput{
		ResourceIdentifier: aws.String(serviceNetworkArn),
		DestinationArn:     aws.String(s3DestinationArn),
		Tags:               cloud.DefaultTags(),
	}
	createALSErr := &vpclattice.ConflictException{
		ResourceType: aws.String("ACCESS_LOG_SUBSCRIPTION"),
	}

	mockLattice.EXPECT().FindServiceNetwork(ctx, sourceName, config.AccountID).Return(serviceNetworkInfo, nil)
	mockLattice.EXPECT().CreateAccessLogSubscriptionWithContext(ctx, createALSInput).Return(nil, createALSErr)

	mgr := NewAccessLogSubscriptionManager(gwlog.FallbackLogger, cloud)
	resp, err := mgr.Create(ctx, accessLogSubscription)
	assert.Nil(t, resp)
	assert.True(t, services.IsConflictError(err))
}

func Test_Create_NewAccessLogSubscriptionForMissingServiceNetwork_ReturnsNotFoundError(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := services.NewMockLattice(c)
	cloud := an_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

	accessLogSubscription := &lattice.AccessLogSubscription{
		Spec: lattice.AccessLogSubscriptionSpec{
			SourceType:     lattice.ServiceNetworkSourceType,
			SourceName:     sourceName,
			DestinationArn: s3DestinationArn,
			EventType:      core.CreateEvent,
		},
	}
	notFoundErr := services.NewNotFoundError("", "")

	mockLattice.EXPECT().FindServiceNetwork(ctx, sourceName, config.AccountID).Return(nil, notFoundErr)

	mgr := NewAccessLogSubscriptionManager(gwlog.FallbackLogger, cloud)
	resp, err := mgr.Create(ctx, accessLogSubscription)
	assert.Nil(t, resp)
	assert.True(t, services.IsNotFoundError(err))
}

func Test_Create_NewAccessLogSubscriptionForMissingService_ReturnsNotFoundError(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := services.NewMockLattice(c)
	cloud := an_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

	accessLogSubscription := &lattice.AccessLogSubscription{
		Spec: lattice.AccessLogSubscriptionSpec{
			SourceType:     lattice.ServiceSourceType,
			SourceName:     sourceName,
			DestinationArn: s3DestinationArn,
			EventType:      core.CreateEvent,
		},
	}
	notFoundErr := services.NewNotFoundError("", "")
	serviceNameProvider := services.NewDefaultLatticeServiceNameProvider(sourceName)

	mockLattice.EXPECT().FindService(ctx, serviceNameProvider).Return(nil, notFoundErr)

	mgr := NewAccessLogSubscriptionManager(gwlog.FallbackLogger, cloud)
	resp, err := mgr.Create(ctx, accessLogSubscription)
	assert.Nil(t, resp)
	assert.True(t, services.IsNotFoundError(err))
}

func Test_Delete_AccessLogSubscriptionExists_ReturnsSuccess(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockCloud := an_aws.NewMockCloud(c)
	mockLattice := services.NewMockLattice(c)

	accessLogSubscription := &lattice.AccessLogSubscription{
		Spec: lattice.AccessLogSubscriptionSpec{
			SourceType:     lattice.ServiceNetworkSourceType,
			SourceName:     sourceName,
			DestinationArn: s3DestinationArn,
			EventType:      core.DeleteEvent,
		},
		Status: &lattice.AccessLogSubscriptionStatus{
			Arn: aws.String(accessLogSubscriptionArn),
		},
	}
	deleteALSInput := &vpclattice.DeleteAccessLogSubscriptionInput{
		AccessLogSubscriptionIdentifier: aws.String(accessLogSubscriptionArn),
	}
	deleteALSOutput := &vpclattice.DeleteAccessLogSubscriptionOutput{}

	mockLattice.EXPECT().DeleteAccessLogSubscriptionWithContext(ctx, deleteALSInput).Return(deleteALSOutput, nil)
	mockCloud.EXPECT().Lattice().Return(mockLattice).AnyTimes()

	mgr := NewAccessLogSubscriptionManager(gwlog.FallbackLogger, mockCloud)
	err := mgr.Delete(ctx, accessLogSubscription)
	assert.Nil(t, err)
}

func Test_Delete_AccessLogSubscriptionDoesNotExist_ReturnsSuccess(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockCloud := an_aws.NewMockCloud(c)
	mockLattice := services.NewMockLattice(c)

	accessLogSubscription := &lattice.AccessLogSubscription{
		Spec: lattice.AccessLogSubscriptionSpec{
			SourceType:     lattice.ServiceNetworkSourceType,
			SourceName:     sourceName,
			DestinationArn: s3DestinationArn,
			EventType:      core.DeleteEvent,
		},
		Status: &lattice.AccessLogSubscriptionStatus{
			Arn: aws.String(accessLogSubscriptionArn),
		},
	}
	deleteALSInput := &vpclattice.DeleteAccessLogSubscriptionInput{
		AccessLogSubscriptionIdentifier: aws.String(accessLogSubscriptionArn),
	}
	deleteALSErr := &vpclattice.ResourceNotFoundException{
		ResourceType: aws.String("ACCESS_LOG_SUBSCRIPTION"),
	}

	mockLattice.EXPECT().DeleteAccessLogSubscriptionWithContext(ctx, deleteALSInput).Return(nil, deleteALSErr)
	mockCloud.EXPECT().Lattice().Return(mockLattice).AnyTimes()

	mgr := NewAccessLogSubscriptionManager(gwlog.FallbackLogger, mockCloud)
	err := mgr.Delete(ctx, accessLogSubscription)
	assert.Nil(t, err)
}
