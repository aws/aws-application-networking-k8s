package lattice

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

func Test_SynthesizeListenerCreate(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockListenerMgr := NewMockListenerManager(c)
	mockTargetGroupManager := NewMockTargetGroupManager(c)

	stack := core.NewDefaultStack(core.StackID{Name: "foo", Namespace: "bar"})

	svc := &model.Service{
		ResourceMeta: core.NewResourceMeta(stack, "AWS:VPCServiceNetwork::Service", "stack-svc-id"),
		Status:       &model.ServiceStatus{Id: "svc-id"},
	}
	assert.NoError(t, stack.AddResource(svc))

	l := &model.Listener{
		ResourceMeta: core.NewResourceMeta(stack, "AWS:VPCServiceNetwork::Listener", "l-id"),
		Spec: model.ListenerSpec{
			StackServiceId: "stack-svc-id",
			DefaultAction: &model.DefaultAction{
				FixedResponseStatusCode: aws.Int64(404),
			},
		},
	}
	assert.NoError(t, stack.AddResource(l))

	mockListenerMgr.EXPECT().Upsert(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return(
		model.ListenerStatus{Id: "new-listener-id"}, nil)

	mockListenerMgr.EXPECT().List(ctx, gomock.Any()).Return([]*vpclattice.ListenerSummary{}, nil)

	ls := NewListenerSynthesizer(gwlog.FallbackLogger, mockListenerMgr, mockTargetGroupManager, stack)
	err := ls.Synthesize(ctx)
	assert.Nil(t, err)
}

func Test_SynthesizeListener_CreatNewHTTPListener_DeleteStaleHTTPSListener(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockListenerMgr := NewMockListenerManager(c)
	mockTargetGroupManager := NewMockTargetGroupManager(c)
	fixResponse404 := &vpclattice.RuleAction{
		FixedResponse: &vpclattice.FixedResponseAction{
			StatusCode: aws.Int64(404),
		},
	}
	stack := core.NewDefaultStack(core.StackID{Name: "foo", Namespace: "bar"})

	svc := &model.Service{
		ResourceMeta: core.NewResourceMeta(stack, "AWS:VPCServiceNetwork::Service", "stack-svc-id"),
		Status:       &model.ServiceStatus{Id: "svc-id"},
	}
	assert.NoError(t, stack.AddResource(svc))

	l := &model.Listener{
		ResourceMeta: core.NewResourceMeta(stack, "AWS:VPCServiceNetwork::Listener", "l-id"),
		Spec: model.ListenerSpec{
			StackServiceId: "stack-svc-id",
			Protocol:       vpclattice.ListenerProtocolHttp,
			Port:           80,
			DefaultAction: &model.DefaultAction{
				FixedResponseStatusCode: aws.Int64(404),
			},
		},
	}
	assert.NoError(t, stack.AddResource(l))

	mockListenerMgr.EXPECT().Upsert(ctx, l, svc, fixResponse404).Return(
		model.ListenerStatus{Id: "new-listener-id"}, nil)

	mockListenerMgr.EXPECT().List(ctx, gomock.Any()).Return([]*vpclattice.ListenerSummary{
		{
			Id:       aws.String("to-delete-id"),
			Protocol: aws.String(vpclattice.ListenerProtocolHttps),
			Port:     aws.Int64(443), // <-- makes this listener unique
		},
	}, nil)

	mockListenerMgr.EXPECT().Delete(ctx, gomock.Any()).DoAndReturn(
		func(ctx context.Context, ml *model.Listener) error {
			assert.Equal(t, "to-delete-id", ml.Status.Id)
			assert.Equal(t, "svc-id", ml.Status.ServiceId)
			return nil
		})

	ls := NewListenerSynthesizer(gwlog.FallbackLogger, mockListenerMgr, mockTargetGroupManager, stack)
	err := ls.Synthesize(ctx)
	assert.Nil(t, err)
}

func Test_SynthesizeListener_CreatNewTLSPassthroughListener_DeleteStaleHTTPSListener(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()

	ctx := context.TODO()
	mockListenerMgr := NewMockListenerManager(c)
	mockTargetGroupManager := NewMockTargetGroupManager(c)

	stack := core.NewDefaultStack(core.StackID{Name: "foo", Namespace: "bar"})

	svc := &model.Service{
		ResourceMeta: core.NewResourceMeta(stack, "AWS:VPCServiceNetwork::Service", "stack-svc-id"),
		Status:       &model.ServiceStatus{Id: "svc-id"},
	}

	assert.NoError(t, stack.AddResource(svc))
	rule := &model.Rule{
		ResourceMeta: core.NewResourceMeta(stack, "AWS:VPCServiceNetwork::Rule", "rule-id"),
		Spec: model.RuleSpec{
			Action: model.RuleAction{
				TargetGroups: []*model.RuleTargetGroup{
					{
						StackTargetGroupId: "stack-tg-id-1",
						Weight:             10,
					},
					{
						StackTargetGroupId: "stack-tg-id-2",
						Weight:             90,
					},
				},
			},
		},
	}
	assert.NoError(t, stack.AddResource(rule))
	mockTargetGroupManager.EXPECT().ResolveRuleTgIds(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, ruleAction *model.RuleAction, stack core.Stack) error {
			ruleAction.TargetGroups[0].LatticeTgId = "lattice-tg-id-1"
			ruleAction.TargetGroups[1].LatticeTgId = "lattice-tg-id-2"
			return nil
		})
	l := &model.Listener{
		ResourceMeta: core.NewResourceMeta(stack, "AWS:VPCServiceNetwork::Listener", "l-id"),
		Spec: model.ListenerSpec{
			StackServiceId: "stack-svc-id",
			Protocol:       vpclattice.ListenerProtocolTlsPassthrough,
			Port:           80,
			DefaultAction:  &model.DefaultAction{Forward: &model.RuleAction{TargetGroups: rule.Spec.Action.TargetGroups}},
		},
	}
	assert.NoError(t, stack.AddResource(l))
	expectedDefaultAction := &vpclattice.RuleAction{
		Forward: &vpclattice.ForwardAction{
			TargetGroups: []*vpclattice.WeightedTargetGroup{
				{
					TargetGroupIdentifier: aws.String("lattice-tg-id-1"),
					Weight:                aws.Int64(10),
				},
				{
					TargetGroupIdentifier: aws.String("lattice-tg-id-2"),
					Weight:                aws.Int64(90),
				},
			},
		},
	}
	mockListenerMgr.EXPECT().Upsert(ctx, l, svc, expectedDefaultAction).Return(
		model.ListenerStatus{Id: "new-listener-id"}, nil)

	mockListenerMgr.EXPECT().List(ctx, gomock.Any()).Return([]*vpclattice.ListenerSummary{
		{
			Id:       aws.String("to-delete-id"),
			Protocol: aws.String(vpclattice.ListenerProtocolHttps),
			Port:     aws.Int64(443),
		},
	}, nil)

	mockListenerMgr.EXPECT().Delete(ctx, gomock.Any()).DoAndReturn(
		func(ctx context.Context, ml *model.Listener) error {
			assert.Equal(t, "to-delete-id", ml.Status.Id)
			assert.Equal(t, "svc-id", ml.Status.ServiceId)
			return nil
		})

	ls := NewListenerSynthesizer(gwlog.FallbackLogger, mockListenerMgr, mockTargetGroupManager, stack)
	err := ls.Synthesize(ctx)
	assert.Nil(t, err)
}

func Test_listenerSynthesizer_getLatticeListenerDefaultAction_FixedResponse404(t *testing.T) {
	latticeFixResponseAction404 := &vpclattice.RuleAction{
		FixedResponse: &vpclattice.FixedResponseAction{
			StatusCode: aws.Int64(404),
		},
	}
	tests := []struct {
		name                       string
		stackListenerDefaultAction *model.DefaultAction
		listenerProtocol           string
		want                       *vpclattice.RuleAction
		wantErr                    error
	}{
		{
			name:                       "HTTP protocol Listener has the 404 fixed response modelListenerDefaultAction, return lattice fixed response 404 DefaultAction",
			stackListenerDefaultAction: &model.DefaultAction{FixedResponseStatusCode: aws.Int64(404)},
			listenerProtocol:           vpclattice.ListenerProtocolHttp,
			want:                       latticeFixResponseAction404,
			wantErr:                    nil,
		},
		{
			name:                       "HTTPS protocol Listener has the 404 fixed response modelListenerDefaultAction, return lattice fixed response 404 DefaultAction",
			stackListenerDefaultAction: &model.DefaultAction{FixedResponseStatusCode: aws.Int64(404)},
			listenerProtocol:           vpclattice.ListenerProtocolHttps,
			want:                       latticeFixResponseAction404,
			wantErr:                    nil,
		},
	}

	c := gomock.NewController(t)
	defer c.Finish()
	mockListenerMgr := NewMockListenerManager(c)
	mockTargetGroupManager := NewMockTargetGroupManager(c)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stack := core.NewDefaultStack(core.StackID{Name: "foo", Namespace: "bar"})
			modelListener := &model.Listener{
				ResourceMeta: core.NewResourceMeta(stack, "AWS:VPCServiceNetwork::Listener", "modelListener-id"),
				Spec: model.ListenerSpec{
					StackServiceId: "stack-svc-id",
					Protocol:       tt.listenerProtocol,
					Port:           80,
					DefaultAction:  tt.stackListenerDefaultAction,
				},
			}
			assert.NoError(t, stack.AddResource(modelListener))

			l := &listenerSynthesizer{
				log:         gwlog.FallbackLogger,
				listenerMgr: mockListenerMgr,
				tgManager:   mockTargetGroupManager,
				stack:       stack,
			}
			got, err := l.getLatticeListenerDefaultAction(context.TODO(), modelListener)

			assert.Equalf(t, tt.want, got, "getLatticeListenerDefaultAction() listenerProtocol: %v", tt.listenerProtocol)
			assert.Equal(t, tt.wantErr, err)
		})
	}
}

func Test_listenerSynthesizer_getLatticeListenerDefaultAction_TLS_PASSTHROUGH_Listener(t *testing.T) {

	tlspassthroughListenerProtocol := "TLS_PASSTHROUGH"

	c := gomock.NewController(t)
	defer c.Finish()

	t.Run("ResolveRuleTgIds success, backfill TLS_PASSTHROUGH Listener DefaultAction target group ids", func(t *testing.T) {
		mockListenerMgr := NewMockListenerManager(c)
		mockTargetGroupManager := NewMockTargetGroupManager(c)
		mockTargetGroupManager.EXPECT().ResolveRuleTgIds(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
			func(ctx context.Context, ruleAction *model.RuleAction, stack core.Stack) error {
				ruleAction.TargetGroups[0].LatticeTgId = "lattice-service-export-tg-id-1"
				ruleAction.TargetGroups[1].LatticeTgId = "lattice-tg-id-2"
				ruleAction.TargetGroups[2].LatticeTgId = model.InvalidBackendRefTgId
				return nil

			})
		stack := core.NewDefaultStack(core.StackID{Name: "foo", Namespace: "bar"})

		modelListener := &model.Listener{
			ResourceMeta: core.NewResourceMeta(stack, "AWS:VPCServiceNetwork::Listener", "modelListener-id"),
			Spec: model.ListenerSpec{
				StackServiceId: "stack-svc-id",
				Protocol:       vpclattice.ListenerProtocolTlsPassthrough,
				Port:           80,
				DefaultAction: &model.DefaultAction{
					Forward: &model.RuleAction{
						TargetGroups: []*model.RuleTargetGroup{
							{
								SvcImportTG: &model.SvcImportTargetGroup{
									K8SClusterName:      "cluster-name",
									K8SServiceName:      "svc-name",
									K8SServiceNamespace: "ns",
									VpcId:               "vpc-id",
								},
								Weight: 10,
							},
							{
								StackTargetGroupId: "lattice-tg-id-2",
								Weight:             30,
							},
							{
								StackTargetGroupId: model.InvalidBackendRefTgId,
								Weight:             60,
							},
						},
					},
				},
			},
		}
		assert.NoError(t, stack.AddResource(modelListener))

		l := &listenerSynthesizer{
			log:         gwlog.FallbackLogger,
			listenerMgr: mockListenerMgr,
			tgManager:   mockTargetGroupManager,
			stack:       stack,
		}
		gotDefaultAction, err := l.getLatticeListenerDefaultAction(context.TODO(), modelListener)
		wantedLatticeListenerDefaultAction := &vpclattice.RuleAction{
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
					{
						TargetGroupIdentifier: aws.String(model.InvalidBackendRefTgId),
						Weight:                aws.Int64(60),
					},
				},
			},
		}
		assert.Equalf(t, wantedLatticeListenerDefaultAction, gotDefaultAction, "getLatticeListenerDefaultAction() tlspassthroughListenerProtocol: %v", tlspassthroughListenerProtocol)
		assert.Nil(t, err)
	})

	t.Run("ResolveRuleTgIds failed, return err", func(t *testing.T) {
		mockListenerMgr := NewMockListenerManager(c)
		mockTargetGroupManager := NewMockTargetGroupManager(c)
		mockTargetGroupManager.EXPECT().ResolveRuleTgIds(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
			func(ctx context.Context, ruleAction *model.RuleAction, stack core.Stack) error {
				return fmt.Errorf("failed to resolve rule tg ids")
			})
		stack := core.NewDefaultStack(core.StackID{Name: "foo", Namespace: "bar"})
		stackRule := &model.Rule{
			ResourceMeta: core.NewResourceMeta(stack, "AWS:VPCServiceNetwork::Rule", "rule-id"),
			Spec: model.RuleSpec{
				Action: model.RuleAction{},
			},
		}
		assert.NoError(t, stack.AddResource(stackRule))
		modelListener := &model.Listener{
			ResourceMeta: core.NewResourceMeta(stack, "AWS:VPCServiceNetwork::Listener", "modelListener-id"),
			Spec: model.ListenerSpec{
				StackServiceId: "stack-svc-id",
				Protocol:       vpclattice.ListenerProtocolTlsPassthrough,
				Port:           80,
				DefaultAction: &model.DefaultAction{
					Forward: &model.RuleAction{
						TargetGroups: []*model.RuleTargetGroup{
							{
								StackTargetGroupId: "lattice-tg-id-1",
								Weight:             100,
							},
						},
					},
				},
			},
		}
		l := &listenerSynthesizer{
			log:         gwlog.FallbackLogger,
			listenerMgr: mockListenerMgr,
			tgManager:   mockTargetGroupManager,
			stack:       stack,
		}
		gotDefaultAction, err := l.getLatticeListenerDefaultAction(context.TODO(), modelListener)
		assert.Nil(t, gotDefaultAction)
		assert.NotNil(t, err)

	})
}
