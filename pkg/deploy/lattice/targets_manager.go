package lattice

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go/service/vpclattice"

	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	"github.com/aws/aws-sdk-go/aws"
)

const (
	// Maximum allowed number of targets per each VPC Lattice RegisterTargets/DeregisterTargets API call
	// https://docs.aws.amazon.com/vpc-lattice/latest/APIReference/API_RegisterTargets.html
	maxTargetsPerLatticeTargetsApiCall = 100
)

//go:generate mockgen -destination targets_manager_mock.go -package lattice github.com/aws/aws-application-networking-k8s/pkg/deploy/lattice TargetsManager

type TargetsManager interface {
	List(ctx context.Context, modelTg *model.TargetGroup) ([]*vpclattice.TargetSummary, error)
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

func (s *defaultTargetsManager) List(ctx context.Context, modelTg *model.TargetGroup) ([]*vpclattice.TargetSummary, error) {
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

	s.log.Debugf(ctx, "Creating targets for target group %s", modelTg.Status.Id)

	latticeTargets, err := s.List(ctx, modelTg)
	if err != nil {
		return err
	}
	staleTargets := s.findStaleTargets(modelTargets, latticeTargets)

	err1 := s.deregisterTargets(ctx, modelTg, staleTargets)
	err2 := s.registerTargets(ctx, modelTg, modelTargets.Spec.TargetList)
	return errors.Join(err1, err2)
}

func (s *defaultTargetsManager) findStaleTargets(
	modelTargets *model.Targets,
	listTargetsOutput []*vpclattice.TargetSummary) []model.Target {

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
			TargetIP: aws.StringValue(target.Id),
			Port:     aws.Int64Value(target.Port),
		}
		if aws.StringValue(target.Status) != vpclattice.TargetStatusDraining && !modelSet.Contains(ipPort) {
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
	latticeTargets := utils.SliceMap(targets, func(t model.Target) *vpclattice.Target {
		return &vpclattice.Target{Id: &t.TargetIP, Port: &t.Port}
	})
	chunks := utils.Chunks(latticeTargets, maxTargetsPerLatticeTargetsApiCall)
	var registerTargetsError error
	for i, chunk := range chunks {
		registerTargetsInput := vpclattice.RegisterTargetsInput{
			TargetGroupIdentifier: &modelTg.Status.Id,
			Targets:               chunk,
		}
		resp, err := s.cloud.Lattice().RegisterTargetsWithContext(ctx, &registerTargetsInput)
		if err != nil {
			registerTargetsError = errors.Join(registerTargetsError, fmt.Errorf("Failed to register targets from VPC Lattice Target Group %s due to %s", modelTg.Status.Id, err))
		}
		if len(resp.Unsuccessful) > 0 {
			registerTargetsError = errors.Join(registerTargetsError, fmt.Errorf("Failed to register targets from VPC Lattice Target Group %s for chunk %d/%d, unsuccessful targets %v",
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
	latticeTargets := utils.SliceMap(targets, func(t model.Target) *vpclattice.Target {
		return &vpclattice.Target{Id: &t.TargetIP, Port: &t.Port}
	})

	chunks := utils.Chunks(latticeTargets, maxTargetsPerLatticeTargetsApiCall)
	var deregisterTargetsError error
	for i, chunk := range chunks {
		deregisterTargetsInput := vpclattice.DeregisterTargetsInput{
			TargetGroupIdentifier: &modelTg.Status.Id,
			Targets:               chunk,
		}
		resp, err := s.cloud.Lattice().DeregisterTargetsWithContext(ctx, &deregisterTargetsInput)
		if err != nil {
			deregisterTargetsError = errors.Join(deregisterTargetsError, fmt.Errorf("Failed to deregister targets from VPC Lattice Target Group %s due to %s", modelTg.Status.Id, err))
		}
		if len(resp.Unsuccessful) > 0 {
			deregisterTargetsError = errors.Join(deregisterTargetsError, fmt.Errorf("Failed to deregister targets from VPC Lattice Target Group %s for chunk %d/%d, unsuccessful targets %v",
				modelTg.Status.Id, i+1, len(chunks), resp.Unsuccessful))
		}
		s.log.Debugf(ctx, "Successfully deregistered %d targets from VPC Lattice Target Group %s for chunk %d/%d", len(resp.Successful), modelTg.Status.Id, i+1, len(chunks))
	}
	return deregisterTargetsError
}
