package lattice

import (
	"context"
	"fmt"

	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// TODO: use the constant on webhook side instead.
	LatticeReadinessGateConditionType = "aws-application-networking-k8s/pod-readiness-gate"

	ReadinessReasonHealthy                = "Healthy"
	ReadinessReasonUnhealthy              = "Unhealthy"
	ReadinessReasonHealthCheckUnavailable = "HealthCheckUnavailable"
	ReadinessReasonTargetNotFound         = "TargetNotFound"
)

func NewTargetsSynthesizer(
	log gwlog.Logger,
	client client.Client,
	tgManager TargetsManager,
	stack core.Stack,
) *targetsSynthesizer {
	return &targetsSynthesizer{
		log:            log,
		client:         client,
		targetsManager: tgManager,
		stack:          stack,
	}
}

type targetsSynthesizer struct {
	log            gwlog.Logger
	client         client.Client
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
	var resTargets []*model.Targets
	err := t.stack.ListResources(&resTargets)
	if err != nil {
		t.log.Errorf("Failed to list targets due to %s", err)
	}

	requeueNeeded := false
	for _, targets := range resTargets {
		tg := &model.TargetGroup{}
		err := t.stack.GetResource(targets.Spec.StackTargetGroupId, tg)
		if err != nil {
			return err
		}

		identifier := model.TgNamePrefix(tg.Spec)
		if tg.Status != nil && tg.Status.Id != "" {
			identifier = tg.Status.Id
		}

		latticeTargets, err := t.targetsManager.List(ctx, tg)
		if err != nil {
			return fmt.Errorf("failed post-synthesize targets %s, ListTargets failure: %w", identifier, err)
		}

		pending, err := t.syncStatus(ctx, targets.Spec.TargetList, latticeTargets)
		if err != nil {
			return fmt.Errorf("failed post-synthesize targets %s, condition sync failure: %w", identifier, err)
		}
		requeueNeeded = requeueNeeded || pending
	}

	if requeueNeeded {
		return fmt.Errorf("%w: target status still in pending", RetryErr)
	}
	return nil
}

func (t *targetsSynthesizer) syncStatus(ctx context.Context, modelTargets []model.Target, latticeTargets []*vpclattice.TargetSummary) (bool, error) {
	// Extract Lattice targets as a set
	latticeTargetMap := make(map[model.Target]*vpclattice.TargetSummary)

	for _, latticeTarget := range latticeTargets {
		ipPort := model.Target{
			TargetIP: aws.StringValue(latticeTarget.Id),
			Port:     aws.Int64Value(latticeTarget.Port),
		}
		latticeTargetMap[ipPort] = latticeTarget
	}

	var requeue bool
	for _, target := range modelTargets {
		// Step 0: Check if the endpoint is not ready yet.
		if !target.Ready && target.TargetRef.Name != "" {
			pod := &corev1.Pod{}
			t.client.Get(ctx, target.TargetRef, pod)

			// Step 1: Check if the pod has the readiness gate spec.
			if !utils.PodHasReadinessGate(pod, LatticeReadinessGateConditionType) {
				continue
			}

			// Step 2: Check if the pod readiness condition is owned by controller.
			cond := utils.FindPodStatusCondition(pod.Status.Conditions, LatticeReadinessGateConditionType)
			if cond.Status == corev1.ConditionTrue {
				continue
			}

			// Step 3: Check if the Lattice target is healthy.
			newCond := corev1.PodCondition{
				Type:   LatticeReadinessGateConditionType,
				Status: corev1.ConditionFalse,
			}
			targetIpPort := model.Target{
				TargetIP: target.TargetIP,
				Port:     target.Port,
			}
			// syncStatus is called at post synthesis, so we can assume:
			// 1. Target for the pod (eventually) exists. If the target doesn't exist, we can simply requeue.
			// 2. Target group will be always in use, except for ServiceExport TGs.
			if latticeTarget, ok := latticeTargetMap[targetIpPort]; ok {
				switch status := aws.StringValue(latticeTarget.Status); status {
				case vpclattice.TargetStatusHealthy, vpclattice.TargetStatusUnused:
					// For ServiceExport TGs, we consider the target healthy in the beginning - as there will be
					// a reasonable time gap between creating a target group and wiring it to the route.
					newCond.Status = corev1.ConditionTrue
					newCond.Reason = ReadinessReasonHealthy
				case vpclattice.TargetStatusUnavailable:
					// Lattice HC not turned on, do not block deployment on this case.
					newCond.Status = corev1.ConditionTrue
					newCond.Reason = ReadinessReasonHealthCheckUnavailable
				default:
					requeue = true
					newCond.Reason = ReadinessReasonUnhealthy
					newCond.Message = fmt.Sprintf("Target health check status: %s", status)
				}
			} else {
				requeue = true
				newCond.Reason = ReadinessReasonTargetNotFound
			}

			// Step 4: Update status.
			utils.SetPodStatusCondition(&pod.Status.Conditions, newCond)
			if err := t.client.Status().Update(ctx, pod); err != nil {
				return requeue, err
			}
		}
	}
	return requeue, nil
}
