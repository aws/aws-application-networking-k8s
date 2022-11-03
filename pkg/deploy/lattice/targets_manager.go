package lattice

import (
	"context"
	"errors"
	"github.com/golang/glog"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/mercury"

	mercury_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

type TargetsManager interface {
	Create(ctx context.Context, targets *latticemodel.Targets) error
}

type defaultTargetsManager struct {
	cloud     mercury_aws.Cloud
	datastore *latticestore.LatticeDataStore
}

func NewTargetsManager(cloud mercury_aws.Cloud, datastore *latticestore.LatticeDataStore) *defaultTargetsManager {
	return &defaultTargetsManager{
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
func (s *defaultTargetsManager) Create(ctx context.Context, targets *latticemodel.Targets) error {
	glog.V(6).Infof("Update Mercury targets API call for %v \n", targets)

	// Need to find TargetGroup ID from datastore
	tgName := latticestore.TargetGroupName(targets.Spec.Name, targets.Spec.Namespace)
	tg, err := s.datastore.GetTargetGroup(tgName, false) // isServiceImport=false

	if err != nil {
		glog.V(6).Infof("Failed to Create targets, service ( name %v namespace %v) not found, retry later\n", targets.Spec.Name, targets.Spec.Namespace)
		return errors.New(LATTICE_RETRY)
	}
	mercurySess := s.cloud.Mercury()
	// find out sdk target list
	listTargetsInput := mercury.ListTargetsInput{
		TargetGroupIdentifier: &tg.ID,
	}

	var delTargetsList []*mercury.Target
	listTargetsOutput, err := mercurySess.ListTargetsAsList(ctx, &listTargetsInput)
	glog.V(6).Infof("TargetsManager-Create, listTargetsOutput %v, err %v \n", listTargetsOutput, err)
	if err != nil {
		glog.V(6).Infof("Failed to create target, tgName %v tg %v\n", tgName, tg)
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
			delTargetsList = append(delTargetsList, &mercury.Target{Id: sdkT.Id, Port: sdkT.Port})
		}
	}

	if len(delTargetsList) > 0 {
		deRegisterTargetsInput := mercury.DeregisterTargetsInput{
			TargetGroupIdentifier: &tg.ID,
			Targets:               delTargetsList,
		}
		deRegisterTargetsOutput, err := mercurySess.DeregisterTargetsWithContext(ctx, &deRegisterTargetsInput)
		glog.V(6).Infof("TargetManager-Create, deregister deleted targets input %v, output %v, err %v\n", deRegisterTargetsInput, deRegisterTargetsOutput, err)
	}
	// TODO following should be done at model level
	var targetList []*mercury.Target
	for _, target := range targets.Spec.TargetIPList {
		port := target.Port
		targetIP := target.TargetIP
		t := mercury.Target{
			Id:   &targetIP,
			Port: &port,
		}
		targetList = append(targetList, &t)
	}

	registerRouteInput := mercury.RegisterTargetsInput{
		TargetGroupIdentifier: &tg.ID,
		Targets:               targetList,
	}
	glog.V(6).Infof("Calling Mercury API register targets input %v \n", registerRouteInput)

	resp, err := mercurySess.RegisterTargetsWithContext(ctx, &registerRouteInput)
	glog.V(6).Infof("register pod to target group resp[%v]\n", resp)
	glog.V(6).Infof("register pod to target group err[%v]\n", err)
	if err != nil {
		glog.V(6).Infof("Fail to register target err[%v]\n", err)
		return err
	}

	isTargetRegisteredUnsuccessful := len(resp.Unsuccessful) > 0
	if isTargetRegisteredUnsuccessful {
		glog.V(6).Infof("Targets register unsuccessfully, will retry later\n")
		return errors.New(LATTICE_RETRY)
	}
	glog.V(6).Infof("Targets register successfully\n")
	return nil
}
