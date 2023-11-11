package deploy

import (
	"context"
	"fmt"
	"github.com/aws/aws-application-networking-k8s/pkg/gateway"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/deploy/externaldns"
	"github.com/aws/aws-application-networking-k8s/pkg/deploy/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
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

func NewLatticeServiceStackDeploy(
	log gwlog.Logger,
	cloud pkg_aws.Cloud,
	k8sClient client.Client,
) *latticeServiceStackDeployer {
	brTgBuilder := gateway.NewBackendRefTargetGroupBuilder(log, k8sClient)

	return &latticeServiceStackDeployer{
		log:                   log,
		cloud:                 cloud,
		k8sClient:             k8sClient,
		latticeServiceManager: lattice.NewServiceManager(log, cloud),
		targetGroupManager:    lattice.NewTargetGroupManager(log, cloud),
		targetsManager:        lattice.NewTargetsManager(log, cloud),
		listenerManager:       lattice.NewListenerManager(log, cloud),
		ruleManager:           lattice.NewRuleManager(log, cloud),
		dnsEndpointManager:    externaldns.NewDnsEndpointManager(log, k8sClient),
		svcExportTgBuilder:    gateway.NewSvcExportTargetGroupBuilder(log, k8sClient),
		svcBuilder:            gateway.NewLatticeServiceBuilder(log, k8sClient, brTgBuilder),
	}
}

func (d *latticeServiceStackDeployer) Deploy(ctx context.Context, stack core.Stack) error {
	targetGroupSynthesizer := lattice.NewTargetGroupSynthesizer(d.log, d.cloud, d.k8sClient, d.targetGroupManager, d.svcExportTgBuilder, d.svcBuilder, stack)
	targetsSynthesizer := lattice.NewTargetsSynthesizer(d.log, d.cloud, d.targetsManager, stack)
	serviceSynthesizer := lattice.NewServiceSynthesizer(d.log, d.latticeServiceManager, d.dnsEndpointManager, stack)
	listenerSynthesizer := lattice.NewListenerSynthesizer(d.log, d.listenerManager, stack)
	ruleSynthesizer := lattice.NewRuleSynthesizer(d.log, d.ruleManager, d.targetGroupManager, stack)

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

	//Handle targetGroup deletion request
	if err := targetGroupSynthesizer.SynthesizeDelete(ctx); err != nil {
		return fmt.Errorf("error during tg delete synthesis %w", err)
	}

	// Do garbage collection for not-in-use targetGroups
	//TODO: run SynthesizeSDKTargetGroups(ctx) as a global garbage collector scheduled background task (i.e., run it as a goroutine in main.go)
	if err := targetGroupSynthesizer.SynthesizeUnusedDelete(ctx); err != nil {
		return fmt.Errorf("error during tg unused delete synthesis %w", err)
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
	synthesizers := []ResourceSynthesizer{
		lattice.NewTargetGroupSynthesizer(d.log, d.cloud, d.k8sclient, d.targetGroupManager, d.svcExportTgBuilder, d.svcBuilder, stack),
		lattice.NewTargetsSynthesizer(d.log, d.cloud, lattice.NewTargetsManager(d.log, d.cloud), stack),
	}
	return deploy(ctx, stack, synthesizers)
}

type accessLogSubscriptionStackDeployer struct {
	log       gwlog.Logger
	k8sClient client.Client
	stack     core.Stack
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
