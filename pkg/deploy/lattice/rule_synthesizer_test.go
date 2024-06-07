package lattice

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

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
			[]*vpclattice.RuleSummary{
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
			[]*vpclattice.RuleSummary{
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
			[]*vpclattice.RuleSummary{
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
