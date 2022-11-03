package lattice

import (
	"context"
	"errors"
	"github.com/aws/aws-sdk-go/aws"
	"testing"

	"github.com/aws/aws-sdk-go/service/mercury"
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
		wantListServiceOutput            []*mercury.ServiceSummary
		listServiceInput                 *mercury.ListServicesInput
	}{
		{
			tags:                             nil,
			meshName:                         "test-mesh-1",
			meshId:                           "id-234567890",
			meshArn:                          "arn-234567890",
			wantServiceName:                  "svc-test-1",
			wantServiceId:                    "id-123456789",
			wantServiceArn:                   "arn-123456789",
			wantMeshServiceAssociationStatus: mercury.MeshServiceAssociationStatusActive,
			wantErr:                          nil,
			wantListServiceOutput:            []*mercury.ServiceSummary{},
			listServiceInput:                 &mercury.ListServicesInput{},
		},
	}

	for _, tt := range tests {
		c := gomock.NewController(t)
		defer c.Finish()
		ctx := context.TODO()
		mockMercurySess := mocks.NewMockMercury(c)
		latticeDataStore := latticestore.NewLatticeDataStore()
		latticeDataStore.AddServiceNetwork(tt.meshName, config.AccountID, tt.meshArn, tt.meshId, latticestore.DATASTORE_SERVICE_NETWORK_CREATED)
		mockCloud := mocks_aws.NewMockCloud(c)

		SVCName := latticestore.AWSServiceName(tt.wantServiceName, "default")
		createServiceOutput := &mercury.CreateServiceOutput{
			Arn:    &tt.wantServiceArn,
			Id:     &tt.wantServiceId,
			Name:   &SVCName,
			Status: aws.String(mercury.ServiceStatusActive),
		}

		createMeshServiceAssociationOutput := &mercury.CreateMeshServiceAssociationOutput{
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

		createServiceInput := &mercury.CreateServiceInput{
			Name: &SVCName,
			Tags: nil,
		}
		associateMeshService := &mercury.CreateMeshServiceAssociationInput{
			MeshIdentifier:    &tt.meshId,
			ServiceIdentifier: &tt.wantServiceId,
		}

		mockMercurySess.EXPECT().ListServicesAsList(ctx, tt.listServiceInput).Return(tt.wantListServiceOutput, nil)
		mockMercurySess.EXPECT().CreateServiceWithContext(ctx, createServiceInput).Return(createServiceOutput, nil)
		mockMercurySess.EXPECT().CreateMeshServiceAssociationWithContext(ctx, associateMeshService).Return(createMeshServiceAssociationOutput, tt.wantErr)
		mockCloud.EXPECT().Mercury().Return(mockMercurySess).AnyTimes()

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
		wantListServiceOutput            []*mercury.ServiceSummary
	}{
		{
			tags:                             nil,
			meshName:                         "test-mesh-1",
			meshId:                           "id-234567890",
			meshArn:                          "arn-234567890",
			wantServiceName:                  "svc-test-1",
			wantServiceId:                    "id-123456789",
			wantServiceArn:                   "arn-123456789",
			wantMeshServiceAssociationStatus: mercury.MeshServiceAssociationStatusActive,
			wantErr:                          nil,
			wantListServiceOutput:            []*mercury.ServiceSummary{},
		},
		{
			tags:                             nil,
			meshName:                         "test-mesh-2",
			meshId:                           "id-234567890",
			meshArn:                          "arn-234567890",
			wantServiceName:                  "svc-test-2",
			wantServiceId:                    "id-123456789",
			wantServiceArn:                   "arn-123456789",
			wantMeshServiceAssociationStatus: mercury.MeshServiceAssociationStatusCreateInProgress,
			wantErr:                          errors.New(LATTICE_RETRY),
			wantListServiceOutput:            []*mercury.ServiceSummary{},
		},
		{
			tags:                             nil,
			meshName:                         "test-mesh-3",
			meshId:                           "id-234567890",
			meshArn:                          "arn-234567890",
			wantServiceName:                  "svc-test-3",
			wantServiceId:                    "id-123456789",
			wantServiceArn:                   "arn-123456789",
			wantMeshServiceAssociationStatus: mercury.MeshServiceAssociationStatusActive,
			wantErr:                          errors.New(LATTICE_RETRY),
			wantListServiceOutput:            []*mercury.ServiceSummary{},
		},
		{
			tags:                             nil,
			meshName:                         "test-mesh-4",
			meshId:                           "id-234567890",
			meshArn:                          "arn-234567890",
			wantServiceName:                  "svc-test-4",
			wantServiceId:                    "id-123456789",
			wantServiceArn:                   "arn-123456789",
			wantMeshServiceAssociationStatus: mercury.MeshServiceAssociationStatusDeleteInProgress,
			wantErr:                          errors.New(LATTICE_RETRY),
			wantListServiceOutput:            []*mercury.ServiceSummary{},
		},
		{
			tags:                             nil,
			meshName:                         "test-mesh-5",
			meshId:                           "id-234567890",
			meshArn:                          "arn-234567890",
			wantServiceName:                  "svc-test-5",
			wantServiceId:                    "id-123456789",
			wantServiceArn:                   "arn-123456789",
			wantMeshServiceAssociationStatus: mercury.MeshServiceAssociationStatusDeleteFailed,
			wantErr:                          errors.New(LATTICE_RETRY),
			wantListServiceOutput:            []*mercury.ServiceSummary{},
		},
	}

	for _, tt := range tests {
		c := gomock.NewController(t)
		defer c.Finish()
		ctx := context.TODO()
		mockMercurySess := mocks.NewMockMercury(c)
		latticeDataStore := latticestore.NewLatticeDataStore()
		latticeDataStore.AddServiceNetwork(tt.meshName, config.AccountID, tt.meshArn, tt.meshId, latticestore.DATASTORE_SERVICE_NETWORK_CREATED)
		mockCloud := mocks_aws.NewMockCloud(c)

		createServiceOutput := &mercury.CreateServiceOutput{
			Arn:    &tt.wantServiceArn,
			Id:     &tt.wantServiceId,
			Name:   &tt.wantServiceName,
			Status: aws.String(mercury.ServiceStatusActive),
		}
		createMeshServiceAssociationOutput := &mercury.CreateMeshServiceAssociationOutput{
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

		mockMercurySess.EXPECT().ListServicesAsList(ctx, gomock.Any()).Return(tt.wantListServiceOutput, nil)
		mockMercurySess.EXPECT().CreateServiceWithContext(ctx, gomock.Any()).Return(createServiceOutput, nil)
		mockMercurySess.EXPECT().CreateMeshServiceAssociationWithContext(ctx, gomock.Any()).Return(createMeshServiceAssociationOutput, tt.wantErr)
		mockCloud.EXPECT().Mercury().Return(mockMercurySess).AnyTimes()

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
		wantListServiceOutput            []*mercury.ServiceSummary
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
			wantListServiceOutput:            []*mercury.ServiceSummary{},
			existingAssociationStatus:        mercury.MeshServiceAssociationStatusCreateInProgress,
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
			wantListServiceOutput:            []*mercury.ServiceSummary{},
			existingAssociationStatus:        mercury.MeshServiceAssociationStatusDeleteInProgress,
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
			wantListServiceOutput:            []*mercury.ServiceSummary{},
			existingAssociationStatus:        mercury.MeshServiceAssociationStatusDeleteFailed,
			existingAssociationErr:           nil,
			wantMeshServiceAssociationStatus: mercury.MeshServiceAssociationStatusActive,
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
			wantListServiceOutput:            []*mercury.ServiceSummary{},
			existingAssociationStatus:        mercury.MeshServiceAssociationStatusCreateFailed,
			existingAssociationErr:           nil,
			wantMeshServiceAssociationStatus: mercury.MeshServiceAssociationStatusActive,
			wantErr:                          nil,
		},
	}

	for _, tt := range tests {
		c := gomock.NewController(t)
		defer c.Finish()
		ctx := context.TODO()
		mockMercurySess := mocks.NewMockMercury(c)
		latticeDataStore := latticestore.NewLatticeDataStore()
		latticeDataStore.AddServiceNetwork(tt.meshName, config.AccountID, tt.meshArn, tt.meshId, latticestore.DATASTORE_SERVICE_NETWORK_CREATED)
		mockCloud := mocks_aws.NewMockCloud(c)

		createMeshServiceAssociationOutput := &mercury.CreateMeshServiceAssociationOutput{
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
		tt.wantListServiceOutput = append(tt.wantListServiceOutput, &mercury.ServiceSummary{
			Arn:  &tt.wantServiceArn,
			Id:   &tt.wantServiceId,
			Name: &SVCName,
		})
		listMeshServiceAssociationsOutput := []*mercury.MeshServiceAssociationSummary{&mercury.MeshServiceAssociationSummary{
			Status: &tt.existingAssociationStatus,
		}}

		mockMercurySess.EXPECT().ListServicesAsList(ctx, gomock.Any()).Return(tt.wantListServiceOutput, nil)
		mockMercurySess.EXPECT().ListMeshServiceAssociationsAsList(ctx, gomock.Any()).Return(listMeshServiceAssociationsOutput, tt.existingAssociationErr)
		if tt.existingAssociationErr == nil {
			mockMercurySess.EXPECT().CreateMeshServiceAssociationWithContext(ctx, gomock.Any()).Return(createMeshServiceAssociationOutput, tt.wantErr)
		}
		mockCloud.EXPECT().Mercury().Return(mockMercurySess).AnyTimes()

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
		wantListServiceOutput            []*mercury.ServiceSummary
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
			wantListServiceOutput:            []*mercury.ServiceSummary{},
			existingAssociationStatus:        mercury.MeshServiceAssociationStatusActive,
			existingAssociationErr:           nil,
			wantMeshServiceAssociationStatus: mercury.MeshServiceAssociationStatusActive,
			wantErr:                          nil,
		},
	}

	for _, tt := range tests {
		c := gomock.NewController(t)
		defer c.Finish()
		ctx := context.TODO()
		mockMercurySess := mocks.NewMockMercury(c)
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
		tt.wantListServiceOutput = append(tt.wantListServiceOutput, &mercury.ServiceSummary{
			Arn:  &tt.wantServiceArn,
			Id:   &tt.wantServiceId,
			Name: &SVCName,
		})
		listMeshServiceAssociationsOutput := []*mercury.MeshServiceAssociationSummary{&mercury.MeshServiceAssociationSummary{
			Status: &tt.existingAssociationStatus,
		}}

		mockMercurySess.EXPECT().ListServicesAsList(ctx, gomock.Any()).Return(tt.wantListServiceOutput, nil)
		mockMercurySess.EXPECT().ListMeshServiceAssociationsAsList(ctx, gomock.Any()).Return(listMeshServiceAssociationsOutput, tt.existingAssociationErr)
		mockCloud.EXPECT().Mercury().Return(mockMercurySess).AnyTimes()

		serviceManager := NewServiceManager(mockCloud, latticeDataStore)
		resp, err := serviceManager.Create(ctx, input)

		assert.Nil(t, err)
		assert.Equal(t, resp.ServiceARN, tt.wantServiceArn)
		assert.Equal(t, resp.ServiceID, tt.wantServiceId)
	}
}
func Test_Delete_ValidateInput(t *testing.T) {
	tests := []struct {
		meshName                           string
		meshId                             string
		meshArn                            string
		wantServiceName                    string
		wantServiceId                      string
		wantServiceArn                     string
		wantMeshServiceAssociationStatus   string
		wantErr                            error
		wantListServiceOutput              []*mercury.ServiceSummary
		deleteMeshServiceAssociationOutput *mercury.DeleteMeshServiceAssociationOutput
		deleteServiceOutput                *mercury.DeleteServiceOutput
		wantListMeshServiceAssociationsErr error
		meshServiceAssociationId           string
	}{
		{
			meshName:                           "test-mesh-1",
			meshId:                             "id-234567890",
			meshArn:                            "arn-234567890",
			wantServiceName:                    "svc-test-1",
			wantServiceId:                      "id-123456789",
			wantServiceArn:                     "arn-123456789",
			wantMeshServiceAssociationStatus:   mercury.MeshServiceAssociationStatusActive,
			wantErr:                            nil,
			wantListServiceOutput:              []*mercury.ServiceSummary{},
			deleteMeshServiceAssociationOutput: &mercury.DeleteMeshServiceAssociationOutput{},
			deleteServiceOutput:                &mercury.DeleteServiceOutput{},
			wantListMeshServiceAssociationsErr: nil,
			meshServiceAssociationId:           "mesh-svc-id-123456789",
		},
	}

	for _, tt := range tests {
		c := gomock.NewController(t)
		defer c.Finish()
		ctx := context.TODO()
		mockMercurySess := mocks.NewMockMercury(c)
		latticeDataStore := latticestore.NewLatticeDataStore()
		latticeDataStore.AddServiceNetwork(tt.meshName, config.AccountID, tt.meshArn, tt.meshId, latticestore.DATASTORE_SERVICE_NETWORK_CREATED)
		mockCloud := mocks_aws.NewMockCloud(c)

		SVCName := latticestore.AWSServiceName(tt.wantServiceName, "default")
		tt.wantListServiceOutput = append(tt.wantListServiceOutput, &mercury.ServiceSummary{
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
		listMeshServiceAssociationsOutput := []*mercury.MeshServiceAssociationSummary{&mercury.MeshServiceAssociationSummary{
			Status: &tt.wantMeshServiceAssociationStatus,
			Id:     &tt.meshServiceAssociationId,
		}}

		listServicesInput := &mercury.ListServicesInput{}
		//deleteServiceRoutingConfigurationInput := &mercury.DeleteServiceRoutingConfigurationInput{ServiceIdentifier: &tt.wantServiceId}
		listMeshServiceAssociationsInput := &mercury.ListMeshServiceAssociationsInput{
			MeshIdentifier:    &tt.meshId,
			ServiceIdentifier: &tt.wantServiceId,
		}
		deleteMeshServiceAssociationInput := &mercury.DeleteMeshServiceAssociationInput{MeshServiceAssociationIdentifier: &tt.meshServiceAssociationId}

		mockMercurySess.EXPECT().ListServicesAsList(ctx, listServicesInput).Return(tt.wantListServiceOutput, nil)
		//mockMercurySess.EXPECT().DeleteServiceRoutingConfigurationWithContext(ctx, deleteServiceRoutingConfigurationInput).Return(tt.deleteServiceRoutingConfigurationOutput, nil)
		mockMercurySess.EXPECT().ListMeshServiceAssociationsAsList(ctx, listMeshServiceAssociationsInput).Return(listMeshServiceAssociationsOutput, tt.wantListMeshServiceAssociationsErr)

		mockMercurySess.EXPECT().DeleteMeshServiceAssociationWithContext(ctx, deleteMeshServiceAssociationInput).Return(tt.deleteMeshServiceAssociationOutput, tt.wantErr)

		mockMercurySess.EXPECT().DeleteServiceWithContext(ctx, gomock.Any()).Return(tt.deleteServiceOutput, tt.wantErr)
		mockCloud.EXPECT().Mercury().Return(mockMercurySess).AnyTimes()

		serviceManager := NewServiceManager(mockCloud, latticeDataStore)
		err := serviceManager.Delete(ctx, input)

		assert.Nil(t, err)

	}
}

func Test_Delete_Disassociation_DeleteService(t *testing.T) {
	tests := []struct {
		meshName                           string
		meshId                             string
		meshArn                            string
		wantServiceName                    string
		wantServiceId                      string
		wantServiceArn                     string
		wantMeshServiceAssociationStatus   string
		wantErr                            error
		wantListServiceOutput              []*mercury.ServiceSummary
		deleteMeshServiceAssociationOutput *mercury.DeleteMeshServiceAssociationOutput
		deleteServiceOutput                *mercury.DeleteServiceOutput
		wantListMeshServiceAssociationsErr error
	}{
		{
			meshName:                           "test-mesh-1",
			meshId:                             "id-234567890",
			meshArn:                            "arn-234567890",
			wantServiceName:                    "svc-test-1",
			wantServiceId:                      "id-123456789",
			wantServiceArn:                     "arn-123456789",
			wantMeshServiceAssociationStatus:   mercury.MeshServiceAssociationStatusActive,
			wantErr:                            nil,
			wantListServiceOutput:              []*mercury.ServiceSummary{},
			deleteMeshServiceAssociationOutput: &mercury.DeleteMeshServiceAssociationOutput{},
			deleteServiceOutput:                &mercury.DeleteServiceOutput{},
			wantListMeshServiceAssociationsErr: nil,
		},
		{
			meshName:                           "test-mesh-1",
			meshId:                             "id-234567890",
			meshArn:                            "arn-234567890",
			wantServiceName:                    "svc-test-1",
			wantServiceId:                      "id-123456789",
			wantServiceArn:                     "arn-123456789",
			wantMeshServiceAssociationStatus:   mercury.MeshServiceAssociationStatusActive,
			wantErr:                            errors.New(LATTICE_RETRY),
			wantListServiceOutput:              []*mercury.ServiceSummary{},
			deleteMeshServiceAssociationOutput: &mercury.DeleteMeshServiceAssociationOutput{},
			deleteServiceOutput:                &mercury.DeleteServiceOutput{},
			wantListMeshServiceAssociationsErr: nil,
		},
		{
			meshName:                           "test-mesh-1",
			meshId:                             "id-234567890",
			meshArn:                            "arn-234567890",
			wantServiceName:                    "svc-test-1",
			wantServiceId:                      "id-123456789",
			wantServiceArn:                     "arn-123456789",
			wantMeshServiceAssociationStatus:   mercury.MeshServiceAssociationStatusActive,
			wantErr:                            errors.New(LATTICE_RETRY),
			wantListServiceOutput:              []*mercury.ServiceSummary{},
			deleteMeshServiceAssociationOutput: &mercury.DeleteMeshServiceAssociationOutput{},
			deleteServiceOutput:                &mercury.DeleteServiceOutput{},
			wantListMeshServiceAssociationsErr: errors.New(LATTICE_RETRY),
		},
	}

	for _, tt := range tests {
		c := gomock.NewController(t)
		defer c.Finish()
		ctx := context.TODO()
		mockMercurySess := mocks.NewMockMercury(c)
		latticeDataStore := latticestore.NewLatticeDataStore()
		latticeDataStore.AddServiceNetwork(tt.meshName, config.AccountID, tt.meshArn, tt.meshId, latticestore.DATASTORE_SERVICE_NETWORK_CREATED)
		mockCloud := mocks_aws.NewMockCloud(c)

		SVCName := latticestore.AWSServiceName(tt.wantServiceName, "default")
		tt.wantListServiceOutput = append(tt.wantListServiceOutput, &mercury.ServiceSummary{
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
		listMeshServiceAssociationsOutput := []*mercury.MeshServiceAssociationSummary{&mercury.MeshServiceAssociationSummary{
			Status: &tt.wantMeshServiceAssociationStatus,
		}}

		mockMercurySess.EXPECT().ListServicesAsList(ctx, gomock.Any()).Return(tt.wantListServiceOutput, nil)
		mockMercurySess.EXPECT().ListMeshServiceAssociationsAsList(ctx, gomock.Any()).Return(listMeshServiceAssociationsOutput, tt.wantListMeshServiceAssociationsErr)
		if tt.wantListMeshServiceAssociationsErr == nil {
			mockMercurySess.EXPECT().DeleteMeshServiceAssociationWithContext(ctx, gomock.Any()).Return(tt.deleteMeshServiceAssociationOutput, tt.wantErr)
		}
		mockMercurySess.EXPECT().DeleteServiceWithContext(ctx, gomock.Any()).Return(tt.deleteServiceOutput, tt.wantErr)
		mockCloud.EXPECT().Mercury().Return(mockMercurySess).AnyTimes()

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
		wantListServiceOutput              []*mercury.ServiceSummary
		deleteMeshServiceAssociationOutput *mercury.DeleteMeshServiceAssociationOutput
		deleteServiceOutput                *mercury.DeleteServiceOutput
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
			wantMeshServiceAssociationStatus:   mercury.MeshServiceAssociationStatusActive,
			wantErr:                            errors.New(LATTICE_RETRY),
			wantListServiceOutput:              []*mercury.ServiceSummary{},
			deleteMeshServiceAssociationOutput: &mercury.DeleteMeshServiceAssociationOutput{},
			deleteServiceOutput:                &mercury.DeleteServiceOutput{},
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
			wantMeshServiceAssociationStatus:   mercury.MeshServiceAssociationStatusActive,
			wantErr:                            nil,
			wantListServiceOutput:              []*mercury.ServiceSummary{},
			deleteMeshServiceAssociationOutput: &mercury.DeleteMeshServiceAssociationOutput{},
			deleteServiceOutput:                &mercury.DeleteServiceOutput{},
			wantListServiceErr:                 errors.New(LATTICE_RETRY),
			wantListMeshServiceAssociationsErr: nil,
		},
	}

	for _, tt := range tests {
		c := gomock.NewController(t)
		defer c.Finish()
		ctx := context.TODO()
		mockMercurySess := mocks.NewMockMercury(c)
		latticeDataStore := latticestore.NewLatticeDataStore()
		latticeDataStore.AddServiceNetwork(tt.meshName, config.AccountID, tt.meshArn, tt.meshId, latticestore.DATASTORE_MESH_CREATED)
		mockCloud := mocks_aws.NewMockCloud(c)

		tt.wantListServiceOutput = append(tt.wantListServiceOutput, &mercury.ServiceSummary{
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
		listMeshServiceAssociationsOutput := []*mercury.MeshServiceAssociationSummary{&mercury.MeshServiceAssociationSummary{
			Status: &tt.wantMeshServiceAssociationStatus,
		}}

		mockMercurySess.EXPECT().ListServicesAsList(ctx, gomock.Any()).Return(tt.wantListServiceOutput, tt.wantListServiceErr)
		if tt.wantListServiceErr == nil {
			mockMercurySess.EXPECT().ListMeshServiceAssociationsAsList(ctx, gomock.Any()).Return(listMeshServiceAssociationsOutput, tt.wantListMeshServiceAssociationsErr)
		}
		mockCloud.EXPECT().Mercury().Return(mockMercurySess).AnyTimes()

		serviceManager := NewServiceManager(mockCloud, latticeDataStore)
		err := serviceManager.Delete(ctx, input)

		assert.Equal(t, err, tt.wantErr)
	}
}
*/
