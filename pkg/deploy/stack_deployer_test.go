package deploy

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
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
			gcFn: func(context.Context, sync.Locker) (TgGcResult, error) { return TgGcResult{}, nil },
		},
		{
			name: "cycle with results",
			gcFn: func(context.Context, sync.Locker) (TgGcResult, error) {
				return TgGcResult{
					att:      10,
					succ:     10,
					duration: time.Second,
				}, nil
			},
		},
		{
			name: "cycle panic",
			gcFn: func(context.Context, sync.Locker) (TgGcResult, error) { panic("") },
		},
		{
			name: "cycle error",
			gcFn: func(context.Context, sync.Locker) (TgGcResult, error) { return TgGcResult{}, errors.New("") },
		},
	}

	nCycles := 10 // run each test at least 10 cycles

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			var n atomic.Int32 // written by the GC goroutine, read by the test
			f := func(ctx context.Context, l sync.Locker) (TgGcResult, error) {
				defer func() {
					if int(n.Add(1)) >= nCycles {
						cancel()
					}
				}()
				return tt.gcFn(ctx, l)
			}
			ivl := time.Millisecond * 10
			tgGc := &TgGc{
				log:     gwlog.FallbackLogger,
				ctx:     ctx,
				ivl:     ivl,
				cycleFn: f,
			}
			tgGc.start()
			assert.Eventually(t, tgGc.isDone.Load, 5*time.Second, ivl, "gc must terminate")
			assert.Equal(t, nCycles, int(n.Load()), fmt.Sprintf("should run only %d cycles", nCycles))
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

// Route and ServiceExport deploys must not wait for the GC scan, which lists target
// groups and reads their tags and can take minutes on a large account.
func TestTgGcScanDoesNotBlockDeploys(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()

	scanStarted := make(chan struct{})
	finishScan := make(chan struct{})
	mockTGManager := lattice.NewMockTargetGroupManager(c)
	mockTGManager.EXPECT().List(gomock.Any()).Do(func(context.Context) {
		close(scanStarted)
		<-finishScan
	}).Return(nil, nil)

	prevGc := tgGc
	defer func() { tgGc = prevGc }()
	tgGc = &TgGc{
		log: gwlog.FallbackLogger,
		ctx: context.Background(),
		cycleFn: NewTgGcFn(lattice.NewTargetGroupSynthesizer(
			gwlog.FallbackLogger, nil, nil, mockTGManager, nil, nil, nil)),
	}
	cycleDone := make(chan struct{})
	go func() {
		defer close(cycleDone)
		tgGc.cycle()
	}()
	select {
	case <-scanStarted:
	case <-cycleDone:
		t.Fatal("GC cycle ended without scanning")
	}

	deployers := map[string]StackDeployer{
		"route":          &latticeServiceStackDeployer{log: gwlog.FallbackLogger},
		"service export": NewTargetGroupStackDeploy(gwlog.FallbackLogger, nil, nil),
	}
	deployed := map[string]chan error{}
	for name, d := range deployers {
		done := make(chan error, 1)
		deployed[name] = done
		go func() { done <- d.Deploy(context.Background(), core.NewDefaultStack(core.StackID{})) }()
	}
	deadline := time.Now().Add(5 * time.Second)
	var blocked []chan error
	for name, done := range deployed {
		select {
		case err := <-done:
			assert.NoError(t, err, name)
		case <-time.After(time.Until(deadline)):
			t.Errorf("%s deploy is blocked by the GC scan", name)
			blocked = append(blocked, done)
		}
	}

	close(finishScan)
	<-cycleDone
	for _, done := range blocked {
		<-done // a blocked deploy finishes once the scan does
	}
}

// The cycle must hand cycleFn the lock deploys take, so that deletes exclude deploys,
// and must not hold it itself.
func TestTgGcCycleLendsDeployLock(t *testing.T) {
	gc := &TgGc{log: gwlog.FallbackLogger, ctx: context.Background()}
	called := false
	gc.cycleFn = func(_ context.Context, l sync.Locker) (TgGcResult, error) {
		called = true
		assert.Same(t, &gc.lock, l, "cycleFn should get the deploy lock")
		assert.True(t, gc.lock.TryRLock(), "deploys must be able to run while the cycle scans")
		gc.lock.RUnlock()
		return TgGcResult{}, nil
	}
	gc.cycle()
	assert.True(t, called)
	assert.True(t, gc.lock.TryLock(), "lock must be free after the cycle")
}
