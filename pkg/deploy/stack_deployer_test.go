package deploy

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
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/aws/aws-application-networking-k8s/pkg/deploy/externaldns"
	"github.com/aws/aws-application-networking-k8s/pkg/deploy/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	lattice_runtime "github.com/aws/aws-application-networking-k8s/pkg/runtime"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

func TestTgGc(t *testing.T) {

	type test struct {
		name string
		gcFn TgGcCycleFn
	}

	tests := []test{
		{
			name: "empty cycle",
			gcFn: func(context.Context) (TgGcResult, error) { return TgGcResult{}, nil },
		},
		{
			name: "cycle with results",
			gcFn: func(context.Context) (TgGcResult, error) {
				return TgGcResult{
					att:      10,
					succ:     10,
					duration: time.Second,
				}, nil
			},
		},
		{
			name: "cycle panic",
			gcFn: func(context.Context) (TgGcResult, error) { panic("") },
		},
		{
			name: "cycle error",
			gcFn: func(context.Context) (TgGcResult, error) { return TgGcResult{}, errors.New("") },
		},
	}

	nCycles := 10 // run each test at least 10 cycles

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			n := 0
			f := func(ctx context.Context) (TgGcResult, error) {
				defer func() {
					n += 1
					if n >= nCycles {
						cancel()
					}
				}()
				return tt.gcFn(ctx)
			}
			ivl := time.Millisecond * 10
			tgGc := &TgGc{
				log:     gwlog.FallbackLogger,
				ctx:     ctx,
				ivl:     ivl,
				cycleFn: f,
			}
			tgGc.start()
			time.Sleep(ivl * (time.Duration(nCycles) + 2)) // sleep enough cycles to terminate
			assert.Equal(t, nCycles, n, fmt.Sprintf("should run only %d cycles", nCycles))
			assert.True(t, tgGc.isDone.Load(), "gc must terminate")
		})
	}
}

func TestDeploy_ListenerErrors(t *testing.T) {
	ctx := context.TODO()
	if tgGc == nil {
		tgGc = &TgGc{}
	}
	deleteErr := errors.New("throttled")
	upsertErr := errors.New("conflict")

	tests := []struct {
		name          string
		listenerMocks func(m *lattice.MockListenerManager)
		rulesRun      bool
		wantErrs      []error
		notErr        error
	}{
		{
			// The rule must still be applied: it is what rewrites a blocked backend to a fixed 500.
			name: "stale listener cleanup failure does not stop later steps",
			listenerMocks: func(m *lattice.MockListenerManager) {
				m.EXPECT().Upsert(ctx, gomock.Any(), gomock.Any()).Return(model.ListenerStatus{Id: "listener-id", ServiceId: "svc-id"}, nil)
				m.EXPECT().List(ctx, "svc-id").Return([]*types.ListenerSummary{
					{Id: aws.String("listener-id"), Port: aws.Int32(80), Protocol: types.ListenerProtocolHttp},
					{Id: aws.String("stale-id"), Port: aws.Int32(443), Protocol: types.ListenerProtocolHttps},
				}, nil)
				m.EXPECT().Delete(ctx, gomock.Any()).Return(deleteErr)
			},
			rulesRun: true,
			wantErrs: []error{lattice.ErrStaleListenerCleanup, deleteErr},
		},
		{
			// Rules would be synthesized for a listener that was never applied.
			name: "listener upsert failure stops the deploy",
			listenerMocks: func(m *lattice.MockListenerManager) {
				m.EXPECT().Upsert(ctx, gomock.Any(), gomock.Any()).Return(model.ListenerStatus{}, upsertErr)
			},
			notErr: lattice.ErrStaleListenerCleanup,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := gomock.NewController(t)

			stack := core.NewDefaultStack(core.StackID{Name: "route", Namespace: "ns"})
			svc := &model.Service{ResourceMeta: core.NewResourceMeta(stack, "AWS:VPCServiceNetwork::Service", "stack-svc-id")}
			assert.NoError(t, stack.AddResource(svc))
			listener := &model.Listener{
				ResourceMeta: core.NewResourceMeta(stack, "AWS:VPCServiceNetwork::Listener", "stack-listener-id"),
				Spec: model.ListenerSpec{
					StackServiceId: svc.ID(),
					Port:           80,
					Protocol:       "HTTP",
					DefaultAction:  &model.DefaultAction{FixedResponseStatusCode: aws.Int64(404)},
				},
			}
			assert.NoError(t, stack.AddResource(listener))
			assert.NoError(t, stack.AddResource(&model.Rule{
				ResourceMeta: core.NewResourceMeta(stack, "AWS:VPCServiceNetwork::Rule", "stack-rule-id"),
				Spec:         model.RuleSpec{StackListenerId: listener.ID(), Priority: 1},
			}))

			svcMgr := lattice.NewMockServiceManager(c)
			svcMgr.EXPECT().Upsert(ctx, gomock.Any()).Return(model.ServiceStatus{Id: "svc-id"}, nil)
			dnsMgr := externaldns.NewMockDnsEndpointManager(c)
			dnsMgr.EXPECT().Create(ctx, gomock.Any()).Return(nil).AnyTimes()

			listenerMgr := lattice.NewMockListenerManager(c)
			tt.listenerMocks(listenerMgr)

			tgMgr := lattice.NewMockTargetGroupManager(c)
			tgMgr.EXPECT().ResolveRuleTgIds(ctx, gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
			tgMgr.EXPECT().List(ctx).Return(nil, nil).AnyTimes()

			// Without an expectation, any call to the rule manager fails the test.
			ruleMgr := lattice.NewMockRuleManager(c)
			if tt.rulesRun {
				ruleMgr.EXPECT().Upsert(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return(model.RuleStatus{Id: "rule-id"}, nil)
				ruleMgr.EXPECT().List(ctx, gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
				ruleMgr.EXPECT().UpdatePriorities(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
			}

			d := &latticeServiceStackDeployer{
				log:                   gwlog.FallbackLogger,
				k8sClient:             testclient.NewClientBuilder().Build(),
				latticeServiceManager: svcMgr,
				targetGroupManager:    tgMgr,
				targetsManager:        lattice.NewMockTargetsManager(c),
				listenerManager:       listenerMgr,
				ruleManager:           ruleMgr,
				dnsEndpointManager:    dnsMgr,
			}

			err := d.Deploy(ctx, stack)
			assert.Error(t, err)
			for _, want := range tt.wantErrs {
				assert.ErrorIs(t, err, want)
			}
			if tt.notErr != nil {
				assert.NotErrorIs(t, err, tt.notErr)
			}
		})
	}
}

func TestRunAfterRules(t *testing.T) {
	ctx := context.TODO()
	pending := fmt.Errorf("%w: target status still in pending", lattice_runtime.NewRetryError())
	newQuotaErr := func() error {
		return fmt.Errorf("error during rule synthesis %w", &lattice.RuleQuotaExceededError{Err: errors.New("quota")})
	}

	tests := []struct {
		name              string
		quota             bool
		postErr           error
		deleteErr         error
		wantErr           error
		wantDeletes       int
		wantTargetPending bool
	}{
		{name: "no errors", wantDeletes: 1},
		{name: "step error without quota error is returned and stops", postErr: pending, wantErr: pending},
		{name: "quota error alone is returned after all steps", quota: true, wantDeletes: 1},
		{
			// a pending readiness gate must not replace the quota error, but is recorded on it
			name: "quota error wins over later step errors", quota: true,
			postErr: pending, deleteErr: errors.New("tg in use"), wantDeletes: 1, wantTargetPending: true,
		},
		{name: "a non-retry readiness step error is not pending targets", quota: true, postErr: errors.New("list failed"), wantDeletes: 1},
		{name: "a retry error from tg delete is not pending targets", quota: true, deleteErr: lattice_runtime.NewRetryError(), wantDeletes: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ruleQuotaErr error
			if tt.quota {
				ruleQuotaErr = newQuotaErr()
			}
			deletes := 0
			d := &latticeServiceStackDeployer{log: gwlog.FallbackLogger}
			err := d.runAfterRules(ctx, ruleQuotaErr,
				afterRulesStep{name: "post", run: func(context.Context) error { return tt.postErr }, readinessGates: true},
				afterRulesStep{name: "delete", run: func(context.Context) error { deletes++; return tt.deleteErr }},
			)
			switch {
			case tt.quota:
				assert.Same(t, ruleQuotaErr, err)
				ruleErr, ok := lattice.AsRuleQuotaExceeded(err)
				if assert.True(t, ok) {
					assert.Equal(t, tt.wantTargetPending, ruleErr.TargetsPending)
				}
			case tt.wantErr != nil:
				assert.ErrorIs(t, err, tt.wantErr)
			default:
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.wantDeletes, deletes)
		})
	}
}
