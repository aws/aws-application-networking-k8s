package lattice

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	"github.com/aws/aws-application-networking-k8s/pkg/webhook"
)

const (
	LatticeReadinessGateConditionType = webhook.PodReadinessGateConditionType

	ReadinessReasonHealthy                = "Healthy"
	ReadinessReasonUnhealthy              = "Unhealthy"
	ReadinessReasonUnused                 = "Unused"
	ReadinessReasonInitial                = "Initial"
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
		t.log.Errorf(ctx, "Failed to list targets due to %s", err)
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
		t.log.Errorf(ctx, "Failed to list targets due to %s", err)
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
		// Step 0: Check if the endpoint has a valid target, and is not ready yet.
		if target.Ready || target.TargetRef.Name == "" {
			continue
		}

		// Step 1: Check if the pod has the readiness gate spec.
		pod := &corev1.Pod{}
		t.client.Get(ctx, target.TargetRef, pod)
		if !utils.PodHasReadinessGate(pod, LatticeReadinessGateConditionType) {
			continue
		}

		// Step 2: Check if the pod readiness condition exists with specific condition type.
		// The condition is considered false when it does not exist.
		cond := utils.FindPodStatusCondition(pod.Status.Conditions, LatticeReadinessGateConditionType)
		if cond != nil && cond.Status == corev1.ConditionTrue {
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
			case vpclattice.TargetStatusHealthy:
				newCond.Status = corev1.ConditionTrue
				newCond.Reason = ReadinessReasonHealthy
			case vpclattice.TargetStatusUnavailable:
				// Lattice HC not turned on. Readiness is designed to work only with HC but do not block deployment on this case.
				newCond.Status = corev1.ConditionTrue
				newCond.Reason = ReadinessReasonHealthCheckUnavailable
			case vpclattice.TargetStatusUnused:
				// Since this logic is called after HTTPRoute is wired, this only happens for ServiceExport TGs.
				// In this case we do not have to evaluate them as Healthy, but we also do not have to requeue.
				newCond.Reason = ReadinessReasonUnused
			case vpclattice.TargetStatusInitial:
				requeue = true
				newCond.Reason = ReadinessReasonInitial
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
	return requeue, nil
}
