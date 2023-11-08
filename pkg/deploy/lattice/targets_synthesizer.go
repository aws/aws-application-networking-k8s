package lattice

import (
	"context"
	"fmt"

	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

func NewTargetsSynthesizer(
	log gwlog.Logger,
	cloud pkg_aws.Cloud,
	tgManager TargetsManager,
	stack core.Stack,
) *targetsSynthesizer {
	return &targetsSynthesizer{
		log:            log,
		cloud:          cloud,
		targetsManager: tgManager,
		stack:          stack,
	}
}

type targetsSynthesizer struct {
	log            gwlog.Logger
	cloud          pkg_aws.Cloud
	targetsManager TargetsManager
	stack          core.Stack
}

func (t *targetsSynthesizer) Synthesize(ctx context.Context) error {
	var resTargets []*model.Targets
	err := t.stack.ListResources(&resTargets)
	if err != nil {
		t.log.Errorf("Failed to list targets due to %s", err)
	}

	for _, targets := range resTargets {
		tg := &model.TargetGroup{}
		err := t.stack.GetResource(targets.Spec.StackTargetGroupId, tg)
		if err != nil {
			return err
		}

		err = t.targetsManager.Update(ctx, targets, tg)
		if err != nil {
			identifier := model.TgNamePrefix(tg.Spec)
			if tg.Status != nil && tg.Status.Id != "" {
				identifier = tg.Status.Id
			}
			return fmt.Errorf("failed to synthesize targets %s due to %s", identifier, err)
		}
	}
	return nil
}

func (t *targetsSynthesizer) PostSynthesize(ctx context.Context) error {
	// nothing to do here
	return nil
}
