package deploy

import (
	"context"

	"github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/deploy/externaldns"
	"github.com/aws/aws-application-networking-k8s/pkg/deploy/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// StackDeployer will deploy a resource stack into AWS and K8S.
type StackDeployer interface {
	// Deploy a resource stack.
	Deploy(ctx context.Context, stack core.Stack) error
}

//var _ StackDeployer = &defaultStackDeployer{}

// TODO,  later might have a single stack, righ now will have
// dedicated stack for serviceNetwork/service/targetgroup
type serviceNetworkStackDeployer struct {
	cloud     aws.Cloud
	k8sclient client.Client
	// TODO vpcID     string

	//TODO others
	latticeServiceNetworkManager lattice.ServiceNetworkManager
	latticeDataStore             *latticestore.LatticeDataStore
}

type ResourceSynthesizer interface {
	Synthesize(ctx context.Context) error
	PostSynthesize(ctx context.Context) error
}

func NewServiceNetworkStackDeployer(cloud aws.Cloud, k8sClient client.Client, latticeDataStore *latticestore.LatticeDataStore) *serviceNetworkStackDeployer {
	return &serviceNetworkStackDeployer{
		cloud:                        cloud,
		k8sclient:                    k8sClient,
		latticeServiceNetworkManager: lattice.NewDefaultServiceNetworkManager(cloud),
		latticeDataStore:             latticeDataStore,
	}
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

func (d *serviceNetworkStackDeployer) Deploy(ctx context.Context, stack core.Stack) error {
	synthesizers := []ResourceSynthesizer{
		lattice.NewServiceNetworkSynthesizer(d.k8sclient, d.latticeServiceNetworkManager, stack, d.latticeDataStore),
	}
	return deploy(ctx, stack, synthesizers)
}

type latticeServiceStackDeployer struct {
	cloud                 aws.Cloud
	k8sclient             client.Client
	latticeServiceManager lattice.ServiceManager
	targetGroupManager    lattice.TargetGroupManager
	targetsManager        lattice.TargetsManager
	listenerManager       lattice.ListenerManager
	ruleManager           lattice.RuleManager
	dnsEndpointManager    externaldns.DnsEndpointManager
	latticeDataStore      *latticestore.LatticeDataStore
}

func NewLatticeServiceStackDeploy(cloud aws.Cloud, k8sClient client.Client, latticeDataStore *latticestore.LatticeDataStore) *latticeServiceStackDeployer {
	return &latticeServiceStackDeployer{
		cloud:                 cloud,
		k8sclient:             k8sClient,
		latticeServiceManager: lattice.NewServiceManager(cloud, latticeDataStore),
		targetGroupManager:    lattice.NewTargetGroupManager(cloud),
		targetsManager:        lattice.NewTargetsManager(cloud, latticeDataStore),
		listenerManager:       lattice.NewListenerManager(cloud, latticeDataStore),
		ruleManager:           lattice.NewRuleManager(cloud, latticeDataStore),
		dnsEndpointManager:    externaldns.NewDnsEndpointManager(k8sClient),
		latticeDataStore:      latticeDataStore,
	}
}

func (d *latticeServiceStackDeployer) Deploy(ctx context.Context, stack core.Stack) error {
	targetGroupSynthesizer := lattice.NewTargetGroupSynthesizer(d.cloud, d.k8sclient, d.targetGroupManager, stack, d.latticeDataStore)
	targetsSynthesizer := lattice.NewTargetsSynthesizer(d.cloud, d.targetsManager, stack, d.latticeDataStore)
	serviceSynthesizer := lattice.NewServiceSynthesizer(d.latticeServiceManager, d.dnsEndpointManager, stack, d.latticeDataStore)
	listenerSynthesizer := lattice.NewListenerSynthesizer(d.listenerManager, stack, d.latticeDataStore)
	ruleSynthesizer := lattice.NewRuleSynthesizer(d.ruleManager, stack, d.latticeDataStore)

	//Handle targetGroups creation request
	if err := targetGroupSynthesizer.SynthesizeTriggeredTargetGroupsCreation(ctx); err != nil {
		return err
	}

	//Handle targets "reconciliation" request (register intend-to-be-registered targets and deregister intend-to-be-registered targets)
	if err := targetsSynthesizer.Synthesize(ctx); err != nil {
		return err
	}

	// Handle latticeService "reconciliation" request
	if err := serviceSynthesizer.Synthesize(ctx); err != nil {
		return err
	}

	//Handle latticeService listeners "reconciliation" request
	if err := listenerSynthesizer.Synthesize(ctx); err != nil {
		return err
	}

	//Handle latticeService listener's rules "reconciliation" request
	if err := ruleSynthesizer.Synthesize(ctx); err != nil {
		return err
	}

	//Handle targetGroup deletion request
	if err := targetGroupSynthesizer.SynthesizeTriggeredTargetGroupsDeletion(ctx); err != nil {
		return err
	}

	// Do garbage collection for not-in-use targetGroups
	//TODO: run SynthesizeSDKTargetGroups(ctx) as a global garbage collector scheduled backgroud task (i.e., run it as a goroutine in main.go)
	if err := targetGroupSynthesizer.SynthesizeSDKTargetGroups(ctx); err != nil {
		return err
	}

	return nil
}

type latticeTargetGroupStackDeployer struct {
	cloud              aws.Cloud
	k8sclient          client.Client
	targetGroupManager lattice.TargetGroupManager
	latticeDatastore   *latticestore.LatticeDataStore
}

// triggered by service export
func NewTargetGroupStackDeploy(cloud aws.Cloud, k8sClient client.Client, latticeDataStore *latticestore.LatticeDataStore) *latticeTargetGroupStackDeployer {
	return &latticeTargetGroupStackDeployer{
		cloud:              cloud,
		k8sclient:          k8sClient,
		targetGroupManager: lattice.NewTargetGroupManager(cloud),
		latticeDatastore:   latticeDataStore,
	}
}

func (d *latticeTargetGroupStackDeployer) Deploy(ctx context.Context, stack core.Stack) error {
	synthesizers := []ResourceSynthesizer{
		lattice.NewTargetGroupSynthesizer(d.cloud, d.k8sclient, d.targetGroupManager, stack, d.latticeDatastore),
		lattice.NewTargetsSynthesizer(d.cloud, lattice.NewTargetsManager(d.cloud, d.latticeDatastore), stack, d.latticeDatastore),
	}
	return deploy(ctx, stack, synthesizers)
}

type latticeTargetsStackDeploy struct {
	k8sclient        client.Client
	stack            core.Stack
	targetsManager   lattice.TargetsManager
	latticeDataStore *latticestore.LatticeDataStore
}

func NewTargetsStackDeploy(cloud aws.Cloud, k8sClient client.Client, latticeDataStore *latticestore.LatticeDataStore) *latticeTargetsStackDeploy {
	return &latticeTargetsStackDeploy{
		k8sclient:        k8sClient,
		targetsManager:   lattice.NewTargetsManager(cloud, latticeDataStore),
		latticeDataStore: latticeDataStore,
	}

}

func (d *latticeTargetsStackDeploy) Deploy(ctx context.Context, stack core.Stack) error {
	var resTargets []*latticemodel.Targets

	d.stack = stack

	d.stack.ListResources(&resTargets)

	for _, targets := range resTargets {
		err := d.targetsManager.Create(ctx, targets)
		if err == nil {
			tgName := latticestore.TargetGroupName(targets.Spec.Name, targets.Spec.Namespace)

			var targetList []latticestore.Target
			for _, target := range targetList {
				t := latticestore.Target{
					TargetIP:   target.TargetIP,
					TargetPort: target.TargetPort,
				}

				targetList = append(targetList, t)

			}
			d.latticeDataStore.UpdateTargetsForTargetGroup(tgName, targets.Spec.RouteName, targetList)
		}

	}
	return nil
}
