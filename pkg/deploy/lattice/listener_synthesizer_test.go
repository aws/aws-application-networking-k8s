package lattice

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/vpclattice/types"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

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

	mockListenerMgr.EXPECT().Upsert(ctx, gomock.Any(), gomock.Any()).Return(
		model.ListenerStatus{Id: "new-listener-id"}, nil)

	mockListenerMgr.EXPECT().List(ctx, gomock.Any()).Return([]*types.ListenerSummary{}, nil)

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
			Protocol:       string(types.ListenerProtocolHttp),
			Port:           80,
			DefaultAction: &model.DefaultAction{
				FixedResponseStatusCode: aws.Int64(404),
			},
		},
	}
	assert.NoError(t, stack.AddResource(l))

	mockListenerMgr.EXPECT().Upsert(ctx, l, svc).Return(
		model.ListenerStatus{Id: "new-listener-id"}, nil)

	mockListenerMgr.EXPECT().List(ctx, gomock.Any()).Return([]*types.ListenerSummary{
		{
			Id:       aws.String("to-delete-id"),
			Protocol: types.ListenerProtocolHttps,
			Port:     aws.Int32(443), // <-- makes this listener unique
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
			Protocol:       string(types.ListenerProtocolTlsPassthrough),
			Port:           80,
			DefaultAction:  &model.DefaultAction{Forward: &model.RuleAction{TargetGroups: rule.Spec.Action.TargetGroups}},
		},
	}
	assert.NoError(t, stack.AddResource(l))
	mockListenerMgr.EXPECT().Upsert(ctx, l, svc).Return(
		model.ListenerStatus{Id: "new-listener-id"}, nil)

	mockListenerMgr.EXPECT().List(ctx, gomock.Any()).Return([]*types.ListenerSummary{
		{
			Id:       aws.String("to-delete-id"),
			Protocol: types.ListenerProtocolHttps,
			Port:     aws.Int32(443),
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

func newServiceOnlyStack(t *testing.T) core.Stack {
	t.Helper()
	stack := core.NewDefaultStack(core.StackID{Name: "foo", Namespace: "bar"})
	assert.NoError(t, stack.AddResource(&model.Service{
		ResourceMeta: core.NewResourceMeta(stack, "AWS:VPCServiceNetwork::Service", "stack-svc-id"),
		Status:       &model.ServiceStatus{Id: "svc-id"},
	}))
	return stack
}

func Test_SynthesizeListener_StaleListenerErrorsAreReturned(t *testing.T) {
	stale := []*types.ListenerSummary{{
		Id:       aws.String("tls-listener-id"),
		Port:     aws.Int32(444),
		Protocol: types.ListenerProtocolTlsPassthrough,
	}}
	tests := []struct {
		name      string
		listErr   error
		deleteErr error
	}{
		{name: "delete fails", deleteErr: errors.New("throttled")},
		{name: "list fails", listErr: errors.New("throttled")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := gomock.NewController(t)
			defer c.Finish()
			ctx := context.TODO()
			mockListenerMgr := NewMockListenerManager(c)
			if tt.listErr != nil {
				mockListenerMgr.EXPECT().List(ctx, "svc-id").Return(nil, tt.listErr)
			} else {
				mockListenerMgr.EXPECT().List(ctx, "svc-id").Return(stale, nil)
				mockListenerMgr.EXPECT().Delete(ctx, gomock.Any()).Return(tt.deleteErr)
			}

			ls := NewListenerSynthesizer(gwlog.FallbackLogger, mockListenerMgr, NewMockTargetGroupManager(c), newServiceOnlyStack(t))
			want := tt.listErr
			if want == nil {
				want = tt.deleteErr
			}
			err := ls.Synthesize(ctx)
			assert.ErrorIs(t, err, want)
			// The stack deployer keeps going past this error, since the stack's own
			// listeners are already up to date.
			assert.ErrorIs(t, err, ErrStaleListenerCleanup)
		})
	}
}

func Test_SynthesizeListener_UpsertErrorIsNotStaleCleanup(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockListenerMgr := NewMockListenerManager(c)
	stack := newServiceOnlyStack(t)
	assert.NoError(t, stack.AddResource(&model.Listener{
		ResourceMeta: core.NewResourceMeta(stack, "AWS:VPCServiceNetwork::Listener", "l-id"),
		Spec: model.ListenerSpec{
			StackServiceId: "stack-svc-id",
			DefaultAction:  &model.DefaultAction{FixedResponseStatusCode: aws.Int64(404)},
		},
	}))
	mockListenerMgr.EXPECT().Upsert(ctx, gomock.Any(), gomock.Any()).Return(model.ListenerStatus{}, errors.New("throttled"))

	ls := NewListenerSynthesizer(gwlog.FallbackLogger, mockListenerMgr, NewMockTargetGroupManager(c), stack)
	err := ls.Synthesize(ctx)
	assert.Error(t, err)
	// A listener in the stack failed to apply, so the deploy must stop here.
	assert.NotErrorIs(t, err, ErrStaleListenerCleanup)
}
