package lattice

import (
	"context"
	"fmt"

	lattice_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

func NewTargetsSynthesizer(
	log gwlog.Logger,
	cloud lattice_aws.Cloud,
	tgManager TargetsManager,
	stack core.Stack,
	latticeDataStore *latticestore.LatticeDataStore,
) *targetsSynthesizer {
	return &targetsSynthesizer{
		log:              log,
		cloud:            cloud,
		targetsManager:   tgManager,
		stack:            stack,
		latticeDataStore: latticeDataStore,
	}
}

type targetsSynthesizer struct {
	log              gwlog.Logger
	cloud            lattice_aws.Cloud
	targetsManager   TargetsManager
	stack            core.Stack
	latticeDataStore *latticestore.LatticeDataStore
}

func (t *targetsSynthesizer) Synthesize(ctx context.Context) error {
	var resTargets []*latticemodel.Targets
	err := t.stack.ListResources(&resTargets)
	if err != nil {
		t.log.Errorf("Failed to list targets due to %s", err)
	}
	return t.SynthesizeTargets(ctx, resTargets)
}

func (t *targetsSynthesizer) SynthesizeTargets(ctx context.Context, resTargets []*latticemodel.Targets) error {
	for _, targets := range resTargets {
		err := t.targetsManager.Create(ctx, targets)
		if err != nil {
			return fmt.Errorf("failed to synthesize targets due to %s", err)
		}

		tgName := latticestore.TargetGroupName(targets.Spec.Name, targets.Spec.Namespace)
		var targetList []latticestore.Target
		for _, target := range targets.Spec.TargetIPList {
			targetList = append(targetList, latticestore.Target{
				TargetIP:   target.TargetIP,
				TargetPort: target.Port,
			})
		}

		err = t.latticeDataStore.UpdateTargetsForTargetGroup(tgName, targets.Spec.RouteName, targetList)
		if err != nil {
			t.log.Errorf("Failed to update targets for target group %s due to %s", tgName, err)
		}
	}
	return nil
}

func (t *targetsSynthesizer) synthesizeSDKTargets(ctx context.Context) error {
	return nil
}

func (t *targetsSynthesizer) PostSynthesize(ctx context.Context) error {
	// nothing to do here
	return nil
}
