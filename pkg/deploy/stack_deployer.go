package deploy

import (
	"context"
	"fmt"
	"sync"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/deploy/externaldns"
	"github.com/aws/aws-application-networking-k8s/pkg/deploy/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/gateway"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

const (
	TG_GC_IVL = time.Second * 30
)

type StackDeployer interface {
	Deploy(ctx context.Context, stack core.Stack) error
}

type ResourceSynthesizer interface {
	Synthesize(ctx context.Context) error
	PostSynthesize(ctx context.Context) error
}

// Deploy a resource stack
func deploy(ctx context.Context, stack core.Stack, synthesizers []ResourceSynthesizer) error {
	for _, synthesizer := range synthesizers {
		if err := synthesizer.Synthesize(ctx); err != nil {
			return err
		}
	}
	for i := len(synthesizers) - 1; i >= 0; i-- {
		if err := synthesizers[i].PostSynthesize(ctx); err != nil {
			return err
		}
	}

	return nil
}

type latticeServiceStackDeployer struct {
	log                   gwlog.Logger
	cloud                 pkg_aws.Cloud
	k8sClient             client.Client
	latticeServiceManager lattice.ServiceManager
	targetGroupManager    lattice.TargetGroupManager
	targetsManager        lattice.TargetsManager
	listenerManager       lattice.ListenerManager
	ruleManager           lattice.RuleManager
	dnsEndpointManager    externaldns.DnsEndpointManager
	svcExportTgBuilder    gateway.SvcExportTargetGroupModelBuilder
	svcBuilder            gateway.LatticeServiceBuilder
}

// Deploys hold the read lock and a TG GC cycle holds the write lock, so the GC never sees a
// target group that a deploy has created but not yet attached to a rule. The lock only
// works within one process, which is why the GC runs on the elected leader only.
var tgGcLock sync.RWMutex

func NewLatticeServiceStackDeploy(
	log gwlog.Logger,
	cloud pkg_aws.Cloud,
	k8sClient client.Client,
) *latticeServiceStackDeployer {
	brTgBuilder := gateway.NewBackendRefTargetGroupBuilder(log, k8sClient)

	tgMgr := lattice.NewTargetGroupManager(log, cloud, k8sClient)
	tgSvcExpBuilder := gateway.NewSvcExportTargetGroupBuilder(log, k8sClient)
	svcBuilder := gateway.NewLatticeServiceBuilder(log, k8sClient, brTgBuilder, nil, nil)

	return &latticeServiceStackDeployer{
		log:                   log,
		cloud:                 cloud,
		k8sClient:             k8sClient,
		latticeServiceManager: lattice.NewServiceManager(log, cloud),
		targetGroupManager:    tgMgr,
		targetsManager:        lattice.NewTargetsManager(log, cloud),
		listenerManager:       lattice.NewListenerManager(log, cloud),
		ruleManager:           lattice.NewRuleManager(log, cloud),
		dnsEndpointManager:    externaldns.NewDnsEndpointManager(log, k8sClient),
		svcExportTgBuilder:    tgSvcExpBuilder,
		svcBuilder:            svcBuilder,
	}
}

type TgGcCycleFn = func(context.Context) (TgGcResult, error)

func NewTgGcFn(tgSynth *lattice.TargetGroupSynthesizer) TgGcCycleFn {
	return func(ctx context.Context) (TgGcResult, error) {
		t0 := time.Now()
		results, err := tgSynth.SynthesizeUnusedDelete(ctx)
		if err != nil {
			return TgGcResult{}, err
		}
		succ := 0
		for _, res := range results {
			if res.Err == nil {
				succ += 1
			}
		}
		return TgGcResult{
			att:      len(results),
			succ:     succ,
			duration: time.Since(t0),
		}, nil
	}
}

// TgGc periodically deletes target groups that are no longer used. It is a manager runnable
// that needs leader election, so standby replicas never delete target groups.
type TgGc struct {
	log     gwlog.Logger
	ivl     time.Duration
	cycleFn TgGcCycleFn
}

var _ manager.LeaderElectionRunnable = &TgGc{}

func NewTgGc(log gwlog.Logger, cloud pkg_aws.Cloud, k8sClient client.Client) *TgGc {
	brTgBuilder := gateway.NewBackendRefTargetGroupBuilder(log, k8sClient)
	tgSynth := lattice.NewTargetGroupSynthesizer(log, cloud, k8sClient,
		lattice.NewTargetGroupManager(log, cloud, k8sClient),
		gateway.NewSvcExportTargetGroupBuilder(log, k8sClient),
		gateway.NewLatticeServiceBuilder(log, k8sClient, brTgBuilder, nil, nil),
		nil)
	return &TgGc{
		log:     log,
		ivl:     TG_GC_IVL,
		cycleFn: NewTgGcFn(tgSynth),
	}
}

type TgGcResult struct {
	// number deletion attempts
	att int
	// number of successful deletions
	succ int
	// cycle duration
	duration time.Duration
}

func (gc *TgGc) NeedLeaderElection() bool {
	return true
}

func (gc *TgGc) Start(ctx context.Context) error {
	ticker := time.NewTicker(gc.ivl)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			gc.log.Info(context.TODO(), "stop GC, ctx is done")
			return nil
		case <-ticker.C:
			if ctx.Err() != nil {
				return nil
			}
			gc.cycle(ctx)
		}
	}
}

func (gc *TgGc) cycle(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			gc.log.Errorf(context.TODO(), "gc cycle panic: %s", r)
		}
		tgGcLock.Unlock()
	}()
	tgGcLock.Lock()
	res, err := gc.cycleFn(ctx)
	if err != nil {
		gc.log.Debugf(context.TODO(), "gc cycle error: %s", err)
	}
	gc.log.Debugw(context.TODO(), "gc stats",
		"delete_attempts", res.att,
		"delete_success", res.succ,
		"duration", res.duration,
	)
}

func (d *latticeServiceStackDeployer) Deploy(ctx context.Context, stack core.Stack) error {
	targetGroupSynthesizer := lattice.NewTargetGroupSynthesizer(d.log, d.cloud, d.k8sClient, d.targetGroupManager, d.svcExportTgBuilder, d.svcBuilder, stack)
	targetsSynthesizer := lattice.NewTargetsSynthesizer(d.log, d.k8sClient, d.targetsManager, stack)
	serviceSynthesizer := lattice.NewServiceSynthesizer(d.log, d.latticeServiceManager, d.dnsEndpointManager, stack)
	listenerSynthesizer := lattice.NewListenerSynthesizer(d.log, d.listenerManager, d.targetGroupManager, stack)
	ruleSynthesizer := lattice.NewRuleSynthesizer(d.log, d.ruleManager, d.targetGroupManager, stack)

	defer func() {
		tgGcLock.RUnlock()
	}()
	tgGcLock.RLock()

	//Handle targetGroups creation request
	if err := targetGroupSynthesizer.SynthesizeCreate(ctx); err != nil {
		return fmt.Errorf("error during tg synthesis %w", err)
	}

	//Handle targets "reconciliation" request (register intend-to-be-registered targets and deregister intend-to-be-registered targets)
	if err := targetsSynthesizer.Synthesize(ctx); err != nil {
		return fmt.Errorf("error during target synthesis %w", err)
	}

	// Handle latticeService "reconciliation" request
	if err := serviceSynthesizer.Synthesize(ctx); err != nil {
		return fmt.Errorf("error during service synthesis %w", err)
	}

	//Handle latticeService listeners "reconciliation" request
	if err := listenerSynthesizer.Synthesize(ctx); err != nil {
		return fmt.Errorf("error during listener synthesis %w", err)
	}

	//Handle latticeService listener's rules "reconciliation" request
	if err := ruleSynthesizer.Synthesize(ctx); err != nil {
		return fmt.Errorf("error during rule synthesis %w", err)
	}

	// Handle pod status update for targets.
	if err := targetsSynthesizer.PostSynthesize(ctx); err != nil {
		return fmt.Errorf("error during target post synthesis %w", err)
	}

	//Handle targetGroup deletion request
	if err := targetGroupSynthesizer.SynthesizeDelete(ctx); err != nil {
		return fmt.Errorf("error during tg delete synthesis %w", err)
	}

	return nil
}

type latticeTargetGroupStackDeployer struct {
	log                gwlog.Logger
	cloud              pkg_aws.Cloud
	k8sclient          client.Client
	targetGroupManager lattice.TargetGroupManager
	svcExportTgBuilder gateway.SvcExportTargetGroupModelBuilder
	svcBuilder         gateway.LatticeServiceBuilder
}

// triggered by service export
func NewTargetGroupStackDeploy(
	log gwlog.Logger,
	cloud pkg_aws.Cloud,
	k8sClient client.Client,
) *latticeTargetGroupStackDeployer {
	brTgBuilder := gateway.NewBackendRefTargetGroupBuilder(log, k8sClient)

	return &latticeTargetGroupStackDeployer{
		log:                log,
		cloud:              cloud,
		k8sclient:          k8sClient,
		targetGroupManager: lattice.NewTargetGroupManager(log, cloud, k8sClient),
		svcExportTgBuilder: gateway.NewSvcExportTargetGroupBuilder(log, k8sClient),
		svcBuilder:         gateway.NewLatticeServiceBuilder(log, k8sClient, brTgBuilder, nil, nil),
	}
}

func (d *latticeTargetGroupStackDeployer) Deploy(ctx context.Context, stack core.Stack) error {
	defer func() {
		tgGcLock.RUnlock()
	}()
	tgGcLock.RLock()

	synthesizers := []ResourceSynthesizer{
		lattice.NewTargetGroupSynthesizer(d.log, d.cloud, d.k8sclient, d.targetGroupManager, d.svcExportTgBuilder, d.svcBuilder, stack),
		lattice.NewTargetsSynthesizer(d.log, d.k8sclient, lattice.NewTargetsManager(d.log, d.cloud), stack),
	}
	return deploy(ctx, stack, synthesizers)
}

type accessLogSubscriptionStackDeployer struct {
	log       gwlog.Logger
	k8sClient client.Client
	manager   lattice.AccessLogSubscriptionManager
}

func NewAccessLogSubscriptionStackDeployer(
	log gwlog.Logger,
	cloud pkg_aws.Cloud,
	k8sClient client.Client,
) *accessLogSubscriptionStackDeployer {
	return &accessLogSubscriptionStackDeployer{
		log:       log,
		k8sClient: k8sClient,
		manager:   lattice.NewAccessLogSubscriptionManager(log, cloud),
	}
}

func (d *accessLogSubscriptionStackDeployer) Deploy(ctx context.Context, stack core.Stack) error {
	synthesizers := []ResourceSynthesizer{
		lattice.NewAccessLogSubscriptionSynthesizer(d.log, d.k8sClient, d.manager, stack),
	}
	return deploy(ctx, stack, synthesizers)
}
