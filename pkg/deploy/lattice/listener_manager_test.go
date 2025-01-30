package lattice

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	mocks "github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

func Test_UpsertListener_NewListener(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)
	tests := []struct {
		name                         string
		listenerProtocol             string
		modelDefaultAction           *model.DefaultAction
		expectedLatticeDefaultAction *vpclattice.RuleAction
	}{
		{
			name:             "HTTP Listener",
			listenerProtocol: vpclattice.ListenerProtocolHttp,
			modelDefaultAction: &model.DefaultAction{
				FixedResponseStatusCode: aws.Int64(404),
			},
			expectedLatticeDefaultAction: &vpclattice.RuleAction{
				FixedResponse: &vpclattice.FixedResponseAction{
					StatusCode: aws.Int64(404),
				},
			},
		},
		{
			name:             "HTTPS Listener",
			listenerProtocol: vpclattice.ListenerProtocolHttps,
			modelDefaultAction: &model.DefaultAction{
				FixedResponseStatusCode: aws.Int64(404),
			},
			expectedLatticeDefaultAction: &vpclattice.RuleAction{
				FixedResponse: &vpclattice.FixedResponseAction{
					StatusCode: aws.Int64(404),
				},
			},
		},
		{
			name:             "TLS_PASSTHROUGH Listener",
			listenerProtocol: vpclattice.ListenerProtocolTlsPassthrough,
			modelDefaultAction: &model.DefaultAction{
				Forward: &model.RuleAction{
					TargetGroups: []*model.RuleTargetGroup{
						{
							LatticeTgId:        "lattice-tg-id-1",
							StackTargetGroupId: "stack-tg-id-1",
							Weight:             100,
						},
					},
				},
			},
			expectedLatticeDefaultAction: &vpclattice.RuleAction{
				Forward: &vpclattice.ForwardAction{
					TargetGroups: []*vpclattice.WeightedTargetGroup{
						{
							TargetGroupIdentifier: aws.String("lattice-tg-id-1"),
							Weight:                aws.Int64(100),
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ml := &model.Listener{
				Spec: model.ListenerSpec{
					Protocol:      tt.listenerProtocol,
					Port:          8181,
					DefaultAction: tt.modelDefaultAction,
				},
			}
			assert.NoError(t, ml.Spec.Validate())
			ms := &model.Service{
				Status: &model.ServiceStatus{Id: "svc-id"},
			}
			assert.NoError(t, ml.Spec.Validate())
			mockLattice.EXPECT().ListListenersWithContext(ctx, gomock.Any()).Return(
				&vpclattice.ListListenersOutput{Items: []*vpclattice.ListenerSummary{}}, nil)

			mockLattice.EXPECT().CreateListenerWithContext(ctx, gomock.Any()).DoAndReturn(
				func(ctx aws.Context, input *vpclattice.CreateListenerInput, opts ...request.Option) (*vpclattice.CreateListenerOutput, error) {
					assert.Equal(t, tt.expectedLatticeDefaultAction, input.DefaultAction)
					return &vpclattice.CreateListenerOutput{Id: aws.String("new-lid")}, nil
				},
			)
			lm := NewListenerManager(gwlog.FallbackLogger, cloud)
			status, err := lm.Upsert(ctx, ml, ms)
			assert.Nil(t, err)
			assert.Equal(t, "new-lid", status.Id)
			assert.Equal(t, "svc-id", status.ServiceId)
		})
	}
}

func Test_UpsertListener_DoNotNeedToUpdateExistingHTTPAndHTTPSListener(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	ms := &model.Service{
		Status: &model.ServiceStatus{Id: "svc-id"},
	}
	for _, listenerProtocol := range []string{vpclattice.ListenerProtocolHttp, vpclattice.ListenerProtocolHttps} {
		t.Run(fmt.Sprintf("Existing %s Listener, do not need to update", listenerProtocol), func(t *testing.T) {
			ml := &model.Listener{
				Spec: model.ListenerSpec{
					Protocol: listenerProtocol,
					Port:     8181,
					DefaultAction: &model.DefaultAction{
						FixedResponseStatusCode: aws.Int64(404),
					},
				},
			}

			mockLattice := mocks.NewMockLattice(c)
			cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)
			mockLattice.EXPECT().ListListenersWithContext(ctx, gomock.Any()).Return(
				&vpclattice.ListListenersOutput{Items: []*vpclattice.ListenerSummary{
					{
						Arn:  aws.String("existing-arn"),
						Id:   aws.String("existing-listener-id"),
						Name: aws.String("existing-name"),
						Port: aws.Int64(8181),
					},
				}}, nil)

			mockLattice.EXPECT().GetListenerWithContext(ctx, gomock.Any()).Times(0)
			mockLattice.EXPECT().UpdateListenerWithContext(ctx, gomock.Any()).Times(0)

			lm := NewListenerManager(gwlog.FallbackLogger, cloud)
			status, err := lm.Upsert(ctx, ml, ms)
			assert.Nil(t, err)
			assert.Equal(t, "existing-listener-id", status.Id)
			assert.Equal(t, "svc-id", status.ServiceId)
			assert.Equal(t, "existing-name", status.Name)
			assert.Equal(t, "existing-arn", status.ListenerArn)

		})
	}
}
func Test_UpsertListener_Update_TLS_PASSTHROUGHListener(t *testing.T) {
	tests := []struct {
		name                            string
		oldLatticeListenerDefaultAction *vpclattice.RuleAction
		newLatticeListenerDefaultAction *vpclattice.RuleAction
		newModelListenerDefaultAction   *model.DefaultAction
		expectUpdateListenerCall        bool
	}{
		{
			name: "Old and new default actions are the same, do not need to call vpclattice.UpdateListener()",
			oldLatticeListenerDefaultAction: &vpclattice.RuleAction{
				Forward: &vpclattice.ForwardAction{
					TargetGroups: []*vpclattice.WeightedTargetGroup{
						{
							TargetGroupIdentifier: aws.String("tg-id-1"),
							Weight:                aws.Int64(15),
						},
					},
				},
			},
			newLatticeListenerDefaultAction: &vpclattice.RuleAction{
				Forward: &vpclattice.ForwardAction{
					TargetGroups: []*vpclattice.WeightedTargetGroup{
						{
							TargetGroupIdentifier: aws.String("tg-id-1"),
							Weight:                aws.Int64(15),
						},
					},
				},
			},
			newModelListenerDefaultAction: &model.DefaultAction{
				Forward: &model.RuleAction{
					TargetGroups: []*model.RuleTargetGroup{
						{
							LatticeTgId: "tg-id-1",
							Weight:      15,
						},
					},
				},
			},
			expectUpdateListenerCall: false,
		},
		{
			name: "Old and new default actions are different, need to call vpclattice.UpdateListener()",
			oldLatticeListenerDefaultAction: &vpclattice.RuleAction{
				Forward: &vpclattice.ForwardAction{
					TargetGroups: []*vpclattice.WeightedTargetGroup{
						{
							TargetGroupIdentifier: aws.String("tg-id-1"),
							Weight:                aws.Int64(15),
						},
						{
							TargetGroupIdentifier: aws.String("tg-id-2"),
							Weight:                aws.Int64(85),
						},
					},
				},
			},
			newLatticeListenerDefaultAction: &vpclattice.RuleAction{
				Forward: &vpclattice.ForwardAction{
					TargetGroups: []*vpclattice.WeightedTargetGroup{
						{
							TargetGroupIdentifier: aws.String("tg-id-1"),
							Weight:                aws.Int64(20),
						},
						{
							TargetGroupIdentifier: aws.String("tg-id-3"),
							Weight:                aws.Int64(80),
						},
					},
				},
			},
			newModelListenerDefaultAction: &model.DefaultAction{
				Forward: &model.RuleAction{
					TargetGroups: []*model.RuleTargetGroup{
						{
							LatticeTgId: "tg-id-1",
							Weight:      20,
						},
						{
							LatticeTgId: "tg-id-3",
							Weight:      80,
						},
					},
				},
			},
			expectUpdateListenerCall: true,
		},
	}

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ml := &model.Listener{
				Spec: model.ListenerSpec{
					Protocol:      vpclattice.ListenerProtocolTlsPassthrough,
					Port:          8181,
					DefaultAction: tt.newModelListenerDefaultAction,
				},
			}
			ms := &model.Service{
				Status: &model.ServiceStatus{Id: "svc-id"},
			}
			mockLattice.EXPECT().ListListenersWithContext(ctx, gomock.Any()).Return(
				&vpclattice.ListListenersOutput{Items: []*vpclattice.ListenerSummary{
					{
						Arn:      aws.String("existing-arn"),
						Id:       aws.String("existing-listener-id"),
						Name:     aws.String("existing-name"),
						Protocol: aws.String(vpclattice.ListenerProtocolTlsPassthrough),
						Port:     aws.Int64(8181),
					},
				}}, nil)

			mockLattice.EXPECT().GetListenerWithContext(ctx, &vpclattice.GetListenerInput{
				ServiceIdentifier:  aws.String("svc-id"),
				ListenerIdentifier: aws.String("existing-listener-id"),
			}).Return(
				&vpclattice.GetListenerOutput{
					DefaultAction: tt.oldLatticeListenerDefaultAction,
					Id:            aws.String("existing-listener-id"),
					Port:          aws.Int64(8181),
					Protocol:      aws.String(vpclattice.ListenerProtocolTlsPassthrough),
				}, nil).Times(1)

			if tt.expectUpdateListenerCall {
				mockLattice.EXPECT().UpdateListenerWithContext(ctx, &vpclattice.UpdateListenerInput{
					ListenerIdentifier: aws.String("existing-listener-id"),
					ServiceIdentifier:  aws.String("svc-id"),
					DefaultAction:      tt.newLatticeListenerDefaultAction,
				}).Return(&vpclattice.UpdateListenerOutput{
					Id:            aws.String("existing-listener-id"),
					DefaultAction: tt.newLatticeListenerDefaultAction,
					Port:          aws.Int64(8181),
					Protocol:      aws.String(vpclattice.ListenerProtocolTlsPassthrough),
				}, nil)
			}

			lm := NewListenerManager(gwlog.FallbackLogger, cloud)
			status, err := lm.Upsert(ctx, ml, ms)
			assert.Nil(t, err)
			assert.Equal(t, "existing-listener-id", status.Id)
			assert.Equal(t, "svc-id", status.ServiceId)
			assert.Equal(t, "existing-name", status.Name)
			assert.Equal(t, "existing-arn", status.ListenerArn)
		})
	}
}

func Test_DeleteListener(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)

	t.Run("test precondition errors", func(t *testing.T) {
		ml := &model.Listener{}

		lm := NewListenerManager(gwlog.FallbackLogger, cloud)
		assert.Error(t, lm.Delete(ctx, nil))
		assert.Error(t, lm.Delete(ctx, ml))
	})

	t.Run("listener only success", func(t *testing.T) {
		ml := &model.Listener{Status: &model.ListenerStatus{Id: "lid", ServiceId: "sid"}}
		mockLattice.EXPECT().DeleteListenerWithContext(ctx, gomock.Any()).DoAndReturn(
			func(ctx aws.Context, input *vpclattice.DeleteListenerInput, opts ...request.Option) (*vpclattice.DeleteListenerOutput, error) {
				assert.Equal(t, "lid", *input.ListenerIdentifier)
				assert.Equal(t, "sid", *input.ServiceIdentifier)
				return &vpclattice.DeleteListenerOutput{}, nil
			},
		)
		lm := NewListenerManager(gwlog.FallbackLogger, cloud)
		assert.NoError(t, lm.Delete(ctx, ml))
	})
}

func Test_defaultListenerManager_needToUpdateDefaultAction(t *testing.T) {
	latticeSvcId := "svc-id"
	latticeListenerId := "listener-id"
	forwardAction1 := &vpclattice.RuleAction{
		Forward: &vpclattice.ForwardAction{
			TargetGroups: []*vpclattice.WeightedTargetGroup{
				{
					TargetGroupIdentifier: aws.String("tg-id-1"),
					Weight:                aws.Int64(15),
				}, {
					TargetGroupIdentifier: aws.String("tg-id-2"),
					Weight:                aws.Int64(85),
				},
			},
		},
	}
	forwardAction1Copy := &vpclattice.RuleAction{
		Forward: &vpclattice.ForwardAction{
			TargetGroups: []*vpclattice.WeightedTargetGroup{
				{
					TargetGroupIdentifier: aws.String("tg-id-1"),
					Weight:                aws.Int64(15),
				}, {
					TargetGroupIdentifier: aws.String("tg-id-2"),
					Weight:                aws.Int64(85),
				},
			},
		},
	}

	assert.False(t, forwardAction1 == forwardAction1Copy) // addresses are different
	assert.Equal(t, forwardAction1, forwardAction1Copy)   // contents are the same
	forwardAction2DifferentWeight := &vpclattice.RuleAction{
		Forward: &vpclattice.ForwardAction{
			TargetGroups: []*vpclattice.WeightedTargetGroup{
				{
					TargetGroupIdentifier: aws.String("tg-id-1"),
					Weight:                aws.Int64(10),
				}, {
					TargetGroupIdentifier: aws.String("tg-id-2"),
					Weight:                aws.Int64(90),
				},
			},
		},
	}
	forwardAction3DifferentTgId := &vpclattice.RuleAction{
		Forward: &vpclattice.ForwardAction{
			TargetGroups: []*vpclattice.WeightedTargetGroup{
				{
					TargetGroupIdentifier: aws.String("tg-id-1"),
					Weight:                aws.Int64(15),
				}, {
					TargetGroupIdentifier: aws.String("tg-id-3"),
					Weight:                aws.Int64(85),
				},
			},
		},
	}
	tests := []struct {
		name                             string
		listenerDefaultActionFromStack   *vpclattice.RuleAction
		listenerDefaultActionFromLattice *vpclattice.RuleAction
		expectNeedToUpdateDefaultAction  bool
		wantErr                          error
	}{
		{
			name: "Same 404 FixedResponseAction from stack and lattice, do not need to update",
			listenerDefaultActionFromStack: &vpclattice.RuleAction{
				FixedResponse: &vpclattice.FixedResponseAction{StatusCode: aws.Int64(404)},
			},
			listenerDefaultActionFromLattice: &vpclattice.RuleAction{
				FixedResponse: &vpclattice.FixedResponseAction{StatusCode: aws.Int64(404)},
			},
			expectNeedToUpdateDefaultAction: false,
			wantErr:                         nil,
		},
		{
			name:                             "Same ForwardAction from stack and lattice, do not need to update",
			listenerDefaultActionFromStack:   forwardAction1,
			listenerDefaultActionFromLattice: forwardAction1Copy,
			expectNeedToUpdateDefaultAction:  false,
			wantErr:                          nil,
		},
		{
			name:                             "Different weight in ForwardAction from stack and lattice, need to update",
			listenerDefaultActionFromStack:   forwardAction1,
			listenerDefaultActionFromLattice: forwardAction2DifferentWeight,
			expectNeedToUpdateDefaultAction:  true,
			wantErr:                          nil,
		}, {
			name:                             "Different tg id in ForwardAction from stack and lattice, need to update",
			listenerDefaultActionFromStack:   forwardAction1,
			listenerDefaultActionFromLattice: forwardAction3DifferentTgId,
			expectNeedToUpdateDefaultAction:  true,
			wantErr:                          nil,
		},
	}
	c := gomock.NewController(t)
	defer c.Finish()
	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockLattice.EXPECT().GetListenerWithContext(context.TODO(), &vpclattice.GetListenerInput{
				ServiceIdentifier:  &latticeSvcId,
				ListenerIdentifier: &latticeListenerId,
			}).Return(&vpclattice.GetListenerOutput{
				DefaultAction: tt.listenerDefaultActionFromLattice,
			}, nil)

			d := &defaultListenerManager{
				log:   gwlog.FallbackLogger,
				cloud: cloud,
			}
			got, err := d.needToUpdateDefaultAction(context.TODO(), latticeSvcId, latticeListenerId, tt.listenerDefaultActionFromStack)
			assert.Equal(t, tt.wantErr, err)
			assert.Equal(t, tt.expectNeedToUpdateDefaultAction, got)
		})
	}
}

func Test_defaultListenerManager_getLatticeListenerDefaultAction_HTTP_HTTPS_Listener(t *testing.T) {
	latticeFixResponseAction404 := &vpclattice.RuleAction{
		FixedResponse: &vpclattice.FixedResponseAction{
			StatusCode: aws.Int64(404),
		},
	}
	tests := []struct {
		name               string
		modelDefaultAction *model.DefaultAction
		listenerProtocol   string
		want               *vpclattice.RuleAction
		wantErr            bool
	}{
		{
			name:               "HTTP protocol Listener has the 404 fixed response modelListenerDefaultAction, return lattice fixed response 404 DefaultAction",
			modelDefaultAction: &model.DefaultAction{FixedResponseStatusCode: aws.Int64(404)},
			listenerProtocol:   vpclattice.ListenerProtocolHttp,
			want:               latticeFixResponseAction404,
		},
		{
			name:               "HTTPS protocol Listener has the 404 fixed response modelListenerDefaultAction, return lattice fixed response 404 DefaultAction",
			modelDefaultAction: &model.DefaultAction{FixedResponseStatusCode: aws.Int64(404)},
			listenerProtocol:   vpclattice.ListenerProtocolHttps,
			want:               latticeFixResponseAction404,
		},
	}

	c := gomock.NewController(t)
	defer c.Finish()
	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stack := core.NewDefaultStack(core.StackID{Name: "foo", Namespace: "bar"})
			modelListener := &model.Listener{
				ResourceMeta: core.NewResourceMeta(stack, "AWS:VPCServiceNetwork::Listener", "modelListener-id"),
				Spec: model.ListenerSpec{
					StackServiceId: "stack-svc-id",
					Protocol:       tt.listenerProtocol,
					Port:           80,
					DefaultAction:  tt.modelDefaultAction,
				},
			}
			assert.NoError(t, modelListener.Spec.Validate())
			assert.NoError(t, stack.AddResource(modelListener))

			d := &defaultListenerManager{
				log:   gwlog.FallbackLogger,
				cloud: cloud,
			}
			got, err := d.getLatticeListenerDefaultAction(context.TODO(), modelListener)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.Equal(t, tt.want, got)
				assert.NoError(t, err)
			}
		})
	}
}

func Test_ListenerManager_getLatticeListenerDefaultAction_TLS_PASSTHROUGH_Listener(t *testing.T) {

	tests := []struct {
		name                       string
		modelDefaultAction         *model.DefaultAction
		wantedLatticeDefaultAction *vpclattice.RuleAction
		wantErr                    bool
	}{
		{
			name: "1 resolved LatticeTgId",
			modelDefaultAction: &model.DefaultAction{
				Forward: &model.RuleAction{
					TargetGroups: []*model.RuleTargetGroup{
						{
							LatticeTgId:        "lattice-tg-id-1",
							StackTargetGroupId: "stack-tg-id-1",
							Weight:             1,
						},
					},
				},
			},
			wantedLatticeDefaultAction: &vpclattice.RuleAction{
				Forward: &vpclattice.ForwardAction{
					TargetGroups: []*vpclattice.WeightedTargetGroup{
						{
							TargetGroupIdentifier: aws.String("lattice-tg-id-1"),
							Weight:                aws.Int64(1),
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "2 resolved LatticeTgIds, 1 InvalidBackendRefTgId",
			modelDefaultAction: &model.DefaultAction{
				Forward: &model.RuleAction{
					TargetGroups: []*model.RuleTargetGroup{
						{
							LatticeTgId: "lattice-service-export-tg-id-1",
							SvcImportTG: &model.SvcImportTargetGroup{
								K8SClusterName:      "cluster-name",
								K8SServiceName:      "svc-name",
								K8SServiceNamespace: "ns",
								VpcId:               "vpc-id",
							},
							Weight: 10,
						},
						{
							LatticeTgId:        model.InvalidBackendRefTgId,
							StackTargetGroupId: model.InvalidBackendRefTgId,
							Weight:             60,
						},
						{
							LatticeTgId:        "lattice-tg-id-2",
							StackTargetGroupId: "stack-tg-id-2",
							Weight:             30,
						},
					},
				},
			},
			wantedLatticeDefaultAction: &vpclattice.RuleAction{
				Forward: &vpclattice.ForwardAction{
					TargetGroups: []*vpclattice.WeightedTargetGroup{
						{
							TargetGroupIdentifier: aws.String("lattice-service-export-tg-id-1"),
							Weight:                aws.Int64(10),
						},
						{
							TargetGroupIdentifier: aws.String("lattice-tg-id-2"),
							Weight:                aws.Int64(30),
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "All InvalidBackendRefTgIds",
			modelDefaultAction: &model.DefaultAction{
				Forward: &model.RuleAction{
					TargetGroups: []*model.RuleTargetGroup{
						{
							LatticeTgId:        model.InvalidBackendRefTgId,
							StackTargetGroupId: model.InvalidBackendRefTgId,
							Weight:             20,
						},
						{
							LatticeTgId:        model.InvalidBackendRefTgId,
							StackTargetGroupId: model.InvalidBackendRefTgId,
							Weight:             80,
						},
					},
				},
			},
			wantedLatticeDefaultAction: nil,
			wantErr:                    true,
		},
	}

	c := gomock.NewController(t)
	defer c.Finish()
	mockLattice := mocks.NewMockLattice(c)
	cloud := pkg_aws.NewDefaultCloud(mockLattice, TestCloudConfig)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stack := core.NewDefaultStack(core.StackID{Name: "foo", Namespace: "bar"})
			modelListener := &model.Listener{
				ResourceMeta: core.NewResourceMeta(stack, "AWS:VPCServiceNetwork::Listener", "modelListener-id"),
				Spec: model.ListenerSpec{
					StackServiceId: "stack-svc-id",
					Protocol:       vpclattice.ListenerProtocolTlsPassthrough,
					Port:           80,
					DefaultAction:  tt.modelDefaultAction,
				},
			}
			assert.NoError(t, stack.AddResource(modelListener))

			d := &defaultListenerManager{
				log:   gwlog.FallbackLogger,
				cloud: cloud,
			}
			gotDefaultAction, err := d.getLatticeListenerDefaultAction(context.TODO(), modelListener)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantedLatticeDefaultAction, gotDefaultAction)
			}
		})
	}
}
