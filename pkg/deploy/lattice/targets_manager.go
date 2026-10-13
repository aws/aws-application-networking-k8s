package lattice

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/vpclattice"
	"github.com/aws/aws-sdk-go-v2/service/vpclattice/types"

	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	"github.com/aws/aws-sdk-go-v2/aws"
)

const (
	// Maximum allowed number of targets per each VPC Lattice RegisterTargets/DeregisterTargets API call
	// https://docs.aws.amazon.com/vpc-lattice/latest/APIReference/API_RegisterTargets.html
	maxTargetsPerLatticeTargetsApiCall = 100
)

//go:generate mockgen -destination targets_manager_mock.go -package lattice github.com/aws/aws-application-networking-k8s/pkg/deploy/lattice TargetsManager

type TargetsManager interface {
	List(ctx context.Context, modelTg *model.TargetGroup) ([]types.TargetSummary, error)
	Update(ctx context.Context, modelTargets *model.Targets, modelTg *model.TargetGroup) error
}

type defaultTargetsManager struct {
	log   gwlog.Logger
	cloud pkg_aws.Cloud
}

func NewTargetsManager(
	log gwlog.Logger,
	cloud pkg_aws.Cloud,
) *defaultTargetsManager {
	return &defaultTargetsManager{
		log:   log,
		cloud: cloud,
	}
}

func (s *defaultTargetsManager) List(ctx context.Context, modelTg *model.TargetGroup) ([]types.TargetSummary, error) {
	lattice := s.cloud.Lattice()
	listTargetsInput := vpclattice.ListTargetsInput{
		TargetGroupIdentifier: &modelTg.Status.Id,
	}
	return lattice.ListTargetsAsList(ctx, &listTargetsInput)
}

func (s *defaultTargetsManager) Update(ctx context.Context, modelTargets *model.Targets, modelTg *model.TargetGroup) error {
	if modelTg.Status == nil || modelTg.Status.Id == "" {
		return errors.New("model target group is missing id")
	}
	if modelTargets.Spec.StackTargetGroupId != modelTg.ID() {
		return fmt.Errorf("target group ID %s does not match target reference ID %s",
			modelTg.ID(), modelTargets.Spec.StackTargetGroupId)
	}

	s.log.Debugf(ctx, "Updating targets for target group %s with %d desired targets", modelTg.Status.Id, len(modelTargets.Spec.TargetList))

	latticeTargets, err := s.List(ctx, modelTg)
	if err != nil {
		return err
	}
	s.log.Debugf(ctx, "Found %d existing targets in VPC Lattice for target group %s", len(latticeTargets), modelTg.Status.Id)

	staleTargets := s.findStaleTargets(modelTargets, latticeTargets)
	if len(staleTargets) > 0 {
		s.log.Infof(ctx, "Found %d stale targets to deregister from target group %s", len(staleTargets), modelTg.Status.Id)
		for _, target := range staleTargets {
			s.log.Debugf(ctx, "Stale target: %s:%d", target.TargetIP, target.Port)
		}
	}

	err1 := s.deregisterTargets(ctx, modelTg, staleTargets)

	// Skip RegisterTargets when every desired target is already registered in
	// a non-draining state. RegisterTargets is idempotent server-side, so
	// re-registering already-present targets is a no-op that nonetheless emits
	// a mutation API call (and CloudTrail event) on every reconcile. With
	// drift detection enabled this fires on every periodic pass. A desired
	// target that is currently Draining is treated as "needs registration" so
	// it gets resurrected.
	var err2 error
	if s.needToRegisterTargets(modelTargets, latticeTargets) {
		err2 = s.registerTargets(ctx, modelTg, modelTargets.Spec.TargetList)
	} else {
		s.log.Debugf(ctx, "All %d desired targets already registered for target group %s, skipping RegisterTargets",
			len(modelTargets.Spec.TargetList), modelTg.Status.Id)
	}
	return errors.Join(err1, err2)
}

// needToRegisterTargets reports whether RegisterTargets must be called. It
// returns true if any desired target is not already present in the live set
// in a non-draining state. A desired target that is present but Draining is
// considered to need registration so that it is resurrected.
//
// Note the deliberate asymmetry with findStaleTargets: stale detection
// *excludes* draining targets (Lattice is already removing them, no need to
// deregister again), whereas registration *includes* desired-but-draining
// targets (we want them back).
func (s *defaultTargetsManager) needToRegisterTargets(
	modelTargets *model.Targets,
	listTargetsOutput []types.TargetSummary) bool {

	nonDrainingLive := utils.NewSet[model.Target]()
	for _, target := range listTargetsOutput {
		if string(target.Status) == string(types.TargetStatusDraining) {
			continue
		}
		nonDrainingLive.Put(model.Target{
			TargetIP: aws.ToString(target.Id),
			Port:     int64(aws.ToInt32(target.Port)),
		})
	}

	for _, target := range modelTargets.Spec.TargetList {
		ipPort := model.Target{
			TargetIP: target.TargetIP,
			Port:     target.Port,
		}
		if !nonDrainingLive.Contains(ipPort) {
			return true
		}
	}
	return false
}

func (s *defaultTargetsManager) findStaleTargets(
	modelTargets *model.Targets,
	listTargetsOutput []types.TargetSummary) []model.Target {

	// Disregard readiness information, and use IP/Port as key.
	modelSet := utils.NewSet[model.Target]()
	for _, target := range modelTargets.Spec.TargetList {
		targetIpPort := model.Target{
			TargetIP: target.TargetIP,
			Port:     target.Port,
		}
		modelSet.Put(targetIpPort)
	}

	staleTargets := make([]model.Target, 0)
	for _, target := range listTargetsOutput {
		ipPort := model.Target{
			TargetIP: aws.ToString(target.Id),
			Port:     int64(aws.ToInt32(target.Port)),
		}
		// Consider targets stale if they are not in the current model set and not already draining
		// This ensures that when pods are recreated with new IPs, old IPs are properly deregistered
		if string(target.Status) != string(types.TargetStatusDraining) && !modelSet.Contains(ipPort) {
			staleTargets = append(staleTargets, ipPort)
		}
	}
	return staleTargets
}

func (s *defaultTargetsManager) registerTargets(
	ctx context.Context,
	modelTg *model.TargetGroup,
	targets []model.Target,
) error {
	if len(targets) == 0 {
		return nil
	}
	latticeTargets := utils.SliceMap(targets, func(t model.Target) types.Target {
		p := int32(t.Port)
		return types.Target{Id: &t.TargetIP, Port: &p}
	})
	chunks := utils.Chunks(latticeTargets, maxTargetsPerLatticeTargetsApiCall)
	var registerTargetsError error
	for i, chunk := range chunks {
		registerTargetsInput := vpclattice.RegisterTargetsInput{
			TargetGroupIdentifier: &modelTg.Status.Id,
			Targets:               chunk,
		}
		resp, err := s.cloud.Lattice().RegisterTargets(ctx, &registerTargetsInput)
		if err != nil {
			registerTargetsError = errors.Join(registerTargetsError, fmt.Errorf("failed to register targets from VPC Lattice Target Group %s due to %s", modelTg.Status.Id, err))
			continue
		}
		if len(resp.Unsuccessful) > 0 {
			registerTargetsError = errors.Join(registerTargetsError, fmt.Errorf("failed to register targets from VPC Lattice Target Group %s for chunk %d/%d, unsuccessful targets %v",
				modelTg.Status.Id, i+1, len(chunks), resp.Unsuccessful))
		}
		s.log.Debugf(ctx, "Successfully registered %d targets from VPC Lattice Target Group %s for chunk %d/%d",
			len(resp.Successful), modelTg.Status.Id, i+1, len(chunks))
	}
	return registerTargetsError
}

func (s *defaultTargetsManager) deregisterTargets(
	ctx context.Context,
	modelTg *model.TargetGroup,
	targets []model.Target,
) error {
	if len(targets) == 0 {
		return nil
	}
	latticeTargets := utils.SliceMap(targets, func(t model.Target) types.Target {
		p := int32(t.Port)
		return types.Target{Id: &t.TargetIP, Port: &p}
	})

	chunks := utils.Chunks(latticeTargets, maxTargetsPerLatticeTargetsApiCall)
	var deregisterTargetsError error
	for i, chunk := range chunks {
		deregisterTargetsInput := vpclattice.DeregisterTargetsInput{
			TargetGroupIdentifier: &modelTg.Status.Id,
			Targets:               chunk,
		}
		resp, err := s.cloud.Lattice().DeregisterTargets(ctx, &deregisterTargetsInput)
		if err != nil {
			deregisterTargetsError = errors.Join(deregisterTargetsError, fmt.Errorf("failed to deregister targets from VPC Lattice Target Group %s due to %s", modelTg.Status.Id, err))
			continue
		}
		if len(resp.Unsuccessful) > 0 {
			deregisterTargetsError = errors.Join(deregisterTargetsError, fmt.Errorf("failed to deregister targets from VPC Lattice Target Group %s for chunk %d/%d, unsuccessful targets %v",
				modelTg.Status.Id, i+1, len(chunks), resp.Unsuccessful))
		}
		s.log.Debugf(ctx, "Successfully deregistered %d targets from VPC Lattice Target Group %s for chunk %d/%d", len(resp.Successful), modelTg.Status.Id, i+1, len(chunks))
	}
	return deregisterTargetsError
}
