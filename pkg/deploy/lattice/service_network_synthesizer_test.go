package lattice

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gateway_api "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/gateway"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"

	mock_client "github.com/aws/aws-application-networking-k8s/mocks/controller-runtime/client"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

func Test_SynthesizeTriggeredGateways(t *testing.T) {
	now := metav1.Now()
	tests := []struct {
		name                string
		gw                  *gateway_api.Gateway
		gwUsedByOtherNS     bool
		meshManagerErr      error
		wantSynthesizerErr  error
		wantDataStoreErr    error
		wantDataStoreStatus string
	}{
		{
			name: "Adding a new Mesh successfully",
			gw: &gateway_api.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name: "mesh1",
				},
			},
			meshManagerErr:      nil,
			wantSynthesizerErr:  nil,
			wantDataStoreErr:    nil,
			wantDataStoreStatus: "",
		},
		{
			name: "Adding a new Mesh associating in progress",
			gw: &gateway_api.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name: "mesh2",
				},
			},
			meshManagerErr:      errors.New(LATTICE_RETRY),
			wantSynthesizerErr:  errors.New(LATTICE_RETRY),
			wantDataStoreErr:    nil,
			wantDataStoreStatus: "",
		},

		{
			name: "Deleting Mesh Successfully",
			gw: &gateway_api.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "mesh3",
					Finalizers:        []string{"gateway.k8s.aws/resources"},
					DeletionTimestamp: &now,
				},
			},
			meshManagerErr:      nil,
			gwUsedByOtherNS:     false,
			wantSynthesizerErr:  nil,
			wantDataStoreErr:    errors.New(latticestore.DATASTORE_SERVICE_NETWORK_NOT_EXIST),
			wantDataStoreStatus: latticestore.DATASTORE_SERVICE_NETWORK_NOT_EXIST,
		},
		{
			name: "Deleting Mesh Skipped due to other NS still uses it",
			gw: &gateway_api.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "mesh3",
					Finalizers:        []string{"gateway.k8s.aws/resources"},
					DeletionTimestamp: &now,
				},
			},
			meshManagerErr:      nil,
			gwUsedByOtherNS:     true,
			wantSynthesizerErr:  nil,
			wantDataStoreErr:    nil,
			wantDataStoreStatus: "",
		},
		{
			name: "Deleting Mesh Successfully in progress",
			gw: &gateway_api.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "mesh4",
					Finalizers:        []string{"gateway.k8s.aws/resources"},
					DeletionTimestamp: &now,
				},
			},
			meshManagerErr:      errors.New(LATTICE_RETRY),
			wantSynthesizerErr:  errors.New(LATTICE_RETRY),
			wantDataStoreErr:    nil,
			wantDataStoreStatus: "",
		},
	}

	for _, tt := range tests {

		fmt.Printf("Testing >>>>> %v\n", tt.name)
		c := gomock.NewController(t)
		defer c.Finish()
		ctx := context.TODO()

		builder := gateway.NewServiceNetworkModelBuilder()

		stack, mesh, _ := builder.Build(context.Background(), tt.gw)

		var meshStatus latticemodel.ServiceNetworkStatus

		ds := latticestore.NewLatticeDataStore()

		mockMeshManager := NewMockServiceNetworkManager(c)

		// testing deleting staled mesh (gateway)
		mock_client := mock_client.NewMockClient(c)

		// testing add or delete of triggered gateway(mesh)
		if !tt.gw.DeletionTimestamp.IsZero() {
			// testing delete
			// insert the record in cache and verify it will be deleted later
			ds.AddServiceNetwork(tt.gw.Name, config.AccountID, "ARN", "id", latticestore.DATASTORE_SERVICE_NETWORK_CREATED)

			gwList := &gateway_api.GatewayList{}

			gwList.Items = append(gwList.Items,
				gateway_api.Gateway{
					ObjectMeta: metav1.ObjectMeta{
						Name: tt.gw.GetObjectMeta().GetName(),
					},
				})
			if tt.gwUsedByOtherNS {
				gwList.Items = append(gwList.Items,
					gateway_api.Gateway{
						ObjectMeta: metav1.ObjectMeta{
							Name:      tt.gw.GetObjectMeta().GetName(),
							Namespace: "non-default",
						},
					},
				)
			}

			mock_client.EXPECT().List(ctx, gomock.Any(), gomock.Any()).DoAndReturn(
				func(ctx context.Context, retGWList *gateway_api.GatewayList, arg3 ...interface{}) error {
					// return empty gatway
					for _, gw := range gwList.Items {
						fmt.Printf("liwwu>>> test append %v\n", gw)
						retGWList.Items = append(retGWList.Items, gw)
					}
					return nil

				},
			)

			if !tt.gwUsedByOtherNS {
				mockMeshManager.EXPECT().Delete(ctx, tt.gw.Name).Return(tt.meshManagerErr)
			}
		} else {
			meshStatus = latticemodel.ServiceNetworkStatus{ServiceNetworkARN: "testing arn", ServiceNetworkID: "87654321"}
			mockMeshManager.EXPECT().Create(ctx, mesh).Return(meshStatus, tt.meshManagerErr)
		}

		meshMeshSynthesizer := NewServiceNetworkSynthesizer(mock_client, mockMeshManager, stack, ds)
		err := meshMeshSynthesizer.synthesizeTriggeredGateways(ctx)

		assert.Equal(t, tt.wantSynthesizerErr, err)

		// verify the local cache for triggered gateway add or delete
		output, err := ds.GetServiceNetworkStatus(tt.gw.Name, config.AccountID)

		fmt.Printf("GetMeshStatus:%v, err %v\n", output, err)
		if tt.gw.DeletionTimestamp.IsZero() {
			// Verify record being added to local store correctly
			assert.Equal(t, meshStatus.ServiceNetworkARN, output.ARN)
			assert.Equal(t, meshStatus.ServiceNetworkID, output.ID)
		}

		assert.Equal(t, tt.wantDataStoreErr, err)

	}

}

type sdkMeshDef struct {
	name           string
	isStale        bool
	meshManagerErr error
}

func Test_SythesizeSDKMeshs(t *testing.T) {
	tests := []struct {
		name                string
		sdkMeshes           []sdkMeshDef
		wantSynthesizerErr  error
		wantDataStoreErr    error
		wantDataStoreStatus string
	}{
		{
			name:                "Deleting SDK Mesh Successfully",
			sdkMeshes:           []sdkMeshDef{{name: "sdkMesh1", isStale: true, meshManagerErr: nil}},
			wantSynthesizerErr:  nil,
			wantDataStoreErr:    nil,
			wantDataStoreStatus: "",
		},
		{
			name: "Deleting one stale SDKMesh Successfully and keep rest",
			sdkMeshes: []sdkMeshDef{
				{name: "sdkMesh21", isStale: false, meshManagerErr: nil},
				{name: "sdkMesh22", isStale: true, meshManagerErr: nil},
				{name: "sdkMesh23", isStale: false, meshManagerErr: nil}},
			wantSynthesizerErr:  nil,
			wantDataStoreErr:    nil,
			wantDataStoreStatus: "",
		},
		{
			name: "Deleting one stale SDKMesh work-in-progress and keep rest",
			sdkMeshes: []sdkMeshDef{
				{name: "sdkMesh21", isStale: false, meshManagerErr: nil},
				{name: "sdkMesh22", isStale: true, meshManagerErr: errors.New("delete-in-progress")},
				{name: "sdkMesh23", isStale: false, meshManagerErr: nil}},
			wantSynthesizerErr:  errors.New(LATTICE_RETRY),
			wantDataStoreErr:    nil,
			wantDataStoreStatus: "",
		},
	}

	for _, tt := range tests {
		fmt.Printf("Testing >>>>> %v\n", tt.name)
		c := gomock.NewController(t)
		defer c.Finish()
		ctx := context.TODO()

		ds := latticestore.NewLatticeDataStore()

		mockMeshManager := NewMockServiceNetworkManager(c)

		// testing deleting staled mesh (gateway)
		mock_client := mock_client.NewMockClient(c)
		sdkMeshsReturned := []string{}

		if len(tt.sdkMeshes) > 0 {
			fmt.Printf("Testing deleting non-existing SDK mesh")

			gwList := &gateway_api.GatewayList{}

			for _, sdkMesh := range tt.sdkMeshes {
				fmt.Printf("sdkMesh %v\n", sdkMesh)
				sdkMeshsReturned = append(sdkMeshsReturned, sdkMesh.name)
				fmt.Printf("sdkMeshsReturned --loop %v\n", sdkMeshsReturned)
				ds.AddServiceNetwork(sdkMesh.name, config.AccountID, "staleMeshARN", "staleMeshId", latticestore.DATASTORE_SERVICE_NETWORK_CREATED)
				if !sdkMesh.isStale {
					gwList.Items = append(gwList.Items,
						gateway_api.Gateway{
							ObjectMeta: metav1.ObjectMeta{
								Name: sdkMesh.name,
							},
						})

				}
			}

			mock_client.EXPECT().List(ctx, gomock.Any(), gomock.Any()).DoAndReturn(
				func(ctx context.Context, retGWList *gateway_api.GatewayList, arg3 ...interface{}) error {
					for _, gw := range gwList.Items {
						retGWList.Items = append(retGWList.Items, gw)

					}
					return nil

				},
			)

			for _, sdkMesh := range tt.sdkMeshes {
				if sdkMesh.isStale {
					// first add this datastore and see if it can be deleted byy business logic
					mockMeshManager.EXPECT().Delete(ctx, sdkMesh.name).Return(sdkMesh.meshManagerErr)

				}
			}
		}

		mockMeshManager.EXPECT().List(ctx).Return(sdkMeshsReturned, nil)

		meshMeshSynthesizer := NewServiceNetworkSynthesizer(mock_client, mockMeshManager, nil, ds)

		err := meshMeshSynthesizer.synthesizeSDKServiceNetworks(ctx)

		assert.Equal(t, tt.wantSynthesizerErr, err)

	}

}
