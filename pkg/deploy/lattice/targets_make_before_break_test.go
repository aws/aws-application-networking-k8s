package lattice

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/vpclattice"
	"github.com/aws/aws-sdk-go-v2/service/vpclattice/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	mocks_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	mocks "github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	lattice_runtime "github.com/aws/aws-application-networking-k8s/pkg/runtime"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

// The tests below use strict gomock expectations: a RegisterTargets or
// DeregisterTargets call that a step does not expect fails the test, so a step
// without an expected DeregisterTargets proves the old targets stayed registered.

const (
	mbbNamespace = "app"
	mbbPort      = 8080
)

var mbbIPs = map[string]string{
	"old-0": "10.0.0.1", "old-1": "10.0.0.2", "old-2": "10.0.0.3",
	"new-0": "10.0.1.1", "new-1": "10.0.1.2", "new-2": "10.0.1.3",
}

var (
	mbbOld = []string{"old-0", "old-1", "old-2"}
	mbbNew = []string{"new-0", "new-1", "new-2"}
)

func mbbPod(name string, ready bool) *corev1.Pod {
	status := corev1.ConditionFalse
	if ready {
		status = corev1.ConditionTrue
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: mbbNamespace},
		Status: corev1.PodStatus{
			PodIP:      mbbIPs[name],
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: status}},
		},
	}
}

func mbbTargets(pods ...string) *model.Targets {
	list := []model.Target{}
	for _, p := range pods {
		list = append(list, model.Target{
			TargetIP:  mbbIPs[p],
			Port:      mbbPort,
			Ready:     true,
			TargetRef: apitypes.NamespacedName{Namespace: mbbNamespace, Name: p},
		})
	}
	return &model.Targets{Spec: model.TargetsSpec{StackTargetGroupId: "tg-stack-id", TargetList: list}}
}

func mbbSummaries(status types.TargetStatus, pods ...string) []types.TargetSummary {
	var out []types.TargetSummary
	for _, p := range pods {
		out = append(out, types.TargetSummary{Id: aws.String(mbbIPs[p]), Port: aws.Int32(mbbPort), Status: status})
	}
	return out
}

func mbbLatticeTargets(pods ...string) []types.Target {
	var out []types.Target
	for _, p := range pods {
		out = append(out, types.Target{Id: aws.String(mbbIPs[p]), Port: aws.Int32(mbbPort)})
	}
	return out
}

func concat(lists ...[]types.TargetSummary) []types.TargetSummary {
	var out []types.TargetSummary
	for _, l := range lists {
		out = append(out, l...)
	}
	return out
}

type mbbEnv struct {
	lattice *mocks.MockLattice
	k8s     client.Client
	mgr     *defaultTargetsManager
	tg      *model.TargetGroup
	now     time.Time
}

// newMBBEnv sets ENABLE_MAKE_BEFORE_BREAK_TARGETS to enabled and creates Ready
// pods old-0..2 (currently registered) and new-0..2 (the replacements).
func newMBBEnv(t *testing.T, enabled bool, opts ...func(*testclient.ClientBuilder)) *mbbEnv {
	prevEnabled, prevTimeout := config.MakeBeforeBreakTargets, config.MakeBeforeBreakTargetsTimeout
	config.MakeBeforeBreakTargets = enabled
	config.MakeBeforeBreakTargetsTimeout = 5 * time.Minute
	t.Cleanup(func() {
		config.MakeBeforeBreakTargets, config.MakeBeforeBreakTargetsTimeout = prevEnabled, prevTimeout
	})

	c := gomock.NewController(t)
	mockCloud := mocks_aws.NewMockCloud(c)
	mockLattice := mocks.NewMockLattice(c)
	mockCloud.EXPECT().Lattice().Return(mockLattice).AnyTimes()

	builder := testclient.NewClientBuilder().WithStatusSubresource(&corev1.Pod{})
	for _, p := range append(append([]string{}, mbbOld...), mbbNew...) {
		builder = builder.WithObjects(mbbPod(p, true))
	}
	for _, o := range opts {
		o(builder)
	}

	env := &mbbEnv{lattice: mockLattice, k8s: builder.Build(), now: time.Unix(1_000_000, 0)}
	env.mgr = NewTargetsManagerWithClient(gwlog.FallbackLogger, mockCloud, env.k8s)
	env.mgr.holds = newTargetHolds(func() time.Time { return env.now })

	stack := core.NewDefaultStack(core.StackID{Name: "svc", Namespace: mbbNamespace})
	env.tg = &model.TargetGroup{
		ResourceMeta: core.NewResourceMeta(stack, "AWS:VPCServiceNetwork::TargetGroup", "tg-stack-id"),
		Status:       &model.TargetGroupStatus{Name: "tg-name", Arn: "tg-arn", Id: "tg-id"},
	}
	return env
}

func (e *mbbEnv) expectList(summaries []types.TargetSummary) *gomock.Call {
	return e.lattice.EXPECT().ListTargetsAsList(gomock.Any(), &vpclattice.ListTargetsInput{TargetGroupIdentifier: aws.String("tg-id")}).
		Return(summaries, nil)
}

func (e *mbbEnv) expectRegister(pods ...string) *gomock.Call {
	targets := mbbLatticeTargets(pods...)
	return e.lattice.EXPECT().RegisterTargets(gomock.Any(), &vpclattice.RegisterTargetsInput{
		TargetGroupIdentifier: aws.String("tg-id"),
		Targets:               targets,
	}).Return(&vpclattice.RegisterTargetsOutput{Successful: targets}, nil)
}

func (e *mbbEnv) expectDeregister(pods ...string) *gomock.Call {
	targets := mbbLatticeTargets(pods...)
	return e.lattice.EXPECT().DeregisterTargets(gomock.Any(), &vpclattice.DeregisterTargetsInput{
		TargetGroupIdentifier: aws.String("tg-id"),
		Targets:               targets,
	}).Return(&vpclattice.DeregisterTargetsOutput{Successful: targets}, nil)
}

func (e *mbbEnv) update(t *testing.T, targets *model.Targets) {
	t.Helper()
	require.NoError(t, e.mgr.Update(context.TODO(), targets, e.tg))
}

func TestMakeBeforeBreak_FlagOffKeepsDeregisterThenRegister(t *testing.T) {
	env := newMBBEnv(t, false)
	env.expectList(mbbSummaries(types.TargetStatusHealthy, mbbOld...))
	gomock.InOrder(
		env.expectDeregister(mbbOld...),
		env.expectRegister(mbbNew...),
	)
	env.update(t, mbbTargets(mbbNew...))
	assert.Equal(t, 0, env.mgr.holds.len())
}

func TestMakeBeforeBreak_NoClientBehavesAsFlagOff(t *testing.T) {
	env := newMBBEnv(t, true)
	env.mgr.client = nil
	env.expectList(mbbSummaries(types.TargetStatusHealthy, mbbOld...))
	gomock.InOrder(
		env.expectDeregister(mbbOld...),
		env.expectRegister(mbbNew...),
	)
	env.update(t, mbbTargets(mbbNew...))
	assert.Equal(t, 0, env.mgr.holds.len())
}

func TestMakeBeforeBreak_HoldsOldTargetsUntilReplacementsHealthy(t *testing.T) {
	env := newMBBEnv(t, true)
	newSet := mbbTargets(mbbNew...)

	// Selector flips: the new targets are registered, the old ones are held.
	env.expectList(mbbSummaries(types.TargetStatusHealthy, mbbOld...))
	env.expectRegister(mbbNew...)
	env.update(t, newSet)
	assert.Equal(t, 3, env.mgr.holds.len())

	// Two of three replacements healthy: still held, nothing to register.
	env.now = env.now.Add(10 * time.Second)
	env.expectList(concat(
		mbbSummaries(types.TargetStatusHealthy, mbbOld...),
		mbbSummaries(types.TargetStatusHealthy, "new-0", "new-1"),
		mbbSummaries(types.TargetStatusInitial, "new-2"),
	))
	env.update(t, newSet)
	assert.Equal(t, 3, env.mgr.holds.len())

	// All replacements healthy: the old targets are deregistered and the timers cleared.
	env.now = env.now.Add(10 * time.Second)
	env.expectList(concat(
		mbbSummaries(types.TargetStatusHealthy, mbbOld...),
		mbbSummaries(types.TargetStatusHealthy, mbbNew...),
	))
	env.expectDeregister(mbbOld...)
	env.update(t, newSet)
	assert.Equal(t, 0, env.mgr.holds.len())
}

func TestMakeBeforeBreak_HoldLogNamesNotReadyReplacements(t *testing.T) {
	env := newMBBEnv(t, true)
	core, logs := observer.New(zap.InfoLevel)
	env.mgr.log = &gwlog.TracedLogger{InnerLogger: zap.New(core).Sugar()}

	// new-0 is healthy, new-1 is still INITIAL and new-2 is not registered yet.
	env.expectList(concat(
		mbbSummaries(types.TargetStatusHealthy, mbbOld...),
		mbbSummaries(types.TargetStatusHealthy, "new-0"),
		mbbSummaries(types.TargetStatusInitial, "new-1"),
	))
	env.expectRegister("new-2")
	env.update(t, mbbTargets(mbbNew...))

	held := logs.FilterMessageSnippet("Holding 3 departing targets").All()
	require.Len(t, held, 1)
	assert.Equal(t,
		"Holding 3 departing targets in target group tg-id until replacements are healthy, not ready: 10.0.1.2:8080 (INITIAL), 10.0.1.3:8080 (NOT_REGISTERED_YET)",
		held[0].Message)
}

func TestMakeBeforeBreak_HealthChecksOffOrUnusedCountAsReady(t *testing.T) {
	for _, status := range []types.TargetStatus{types.TargetStatusUnavailable, types.TargetStatusUnused} {
		t.Run(string(status), func(t *testing.T) {
			env := newMBBEnv(t, true)
			newSet := mbbTargets(mbbNew...)

			// Just registered, so not in the pre-register read: held.
			env.expectList(mbbSummaries(types.TargetStatusHealthy, mbbOld...))
			env.expectRegister(mbbNew...)
			env.update(t, newSet)

			env.expectList(concat(
				mbbSummaries(types.TargetStatusHealthy, mbbOld...),
				mbbSummaries(status, mbbNew...),
			))
			env.expectDeregister(mbbOld...)
			env.update(t, newSet)
			assert.Equal(t, 0, env.mgr.holds.len())
		})
	}
}

func TestMakeBeforeBreak_DrainingReplacementIsNotReady(t *testing.T) {
	env := newMBBEnv(t, true)
	// new-0 is DRAINING from an earlier removal: it is registered again and
	// is not ready yet, so old-0 is held.
	env.expectList(concat(
		mbbSummaries(types.TargetStatusHealthy, "old-0"),
		mbbSummaries(types.TargetStatusDraining, "new-0"),
	))
	env.expectRegister("new-0")
	env.update(t, mbbTargets("new-0"))
	assert.Equal(t, 1, env.mgr.holds.len())
}

func TestMakeBeforeBreak_RollbackRemovesNewTargetsAtOnce(t *testing.T) {
	env := newMBBEnv(t, true)
	env.expectList(mbbSummaries(types.TargetStatusHealthy, mbbOld...))
	env.expectRegister(mbbNew...)
	env.update(t, mbbTargets(mbbNew...))
	require.Equal(t, 3, env.mgr.holds.len())

	// The selector flips back while the old targets are held and still HEALTHY:
	// the old set is ready, so the new targets are deregistered immediately.
	env.expectList(concat(
		mbbSummaries(types.TargetStatusHealthy, mbbOld...),
		mbbSummaries(types.TargetStatusInitial, mbbNew...),
	))
	env.expectDeregister(mbbNew...)
	env.update(t, mbbTargets(mbbOld...))
	assert.Equal(t, 0, env.mgr.holds.len())
}

func TestMakeBeforeBreak_TerminatingPodIsNotHeld(t *testing.T) {
	env := newMBBEnv(t, true)
	ctx := context.TODO()
	// Rolling update step: new-0 is added (not yet healthy) and old-2 is being
	// deleted. The replacements are not ready, but old-2 is terminating, so it
	// is deregistered at once.
	pod := &corev1.Pod{}
	require.NoError(t, env.k8s.Get(ctx, apitypes.NamespacedName{Namespace: mbbNamespace, Name: "old-2"}, pod))
	pod.Finalizers = []string{"test/keep"}
	require.NoError(t, env.k8s.Update(ctx, pod))
	require.NoError(t, env.k8s.Delete(ctx, pod))

	env.expectList(mbbSummaries(types.TargetStatusHealthy, mbbOld...))
	env.expectRegister("new-0")
	env.expectDeregister("old-2")
	env.update(t, mbbTargets("old-0", "old-1", "new-0"))
	assert.Equal(t, 0, env.mgr.holds.len())
}

func TestMakeBeforeBreak_PlainScaleDown(t *testing.T) {
	env := newMBBEnv(t, true)
	// The remaining targets are all HEALTHY: nothing to wait for.
	env.expectList(mbbSummaries(types.TargetStatusHealthy, mbbOld...))
	env.expectDeregister("old-2")
	env.update(t, mbbTargets("old-0", "old-1"))
	assert.Equal(t, 0, env.mgr.holds.len())
}

func TestMakeBeforeBreak_PodGoneNotReadyOrSharedIPIsNotHeld(t *testing.T) {
	env := newMBBEnv(t, true)
	ctx := context.TODO()
	// old-0 pod deleted, old-1 pod not Ready, old-2 IP claimed by two pods.
	require.NoError(t, env.k8s.Delete(ctx, mbbPod("old-0", true)))
	pod := &corev1.Pod{}
	require.NoError(t, env.k8s.Get(ctx, apitypes.NamespacedName{Namespace: mbbNamespace, Name: "old-1"}, pod))
	pod.Status.Conditions[0].Status = corev1.ConditionFalse
	require.NoError(t, env.k8s.Status().Update(ctx, pod))
	dup := mbbPod("old-2", true)
	dup.Name = "dup"
	require.NoError(t, env.k8s.Create(ctx, dup))

	env.expectList(mbbSummaries(types.TargetStatusHealthy, mbbOld...))
	env.expectRegister(mbbNew...)
	env.expectDeregister(mbbOld...)
	env.update(t, mbbTargets(mbbNew...))
	assert.Equal(t, 0, env.mgr.holds.len())
}

func TestMakeBeforeBreak_OldPodsDeletedDuringHold(t *testing.T) {
	env := newMBBEnv(t, true)
	newSet := mbbTargets(mbbNew...)
	env.expectList(mbbSummaries(types.TargetStatusHealthy, mbbOld...))
	env.expectRegister(mbbNew...)
	env.update(t, newSet)

	// The old pods are removed before the replacements are healthy.
	for _, p := range mbbOld {
		require.NoError(t, env.k8s.Delete(context.TODO(), mbbPod(p, true)))
	}
	env.now = env.now.Add(30 * time.Second)
	env.expectList(concat(
		mbbSummaries(types.TargetStatusHealthy, mbbOld...),
		mbbSummaries(types.TargetStatusInitial, mbbNew...),
	))
	env.expectDeregister(mbbOld...)
	env.update(t, newSet)
	assert.Equal(t, 0, env.mgr.holds.len())
}

func TestMakeBeforeBreak_TimeoutDeregistersAnyway(t *testing.T) {
	env := newMBBEnv(t, true)
	newSet := mbbTargets(mbbNew...)
	stuck := concat(
		mbbSummaries(types.TargetStatusHealthy, mbbOld...),
		mbbSummaries(types.TargetStatusUnhealthy, mbbNew...),
	)

	env.expectList(mbbSummaries(types.TargetStatusHealthy, mbbOld...))
	env.expectRegister(mbbNew...)
	env.update(t, newSet)

	// Still within the timeout: held.
	env.now = env.now.Add(5*time.Minute - time.Second)
	env.expectList(stuck)
	env.update(t, newSet)
	assert.Equal(t, 3, env.mgr.holds.len())

	// The replacements never became healthy: released at the timeout.
	env.now = env.now.Add(time.Second)
	env.expectList(stuck)
	env.expectDeregister(mbbOld...)
	env.update(t, newSet)
	assert.Equal(t, 0, env.mgr.holds.len())
}

func TestMakeBeforeBreak_EmptyModelDeregistersEverything(t *testing.T) {
	env := newMBBEnv(t, true)
	env.expectList(mbbSummaries(types.TargetStatusHealthy, mbbOld...))
	env.expectDeregister(mbbOld...)
	env.update(t, mbbTargets())
	assert.Equal(t, 0, env.mgr.holds.len())
}

func TestMakeBeforeBreak_PodListFailureDeregisters(t *testing.T) {
	env := newMBBEnv(t, true, func(b *testclient.ClientBuilder) {
		b.WithInterceptorFuncs(interceptor.Funcs{
			List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				return errors.New("list failed")
			},
		})
	})
	env.expectList(mbbSummaries(types.TargetStatusHealthy, mbbOld...))
	env.expectRegister(mbbNew...)
	env.expectDeregister(mbbOld...)
	env.update(t, mbbTargets(mbbNew...))
	assert.Equal(t, 0, env.mgr.holds.len())
}

func TestTargetsSynthesizer_MakeBeforeBreakRequeue(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
		lattice []types.TargetSummary
		requeue bool
	}{
		{
			name:    "flag on, departing target still registered",
			enabled: true,
			lattice: concat(mbbSummaries(types.TargetStatusInitial, mbbNew...), mbbSummaries(types.TargetStatusHealthy, "old-0")),
			requeue: true,
		},
		{
			name:    "flag on, departing target draining",
			enabled: true,
			lattice: concat(mbbSummaries(types.TargetStatusHealthy, mbbNew...), mbbSummaries(types.TargetStatusDraining, "old-0")),
			requeue: false,
		},
		{
			name:    "flag on, only desired targets",
			enabled: true,
			lattice: mbbSummaries(types.TargetStatusInitial, mbbNew...),
			requeue: false,
		},
		{
			name:    "flag off, departing target still registered",
			enabled: false,
			lattice: concat(mbbSummaries(types.TargetStatusInitial, mbbNew...), mbbSummaries(types.TargetStatusHealthy, "old-0")),
			requeue: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prev := config.MakeBeforeBreakTargets
			config.MakeBeforeBreakTargets = tt.enabled
			t.Cleanup(func() { config.MakeBeforeBreakTargets = prev })

			c := gomock.NewController(t)
			mockTargetsManager := NewMockTargetsManager(c)
			stack := core.NewDefaultStack(core.StackID{Name: "svc", Namespace: mbbNamespace})
			modelTg := &model.TargetGroup{
				ResourceMeta: core.NewResourceMeta(stack, "AWS:VPCServiceNetwork::TargetGroup", "tg-stack-id"),
				Status:       &model.TargetGroupStatus{Name: "tg-name", Arn: "tg-arn", Id: "tg-id"},
			}
			require.NoError(t, stack.AddResource(modelTg))
			// Ready targets, so readiness gate handling does not requeue.
			model.NewTargets(stack, mbbTargets(mbbNew...).Spec)
			mockTargetsManager.EXPECT().List(gomock.Any(), gomock.Any()).Return(tt.lattice, nil)

			synthesizer := NewTargetsSynthesizer(gwlog.FallbackLogger, testclient.NewClientBuilder().Build(), mockTargetsManager, stack)
			err := synthesizer.PostSynthesize(context.TODO())
			if tt.requeue {
				var requeueNeededAfter *lattice_runtime.RequeueNeededAfter
				assert.True(t, errors.As(err, &requeueNeededAfter), "want requeue, got %v", err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestMakeBeforeBreak_NormalDeregisterDoesNotRequeue(t *testing.T) {
	env := newMBBEnv(t, true)
	ctx := context.TODO()
	// old-2 is terminating, so it is deregistered at once and never held.
	pod := &corev1.Pod{}
	require.NoError(t, env.k8s.Get(ctx, apitypes.NamespacedName{Namespace: mbbNamespace, Name: "old-2"}, pod))
	pod.Finalizers = []string{"test/keep"}
	require.NoError(t, env.k8s.Update(ctx, pod))
	require.NoError(t, env.k8s.Delete(ctx, pod))

	desired := mbbTargets("old-0", "old-1", "new-0")
	env.expectList(mbbSummaries(types.TargetStatusHealthy, mbbOld...))
	env.expectRegister("new-0")
	env.expectDeregister("old-2")
	env.update(t, desired)

	// PostSynthesize lists the target group again after Update. The deregistered
	// old-2 shows as DRAINING there, which is not a held target, so no requeue.
	env.expectList(concat(
		mbbSummaries(types.TargetStatusHealthy, "old-0", "old-1", "new-0"),
		mbbSummaries(types.TargetStatusDraining, "old-2"),
	))
	stack := core.NewDefaultStack(core.StackID{Name: "svc", Namespace: mbbNamespace})
	tg := &model.TargetGroup{
		ResourceMeta: core.NewResourceMeta(stack, "AWS:VPCServiceNetwork::TargetGroup", "tg-stack-id"),
		Status:       env.tg.Status,
	}
	require.NoError(t, stack.AddResource(tg))
	model.NewTargets(stack, desired.Spec)
	synthesizer := NewTargetsSynthesizer(gwlog.FallbackLogger, env.k8s, env.mgr, stack)
	assert.NoError(t, synthesizer.PostSynthesize(ctx))
}

func TestNotReadyReplacements(t *testing.T) {
	desired := mbbTargets(mbbNew...).Spec.TargetList

	// UNAVAILABLE and UNUSED count as ready; new-2 is not registered yet.
	got := notReadyReplacements(desired, concat(
		mbbSummaries(types.TargetStatusHealthy, "new-0"),
		mbbSummaries(types.TargetStatusUnavailable, "new-1"),
	))
	assert.Equal(t, []string{"10.0.1.3:8080 (NOT_REGISTERED_YET)"}, got)

	// A DRAINING replacement is registered again, so it is not ready.
	got = notReadyReplacements(desired, concat(
		mbbSummaries(types.TargetStatusHealthy, "new-0", "new-1"),
		mbbSummaries(types.TargetStatusDraining, "new-2"),
	))
	assert.Equal(t, []string{"10.0.1.3:8080 (DRAINING)"}, got)

	assert.Empty(t, notReadyReplacements(desired, concat(
		mbbSummaries(types.TargetStatusHealthy, "new-0", "new-1"),
		mbbSummaries(types.TargetStatusUnused, "new-2"),
	)))
}

func TestJoinCapped(t *testing.T) {
	items := []string{"a", "b", "c", "d", "e", "f", "g"}
	assert.Equal(t, "a, b", joinCapped(items[:2]))
	assert.Equal(t, "a, b, c, d, e and 2 more", joinCapped(items))
}

func TestTargetHolds_ForgetDropsOnlyThatTargetGroup(t *testing.T) {
	h := newTargetHolds(time.Now)
	now := time.Unix(1_000_000, 0)
	a := holdKey{tgId: "tg-a", ip: "10.0.0.1", port: 80}
	b := holdKey{tgId: "tg-b", ip: "10.0.0.2", port: 80}
	h.start(a, now)
	h.start(b, now)

	h.forget("tg-a")
	assert.Equal(t, 1, h.len())
	// tg-b kept its original hold start.
	assert.Equal(t, now, h.start(b, now.Add(time.Hour)))
}
