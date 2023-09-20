package lattice

import (
	"context"
	"errors"
	"fmt"

	"github.com/golang/glog"

	lattice_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

func NewTargetsSynthesizer(cloud lattice_aws.Cloud, tgManager TargetsManager, stack core.Stack, latticeDataStore *latticestore.LatticeDataStore) *targetsSynthesizer {
	return &targetsSynthesizer{
		cloud:            cloud,
		targetsManager:   tgManager,
		stack:            stack,
		latticeDataStore: latticeDataStore,
	}
}

type targetsSynthesizer struct {
	cloud            lattice_aws.Cloud
	targetsManager   TargetsManager
	stack            core.Stack
	latticeDataStore *latticestore.LatticeDataStore
}

func (t *targetsSynthesizer) Synthesize(ctx context.Context) error {
	var resTargets []*latticemodel.Targets

	t.stack.ListResources(&resTargets)
	glog.V(6).Infof("Synthesize Targets: %v \n", resTargets)

	return t.SynthesizeTargets(ctx, resTargets)

}

func (t *targetsSynthesizer) SynthesizeTargets(ctx context.Context, resTargets []*latticemodel.Targets) error {

	for _, targets := range resTargets {
		err := t.targetsManager.Create(ctx, targets)

		if err != nil {
			errmsg := fmt.Sprintf("TargetSynthesize: Failed to create targets :%v , err:%v\n", targets, err)
			glog.V(6).Infof("Errmsg: %s \n", errmsg)
			return errors.New(errmsg)

		}

		tgName := latticestore.TargetGroupName(targets.Spec.Name, targets.Spec.Namespace)

		var targetList []latticestore.Target

		for _, target := range targets.Spec.TargetIPList {
			targetList = append(targetList, latticestore.Target{
				TargetIP:   target.TargetIP,
				TargetPort: target.Port,
			})
		}

		t.latticeDataStore.UpdateTargetsForTargetGroup(tgName, targets.Spec.RouteName, targetList)

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
