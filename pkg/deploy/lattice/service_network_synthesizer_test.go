package lattice

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	mock_client "github.com/aws/aws-application-networking-k8s/mocks/controller-runtime/client"
	"github.com/aws/aws-application-networking-k8s/pkg/gateway"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

func Test_SynthesizeTriggeredGateways(t *testing.T) {
	now := metav1.Now()
	tests := []struct {
		name               string
		gw                 *gwv1beta1.Gateway
		gwUsedByOtherNS    bool
		snMgrErr           error
		wantSynthesizerErr error
	}{
		{
			name: "Adding a new SN successfully",
			gw: &gwv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sn-1",
				},
			},
			snMgrErr:           nil,
			wantSynthesizerErr: nil,
		},
		{
			name: "Adding a new SN associating in progress",
			gw: &gwv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name: "sn-2",
				},
			},
			snMgrErr:           errors.New(LATTICE_RETRY),
			wantSynthesizerErr: errors.New(LATTICE_RETRY),
		},

		{
			name: "Deleting SN Successfully",
			gw: &gwv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "sn-3",
					Finalizers:        []string{"gateway.k8s.aws/resources"},
					DeletionTimestamp: &now,
				},
			},
			snMgrErr:           nil,
			gwUsedByOtherNS:    false,
			wantSynthesizerErr: nil,
		},
		{
			name: "Deleting SN Skipped due to other NS still uses it",
			gw: &gwv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "sn-3",
					Finalizers:        []string{"gateway.k8s.aws/resources"},
					DeletionTimestamp: &now,
				},
			},
			snMgrErr:           nil,
			gwUsedByOtherNS:    true,
			wantSynthesizerErr: nil,
		},
		{
			name: "Deleting SN Successfully in progress",
			gw: &gwv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "sn-4",
					Finalizers:        []string{"gateway.k8s.aws/resources"},
					DeletionTimestamp: &now,
				},
			},
			snMgrErr:           errors.New(LATTICE_RETRY),
			wantSynthesizerErr: errors.New(LATTICE_RETRY),
		},
	}

	c := gomock.NewController(t)
	defer c.Finish()
	k8sClient := mock_client.NewMockClient(c)
	k8sClient.EXPECT().List(context.Background(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := gomock.NewController(t)
			defer c.Finish()
			ctx := context.TODO()

			builder := gateway.NewServiceNetworkModelBuilder(k8sClient)
			stack, sn, _ := builder.Build(context.Background(), tt.gw)

			var status model.ServiceNetworkStatus
			mockSvcNetworkMgr := NewMockServiceNetworkManager(c)

			// testing deleting staled sn (gateway)
			mockClient := mock_client.NewMockClient(c)

			// testing add or delete of triggered gateway(sn)
			if !tt.gw.DeletionTimestamp.IsZero() {
				// testing delete

				gwList := &gwv1beta1.GatewayList{}

				gwList.Items = append(gwList.Items,
					gwv1beta1.Gateway{
						ObjectMeta: metav1.ObjectMeta{
							Name: tt.gw.GetObjectMeta().GetName(),
						},
					})
				if tt.gwUsedByOtherNS {
					gwList.Items = append(gwList.Items,
						gwv1beta1.Gateway{
							ObjectMeta: metav1.ObjectMeta{
								Name:      tt.gw.GetObjectMeta().GetName(),
								Namespace: "non-default",
							},
						},
					)
				}

				mockClient.EXPECT().List(ctx, gomock.Any(), gomock.Any()).DoAndReturn(
					func(ctx context.Context, retGWList *gwv1beta1.GatewayList, arg3 ...interface{}) error {
						// return empty gatway
						for _, gw := range gwList.Items {
							retGWList.Items = append(retGWList.Items, gw)
						}
						return nil

					},
				)

				if !tt.gwUsedByOtherNS {
					mockSvcNetworkMgr.EXPECT().Delete(ctx, tt.gw.Name).Return(tt.snMgrErr)
				}
			} else {
				status = model.ServiceNetworkStatus{ServiceNetworkARN: "testing arn", ServiceNetworkID: "87654321"}
				mockSvcNetworkMgr.EXPECT().CreateOrUpdate(ctx, sn).Return(status, tt.snMgrErr)
			}

			snSynthesizer := NewServiceNetworkSynthesizer(gwlog.FallbackLogger, mockClient, mockSvcNetworkMgr, stack)
			err := snSynthesizer.synthesizeTriggeredGateways(ctx)
			assert.Equal(t, tt.wantSynthesizerErr, err)
		})
	}
}

type sdkSvcNetworkDef struct {
	name         string
	isStale      bool
	snManagerErr error
}

func Test_SynthesizeSDKSvcNetworks(t *testing.T) {
	tests := []struct {
		name                string
		sdkSvcNetworks      []sdkSvcNetworkDef
		wantSynthesizerErr  error
		wantDataStoreErr    error
		wantDataStoreStatus string
	}{
		{
			name:                "Deleting SDK SN Successfully",
			sdkSvcNetworks:      []sdkSvcNetworkDef{{name: "sdkSvcNetwork1", isStale: true, snManagerErr: nil}},
			wantSynthesizerErr:  nil,
			wantDataStoreErr:    nil,
			wantDataStoreStatus: "",
		},
		{
			name: "Deleting one stale SDK SN Successfully and keep rest",
			sdkSvcNetworks: []sdkSvcNetworkDef{
				{name: "sdkSvcNetwork21", isStale: false, snManagerErr: nil},
				{name: "sdkSvcNetwork22", isStale: true, snManagerErr: nil},
				{name: "sdkSvcNetwork23", isStale: false, snManagerErr: nil}},
			wantSynthesizerErr:  nil,
			wantDataStoreErr:    nil,
			wantDataStoreStatus: "",
		},
		{
			name: "Deleting one stale SDK SN work-in-progress and keep rest",
			sdkSvcNetworks: []sdkSvcNetworkDef{
				{name: "sdkSvcNetwork21", isStale: false, snManagerErr: nil},
				{name: "sdkSvcNetwork22", isStale: true, snManagerErr: errors.New("delete-in-progress")},
				{name: "sdkSvcNetwork23", isStale: false, snManagerErr: nil}},
			wantSynthesizerErr:  errors.New(LATTICE_RETRY),
			wantDataStoreErr:    nil,
			wantDataStoreStatus: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := gomock.NewController(t)
			defer c.Finish()
			ctx := context.TODO()

			mockSnMgr := NewMockServiceNetworkManager(c)

			mockClient := mock_client.NewMockClient(c)
			sdkItemsReturned := []string{}

			if len(tt.sdkSvcNetworks) > 0 {
				fmt.Printf("Testing deleting non-existing SDK SN")

				gwList := &gwv1beta1.GatewayList{}

				for _, sdkSvcNetwork := range tt.sdkSvcNetworks {
					fmt.Printf("sdkSvcNetwork %v\n", sdkSvcNetwork)
					sdkItemsReturned = append(sdkItemsReturned, sdkSvcNetwork.name)
					fmt.Printf("sdkSvcNetworksReturned --loop %v\n", sdkItemsReturned)

					if !sdkSvcNetwork.isStale {
						gwList.Items = append(gwList.Items,
							gwv1beta1.Gateway{
								ObjectMeta: metav1.ObjectMeta{
									Name: sdkSvcNetwork.name,
								},
							})

					}
				}

				mockClient.EXPECT().List(ctx, gomock.Any(), gomock.Any()).DoAndReturn(
					func(ctx context.Context, retGWList *gwv1beta1.GatewayList, arg3 ...interface{}) error {
						for _, gw := range gwList.Items {
							retGWList.Items = append(retGWList.Items, gw)

						}
						return nil

					},
				)

				for _, sdkSvcNetwork := range tt.sdkSvcNetworks {
					if sdkSvcNetwork.isStale {
						// first add this datastore and see if it can be deleted byy business logic
						mockSnMgr.EXPECT().Delete(ctx, sdkSvcNetwork.name).Return(sdkSvcNetwork.snManagerErr)

					}
				}
			}

			mockSnMgr.EXPECT().List(ctx).Return(sdkItemsReturned, nil)
			snSynthesizer := NewServiceNetworkSynthesizer(gwlog.FallbackLogger, mockClient, mockSnMgr, nil)
			err := snSynthesizer.synthesizeSDKServiceNetworks(ctx)
			assert.Equal(t, tt.wantSynthesizerErr, err)
		})
	}
}
