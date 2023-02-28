package lattice

import (
	"context"
	"errors"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	"testing"

	mocks_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	mocks "github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

// ServiceNetwork does not exist before,happy case.
func Test_CreateServiceNetwork_MeshNotExist_NoAssociation(t *testing.T) {
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
	meshCreateOutput := &vpclattice.CreateServiceNetworkOutput{
		Arn:  &arn,
		Id:   &id,
		Name: &name,
	}
	listServiceNetworkOutput := []*vpclattice.ServiceNetworkSummary{}

	createServiceNetworkInput := &vpclattice.CreateServiceNetworkInput{
		Name: &name,
		Tags: make(map[string]*string),
	}
	createServiceNetworkInput.Tags[latticemodel.K8SServiceNetworkOwnedByVPC] = &config.VpcID

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockCloud := mocks_aws.NewMockCloud(c)
	mockVpcLatticeSess := mocks.NewMockLattice(c)
	mockVpcLatticeSess.EXPECT().ListServiceNetworksAsList(ctx, gomock.Any()).Return(listServiceNetworkOutput, nil)
	mockVpcLatticeSess.EXPECT().CreateServiceNetworkWithContext(ctx, createServiceNetworkInput).Return(meshCreateOutput, nil)
	mockCloud.EXPECT().Lattice().Return(mockVpcLatticeSess).AnyTimes()

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
	item := vpclattice.ServiceNetworkSummary{
		Arn:  &arn,
		Id:   &id,
		Name: &name,
	}
	listServiceNetworkOutput := []*vpclattice.ServiceNetworkSummary{&item}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockVpcLatticeSess := mocks.NewMockLattice(c)
	mockCloud := mocks_aws.NewMockCloud(c)
	mockVpcLatticeSess.EXPECT().ListServiceNetworksAsList(ctx, gomock.Any()).Return(listServiceNetworkOutput, errors.New("ERROR"))
	mockCloud.EXPECT().Lattice().Return(mockVpcLatticeSess)

	meshManager := NewDefaultServiceNetworkManager(mockCloud)
	resp, err := meshManager.Create(ctx, &meshCreateInput)

	assert.NotNil(t, err)
	assert.Equal(t, err, errors.New("ERROR"))
	assert.Equal(t, resp.ServiceNetworkARN, "")
	assert.Equal(t, resp.ServiceNetworkID, "")
}

// ServiceNetwork already exists, association is in ServiceNetworkVpcAssociationStatusCreateInProgress.

func Test_CreateServiceNetwork_MeshAlreadyExist_ServiceNetworkVpcAssociationStatusCreateInProgress(t *testing.T) {
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
	item := vpclattice.ServiceNetworkSummary{
		Arn:  &meshArn,
		Id:   &meshId,
		Name: &name,
	}
	listServiceNetworkOutput := []*vpclattice.ServiceNetworkSummary{&item}

	status := vpclattice.ServiceNetworkVpcAssociationStatusCreateInProgress
	items := vpclattice.ServiceNetworkVpcAssociationSummary{
		ServiceNetworkArn:  &meshArn,
		ServiceNetworkId:   &meshId,
		ServiceNetworkName: &meshId,
		Status:             &status,
		VpcId:              &vpcId,
	}
	statusServiceNetworkVPCOutput := []*vpclattice.ServiceNetworkVpcAssociationSummary{&items}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockVpcLatticeSess := mocks.NewMockLattice(c)
	mockCloud := mocks_aws.NewMockCloud(c)
	mockVpcLatticeSess.EXPECT().ListServiceNetworksAsList(ctx, gomock.Any()).Return(listServiceNetworkOutput, nil)
	mockVpcLatticeSess.EXPECT().ListServiceNetworkVpcAssociationsAsList(ctx, gomock.Any()).Return(statusServiceNetworkVPCOutput, nil)
	mockCloud.EXPECT().Lattice().Return(mockVpcLatticeSess).AnyTimes()

	meshManager := NewDefaultServiceNetworkManager(mockCloud)
	resp, err := meshManager.Create(ctx, &meshCreateInput)

	assert.NotNil(t, err)
	assert.Equal(t, err, errors.New(LATTICE_RETRY))
	assert.Equal(t, resp.ServiceNetworkARN, "")
	assert.Equal(t, resp.ServiceNetworkID, "")
}

// ServiceNetwork already exists, association is in ServiceNetworkVpcAssociationStatusDeleteInProgress.

func Test_CreateServiceNetwork_MeshAlreadyExist_ServiceNetworkVpcAssociationStatusDeleteInProgress(t *testing.T) {
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
	item := vpclattice.ServiceNetworkSummary{
		Arn:  &meshArn,
		Id:   &meshId,
		Name: &name,
	}
	listServiceNetworkOutput := []*vpclattice.ServiceNetworkSummary{&item}

	status := vpclattice.ServiceNetworkVpcAssociationStatusDeleteInProgress
	items := vpclattice.ServiceNetworkVpcAssociationSummary{
		ServiceNetworkArn:  &meshArn,
		ServiceNetworkId:   &meshId,
		ServiceNetworkName: &meshId,
		Status:             &status,
		VpcId:              &vpcId,
	}
	statusServiceNetworkVPCOutput := []*vpclattice.ServiceNetworkVpcAssociationSummary{&items}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockVpcLatticeSess := mocks.NewMockLattice(c)
	mockCloud := mocks_aws.NewMockCloud(c)
	mockVpcLatticeSess.EXPECT().ListServiceNetworksAsList(ctx, gomock.Any()).Return(listServiceNetworkOutput, nil)
	mockVpcLatticeSess.EXPECT().ListServiceNetworkVpcAssociationsAsList(ctx, gomock.Any()).Return(statusServiceNetworkVPCOutput, nil)
	mockCloud.EXPECT().Lattice().Return(mockVpcLatticeSess).AnyTimes()

	meshManager := NewDefaultServiceNetworkManager(mockCloud)
	resp, err := meshManager.Create(ctx, &meshCreateInput)

	assert.NotNil(t, err)
	assert.Equal(t, err, errors.New(LATTICE_RETRY))
	assert.Equal(t, resp.ServiceNetworkARN, "")
	assert.Equal(t, resp.ServiceNetworkID, "")
}

// ServiceNetwork already exists, association is ServiceNetworkVpcAssociationStatusActive.
func Test_CreateServiceNetwork_MeshAlreadyExist_ServiceNetworkVpcAssociationStatusActive(t *testing.T) {
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
	item := vpclattice.ServiceNetworkSummary{
		Arn:  &meshArn,
		Id:   &meshId,
		Name: &name,
	}
	listServiceNetworkOutput := []*vpclattice.ServiceNetworkSummary{&item}

	status := vpclattice.ServiceNetworkVpcAssociationStatusActive
	items := vpclattice.ServiceNetworkVpcAssociationSummary{
		ServiceNetworkArn:  &meshArn,
		ServiceNetworkId:   &meshId,
		ServiceNetworkName: &meshId,
		Status:             &status,
		VpcId:              &config.VpcID,
	}
	statusServiceNetworkVPCOutput := []*vpclattice.ServiceNetworkVpcAssociationSummary{&items}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockVpcLatticeSess := mocks.NewMockLattice(c)
	mockCloud := mocks_aws.NewMockCloud(c)
	mockVpcLatticeSess.EXPECT().ListServiceNetworksAsList(ctx, gomock.Any()).Return(listServiceNetworkOutput, nil)
	mockVpcLatticeSess.EXPECT().ListServiceNetworkVpcAssociationsAsList(ctx, gomock.Any()).Return(statusServiceNetworkVPCOutput, nil)
	mockCloud.EXPECT().Lattice().Return(mockVpcLatticeSess).AnyTimes()

	meshManager := NewDefaultServiceNetworkManager(mockCloud)
	resp, err := meshManager.Create(ctx, &meshCreateInput)

	assert.Nil(t, err)
	assert.Equal(t, resp.ServiceNetworkARN, meshArn)
	assert.Equal(t, resp.ServiceNetworkID, meshId)
}

// ServiceNetwork already exists, association is ServiceNetworkVpcAssociationStatusCreateFailed.
/* TODO for association
func Test_CreateServiceNetwork_MeshAlreadyExist_ServiceNetworkVpcAssociationStatusCreateFailed(t *testing.T) {
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
	item := vpclattice.ServiceNetworkSummary{
		Arn:  &meshArn,
		Id:   &meshId,
		Name: &name,
	}
	listServiceNetworkOutput := []*vpclattice.ServiceNetworkSummary{&item}

	status := vpclattice.ServiceNetworkVpcAssociationStatusCreateFailed
	items := vpclattice.ServiceNetworkVpcAssociationSummary{
		ServiceNetworkArn:  &meshArn,
		ServiceNetworkId:   &meshId,
		ServiceNetworkName: &meshId,
		Status:             &status,
		VpcId:              &config.VpcID,
	}
	statusServiceNetworkVPCOutput := []*vpclattice.ServiceNetworkVpcAssociationSummary{&items}

	associationStatus := vpclattice.ServiceNetworkVpcAssociationStatusActive
	createServiceNetworkVPCAssociationOutput := &vpclattice.CreateServiceNetworkVpcAssociationOutput{
		Status: &associationStatus,
	}
	createServiceNetworkVpcAssociationInput := &vpclattice.CreateServiceNetworkVpcAssociationInput{
		ServiceNetworkIdentifier: &meshId,
		VpcIdentifier:            &config.VpcID,
	}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockVpcLatticeSess := mocks.NewMockLattice(c)
	mockCloud := mocks_aws.NewMockCloud(c)
	mockVpcLatticeSess.EXPECT().ListServiceNetworksAsList(ctx, gomock.Any()).Return(listServiceNetworkOutput, nil)
	mockVpcLatticeSess.EXPECT().ListServiceNetworkVpcAssociationsAsList(ctx, gomock.Any()).Return(statusServiceNetworkVPCOutput, nil)
	mockVpcLatticeSess.EXPECT().CreateServiceNetworkVpcAssociationWithContext(ctx, createServiceNetworkVpcAssociationInput).Return(createServiceNetworkVPCAssociationOutput, nil)
	mockCloud.EXPECT().Lattice().Return(mockVpcLatticeSess).AnyTimes()

	meshManager := NewDefaultServiceNetworkManager(mockCloud)
	resp, err := meshManager.Create(ctx, &meshCreateInput)

	assert.Nil(t, err)
	assert.Equal(t, resp.ServiceNetworkARN, meshArn)
	assert.Equal(t, resp.ServiceNetworkID, meshId)
}
*/

// ServiceNetwork already exists, associated with other VPC
/* TODO for association test
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
	item := vpclattice.ServiceNetworkSummary{
		Arn:  &meshArn,
		Id:   &meshId,
		Name: &name,
	}
	listServiceNetworkOutput := []*vpclattice.ServiceNetworkSummary{&item}

	status := vpclattice.ServiceNetworkVpcAssociationStatusCreateFailed
	items := vpclattice.ServiceNetworkVpcAssociationSummary{
		ServiceNetworkArn:  &meshArn,
		ServiceNetworkId:   &meshId,
		ServiceNetworkName: &meshId,
		Status:             &status,
		VpcId:              &vpcId,
	}
	statusServiceNetworkVPCOutput := []*vpclattice.ServiceNetworkVpcAssociationSummary{&items}

	associationStatus := vpclattice.ServiceNetworkVpcAssociationStatusActive
	createServiceNetworkVPCAssociationOutput := &vpclattice.CreateServiceNetworkVpcAssociationOutput{
		Status: &associationStatus,
	}
	createServiceNetworkVpcAssociationInput := &vpclattice.CreateServiceNetworkVpcAssociationInput{
		ServiceNetworkIdentifier: &meshId,
		VpcIdentifier:            &config.VpcID,
	}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockVpcLatticeSess := mocks.NewMockLattice(c)
	mockCloud := mocks_aws.NewMockCloud(c)
	mockVpcLatticeSess.EXPECT().ListServiceNetworksAsList(ctx, gomock.Any()).Return(listServiceNetworkOutput, nil)
	mockVpcLatticeSess.EXPECT().ListServiceNetworkVpcAssociationsAsList(ctx, gomock.Any()).Return(statusServiceNetworkVPCOutput, nil)
	mockVpcLatticeSess.EXPECT().CreateServiceNetworkVpcAssociationWithContext(ctx, createServiceNetworkVpcAssociationInput).Return(createServiceNetworkVPCAssociationOutput, nil)
	mockCloud.EXPECT().Lattice().Return(mockVpcLatticeSess).AnyTimes()

	meshManager := NewDefaultServiceNetworkManager(mockCloud)
	resp, err := meshManager.Create(ctx, &meshCreateInput)

	assert.Nil(t, err)
	assert.Equal(t, resp.ServiceNetworkARN, meshArn)
	assert.Equal(t, resp.ServiceNetworkID, meshId)
}
*/

// ServiceNetwork does not exists, association is ServiceNetworkVpcAssociationStatusFailed.
/* TODO test for association
func Test_CreateServiceNetwork_MeshNotExist_ServiceNetworkVpcAssociationStatusFailed(t *testing.T) {
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

	listServiceNetworkOutput := []*vpclattice.ServiceNetworkSummary{}
	associationStatus := vpclattice.ServiceNetworkVpcAssociationStatusCreateFailed
	createServiceNetworkVPCAssociationOutput := &vpclattice.CreateServiceNetworkVpcAssociationOutput{
		Status: &associationStatus,
	}
	meshCreateOutput := &vpclattice.CreateServiceNetworkOutput{
		Arn:  &meshArn,
		Id:   &meshId,
		Name: &name,
	}
	meshCreateInput := &vpclattice.CreateServiceNetworkInput{
		Name: &name,
	}
	createServiceNetworkVpcAssociationInput := &vpclattice.CreateServiceNetworkVpcAssociationInput{
		ServiceNetworkIdentifier: &meshId,
		VpcIdentifier:            &config.VpcID,
	}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockVpcLatticeSess := mocks.NewMockLattice(c)
	mockCloud := mocks_aws.NewMockCloud(c)
	mockVpcLatticeSess.EXPECT().ListServiceNetworksAsList(ctx, gomock.Any()).Return(listServiceNetworkOutput, nil)
	mockVpcLatticeSess.EXPECT().CreateServiceNetworkWithContext(ctx, meshCreateInput).Return(meshCreateOutput, nil)
	mockVpcLatticeSess.EXPECT().CreateServiceNetworkVpcAssociationWithContext(ctx, createServiceNetworkVpcAssociationInput).Return(createServiceNetworkVPCAssociationOutput, nil)
	mockCloud.EXPECT().Lattice().Return(mockVpcLatticeSess).AnyTimes()

	meshManager := NewDefaultServiceNetworkManager(mockCloud)
	resp, err := meshManager.Create(ctx, &CreateInput)

	assert.NotNil(t, err)
	assert.Equal(t, err, errors.New(LATTICE_RETRY))
	assert.Equal(t, resp.ServiceNetworkARN, "")
	assert.Equal(t, resp.ServiceNetworkID, "")
}
*/

// ServiceNetwork does not exists, association is ServiceNetworkVpcAssociationStatusCreateInProgress.
/* TODO for assocition
func Test_CreateServiceNetwork_MeshNOTExist_ServiceNetworkVpcAssociationStatusCreateInProgress(t *testing.T) {
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
	listServiceNetworkOutput := []*vpclattice.ServiceNetworkSummary{}
	associationStatus := vpclattice.ServiceNetworkVpcAssociationStatusCreateInProgress
	createServiceNetworkVPCAssociationOutput := &vpclattice.CreateServiceNetworkVpcAssociationOutput{
		Status: &associationStatus,
	}
	meshCreateOutput := &vpclattice.CreateServiceNetworkOutput{
		Arn:  &meshArn,
		Id:   &meshId,
		Name: &name,
	}
	meshCreateInput := &vpclattice.CreateServiceNetworkInput{
		Name: &name,
	}
	createServiceNetworkVpcAssociationInput := &vpclattice.CreateServiceNetworkVpcAssociationInput{
		ServiceNetworkIdentifier: &meshId,
		VpcIdentifier:            &config.VpcID,
	}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockVpcLatticeSess := mocks.NewMockLattice(c)
	mockCloud := mocks_aws.NewMockCloud(c)
	mockVpcLatticeSess.EXPECT().ListServiceNetworksAsList(ctx, gomock.Any()).Return(listServiceNetworkOutput, nil)
	mockVpcLatticeSess.EXPECT().CreateServiceNetworkWithContext(ctx, meshCreateInput).Return(meshCreateOutput, nil)
	mockVpcLatticeSess.EXPECT().CreateServiceNetworkVpcAssociationWithContext(ctx, createServiceNetworkVpcAssociationInput).Return(createServiceNetworkVPCAssociationOutput, nil)
	mockCloud.EXPECT().Lattice().Return(mockVpcLatticeSess).AnyTimes()

	meshManager := NewDefaultServiceNetworkManager(mockCloud)
	resp, err := meshManager.Create(ctx, &CreateInput)

	assert.NotNil(t, err)
	assert.Equal(t, err, errors.New(LATTICE_RETRY))
	assert.Equal(t, resp.ServiceNetworkARN, "")
	assert.Equal(t, resp.ServiceNetworkID, "")
}
*/

// ServiceNetwork does not exists, association is ServiceNetworkVpcAssociationStatusDeleteInProgress.
/* TODO for association
func Test_CreateServiceNetwork_MeshNotExist_ServiceNetworkVpcAssociationStatusDeleteInProgress(t *testing.T) {
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
	listServiceNetworkOutput := []*vpclattice.ServiceNetworkSummary{}
	associationStatus := vpclattice.ServiceNetworkVpcAssociationStatusDeleteInProgress
	createServiceNetworkVPCAssociationOutput := &vpclattice.CreateServiceNetworkVpcAssociationOutput{
		Status: &associationStatus,
	}
	meshCreateOutput := &vpclattice.CreateServiceNetworkOutput{
		Arn:  &meshArn,
		Id:   &meshId,
		Name: &name,
	}
	meshCreateInput := &vpclattice.CreateServiceNetworkInput{
		Name: &name,
	}
	createServiceNetworkVpcAssociationInput := &vpclattice.CreateServiceNetworkVpcAssociationInput{
		ServiceNetworkIdentifier: &meshId,
		VpcIdentifier:            &config.VpcID,
	}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockVpcLatticeSess := mocks.NewMockLattice(c)
	mockCloud := mocks_aws.NewMockCloud(c)
	mockVpcLatticeSess.EXPECT().ListServiceNetworksAsList(ctx, gomock.Any()).Return(listServiceNetworkOutput, nil)
	mockVpcLatticeSess.EXPECT().CreateServiceNetworkWithContext(ctx, meshCreateInput).Return(meshCreateOutput, nil)
	mockVpcLatticeSess.EXPECT().CreateServiceNetworkVpcAssociationWithContext(ctx, createServiceNetworkVpcAssociationInput).Return(createServiceNetworkVPCAssociationOutput, nil)
	mockCloud.EXPECT().Lattice().Return(mockVpcLatticeSess).AnyTimes()

	meshManager := NewDefaultServiceNetworkManager(mockCloud)
	resp, err := meshManager.Create(ctx, &CreateInput)

	assert.NotNil(t, err)
	assert.Equal(t, err, errors.New(LATTICE_RETRY))
	assert.Equal(t, resp.ServiceNetworkARN, "")
	assert.Equal(t, resp.ServiceNetworkID, "")
}
*/

// ServiceNetwork does not exists, association returns Error.
/* TODO for association
func Test_CreateServiceNetwork_MeshNotExist_ServiceNetworkVpcAssociationReturnsError(t *testing.T) {
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
	listServiceNetworkOutput := []*vpclattice.ServiceNetworkSummary{}
	createServiceNetworkVPCAssociationOutput := &vpclattice.CreateServiceNetworkVpcAssociationOutput{}
	meshCreateOutput := &vpclattice.CreateServiceNetworkOutput{
		Arn:  &meshArn,
		Id:   &meshId,
		Name: &name,
	}
	meshCreateInput := &vpclattice.CreateServiceNetworkInput{
		Name: &name,
	}
	createServiceNetworkVpcAssociationInput := &vpclattice.CreateServiceNetworkVpcAssociationInput{
		ServiceNetworkIdentifier: &meshId,
		VpcIdentifier:            &config.VpcID,
	}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockVpcLatticeSess := mocks.NewMockLattice(c)
	mockCloud := mocks_aws.NewMockCloud(c)
	mockVpcLatticeSess.EXPECT().ListServiceNetworksAsList(ctx, gomock.Any()).Return(listServiceNetworkOutput, nil)
	mockVpcLatticeSess.EXPECT().CreateServiceNetworkWithContext(ctx, meshCreateInput).Return(meshCreateOutput, nil)
	mockVpcLatticeSess.EXPECT().CreateServiceNetworkVpcAssociationWithContext(ctx, createServiceNetworkVpcAssociationInput).Return(createServiceNetworkVPCAssociationOutput, errors.New("ERROR"))
	mockCloud.EXPECT().Lattice().Return(mockVpcLatticeSess).AnyTimes()

	meshManager := NewDefaultServiceNetworkManager(mockCloud)
	resp, err := meshManager.Create(ctx, &CreateInput)

	assert.NotNil(t, err)
	assert.Equal(t, err, errors.New("ERROR"))
	assert.Equal(t, resp.ServiceNetworkARN, "")
	assert.Equal(t, resp.ServiceNetworkID, "")
}
*/

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
	meshCreateOutput := &vpclattice.CreateServiceNetworkOutput{
		Arn:  &arn,
		Id:   &id,
		Name: &name,
	}
	listServiceNetworkOutput := []*vpclattice.ServiceNetworkSummary{}
	meshCreateInput := &vpclattice.CreateServiceNetworkInput{
		Name: &name,
		Tags: make(map[string]*string),
	}

	meshCreateInput.Tags[latticemodel.K8SServiceNetworkOwnedByVPC] = &config.VpcID

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockVpcLatticeSess := mocks.NewMockLattice(c)
	mockCloud := mocks_aws.NewMockCloud(c)
	mockVpcLatticeSess.EXPECT().ListServiceNetworksAsList(ctx, gomock.Any()).Return(listServiceNetworkOutput, nil)
	mockVpcLatticeSess.EXPECT().CreateServiceNetworkWithContext(ctx, meshCreateInput).Return(meshCreateOutput, errors.New("ERROR"))
	mockCloud.EXPECT().Lattice().Return(mockVpcLatticeSess).AnyTimes()

	meshManager := NewDefaultServiceNetworkManager(mockCloud)
	resp, err := meshManager.Create(ctx, &CreateInput)

	assert.NotNil(t, err)
	assert.Equal(t, err, errors.New("ERROR"))
	assert.Equal(t, resp.ServiceNetworkARN, "")
	assert.Equal(t, resp.ServiceNetworkID, "")
}

func Test_DeleteMesh_MeshNotExist(t *testing.T) {
	listServiceNetworkOutput := []*vpclattice.ServiceNetworkSummary{}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockVpcLatticeSess := mocks.NewMockLattice(c)
	mockCloud := mocks_aws.NewMockCloud(c)
	mockVpcLatticeSess.EXPECT().ListServiceNetworksAsList(ctx, gomock.Any()).Return(listServiceNetworkOutput, nil)
	mockCloud.EXPECT().Lattice().Return(mockVpcLatticeSess)

	meshManager := NewDefaultServiceNetworkManager(mockCloud)
	err := meshManager.Delete(ctx, "test")

	assert.Nil(t, err)
}

func Test_DeleteMesh_MeshExistsNoAssociation(t *testing.T) {
	arn := "123456789"
	id := "123456789"
	name := "test"
	item := vpclattice.ServiceNetworkSummary{
		Arn:  &arn,
		Id:   &id,
		Name: &name,
	}
	listServiceNetworkOutput := []*vpclattice.ServiceNetworkSummary{&item}

	statusServiceNetworkVPCOutput := []*vpclattice.ServiceNetworkVpcAssociationSummary{}

	deleteMeshOutput := &vpclattice.DeleteServiceNetworkOutput{}
	deleteMeshInout := &vpclattice.DeleteServiceNetworkInput{ServiceNetworkIdentifier: &id}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockVpcLatticeSess := mocks.NewMockLattice(c)
	mockCloud := mocks_aws.NewMockCloud(c)
	mockVpcLatticeSess.EXPECT().ListServiceNetworksAsList(ctx, gomock.Any()).Return(listServiceNetworkOutput, nil)
	mockVpcLatticeSess.EXPECT().ListServiceNetworkVpcAssociationsAsList(ctx, gomock.Any()).Return(statusServiceNetworkVPCOutput, nil)
	mockVpcLatticeSess.EXPECT().DeleteServiceNetworkWithContext(ctx, deleteMeshInout).Return(deleteMeshOutput, nil)
	mockCloud.EXPECT().Lattice().Return(mockVpcLatticeSess).AnyTimes()

	meshManager := NewDefaultServiceNetworkManager(mockCloud)
	err := meshManager.Delete(ctx, "test")

	assert.Nil(t, err)
}

func Test_DeleteMesh_MeshExists_AssociationsWithOtherVPCExists(t *testing.T) {
	arn := "123456789"
	id := "123456789"
	name := "test"
	vpcId := "123456789"
	item := vpclattice.ServiceNetworkSummary{
		Arn:  &arn,
		Id:   &id,
		Name: &name,
	}
	listServiceNetworkOutput := []*vpclattice.ServiceNetworkSummary{&item}

	ServiceNetworkVpcAssociationSummaryItem := vpclattice.ServiceNetworkVpcAssociationSummary{
		VpcId: &vpcId,
	}
	statusServiceNetworkVPCOutput := []*vpclattice.ServiceNetworkVpcAssociationSummary{&ServiceNetworkVpcAssociationSummaryItem}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockVpcLatticeSess := mocks.NewMockLattice(c)
	mockCloud := mocks_aws.NewMockCloud(c)
	mockVpcLatticeSess.EXPECT().ListServiceNetworksAsList(ctx, gomock.Any()).Return(listServiceNetworkOutput, nil)
	mockVpcLatticeSess.EXPECT().ListServiceNetworkVpcAssociationsAsList(ctx, gomock.Any()).Return(statusServiceNetworkVPCOutput, nil)
	mockCloud.EXPECT().Lattice().Return(mockVpcLatticeSess).AnyTimes()

	meshManager := NewDefaultServiceNetworkManager(mockCloud)
	err := meshManager.Delete(ctx, "test")

	assert.Nil(t, err)
}

func Test_DeleteMesh_MeshExistsAssociatedWithVPC_Deleting(t *testing.T) {
	arn := "123456789"
	id := "123456789"
	name := "test"
	itemMesh := vpclattice.ServiceNetworkSummary{
		Arn:  &arn,
		Id:   &id,
		Name: &name,
	}
	listServiceNetworkOutput := []*vpclattice.ServiceNetworkSummary{&itemMesh}

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
	deleteMeshOutput := &vpclattice.DeleteServiceNetworkOutput{}
	deleteServiceNetworkVpcAssociationInput := &vpclattice.DeleteServiceNetworkVpcAssociationInput{ServiceNetworkVpcAssociationIdentifier: &associationID}
	deleteMeshInput := &vpclattice.DeleteServiceNetworkInput{ServiceNetworkIdentifier: &id}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockVpcLatticeSess := mocks.NewMockLattice(c)
	mockCloud := mocks_aws.NewMockCloud(c)
	mockVpcLatticeSess.EXPECT().ListServiceNetworksAsList(ctx, gomock.Any()).Return(listServiceNetworkOutput, nil)
	mockVpcLatticeSess.EXPECT().ListServiceNetworkVpcAssociationsAsList(ctx, gomock.Any()).Return(statusServiceNetworkVPCOutput, nil)
	mockVpcLatticeSess.EXPECT().DeleteServiceNetworkVpcAssociationWithContext(ctx, deleteServiceNetworkVpcAssociationInput).Return(deleteServiceNetworkVpcAssociationOutput, nil)
	mockVpcLatticeSess.EXPECT().DeleteServiceNetworkWithContext(ctx, deleteMeshInput).Return(deleteMeshOutput, nil)
	mockCloud.EXPECT().Lattice().Return(mockVpcLatticeSess).AnyTimes()

	meshManager := NewDefaultServiceNetworkManager(mockCloud)
	err := meshManager.Delete(ctx, "test")

	assert.NotNil(t, err)
	assert.Equal(t, err, errors.New(LATTICE_RETRY))
}

func Test_DeleteMesh_MeshExistsAssociatedWithOtherVPC(t *testing.T) {
	arn := "123456789"
	id := "123456789"
	name := "test"
	itemMesh := vpclattice.ServiceNetworkSummary{
		Arn:  &arn,
		Id:   &id,
		Name: &name,
	}
	listServiceNetworkOutput := []*vpclattice.ServiceNetworkSummary{&itemMesh}

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
	mockVpcLatticeSess := mocks.NewMockLattice(c)
	mockCloud := mocks_aws.NewMockCloud(c)
	mockVpcLatticeSess.EXPECT().ListServiceNetworksAsList(ctx, gomock.Any()).Return(listServiceNetworkOutput, nil)
	mockVpcLatticeSess.EXPECT().ListServiceNetworkVpcAssociationsAsList(ctx, gomock.Any()).Return(statusServiceNetworkVPCOutput, nil)
	mockCloud.EXPECT().Lattice().Return(mockVpcLatticeSess).AnyTimes()

	meshManager := NewDefaultServiceNetworkManager(mockCloud)
	err := meshManager.Delete(ctx, "test")

	assert.Nil(t, err)
}

func Test_ListMesh_MeshExists(t *testing.T) {
	arn := "123456789"
	id := "123456789"
	name1 := "test1"
	itemMesh1 := vpclattice.ServiceNetworkSummary{
		Arn:  &arn,
		Id:   &id,
		Name: &name1,
	}
	name2 := "test2"
	itemMesh2 := vpclattice.ServiceNetworkSummary{
		Arn:  &arn,
		Id:   &id,
		Name: &name2,
	}
	listServiceNetworkOutput := []*vpclattice.ServiceNetworkSummary{&itemMesh1, &itemMesh2}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockVpcLatticeSess := mocks.NewMockLattice(c)
	mockCloud := mocks_aws.NewMockCloud(c)
	mockVpcLatticeSess.EXPECT().ListServiceNetworksAsList(ctx, gomock.Any()).Return(listServiceNetworkOutput, nil)
	mockCloud.EXPECT().Lattice().Return(mockVpcLatticeSess)

	meshManager := NewDefaultServiceNetworkManager(mockCloud)
	meshList, err := meshManager.List(ctx)

	assert.Nil(t, err)
	assert.Equal(t, meshList, []string{"test1", "test2"})
}

func Test_ListMesh_NoMesh(t *testing.T) {
	listServiceNetworkOutput := []*vpclattice.ServiceNetworkSummary{}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockVpcLatticeSess := mocks.NewMockLattice(c)
	mockCloud := mocks_aws.NewMockCloud(c)
	mockVpcLatticeSess.EXPECT().ListServiceNetworksAsList(ctx, gomock.Any()).Return(listServiceNetworkOutput, nil)
	mockCloud.EXPECT().Lattice().Return(mockVpcLatticeSess)

	meshManager := NewDefaultServiceNetworkManager(mockCloud)
	meshList, err := meshManager.List(ctx)

	assert.Nil(t, err)
	assert.Equal(t, meshList, []string{})
}
