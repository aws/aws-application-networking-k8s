package lattice

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/aws"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/aws/aws-sdk-go/service/vpclattice"

	mocks_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	mocks "github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

func Test_Create_ValidateService(t *testing.T) {
	tests := []struct {
		name                             string
		tags                             map[string]*string
		meshName                         string
		meshId                           string
		meshArn                          string
		wantServiceName                  string
		wantServiceId                    string
		wantServiceArn                   string
		wantMeshServiceAssociationStatus string
		wantErr                          error
		wantListServiceOutput            []*vpclattice.ServiceSummary
		listServiceInput                 *vpclattice.ListServicesInput
	}{
		{
			name:                             "Test_Create_ValidateService",
			tags:                             nil,
			meshName:                         "test-mesh-1",
			meshId:                           "id-234567890",
			meshArn:                          "arn-234567890",
			wantServiceName:                  "svc-test-1",
			wantServiceId:                    "id-123456789",
			wantServiceArn:                   "arn-123456789",
			wantMeshServiceAssociationStatus: vpclattice.ServiceNetworkServiceAssociationStatusActive,
			wantErr:                          nil,
			wantListServiceOutput:            []*vpclattice.ServiceSummary{},
			listServiceInput:                 &vpclattice.ListServicesInput{},
		},
	}

	for _, tt := range tests {
		fmt.Printf("Testing >>>>> %v\n", tt.name)
		c := gomock.NewController(t)
		defer c.Finish()
		ctx := context.TODO()
		mockVpcLatticeSess := mocks.NewMockLattice(c)
		latticeDataStore := latticestore.NewLatticeDataStore()
		mockCloud := mocks_aws.NewMockCloud(c)
		mockCloud.EXPECT().GetAccountID().Return("123456789012").AnyTimes()
		mockCloud.EXPECT().GetVpcID().Return("").AnyTimes()
		mockCloud.EXPECT().GetServiceNetworkName().Return(tt.meshId).AnyTimes()

		latticeDataStore.AddServiceNetwork(tt.meshName, mockCloud.GetAccountID(), tt.meshArn, tt.meshId, latticestore.DATASTORE_SERVICE_NETWORK_CREATED)

		SVCName := latticestore.LatticeServiceName(tt.wantServiceName, "default")
		createServiceOutput := &vpclattice.CreateServiceOutput{
			Arn:    &tt.wantServiceArn,
			Id:     &tt.wantServiceId,
			Name:   &SVCName,
			Status: aws.String(vpclattice.ServiceStatusActive),
		}

		createServiceNetworkServiceAssociationOutput := &vpclattice.CreateServiceNetworkServiceAssociationOutput{
			Arn:      nil,
			DnsEntry: nil,
			Id:       nil,
			Status:   &tt.wantMeshServiceAssociationStatus,
		}

		input := &latticemodel.Service{
			Spec: latticemodel.ServiceSpec{
				Name:                tt.wantServiceName,
				Namespace:           "default",
				Protocols:           []*string{aws.String("http")},
				ServiceNetworkNames: []string{tt.meshName},
			},
			Status: &latticemodel.ServiceStatus{ServiceARN: "", ServiceID: ""},
		}

		createServiceInput := &vpclattice.CreateServiceInput{
			Name: &SVCName,
			Tags: make(map[string]*string),
		}
		vpcId := mockCloud.GetVpcID()
		createServiceInput.Tags[latticemodel.K8SServiceOwnedByVPC] = &vpcId
		associateMeshService := &vpclattice.CreateServiceNetworkServiceAssociationInput{
			ServiceNetworkIdentifier: &tt.meshId,
			ServiceIdentifier:        &tt.wantServiceId,
		}

		mockVpcLatticeSess.EXPECT().ListServicesAsList(ctx, tt.listServiceInput).Return(tt.wantListServiceOutput, nil)
		mockVpcLatticeSess.EXPECT().CreateServiceWithContext(ctx, createServiceInput).Return(createServiceOutput, nil)

		mockVpcLatticeSess.EXPECT().ListServiceNetworkServiceAssociationsAsList(ctx, gomock.Any())
		mockVpcLatticeSess.EXPECT().ListServiceNetworkServiceAssociationsAsList(ctx, gomock.Any())
		mockVpcLatticeSess.EXPECT().CreateServiceNetworkServiceAssociationWithContext(ctx, associateMeshService).Return(createServiceNetworkServiceAssociationOutput, tt.wantErr)
		mockCloud.EXPECT().Lattice().Return(mockVpcLatticeSess).AnyTimes()

		serviceManager := NewServiceManager(mockCloud, latticeDataStore)
		resp, err := serviceManager.Create(ctx, input)

		if tt.wantErr != nil {
			assert.NotNil(t, err)
			assert.Equal(t, err, errors.New(LATTICE_RETRY))
			assert.Equal(t, resp.ServiceARN, "")
			assert.Equal(t, resp.ServiceID, "")
		} else {
			assert.Nil(t, err)
			assert.Equal(t, resp.ServiceARN, tt.wantServiceArn)
			assert.Equal(t, resp.ServiceID, tt.wantServiceId)
		}
	}
}

func Test_Create_CreateService_MeshServiceAssociation(t *testing.T) {
	tests := []struct {
		name                             string
		tags                             map[string]*string
		meshName                         string
		meshId                           string
		meshArn                          string
		wantServiceName                  string
		wantServiceId                    string
		wantServiceArn                   string
		wantMeshServiceAssociationStatus string
		wantErr                          error
		wantListServiceOutput            []*vpclattice.ServiceSummary
	}{
		{
			name:                             "test-1",
			tags:                             nil,
			meshName:                         "test-mesh-1",
			meshId:                           "id-234567890",
			meshArn:                          "arn-234567890",
			wantServiceName:                  "svc-test-1",
			wantServiceId:                    "id-123456789",
			wantServiceArn:                   "arn-123456789",
			wantMeshServiceAssociationStatus: vpclattice.ServiceNetworkServiceAssociationStatusActive,
			wantErr:                          nil,
			wantListServiceOutput:            []*vpclattice.ServiceSummary{},
		},

		{
			name:                             "test-2",
			tags:                             nil,
			meshName:                         "test-mesh-2",
			meshId:                           "id-234567890",
			meshArn:                          "arn-234567890",
			wantServiceName:                  "svc-test-2",
			wantServiceId:                    "id-123456789",
			wantServiceArn:                   "arn-123456789",
			wantMeshServiceAssociationStatus: vpclattice.ServiceNetworkServiceAssociationStatusCreateInProgress,
			wantErr:                          errors.New(LATTICE_RETRY),
			wantListServiceOutput:            []*vpclattice.ServiceSummary{},
		},
		{
			name:                             "test-3",
			tags:                             nil,
			meshName:                         "test-mesh-3",
			meshId:                           "id-234567890",
			meshArn:                          "arn-234567890",
			wantServiceName:                  "svc-test-3",
			wantServiceId:                    "id-123456789",
			wantServiceArn:                   "arn-123456789",
			wantMeshServiceAssociationStatus: vpclattice.ServiceNetworkServiceAssociationStatusActive,
			wantErr:                          errors.New(LATTICE_RETRY),
			wantListServiceOutput:            []*vpclattice.ServiceSummary{},
		},
		{
			name:                             "test-4",
			tags:                             nil,
			meshName:                         "test-mesh-4",
			meshId:                           "id-234567890",
			meshArn:                          "arn-234567890",
			wantServiceName:                  "svc-test-4",
			wantServiceId:                    "id-123456789",
			wantServiceArn:                   "arn-123456789",
			wantMeshServiceAssociationStatus: vpclattice.ServiceNetworkServiceAssociationStatusDeleteInProgress,
			wantErr:                          errors.New(LATTICE_RETRY),
			wantListServiceOutput:            []*vpclattice.ServiceSummary{},
		},
		{
			name:                             "test-5",
			tags:                             nil,
			meshName:                         "test-mesh-5",
			meshId:                           "id-234567890",
			meshArn:                          "arn-234567890",
			wantServiceName:                  "svc-test-5",
			wantServiceId:                    "id-123456789",
			wantServiceArn:                   "arn-123456789",
			wantMeshServiceAssociationStatus: vpclattice.ServiceNetworkServiceAssociationStatusDeleteFailed,
			wantErr:                          errors.New(LATTICE_RETRY),
			wantListServiceOutput:            []*vpclattice.ServiceSummary{},
		},
	}

	for _, tt := range tests {
		fmt.Printf("testing >>> %v \n", tt.name)
		c := gomock.NewController(t)
		defer c.Finish()
		ctx := context.TODO()
		mockVpcLatticeSess := mocks.NewMockLattice(c)
		latticeDataStore := latticestore.NewLatticeDataStore()
		mockCloud := mocks_aws.NewMockCloud(c)
		mockCloud.EXPECT().GetAccountID().Return("123456789012").AnyTimes()
		mockCloud.EXPECT().GetServiceNetworkName().Return(tt.meshName).AnyTimes()
		mockCloud.EXPECT().GetVpcID().Return("").AnyTimes()

		latticeDataStore.AddServiceNetwork(tt.meshName, mockCloud.GetAccountID(), tt.meshArn, tt.meshId, latticestore.DATASTORE_SERVICE_NETWORK_CREATED)

		createServiceOutput := &vpclattice.CreateServiceOutput{
			Arn:    &tt.wantServiceArn,
			Id:     &tt.wantServiceId,
			Name:   &tt.wantServiceName,
			Status: aws.String(vpclattice.ServiceStatusActive),
		}
		createServiceNetworkServiceAssociationOutput := &vpclattice.CreateServiceNetworkServiceAssociationOutput{
			Arn:      nil,
			DnsEntry: nil,
			Id:       nil,
			Status:   &tt.wantMeshServiceAssociationStatus,
		}
		input := &latticemodel.Service{
			Spec: latticemodel.ServiceSpec{
				Name:                tt.wantServiceName,
				Protocols:           []*string{aws.String("http")},
				ServiceNetworkNames: []string{tt.meshName},
			},
			Status: &latticemodel.ServiceStatus{ServiceARN: "", ServiceID: ""},
		}

		mockVpcLatticeSess.EXPECT().ListServicesAsList(ctx, gomock.Any()).Return(tt.wantListServiceOutput, nil)
		mockVpcLatticeSess.EXPECT().CreateServiceWithContext(ctx, gomock.Any()).Return(createServiceOutput, nil)

		mockVpcLatticeSess.EXPECT().ListServiceNetworkServiceAssociationsAsList(ctx, gomock.Any())
		if tt.wantErr == nil {
			mockVpcLatticeSess.EXPECT().ListServiceNetworkServiceAssociationsAsList(ctx, gomock.Any())
		}
		mockVpcLatticeSess.EXPECT().CreateServiceNetworkServiceAssociationWithContext(ctx, gomock.Any()).Return(createServiceNetworkServiceAssociationOutput, tt.wantErr)
		mockCloud.EXPECT().Lattice().Return(mockVpcLatticeSess).AnyTimes()

		serviceManager := NewServiceManager(mockCloud, latticeDataStore)
		resp, err := serviceManager.Create(ctx, input)

		if tt.wantErr != nil {
			assert.NotNil(t, err)
			assert.Equal(t, err, errors.New(LATTICE_RETRY))
			assert.Equal(t, resp.ServiceARN, "")
			assert.Equal(t, resp.ServiceID, "")
		} else {
			assert.Nil(t, err)
			assert.Equal(t, resp.ServiceARN, tt.wantServiceArn)
			assert.Equal(t, resp.ServiceID, tt.wantServiceId)
		}
	}
}

func Test_Create_MeshServiceAssociation(t *testing.T) {
	tests := []struct {
		name                             string
		tags                             map[string]*string
		meshName                         string
		meshId                           string
		meshArn                          string
		wantServiceName                  string
		wantServiceId                    string
		wantServiceArn                   string
		wantMeshServiceAssociationStatus string
		wantErr                          error
		wantListServiceOutput            []*vpclattice.ServiceSummary
		existingAssociationStatus        string
		existingAssociationErr           error
	}{
		{
			name:                             "test-1",
			tags:                             nil,
			meshName:                         "test-mesh-1",
			meshId:                           "id-234567890",
			meshArn:                          "arn-234567890",
			wantServiceName:                  "svc-test-1",
			wantServiceId:                    "id-123456789",
			wantServiceArn:                   "arn-123456789",
			wantListServiceOutput:            []*vpclattice.ServiceSummary{},
			existingAssociationStatus:        vpclattice.ServiceNetworkServiceAssociationStatusCreateInProgress,
			existingAssociationErr:           errors.New(LATTICE_RETRY),
			wantMeshServiceAssociationStatus: "",
			wantErr:                          errors.New(LATTICE_RETRY),
		},
		{
			name:                             "test-2",
			tags:                             nil,
			meshName:                         "test-mesh-2",
			meshId:                           "id-234567890",
			meshArn:                          "arn-234567890",
			wantServiceName:                  "svc-test-2",
			wantServiceId:                    "id-123456789",
			wantServiceArn:                   "arn-123456789",
			wantListServiceOutput:            []*vpclattice.ServiceSummary{},
			existingAssociationStatus:        vpclattice.ServiceNetworkServiceAssociationStatusDeleteInProgress,
			existingAssociationErr:           errors.New(LATTICE_RETRY),
			wantMeshServiceAssociationStatus: "",
			wantErr:                          errors.New(LATTICE_RETRY),
		},
		{
			name:                             "test-3",
			tags:                             nil,
			meshName:                         "test-mesh-3",
			meshId:                           "id-234567890",
			meshArn:                          "arn-234567890",
			wantServiceName:                  "svc-test-3",
			wantServiceId:                    "id-123456789",
			wantServiceArn:                   "arn-123456789",
			wantListServiceOutput:            []*vpclattice.ServiceSummary{},
			existingAssociationStatus:        vpclattice.ServiceNetworkServiceAssociationStatusDeleteFailed,
			existingAssociationErr:           nil,
			wantMeshServiceAssociationStatus: vpclattice.ServiceNetworkServiceAssociationStatusActive,
			wantErr:                          nil,
		},
		{
			name:                             "test-4",
			tags:                             nil,
			meshName:                         "test-mesh-4",
			meshId:                           "id-234567890",
			meshArn:                          "arn-234567890",
			wantServiceName:                  "svc-test-4",
			wantServiceId:                    "id-123456789",
			wantServiceArn:                   "arn-123456789",
			wantListServiceOutput:            []*vpclattice.ServiceSummary{},
			existingAssociationStatus:        vpclattice.ServiceNetworkServiceAssociationStatusCreateFailed,
			existingAssociationErr:           nil,
			wantMeshServiceAssociationStatus: vpclattice.ServiceNetworkServiceAssociationStatusActive,
			wantErr:                          nil,
		},
	}

	for _, tt := range tests {
		fmt.Printf("testing >>> %v \n", tt.name)
		c := gomock.NewController(t)
		defer c.Finish()
		ctx := context.TODO()
		mockVpcLatticeSess := mocks.NewMockLattice(c)
		latticeDataStore := latticestore.NewLatticeDataStore()
		mockCloud := mocks_aws.NewMockCloud(c)
		mockCloud.EXPECT().GetAccountID().Return("123456789012").AnyTimes()
		mockCloud.EXPECT().GetServiceNetworkName().Return(tt.meshName).AnyTimes()

		latticeDataStore.AddServiceNetwork(tt.meshName, mockCloud.GetAccountID(), tt.meshArn, tt.meshId, latticestore.DATASTORE_SERVICE_NETWORK_CREATED)

		createServiceNetworkServiceAssociationOutput := &vpclattice.CreateServiceNetworkServiceAssociationOutput{
			Arn:      nil,
			DnsEntry: nil,
			Id:       nil,
			Status:   &tt.wantMeshServiceAssociationStatus,
		}
		input := &latticemodel.Service{
			Spec: latticemodel.ServiceSpec{
				Name:                tt.wantServiceName,
				Namespace:           "default",
				Protocols:           []*string{aws.String("http")},
				ServiceNetworkNames: []string{tt.meshName},
			},
			Status: &latticemodel.ServiceStatus{ServiceARN: "", ServiceID: ""},
		}
		SVCName := latticestore.LatticeServiceName(tt.wantServiceName, "default")
		tt.wantListServiceOutput = append(tt.wantListServiceOutput, &vpclattice.ServiceSummary{
			Arn:  &tt.wantServiceArn,
			Id:   &tt.wantServiceId,
			Name: &SVCName,
		})
		listMeshServiceAssociationsOutput := []*vpclattice.ServiceNetworkServiceAssociationSummary{&vpclattice.ServiceNetworkServiceAssociationSummary{
			Status: &tt.existingAssociationStatus,
		}}

		mockVpcLatticeSess.EXPECT().ListServicesAsList(ctx, gomock.Any()).Return(tt.wantListServiceOutput, nil)
		mockVpcLatticeSess.EXPECT().ListServiceNetworkServiceAssociationsAsList(ctx, gomock.Any()).Return(listMeshServiceAssociationsOutput, tt.existingAssociationErr)
		if tt.wantErr == nil {
			mockVpcLatticeSess.EXPECT().ListServiceNetworkServiceAssociationsAsList(ctx, gomock.Any())
		}
		mockVpcLatticeSess.EXPECT().CreateServiceNetworkServiceAssociationWithContext(ctx, gomock.Any()).Return(createServiceNetworkServiceAssociationOutput, tt.wantErr)
		mockCloud.EXPECT().Lattice().Return(mockVpcLatticeSess).AnyTimes()

		serviceManager := NewServiceManager(mockCloud, latticeDataStore)
		resp, err := serviceManager.Create(ctx, input)

		if tt.existingAssociationErr != nil {
			assert.NotNil(t, err)
			assert.Equal(t, err, errors.New(LATTICE_RETRY))
			assert.Equal(t, resp.ServiceARN, "")
			assert.Equal(t, resp.ServiceID, "")
		} else {
			assert.Nil(t, err)
			assert.Equal(t, resp.ServiceARN, tt.wantServiceArn)
			assert.Equal(t, resp.ServiceID, tt.wantServiceId)
		}
	}
}

func Test_Create_Check(t *testing.T) {
	tests := []struct {
		tags                             map[string]*string
		meshName                         string
		meshId                           string
		meshArn                          string
		wantServiceName                  string
		wantServiceId                    string
		wantServiceArn                   string
		wantMeshServiceAssociationStatus string
		wantErr                          error
		wantListServiceOutput            []*vpclattice.ServiceSummary
		existingAssociationStatus        string
		existingAssociationErr           error
	}{
		{
			tags:                             nil,
			meshName:                         "test-mesh-1",
			meshId:                           "id-234567890",
			meshArn:                          "arn-234567890",
			wantServiceName:                  "svc-test-1",
			wantServiceId:                    "id-123456789",
			wantServiceArn:                   "arn-123456789",
			wantListServiceOutput:            []*vpclattice.ServiceSummary{},
			existingAssociationStatus:        vpclattice.ServiceNetworkServiceAssociationStatusActive,
			existingAssociationErr:           nil,
			wantMeshServiceAssociationStatus: vpclattice.ServiceNetworkServiceAssociationStatusActive,
			wantErr:                          nil,
		},
	}

	for _, tt := range tests {
		c := gomock.NewController(t)
		defer c.Finish()
		ctx := context.TODO()
		mockVpcLatticeSess := mocks.NewMockLattice(c)
		latticeDataStore := latticestore.NewLatticeDataStore()
		mockCloud := mocks_aws.NewMockCloud(c)
		mockCloud.EXPECT().GetAccountID().Return("123456789012").AnyTimes()
		mockCloud.EXPECT().GetServiceNetworkName().Return(tt.meshName).AnyTimes()

		latticeDataStore.AddServiceNetwork(tt.meshName, mockCloud.GetAccountID(), tt.meshArn, tt.meshId, latticestore.DATASTORE_SERVICE_NETWORK_CREATED)

		input := &latticemodel.Service{
			Spec: latticemodel.ServiceSpec{
				Name:                tt.wantServiceName,
				Namespace:           "default",
				Protocols:           []*string{aws.String("http")},
				ServiceNetworkNames: []string{tt.meshName},
			},
			Status: &latticemodel.ServiceStatus{ServiceARN: "", ServiceID: ""},
		}
		SVCName := latticestore.LatticeServiceName(tt.wantServiceName, "default")
		tt.wantListServiceOutput = append(tt.wantListServiceOutput, &vpclattice.ServiceSummary{
			Arn:  &tt.wantServiceArn,
			Id:   &tt.wantServiceId,
			Name: &SVCName,
		})
		listMeshServiceAssociationsOutput := []*vpclattice.ServiceNetworkServiceAssociationSummary{&vpclattice.ServiceNetworkServiceAssociationSummary{
			ServiceNetworkName: &tt.meshName,
			Status:             &tt.existingAssociationStatus,
		}}

		mockVpcLatticeSess.EXPECT().ListServicesAsList(ctx, gomock.Any()).Return(tt.wantListServiceOutput, nil)
		mockVpcLatticeSess.EXPECT().ListServiceNetworkServiceAssociationsAsList(ctx, gomock.Any()).Return(listMeshServiceAssociationsOutput, tt.existingAssociationErr)
		mockVpcLatticeSess.EXPECT().ListServiceNetworkServiceAssociationsAsList(ctx, gomock.Any()).Return(listMeshServiceAssociationsOutput, tt.existingAssociationErr)
		mockCloud.EXPECT().Lattice().Return(mockVpcLatticeSess).AnyTimes()

		serviceManager := NewServiceManager(mockCloud, latticeDataStore)
		resp, err := serviceManager.Create(ctx, input)

		assert.Nil(t, err)
		assert.Equal(t, resp.ServiceARN, tt.wantServiceArn)
		assert.Equal(t, resp.ServiceID, tt.wantServiceId)
	}
}
func Test_Delete_ValidateInput(t *testing.T) {
	tests := []struct {
		meshName                                     string
		meshId                                       string
		meshArn                                      string
		wantServiceName                              string
		wantServiceId                                string
		wantServiceArn                               string
		wantMeshServiceAssociationStatus             string
		wantErr                                      error
		wantListServiceOutput                        []*vpclattice.ServiceSummary
		deleteServiceNetworkServiceAssociationOutput *vpclattice.DeleteServiceNetworkServiceAssociationOutput
		deleteServiceOutput                          *vpclattice.DeleteServiceOutput
		wantListMeshServiceAssociationsErr           error
		meshServiceAssociationId                     string
	}{
		{
			meshName:                         "test-mesh-1",
			meshId:                           "id-234567890",
			meshArn:                          "arn-234567890",
			wantServiceName:                  "svc-test-1",
			wantServiceId:                    "id-123456789",
			wantServiceArn:                   "arn-123456789",
			wantMeshServiceAssociationStatus: vpclattice.ServiceNetworkServiceAssociationStatusActive,
			wantErr:                          nil,
			wantListServiceOutput:            []*vpclattice.ServiceSummary{},
			deleteServiceNetworkServiceAssociationOutput: &vpclattice.DeleteServiceNetworkServiceAssociationOutput{},
			deleteServiceOutput:                          &vpclattice.DeleteServiceOutput{},
			wantListMeshServiceAssociationsErr:           nil,
			meshServiceAssociationId:                     "mesh-svc-id-123456789",
		},
	}

	for _, tt := range tests {
		c := gomock.NewController(t)
		defer c.Finish()
		ctx := context.TODO()
		mockVpcLatticeSess := mocks.NewMockLattice(c)
		latticeDataStore := latticestore.NewLatticeDataStore()
		mockCloud := mocks_aws.NewMockCloud(c)
		mockCloud.EXPECT().GetAccountID().Return("123456789012").AnyTimes()
		mockCloud.EXPECT().GetServiceNetworkName().Return(tt.meshName).AnyTimes()

		latticeDataStore.AddServiceNetwork(tt.meshName, mockCloud.GetAccountID(), tt.meshArn, tt.meshId, latticestore.DATASTORE_SERVICE_NETWORK_CREATED)

		SVCName := latticestore.LatticeServiceName(tt.wantServiceName, "default")
		tt.wantListServiceOutput = append(tt.wantListServiceOutput, &vpclattice.ServiceSummary{
			Arn:  &tt.wantServiceArn,
			Id:   &tt.wantServiceId,
			Name: &SVCName,
		})
		input := &latticemodel.Service{
			Spec: latticemodel.ServiceSpec{
				Name:                tt.wantServiceName,
				Namespace:           "default",
				ServiceNetworkNames: []string{tt.meshName},
			},
		}
		listMeshServiceAssociationsOutput := []*vpclattice.ServiceNetworkServiceAssociationSummary{&vpclattice.ServiceNetworkServiceAssociationSummary{
			Status: &tt.wantMeshServiceAssociationStatus,
			Id:     &tt.meshServiceAssociationId,
		}}

		listServicesInput := &vpclattice.ListServicesInput{}
		listMeshServiceAssociationsInput := &vpclattice.ListServiceNetworkServiceAssociationsInput{
			ServiceIdentifier: &tt.wantServiceId,
		}
		deleteMeshServiceAssociationInput := &vpclattice.DeleteServiceNetworkServiceAssociationInput{ServiceNetworkServiceAssociationIdentifier: &tt.meshServiceAssociationId}

		mockVpcLatticeSess.EXPECT().ListServicesAsList(ctx, listServicesInput).Return(tt.wantListServiceOutput, nil)
		mockVpcLatticeSess.EXPECT().ListServiceNetworkServiceAssociationsAsList(ctx, listMeshServiceAssociationsInput).Return(listMeshServiceAssociationsOutput, tt.wantListMeshServiceAssociationsErr)

		mockVpcLatticeSess.EXPECT().DeleteServiceNetworkServiceAssociationWithContext(ctx, deleteMeshServiceAssociationInput).Return(tt.deleteServiceNetworkServiceAssociationOutput, tt.wantErr)

		mockVpcLatticeSess.EXPECT().DeleteServiceWithContext(ctx, gomock.Any()).Return(tt.deleteServiceOutput, tt.wantErr)
		mockCloud.EXPECT().Lattice().Return(mockVpcLatticeSess).AnyTimes()

		serviceManager := NewServiceManager(mockCloud, latticeDataStore)
		err := serviceManager.Delete(ctx, input)

		assert.Nil(t, err)

	}
}

func Test_Delete_Disassociation_DeleteService(t *testing.T) {
	tests := []struct {
		name                                         string
		meshName                                     string
		meshId                                       string
		meshArn                                      string
		wantServiceName                              string
		wantServiceId                                string
		wantServiceArn                               string
		wantMeshServiceAssociationStatus             string
		wantErr                                      error
		wantListServiceOutput                        []*vpclattice.ServiceSummary
		deleteServiceNetworkServiceAssociationOutput *vpclattice.DeleteServiceNetworkServiceAssociationOutput
		deleteServiceOutput                          *vpclattice.DeleteServiceOutput
		wantListMeshServiceAssociationsErr           error
	}{
		{
			name:                             "test-1",
			meshName:                         "test-mesh-1",
			meshId:                           "id-234567890",
			meshArn:                          "arn-234567890",
			wantServiceName:                  "svc-test-1",
			wantServiceId:                    "id-123456789",
			wantServiceArn:                   "arn-123456789",
			wantMeshServiceAssociationStatus: vpclattice.ServiceNetworkServiceAssociationStatusActive,
			wantErr:                          nil,
			wantListServiceOutput:            []*vpclattice.ServiceSummary{},
			deleteServiceNetworkServiceAssociationOutput: &vpclattice.DeleteServiceNetworkServiceAssociationOutput{},
			deleteServiceOutput:                          &vpclattice.DeleteServiceOutput{},
			wantListMeshServiceAssociationsErr:           nil,
		},

		{
			name:                             "test-2",
			meshName:                         "test-mesh-1",
			meshId:                           "id-234567890",
			meshArn:                          "arn-234567890",
			wantServiceName:                  "svc-test-1",
			wantServiceId:                    "id-123456789",
			wantServiceArn:                   "arn-123456789",
			wantMeshServiceAssociationStatus: vpclattice.ServiceNetworkServiceAssociationStatusActive,
			wantErr:                          errors.New(LATTICE_RETRY),
			wantListServiceOutput:            []*vpclattice.ServiceSummary{},
			deleteServiceNetworkServiceAssociationOutput: &vpclattice.DeleteServiceNetworkServiceAssociationOutput{},
			deleteServiceOutput:                          &vpclattice.DeleteServiceOutput{},
			wantListMeshServiceAssociationsErr:           nil,
		},

		{
			name:                             "test-3",
			meshName:                         "test-mesh-1",
			meshId:                           "id-234567890",
			meshArn:                          "arn-234567890",
			wantServiceName:                  "svc-test-1",
			wantServiceId:                    "id-123456789",
			wantServiceArn:                   "arn-123456789",
			wantMeshServiceAssociationStatus: vpclattice.ServiceNetworkServiceAssociationStatusActive,
			wantErr:                          errors.New(LATTICE_RETRY),
			wantListServiceOutput:            []*vpclattice.ServiceSummary{},
			deleteServiceNetworkServiceAssociationOutput: &vpclattice.DeleteServiceNetworkServiceAssociationOutput{},
			deleteServiceOutput:                          &vpclattice.DeleteServiceOutput{},
			wantListMeshServiceAssociationsErr:           errors.New(LATTICE_RETRY),
		},
	}

	for _, tt := range tests {
		fmt.Printf("testing >>>>> %v \n", tt.name)
		c := gomock.NewController(t)
		defer c.Finish()
		ctx := context.TODO()
		mockVpcLatticeSess := mocks.NewMockLattice(c)
		latticeDataStore := latticestore.NewLatticeDataStore()
		mockCloud := mocks_aws.NewMockCloud(c)
		mockCloud.EXPECT().GetAccountID().Return("123456789012").AnyTimes()
		mockCloud.EXPECT().GetServiceNetworkName().Return(tt.meshName).AnyTimes()

		latticeDataStore.AddServiceNetwork(tt.meshName, mockCloud.GetAccountID(), tt.meshArn, tt.meshId, latticestore.DATASTORE_SERVICE_NETWORK_CREATED)

		SVCName := latticestore.LatticeServiceName(tt.wantServiceName, "default")
		tt.wantListServiceOutput = append(tt.wantListServiceOutput, &vpclattice.ServiceSummary{
			Arn:  &tt.wantServiceArn,
			Id:   &tt.wantServiceId,
			Name: &SVCName,
		})
		input := &latticemodel.Service{
			Spec: latticemodel.ServiceSpec{
				Name:                tt.wantServiceName,
				Namespace:           "default",
				ServiceNetworkNames: []string{tt.meshName},
			},
		}
		listMeshServiceAssociationsOutput := []*vpclattice.ServiceNetworkServiceAssociationSummary{&vpclattice.ServiceNetworkServiceAssociationSummary{
			Status: &tt.wantMeshServiceAssociationStatus,
		}}

		mockVpcLatticeSess.EXPECT().ListServicesAsList(ctx, gomock.Any()).Return(tt.wantListServiceOutput, nil)
		mockVpcLatticeSess.EXPECT().ListServiceNetworkServiceAssociationsAsList(ctx, gomock.Any()).Return(listMeshServiceAssociationsOutput, tt.wantListMeshServiceAssociationsErr)
		//if tt.wantListMeshServiceAssociationsErr == nil {
		mockVpcLatticeSess.EXPECT().DeleteServiceNetworkServiceAssociationWithContext(ctx, gomock.Any()).Return(tt.deleteServiceNetworkServiceAssociationOutput, tt.wantErr)
		//}
		fmt.Printf("tt.wantListMeshServiceAssociationsErr : %v \n", tt.wantListMeshServiceAssociationsErr)
		if tt.wantErr == nil && tt.wantListMeshServiceAssociationsErr == nil {
			mockVpcLatticeSess.EXPECT().DeleteServiceWithContext(ctx, gomock.Any()).Return(tt.deleteServiceOutput, tt.wantErr)
		}
		mockCloud.EXPECT().Lattice().Return(mockVpcLatticeSess).AnyTimes()

		serviceManager := NewServiceManager(mockCloud, latticeDataStore)
		err := serviceManager.Delete(ctx, input)

		if tt.wantErr != nil {
			assert.NotNil(t, err)
			assert.Equal(t, err, errors.New(LATTICE_RETRY))
		} else {
			assert.Nil(t, err)
		}
	}
}

/* TODO do we still need this test?
func Test_Delete_ReturnErrorFail(t *testing.T) {
	tests := []struct {
		meshName                           string
		meshId                             string
		meshArn                            string
		wantServiceName                    string
		wantServiceId                      string
		wantServiceArn                     string
		wantMeshServiceAssociationStatus   string
		wantErr                            error
		wantListServiceOutput              []*vpclattice.ServiceSummary
		deleteServiceNetworkServiceAssociationOutput *vpclattice.DeleteServiceNetworkServiceAssociationOutput
		deleteServiceOutput                *vpclattice.DeleteServiceOutput
		wantListServiceErr                 error
		wantListMeshServiceAssociationsErr error
	}{
		{
			meshName:                           "test-mesh-2",
			meshId:                             "id-234567890",
			meshArn:                            "arn-234567890",
			wantServiceName:                    "svc-test-2",
			wantServiceId:                      "id-123456789",
			wantServiceArn:                     "arn-123456789",
			wantMeshServiceAssociationStatus:   vpclattice.ServiceNetworkServiceAssociationStatusActive,
			wantErr:                            errors.New(LATTICE_RETRY),
			wantListServiceOutput:              []*vpclattice.ServiceSummary{},
			deleteServiceNetworkServiceAssociationOutput: &vpclattice.DeleteServiceNetworkServiceAssociationOutput{},
			deleteServiceOutput:                &vpclattice.DeleteServiceOutput{},
			wantListServiceErr:                 nil,
			wantListMeshServiceAssociationsErr: nil,
		},
		{
			meshName:                           "test-mesh-1",
			meshId:                             "id-234567890",
			meshArn:                            "arn-234567890",
			wantServiceName:                    "svc-test-1",
			wantServiceId:                      "id-123456789",
			wantServiceArn:                     "arn-123456789",
			wantMeshServiceAssociationStatus:   vpclattice.ServiceNetworkServiceAssociationStatusActive,
			wantErr:                            nil,
			wantListServiceOutput:              []*vpclattice.ServiceSummary{},
			deleteServiceNetworkServiceAssociationOutput: &vpclattice.DeleteServiceNetworkServiceAssociationOutput{},
			deleteServiceOutput:                &vpclattice.DeleteServiceOutput{},
			wantListServiceErr:                 errors.New(LATTICE_RETRY),
			wantListMeshServiceAssociationsErr: nil,
		},
	}

	for _, tt := range tests {
		c := gomock.NewController(t)
		defer c.Finish()
		ctx := context.TODO()
		mockVpcLatticeSess := mocks.NewMockLattice(c)
		latticeDataStore := latticestore.NewLatticeDataStore()
		latticeDataStore.AddServiceNetwork(tt.meshName, mockCloud.GetAccountID(), tt.meshArn, tt.meshId, latticestore.DATASTORE_MESH_CREATED)
		mockCloud := mocks_aws.NewMockCloud(c)

		tt.wantListServiceOutput = append(tt.wantListServiceOutput, &vpclattice.ServiceSummary{
			Arn:  &tt.wantServiceArn,
			Id:   &tt.wantServiceId,
			Name: &tt.wantServiceName,
		})
		input := &latticemodel.Service{
			Spec: latticemodel.ServiceSpec{
				Name:     tt.wantServiceName,
				ServiceNetworkName: tt.meshName,
			},
		}
		listMeshServiceAssociationsOutput := []*vpclattice.ServiceNetworkServiceAssociationSummary{&vpclattice.ServiceNetworkServiceAssociationSummary{
			Status: &tt.wantMeshServiceAssociationStatus,
		}}

		mockVpcLatticeSess.EXPECT().ListServicesAsList(ctx, gomock.Any()).Return(tt.wantListServiceOutput, tt.wantListServiceErr)
		if tt.wantListServiceErr == nil {
			mockVpcLatticeSess.EXPECT().ListServiceNetworkServiceAssociationsAsList(ctx, gomock.Any()).Return(listMeshServiceAssociationsOutput, tt.wantListMeshServiceAssociationsErr)
		}
		mockCloud.EXPECT().Lattice().Return(mockVpcLatticeSess).AnyTimes()

		serviceManager := NewServiceManager(mockCloud, latticeDataStore)
		err := serviceManager.Delete(ctx, input)

		assert.Equal(t, err, tt.wantErr)
	}
}
*/

func Test_serviceNetworkAssociationMgr(t *testing.T) {
	type SNDisAssocStatus struct {
		snName       string
		needDisassoc bool
		status       string
	}
	type SNAssocStatus struct {
		snName            string
		snID              string
		associatedAlready bool
		status            string
	}
	tests := []struct {
		name             string
		serviceName      string
		serviceID        string
		serviceNapespace string
		desiredSNs       []SNAssocStatus
		desiredSNInCache bool
		existingSNs      []SNDisAssocStatus
		errOnAssociating bool
		wantErr          bool
	}{
		{
			name:             "testing () --> (sn1, sn2) happy path",
			serviceName:      "svc-123",
			serviceID:        "svc-123-id",
			serviceNapespace: "default",
			desiredSNs: []SNAssocStatus{
				{snName: "sn1", snID: "sn1-id", associatedAlready: false,
					status: vpclattice.ServiceNetworkServiceAssociationStatusActive},
				{snName: "sn2", snID: "sn2-id", associatedAlready: false,
					status: vpclattice.ServiceNetworkServiceAssociationStatusActive},
			},
			desiredSNInCache: true,
			existingSNs:      []SNDisAssocStatus{},
			errOnAssociating: false,
			wantErr:          false,
		},
		{
			name:             "testing () --> (sn1, sn2), sn1 association-in-progress",
			serviceName:      "svc-123",
			serviceID:        "svc-123-id",
			serviceNapespace: "default",
			desiredSNs: []SNAssocStatus{
				{snName: "sn1", snID: "sn1-id", associatedAlready: false,
					status: vpclattice.ServiceNetworkServiceAssociationStatusActive},
				{snName: "sn2", snID: "sn2-id", associatedAlready: false,
					status: vpclattice.ServiceNetworkServiceAssociationStatusCreateInProgress},
			},
			desiredSNInCache: true,
			existingSNs:      []SNDisAssocStatus{},
			errOnAssociating: true,
			wantErr:          true,
		},

		{
			name:             "testing (sn1, sn2) --> (sn1, sn2, sn3) happy path",
			serviceName:      "svc-123",
			serviceID:        "svc-123-id",
			serviceNapespace: "default",
			desiredSNs: []SNAssocStatus{
				{snName: "sn1", snID: "sn1-id", associatedAlready: true,
					status: vpclattice.ServiceNetworkServiceAssociationStatusActive},
				{snName: "sn2", snID: "sn2-id", associatedAlready: true,
					status: vpclattice.ServiceNetworkServiceAssociationStatusActive},
				{snName: "sn3", snID: "sn3-id", associatedAlready: false,
					status: vpclattice.ServiceNetworkServiceAssociationStatusActive},
			},
			desiredSNInCache: true,
			existingSNs: []SNDisAssocStatus{
				{snName: "sn1", needDisassoc: false,
					status: vpclattice.ServiceNetworkServiceAssociationStatusActive},
				{snName: "sn2", needDisassoc: false,
					status: vpclattice.ServiceNetworkServiceAssociationStatusActive},
			},
			errOnAssociating: false,
			wantErr:          false,
		},

		{
			name:             "testing (sn1, sn2) --> (sn1, sn2, sn3) sn3 work-in-progress",
			serviceName:      "svc-123",
			serviceID:        "svc-123-id",
			serviceNapespace: "default",
			desiredSNs: []SNAssocStatus{
				{snName: "sn1", snID: "sn1-id", associatedAlready: true,
					status: vpclattice.ServiceNetworkServiceAssociationStatusActive},
				{snName: "sn2", snID: "sn2-id", associatedAlready: true,
					status: vpclattice.ServiceNetworkServiceAssociationStatusActive},
				{snName: "sn3", snID: "sn3-id", associatedAlready: false,
					status: vpclattice.ServiceNetworkServiceAssociationStatusCreateInProgress},
			},
			desiredSNInCache: true,
			existingSNs: []SNDisAssocStatus{
				{snName: "sn1", needDisassoc: false,
					status: vpclattice.ServiceNetworkServiceAssociationStatusActive},
				{snName: "sn2", needDisassoc: false,
					status: vpclattice.ServiceNetworkServiceAssociationStatusCreateInProgress},
			},
			errOnAssociating: true,
			wantErr:          true,
		},

		{
			name:             "testing (sn1, sn2, sn3) --> ( sn2, sn3), happy path",
			serviceName:      "svc-123",
			serviceID:        "svc-123-id",
			serviceNapespace: "default",
			desiredSNs: []SNAssocStatus{

				{snName: "sn2", snID: "sn2-id", associatedAlready: true,
					status: vpclattice.ServiceNetworkServiceAssociationStatusActive},
				{snName: "sn3", snID: "sn3-id", associatedAlready: true,
					status: vpclattice.ServiceNetworkServiceAssociationStatusActive},
			},

			desiredSNInCache: true,
			existingSNs: []SNDisAssocStatus{
				{snName: "sn1", needDisassoc: true,
					status: vpclattice.ServiceNetworkServiceAssociationStatusActive},
				{snName: "sn2", needDisassoc: false,
					status: vpclattice.ServiceNetworkServiceAssociationStatusActive},
				{snName: "sn3", needDisassoc: false,
					status: vpclattice.ServiceNetworkServiceAssociationStatusActive},
			},
			errOnAssociating: false,
			wantErr:          false,
		},
		{
			name:             "testing (sn1, sn2, sn3) --> ( sn2, sn3), sn1 disassoc-in-progress",
			serviceName:      "svc-123",
			serviceID:        "svc-123-id",
			serviceNapespace: "default",
			desiredSNs: []SNAssocStatus{

				{snName: "sn2", snID: "sn2-id", associatedAlready: true,
					status: vpclattice.ServiceNetworkServiceAssociationStatusActive},
				{snName: "sn3", snID: "sn3-id", associatedAlready: true,
					status: vpclattice.ServiceNetworkServiceAssociationStatusActive},
			},

			desiredSNInCache: true,
			existingSNs: []SNDisAssocStatus{
				{snName: "sn1", needDisassoc: true,
					status: vpclattice.ServiceNetworkServiceAssociationStatusDeleteInProgress},
				{snName: "sn2", needDisassoc: false,
					status: vpclattice.ServiceNetworkServiceAssociationStatusActive},
				{snName: "sn3", needDisassoc: false,
					status: vpclattice.ServiceNetworkServiceAssociationStatusActive},
			},
			errOnAssociating: false,
			wantErr:          true,
		},
		{
			name:             "testing (sn1, sn2, sn3) --> ( ), happy path",
			serviceName:      "svc-123",
			serviceID:        "svc-123-id",
			serviceNapespace: "default",
			desiredSNs:       []SNAssocStatus{},

			desiredSNInCache: true,
			existingSNs: []SNDisAssocStatus{
				{snName: "sn1", needDisassoc: true,
					status: vpclattice.ServiceNetworkServiceAssociationStatusActive},
				{snName: "sn2", needDisassoc: true,
					status: vpclattice.ServiceNetworkServiceAssociationStatusActive},
				{snName: "sn3", needDisassoc: true,
					status: vpclattice.ServiceNetworkServiceAssociationStatusActive},
			},
			errOnAssociating: false,
			wantErr:          false,
		},

		{
			name:             "testing (sn1, sn2, sn3) --> ( ), sn2 delete-progress",
			serviceName:      "svc-123",
			serviceID:        "svc-123-id",
			serviceNapespace: "default",
			desiredSNs:       []SNAssocStatus{},

			desiredSNInCache: true,
			existingSNs: []SNDisAssocStatus{
				{snName: "sn1", needDisassoc: true,
					status: vpclattice.ServiceNetworkServiceAssociationStatusActive},
				{snName: "sn2", needDisassoc: true,
					status: vpclattice.ServiceNetworkServiceAssociationStatusDeleteInProgress},
				{snName: "sn3", needDisassoc: true,
					status: vpclattice.ServiceNetworkServiceAssociationStatusActive},
			},
			errOnAssociating: false,
			wantErr:          true,
		},
	}

	for _, tt := range tests {
		fmt.Printf("Testing >>>>>  %v \n", tt.name)

		c := gomock.NewController(t)
		defer c.Finish()
		ctx := context.TODO()
		mockVpcLatticeSess := mocks.NewMockLattice(c)

		latticeDataStore := latticestore.NewLatticeDataStore()
		mockCloud := mocks_aws.NewMockCloud(c)
		mockCloud.EXPECT().GetAccountID().Return("123456789012").AnyTimes()

		if tt.desiredSNInCache {
			for _, sn := range tt.desiredSNs {
				latticeDataStore.AddServiceNetwork(sn.snName, mockCloud.GetAccountID(), "snARN", sn.snID, latticestore.DATASTORE_SERVICE_NETWORK_CREATED)
			}
		}

		mockCloud.EXPECT().Lattice().Return(mockVpcLatticeSess).AnyTimes()

		serviceManager := NewServiceManager(mockCloud, latticeDataStore)

		desiredSNNames := []string{}
		for _, sn := range tt.desiredSNs {
			desiredSNNames = append(desiredSNNames, sn.snName)

		}

		// test adding more association path

		listMeshServiceAssociationsOutput := []*vpclattice.ServiceNetworkServiceAssociationSummary{}

		for i := 0; i < len(tt.desiredSNs); i++ {
			if tt.desiredSNs[i].associatedAlready {
				activeStatus := vpclattice.ServiceNetworkServiceAssociationStatusActive
				listMeshServiceAssociationsOutput = []*vpclattice.ServiceNetworkServiceAssociationSummary{
					{Status: &activeStatus},
				}

			} else {
				listMeshServiceAssociationsOutput = []*vpclattice.ServiceNetworkServiceAssociationSummary{}
			}
			mockVpcLatticeSess.EXPECT().ListServiceNetworkServiceAssociationsAsList(ctx, gomock.Any()).Return(listMeshServiceAssociationsOutput, nil)

			if !tt.desiredSNs[i].associatedAlready {
				snAssocOutput := vpclattice.CreateServiceNetworkServiceAssociationOutput{
					Status: &tt.desiredSNs[i].status,
				}
				mockVpcLatticeSess.EXPECT().CreateServiceNetworkServiceAssociationWithContext(ctx,
					gomock.Any()).Return(&snAssocOutput, nil)
			}

			if tt.desiredSNs[i].status != vpclattice.ServiceNetworkServiceAssociationStatusActive {
				break
			}
		}

		// testing delete association path
		if !tt.errOnAssociating {
			listMeshServiceAssociationsOutput = []*vpclattice.ServiceNetworkServiceAssociationSummary{}
			for i := 0; i < len(tt.existingSNs); i++ {
				listMeshServiceAssociationsOutput = append(listMeshServiceAssociationsOutput,
					&vpclattice.ServiceNetworkServiceAssociationSummary{ServiceNetworkName: &tt.existingSNs[i].snName})

			}
			mockVpcLatticeSess.EXPECT().ListServiceNetworkServiceAssociationsAsList(ctx, gomock.Any()).Return(listMeshServiceAssociationsOutput, nil)

			for i := 0; i < len(tt.existingSNs); i++ {
				if tt.existingSNs[i].needDisassoc {
					deleteAssocOutput := &vpclattice.DeleteServiceNetworkServiceAssociationOutput{
						Status: &tt.existingSNs[i].status,
					}
					mockVpcLatticeSess.EXPECT().DeleteServiceNetworkServiceAssociationWithContext(ctx, gomock.Any()).Return(deleteAssocOutput, nil)
					if tt.existingSNs[i].status == vpclattice.ServiceNetworkServiceAssociationStatusDeleteInProgress {
						break
					}
				}
			}

		}
		err := serviceManager.serviceNetworkAssociationMgr(ctx, desiredSNNames, tt.serviceID)
		if !tt.wantErr {

			assert.Nil(t, err)
		}

	}
}
