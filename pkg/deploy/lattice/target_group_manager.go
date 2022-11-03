package lattice

import (
	"context"
	"errors"
	"github.com/golang/glog"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/mercury"

	mercury_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

type TargetGroupManager interface {
	Create(ctx context.Context, targetGroup *latticemodel.TargetGroup) (latticemodel.TargetGroupStatus, error)
	Delete(ctx context.Context, targetGroup *latticemodel.TargetGroup) error
	List(ctx context.Context) ([]mercury.GetTargetGroupOutput, error)
	Get(tx context.Context, targetGroup *latticemodel.TargetGroup) (latticemodel.TargetGroupStatus, error)
}

type defaultTargetGroupManager struct {
	cloud mercury_aws.Cloud
}

func NewTargetGroupManager(cloud mercury_aws.Cloud) *defaultTargetGroupManager {
	return &defaultTargetGroupManager{
		cloud: cloud,
	}
}

// Create will try to create a target group
// return error when:
//		ListTargetGroupsAsList() returns error
//		CreateTargetGroupWithContext returns error
// return errors.New(LATTICE_RETRY) when:
// 		CreateTargetGroupWithContext returns
//			TG is TargetGroupStatusUpdateInProgress
//			TG is MeshVpcAssociationStatusFailed
//			TG is TargetGroupStatusCreateInProgress
//			TG is TargetGroupStatusFailed
// return nil when:
// 		TG is TargetGroupStatusActive
//
func (s *defaultTargetGroupManager) Create(ctx context.Context, targetGroup *latticemodel.TargetGroup) (latticemodel.TargetGroupStatus, error) {

	glog.V(6).Infof("Create Target Group API call for name %s \n", targetGroup.Spec.Name)
	// check if exists
	tgSummary, err := s.findTGByName(ctx, targetGroup.Spec.Name)
	if err != nil {
		return latticemodel.TargetGroupStatus{TargetGroupARN: "", TargetGroupID: ""}, err
	}
	if tgSummary != nil {
		return latticemodel.TargetGroupStatus{TargetGroupARN: aws.StringValue(tgSummary.Arn), TargetGroupID: aws.StringValue(tgSummary.Id)}, err
	}

	glog.V(6).Infof("create targetgropu API here %v\n", targetGroup)
	port := int64(targetGroup.Spec.Config.Port)
	config := &mercury.TargetGroupConfig{
		Port:            &port,
		Protocol:        &targetGroup.Spec.Config.Protocol,
		ProtocolVersion: &targetGroup.Spec.Config.ProtocolVersion,
		VpcIdentifier:   &targetGroup.Spec.Config.VpcID,
	}

	targetGroupType := string(targetGroup.Spec.Type)
	createTargetGroupInput := mercury.CreateTargetGroupInput{
		Config: config,
		Name:   &targetGroup.Spec.Name,
		Type:   &targetGroupType,
	}
	mercurySess := s.cloud.Mercury()
	resp, err := mercurySess.CreateTargetGroupWithContext(ctx, &createTargetGroupInput)
	glog.V(2).Infof("create target group >>>> req [%v], resp[%v] err[%v]\n", createTargetGroupInput, resp, err)

	if err != nil {
		glog.V(6).Infof("fail to create target group %v \n", err)
		return latticemodel.TargetGroupStatus{TargetGroupARN: "", TargetGroupID: ""}, err
	} else {
		tgArn := aws.StringValue(resp.Arn)
		tgId := aws.StringValue(resp.Id)
		tgStatus := aws.StringValue(resp.Status)
		switch tgStatus {
		case mercury.TargetGroupStatusCreateInProgress:
			return latticemodel.TargetGroupStatus{TargetGroupARN: "", TargetGroupID: ""}, errors.New(LATTICE_RETRY)
		case mercury.TargetGroupStatusActive:
			return latticemodel.TargetGroupStatus{TargetGroupARN: tgArn, TargetGroupID: tgId}, nil
		case mercury.TargetGroupStatusCreateFailed:
			return latticemodel.TargetGroupStatus{TargetGroupARN: "", TargetGroupID: ""}, errors.New(LATTICE_RETRY)
		case mercury.TargetGroupStatusDeleteFailed:
			return latticemodel.TargetGroupStatus{TargetGroupARN: "", TargetGroupID: ""}, errors.New(LATTICE_RETRY)
		case mercury.TargetGroupStatusDeleteInProgress:
			return latticemodel.TargetGroupStatus{TargetGroupARN: "", TargetGroupID: ""}, errors.New(LATTICE_RETRY)
		}
	}
	return latticemodel.TargetGroupStatus{TargetGroupARN: "", TargetGroupID: ""}, nil
}

func (s *defaultTargetGroupManager) Get(ctx context.Context, targetGroup *latticemodel.TargetGroup) (latticemodel.TargetGroupStatus, error) {
	glog.V(6).Infof("Create Mercury Target Group API call for name %s \n", targetGroup.Spec.Name)
	// check if exists
	tgSummary, err := s.findTGByName(ctx, targetGroup.Spec.Name)
	if err != nil {
		return latticemodel.TargetGroupStatus{TargetGroupARN: "", TargetGroupID: ""}, err
	}
	if tgSummary != nil {
		return latticemodel.TargetGroupStatus{TargetGroupARN: aws.StringValue(tgSummary.Arn), TargetGroupID: aws.StringValue(tgSummary.Id)}, err
	}

	return latticemodel.TargetGroupStatus{TargetGroupARN: "", TargetGroupID: ""}, errors.New("Non existing Target Group")
}

func (s *defaultTargetGroupManager) Delete(ctx context.Context, targetGroup *latticemodel.TargetGroup) error {
	glog.V(6).Infof("Manager: Deleting target group %v \n", targetGroup)

	if targetGroup.Spec.LatticeID == "" {
		glog.V(6).Info("TargetGroupManager: Delete API ignored for empty MercuryID\n")
		return nil
	}

	mercurySess := s.cloud.Mercury()
	// de-register all targets first
	listTargetsInput := mercury.ListTargetsInput{
		TargetGroupIdentifier: &targetGroup.Spec.LatticeID,
	}
	glog.V(6).Infof("TG manager list, listReq %v\n", listTargetsInput)
	listResp, err := mercurySess.ListTargetsAsList(ctx, &listTargetsInput)
	glog.V(6).Infof("manager delete,  listResp %v, err: %v \n", listResp, err)
	if err != nil {
		return err
	}

	var targets []*mercury.Target
	for _, t := range listResp {
		targets = append(targets, &mercury.Target{
			Id:   t.Id,
			Port: t.Port,
		})
	}

	iftargetsRegistered := len(targets) > 0
	if iftargetsRegistered {

		deRegisterInput := mercury.DeregisterTargetsInput{
			TargetGroupIdentifier: &targetGroup.Spec.LatticeID,
			Targets:               targets,
		}
		glog.V(6).Infof("TG manager deregister: Input : %v\n", deRegisterInput)
		deRegResp, err := mercurySess.DeregisterTargetsWithContext(ctx, &deRegisterInput)
		glog.V(6).Infof("manager deregister resp %v err %v \n", deRegResp, err)
		if err != nil {
			return err
		}

		isDeRegRespUnsuccessful := len(deRegResp.Unsuccessful) > 0
		if isDeRegRespUnsuccessful {
			glog.V(6).Infof("Targets deregister unsuccessfully, will retry later \n")
			return errors.New(LATTICE_RETRY)
		}

	}

	deleteTGInput := mercury.DeleteTargetGroupInput{
		TargetGroupIdentifier: &targetGroup.Spec.LatticeID,
	}

	delResp, err := mercurySess.DeleteTargetGroupWithContext(ctx, &deleteTGInput)

	glog.V(2).Infof("TGManager delTGInput >>>> %v delTGResp :%v, err %v \n", deleteTGInput, delResp, err)

	return err
}

func (s *defaultTargetGroupManager) List(ctx context.Context) ([]mercury.GetTargetGroupOutput, error) {
	mercurySess := s.cloud.Mercury()
	var tgList []mercury.GetTargetGroupOutput
	targetGroupListInput := mercury.ListTargetGroupsInput{}
	resp, err := mercurySess.ListTargetGroupsAsList(ctx, &targetGroupListInput)
	glog.V(6).Infof("ManagerList: %v, err: %v \n", resp, err)
	if err != nil {
		return tgList, err
	}

	for _, tg := range resp {
		tgInput := mercury.GetTargetGroupInput{
			TargetGroupIdentifier: tg.Id,
		}

		tgOutput, err := mercurySess.GetTargetGroupWithContext(ctx, &tgInput)
		//glog.V(6).Infof("MangerTG: tgOUtput %v err %v \n", tgOutput, err)
		if err != nil {
			continue
		}

		//glog.V(6).Infof("Manager-List: tg-vpc %v , config.vpc %v\n", aws.StringValue(tgOutput.Config.VpcId), config.VpcID)

		if tgOutput.Config != nil && aws.StringValue(tgOutput.Config.VpcIdentifier) == config.VpcID {
			tgList = append(tgList, *tgOutput)
		}
	}
	return tgList, err
}

func (s *defaultTargetGroupManager) findTGByName(ctx context.Context, targetGroup string) (*mercury.TargetGroupSummary, error) {
	mercurySess := s.cloud.Mercury()
	targetGroupListInput := mercury.ListTargetGroupsInput{}
	resp, err := mercurySess.ListTargetGroupsAsList(ctx, &targetGroupListInput)

	if err == nil {
		glog.V(6).Infof("findTGByName: resp %v \n", resp)
		for _, r := range resp {
			if aws.StringValue(r.Name) == targetGroup {
				glog.V(6).Info("targetgroup ", targetGroup, " already exists with arn ", *r.Arn, "\n")
				status := aws.StringValue(r.Status)
				switch status {
				case mercury.TargetGroupStatusCreateInProgress:
					return nil, errors.New(LATTICE_RETRY)
				case mercury.TargetGroupStatusActive:
					return r, nil
				case mercury.TargetGroupStatusCreateFailed:
					return nil, nil
				case mercury.TargetGroupStatusDeleteFailed:
					return r, nil
				case mercury.TargetGroupStatusDeleteInProgress:
					return nil, errors.New(LATTICE_RETRY)
				}
			}
		}
	} else {
		glog.V(6).Infof("findTGByName, listTargetGroupsAsList failed err %v\n", err)
		return nil, err
	}
	return nil, nil
}
