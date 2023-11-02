package lattice

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"

	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

type TargetsManager interface {
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

func (s *defaultTargetsManager) Update(ctx context.Context, modelTargets *model.Targets, modelTg *model.TargetGroup) error {
	if modelTg.Status == nil || modelTg.Status.Id == "" {
		return errors.New("model target group is missing id")
	}
	if modelTargets.Spec.StackTargetGroupId != modelTg.ID() {
		return fmt.Errorf("target group ID %s does not match target reference ID %s",
			modelTg.ID(), modelTargets.Spec.StackTargetGroupId)
	}

	s.log.Debugf("Creating targets for target group %s", modelTg.Status.Id)

	lattice := s.cloud.Lattice()
	listTargetsInput := vpclattice.ListTargetsInput{
		TargetGroupIdentifier: &modelTg.Status.Id,
	}
	listTargetsOutput, err := lattice.ListTargetsAsList(ctx, &listTargetsInput)
	if err != nil {
		return err
	}

	err1 := s.deregisterStaleTargets(ctx, modelTargets, modelTg, listTargetsOutput)
	err2 := s.registerTargets(ctx, modelTargets, modelTg)
	return errors.Join(err1, err2)
}

func (s *defaultTargetsManager) registerTargets(
	ctx context.Context,
	modelTargets *model.Targets,
	modelTg *model.TargetGroup,
) error {
	// No targets to register
	if len(modelTargets.Spec.TargetList) == 0 {
		return nil
	}
	latticeTargets := modelTargetsToLatticeTargets(modelTargets.Spec.TargetList)
	// Partition the targets into groups of 100, because that is the allowed max number of targets per RegisterTargets API call
	// https://docs.aws.amazon.com/vpc-lattice/latest/APIReference/API_RegisterTargets.html
	partitionedLatticeTargets := getPartitionedLatticeTargets(latticeTargets, 100)
	var registerTargetsError error
	for i, targets := range partitionedLatticeTargets {
		registerRouteInput := vpclattice.RegisterTargetsInput{
			TargetGroupIdentifier: &modelTg.Status.Id,
			Targets:               targets,
		}
		resp, err := s.cloud.Lattice().RegisterTargetsWithContext(ctx, &registerRouteInput)
		if err != nil {
			registerTargetsError = errors.Join(registerTargetsError, fmt.Errorf("failed RegisterTargets %s due to %s", modelTg.Status.Id, err))
		}
		if len(resp.Unsuccessful) > 0 {
			registerTargetsError = errors.Join(registerTargetsError, fmt.Errorf("failed RegisterTargets (Unsuccessful=%d) %s, will retry",
				len(resp.Unsuccessful), modelTg.Status.Id))

		}
		s.log.Infof("Success Register %d Targets for partition(%d/%d) for target group: %s",
			len(resp.Successful), i+1, len(partitionedLatticeTargets), modelTg.Status.Id)
	}
	return registerTargetsError
}

func (s *defaultTargetsManager) deregisterStaleTargets(
	ctx context.Context,
	modelTargets *model.Targets,
	modelTg *model.TargetGroup,
	listTargetsOutput []*vpclattice.TargetSummary,
) error {
	var targetsToDeregister []*vpclattice.Target
	for _, latticeTarget := range listTargetsOutput {
		isStale := true
		for _, t := range modelTargets.Spec.TargetList {
			if (aws.StringValue(latticeTarget.Id) == t.TargetIP) && (aws.Int64Value(latticeTarget.Port) == t.Port) {
				isStale = false
				break
			}
		}
		if isStale {
			targetsToDeregister = append(targetsToDeregister, &vpclattice.Target{Id: latticeTarget.Id, Port: latticeTarget.Port})
		}
	}
	if len(targetsToDeregister) == 0 {
		return nil
	}
	partitionedTargetsToDeregister := getPartitionedLatticeTargets(targetsToDeregister, 100)
	var deregisterTargetsError error
	for i, targets := range partitionedTargetsToDeregister {
		deregisterTargetsInput := vpclattice.DeregisterTargetsInput{
			TargetGroupIdentifier: &modelTg.Status.Id,
			Targets:               targets,
		}
		resp, err := s.cloud.Lattice().DeregisterTargetsWithContext(ctx, &deregisterTargetsInput)
		if err != nil {
			deregisterTargetsError = errors.Join(deregisterTargetsError, fmt.Errorf("failed DeregisterTargets %s due to %s", modelTg.Status.Id, err))
		}
		if len(resp.Unsuccessful) > 0 {
			deregisterTargetsError = errors.Join(deregisterTargetsError, fmt.Errorf("failed DeregisterTargets (Unsuccessful=%d) for target group %s",
				len(resp.Unsuccessful), modelTg.Status.Id))
		}
		s.log.Infof("Success DeregisterTargets for partition (%d/%d) for target group %s", i, len(partitionedTargetsToDeregister), modelTg.Status.Id)
	}
	return deregisterTargetsError
}

func getPartitionedLatticeTargets(targets []*vpclattice.Target, size int) [][]*vpclattice.Target {
	var partitions [][]*vpclattice.Target
	for len(targets) > 0 {
		// Get the next partition's size
		end := size
		if end > len(targets) {
			end = len(targets)
		}
		// Append the partition to the list of partitions
		partitions = append(partitions, targets[:end])

		// Move the start of the slice forward
		targets = targets[end:]
	}
	return partitions

}

func modelTargetsToLatticeTargets(modelTargets []model.Target) []*vpclattice.Target {
	var targets []*vpclattice.Target
	for _, modelTarget := range modelTargets {
		port := modelTarget.Port
		targetIP := modelTarget.TargetIP
		t := vpclattice.Target{
			Id:   &targetIP,
			Port: &port,
		}
		targets = append(targets, &t)
	}
	return targets
}
