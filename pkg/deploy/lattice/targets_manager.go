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

	s.deregisterStaleTargets(ctx, modelTargets, modelTg, listTargetsOutput)
	return s.registerTargets(ctx, modelTargets, modelTg)
}

func (s *defaultTargetsManager) registerTargets(
	ctx context.Context,
	modelTargets *model.Targets,
	modelTg *model.TargetGroup,
) error {
	var latticeTargets []*vpclattice.Target
	for _, modelTarget := range modelTargets.Spec.TargetList {
		port := modelTarget.Port
		targetIP := modelTarget.TargetIP
		t := vpclattice.Target{
			Id:   &targetIP,
			Port: &port,
		}
		latticeTargets = append(latticeTargets, &t)
	}

	// No targets to register
	if len(latticeTargets) == 0 {
		return nil
	}

	registerRouteInput := vpclattice.RegisterTargetsInput{
		TargetGroupIdentifier: &modelTg.Status.Id,
		Targets:               latticeTargets,
	}

	resp, err := s.cloud.Lattice().RegisterTargetsWithContext(ctx, &registerRouteInput)
	if err != nil {
		return fmt.Errorf("Failed RegisterTargets %s due to %s", modelTg.Status.Id, err)
	}

	if len(resp.Unsuccessful) > 0 {
		s.log.Infof("Failed RegisterTargets (Unsuccessful=%d) %s, will retry",
			len(resp.Unsuccessful), modelTg.Status.Id)
		return errors.New(LATTICE_RETRY)
	}

	s.log.Infof("Success RegisterTargets %d, %s", len(resp.Successful), modelTg.Status.Id)
	return nil
}

func (s *defaultTargetsManager) deregisterStaleTargets(
	ctx context.Context,
	modelTargets *model.Targets,
	modelTg *model.TargetGroup,
	listTargetsOutput []*vpclattice.TargetSummary,
) {
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

	if len(targetsToDeregister) > 0 {
		deregisterTargetsInput := vpclattice.DeregisterTargetsInput{
			TargetGroupIdentifier: &modelTg.Status.Id,
			Targets:               targetsToDeregister,
		}
		_, err := s.cloud.Lattice().DeregisterTargetsWithContext(ctx, &deregisterTargetsInput)
		if err != nil {
			s.log.Infof("Failed DeregisterTargets %s due to %s", modelTg.Status.Id, err)
		} else {
			s.log.Infof("Success DeregisterTargets %s", modelTg.Status.Id)
		}
	}
}
