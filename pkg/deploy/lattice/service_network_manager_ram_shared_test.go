package lattice

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	mocks "github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

// Test_isLocalServiceNetwork_LocalNetwork tests detection of local service networks
func Test_isLocalServiceNetwork_LocalNetwork(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()

	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)

	// Local network ARN with same account ID as TestCloudConfig
	localArn := aws.String("arn:aws:vpc-lattice:us-west-2:account-id:servicenetwork/sn-12345678")

	isLocal, err := snMgr.isLocalServiceNetwork(localArn)

	assert.Nil(t, err)
	assert.True(t, isLocal, "Network with same account ID should be detected as local")
}

// Test_isLocalServiceNetwork_RAMSharedNetwork tests detection of RAM-shared service networks
func Test_isLocalServiceNetwork_RAMSharedNetwork(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()

	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)

	// RAM-shared network ARN with different account ID
	ramSharedArn := aws.String("arn:aws:vpc-lattice:us-west-2:248189924968:servicenetwork/sn-12345678")

	isLocal, err := snMgr.isLocalServiceNetwork(ramSharedArn)

	assert.Nil(t, err)
	assert.False(t, isLocal, "Network with different account ID should be detected as RAM-shared")
}

// Test_isLocalServiceNetwork_InvalidARN tests handling of invalid ARN format
func Test_isLocalServiceNetwork_InvalidARN(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()

	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)

	// Invalid ARN format
	invalidArn := aws.String("not-a-valid-arn")

	isLocal, err := snMgr.isLocalServiceNetwork(invalidArn)

	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "failed to parse")
	assert.False(t, isLocal, "Invalid ARN should return false for safety")
}

// Test_isLocalServiceNetwork_NilARN tests handling of nil ARN
func Test_isLocalServiceNetwork_NilARN(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()

	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)

	isLocal, err := snMgr.isLocalServiceNetwork(nil)

	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "ARN is nil")
	assert.False(t, isLocal, "Nil ARN should return false for safety")
}

// Test_UpsertVpcAssociation_RAMSharedNetwork_ExistingAssociation tests that RAM-shared networks skip ownership checks
func Test_UpsertVpcAssociation_RAMSharedNetwork_ExistingAssociation(t *testing.T) {
	securityGroupIds := []*string{aws.String("sg-123456789"), aws.String("sg-987654321")}

	// RAM-shared network with different account ID
	snId := "sn-12345678912345678"
	snArn := "arn:aws:vpc-lattice:us-west-2:248189924968:servicenetwork/sn-12345678912345678"
	snvaArn := "arn:aws:vpc-lattice:us-west-2:248189924968:servicenetworkvpcassociation/snva-12345678912345678"
	name := "ram-shared-network"
	vpcId := config.VpcID

	item := vpclattice.ServiceNetworkSummary{
		Arn:  &snArn,
		Id:   &snId,
		Name: &name,
	}

	status := vpclattice.ServiceNetworkVpcAssociationStatusActive
	items := vpclattice.ServiceNetworkVpcAssociationSummary{
		ServiceNetworkArn:  &snArn,
		ServiceNetworkId:   &snId,
		ServiceNetworkName: &snId,
		Status:             &status,
		VpcId:              &vpcId,
		Arn:                &snvaArn,
	}
	statusServiceNetworkVPCOutput := []*vpclattice.ServiceNetworkVpcAssociationSummary{&items}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

	mockLattice.EXPECT().FindServiceNetwork(ctx, gomock.Any()).Return(
		&mocks.ServiceNetworkInfo{
			SvcNetwork: item,
			Tags:       nil,
		}, nil)
	mockLattice.EXPECT().ListServiceNetworkVpcAssociationsAsList(ctx, gomock.Any()).Return(statusServiceNetworkVPCOutput, nil)

	// Critical: For RAM-shared networks, we should NOT call GetServiceNetworkVpcAssociationWithContext,
	// TryOwn, or UpdateServiceNetworkVpcAssociation - they return early
	mockLattice.EXPECT().GetServiceNetworkVpcAssociationWithContext(ctx, gomock.Any()).Times(0)
	mockLattice.EXPECT().UpdateServiceNetworkVpcAssociationWithContext(ctx, gomock.Any()).Times(0)

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)
	resp, err := snMgr.UpsertVpcAssociation(ctx, name, securityGroupIds, nil)

	assert.Nil(t, err)
	assert.Equal(t, snvaArn, resp, "Should return existing VPC association ARN for RAM-shared network")
}

// Test_UpsertVpcAssociation_RAMSharedNetwork_ReadOnly tests that RAM-shared networks are read-only
func Test_UpsertVpcAssociation_RAMSharedNetwork_ReadOnly(t *testing.T) {
	newSecurityGroupIds := []*string{aws.String("sg-999999999")}

	// RAM-shared network with different account ID
	snId := "sn-12345678912345678"
	snArn := "arn:aws:vpc-lattice:us-west-2:248189924968:servicenetwork/sn-12345678912345678"
	snvaArn := "arn:aws:vpc-lattice:us-west-2:248189924968:servicenetworkvpcassociation/snva-12345678912345678"
	name := "ram-shared-network"
	vpcId := config.VpcID

	item := vpclattice.ServiceNetworkSummary{
		Arn:  &snArn,
		Id:   &snId,
		Name: &name,
	}

	status := vpclattice.ServiceNetworkVpcAssociationStatusActive
	items := vpclattice.ServiceNetworkVpcAssociationSummary{
		ServiceNetworkArn:  &snArn,
		ServiceNetworkId:   &snId,
		ServiceNetworkName: &snId,
		Status:             &status,
		VpcId:              &vpcId,
		Arn:                &snvaArn,
	}
	statusServiceNetworkVPCOutput := []*vpclattice.ServiceNetworkVpcAssociationSummary{&items}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

	mockLattice.EXPECT().FindServiceNetwork(ctx, gomock.Any()).Return(
		&mocks.ServiceNetworkInfo{
			SvcNetwork: item,
			Tags:       nil,
		}, nil)
	mockLattice.EXPECT().ListServiceNetworkVpcAssociationsAsList(ctx, gomock.Any()).Return(statusServiceNetworkVPCOutput, nil)

	// Even though security groups are different, RAM-shared networks should NOT be updated
	// and should not even check the existing association
	mockLattice.EXPECT().GetServiceNetworkVpcAssociationWithContext(ctx, gomock.Any()).Times(0)
	mockLattice.EXPECT().UpdateServiceNetworkVpcAssociationWithContext(ctx, gomock.Any()).Times(0)

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)
	resp, err := snMgr.UpsertVpcAssociation(ctx, name, newSecurityGroupIds, nil)

	assert.Nil(t, err)
	assert.Equal(t, snvaArn, resp, "Should return existing VPC association ARN without modifications")
}

// Test_UpsertVpcAssociation_LocalNetwork_WithUpdates tests that local networks CAN be updated
func Test_UpsertVpcAssociation_LocalNetwork_WithUpdates(t *testing.T) {
	existingSecurityGroupIds := []*string{aws.String("sg-111111111")}
	newSecurityGroupIds := []*string{aws.String("sg-222222222"), aws.String("sg-333333333")}

	// Local network with same account ID
	snId := "sn-12345678912345678"
	snArn := "arn:aws:vpc-lattice:us-west-2:account-id:servicenetwork/sn-12345678912345678"
	snvaArn := "arn:aws:vpc-lattice:us-west-2:account-id:servicenetworkvpcassociation/snva-12345678912345678"
	name := "local-network"
	vpcId := config.VpcID

	item := vpclattice.ServiceNetworkSummary{
		Arn:  &snArn,
		Id:   &snId,
		Name: &name,
	}

	status := vpclattice.ServiceNetworkVpcAssociationStatusActive
	items := vpclattice.ServiceNetworkVpcAssociationSummary{
		ServiceNetworkArn:  &snArn,
		ServiceNetworkId:   &snId,
		ServiceNetworkName: &snId,
		Status:             &status,
		VpcId:              &vpcId,
		Arn:                &snvaArn,
	}
	statusServiceNetworkVPCOutput := []*vpclattice.ServiceNetworkVpcAssociationSummary{&items}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	mockTagging := mocks.NewMockTagging(c)
	cloud := pkg_aws.NewDefaultCloudWithTagging(mockLattice, mockTagging, TestCloudConfig)

	mockLattice.EXPECT().FindServiceNetwork(ctx, gomock.Any()).Return(
		&mocks.ServiceNetworkInfo{
			SvcNetwork: item,
			Tags:       nil,
		}, nil)
	mockLattice.EXPECT().GetServiceNetworkVpcAssociationWithContext(ctx, gomock.Any()).Return(&vpclattice.GetServiceNetworkVpcAssociationOutput{
		Arn:                &snvaArn,
		ServiceNetworkArn:  &snArn,
		ServiceNetworkId:   &snId,
		ServiceNetworkName: &name,
		Status:             &status,
		VpcId:              &vpcId,
		SecurityGroupIds:   existingSecurityGroupIds,
	}, nil)
	mockLattice.EXPECT().ListServiceNetworkVpcAssociationsAsList(ctx, gomock.Any()).Return(statusServiceNetworkVPCOutput, nil)
	mockLattice.EXPECT().ListTagsForResourceWithContext(ctx, gomock.Any()).Return(&vpclattice.ListTagsForResourceOutput{
		Tags: cloud.DefaultTags(),
	}, nil)

	mockTagging.EXPECT().UpdateTags(ctx, gomock.Any(), gomock.Any(), nil).Return(nil)

	// Local networks SHOULD be updated when security groups change
	mockLattice.EXPECT().UpdateServiceNetworkVpcAssociationWithContext(ctx, gomock.Any()).Return(&vpclattice.UpdateServiceNetworkVpcAssociationOutput{
		Arn:              &snvaArn,
		Id:               &snId,
		SecurityGroupIds: newSecurityGroupIds,
		Status:           &status,
	}, nil)

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)
	resp, err := snMgr.UpsertVpcAssociation(ctx, name, newSecurityGroupIds, nil)

	assert.Nil(t, err)
	assert.Equal(t, snvaArn, resp, "Should return VPC association ARN after successful update")
}
