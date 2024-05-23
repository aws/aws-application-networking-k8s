package deploy

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"

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

var tgGcOnce sync.Once
var tgGc *TgGc

func NewLatticeServiceStackDeploy(
	log gwlog.Logger,
	cloud pkg_aws.Cloud,
	k8sClient client.Client,
) *latticeServiceStackDeployer {
	brTgBuilder := gateway.NewBackendRefTargetGroupBuilder(log, k8sClient)

	tgMgr := lattice.NewTargetGroupManager(log, cloud)
	tgSvcExpBuilder := gateway.NewSvcExportTargetGroupBuilder(log, k8sClient)
	svcBuilder := gateway.NewLatticeServiceBuilder(log, k8sClient, brTgBuilder)

	tgGcOnce.Do(func() {
		// TODO: need to refactor TG synthesizer. Remove stack from constructor
		// arguments and use it as Synth argument. That will help with Synth
		// reuse for GC purposes
		tgGcSynth := lattice.NewTargetGroupSynthesizer(log, cloud, k8sClient, tgMgr, tgSvcExpBuilder, svcBuilder, nil)
		tgGcFn := NewTgGcFn(tgGcSynth)
		tgGc = &TgGc{
			lock:    sync.Mutex{},
			log:     log.Named("tg-gc"),
			ctx:     context.TODO(),
			isDone:  atomic.Bool{},
			ivl:     TG_GC_IVL,
			cycleFn: tgGcFn,
		}
		tgGc.start()
	})

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

type TgGc struct {
	lock    sync.Mutex
	log     gwlog.Logger
	ctx     context.Context
	isDone  atomic.Bool
	ivl     time.Duration
	cycleFn TgGcCycleFn
}

type TgGcResult struct {
	// number deletion attempts
	att int
	// number of successful deletions
	succ int
	// cycle duration
	duration time.Duration
}

func (gc *TgGc) start() {
	ticker := time.NewTicker(gc.ivl)
	go func() {
		for {
			select {
			case <-gc.ctx.Done():
				gc.log.Info("stop GC, ctx is done")
				gc.isDone.Store(true)
				return
			case <-ticker.C:
				gc.cycle()
			}
		}
	}()
}

func (gc *TgGc) cycle() {
	defer func() {
		if r := recover(); r != nil {
			gc.log.Errorf("gc cycle panic: %s", r)
		}
		gc.lock.Unlock()
	}()
	gc.lock.Lock()
	res, err := gc.cycleFn(gc.ctx)
	if err != nil {
		gc.log.Debugf("gc cycle error: %s", err)
	}
	gc.log.Debugw("gc stats",
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

	// We need to block GC when we deploy stack. Stack deployer first creates TG and then
	// associate TG with Service. If GC will run in between it can delete newly created TG
	// before association since it's dangling TG. This lock also prevents concurrent
	// deployments, only one deployment can run at the time.
	//
	// TODO: This place can become a contention. May be debug log with lock waiting time?
	defer func() {
		tgGc.lock.Unlock()
	}()
	tgGc.lock.Lock()

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
		targetGroupManager: lattice.NewTargetGroupManager(log, cloud),
		svcExportTgBuilder: gateway.NewSvcExportTargetGroupBuilder(log, k8sClient),
		svcBuilder:         gateway.NewLatticeServiceBuilder(log, k8sClient, brTgBuilder),
	}
}

func (d *latticeTargetGroupStackDeployer) Deploy(ctx context.Context, stack core.Stack) error {
	defer func() {
		tgGc.lock.Unlock()
	}()
	tgGc.lock.Lock()

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
