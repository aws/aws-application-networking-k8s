package lattice

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"

	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

const (
	// Maximum allowed number of targets per each VPC Lattice RegisterTargets/DeregisterTargets API call
	// https://docs.aws.amazon.com/vpc-lattice/latest/APIReference/API_RegisterTargets.html
	maxTargetsPerLatticeTargetsApiCall = 100
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
	latticeTargets := utils.SliceMap(modelTargets.Spec.TargetList, func(t model.Target) *vpclattice.Target {
		return &vpclattice.Target{Id: &t.TargetIP, Port: &t.Port}
	})
	chunks := utils.Chunks(latticeTargets, maxTargetsPerLatticeTargetsApiCall)
	var registerTargetsError error
	for i, targets := range chunks {
		registerRouteInput := vpclattice.RegisterTargetsInput{
			TargetGroupIdentifier: &modelTg.Status.Id,
			Targets:               targets,
		}
		resp, err := s.cloud.Lattice().RegisterTargetsWithContext(ctx, &registerRouteInput)
		if err != nil {
			registerTargetsError = errors.Join(registerTargetsError, fmt.Errorf("Failed to register targets from VPC Lattice Target Group %s due to %s", modelTg.Status.Id, err))
		}
		if len(resp.Unsuccessful) > 0 {
			registerTargetsError = errors.Join(registerTargetsError, fmt.Errorf("Failed to register targets from VPC Lattice Target Group %s for chunk %d/%d, unsuccessful targets %v",
				modelTg.Status.Id, i+1, len(chunks), resp.Unsuccessful))
		}
		s.log.Debugf("Successfully registered %d targets from VPC Lattice Target Group %s for chunk %d/%d",
			len(resp.Successful), modelTg.Status.Id, i+1, len(chunks))
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

	chunks := utils.Chunks(targetsToDeregister, maxTargetsPerLatticeTargetsApiCall)
	var deregisterTargetsError error
	for i, targets := range chunks {
		deregisterTargetsInput := vpclattice.DeregisterTargetsInput{
			TargetGroupIdentifier: &modelTg.Status.Id,
			Targets:               targets,
		}
		resp, err := s.cloud.Lattice().DeregisterTargetsWithContext(ctx, &deregisterTargetsInput)
		if err != nil {
			deregisterTargetsError = errors.Join(deregisterTargetsError, fmt.Errorf("Failed to deregister targets from VPC Lattice Target Group %s due to %s", modelTg.Status.Id, err))
		}
		if len(resp.Unsuccessful) > 0 {
			deregisterTargetsError = errors.Join(deregisterTargetsError, fmt.Errorf("Failed to deregister targets from VPC Lattice Target Group %s for chunk %d/%d, unsuccessful targets %v",
				modelTg.Status.Id, i+1, len(chunks), resp.Unsuccessful))
		}
		s.log.Debugf("Successfully deregistered %d targets from VPC Lattice Target Group %s for chunk %d/%d", resp.Successful, modelTg.Status.Id, i+1, len(chunks))
	}
	return deregisterTargetsError
}
