package lattice

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go/service/vpclattice"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"

	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	mocks "github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-sdk-go/aws"
)

func Test_CreateOrUpdateServiceNetwork_SnNotExist_NeedToAssociate(t *testing.T) {
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

	mockLattice.EXPECT().CreateServiceNetworkWithContext(ctx, createServiceNetworkInput).Return(snCreateOutput, nil)
	snId := "12345678912345678912"
	createServiceNetworkVpcAssociationInput := &vpclattice.CreateServiceNetworkVpcAssociationInput{
		ServiceNetworkIdentifier: &snId,
		VpcIdentifier:            &config.VpcID,
		Tags:                     cloud.DefaultTags(),
	}
	associationStatus := vpclattice.ServiceNetworkVpcAssociationStatusActive
	createServiceNetworkVPCAssociationOutput := &vpclattice.CreateServiceNetworkVpcAssociationOutput{
		Status: &associationStatus,
	}
	mockLattice.EXPECT().
		CreateServiceNetworkVpcAssociationWithContext(ctx, createServiceNetworkVpcAssociationInput).
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
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

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
	item := vpclattice.ServiceNetworkSummary{
		Arn:  &snArn,
		Id:   &snId,
		Name: &name,
	}

	status := vpclattice.ServiceNetworkVpcAssociationStatusCreateInProgress
	items := vpclattice.ServiceNetworkVpcAssociationSummary{
		ServiceNetworkArn:  &snArn,
		ServiceNetworkId:   &snId,
		ServiceNetworkName: &snId,
		Status:             &status,
		VpcId:              &vpcId,
	}
	statusServiceNetworkVPCOutput := []*vpclattice.ServiceNetworkVpcAssociationSummary{&items}

	mockLattice.EXPECT().ListServiceNetworkVpcAssociationsAsList(ctx, gomock.Any()).Return(statusServiceNetworkVPCOutput, nil)
	mockLattice.EXPECT().FindServiceNetwork(ctx, gomock.Any()).Return(
		&mocks.ServiceNetworkInfo{
			SvcNetwork: item,
			Tags:       nil,
		}, nil)

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)
	resp, err := snMgr.CreateOrUpdate(ctx, &snCreateInput)

	assert.NotNil(t, err)
	assert.Equal(t, err, errors.New(LATTICE_RETRY))
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
	item := vpclattice.ServiceNetworkSummary{
		Arn:  &snArn,
		Id:   &snId,
		Name: &name,
	}

	status := vpclattice.ServiceNetworkVpcAssociationStatusDeleteInProgress
	items := vpclattice.ServiceNetworkVpcAssociationSummary{
		ServiceNetworkArn:  &snArn,
		ServiceNetworkId:   &snId,
		ServiceNetworkName: &snId,
		Status:             &status,
		VpcId:              &vpcId,
	}
	statusServiceNetworkVPCOutput := []*vpclattice.ServiceNetworkVpcAssociationSummary{&items}

	mockLattice.EXPECT().ListServiceNetworkVpcAssociationsAsList(ctx, gomock.Any()).Return(statusServiceNetworkVPCOutput, nil)
	mockLattice.EXPECT().FindServiceNetwork(ctx, gomock.Any()).Return(
		&mocks.ServiceNetworkInfo{
			SvcNetwork: item,
			Tags:       nil,
		}, nil)

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)
	resp, err := snMgr.CreateOrUpdate(ctx, &snCreateInput)

	assert.NotNil(t, err)
	assert.Equal(t, err, errors.New(LATTICE_RETRY))
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
		VpcId:              &config.VpcID,
	}
	statusServiceNetworkVPCOutput := []*vpclattice.ServiceNetworkVpcAssociationSummary{&items}

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
	item := vpclattice.ServiceNetworkSummary{
		Arn:  &snArn,
		Id:   &snId,
		Name: &name,
	}

	status := vpclattice.ServiceNetworkVpcAssociationStatusCreateFailed
	items := vpclattice.ServiceNetworkVpcAssociationSummary{
		ServiceNetworkArn:  &snArn,
		ServiceNetworkId:   &snId,
		ServiceNetworkName: &snId,
		Status:             &status,
		VpcId:              &config.VpcID,
	}
	statusServiceNetworkVPCOutput := []*vpclattice.ServiceNetworkVpcAssociationSummary{&items}

	associationStatus := vpclattice.ServiceNetworkVpcAssociationStatusActive
	createServiceNetworkVPCAssociationOutput := &vpclattice.CreateServiceNetworkVpcAssociationOutput{
		Status: &associationStatus,
	}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)
	mockLattice.EXPECT().ListServiceNetworkVpcAssociationsAsList(ctx, gomock.Any()).Return(statusServiceNetworkVPCOutput, nil)
	snTagsOuput := &vpclattice.ListTagsForResourceOutput{
		Tags: make(map[string]*string),
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
	mockLattice.EXPECT().CreateServiceNetworkVpcAssociationWithContext(ctx, createServiceNetworkVpcAssociationInput).Return(createServiceNetworkVPCAssociationOutput, nil)

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
	item := vpclattice.ServiceNetworkSummary{
		Arn:  &snArn,
		Id:   &snId,
		Name: &name,
	}

	status := vpclattice.ServiceNetworkVpcAssociationStatusCreateFailed
	items := vpclattice.ServiceNetworkVpcAssociationSummary{
		ServiceNetworkArn:  &snArn,
		ServiceNetworkId:   &snId,
		ServiceNetworkName: &snId,
		Status:             &status,
		VpcId:              &vpcId,
	}
	statusServiceNetworkVPCOutput := []*vpclattice.ServiceNetworkVpcAssociationSummary{&items}

	associationStatus := vpclattice.ServiceNetworkVpcAssociationStatusActive
	createServiceNetworkVPCAssociationOutput := &vpclattice.CreateServiceNetworkVpcAssociationOutput{
		Status: &associationStatus,
	}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)
	mockLattice.EXPECT().ListServiceNetworkVpcAssociationsAsList(ctx, gomock.Any()).Return(statusServiceNetworkVPCOutput, nil)
	snTagsOuput := &vpclattice.ListTagsForResourceOutput{
		Tags: make(map[string]*string),
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
	mockLattice.EXPECT().CreateServiceNetworkVpcAssociationWithContext(ctx, createServiceNetworkVpcAssociationInput).Return(createServiceNetworkVPCAssociationOutput, nil)

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
	mockLattice.EXPECT().CreateServiceNetworkWithContext(ctx, snCreateInput).Return(snCreateOutput, nil)
	mockLattice.EXPECT().CreateServiceNetworkVpcAssociationWithContext(ctx, createServiceNetworkVpcAssociationInput).Return(createServiceNetworkVPCAssociationOutput, errors.New("ERROR"))

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
	mockLattice.EXPECT().CreateServiceNetworkWithContext(ctx, snCreateInput).Return(snCreateOutput, errors.New("ERROR"))

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)
	resp, err := snMgr.CreateOrUpdate(ctx, &CreateInput)

	assert.NotNil(t, err)
	assert.Equal(t, err, errors.New("ERROR"))
	assert.Equal(t, resp.ServiceNetworkARN, "")
	assert.Equal(t, resp.ServiceNetworkID, "")
}

func Test_defaultServiceNetworkManager_CreateOrUpdate_SnExists_SnvaExists_UpdateSNVASecurityGroups(t *testing.T) {
	securityGroupIds := []*string{aws.String("sg-123456789"), aws.String("sg-987654321")}

	snId := "12345678912345678912"
	snArn := "12345678912345678912"
	snvaArn := "12345678912345678912"
	name := "test"
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
		VpcId:              &config.VpcID,
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
	mockLattice.EXPECT().GetServiceNetworkVpcAssociationWithContext(ctx, gomock.Any()).Return(&vpclattice.GetServiceNetworkVpcAssociationOutput{
		ServiceNetworkArn:  &snArn,
		ServiceNetworkId:   &snId,
		ServiceNetworkName: &name,
		Status:             aws.String(vpclattice.ServiceNetworkVpcAssociationStatusActive),
		VpcId:              &vpcId,
		//SecurityGroupIds:   []*string{aws.String("sg-123456789"), aws.String("sg-987654321")},
	}, nil)
	mockLattice.EXPECT().ListServiceNetworkVpcAssociationsAsList(ctx, gomock.Any()).Return(statusServiceNetworkVPCOutput, nil)
	mockLattice.EXPECT().ListTagsForResourceWithContext(ctx, gomock.Any()).Return(&vpclattice.ListTagsForResourceOutput{
		Tags: cloud.DefaultTags(),
	}, nil)
	mockLattice.EXPECT().CreateServiceNetworkServiceAssociationWithContext(ctx, gomock.Any()).MaxTimes(0)
	mockLattice.EXPECT().UpdateServiceNetworkVpcAssociationWithContext(ctx, gomock.Any()).Return(&vpclattice.UpdateServiceNetworkVpcAssociationOutput{
		Arn:              &snArn,
		Id:               &snId,
		SecurityGroupIds: securityGroupIds,
		Status:           aws.String(vpclattice.ServiceNetworkVpcAssociationStatusActive),
	}, nil)

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)
	resp, err := snMgr.UpsertVpcAssociation(ctx, name, securityGroupIds)

	assert.Equal(t, err, nil)
	assert.Equal(t, resp, snArn)
}

func Test_defaultServiceNetworkManager_CreateOrUpdate_SnExists_SnvaExists_SecurityGroupsDoNotNeedToBeUpdated(t *testing.T) {
	securityGroupIds := []*string{aws.String("sg-123456789"), aws.String("sg-987654321")}
	snId := "12345678912345678912"
	snArn := "12345678912345678912"
	snvaArn := "12345678912345678912"
	name := "test"
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
		VpcId:              &config.VpcID,
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
	mockLattice.EXPECT().GetServiceNetworkVpcAssociationWithContext(ctx, gomock.Any()).Return(&vpclattice.GetServiceNetworkVpcAssociationOutput{
		ServiceNetworkArn:  &snArn,
		ServiceNetworkId:   &snId,
		ServiceNetworkName: &name,
		Status:             &status,
		VpcId:              &vpcId,
		SecurityGroupIds:   securityGroupIds,
	}, nil)
	mockLattice.EXPECT().ListServiceNetworkVpcAssociationsAsList(ctx, gomock.Any()).Return(statusServiceNetworkVPCOutput, nil)
	mockLattice.EXPECT().ListTagsForResourceWithContext(ctx, gomock.Any()).Return(&vpclattice.ListTagsForResourceOutput{
		Tags: cloud.DefaultTags(),
	}, nil)
	mockLattice.EXPECT().CreateServiceNetworkServiceAssociationWithContext(ctx, gomock.Any()).Times(0)
	mockLattice.EXPECT().UpdateServiceNetworkVpcAssociationWithContext(ctx, gomock.Any()).Times(0)

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)
	resp, err := snMgr.UpsertVpcAssociation(ctx, name, securityGroupIds)

	assert.Equal(t, err, nil)
	assert.Equal(t, resp, snArn)
}
func Test_defaultServiceNetworkManager_CreateOrUpdate_SnExists_SnvaCreateInProgress_WillNotInvokeLatticeUpdateSNVA(t *testing.T) {
	securityGroupIds := []*string{aws.String("sg-123456789"), aws.String("sg-987654321")}
	snId := "12345678912345678912"
	snArn := "12345678912345678912"
	snvaArn := "12345678912345678912"
	name := "test"
	item := vpclattice.ServiceNetworkSummary{
		Arn:  &snArn,
		Id:   &snId,
		Name: &name,
	}

	status := vpclattice.ServiceNetworkVpcAssociationStatusCreateInProgress
	items := vpclattice.ServiceNetworkVpcAssociationSummary{
		ServiceNetworkArn:  &snArn,
		ServiceNetworkId:   &snId,
		ServiceNetworkName: &snId,
		Status:             &status,
		VpcId:              &config.VpcID,
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
	mockLattice.EXPECT().GetServiceNetworkVpcAssociationWithContext(ctx, gomock.Any()).Times(0)
	mockLattice.EXPECT().ListServiceNetworkVpcAssociationsAsList(ctx, gomock.Any()).Return(statusServiceNetworkVPCOutput, nil)
	mockLattice.EXPECT().CreateServiceNetworkServiceAssociationWithContext(ctx, gomock.Any()).Times(0)
	mockLattice.EXPECT().UpdateServiceNetworkVpcAssociationWithContext(ctx, gomock.Any()).Times(0)

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)
	_, err := snMgr.UpsertVpcAssociation(ctx, name, securityGroupIds)

	assert.Equal(t, err, errors.New(LATTICE_RETRY))
}

func Test_defaultServiceNetworkManager_CreateOrUpdate_SnExists_SnvaExists_CannotUpdateSecurityGroupsFromNonemptyToEmpty(t *testing.T) {
	securityGroupIds := []*string{aws.String("sg-123456789"), aws.String("sg-987654321")}
	snId := "12345678912345678912"
	snArn := "12345678912345678912"
	snvaArn := "12345678912345678912"
	name := "test"
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
		VpcId:              &config.VpcID,
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
	mockLattice.EXPECT().GetServiceNetworkVpcAssociationWithContext(ctx, gomock.Any()).Return(&vpclattice.GetServiceNetworkVpcAssociationOutput{
		ServiceNetworkArn:  &snArn,
		ServiceNetworkId:   &snId,
		ServiceNetworkName: &name,
		Status:             aws.String(vpclattice.ServiceNetworkVpcAssociationStatusActive),
		VpcId:              &config.VpcID,
		SecurityGroupIds:   securityGroupIds,
	}, nil)
	mockLattice.EXPECT().ListServiceNetworkVpcAssociationsAsList(ctx, gomock.Any()).Return(statusServiceNetworkVPCOutput, nil)
	mockLattice.EXPECT().ListTagsForResourceWithContext(ctx, gomock.Any()).Return(&vpclattice.ListTagsForResourceOutput{
		Tags: cloud.DefaultTags(),
	}, nil)
	mockLattice.EXPECT().CreateServiceNetworkServiceAssociationWithContext(ctx, gomock.Any()).Times(0)
	updateSNVAError := errors.New("InvalidParameterException SecurityGroupIds cannot be empty")
	mockLattice.EXPECT().UpdateServiceNetworkVpcAssociationWithContext(ctx, gomock.Any()).Return(&vpclattice.UpdateServiceNetworkVpcAssociationOutput{}, updateSNVAError)

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)
	_, err := snMgr.UpsertVpcAssociation(ctx, name, []*string{})

	assert.Equal(t, err, updateSNVAError)
}
