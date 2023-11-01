package lattice

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"

	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	mocks "github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

// ServiceNetwork does not exist before, happy case.
func Test_CreateOrUpdateServiceNetwork_SnNotExist_NoNeedToAssociate(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

	snCreateInput := model.ServiceNetwork{
		Spec: model.ServiceNetworkSpec{
			Name:           "test",
			Account:        "123456789",
			AssociateToVPC: false,
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
	createServiceNetworkInput.Tags[model.K8SServiceNetworkOwnedByVPC] = &config.VpcID

	mockLattice.EXPECT().CreateServiceNetworkWithContext(ctx, createServiceNetworkInput).Return(snCreateOutput, nil)
	mockLattice.EXPECT().FindServiceNetwork(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, &mocks.NotFoundError{}).Times(1)
	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)
	resp, err := snMgr.CreateOrUpdate(ctx, &snCreateInput)

	assert.Nil(t, err)
	assert.Equal(t, resp.ServiceNetworkARN, arn)
	assert.Equal(t, resp.ServiceNetworkID, id)
}

func Test_CreateOrUpdateServiceNetwork_SnNotExist_NeedToAssociate(t *testing.T) {
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
	createServiceNetworkInput.Tags[model.K8SServiceNetworkOwnedByVPC] = &config.VpcID

	mockLattice.EXPECT().CreateServiceNetworkWithContext(ctx, createServiceNetworkInput).Return(snCreateOutput, nil)
	snId := "12345678912345678912"
	createServiceNetworkVpcAssociationInput := &vpclattice.CreateServiceNetworkVpcAssociationInput{
		ServiceNetworkIdentifier: &snId,
		VpcIdentifier:            &config.VpcID,
	}
	associationStatus := vpclattice.ServiceNetworkVpcAssociationStatusActive
	createServiceNetworkVPCAssociationOutput := &vpclattice.CreateServiceNetworkVpcAssociationOutput{
		Status: &associationStatus,
	}
	mockLattice.EXPECT().
		CreateServiceNetworkVpcAssociationWithContext(ctx, createServiceNetworkVpcAssociationInput).
		Return(createServiceNetworkVPCAssociationOutput, nil)

	mockLattice.EXPECT().
		FindServiceNetwork(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).Times(1)

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

	mockLattice.EXPECT().FindServiceNetwork(ctx, gomock.Any(), gomock.Any()).Return(nil, errors.New("ERROR"))

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
	mockLattice.EXPECT().FindServiceNetwork(ctx, gomock.Any(), gomock.Any()).Return(
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
	mockLattice.EXPECT().FindServiceNetwork(ctx, gomock.Any(), gomock.Any()).Return(
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
	}
	statusServiceNetworkVPCOutput := []*vpclattice.ServiceNetworkVpcAssociationSummary{&items}

	mockLattice.EXPECT().FindServiceNetwork(ctx, gomock.Any(), gomock.Any()).Return(
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
	}, nil)
	mockLattice.EXPECT().ListServiceNetworkVpcAssociationsAsList(ctx, gomock.Any()).Return(statusServiceNetworkVPCOutput, nil)
	mockLattice.EXPECT().UpdateServiceNetworkVpcAssociationWithContext(ctx, gomock.Any()).Times(0)

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)
	resp, err := snMgr.CreateOrUpdate(ctx, &snCreateInput)

	assert.Nil(t, err)
	assert.Equal(t, resp.ServiceNetworkARN, snArn)
	assert.Equal(t, resp.ServiceNetworkID, snId)
}

func Test_defaultServiceNetworkManager_CreateOrUpdate_SnExists_SnvaExists_UpdateSNVASecurityGroups(t *testing.T) {
	securityGroupIds := []*string{aws.String("sg-123456789"), aws.String("sg-987654321")}
	snCreateInput := model.ServiceNetwork{
		Spec: model.ServiceNetworkSpec{
			Name:             "test",
			Account:          "123456789",
			AssociateToVPC:   true,
			SecurityGroupIds: securityGroupIds,
		},
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

	status := vpclattice.ServiceNetworkVpcAssociationStatusActive
	items := vpclattice.ServiceNetworkVpcAssociationSummary{
		ServiceNetworkArn:  &snArn,
		ServiceNetworkId:   &snId,
		ServiceNetworkName: &snId,
		Status:             &status,
		VpcId:              &config.VpcID,
	}
	statusServiceNetworkVPCOutput := []*vpclattice.ServiceNetworkVpcAssociationSummary{&items}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)
	mockLattice.EXPECT().FindServiceNetwork(ctx, gomock.Any(), gomock.Any()).Return(
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
	mockLattice.EXPECT().CreateServiceNetworkServiceAssociationWithContext(ctx, gomock.Any()).MaxTimes(0)
	mockLattice.EXPECT().UpdateServiceNetworkVpcAssociationWithContext(ctx, gomock.Any()).Return(&vpclattice.UpdateServiceNetworkVpcAssociationOutput{
		Arn:              &snArn,
		Id:               &snId,
		SecurityGroupIds: securityGroupIds,
		Status:           aws.String(vpclattice.ServiceNetworkVpcAssociationStatusActive),
	}, nil)

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)
	resp, err := snMgr.CreateOrUpdate(ctx, &snCreateInput)

	assert.Equal(t, err, nil)
	assert.Equal(t, resp.ServiceNetworkARN, snArn)
	assert.Equal(t, resp.ServiceNetworkID, snId)
	assert.Equal(t, resp.SnvaSecurityGroupIds, securityGroupIds)
}

func Test_defaultServiceNetworkManager_CreateOrUpdate_SnExists_SnvaExists_SecurityGroupsDoNotNeedToBeUpdated(t *testing.T) {
	securityGroupIds := []*string{aws.String("sg-123456789"), aws.String("sg-987654321")}
	desiredSn := model.ServiceNetwork{
		Spec: model.ServiceNetworkSpec{
			Name:             "test",
			Account:          "123456789",
			AssociateToVPC:   true,
			SecurityGroupIds: securityGroupIds,
		},
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

	status := vpclattice.ServiceNetworkVpcAssociationStatusActive
	items := vpclattice.ServiceNetworkVpcAssociationSummary{
		ServiceNetworkArn:  &snArn,
		ServiceNetworkId:   &snId,
		ServiceNetworkName: &snId,
		Status:             &status,
		VpcId:              &config.VpcID,
	}
	statusServiceNetworkVPCOutput := []*vpclattice.ServiceNetworkVpcAssociationSummary{&items}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)
	mockLattice.EXPECT().FindServiceNetwork(ctx, gomock.Any(), gomock.Any()).Return(
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
	mockLattice.EXPECT().CreateServiceNetworkServiceAssociationWithContext(ctx, gomock.Any()).Times(0)
	mockLattice.EXPECT().UpdateServiceNetworkVpcAssociationWithContext(ctx, gomock.Any()).Times(0)

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)
	resp, err := snMgr.CreateOrUpdate(ctx, &desiredSn)

	assert.Equal(t, err, nil)
	assert.Equal(t, resp.ServiceNetworkARN, snArn)
	assert.Equal(t, resp.ServiceNetworkID, snId)
	assert.Equal(t, resp.SnvaSecurityGroupIds, securityGroupIds)
}
func Test_defaultServiceNetworkManager_CreateOrUpdate_SnExists_SnvaCreateInProgress_WillNotInvokeLatticeUpdateSNVA(t *testing.T) {
	securityGroupIds := []*string{aws.String("sg-123456789"), aws.String("sg-987654321")}
	desiredSn := model.ServiceNetwork{
		Spec: model.ServiceNetworkSpec{
			Name:             "test",
			Account:          "123456789",
			AssociateToVPC:   true,
			SecurityGroupIds: securityGroupIds,
		},
	}
	snId := "12345678912345678912"
	snArn := "12345678912345678912"
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
	}
	statusServiceNetworkVPCOutput := []*vpclattice.ServiceNetworkVpcAssociationSummary{&items}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)
	mockLattice.EXPECT().FindServiceNetwork(ctx, gomock.Any(), gomock.Any()).Return(
		&mocks.ServiceNetworkInfo{
			SvcNetwork: item,
			Tags:       nil,
		}, nil)
	mockLattice.EXPECT().GetServiceNetworkVpcAssociationWithContext(ctx, gomock.Any()).Times(0)
	mockLattice.EXPECT().ListServiceNetworkVpcAssociationsAsList(ctx, gomock.Any()).Return(statusServiceNetworkVPCOutput, nil)
	mockLattice.EXPECT().CreateServiceNetworkServiceAssociationWithContext(ctx, gomock.Any()).Times(0)
	mockLattice.EXPECT().UpdateServiceNetworkVpcAssociationWithContext(ctx, gomock.Any()).Times(0)

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)
	resp, err := snMgr.CreateOrUpdate(ctx, &desiredSn)

	assert.Equal(t, err, errors.New(LATTICE_RETRY))
	assert.Equal(t, resp.ServiceNetworkARN, "")
	assert.Equal(t, resp.ServiceNetworkID, "")
}

func Test_defaultServiceNetworkManager_CreateOrUpdate_SnExists_SnvaExists_CannotUpdateSecurityGroupsFromNonemptyToEmpty(t *testing.T) {
	securityGroupIds := []*string{aws.String("sg-123456789"), aws.String("sg-987654321")}
	desiredSn := model.ServiceNetwork{
		Spec: model.ServiceNetworkSpec{
			Name:             "test",
			Account:          "123456789",
			AssociateToVPC:   true,
			SecurityGroupIds: []*string{},
		},
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

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	mockCloud := pkg_aws.NewMockCloud(c)
	mockLattice.EXPECT().FindServiceNetwork(ctx, gomock.Any(), gomock.Any()).Return(
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
	mockLattice.EXPECT().CreateServiceNetworkServiceAssociationWithContext(ctx, gomock.Any()).Times(0)
	updateSNVAError := errors.New("InvalidParameterException SecurityGroupIds cannot be empty")
	mockLattice.EXPECT().UpdateServiceNetworkVpcAssociationWithContext(ctx, gomock.Any()).Return(&vpclattice.UpdateServiceNetworkVpcAssociationOutput{}, updateSNVAError)

	mockCloud.EXPECT().Lattice().Return(mockLattice).AnyTimes()
	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, mockCloud)
	resp, err := snMgr.CreateOrUpdate(ctx, &desiredSn)

	assert.Equal(t, err, updateSNVAError)
	assert.Equal(t, resp.ServiceNetworkARN, "")
	assert.Equal(t, resp.ServiceNetworkID, "")
}

func Test_CreateOrUpdateServiceNetwork_SnAlreadyExist_AssociateToNotAssociate(t *testing.T) {
	snCreateInput := model.ServiceNetwork{
		Spec: model.ServiceNetworkSpec{
			Name:           "test",
			Account:        "123456789",
			AssociateToVPC: false,
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

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)
	mockLattice.EXPECT().FindServiceNetwork(ctx, gomock.Any(), gomock.Any()).Return(
		&mocks.ServiceNetworkInfo{
			SvcNetwork: item,
			Tags:       nil,
		}, nil)
	mockLattice.EXPECT().ListServiceNetworkVpcAssociationsAsList(ctx, gomock.Any()).Return(statusServiceNetworkVPCOutput, nil)
	deleteInProgressStatus := vpclattice.ServiceNetworkVpcAssociationStatusDeleteInProgress
	deleteServiceNetworkVpcAssociationOutput := &vpclattice.DeleteServiceNetworkVpcAssociationOutput{Status: &deleteInProgressStatus}

	mockLattice.EXPECT().DeleteServiceNetworkVpcAssociationWithContext(ctx, gomock.Any()).Return(deleteServiceNetworkVpcAssociationOutput, nil)

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)
	_, err := snMgr.CreateOrUpdate(ctx, &snCreateInput)

	assert.Equal(t, err, errors.New(LATTICE_RETRY))

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
	createServiceNetworkVpcAssociationInput := &vpclattice.CreateServiceNetworkVpcAssociationInput{
		ServiceNetworkIdentifier: &snId,
		VpcIdentifier:            &config.VpcID,
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
	snTagsOuput.Tags[model.K8SServiceNetworkOwnedByVPC] = &config.VpcID
	mockLattice.EXPECT().FindServiceNetwork(ctx, gomock.Any(), gomock.Any()).Return(
		&mocks.ServiceNetworkInfo{
			SvcNetwork: item,
			Tags:       snTagsOuput.Tags,
		}, nil)
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
	createServiceNetworkVpcAssociationInput := &vpclattice.CreateServiceNetworkVpcAssociationInput{
		ServiceNetworkIdentifier: &snId,
		VpcIdentifier:            &config.VpcID,
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
	dummy_vpc := "dummy-vpc-id"
	snTagsOuput.Tags[model.K8SServiceNetworkOwnedByVPC] = &dummy_vpc
	mockLattice.EXPECT().FindServiceNetwork(ctx, gomock.Any(), gomock.Any()).Return(
		&mocks.ServiceNetworkInfo{
			SvcNetwork: item,
			Tags:       snTagsOuput.Tags,
		}, nil)
	mockLattice.EXPECT().CreateServiceNetworkVpcAssociationWithContext(ctx, createServiceNetworkVpcAssociationInput).Return(createServiceNetworkVPCAssociationOutput, nil)

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)
	resp, err := snMgr.CreateOrUpdate(ctx, &snCreateInput)

	assert.Nil(t, err)
	assert.Equal(t, resp.ServiceNetworkARN, snArn)
	assert.Equal(t, resp.ServiceNetworkID, snId)
}

// ServiceNetwork does not exists, association is ServiceNetworkVpcAssociationStatusFailed.
func Test_CreateOrUpdateServiceNetwork_SnNotExist_ServiceNetworkVpcAssociationStatusFailed(t *testing.T) {
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

	associationStatus := vpclattice.ServiceNetworkVpcAssociationStatusCreateFailed
	createServiceNetworkVPCAssociationOutput := &vpclattice.CreateServiceNetworkVpcAssociationOutput{
		Status: &associationStatus,
	}
	snCreateOutput := &vpclattice.CreateServiceNetworkOutput{
		Arn:  &snArn,
		Id:   &snId,
		Name: &name,
	}
	snCreateInput := &vpclattice.CreateServiceNetworkInput{
		Name: &name,
		Tags: cloud.DefaultTags(),
	}
	snCreateInput.Tags[model.K8SServiceNetworkOwnedByVPC] = &config.VpcID

	createServiceNetworkVpcAssociationInput := &vpclattice.CreateServiceNetworkVpcAssociationInput{
		ServiceNetworkIdentifier: &snId,
		VpcIdentifier:            &config.VpcID,
	}

	mockLattice.EXPECT().FindServiceNetwork(ctx, gomock.Any(), gomock.Any()).Return(nil, nil)
	mockLattice.EXPECT().CreateServiceNetworkWithContext(ctx, snCreateInput).Return(snCreateOutput, nil)
	mockLattice.EXPECT().CreateServiceNetworkVpcAssociationWithContext(ctx, createServiceNetworkVpcAssociationInput).Return(createServiceNetworkVPCAssociationOutput, nil)

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)
	resp, err := snMgr.CreateOrUpdate(ctx, &CreateInput)

	assert.NotNil(t, err)
	assert.Equal(t, err, errors.New(LATTICE_RETRY))
	assert.Equal(t, resp.ServiceNetworkARN, "")
	assert.Equal(t, resp.ServiceNetworkID, "")
}

// ServiceNetwork does not exists, association is ServiceNetworkVpcAssociationStatusCreateInProgress.
func Test_CreateOrUpdateServiceNetwork_SnNOTExist_ServiceNetworkVpcAssociationStatusCreateInProgress(t *testing.T) {
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
	associationStatus := vpclattice.ServiceNetworkVpcAssociationStatusCreateInProgress
	createServiceNetworkVPCAssociationOutput := &vpclattice.CreateServiceNetworkVpcAssociationOutput{
		Status: &associationStatus,
	}
	snCreateOutput := &vpclattice.CreateServiceNetworkOutput{
		Arn:  &snArn,
		Id:   &snId,
		Name: &name,
	}
	snCreateInput := &vpclattice.CreateServiceNetworkInput{
		Name: &name,
		Tags: cloud.DefaultTags(),
	}
	snCreateInput.Tags[model.K8SServiceNetworkOwnedByVPC] = &config.VpcID
	createServiceNetworkVpcAssociationInput := &vpclattice.CreateServiceNetworkVpcAssociationInput{
		ServiceNetworkIdentifier: &snId,
		VpcIdentifier:            &config.VpcID,
	}

	mockLattice.EXPECT().FindServiceNetwork(ctx, gomock.Any(), gomock.Any()).Return(nil, nil)
	mockLattice.EXPECT().CreateServiceNetworkWithContext(ctx, snCreateInput).Return(snCreateOutput, nil)
	mockLattice.EXPECT().CreateServiceNetworkVpcAssociationWithContext(ctx, createServiceNetworkVpcAssociationInput).Return(createServiceNetworkVPCAssociationOutput, nil)

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)
	resp, err := snMgr.CreateOrUpdate(ctx, &CreateInput)

	assert.NotNil(t, err)
	assert.Equal(t, err, errors.New(LATTICE_RETRY))
	assert.Equal(t, resp.ServiceNetworkARN, "")
	assert.Equal(t, resp.ServiceNetworkID, "")
}

// ServiceNetwork does not exists, association is ServiceNetworkVpcAssociationStatusDeleteInProgress.
func Test_CreateOrUpdateServiceNetwork_SnNotExist_ServiceNetworkVpcAssociationStatusDeleteInProgress(t *testing.T) {
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
	associationStatus := vpclattice.ServiceNetworkVpcAssociationStatusDeleteInProgress
	createServiceNetworkVPCAssociationOutput := &vpclattice.CreateServiceNetworkVpcAssociationOutput{
		Status: &associationStatus,
	}
	snCreateOutput := &vpclattice.CreateServiceNetworkOutput{
		Arn:  &snArn,
		Id:   &snId,
		Name: &name,
	}
	snCreateInput := &vpclattice.CreateServiceNetworkInput{
		Name: &name,
		Tags: cloud.DefaultTags(),
	}
	snCreateInput.Tags[model.K8SServiceNetworkOwnedByVPC] = &config.VpcID
	createServiceNetworkVpcAssociationInput := &vpclattice.CreateServiceNetworkVpcAssociationInput{
		ServiceNetworkIdentifier: &snId,
		VpcIdentifier:            &config.VpcID,
	}

	mockLattice.EXPECT().FindServiceNetwork(ctx, gomock.Any(), gomock.Any()).Return(nil, nil)
	mockLattice.EXPECT().CreateServiceNetworkWithContext(ctx, snCreateInput).Return(snCreateOutput, nil)
	mockLattice.EXPECT().CreateServiceNetworkVpcAssociationWithContext(ctx, createServiceNetworkVpcAssociationInput).Return(createServiceNetworkVPCAssociationOutput, nil)

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)
	resp, err := snMgr.CreateOrUpdate(ctx, &CreateInput)

	assert.NotNil(t, err)
	assert.Equal(t, err, errors.New(LATTICE_RETRY))
	assert.Equal(t, resp.ServiceNetworkARN, "")
	assert.Equal(t, resp.ServiceNetworkID, "")
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
	snCreateInput.Tags[model.K8SServiceNetworkOwnedByVPC] = &config.VpcID
	createServiceNetworkVpcAssociationInput := &vpclattice.CreateServiceNetworkVpcAssociationInput{
		ServiceNetworkIdentifier: &snId,
		VpcIdentifier:            &config.VpcID,
	}

	mockLattice.EXPECT().FindServiceNetwork(ctx, gomock.Any(), gomock.Any()).Return(nil, nil)
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

	snCreateInput.Tags[model.K8SServiceNetworkOwnedByVPC] = &config.VpcID

	mockLattice.EXPECT().FindServiceNetwork(ctx, gomock.Any(), gomock.Any()).Return(nil, nil)
	mockLattice.EXPECT().CreateServiceNetworkWithContext(ctx, snCreateInput).Return(snCreateOutput, errors.New("ERROR"))

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)
	resp, err := snMgr.CreateOrUpdate(ctx, &CreateInput)

	assert.NotNil(t, err)
	assert.Equal(t, err, errors.New("ERROR"))
	assert.Equal(t, resp.ServiceNetworkARN, "")
	assert.Equal(t, resp.ServiceNetworkID, "")
}

func Test_DeleteSn_SnNotExist(t *testing.T) {

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)
	mockLattice.EXPECT().FindServiceNetwork(ctx, gomock.Any(), gomock.Any()).Return(nil, &mocks.NotFoundError{})

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)
	err := snMgr.Delete(ctx, "test")

	assert.Nil(t, err)
}

// delete a service network, which has no association and also was created by this VPC
func Test_DeleteSn_SnExistsNoAssociation(t *testing.T) {
	arn := "123456789"
	id := "123456789"
	name := "test"
	item := vpclattice.ServiceNetworkSummary{
		Arn:  &arn,
		Id:   &id,
		Name: &name,
	}

	statusServiceNetworkVPCOutput := []*vpclattice.ServiceNetworkVpcAssociationSummary{}

	deleteSnOutput := &vpclattice.DeleteServiceNetworkOutput{}
	deleteSnInout := &vpclattice.DeleteServiceNetworkInput{ServiceNetworkIdentifier: &id}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)
	mockLattice.EXPECT().ListServiceNetworkVpcAssociationsAsList(ctx, gomock.Any()).Return(statusServiceNetworkVPCOutput, nil)

	snTagsOuput := &vpclattice.ListTagsForResourceOutput{
		Tags: make(map[string]*string),
	}
	snTagsOuput.Tags[model.K8SServiceNetworkOwnedByVPC] = &config.VpcID
	mockLattice.EXPECT().FindServiceNetwork(ctx, gomock.Any(), gomock.Any()).Return(
		&mocks.ServiceNetworkInfo{
			SvcNetwork: item,
			Tags:       snTagsOuput.Tags,
		}, nil)
	mockLattice.EXPECT().DeleteServiceNetworkWithContext(ctx, deleteSnInout).Return(deleteSnOutput, nil)

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)
	err := snMgr.Delete(ctx, "test")

	assert.Nil(t, err)
}

// Deleting a service netwrok, when
// * the service network is associated with current VPC
// * and it is this VPC creates this service network
func Test_DeleteSn_SnExistsAssociatedWithVPC_Deleting(t *testing.T) {
	arn := "123456789"
	id := "123456789"
	name := "test"
	item := vpclattice.ServiceNetworkSummary{
		Arn:  &arn,
		Id:   &id,
		Name: &name,
	}

	associationArn := "123456789"
	associationID := "123456789"
	associationStatus := vpclattice.ServiceNetworkVpcAssociationStatusActive
	associationVPCId := config.VpcID
	itemAssociation := vpclattice.ServiceNetworkVpcAssociationSummary{
		Arn:                &associationArn,
		Id:                 &associationID,
		ServiceNetworkArn:  &arn,
		ServiceNetworkId:   &id,
		ServiceNetworkName: &name,
		Status:             &associationStatus,
		VpcId:              &associationVPCId,
	}
	statusServiceNetworkVPCOutput := []*vpclattice.ServiceNetworkVpcAssociationSummary{&itemAssociation}

	deleteInProgressStatus := vpclattice.ServiceNetworkVpcAssociationStatusDeleteInProgress
	deleteServiceNetworkVpcAssociationOutput := &vpclattice.DeleteServiceNetworkVpcAssociationOutput{Status: &deleteInProgressStatus}
	deleteServiceNetworkVpcAssociationInput := &vpclattice.DeleteServiceNetworkVpcAssociationInput{ServiceNetworkVpcAssociationIdentifier: &associationID}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)
	mockLattice.EXPECT().ListServiceNetworkVpcAssociationsAsList(ctx, gomock.Any()).Return(statusServiceNetworkVPCOutput, nil)

	snTagsOuput := &vpclattice.ListTagsForResourceOutput{
		Tags: make(map[string]*string),
	}
	snTagsOuput.Tags[model.K8SServiceNetworkOwnedByVPC] = &config.VpcID
	mockLattice.EXPECT().FindServiceNetwork(ctx, gomock.Any(), gomock.Any()).Return(
		&mocks.ServiceNetworkInfo{
			SvcNetwork: item,
			Tags:       snTagsOuput.Tags,
		}, nil)
	mockLattice.EXPECT().DeleteServiceNetworkVpcAssociationWithContext(ctx, deleteServiceNetworkVpcAssociationInput).Return(deleteServiceNetworkVpcAssociationOutput, nil)

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)
	err := snMgr.Delete(ctx, "test")

	assert.NotNil(t, err)
	assert.Equal(t, err, errors.New(LATTICE_RETRY))
}

func Test_DeleteSn_SnExistsAssociatedWithOtherVPC(t *testing.T) {
	arn := "123456789"
	id := "123456789"
	name := "test"
	item := vpclattice.ServiceNetworkSummary{
		Arn:  &arn,
		Id:   &id,
		Name: &name,
	}

	associationArn := "123456789"
	associationID := "123456789"
	associationStatus := vpclattice.ServiceNetworkVpcAssociationStatusActive
	associationVPCId := "other-vpc-id"
	itemAssociation := vpclattice.ServiceNetworkVpcAssociationSummary{
		Arn:                &associationArn,
		Id:                 &associationID,
		ServiceNetworkArn:  &arn,
		ServiceNetworkId:   &id,
		ServiceNetworkName: &name,
		Status:             &associationStatus,
		VpcId:              &associationVPCId,
	}
	statusServiceNetworkVPCOutput := []*vpclattice.ServiceNetworkVpcAssociationSummary{&itemAssociation}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)
	mockLattice.EXPECT().ListServiceNetworkVpcAssociationsAsList(ctx, gomock.Any()).Return(statusServiceNetworkVPCOutput, nil)

	snTagsOuput := &vpclattice.ListTagsForResourceOutput{
		Tags: make(map[string]*string),
	}
	snTagsOuput.Tags[model.K8SServiceNetworkOwnedByVPC] = &config.VpcID
	mockLattice.EXPECT().FindServiceNetwork(ctx, gomock.Any(), gomock.Any()).Return(
		&mocks.ServiceNetworkInfo{
			SvcNetwork: item,
			Tags:       snTagsOuput.Tags,
		}, nil)

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)
	err := snMgr.Delete(ctx, "test")

	assert.NotNil(t, err)
	assert.Equal(t, err, errors.New(LATTICE_RETRY))
}

func Test_DeleteSn_SnExistsAssociatedWithOtherVPC_NotCreatedByVPC(t *testing.T) {
	arn := "123456789"
	id := "123456789"
	name := "test"
	item := vpclattice.ServiceNetworkSummary{
		Arn:  &arn,
		Id:   &id,
		Name: &name,
	}

	associationArn := "123456789"
	associationID := "123456789"
	associationStatus := vpclattice.ServiceNetworkVpcAssociationStatusActive
	associationVPCId := "123456789"
	itemAssociation := vpclattice.ServiceNetworkVpcAssociationSummary{
		Arn:                &associationArn,
		Id:                 &associationID,
		ServiceNetworkArn:  &arn,
		ServiceNetworkId:   &id,
		ServiceNetworkName: &name,
		Status:             &associationStatus,
		VpcId:              &associationVPCId,
	}
	statusServiceNetworkVPCOutput := []*vpclattice.ServiceNetworkVpcAssociationSummary{&itemAssociation}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)
	mockLattice.EXPECT().FindServiceNetwork(ctx, gomock.Any(), gomock.Any()).Return(
		&mocks.ServiceNetworkInfo{
			SvcNetwork: item,
			Tags:       nil,
		}, nil)
	mockLattice.EXPECT().ListServiceNetworkVpcAssociationsAsList(ctx, gomock.Any()).Return(statusServiceNetworkVPCOutput, nil)

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)
	err := snMgr.Delete(ctx, "test")

	assert.Nil(t, err)
}

func Test_DeleteSn_SnExistsAssociatedWithOtherVPC_CreatedByVPC(t *testing.T) {
	arn := "123456789"
	id := "123456789"
	name := "test"
	item := vpclattice.ServiceNetworkSummary{
		Arn:  &arn,
		Id:   &id,
		Name: &name,
	}

	associationArn := "123456789"
	associationID := "123456789"
	associationStatus := vpclattice.ServiceNetworkVpcAssociationStatusActive
	associationVPCId := "123456789"
	itemAssociation := vpclattice.ServiceNetworkVpcAssociationSummary{
		Arn:                &associationArn,
		Id:                 &associationID,
		ServiceNetworkArn:  &arn,
		ServiceNetworkId:   &id,
		ServiceNetworkName: &name,
		Status:             &associationStatus,
		VpcId:              &associationVPCId,
	}
	statusServiceNetworkVPCOutput := []*vpclattice.ServiceNetworkVpcAssociationSummary{&itemAssociation}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)
	mockLattice.EXPECT().ListServiceNetworkVpcAssociationsAsList(ctx, gomock.Any()).Return(statusServiceNetworkVPCOutput, nil)
	snTagsOutput := &vpclattice.ListTagsForResourceOutput{
		Tags: make(map[string]*string),
	}
	snTagsOutput.Tags[model.K8SServiceNetworkOwnedByVPC] = &config.VpcID
	mockLattice.EXPECT().FindServiceNetwork(ctx, gomock.Any(), gomock.Any()).Return(
		&mocks.ServiceNetworkInfo{
			SvcNetwork: item,
			Tags:       snTagsOutput.Tags,
		}, nil)

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)
	err := snMgr.Delete(ctx, "test")

	assert.NotNil(t, err)
	assert.Equal(t, err, errors.New(LATTICE_RETRY))
}

func Test_ListSn_SnExists(t *testing.T) {
	arn := "123456789"
	id := "123456789"
	name1 := "test1"
	item1 := vpclattice.ServiceNetworkSummary{
		Arn:  &arn,
		Id:   &id,
		Name: &name1,
	}
	name2 := "test2"
	item2 := vpclattice.ServiceNetworkSummary{
		Arn:  &arn,
		Id:   &id,
		Name: &name2,
	}
	listServiceNetworkOutput := []*vpclattice.ServiceNetworkSummary{&item1, &item2}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)
	mockLattice.EXPECT().ListServiceNetworksAsList(ctx, gomock.Any()).Return(listServiceNetworkOutput, nil)

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)
	snList, err := snMgr.List(ctx)

	assert.Nil(t, err)
	assert.Equal(t, snList, []string{"test1", "test2"})
}

func Test_ListSn_NoSn(t *testing.T) {
	listServiceNetworkOutput := []*vpclattice.ServiceNetworkSummary{}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)
	mockLattice.EXPECT().ListServiceNetworksAsList(ctx, gomock.Any()).Return(listServiceNetworkOutput, nil)

	snMgr := NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud)
	snList, err := snMgr.List(ctx)

	assert.Nil(t, err)
	assert.Equal(t, snList, []string{})
}
