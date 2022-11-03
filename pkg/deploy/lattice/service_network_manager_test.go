package lattice

import (
	"context"
	"errors"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-sdk-go/service/mercury"
	"testing"

	mocks_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	mocks "github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

// ServiceNetwork does not exist before,happy case.
func Test_CreateServiceNetwork_MeshNotExist(t *testing.T) {
	meshCreateInput := latticemodel.ServiceNetwork{
		Spec: latticemodel.ServiceNetworkSpec{
			Name:    "test",
			Account: "123456789",
		},
		Status: &latticemodel.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""},
	}
	arn := "12345678912345678912"
	id := "12345678912345678912"
	name := "test"
	meshCreateOutput := &mercury.CreateMeshOutput{
		Arn:  &arn,
		Id:   &id,
		Name: &name,
	}
	listServiceNetworkOutput := []*mercury.MeshSummary{}
	status := mercury.MeshVpcAssociationStatusActive
	createServiceNetworkVPCAssociationOutput := &mercury.CreateMeshVpcAssociationOutput{
		Status: &status,
	}
	createServiceNetworkInput := &mercury.CreateMeshInput{
		Name: &name,
	}
	createServiceNetworkVpcAssociationInput := &mercury.CreateMeshVpcAssociationInput{
		MeshIdentifier: &id,
		VpcIdentifier:  &config.VpcID,
	}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockCloud := mocks_aws.NewMockCloud(c)
	mockMercurySess := mocks.NewMockMercury(c)
	mockMercurySess.EXPECT().ListMeshesAsList(ctx, gomock.Any()).Return(listServiceNetworkOutput, nil)
	mockMercurySess.EXPECT().CreateMeshWithContext(ctx, createServiceNetworkInput).Return(meshCreateOutput, nil)
	mockMercurySess.EXPECT().CreateMeshVpcAssociationWithContext(ctx, createServiceNetworkVpcAssociationInput).Return(createServiceNetworkVPCAssociationOutput, nil)
	mockCloud.EXPECT().Mercury().Return(mockMercurySess).AnyTimes()

	meshManager := NewDefaultServiceNetworkManager(mockCloud)
	resp, err := meshManager.Create(ctx, &meshCreateInput)

	assert.Nil(t, err)
	assert.Equal(t, resp.ServiceNetworkARN, arn)
	assert.Equal(t, resp.ServiceNetworkID, id)
}

// List and find mesh does not work.
func Test_CreateServiceNetwork_ListFailed(t *testing.T) {
	meshCreateInput := latticemodel.ServiceNetwork{
		Spec: latticemodel.ServiceNetworkSpec{
			Name:    "test",
			Account: "123456789",
		},
		Status: &latticemodel.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""},
	}
	arn := "12345678912345678912"
	id := "12345678912345678912"
	name := "test"
	item := mercury.MeshSummary{
		Arn:  &arn,
		Id:   &id,
		Name: &name,
	}
	listServiceNetworkOutput := []*mercury.MeshSummary{&item}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockMercurySess := mocks.NewMockMercury(c)
	mockCloud := mocks_aws.NewMockCloud(c)
	mockMercurySess.EXPECT().ListMeshesAsList(ctx, gomock.Any()).Return(listServiceNetworkOutput, errors.New("ERROR"))
	mockCloud.EXPECT().Mercury().Return(mockMercurySess)

	meshManager := NewDefaultServiceNetworkManager(mockCloud)
	resp, err := meshManager.Create(ctx, &meshCreateInput)

	assert.NotNil(t, err)
	assert.Equal(t, err, errors.New("ERROR"))
	assert.Equal(t, resp.ServiceNetworkARN, "")
	assert.Equal(t, resp.ServiceNetworkID, "")
}

// ServiceNetwork already exists, association is in MeshVpcAssociationStatusCreateInProgress.

func Test_CreateServiceNetwork_MeshAlreadyExist_MeshVpcAssociationStatusCreateInProgress(t *testing.T) {
	meshCreateInput := latticemodel.ServiceNetwork{
		Spec: latticemodel.ServiceNetworkSpec{
			Name:    "test",
			Account: "123456789",
		},
		Status: &latticemodel.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""},
	}
	meshId := "12345678912345678912"
	meshArn := "12345678912345678912"
	name := "test"
	vpcId := config.VpcID
	item := mercury.MeshSummary{
		Arn:  &meshArn,
		Id:   &meshId,
		Name: &name,
	}
	listServiceNetworkOutput := []*mercury.MeshSummary{&item}

	status := mercury.MeshVpcAssociationStatusCreateInProgress
	items := mercury.MeshVpcAssociationSummary{
		MeshArn:  &meshArn,
		MeshId:   &meshId,
		MeshName: &meshId,
		Status:   &status,
		VpcId:    &vpcId,
	}
	statusServiceNetworkVPCOutput := []*mercury.MeshVpcAssociationSummary{&items}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockMercurySess := mocks.NewMockMercury(c)
	mockCloud := mocks_aws.NewMockCloud(c)
	mockMercurySess.EXPECT().ListMeshesAsList(ctx, gomock.Any()).Return(listServiceNetworkOutput, nil)
	mockMercurySess.EXPECT().ListMeshVpcAssociationsAsList(ctx, gomock.Any()).Return(statusServiceNetworkVPCOutput, nil)
	mockCloud.EXPECT().Mercury().Return(mockMercurySess).AnyTimes()

	meshManager := NewDefaultServiceNetworkManager(mockCloud)
	resp, err := meshManager.Create(ctx, &meshCreateInput)

	assert.NotNil(t, err)
	assert.Equal(t, err, errors.New(LATTICE_RETRY))
	assert.Equal(t, resp.ServiceNetworkARN, "")
	assert.Equal(t, resp.ServiceNetworkID, "")
}

// ServiceNetwork already exists, association is in MeshVpcAssociationStatusDeleteInProgress.

func Test_CreateServiceNetwork_MeshAlreadyExist_MeshVpcAssociationStatusDeleteInProgress(t *testing.T) {
	meshCreateInput := latticemodel.ServiceNetwork{
		Spec: latticemodel.ServiceNetworkSpec{
			Name:    "test",
			Account: "123456789",
		},
		Status: &latticemodel.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""},
	}
	meshId := "12345678912345678912"
	meshArn := "12345678912345678912"
	name := "test"
	vpcId := config.VpcID
	item := mercury.MeshSummary{
		Arn:  &meshArn,
		Id:   &meshId,
		Name: &name,
	}
	listServiceNetworkOutput := []*mercury.MeshSummary{&item}

	status := mercury.MeshVpcAssociationStatusDeleteInProgress
	items := mercury.MeshVpcAssociationSummary{
		MeshArn:  &meshArn,
		MeshId:   &meshId,
		MeshName: &meshId,
		Status:   &status,
		VpcId:    &vpcId,
	}
	statusServiceNetworkVPCOutput := []*mercury.MeshVpcAssociationSummary{&items}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockMercurySess := mocks.NewMockMercury(c)
	mockCloud := mocks_aws.NewMockCloud(c)
	mockMercurySess.EXPECT().ListMeshesAsList(ctx, gomock.Any()).Return(listServiceNetworkOutput, nil)
	mockMercurySess.EXPECT().ListMeshVpcAssociationsAsList(ctx, gomock.Any()).Return(statusServiceNetworkVPCOutput, nil)
	mockCloud.EXPECT().Mercury().Return(mockMercurySess).AnyTimes()

	meshManager := NewDefaultServiceNetworkManager(mockCloud)
	resp, err := meshManager.Create(ctx, &meshCreateInput)

	assert.NotNil(t, err)
	assert.Equal(t, err, errors.New(LATTICE_RETRY))
	assert.Equal(t, resp.ServiceNetworkARN, "")
	assert.Equal(t, resp.ServiceNetworkID, "")
}

// ServiceNetwork already exists, association is MeshVpcAssociationStatusActive.
func Test_CreateServiceNetwork_MeshAlreadyExist_MeshVpcAssociationStatusActive(t *testing.T) {
	meshCreateInput := latticemodel.ServiceNetwork{
		Spec: latticemodel.ServiceNetworkSpec{
			Name:    "test",
			Account: "123456789",
		},
		Status: &latticemodel.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""},
	}
	meshId := "12345678912345678912"
	meshArn := "12345678912345678912"
	name := "test"
	item := mercury.MeshSummary{
		Arn:  &meshArn,
		Id:   &meshId,
		Name: &name,
	}
	listServiceNetworkOutput := []*mercury.MeshSummary{&item}

	status := mercury.MeshVpcAssociationStatusActive
	items := mercury.MeshVpcAssociationSummary{
		MeshArn:  &meshArn,
		MeshId:   &meshId,
		MeshName: &meshId,
		Status:   &status,
		VpcId:    &config.VpcID,
	}
	statusServiceNetworkVPCOutput := []*mercury.MeshVpcAssociationSummary{&items}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockMercurySess := mocks.NewMockMercury(c)
	mockCloud := mocks_aws.NewMockCloud(c)
	mockMercurySess.EXPECT().ListMeshesAsList(ctx, gomock.Any()).Return(listServiceNetworkOutput, nil)
	mockMercurySess.EXPECT().ListMeshVpcAssociationsAsList(ctx, gomock.Any()).Return(statusServiceNetworkVPCOutput, nil)
	mockCloud.EXPECT().Mercury().Return(mockMercurySess).AnyTimes()

	meshManager := NewDefaultServiceNetworkManager(mockCloud)
	resp, err := meshManager.Create(ctx, &meshCreateInput)

	assert.Nil(t, err)
	assert.Equal(t, resp.ServiceNetworkARN, meshArn)
	assert.Equal(t, resp.ServiceNetworkID, meshId)
}

// ServiceNetwork already exists, association is MeshVpcAssociationStatusCreateFailed.
func Test_CreateServiceNetwork_MeshAlreadyExist_MeshVpcAssociationStatusCreateFailed(t *testing.T) {
	meshCreateInput := latticemodel.ServiceNetwork{
		Spec: latticemodel.ServiceNetworkSpec{
			Name:    "test",
			Account: "123456789",
		},
		Status: &latticemodel.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""},
	}
	meshId := "12345678912345678912"
	meshArn := "12345678912345678912"
	name := "test"
	item := mercury.MeshSummary{
		Arn:  &meshArn,
		Id:   &meshId,
		Name: &name,
	}
	listServiceNetworkOutput := []*mercury.MeshSummary{&item}

	status := mercury.MeshVpcAssociationStatusCreateFailed
	items := mercury.MeshVpcAssociationSummary{
		MeshArn:  &meshArn,
		MeshId:   &meshId,
		MeshName: &meshId,
		Status:   &status,
		VpcId:    &config.VpcID,
	}
	statusServiceNetworkVPCOutput := []*mercury.MeshVpcAssociationSummary{&items}

	associationStatus := mercury.MeshVpcAssociationStatusActive
	createServiceNetworkVPCAssociationOutput := &mercury.CreateMeshVpcAssociationOutput{
		Status: &associationStatus,
	}
	createServiceNetworkVpcAssociationInput := &mercury.CreateMeshVpcAssociationInput{
		MeshIdentifier: &meshId,
		VpcIdentifier:  &config.VpcID,
	}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockMercurySess := mocks.NewMockMercury(c)
	mockCloud := mocks_aws.NewMockCloud(c)
	mockMercurySess.EXPECT().ListMeshesAsList(ctx, gomock.Any()).Return(listServiceNetworkOutput, nil)
	mockMercurySess.EXPECT().ListMeshVpcAssociationsAsList(ctx, gomock.Any()).Return(statusServiceNetworkVPCOutput, nil)
	mockMercurySess.EXPECT().CreateMeshVpcAssociationWithContext(ctx, createServiceNetworkVpcAssociationInput).Return(createServiceNetworkVPCAssociationOutput, nil)
	mockCloud.EXPECT().Mercury().Return(mockMercurySess).AnyTimes()

	meshManager := NewDefaultServiceNetworkManager(mockCloud)
	resp, err := meshManager.Create(ctx, &meshCreateInput)

	assert.Nil(t, err)
	assert.Equal(t, resp.ServiceNetworkARN, meshArn)
	assert.Equal(t, resp.ServiceNetworkID, meshId)
}

// ServiceNetwork already exists, associated with other VPC
func Test_CreateServiceNetwork_MeshAlreadyExist_MeshAssociatedWithOtherVPC(t *testing.T) {
	meshCreateInput := latticemodel.ServiceNetwork{
		Spec: latticemodel.ServiceNetworkSpec{
			Name:    "test",
			Account: "123456789",
		},
		Status: &latticemodel.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""},
	}
	meshId := "12345678912345678912"
	meshArn := "12345678912345678912"
	name := "test"
	vpcId := "123445677"
	item := mercury.MeshSummary{
		Arn:  &meshArn,
		Id:   &meshId,
		Name: &name,
	}
	listServiceNetworkOutput := []*mercury.MeshSummary{&item}

	status := mercury.MeshVpcAssociationStatusCreateFailed
	items := mercury.MeshVpcAssociationSummary{
		MeshArn:  &meshArn,
		MeshId:   &meshId,
		MeshName: &meshId,
		Status:   &status,
		VpcId:    &vpcId,
	}
	statusServiceNetworkVPCOutput := []*mercury.MeshVpcAssociationSummary{&items}

	associationStatus := mercury.MeshVpcAssociationStatusActive
	createServiceNetworkVPCAssociationOutput := &mercury.CreateMeshVpcAssociationOutput{
		Status: &associationStatus,
	}
	createServiceNetworkVpcAssociationInput := &mercury.CreateMeshVpcAssociationInput{
		MeshIdentifier: &meshId,
		VpcIdentifier:  &config.VpcID,
	}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockMercurySess := mocks.NewMockMercury(c)
	mockCloud := mocks_aws.NewMockCloud(c)
	mockMercurySess.EXPECT().ListMeshesAsList(ctx, gomock.Any()).Return(listServiceNetworkOutput, nil)
	mockMercurySess.EXPECT().ListMeshVpcAssociationsAsList(ctx, gomock.Any()).Return(statusServiceNetworkVPCOutput, nil)
	mockMercurySess.EXPECT().CreateMeshVpcAssociationWithContext(ctx, createServiceNetworkVpcAssociationInput).Return(createServiceNetworkVPCAssociationOutput, nil)
	mockCloud.EXPECT().Mercury().Return(mockMercurySess).AnyTimes()

	meshManager := NewDefaultServiceNetworkManager(mockCloud)
	resp, err := meshManager.Create(ctx, &meshCreateInput)

	assert.Nil(t, err)
	assert.Equal(t, resp.ServiceNetworkARN, meshArn)
	assert.Equal(t, resp.ServiceNetworkID, meshId)
}

// ServiceNetwork does not exists, association is MeshVpcAssociationStatusFailed.
func Test_CreateServiceNetwork_MeshNotExist_MeshVpcAssociationStatusFailed(t *testing.T) {
	CreateInput := latticemodel.ServiceNetwork{
		Spec: latticemodel.ServiceNetworkSpec{
			Name:    "test",
			Account: "123456789",
		},
		Status: &latticemodel.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""},
	}
	meshId := "12345678912345678912"
	meshArn := "12345678912345678912"
	name := "test"

	listServiceNetworkOutput := []*mercury.MeshSummary{}
	associationStatus := mercury.MeshVpcAssociationStatusCreateFailed
	createServiceNetworkVPCAssociationOutput := &mercury.CreateMeshVpcAssociationOutput{
		Status: &associationStatus,
	}
	meshCreateOutput := &mercury.CreateMeshOutput{
		Arn:  &meshArn,
		Id:   &meshId,
		Name: &name,
	}
	meshCreateInput := &mercury.CreateMeshInput{
		Name: &name,
	}
	createServiceNetworkVpcAssociationInput := &mercury.CreateMeshVpcAssociationInput{
		MeshIdentifier: &meshId,
		VpcIdentifier:  &config.VpcID,
	}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockMercurySess := mocks.NewMockMercury(c)
	mockCloud := mocks_aws.NewMockCloud(c)
	mockMercurySess.EXPECT().ListMeshesAsList(ctx, gomock.Any()).Return(listServiceNetworkOutput, nil)
	mockMercurySess.EXPECT().CreateMeshWithContext(ctx, meshCreateInput).Return(meshCreateOutput, nil)
	mockMercurySess.EXPECT().CreateMeshVpcAssociationWithContext(ctx, createServiceNetworkVpcAssociationInput).Return(createServiceNetworkVPCAssociationOutput, nil)
	mockCloud.EXPECT().Mercury().Return(mockMercurySess).AnyTimes()

	meshManager := NewDefaultServiceNetworkManager(mockCloud)
	resp, err := meshManager.Create(ctx, &CreateInput)

	assert.NotNil(t, err)
	assert.Equal(t, err, errors.New(LATTICE_RETRY))
	assert.Equal(t, resp.ServiceNetworkARN, "")
	assert.Equal(t, resp.ServiceNetworkID, "")
}

// ServiceNetwork does not exists, association is MeshVpcAssociationStatusCreateInProgress.
func Test_CreateServiceNetwork_MeshNOTExist_MeshVpcAssociationStatusCreateInProgress(t *testing.T) {
	CreateInput := latticemodel.ServiceNetwork{
		Spec: latticemodel.ServiceNetworkSpec{
			Name:    "test",
			Account: "123456789",
		},
		Status: &latticemodel.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""},
	}
	meshId := "12345678912345678912"
	meshArn := "12345678912345678912"
	name := "test"
	listServiceNetworkOutput := []*mercury.MeshSummary{}
	associationStatus := mercury.MeshVpcAssociationStatusCreateInProgress
	createServiceNetworkVPCAssociationOutput := &mercury.CreateMeshVpcAssociationOutput{
		Status: &associationStatus,
	}
	meshCreateOutput := &mercury.CreateMeshOutput{
		Arn:  &meshArn,
		Id:   &meshId,
		Name: &name,
	}
	meshCreateInput := &mercury.CreateMeshInput{
		Name: &name,
	}
	createServiceNetworkVpcAssociationInput := &mercury.CreateMeshVpcAssociationInput{
		MeshIdentifier: &meshId,
		VpcIdentifier:  &config.VpcID,
	}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockMercurySess := mocks.NewMockMercury(c)
	mockCloud := mocks_aws.NewMockCloud(c)
	mockMercurySess.EXPECT().ListMeshesAsList(ctx, gomock.Any()).Return(listServiceNetworkOutput, nil)
	mockMercurySess.EXPECT().CreateMeshWithContext(ctx, meshCreateInput).Return(meshCreateOutput, nil)
	mockMercurySess.EXPECT().CreateMeshVpcAssociationWithContext(ctx, createServiceNetworkVpcAssociationInput).Return(createServiceNetworkVPCAssociationOutput, nil)
	mockCloud.EXPECT().Mercury().Return(mockMercurySess).AnyTimes()

	meshManager := NewDefaultServiceNetworkManager(mockCloud)
	resp, err := meshManager.Create(ctx, &CreateInput)

	assert.NotNil(t, err)
	assert.Equal(t, err, errors.New(LATTICE_RETRY))
	assert.Equal(t, resp.ServiceNetworkARN, "")
	assert.Equal(t, resp.ServiceNetworkID, "")
}

// ServiceNetwork does not exists, association is MeshVpcAssociationStatusDeleteInProgress.
func Test_CreateServiceNetwork_MeshNotExist_MeshVpcAssociationStatusDeleteInProgress(t *testing.T) {
	CreateInput := latticemodel.ServiceNetwork{
		Spec: latticemodel.ServiceNetworkSpec{
			Name:    "test",
			Account: "123456789",
		},
		Status: &latticemodel.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""},
	}
	meshId := "12345678912345678912"
	meshArn := "12345678912345678912"
	name := "test"
	listServiceNetworkOutput := []*mercury.MeshSummary{}
	associationStatus := mercury.MeshVpcAssociationStatusDeleteInProgress
	createServiceNetworkVPCAssociationOutput := &mercury.CreateMeshVpcAssociationOutput{
		Status: &associationStatus,
	}
	meshCreateOutput := &mercury.CreateMeshOutput{
		Arn:  &meshArn,
		Id:   &meshId,
		Name: &name,
	}
	meshCreateInput := &mercury.CreateMeshInput{
		Name: &name,
	}
	createServiceNetworkVpcAssociationInput := &mercury.CreateMeshVpcAssociationInput{
		MeshIdentifier: &meshId,
		VpcIdentifier:  &config.VpcID,
	}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockMercurySess := mocks.NewMockMercury(c)
	mockCloud := mocks_aws.NewMockCloud(c)
	mockMercurySess.EXPECT().ListMeshesAsList(ctx, gomock.Any()).Return(listServiceNetworkOutput, nil)
	mockMercurySess.EXPECT().CreateMeshWithContext(ctx, meshCreateInput).Return(meshCreateOutput, nil)
	mockMercurySess.EXPECT().CreateMeshVpcAssociationWithContext(ctx, createServiceNetworkVpcAssociationInput).Return(createServiceNetworkVPCAssociationOutput, nil)
	mockCloud.EXPECT().Mercury().Return(mockMercurySess).AnyTimes()

	meshManager := NewDefaultServiceNetworkManager(mockCloud)
	resp, err := meshManager.Create(ctx, &CreateInput)

	assert.NotNil(t, err)
	assert.Equal(t, err, errors.New(LATTICE_RETRY))
	assert.Equal(t, resp.ServiceNetworkARN, "")
	assert.Equal(t, resp.ServiceNetworkID, "")
}

// ServiceNetwork does not exists, association returns Error.
func Test_CreateServiceNetwork_MeshNotExist_MeshVpcAssociationReturnsError(t *testing.T) {
	CreateInput := latticemodel.ServiceNetwork{
		Spec: latticemodel.ServiceNetworkSpec{
			Name:    "test",
			Account: "123456789",
		},
		Status: &latticemodel.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""},
	}
	meshId := "12345678912345678912"
	meshArn := "12345678912345678912"
	name := "test"
	listServiceNetworkOutput := []*mercury.MeshSummary{}
	createServiceNetworkVPCAssociationOutput := &mercury.CreateMeshVpcAssociationOutput{}
	meshCreateOutput := &mercury.CreateMeshOutput{
		Arn:  &meshArn,
		Id:   &meshId,
		Name: &name,
	}
	meshCreateInput := &mercury.CreateMeshInput{
		Name: &name,
	}
	createServiceNetworkVpcAssociationInput := &mercury.CreateMeshVpcAssociationInput{
		MeshIdentifier: &meshId,
		VpcIdentifier:  &config.VpcID,
	}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockMercurySess := mocks.NewMockMercury(c)
	mockCloud := mocks_aws.NewMockCloud(c)
	mockMercurySess.EXPECT().ListMeshesAsList(ctx, gomock.Any()).Return(listServiceNetworkOutput, nil)
	mockMercurySess.EXPECT().CreateMeshWithContext(ctx, meshCreateInput).Return(meshCreateOutput, nil)
	mockMercurySess.EXPECT().CreateMeshVpcAssociationWithContext(ctx, createServiceNetworkVpcAssociationInput).Return(createServiceNetworkVPCAssociationOutput, errors.New("ERROR"))
	mockCloud.EXPECT().Mercury().Return(mockMercurySess).AnyTimes()

	meshManager := NewDefaultServiceNetworkManager(mockCloud)
	resp, err := meshManager.Create(ctx, &CreateInput)

	assert.NotNil(t, err)
	assert.Equal(t, err, errors.New("ERROR"))
	assert.Equal(t, resp.ServiceNetworkARN, "")
	assert.Equal(t, resp.ServiceNetworkID, "")
}

// Mesh does not exist and failed to create.
func Test_CreateMesh_MeshNotExist_MeshCreateFailed(t *testing.T) {
	CreateInput := latticemodel.ServiceNetwork{
		Spec: latticemodel.ServiceNetworkSpec{
			Name:    "test",
			Account: "123456789",
		},
		Status: &latticemodel.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""},
	}
	arn := "12345678912345678912"
	id := "12345678912345678912"
	name := "test"
	meshCreateOutput := &mercury.CreateMeshOutput{
		Arn:  &arn,
		Id:   &id,
		Name: &name,
	}
	listServiceNetworkOutput := []*mercury.MeshSummary{}
	meshCreateInput := &mercury.CreateMeshInput{
		Name: &name,
	}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockMercurySess := mocks.NewMockMercury(c)
	mockCloud := mocks_aws.NewMockCloud(c)
	mockMercurySess.EXPECT().ListMeshesAsList(ctx, gomock.Any()).Return(listServiceNetworkOutput, nil)
	mockMercurySess.EXPECT().CreateMeshWithContext(ctx, meshCreateInput).Return(meshCreateOutput, errors.New("ERROR"))
	mockCloud.EXPECT().Mercury().Return(mockMercurySess).AnyTimes()

	meshManager := NewDefaultServiceNetworkManager(mockCloud)
	resp, err := meshManager.Create(ctx, &CreateInput)

	assert.NotNil(t, err)
	assert.Equal(t, err, errors.New("ERROR"))
	assert.Equal(t, resp.ServiceNetworkARN, "")
	assert.Equal(t, resp.ServiceNetworkID, "")
}

func Test_DeleteMesh_MeshNotExist(t *testing.T) {
	listServiceNetworkOutput := []*mercury.MeshSummary{}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockMercurySess := mocks.NewMockMercury(c)
	mockCloud := mocks_aws.NewMockCloud(c)
	mockMercurySess.EXPECT().ListMeshesAsList(ctx, gomock.Any()).Return(listServiceNetworkOutput, nil)
	mockCloud.EXPECT().Mercury().Return(mockMercurySess)

	meshManager := NewDefaultServiceNetworkManager(mockCloud)
	err := meshManager.Delete(ctx, "test")

	assert.Nil(t, err)
}

func Test_DeleteMesh_MeshExistsNoAssociation(t *testing.T) {
	arn := "123456789"
	id := "123456789"
	name := "test"
	item := mercury.MeshSummary{
		Arn:  &arn,
		Id:   &id,
		Name: &name,
	}
	listServiceNetworkOutput := []*mercury.MeshSummary{&item}

	statusServiceNetworkVPCOutput := []*mercury.MeshVpcAssociationSummary{}

	deleteMeshOutput := &mercury.DeleteMeshOutput{}
	deleteMeshInout := &mercury.DeleteMeshInput{MeshIdentifier: &id}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockMercurySess := mocks.NewMockMercury(c)
	mockCloud := mocks_aws.NewMockCloud(c)
	mockMercurySess.EXPECT().ListMeshesAsList(ctx, gomock.Any()).Return(listServiceNetworkOutput, nil)
	mockMercurySess.EXPECT().ListMeshVpcAssociationsAsList(ctx, gomock.Any()).Return(statusServiceNetworkVPCOutput, nil)
	mockMercurySess.EXPECT().DeleteMeshWithContext(ctx, deleteMeshInout).Return(deleteMeshOutput, nil)
	mockCloud.EXPECT().Mercury().Return(mockMercurySess).AnyTimes()

	meshManager := NewDefaultServiceNetworkManager(mockCloud)
	err := meshManager.Delete(ctx, "test")

	assert.Nil(t, err)
}

func Test_DeleteMesh_MeshExists_AssociationsWithOtherVPCExists(t *testing.T) {
	arn := "123456789"
	id := "123456789"
	name := "test"
	vpcId := "123456789"
	item := mercury.MeshSummary{
		Arn:  &arn,
		Id:   &id,
		Name: &name,
	}
	listServiceNetworkOutput := []*mercury.MeshSummary{&item}

	meshVpcAssociationSummaryItem := mercury.MeshVpcAssociationSummary{
		VpcId: &vpcId,
	}
	statusServiceNetworkVPCOutput := []*mercury.MeshVpcAssociationSummary{&meshVpcAssociationSummaryItem}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockMercurySess := mocks.NewMockMercury(c)
	mockCloud := mocks_aws.NewMockCloud(c)
	mockMercurySess.EXPECT().ListMeshesAsList(ctx, gomock.Any()).Return(listServiceNetworkOutput, nil)
	mockMercurySess.EXPECT().ListMeshVpcAssociationsAsList(ctx, gomock.Any()).Return(statusServiceNetworkVPCOutput, nil)
	mockCloud.EXPECT().Mercury().Return(mockMercurySess).AnyTimes()

	meshManager := NewDefaultServiceNetworkManager(mockCloud)
	err := meshManager.Delete(ctx, "test")

	assert.Nil(t, err)
}

func Test_DeleteMesh_MeshExistsAssociatedWithVPC_Deleting(t *testing.T) {
	arn := "123456789"
	id := "123456789"
	name := "test"
	itemMesh := mercury.MeshSummary{
		Arn:  &arn,
		Id:   &id,
		Name: &name,
	}
	listServiceNetworkOutput := []*mercury.MeshSummary{&itemMesh}

	associationArn := "123456789"
	associationID := "123456789"
	associationStatus := mercury.MeshVpcAssociationStatusActive
	associationVPCId := config.VpcID
	itemAssociation := mercury.MeshVpcAssociationSummary{
		Arn:      &associationArn,
		Id:       &associationID,
		MeshArn:  &arn,
		MeshId:   &id,
		MeshName: &name,
		Status:   &associationStatus,
		VpcId:    &associationVPCId,
	}
	statusServiceNetworkVPCOutput := []*mercury.MeshVpcAssociationSummary{&itemAssociation}

	deleteInProgressStatus := mercury.MeshVpcAssociationStatusDeleteInProgress
	deleteMeshVpcAssociationOutput := &mercury.DeleteMeshVpcAssociationOutput{Status: &deleteInProgressStatus}
	deleteMeshOutput := &mercury.DeleteMeshOutput{}
	deleteMeshVpcAssociationInput := &mercury.DeleteMeshVpcAssociationInput{MeshVpcAssociationIdentifier: &associationID}
	deleteMeshInput := &mercury.DeleteMeshInput{MeshIdentifier: &id}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockMercurySess := mocks.NewMockMercury(c)
	mockCloud := mocks_aws.NewMockCloud(c)
	mockMercurySess.EXPECT().ListMeshesAsList(ctx, gomock.Any()).Return(listServiceNetworkOutput, nil)
	mockMercurySess.EXPECT().ListMeshVpcAssociationsAsList(ctx, gomock.Any()).Return(statusServiceNetworkVPCOutput, nil)
	mockMercurySess.EXPECT().DeleteMeshVpcAssociationWithContext(ctx, deleteMeshVpcAssociationInput).Return(deleteMeshVpcAssociationOutput, nil)
	mockMercurySess.EXPECT().DeleteMeshWithContext(ctx, deleteMeshInput).Return(deleteMeshOutput, nil)
	mockCloud.EXPECT().Mercury().Return(mockMercurySess).AnyTimes()

	meshManager := NewDefaultServiceNetworkManager(mockCloud)
	err := meshManager.Delete(ctx, "test")

	assert.NotNil(t, err)
	assert.Equal(t, err, errors.New(LATTICE_RETRY))
}

func Test_DeleteMesh_MeshExistsAssociatedWithOtherVPC(t *testing.T) {
	arn := "123456789"
	id := "123456789"
	name := "test"
	itemMesh := mercury.MeshSummary{
		Arn:  &arn,
		Id:   &id,
		Name: &name,
	}
	listServiceNetworkOutput := []*mercury.MeshSummary{&itemMesh}

	associationArn := "123456789"
	associationID := "123456789"
	associationStatus := mercury.MeshVpcAssociationStatusActive
	associationVPCId := "123456789"
	itemAssociation := mercury.MeshVpcAssociationSummary{
		Arn:      &associationArn,
		Id:       &associationID,
		MeshArn:  &arn,
		MeshId:   &id,
		MeshName: &name,
		Status:   &associationStatus,
		VpcId:    &associationVPCId,
	}
	statusServiceNetworkVPCOutput := []*mercury.MeshVpcAssociationSummary{&itemAssociation}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockMercurySess := mocks.NewMockMercury(c)
	mockCloud := mocks_aws.NewMockCloud(c)
	mockMercurySess.EXPECT().ListMeshesAsList(ctx, gomock.Any()).Return(listServiceNetworkOutput, nil)
	mockMercurySess.EXPECT().ListMeshVpcAssociationsAsList(ctx, gomock.Any()).Return(statusServiceNetworkVPCOutput, nil)
	mockCloud.EXPECT().Mercury().Return(mockMercurySess).AnyTimes()

	meshManager := NewDefaultServiceNetworkManager(mockCloud)
	err := meshManager.Delete(ctx, "test")

	assert.Nil(t, err)
}

func Test_ListMesh_MeshExists(t *testing.T) {
	arn := "123456789"
	id := "123456789"
	name1 := "test1"
	itemMesh1 := mercury.MeshSummary{
		Arn:  &arn,
		Id:   &id,
		Name: &name1,
	}
	name2 := "test2"
	itemMesh2 := mercury.MeshSummary{
		Arn:  &arn,
		Id:   &id,
		Name: &name2,
	}
	listServiceNetworkOutput := []*mercury.MeshSummary{&itemMesh1, &itemMesh2}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockMercurySess := mocks.NewMockMercury(c)
	mockCloud := mocks_aws.NewMockCloud(c)
	mockMercurySess.EXPECT().ListMeshesAsList(ctx, gomock.Any()).Return(listServiceNetworkOutput, nil)
	mockCloud.EXPECT().Mercury().Return(mockMercurySess)

	meshManager := NewDefaultServiceNetworkManager(mockCloud)
	meshList, err := meshManager.List(ctx)

	assert.Nil(t, err)
	assert.Equal(t, meshList, []string{"test1", "test2"})
}

func Test_ListMesh_NoMesh(t *testing.T) {
	listServiceNetworkOutput := []*mercury.MeshSummary{}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockMercurySess := mocks.NewMockMercury(c)
	mockCloud := mocks_aws.NewMockCloud(c)
	mockMercurySess.EXPECT().ListMeshesAsList(ctx, gomock.Any()).Return(listServiceNetworkOutput, nil)
	mockCloud.EXPECT().Mercury().Return(mockMercurySess)

	meshManager := NewDefaultServiceNetworkManager(mockCloud)
	meshList, err := meshManager.List(ctx)

	assert.Nil(t, err)
	assert.Equal(t, meshList, []string{})
}
