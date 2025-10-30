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
			mockTagging := mocks.NewMockTagging(c)
			cloud := pkg_aws.NewDefaultCloudWithTagging(mockLattice, mockTagging, TestCloudConfig)
			mockLattice.EXPECT().ListListenersWithContext(ctx, gomock.Any()).Return(
				&vpclattice.ListListenersOutput{Items: []*vpclattice.ListenerSummary{
					{
						Arn:  aws.String("existing-arn"),
						Id:   aws.String("existing-listener-id"),
						Name: aws.String("existing-name"),
						Port: aws.Int64(8181),
					},
				}}, nil)

			mockTagging.EXPECT().UpdateTags(ctx, "existing-arn", gomock.Any(), nil).Return(nil)

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
	mockTagging := mocks.NewMockTagging(c)
	cloud := pkg_aws.NewDefaultCloudWithTagging(mockLattice, mockTagging, TestCloudConfig)
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

			mockTagging.EXPECT().UpdateTags(ctx, "existing-arn", gomock.Any(), nil).Return(nil)

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

func Test_ListenerManager_WithAdditionalTags_Create(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	mockTagging := mocks.NewMockTagging(c)
	cloud := pkg_aws.NewDefaultCloudWithTagging(mockLattice, mockTagging, TestCloudConfig)

	ml := &model.Listener{
		Spec: model.ListenerSpec{
			Protocol: vpclattice.ListenerProtocolHttp,
			Port:     8080,
			DefaultAction: &model.DefaultAction{
				FixedResponseStatusCode: aws.Int64(404),
			},
			AdditionalTags: mocks.Tags{
				"Environment": &[]string{"Test"}[0],
				"Project":     &[]string{"ListenerManager"}[0],
			},
		},
	}

	ms := &model.Service{
		Status: &model.ServiceStatus{Id: "svc-id"},
	}

	mockLattice.EXPECT().ListListenersWithContext(ctx, gomock.Any()).Return(
		&vpclattice.ListListenersOutput{Items: []*vpclattice.ListenerSummary{}}, nil)

	expectedTags := cloud.MergeTags(cloud.DefaultTags(), ml.Spec.AdditionalTags)

	mockLattice.EXPECT().CreateListenerWithContext(ctx, gomock.Any()).DoAndReturn(
		func(ctx context.Context, input *vpclattice.CreateListenerInput, opts ...interface{}) (*vpclattice.CreateListenerOutput, error) {
			assert.Equal(t, expectedTags, input.Tags, "Listener tags should include additional tags")
			assert.Equal(t, int64(8080), *input.Port)
			assert.Equal(t, "HTTP", *input.Protocol)

			return &vpclattice.CreateListenerOutput{
				Arn:  aws.String("listener-arn"),
				Id:   aws.String("listener-id"),
				Name: aws.String("listener-name"),
			}, nil
		})

	lm := NewListenerManager(gwlog.FallbackLogger, cloud)
	status, err := lm.Upsert(ctx, ml, ms)
	assert.Nil(t, err)
	assert.Equal(t, "listener-arn", status.ListenerArn)
	assert.Equal(t, "listener-id", status.Id)
}

func Test_ListenerManager_WithAdditionalTags_UpdateHTTP(t *testing.T) {
	// Test case: update existing HTTP listener with additional tags (no action update needed)
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	mockTagging := mocks.NewMockTagging(c)
	cloud := pkg_aws.NewDefaultCloudWithTagging(mockLattice, mockTagging, TestCloudConfig)

	ml := &model.Listener{
		Spec: model.ListenerSpec{
			Protocol: vpclattice.ListenerProtocolHttp,
			Port:     8080,
			DefaultAction: &model.DefaultAction{
				FixedResponseStatusCode: aws.Int64(404),
			},
			AdditionalTags: mocks.Tags{
				"Environment": &[]string{"Prod"}[0],
				"Project":     &[]string{"ListenerUpdate"}[0],
			},
		},
	}

	ms := &model.Service{
		Status: &model.ServiceStatus{Id: "svc-id"},
	}

	mockLattice.EXPECT().ListListenersWithContext(ctx, gomock.Any()).Return(
		&vpclattice.ListListenersOutput{Items: []*vpclattice.ListenerSummary{
			{
				Arn:  aws.String("existing-arn"),
				Id:   aws.String("existing-id"),
				Name: aws.String("existing-name"),
				Port: aws.Int64(8080),
			},
		}}, nil)

	mockTagging.EXPECT().UpdateTags(ctx, "existing-arn", ml.Spec.AdditionalTags, nil).Return(nil)

	// No UpdateListener call expected for HTTP listeners (only tags are updated)
	mockLattice.EXPECT().UpdateListenerWithContext(ctx, gomock.Any()).Times(0)
	mockLattice.EXPECT().GetListenerWithContext(ctx, gomock.Any()).Times(0)

	lm := NewListenerManager(gwlog.FallbackLogger, cloud)
	status, err := lm.Upsert(ctx, ml, ms)
	assert.Nil(t, err)
	assert.Equal(t, "existing-arn", status.ListenerArn)
	assert.Equal(t, "existing-id", status.Id)
}

func Test_ListenerManager_WithAdditionalTags_UpdateTLSPassthrough(t *testing.T) {
	// Test case: update existing TLS_PASSTHROUGH listener with additional tags and action update
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	mockTagging := mocks.NewMockTagging(c)
	cloud := pkg_aws.NewDefaultCloudWithTagging(mockLattice, mockTagging, TestCloudConfig)

	ml := &model.Listener{
		Spec: model.ListenerSpec{
			Protocol: vpclattice.ListenerProtocolTlsPassthrough,
			Port:     443,
			DefaultAction: &model.DefaultAction{
				Forward: &model.RuleAction{
					TargetGroups: []*model.RuleTargetGroup{
						{
							LatticeTgId: "tg-id-1",
							Weight:      100,
						},
					},
				},
			},
			AdditionalTags: mocks.Tags{
				"Environment": &[]string{"Staging"}[0],
				"Project":     &[]string{"TLSListener"}[0],
			},
		},
	}

	ms := &model.Service{
		Status: &model.ServiceStatus{Id: "svc-id"},
	}

	// Existing TLS_PASSTHROUGH listener found
	mockLattice.EXPECT().ListListenersWithContext(ctx, gomock.Any()).Return(
		&vpclattice.ListListenersOutput{Items: []*vpclattice.ListenerSummary{
			{
				Arn:      aws.String("existing-tls-arn"),
				Id:       aws.String("existing-tls-id"),
				Name:     aws.String("existing-tls-name"),
				Port:     aws.Int64(443),
				Protocol: aws.String(vpclattice.ListenerProtocolTlsPassthrough),
			},
		}}, nil)

	mockTagging.EXPECT().UpdateTags(ctx, "existing-tls-arn", ml.Spec.AdditionalTags, nil).Return(nil)

	mockLattice.EXPECT().GetListenerWithContext(ctx, gomock.Any()).Return(
		&vpclattice.GetListenerOutput{
			DefaultAction: &vpclattice.RuleAction{
				Forward: &vpclattice.ForwardAction{
					TargetGroups: []*vpclattice.WeightedTargetGroup{
						{
							TargetGroupIdentifier: aws.String("old-tg-id"),
							Weight:                aws.Int64(100),
						},
					},
				},
			},
		}, nil)

	// Mock UpdateListener call (action is different, so update needed)
	mockLattice.EXPECT().UpdateListenerWithContext(ctx, gomock.Any()).Return(
		&vpclattice.UpdateListenerOutput{
			Id: aws.String("existing-tls-id"),
		}, nil)

	lm := NewListenerManager(gwlog.FallbackLogger, cloud)
	status, err := lm.Upsert(ctx, ml, ms)
	assert.Nil(t, err)
	assert.Equal(t, "existing-tls-arn", status.ListenerArn)
	assert.Equal(t, "existing-tls-id", status.Id)
}

func Test_ListenerManager_WithTakeoverAnnotation_UpdateTags(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	mockTagging := mocks.NewMockTagging(c)
	cloud := pkg_aws.NewDefaultCloudWithTagging(mockLattice, mockTagging, TestCloudConfig)

	ml := &model.Listener{
		Spec: model.ListenerSpec{
			Protocol: vpclattice.ListenerProtocolHttp,
			Port:     8080,
			DefaultAction: &model.DefaultAction{
				FixedResponseStatusCode: aws.Int64(404),
			},
			AdditionalTags: mocks.Tags{
				"Environment": &[]string{"Takeover"}[0],
			},
		},
	}

	ms := &model.Service{
		Spec: model.ServiceSpec{
			AllowTakeoverFrom: "other-account/other-cluster/other-vpc",
		},
		Status: &model.ServiceStatus{Id: "svc-id"},
	}

	mockLattice.EXPECT().ListListenersWithContext(ctx, gomock.Any()).Return(
		&vpclattice.ListListenersOutput{Items: []*vpclattice.ListenerSummary{
			{
				Arn:  aws.String("existing-arn"),
				Id:   aws.String("existing-id"),
				Name: aws.String("existing-name"),
				Port: aws.Int64(8080),
			},
		}}, nil)

	expectedAwsManagedTags := mocks.Tags{
		pkg_aws.TagManagedBy: cloud.DefaultTags()[pkg_aws.TagManagedBy],
	}
	mockTagging.EXPECT().UpdateTags(ctx, "existing-arn", ml.Spec.AdditionalTags, expectedAwsManagedTags).Return(nil)

	lm := NewListenerManager(gwlog.FallbackLogger, cloud)
	status, err := lm.Upsert(ctx, ml, ms)
	assert.Nil(t, err)
	assert.Equal(t, "existing-arn", status.ListenerArn)
	assert.Equal(t, "existing-id", status.Id)
}
