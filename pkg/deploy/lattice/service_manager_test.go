package lattice

import (
	"context"
	"errors"
	"github.com/aws/aws-sdk-go/aws"
	"testing"

	"github.com/aws/aws-sdk-go/service/vpclattice"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	mocks_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	mocks "github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

func Test_Create_ValidateService(t *testing.T) {
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
		listServiceInput                 *vpclattice.ListServicesInput
	}{
		{
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
		c := gomock.NewController(t)
		defer c.Finish()
		ctx := context.TODO()
		mockVpcLatticeSess := mocks.NewMockLattice(c)
		latticeDataStore := latticestore.NewLatticeDataStore()
		latticeDataStore.AddServiceNetwork(tt.meshName, config.AccountID, tt.meshArn, tt.meshId, latticestore.DATASTORE_SERVICE_NETWORK_CREATED)
		mockCloud := mocks_aws.NewMockCloud(c)

		SVCName := latticestore.AWSServiceName(tt.wantServiceName, "default")
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
				Name:               tt.wantServiceName,
				Namespace:          "default",
				Protocols:          []*string{aws.String("http")},
				ServiceNetworkName: tt.meshName,
			},
			Status: &latticemodel.ServiceStatus{ServiceARN: "", ServiceID: ""},
		}

		createServiceInput := &vpclattice.CreateServiceInput{
			Name: &SVCName,
			Tags: nil,
		}
		associateMeshService := &vpclattice.CreateServiceNetworkServiceAssociationInput{
			ServiceNetworkIdentifier: &tt.meshId,
			ServiceIdentifier:        &tt.wantServiceId,
		}

		mockVpcLatticeSess.EXPECT().ListServicesAsList(ctx, tt.listServiceInput).Return(tt.wantListServiceOutput, nil)
		mockVpcLatticeSess.EXPECT().CreateServiceWithContext(ctx, createServiceInput).Return(createServiceOutput, nil)
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
		c := gomock.NewController(t)
		defer c.Finish()
		ctx := context.TODO()
		mockVpcLatticeSess := mocks.NewMockLattice(c)
		latticeDataStore := latticestore.NewLatticeDataStore()
		latticeDataStore.AddServiceNetwork(tt.meshName, config.AccountID, tt.meshArn, tt.meshId, latticestore.DATASTORE_SERVICE_NETWORK_CREATED)
		mockCloud := mocks_aws.NewMockCloud(c)

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
				Name:               tt.wantServiceName,
				Protocols:          []*string{aws.String("http")},
				ServiceNetworkName: tt.meshName,
			},
			Status: &latticemodel.ServiceStatus{ServiceARN: "", ServiceID: ""},
		}

		mockVpcLatticeSess.EXPECT().ListServicesAsList(ctx, gomock.Any()).Return(tt.wantListServiceOutput, nil)
		mockVpcLatticeSess.EXPECT().CreateServiceWithContext(ctx, gomock.Any()).Return(createServiceOutput, nil)
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
			existingAssociationStatus:        vpclattice.ServiceNetworkServiceAssociationStatusCreateInProgress,
			existingAssociationErr:           errors.New(LATTICE_RETRY),
			wantMeshServiceAssociationStatus: "",
			wantErr:                          errors.New(LATTICE_RETRY),
		},
		{
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
		c := gomock.NewController(t)
		defer c.Finish()
		ctx := context.TODO()
		mockVpcLatticeSess := mocks.NewMockLattice(c)
		latticeDataStore := latticestore.NewLatticeDataStore()
		latticeDataStore.AddServiceNetwork(tt.meshName, config.AccountID, tt.meshArn, tt.meshId, latticestore.DATASTORE_SERVICE_NETWORK_CREATED)
		mockCloud := mocks_aws.NewMockCloud(c)

		createServiceNetworkServiceAssociationOutput := &vpclattice.CreateServiceNetworkServiceAssociationOutput{
			Arn:      nil,
			DnsEntry: nil,
			Id:       nil,
			Status:   &tt.wantMeshServiceAssociationStatus,
		}
		input := &latticemodel.Service{
			Spec: latticemodel.ServiceSpec{
				Name:               tt.wantServiceName,
				Namespace:          "default",
				Protocols:          []*string{aws.String("http")},
				ServiceNetworkName: tt.meshName,
			},
			Status: &latticemodel.ServiceStatus{ServiceARN: "", ServiceID: ""},
		}
		SVCName := latticestore.AWSServiceName(tt.wantServiceName, "default")
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
		if tt.existingAssociationErr == nil {
			mockVpcLatticeSess.EXPECT().CreateServiceNetworkServiceAssociationWithContext(ctx, gomock.Any()).Return(createServiceNetworkServiceAssociationOutput, tt.wantErr)
		}
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
		latticeDataStore.AddServiceNetwork(tt.meshName, config.AccountID, tt.meshArn, tt.meshId, latticestore.DATASTORE_SERVICE_NETWORK_CREATED)
		mockCloud := mocks_aws.NewMockCloud(c)

		input := &latticemodel.Service{
			Spec: latticemodel.ServiceSpec{
				Name:               tt.wantServiceName,
				Namespace:          "default",
				Protocols:          []*string{aws.String("http")},
				ServiceNetworkName: tt.meshName,
			},
			Status: &latticemodel.ServiceStatus{ServiceARN: "", ServiceID: ""},
		}
		SVCName := latticestore.AWSServiceName(tt.wantServiceName, "default")
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
		latticeDataStore.AddServiceNetwork(tt.meshName, config.AccountID, tt.meshArn, tt.meshId, latticestore.DATASTORE_SERVICE_NETWORK_CREATED)
		mockCloud := mocks_aws.NewMockCloud(c)

		SVCName := latticestore.AWSServiceName(tt.wantServiceName, "default")
		tt.wantListServiceOutput = append(tt.wantListServiceOutput, &vpclattice.ServiceSummary{
			Arn:  &tt.wantServiceArn,
			Id:   &tt.wantServiceId,
			Name: &SVCName,
		})
		input := &latticemodel.Service{
			Spec: latticemodel.ServiceSpec{
				Name:               tt.wantServiceName,
				Namespace:          "default",
				ServiceNetworkName: tt.meshName,
			},
		}
		listMeshServiceAssociationsOutput := []*vpclattice.ServiceNetworkServiceAssociationSummary{&vpclattice.ServiceNetworkServiceAssociationSummary{
			Status: &tt.wantMeshServiceAssociationStatus,
			Id:     &tt.meshServiceAssociationId,
		}}

		listServicesInput := &vpclattice.ListServicesInput{}
		//deleteServiceRoutingConfigurationInput := &vpclattice.DeleteServiceRoutingConfigurationInput{ServiceIdentifier: &tt.wantServiceId}
		listMeshServiceAssociationsInput := &vpclattice.ListServiceNetworkServiceAssociationsInput{
			ServiceNetworkIdentifier: &tt.meshId,
			ServiceIdentifier:        &tt.wantServiceId,
		}
		deleteMeshServiceAssociationInput := &vpclattice.DeleteServiceNetworkServiceAssociationInput{ServiceNetworkServiceAssociationIdentifier: &tt.meshServiceAssociationId}

		mockVpcLatticeSess.EXPECT().ListServicesAsList(ctx, listServicesInput).Return(tt.wantListServiceOutput, nil)
		//mockVpcLatticeSess.EXPECT().DeleteServiceRoutingConfigurationWithContext(ctx, deleteServiceRoutingConfigurationInput).Return(tt.deleteServiceRoutingConfigurationOutput, nil)
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
		c := gomock.NewController(t)
		defer c.Finish()
		ctx := context.TODO()
		mockVpcLatticeSess := mocks.NewMockLattice(c)
		latticeDataStore := latticestore.NewLatticeDataStore()
		latticeDataStore.AddServiceNetwork(tt.meshName, config.AccountID, tt.meshArn, tt.meshId, latticestore.DATASTORE_SERVICE_NETWORK_CREATED)
		mockCloud := mocks_aws.NewMockCloud(c)

		SVCName := latticestore.AWSServiceName(tt.wantServiceName, "default")
		tt.wantListServiceOutput = append(tt.wantListServiceOutput, &vpclattice.ServiceSummary{
			Arn:  &tt.wantServiceArn,
			Id:   &tt.wantServiceId,
			Name: &SVCName,
		})
		input := &latticemodel.Service{
			Spec: latticemodel.ServiceSpec{
				Name:               tt.wantServiceName,
				Namespace:          "default",
				ServiceNetworkName: tt.meshName,
			},
		}
		listMeshServiceAssociationsOutput := []*vpclattice.ServiceNetworkServiceAssociationSummary{&vpclattice.ServiceNetworkServiceAssociationSummary{
			Status: &tt.wantMeshServiceAssociationStatus,
		}}

		mockVpcLatticeSess.EXPECT().ListServicesAsList(ctx, gomock.Any()).Return(tt.wantListServiceOutput, nil)
		mockVpcLatticeSess.EXPECT().ListServiceNetworkServiceAssociationsAsList(ctx, gomock.Any()).Return(listMeshServiceAssociationsOutput, tt.wantListMeshServiceAssociationsErr)
		if tt.wantListMeshServiceAssociationsErr == nil {
			mockVpcLatticeSess.EXPECT().DeleteServiceNetworkServiceAssociationWithContext(ctx, gomock.Any()).Return(tt.deleteServiceNetworkServiceAssociationOutput, tt.wantErr)
		}
		mockVpcLatticeSess.EXPECT().DeleteServiceWithContext(ctx, gomock.Any()).Return(tt.deleteServiceOutput, tt.wantErr)
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
		latticeDataStore.AddServiceNetwork(tt.meshName, config.AccountID, tt.meshArn, tt.meshId, latticestore.DATASTORE_MESH_CREATED)
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
