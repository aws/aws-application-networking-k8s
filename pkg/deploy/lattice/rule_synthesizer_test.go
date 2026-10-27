package lattice

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/vpclattice/types"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

func Test_SynthesizeRule(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockRuleMgr := NewMockRuleManager(c)
	mockTgMgr := NewMockTargetGroupManager(c)

	stack := core.NewDefaultStack(core.StackID{Name: "foo", Namespace: "bar"})

	// each rule references a stack and a service which need to be present in the stack
	// in order to proceed, these just need their status+id
	svc := &model.Service{
		ResourceMeta: core.NewResourceMeta(stack, "AWS:VPCServiceNetwork::Service", "svc-id"),
		Status:       &model.ServiceStatus{Id: "svc-id"},
	}
	assert.NoError(t, stack.AddResource(svc))

	l := &model.Listener{
		ResourceMeta: core.NewResourceMeta(stack, "AWS:VPCServiceNetwork::Listener", "listener-id"),
		Spec:         model.ListenerSpec{StackServiceId: svc.ID()},
		Status:       &model.ListenerStatus{Id: "listener-id"},
	}
	assert.NoError(t, stack.AddResource(l))

	// then we resolve target groups, which sets the LatticeTgId field on each rule
	// these can already be populated, or can come from the stack as a svcExport or svc
	// we unit test tg resolution separately, so we'll take the easy way here and not
	// have any actions/tg references
	r := &model.Rule{
		ResourceMeta: core.NewResourceMeta(stack, "AWS:VPCServiceNetwork::Rule", "rule-id"),
		Spec: model.RuleSpec{
			StackListenerId: l.ID(),
			Priority:        1,
			CreateTime:      time.Time{},
			Action:          model.RuleAction{},
		},
		Status: nil,
	}
	assert.NoError(t, stack.AddResource(r))

	// then we call create on the rule manager, using the rule status
	// if there were no pre-existing rules, then we're done
	t.Run("no pre-existing rules", func(t *testing.T) {
		mockRuleMgr.EXPECT().Upsert(ctx, r, l, svc).Return(model.RuleStatus{
			Id:       "rule-id",
			Priority: 1, // <-- this matching means we don't update rules
		}, nil)

		mockRuleMgr.EXPECT().List(ctx, "svc-id", "listener-id").Return(
			[]types.RuleSummary{
				{
					Id:        aws.String("default-id"),
					IsDefault: aws.Bool(true),
				},
				{
					Id: aws.String("rule-id"),
				},
			}, nil)
		mockTgMgr.EXPECT().ResolveRuleTgIds(ctx, &r.Spec.Action, stack).Return(nil)
		rs := NewRuleSynthesizer(gwlog.FallbackLogger, mockRuleMgr, mockTgMgr, stack)
		rs.Synthesize(ctx)
	})

	// if there were pre-existing rules, we need to remove the previous ones that are no longer valid
	t.Run("pre-existing rule to remove", func(t *testing.T) {
		mockRuleMgr.EXPECT().Upsert(ctx, r, l, svc).Return(model.RuleStatus{
			Id:       "rule-id",
			Priority: 1,
		}, nil)

		mockRuleMgr.EXPECT().List(ctx, "svc-id", "listener-id").Return(
			[]types.RuleSummary{
				{
					Id:        aws.String("default-id"),
					IsDefault: aws.Bool(true),
				},
				{
					Id: aws.String("rule-id"),
				},
				{
					Id: aws.String("delete-rule-id"), // <-- should delete this rule
				},
			}, nil)

		mockRuleMgr.EXPECT().Delete(ctx, "delete-rule-id", "svc-id", "listener-id").Return(nil)
		mockTgMgr.EXPECT().ResolveRuleTgIds(ctx, &r.Spec.Action, stack).Return(nil)

		rs := NewRuleSynthesizer(gwlog.FallbackLogger, mockRuleMgr, mockTgMgr, stack)
		rs.Synthesize(ctx)
	})

	// if there are pre-existing rules, we need to update priorities afterward
	t.Run("pre-existing rule to update", func(t *testing.T) {
		mockRuleMgr.EXPECT().Upsert(ctx, r, l, svc).Return(model.RuleStatus{
			Id:       "rule-id",
			Priority: r.Spec.Priority + 1, // <-- this should trigger an update
		}, nil)

		mockRuleMgr.EXPECT().List(ctx, "svc-id", "listener-id").Return(
			[]types.RuleSummary{
				{
					Id:        aws.String("default-id"),
					IsDefault: aws.Bool(true),
				},
				{
					Id: aws.String("rule-id"),
				},
			}, nil)

		mockRuleMgr.EXPECT().UpdatePriorities(ctx, "svc-id", "listener-id", gomock.Any()).DoAndReturn(
			func(ctx context.Context, svcId string, listenerId string, rules []*model.Rule) error {
				assert.Equal(t, 1, len(rules))
				return nil
			})
		mockTgMgr.EXPECT().ResolveRuleTgIds(ctx, &r.Spec.Action, stack).Return(nil)

		rs := NewRuleSynthesizer(gwlog.FallbackLogger, mockRuleMgr, mockTgMgr, stack)
		rs.Synthesize(ctx)
	})
}

// quotaSynthFixture builds a stack with one service, one listener and the given rules.
func quotaSynthFixture(t *testing.T, ruleCount int) (core.Stack, *model.Service, *model.Listener, []*model.Rule) {
	stack := core.NewDefaultStack(core.StackID{Name: "foo", Namespace: "bar"})
	svc := &model.Service{
		ResourceMeta: core.NewResourceMeta(stack, "AWS:VPCServiceNetwork::Service", "svc-id"),
		Status:       &model.ServiceStatus{Id: "svc-id"},
	}
	assert.NoError(t, stack.AddResource(svc))
	l := &model.Listener{
		ResourceMeta: core.NewResourceMeta(stack, "AWS:VPCServiceNetwork::Listener", "listener-id"),
		Spec:         model.ListenerSpec{StackServiceId: svc.ID(), Port: 80, Protocol: "HTTP"},
		Status:       &model.ListenerStatus{Id: "listener-id"},
	}
	assert.NoError(t, stack.AddResource(l))

	var rules []*model.Rule
	for i := 0; i < ruleCount; i++ {
		r := &model.Rule{
			ResourceMeta: core.NewResourceMeta(stack, "AWS:VPCServiceNetwork::Rule", fmt.Sprintf("r%d", i)),
			Spec: model.RuleSpec{
				StackListenerId: l.ID(),
				Priority:        int64(i + 1),
				RuleIndex:       i,
			},
		}
		assert.NoError(t, stack.AddResource(r))
		rules = append(rules, r)
	}
	return stack, svc, l, rules
}

func Test_SynthesizeRule_QuotaExceeded(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockRuleMgr := NewMockRuleManager(c)
	mockTgMgr := NewMockTargetGroupManager(c)
	stack, svc, l, rules := quotaSynthFixture(t, 3)
	rules[1].Spec.RuleName = "checkout"

	quotaErr := &types.ServiceQuotaExceededException{Message: aws.String("quota")}
	mockTgMgr.EXPECT().ResolveRuleTgIds(ctx, gomock.Any(), stack).Return(nil).AnyTimes()
	mockRuleMgr.EXPECT().Upsert(ctx, rules[0], l, svc).Return(model.RuleStatus{Id: "ok-id", Priority: 1}, nil)
	// rules 1 and 2 hit the quota on the first attempt and again on the in-pass retry
	mockRuleMgr.EXPECT().Upsert(ctx, rules[1], l, svc).Return(model.RuleStatus{}, quotaErr).Times(2)
	mockRuleMgr.EXPECT().Upsert(ctx, rules[2], l, svc).Return(model.RuleStatus{}, quotaErr).Times(2)

	// stale rules are still cleaned up, which frees quota for the dropped rules
	mockRuleMgr.EXPECT().List(ctx, "svc-id", "listener-id").Return([]types.RuleSummary{
		{Id: aws.String("default-id"), IsDefault: aws.Bool(true)},
		{Id: aws.String("ok-id")},
		{Id: aws.String("stale-id")},
	}, nil)
	mockRuleMgr.EXPECT().Delete(ctx, "stale-id", "svc-id", "listener-id").Return(nil)

	err := NewRuleSynthesizer(gwlog.FallbackLogger, mockRuleMgr, mockTgMgr, stack).Synthesize(ctx)

	ruleErr, ok := services.AsRuleQuotaExceeded(err)
	if assert.True(t, ok) {
		assert.ElementsMatch(t, []services.DroppedRule{
			{RuleIndex: 1, RuleName: "checkout", Priority: 2, ListenerPort: 80, ListenerProtocol: "HTTP"},
			{RuleIndex: 2, Priority: 3, ListenerPort: 80, ListenerProtocol: "HTTP"},
		}, ruleErr.Dropped)
	}
	got, isQuota := services.AsServiceQuotaExceeded(err)
	assert.True(t, isQuota)
	assert.Same(t, quotaErr, got)
}

// Every rule on the listener is replaced while at the quota: nothing succeeds on the first attempt, but cleaning
// up the old rules frees quota, so the in-pass retry succeeds and no error is reported.
func Test_SynthesizeRule_QuotaFreedByCleanup(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockRuleMgr := NewMockRuleManager(c)
	mockTgMgr := NewMockTargetGroupManager(c)
	stack, svc, l, rules := quotaSynthFixture(t, 1)

	mockTgMgr.EXPECT().ResolveRuleTgIds(ctx, gomock.Any(), stack).Return(nil).AnyTimes()
	gomock.InOrder(
		mockRuleMgr.EXPECT().Upsert(ctx, rules[0], l, svc).Return(model.RuleStatus{}, &types.ServiceQuotaExceededException{}),
		mockRuleMgr.EXPECT().List(ctx, "svc-id", "listener-id").Return([]types.RuleSummary{
			{Id: aws.String("default-id"), IsDefault: aws.Bool(true)},
			{Id: aws.String("old-id")},
		}, nil),
		mockRuleMgr.EXPECT().Delete(ctx, "old-id", "svc-id", "listener-id").Return(nil),
		mockRuleMgr.EXPECT().Upsert(ctx, rules[0], l, svc).Return(model.RuleStatus{Id: "new-id", Priority: 1}, nil),
	)

	assert.NoError(t, NewRuleSynthesizer(gwlog.FallbackLogger, mockRuleMgr, mockTgMgr, stack).Synthesize(ctx))
}

func Test_SynthesizeRule_NonQuotaErrorAbortsAfterDrop(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockRuleMgr := NewMockRuleManager(c)
	mockTgMgr := NewMockTargetGroupManager(c)
	stack, svc, l, rules := quotaSynthFixture(t, 2)

	boom := errors.New("boom")
	mockTgMgr.EXPECT().ResolveRuleTgIds(ctx, gomock.Any(), stack).Return(nil).AnyTimes()
	mockRuleMgr.EXPECT().Upsert(ctx, rules[0], l, svc).Return(model.RuleStatus{}, &types.ServiceQuotaExceededException{})
	mockRuleMgr.EXPECT().Upsert(ctx, rules[1], l, svc).Return(model.RuleStatus{}, boom)

	err := NewRuleSynthesizer(gwlog.FallbackLogger, mockRuleMgr, mockTgMgr, stack).Synthesize(ctx)

	assert.ErrorIs(t, err, boom)
	_, isRuleQuota := services.AsRuleQuotaExceeded(err)
	assert.False(t, isRuleQuota)
}

// Nothing was deleted, so there is no freed quota and the dropped rule is not retried within the pass.
func Test_SynthesizeRule_NoRetryWithoutCleanup(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockRuleMgr := NewMockRuleManager(c)
	mockTgMgr := NewMockTargetGroupManager(c)
	stack, svc, l, rules := quotaSynthFixture(t, 2)

	mockTgMgr.EXPECT().ResolveRuleTgIds(ctx, gomock.Any(), stack).Return(nil).AnyTimes()
	mockRuleMgr.EXPECT().Upsert(ctx, rules[0], l, svc).Return(model.RuleStatus{Id: "ok-id", Priority: 1}, nil)
	mockRuleMgr.EXPECT().Upsert(ctx, rules[1], l, svc).Return(model.RuleStatus{}, &types.ServiceQuotaExceededException{}).Times(1)
	mockRuleMgr.EXPECT().List(ctx, "svc-id", "listener-id").Return([]types.RuleSummary{
		{Id: aws.String("default-id"), IsDefault: aws.Bool(true)},
		{Id: aws.String("ok-id")},
	}, nil)

	err := NewRuleSynthesizer(gwlog.FallbackLogger, mockRuleMgr, mockTgMgr, stack).Synthesize(ctx)

	ruleErr, ok := services.AsRuleQuotaExceeded(err)
	if assert.True(t, ok) {
		assert.Len(t, ruleErr.Dropped, 1)
	}
}
