package lattice

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"

	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

type TargetsManager interface {
	Create(ctx context.Context, targets *model.Targets) error
}

type defaultTargetsManager struct {
	log       gwlog.Logger
	cloud     pkg_aws.Cloud
	datastore *latticestore.LatticeDataStore
}

func NewTargetsManager(
	log gwlog.Logger,
	cloud pkg_aws.Cloud,
	datastore *latticestore.LatticeDataStore,
) *defaultTargetsManager {
	return &defaultTargetsManager{
		log:       log,
		cloud:     cloud,
		datastore: datastore,
	}
}

// Create will try to register targets to the target group
// return Retry when:
//
//		Target group does not exist
//		nonempty unsuccessfully registered targets list
//	otherwise:
//	nil
func (s *defaultTargetsManager) Create(ctx context.Context, targets *model.Targets) error {
	s.log.Debugf("Creating targets for target group %s-%s", targets.Spec.Name, targets.Spec.Namespace)

	// Need to find TargetGroup ID from datastore
	tgName := latticestore.TargetGroupName(targets.Spec.Name, targets.Spec.Namespace)
	tg, err := s.datastore.GetTargetGroup(tgName, targets.Spec.RouteName, false) // isServiceImport=false
	if err != nil {
		s.log.Debugf("Failed to Create targets, service %s-%s was not found, will retry later",
			targets.Spec.Name, targets.Spec.Namespace)
		return errors.New(LATTICE_RETRY)
	}

	vpcLatticeSess := s.cloud.Lattice()
	listTargetsInput := vpclattice.ListTargetsInput{
		TargetGroupIdentifier: &tg.ID,
	}
	var delTargetsList []*vpclattice.Target
	listTargetsOutput, err := vpcLatticeSess.ListTargetsAsList(ctx, &listTargetsInput)
	if err != nil {
		return err
	}

	for _, sdkT := range listTargetsOutput {
		// check if sdkT is in input target list
		isStale := true
		for _, t := range targets.Spec.TargetIPList {
			if (aws.StringValue(sdkT.Id) == t.TargetIP) && (aws.Int64Value(sdkT.Port) == t.Port) {
				isStale = false
				break
			}
		}
		if isStale {
			delTargetsList = append(delTargetsList, &vpclattice.Target{Id: sdkT.Id, Port: sdkT.Port})
		}
	}

	if len(delTargetsList) > 0 {
		deRegisterTargetsInput := vpclattice.DeregisterTargetsInput{
			TargetGroupIdentifier: &tg.ID,
			Targets:               delTargetsList,
		}
		_, err := vpcLatticeSess.DeregisterTargetsWithContext(ctx, &deRegisterTargetsInput)
		if err != nil {
			s.log.Errorf("Deregistering targets for target group %s failed due to %s", tg.ID, err)
		}
	}

	// TODO following should be done at model level
	var targetList []*vpclattice.Target
	for _, target := range targets.Spec.TargetIPList {
		port := target.Port
		targetIP := target.TargetIP
		t := vpclattice.Target{
			Id:   &targetIP,
			Port: &port,
		}
		targetList = append(targetList, &t)
	}

	// No targets to register
	if len(targetList) == 0 {
		return nil
	}

	registerRouteInput := vpclattice.RegisterTargetsInput{
		TargetGroupIdentifier: &tg.ID,
		Targets:               targetList,
	}

	resp, err := vpcLatticeSess.RegisterTargetsWithContext(ctx, &registerRouteInput)
	if err != nil {
		return err
	}

	isTargetRegisteredUnsuccessful := len(resp.Unsuccessful) > 0
	if isTargetRegisteredUnsuccessful {
		s.log.Debugf("Failed to register targets for target group %s, will retry later", tg.ID)
		return errors.New(LATTICE_RETRY)
	}

	s.log.Debugf("Successfully registered targets for target group %s", tg.ID)
	return nil
}
