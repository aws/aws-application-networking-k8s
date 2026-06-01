package lattice

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/vpclattice"
	"github.com/aws/aws-sdk-go-v2/service/vpclattice/types"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/aws/aws-application-networking-k8s/pkg/config"
	lattice_runtime "github.com/aws/aws-application-networking-k8s/pkg/runtime"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"

	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	mocks "github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

func Test_CreateOrUpdateServiceNetwork_SnNotExist_NeedToAssociate(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	mockTagging := mocks.NewMockTagging(c)
	cloud := pkg_aws.NewDefaultCloudWithTagging(mockLattice, mockTagging, TestCloudConfig)

	snCreateInput := model.ServiceNetwork{
		Spec: model.ServiceNetworkSpec{
			Name:    "test",
			Account: "123456789",
		},
		Status: &model.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""},
	}
	arn := "12345678912345678912"
	id := "12345678912345678912"
	name := "test"
	snCreateOutput := &vpclattice.CreateServiceNetworkOutput{
		Arn:  &arn,
		Id:   &id,
		Name: &name,
	}

	createServiceNetworkInput := &vpclattice.CreateServiceNetworkInput{
		Name: &name,
		Tags: cloud.DefaultTags(),
	}

	mockLattice.EXPECT().CreateServiceNetwork(ctx, createServiceNetworkInput).Return(snCreateOutput, nil)
	snId := "12345678912345678912"
	createServiceNetworkVpcAssociationInput := &vpclattice.CreateServiceNetworkVpcAssociationInput{
		ServiceNetworkIdentifier: &snId,
		VpcIdentifier:            &config.VpcID,
		Tags:                     cloud.DefaultTags(),
	}
	associationStatus := types.ServiceNetworkVpcAssociationStatusActive
	createServiceNetworkVPCAssociationOutput := &vpclattice.CreateServiceNetworkVpcAssociationOutput{
		Status: associationStatus,
	}
	mockLattice.EXPECT().
		CreateServiceNetworkVpcAssociation(ctx, createServiceNetworkVpcAssociationInput).
		Return(createServiceNetworkVPCAssociationOutput, nil)

	mockLattice.EXPECT().
		FindServiceNetwork(gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)
	resp, err := snMgr.CreateOrUpdate(ctx, &snCreateInput)

	assert.Nil(t, err)
	assert.Equal(t, arn, resp.ServiceNetworkARN)
	assert.Equal(t, id, resp.ServiceNetworkID)
}

// List and find sn does not work.
func Test_CreateOrUpdateServiceNetwork_ListFailed(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	mockTagging := mocks.NewMockTagging(c)
	cloud := pkg_aws.NewDefaultCloudWithTagging(mockLattice, mockTagging, TestCloudConfig)

	snCreateInput := model.ServiceNetwork{
		Spec: model.ServiceNetworkSpec{
			Name:    "test",
			Account: "123456789",
		},
		Status: &model.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""},
	}

	mockLattice.EXPECT().FindServiceNetwork(ctx, gomock.Any()).Return(nil, errors.New("ERROR"))

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)
	resp, err := snMgr.CreateOrUpdate(ctx, &snCreateInput)

	assert.NotNil(t, err)
	assert.Equal(t, resp.ServiceNetworkARN, "")
	assert.Equal(t, resp.ServiceNetworkID, "")
}

// ServiceNetwork already exists, association is in ServiceNetworkVpcAssociationStatusCreateInProgress.

func Test_CreateOrUpdateServiceNetwork_SnAlreadyExist_ServiceNetworkVpcAssociationStatusCreateInProgress(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

	snCreateInput := model.ServiceNetwork{
		Spec: model.ServiceNetworkSpec{
			Name:           "test",
			Account:        "123456789",
			AssociateToVPC: true,
		},
		Status: &model.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""},
	}
	snId := "12345678912345678912"
	snArn := "12345678912345678912"
	name := "test"
	vpcId := config.VpcID
	item := types.ServiceNetworkSummary{
		Arn:  &snArn,
		Id:   &snId,
		Name: &name,
	}

	status := types.ServiceNetworkVpcAssociationStatusCreateInProgress
	items := types.ServiceNetworkVpcAssociationSummary{
		ServiceNetworkArn:  &snArn,
		ServiceNetworkId:   &snId,
		ServiceNetworkName: &snId,
		Status:             status,
		VpcId:              &vpcId,
	}
	statusServiceNetworkVPCOutput := []types.ServiceNetworkVpcAssociationSummary{items}

	mockLattice.EXPECT().ListServiceNetworkVpcAssociationsAsList(ctx, gomock.Any()).Return(statusServiceNetworkVPCOutput, nil)
	mockLattice.EXPECT().FindServiceNetwork(ctx, gomock.Any()).Return(
		&mocks.ServiceNetworkInfo{
			SvcNetwork: item,
			Tags:       nil,
		}, nil)

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)
	resp, err := snMgr.CreateOrUpdate(ctx, &snCreateInput)

	assert.NotNil(t, err)
	var requeueNeededAfter *lattice_runtime.RequeueNeededAfter
	assert.True(t, errors.As(err, &requeueNeededAfter))
	assert.Equal(t, resp.ServiceNetworkARN, "")
	assert.Equal(t, resp.ServiceNetworkID, "")
}

// ServiceNetwork already exists, association is in ServiceNetworkVpcAssociationStatusDeleteInProgress.

func Test_CreateOrUpdateServiceNetwork_SnAlreadyExist_ServiceNetworkVpcAssociationStatusDeleteInProgress(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

	snCreateInput := model.ServiceNetwork{
		Spec: model.ServiceNetworkSpec{
			Name:    "test",
			Account: "123456789",
		},
		Status: &model.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""},
	}
	snId := "12345678912345678912"
	snArn := "12345678912345678912"
	name := "test"
	vpcId := config.VpcID
	item := types.ServiceNetworkSummary{
		Arn:  &snArn,
		Id:   &snId,
		Name: &name,
	}

	status := types.ServiceNetworkVpcAssociationStatusDeleteInProgress
	items := types.ServiceNetworkVpcAssociationSummary{
		ServiceNetworkArn:  &snArn,
		ServiceNetworkId:   &snId,
		ServiceNetworkName: &snId,
		Status:             status,
		VpcId:              &vpcId,
	}
	statusServiceNetworkVPCOutput := []types.ServiceNetworkVpcAssociationSummary{items}

	mockLattice.EXPECT().ListServiceNetworkVpcAssociationsAsList(ctx, gomock.Any()).Return(statusServiceNetworkVPCOutput, nil)
	mockLattice.EXPECT().FindServiceNetwork(ctx, gomock.Any()).Return(
		&mocks.ServiceNetworkInfo{
			SvcNetwork: item,
			Tags:       nil,
		}, nil)

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)
	resp, err := snMgr.CreateOrUpdate(ctx, &snCreateInput)

	assert.NotNil(t, err)
	var requeueNeededAfter *lattice_runtime.RequeueNeededAfter
	assert.True(t, errors.As(err, &requeueNeededAfter))
	assert.Equal(t, resp.ServiceNetworkARN, "")
	assert.Equal(t, resp.ServiceNetworkID, "")
}

// ServiceNetwork already exists, association is ServiceNetworkVpcAssociationStatusActive.
func Test_CreateOrUpdateServiceNetwork_SnAlreadyExist_ServiceNetworkVpcAssociationStatusActive(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

	snCreateInput := model.ServiceNetwork{
		Spec: model.ServiceNetworkSpec{
			Name:           "test",
			Account:        "123456789",
			AssociateToVPC: true,
		},
		Status: &model.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""},
	}
	snId := "12345678912345678912"
	snArn := "12345678912345678912"
	name := "test"
	item := types.ServiceNetworkSummary{
		Arn:  &snArn,
		Id:   &snId,
		Name: &name,
	}

	status := types.ServiceNetworkVpcAssociationStatusActive
	items := types.ServiceNetworkVpcAssociationSummary{
		ServiceNetworkArn:  &snArn,
		ServiceNetworkId:   &snId,
		ServiceNetworkName: &snId,
		Status:             status,
		VpcId:              &config.VpcID,
	}
	statusServiceNetworkVPCOutput := []types.ServiceNetworkVpcAssociationSummary{items}

	mockLattice.EXPECT().FindServiceNetwork(ctx, gomock.Any()).Return(
		&mocks.ServiceNetworkInfo{
			SvcNetwork: item,
			Tags:       nil,
		}, nil)
	mockLattice.EXPECT().ListServiceNetworkVpcAssociationsAsList(ctx, gomock.Any()).Return(statusServiceNetworkVPCOutput, nil)

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)
	resp, err := snMgr.CreateOrUpdate(ctx, &snCreateInput)

	assert.Nil(t, err)
	assert.Equal(t, resp.ServiceNetworkARN, snArn)
	assert.Equal(t, resp.ServiceNetworkID, snId)
}

// ServiceNetwork already exists, association is ServiceNetworkVpcAssociationStatusCreateFailed.
func Test_CreateOrUpdateServiceNetwork_SnAlreadyExist_ServiceNetworkVpcAssociationStatusCreateFailed(t *testing.T) {
	snCreateInput := model.ServiceNetwork{
		Spec: model.ServiceNetworkSpec{
			Name:           "test",
			Account:        "123456789",
			AssociateToVPC: true,
		},
		Status: &model.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""},
	}
	snId := "12345678912345678912"
	snArn := "12345678912345678912"
	name := "test"
	item := types.ServiceNetworkSummary{
		Arn:  &snArn,
		Id:   &snId,
		Name: &name,
	}

	status := types.ServiceNetworkVpcAssociationStatusCreateFailed
	items := types.ServiceNetworkVpcAssociationSummary{
		ServiceNetworkArn:  &snArn,
		ServiceNetworkId:   &snId,
		ServiceNetworkName: &snId,
		Status:             status,
		VpcId:              &config.VpcID,
	}
	statusServiceNetworkVPCOutput := []types.ServiceNetworkVpcAssociationSummary{items}

	associationStatus := types.ServiceNetworkVpcAssociationStatusActive
	createServiceNetworkVPCAssociationOutput := &vpclattice.CreateServiceNetworkVpcAssociationOutput{
		Status: associationStatus,
	}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)
	mockLattice.EXPECT().ListServiceNetworkVpcAssociationsAsList(ctx, gomock.Any()).Return(statusServiceNetworkVPCOutput, nil)
	snTagsOuput := &vpclattice.ListTagsForResourceOutput{
		Tags: make(map[string]string),
	}
	mockLattice.EXPECT().FindServiceNetwork(ctx, gomock.Any()).Return(
		&mocks.ServiceNetworkInfo{
			SvcNetwork: item,
			Tags:       snTagsOuput.Tags,
		}, nil)

	createServiceNetworkVpcAssociationInput := &vpclattice.CreateServiceNetworkVpcAssociationInput{
		ServiceNetworkIdentifier: &snId,
		VpcIdentifier:            &config.VpcID,
		Tags:                     cloud.DefaultTags(),
	}
	mockLattice.EXPECT().CreateServiceNetworkVpcAssociation(ctx, createServiceNetworkVpcAssociationInput).Return(createServiceNetworkVPCAssociationOutput, nil)

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)
	resp, err := snMgr.CreateOrUpdate(ctx, &snCreateInput)

	assert.Nil(t, err)
	assert.Equal(t, resp.ServiceNetworkARN, snArn)
	assert.Equal(t, resp.ServiceNetworkID, snId)
}

// ServiceNetwork already exists, associated with other VPC
func Test_CreateOrUpdateServiceNetwork_SnAlreadyExist_SnAssociatedWithOtherVPC(t *testing.T) {
	snCreateInput := model.ServiceNetwork{
		Spec: model.ServiceNetworkSpec{
			Name:           "test",
			Account:        "123456789",
			AssociateToVPC: true,
		},
		Status: &model.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""},
	}
	snId := "12345678912345678912"
	snArn := "12345678912345678912"
	name := "test"
	vpcId := "123445677"
	item := types.ServiceNetworkSummary{
		Arn:  &snArn,
		Id:   &snId,
		Name: &name,
	}

	status := types.ServiceNetworkVpcAssociationStatusCreateFailed
	items := types.ServiceNetworkVpcAssociationSummary{
		ServiceNetworkArn:  &snArn,
		ServiceNetworkId:   &snId,
		ServiceNetworkName: &snId,
		Status:             status,
		VpcId:              &vpcId,
	}
	statusServiceNetworkVPCOutput := []types.ServiceNetworkVpcAssociationSummary{items}

	associationStatus := types.ServiceNetworkVpcAssociationStatusActive
	createServiceNetworkVPCAssociationOutput := &vpclattice.CreateServiceNetworkVpcAssociationOutput{
		Status: associationStatus,
	}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)
	mockLattice.EXPECT().ListServiceNetworkVpcAssociationsAsList(ctx, gomock.Any()).Return(statusServiceNetworkVPCOutput, nil)
	snTagsOuput := &vpclattice.ListTagsForResourceOutput{
		Tags: make(map[string]string),
	}
	mockLattice.EXPECT().FindServiceNetwork(ctx, gomock.Any()).Return(
		&mocks.ServiceNetworkInfo{
			SvcNetwork: item,
			Tags:       snTagsOuput.Tags,
		}, nil)

	createServiceNetworkVpcAssociationInput := &vpclattice.CreateServiceNetworkVpcAssociationInput{
		ServiceNetworkIdentifier: &snId,
		VpcIdentifier:            &config.VpcID,
		Tags:                     cloud.DefaultTags(),
	}
	mockLattice.EXPECT().CreateServiceNetworkVpcAssociation(ctx, createServiceNetworkVpcAssociationInput).Return(createServiceNetworkVPCAssociationOutput, nil)

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)
	resp, err := snMgr.CreateOrUpdate(ctx, &snCreateInput)

	assert.Nil(t, err)
	assert.Equal(t, resp.ServiceNetworkARN, snArn)
	assert.Equal(t, resp.ServiceNetworkID, snId)
}

// ServiceNetwork does not exists, association returns Error.
func Test_CreateOrUpdateServiceNetwork_SnNotExist_ServiceNetworkVpcAssociationReturnsError(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

	CreateInput := model.ServiceNetwork{
		Spec: model.ServiceNetworkSpec{
			Name:           "test",
			Account:        "123456789",
			AssociateToVPC: true,
		},
		Status: &model.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""},
	}
	snId := "12345678912345678912"
	snArn := "12345678912345678912"
	name := "test"
	createServiceNetworkVPCAssociationOutput := &vpclattice.CreateServiceNetworkVpcAssociationOutput{}
	snCreateOutput := &vpclattice.CreateServiceNetworkOutput{
		Arn:  &snArn,
		Id:   &snId,
		Name: &name,
	}
	snCreateInput := &vpclattice.CreateServiceNetworkInput{
		Name: &name,
		Tags: cloud.DefaultTags(),
	}
	createServiceNetworkVpcAssociationInput := &vpclattice.CreateServiceNetworkVpcAssociationInput{
		ServiceNetworkIdentifier: &snId,
		VpcIdentifier:            &config.VpcID,
		Tags:                     cloud.DefaultTags(),
	}

	mockLattice.EXPECT().FindServiceNetwork(ctx, gomock.Any()).Return(nil, nil)
	mockLattice.EXPECT().CreateServiceNetwork(ctx, snCreateInput).Return(snCreateOutput, nil)
	mockLattice.EXPECT().CreateServiceNetworkVpcAssociation(ctx, createServiceNetworkVpcAssociationInput).Return(createServiceNetworkVPCAssociationOutput, errors.New("ERROR"))

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)
	resp, err := snMgr.CreateOrUpdate(ctx, &CreateInput)

	assert.NotNil(t, err)
	assert.Equal(t, err, errors.New("ERROR"))
	assert.Equal(t, resp.ServiceNetworkARN, "")
	assert.Equal(t, resp.ServiceNetworkID, "")
}

// Sn does not exist and failed to create.
func Test_CreateSn_SnNotExist_SnCreateFailed(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

	CreateInput := model.ServiceNetwork{
		Spec: model.ServiceNetworkSpec{
			Name:    "test",
			Account: "123456789",
		},
		Status: &model.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""},
	}
	arn := "12345678912345678912"
	id := "12345678912345678912"
	name := "test"
	snCreateOutput := &vpclattice.CreateServiceNetworkOutput{
		Arn:  &arn,
		Id:   &id,
		Name: &name,
	}
	snCreateInput := &vpclattice.CreateServiceNetworkInput{
		Name: &name,
		Tags: cloud.DefaultTags(),
	}

	mockLattice.EXPECT().FindServiceNetwork(ctx, gomock.Any()).Return(nil, nil)
	mockLattice.EXPECT().CreateServiceNetwork(ctx, snCreateInput).Return(snCreateOutput, errors.New("ERROR"))

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)
	resp, err := snMgr.CreateOrUpdate(ctx, &CreateInput)

	assert.NotNil(t, err)
	assert.Equal(t, err, errors.New("ERROR"))
	assert.Equal(t, resp.ServiceNetworkARN, "")
	assert.Equal(t, resp.ServiceNetworkID, "")
}

func Test_defaultServiceNetworkManager_CreateOrUpdate_SnExists_SnvaExists_UpdateSNVASecurityGroups(t *testing.T) {
	securityGroupIds := []string{"sg-123456789", "sg-987654321"}

	snId := "sn-12345678912345678"
	snArn := "arn:aws:vpc-lattice:region:account-id:servicenetwork/sn-12345678912345678"
	snvaArn := "arn:aws:vpc-lattice:region:account-id:servicenetworkvpcassociation/snva-12345678912345678"
	name := "test"
	vpcId := config.VpcID
	item := types.ServiceNetworkSummary{
		Arn:  &snArn,
		Id:   &snId,
		Name: &name,
	}

	status := types.ServiceNetworkVpcAssociationStatusActive
	items := types.ServiceNetworkVpcAssociationSummary{
		ServiceNetworkArn:  &snArn,
		ServiceNetworkId:   &snId,
		ServiceNetworkName: &snId,
		Status:             status,
		VpcId:              &config.VpcID,
		Arn:                &snvaArn,
	}
	statusServiceNetworkVPCOutput := []types.ServiceNetworkVpcAssociationSummary{items}

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
	mockLattice.EXPECT().GetServiceNetworkVpcAssociation(ctx, gomock.Any()).Return(&vpclattice.GetServiceNetworkVpcAssociationOutput{
		ServiceNetworkArn:  &snArn,
		ServiceNetworkId:   &snId,
		ServiceNetworkName: &name,
		Status:             types.ServiceNetworkVpcAssociationStatusActive,
		VpcId:              &vpcId,
		//SecurityGroupIds:   []string{"sg-123456789", "sg-987654321"},
	}, nil)
	mockLattice.EXPECT().ListServiceNetworkVpcAssociationsAsList(ctx, gomock.Any()).Return(statusServiceNetworkVPCOutput, nil)
	mockLattice.EXPECT().ListTagsForResource(ctx, gomock.Any()).Return(&vpclattice.ListTagsForResourceOutput{
		Tags: cloud.DefaultTags(),
	}, nil)

	mockTagging.EXPECT().UpdateTags(ctx, gomock.Any(), gomock.Any(), nil).Return(nil)

	mockLattice.EXPECT().CreateServiceNetworkServiceAssociation(ctx, gomock.Any()).MaxTimes(0)
	mockLattice.EXPECT().UpdateServiceNetworkVpcAssociation(ctx, gomock.Any()).Return(&vpclattice.UpdateServiceNetworkVpcAssociationOutput{
		Arn:              &snArn,
		Id:               &snId,
		SecurityGroupIds: securityGroupIds,
		Status:           types.ServiceNetworkVpcAssociationStatusActive,
	}, nil)

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)
	resp, err := snMgr.UpsertVpcAssociation(ctx, name, securityGroupIds, nil)

	assert.Equal(t, err, nil)
	assert.Equal(t, resp, snvaArn)
}

func Test_defaultServiceNetworkManager_CreateOrUpdate_SnExists_SnvaExists_SecurityGroupsDoNotNeedToBeUpdated(t *testing.T) {
	securityGroupIds := []string{"sg-123456789", "sg-987654321"}
	snId := "sn-12345678912345678"
	snArn := "arn:aws:vpc-lattice:region:account-id:servicenetwork/sn-12345678912345678"
	snvaArn := "arn:aws:vpc-lattice:region:account-id:servicenetworkvpcassociation/snva-12345678912345678"
	name := "test"
	vpcId := config.VpcID
	item := types.ServiceNetworkSummary{
		Arn:  &snArn,
		Id:   &snId,
		Name: &name,
	}

	status := types.ServiceNetworkVpcAssociationStatusActive
	items := types.ServiceNetworkVpcAssociationSummary{
		ServiceNetworkArn:  &snArn,
		ServiceNetworkId:   &snId,
		ServiceNetworkName: &snId,
		Status:             status,
		VpcId:              &config.VpcID,
		Arn:                &snvaArn,
	}
	statusServiceNetworkVPCOutput := []types.ServiceNetworkVpcAssociationSummary{items}

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
	mockLattice.EXPECT().GetServiceNetworkVpcAssociation(ctx, gomock.Any()).Return(&vpclattice.GetServiceNetworkVpcAssociationOutput{
		Arn:                &snvaArn,
		ServiceNetworkArn:  &snArn,
		ServiceNetworkId:   &snId,
		ServiceNetworkName: &name,
		Status:             status,
		VpcId:              &vpcId,
		SecurityGroupIds:   securityGroupIds,
	}, nil)
	mockLattice.EXPECT().ListServiceNetworkVpcAssociationsAsList(ctx, gomock.Any()).Return(statusServiceNetworkVPCOutput, nil)
	mockLattice.EXPECT().ListTagsForResource(ctx, gomock.Any()).Return(&vpclattice.ListTagsForResourceOutput{
		Tags: cloud.DefaultTags(),
	}, nil)

	mockTagging.EXPECT().UpdateTags(ctx, snvaArn, nil, nil).Return(nil)

	mockLattice.EXPECT().CreateServiceNetworkServiceAssociation(ctx, gomock.Any()).Times(0)
	mockLattice.EXPECT().UpdateServiceNetworkVpcAssociation(ctx, gomock.Any()).Times(0)

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)
	resp, err := snMgr.UpsertVpcAssociation(ctx, name, securityGroupIds, nil)

	assert.Equal(t, err, nil)
	assert.Equal(t, resp, snvaArn)
}
func Test_defaultServiceNetworkManager_CreateOrUpdate_SnExists_SnvaCreateInProgress_WillNotInvokeLatticeUpdateSNVA(t *testing.T) {
	securityGroupIds := []string{"sg-123456789", "sg-987654321"}
	snId := "12345678912345678912"
	snArn := "12345678912345678912"
	snvaArn := "12345678912345678912"
	name := "test"
	item := types.ServiceNetworkSummary{
		Arn:  &snArn,
		Id:   &snId,
		Name: &name,
	}

	status := types.ServiceNetworkVpcAssociationStatusCreateInProgress
	items := types.ServiceNetworkVpcAssociationSummary{
		ServiceNetworkArn:  &snArn,
		ServiceNetworkId:   &snId,
		ServiceNetworkName: &snId,
		Status:             status,
		VpcId:              &config.VpcID,
		Arn:                &snvaArn,
	}
	statusServiceNetworkVPCOutput := []types.ServiceNetworkVpcAssociationSummary{items}

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
	mockLattice.EXPECT().GetServiceNetworkVpcAssociation(ctx, gomock.Any()).Times(0)
	mockLattice.EXPECT().ListServiceNetworkVpcAssociationsAsList(ctx, gomock.Any()).Return(statusServiceNetworkVPCOutput, nil)
	mockLattice.EXPECT().CreateServiceNetworkServiceAssociation(ctx, gomock.Any()).Times(0)
	mockLattice.EXPECT().UpdateServiceNetworkVpcAssociation(ctx, gomock.Any()).Times(0)

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)
	_, err := snMgr.UpsertVpcAssociation(ctx, name, securityGroupIds, nil)

	var requeueNeededAfter *lattice_runtime.RequeueNeededAfter
	assert.True(t, errors.As(err, &requeueNeededAfter))
}

func Test_defaultServiceNetworkManager_CreateOrUpdate_SnExists_SnvaExists_CannotUpdateSecurityGroupsFromNonemptyToEmpty(t *testing.T) {
	securityGroupIds := []string{"sg-123456789", "sg-987654321"}
	snId := "sn-12345678912345678"
	snArn := "arn:aws:vpc-lattice:region:account-id:servicenetwork/sn-12345678912345678"
	snvaArn := "arn:aws:vpc-lattice:region:account-id:servicenetworkvpcassociation/snva-12345678912345678"
	name := "test"
	item := types.ServiceNetworkSummary{
		Arn:  &snArn,
		Id:   &snId,
		Name: &name,
	}

	status := types.ServiceNetworkVpcAssociationStatusActive
	items := types.ServiceNetworkVpcAssociationSummary{
		ServiceNetworkArn:  &snArn,
		ServiceNetworkId:   &snId,
		ServiceNetworkName: &snId,
		Status:             status,
		VpcId:              &config.VpcID,
		Arn:                &snvaArn,
	}
	statusServiceNetworkVPCOutput := []types.ServiceNetworkVpcAssociationSummary{items}

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
	mockLattice.EXPECT().GetServiceNetworkVpcAssociation(ctx, gomock.Any()).Return(&vpclattice.GetServiceNetworkVpcAssociationOutput{
		Arn:                &snvaArn,
		ServiceNetworkArn:  &snArn,
		ServiceNetworkId:   &snId,
		ServiceNetworkName: &name,
		Status:             types.ServiceNetworkVpcAssociationStatusActive,
		VpcId:              &config.VpcID,
		SecurityGroupIds:   securityGroupIds,
	}, nil)
	mockLattice.EXPECT().ListServiceNetworkVpcAssociationsAsList(ctx, gomock.Any()).Return(statusServiceNetworkVPCOutput, nil)
	mockLattice.EXPECT().ListTagsForResource(ctx, gomock.Any()).Return(&vpclattice.ListTagsForResourceOutput{
		Tags: cloud.DefaultTags(),
	}, nil)

	mockTagging.EXPECT().UpdateTags(ctx, snvaArn, gomock.Any(), nil).Return(nil)

	mockLattice.EXPECT().CreateServiceNetworkServiceAssociation(ctx, gomock.Any()).Times(0)
	updateSNVAError := errors.New("InvalidParameterException SecurityGroupIds cannot be empty")
	mockLattice.EXPECT().UpdateServiceNetworkVpcAssociation(ctx, gomock.Any()).Return(&vpclattice.UpdateServiceNetworkVpcAssociationOutput{}, updateSNVAError)

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)
	_, err := snMgr.UpsertVpcAssociation(ctx, name, []string{}, nil)

	assert.Equal(t, err, updateSNVAError)
}

func Test_UpsertVpcAssociation_WithAdditionalTags_ExistingAssociation(t *testing.T) {
	securityGroupIds := []string{"sg-123456789", "sg-987654321"}
	snId := "sn-12345678912345678"
	snArn := "arn:aws:vpc-lattice:region:account-id:servicenetwork/sn-12345678912345678"
	snvaArn := "arn:aws:vpc-lattice:region:account-id:servicenetworkvpcassociation/snva-12345678912345678"
	name := "test"
	vpcId := config.VpcID
	item := types.ServiceNetworkSummary{
		Arn:  &snArn,
		Id:   &snId,
		Name: &name,
	}

	status := types.ServiceNetworkVpcAssociationStatusActive
	items := types.ServiceNetworkVpcAssociationSummary{
		ServiceNetworkArn:  &snArn,
		ServiceNetworkId:   &snId,
		ServiceNetworkName: &snId,
		Status:             status,
		VpcId:              &config.VpcID,
		Arn:                &snvaArn,
	}
	statusServiceNetworkVPCOutput := []types.ServiceNetworkVpcAssociationSummary{items}

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
	mockLattice.EXPECT().GetServiceNetworkVpcAssociation(ctx, gomock.Any()).Return(&vpclattice.GetServiceNetworkVpcAssociationOutput{
		Arn:                &snvaArn,
		ServiceNetworkArn:  &snArn,
		ServiceNetworkId:   &snId,
		ServiceNetworkName: &name,
		Status:             types.ServiceNetworkVpcAssociationStatusActive,
		VpcId:              &vpcId,
		SecurityGroupIds:   securityGroupIds,
	}, nil)
	mockLattice.EXPECT().ListServiceNetworkVpcAssociationsAsList(ctx, gomock.Any()).Return(statusServiceNetworkVPCOutput, nil)
	mockLattice.EXPECT().ListTagsForResource(ctx, gomock.Any()).Return(&vpclattice.ListTagsForResourceOutput{
		Tags: cloud.DefaultTags(),
	}, nil)

	additionalTags := mocks.Tags{
		"Environment": "Test",
		"Project":     "SNManager",
	}

	mockTagging.EXPECT().UpdateTags(ctx, snvaArn, additionalTags, nil).Return(nil)

	mockLattice.EXPECT().UpdateServiceNetworkVpcAssociation(ctx, gomock.Any()).Times(0)

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)
	resp, err := snMgr.UpsertVpcAssociation(ctx, name, securityGroupIds, additionalTags)

	assert.Equal(t, err, nil)
	assert.Equal(t, resp, snvaArn)
}

func Test_UpsertVpcAssociation_WithAdditionalTags_NoExistingAssociation(t *testing.T) {
	securityGroupIds := []string{"sg-123456789", "sg-987654321"}
	snId := "12345678912345678912"
	snArn := "12345678912345678912"
	name := "test"
	item := types.ServiceNetworkSummary{
		Arn:  &snArn,
		Id:   &snId,
		Name: &name,
	}

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

	mockLattice.EXPECT().ListServiceNetworkVpcAssociationsAsList(ctx, gomock.Any()).Return([]types.ServiceNetworkVpcAssociationSummary{}, nil)

	additionalTags := mocks.Tags{
		"Environment": "Prod",
		"Project":     "CreateTest",
	}

	expectedTags := cloud.MergeTags(cloud.DefaultTags(), additionalTags)

	mockLattice.EXPECT().CreateServiceNetworkVpcAssociation(ctx, gomock.Any()).DoAndReturn(
		func(ctx context.Context, input *vpclattice.CreateServiceNetworkVpcAssociationInput, opts ...interface{}) (*vpclattice.CreateServiceNetworkVpcAssociationOutput, error) {
			assert.Equal(t, expectedTags, input.Tags, "Tags should include both default and additional tags")

			return &vpclattice.CreateServiceNetworkVpcAssociationOutput{
				Arn:    &snArn,
				Status: types.ServiceNetworkVpcAssociationStatusActive,
			}, nil
		})

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)
	resp, err := snMgr.UpsertVpcAssociation(ctx, name, securityGroupIds, additionalTags)

	assert.Equal(t, err, nil)
	assert.Equal(t, resp, snArn)
}

func Test_Upsert_NotFound_Creates(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

	name := "test-sn"
	arn := "arn:aws:vpc-lattice:us-west-2:123456789:servicenetwork/sn-123"
	id := "sn-123"

	mockLattice.EXPECT().FindServiceNetwork(gomock.Any(), name).Return(nil, mocks.NewNotFoundError("ServiceNetwork", name))
	mockLattice.EXPECT().CreateServiceNetwork(gomock.Any(), gomock.Any()).
		Return(&vpclattice.CreateServiceNetworkOutput{
			Arn:  &arn,
			Id:   &id,
			Name: &name,
		}, nil)

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)
	status, err := snMgr.Upsert(ctx, name, nil)

	assert.Nil(t, err)
	assert.Equal(t, arn, status.ServiceNetworkARN)
	assert.Equal(t, id, status.ServiceNetworkID)
}

func Test_Upsert_Exists_Adopts(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	mockTagging := mocks.NewMockTagging(c)
	cloud := pkg_aws.NewDefaultCloudWithTagging(mockLattice, mockTagging, TestCloudConfig)

	name := "test-sn"
	snArn := "arn:aws:vpc-lattice:us-west-2:123456789:servicenetwork/sn-123"
	snId := "sn-123"

	mockLattice.EXPECT().FindServiceNetwork(gomock.Any(), name).
		Return(&mocks.ServiceNetworkInfo{
			SvcNetwork: types.ServiceNetworkSummary{
				Arn:  aws.String(snArn),
				Id:   aws.String(snId),
				Name: aws.String(name),
			},
		}, nil)
	// TryOwn calls getTags → ListTagsForResource; empty tags = no owner = adopt
	mockLattice.EXPECT().ListTagsForResource(gomock.Any(), gomock.Any()).
		Return(&vpclattice.ListTagsForResourceOutput{Tags: map[string]string{}}, nil)
	// TryOwn then calls TagResource to claim ownership
	mockLattice.EXPECT().TagResource(gomock.Any(), gomock.Any()).Return(nil, nil)
	// UpdateTags after adoption
	mockTagging.EXPECT().UpdateTags(gomock.Any(), snArn, gomock.Any(), gomock.Any()).Return(nil)

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)
	status, err := snMgr.Upsert(ctx, name, nil)

	assert.Nil(t, err)
	assert.Equal(t, snArn, status.ServiceNetworkARN)
	assert.Equal(t, snId, status.ServiceNetworkID)
}

func Test_Upsert_FindError(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

	mockLattice.EXPECT().FindServiceNetwork(gomock.Any(), "test-sn").
		Return(nil, errors.New("api error"))

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)
	_, err := snMgr.Upsert(ctx, "test-sn", nil)

	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "api error")
}

func Test_Upsert_CreateError(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

	mockLattice.EXPECT().FindServiceNetwork(gomock.Any(), "test-sn").Return(nil, mocks.NewNotFoundError("ServiceNetwork", "test-sn"))
	mockLattice.EXPECT().CreateServiceNetwork(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("create failed"))

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)
	_, err := snMgr.Upsert(ctx, "test-sn", nil)

	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "create failed")
}

func Test_Upsert_TryOwnFails(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

	snArn := "arn:aws:vpc-lattice:us-west-2:999999999:servicenetwork/sn-other"
	mockLattice.EXPECT().FindServiceNetwork(gomock.Any(), "test-sn").
		Return(&mocks.ServiceNetworkInfo{
			SvcNetwork: types.ServiceNetworkSummary{
				Arn:  aws.String(snArn),
				Id:   aws.String("sn-other"),
				Name: aws.String("test-sn"),
			},
		}, nil)
	// TryOwn calls getTags → ListTagsForResource; returns different owner
	mockLattice.EXPECT().ListTagsForResource(gomock.Any(), gomock.Any()).
		Return(&vpclattice.ListTagsForResourceOutput{
			Tags: map[string]string{
				"application-networking.k8s.aws/ManagedBy": "other-cluster",
			},
		}, nil)

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)
	_, err := snMgr.Upsert(ctx, "test-sn", nil)

	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "owned by another")
}

func Test_Delete_NotFound(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

	mockLattice.EXPECT().FindServiceNetwork(gomock.Any(), "test-sn").
		Return(nil, mocks.NewNotFoundError("ServiceNetwork", "test-sn"))

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)
	err := snMgr.Delete(ctx, "test-sn")

	assert.Nil(t, err)
}

func Test_Delete_NotOwned_Skips(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

	snArn := "arn:aws:vpc-lattice:us-west-2:999999999:servicenetwork/sn-other"
	mockLattice.EXPECT().FindServiceNetwork(gomock.Any(), "test-sn").
		Return(&mocks.ServiceNetworkInfo{
			SvcNetwork: types.ServiceNetworkSummary{
				Arn:  aws.String(snArn),
				Id:   aws.String("sn-other"),
				Name: aws.String("test-sn"),
			},
		}, nil)
	// IsArnManaged calls getTags → ListTagsForResource; returns different owner
	mockLattice.EXPECT().ListTagsForResource(gomock.Any(), gomock.Any()).
		Return(&vpclattice.ListTagsForResourceOutput{
			Tags: map[string]string{
				"application-networking.k8s.aws/ManagedBy": "other-cluster",
			},
		}, nil)

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)
	err := snMgr.Delete(ctx, "test-sn")

	assert.Nil(t, err)
}

func Test_Delete_Owned_Deletes(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

	snArn := "arn:aws:vpc-lattice:us-west-2:account-id:servicenetwork/sn-123"
	mockLattice.EXPECT().FindServiceNetwork(gomock.Any(), "test-sn").
		Return(&mocks.ServiceNetworkInfo{
			SvcNetwork: types.ServiceNetworkSummary{
				Arn:  aws.String(snArn),
				Id:   aws.String("sn-123"),
				Name: aws.String("test-sn"),
			},
		}, nil)
	// IsArnManaged: owned by this controller
	mockLattice.EXPECT().ListTagsForResource(gomock.Any(), gomock.Any()).
		Return(&vpclattice.ListTagsForResourceOutput{
			Tags: cloud.DefaultTags(),
		}, nil)
	mockLattice.EXPECT().DeleteServiceNetwork(gomock.Any(), gomock.Any()).Return(nil, nil)

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)
	err := snMgr.Delete(ctx, "test-sn")

	assert.Nil(t, err)
}

func Test_Delete_ConflictException(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

	snArn := "arn:aws:vpc-lattice:us-west-2:account-id:servicenetwork/sn-123"
	mockLattice.EXPECT().FindServiceNetwork(gomock.Any(), "test-sn").
		Return(&mocks.ServiceNetworkInfo{
			SvcNetwork: types.ServiceNetworkSummary{
				Arn:  aws.String(snArn),
				Id:   aws.String("sn-123"),
				Name: aws.String("test-sn"),
			},
		}, nil)
	mockLattice.EXPECT().ListTagsForResource(gomock.Any(), gomock.Any()).
		Return(&vpclattice.ListTagsForResourceOutput{
			Tags: cloud.DefaultTags(),
		}, nil)
	mockLattice.EXPECT().DeleteServiceNetwork(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("ConflictException: service network has VPC(s) associated"))

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)
	err := snMgr.Delete(ctx, "test-sn")

	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "ConflictException")
}
