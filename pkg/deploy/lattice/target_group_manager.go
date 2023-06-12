package lattice

import (
	"context"
	"errors"
	"github.com/golang/glog"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"

	lattice_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

type TargetGroupManager interface {
	Create(ctx context.Context, targetGroup *latticemodel.TargetGroup) (latticemodel.TargetGroupStatus, error)
	Delete(ctx context.Context, targetGroup *latticemodel.TargetGroup) error
	List(ctx context.Context) ([]targetGroupOutput, error)
	Get(tx context.Context, targetGroup *latticemodel.TargetGroup) (latticemodel.TargetGroupStatus, error)
}

type defaultTargetGroupManager struct {
	cloud lattice_aws.Cloud
}

func NewTargetGroupManager(cloud lattice_aws.Cloud) *defaultTargetGroupManager {
	return &defaultTargetGroupManager{
		cloud: cloud,
	}
}

func getLatticeTGName(targetGroup *latticemodel.TargetGroup) string {
	var tgName string
	if config.UseLongTGName {
		tgName = latticestore.TargetGroupLongName(targetGroup.Spec.Name,
			targetGroup.Spec.Config.K8SHTTPRouteName, config.VpcID)
	} else {
		tgName = targetGroup.Spec.Name
	}

	return tgName
}

// Create will try to create a target group
// return error when:
//
//	ListTargetGroupsAsList() returns error
//	CreateTargetGroupWithContext returns error
//
// return errors.New(LATTICE_RETRY) when:
//
//	CreateTargetGroupWithContext returns
//		TG is TargetGroupStatusUpdateInProgress
//		TG is MeshVpcAssociationStatusFailed
//		TG is TargetGroupStatusCreateInProgress
//		TG is TargetGroupStatusFailed
//
// return nil when:
//
//	TG is TargetGroupStatusActive
func (s *defaultTargetGroupManager) Create(ctx context.Context, targetGroup *latticemodel.TargetGroup) (latticemodel.TargetGroupStatus, error) {

	glog.V(6).Infof("Create Target Group API call for name %s \n", targetGroup.Spec.Name)

	latticeTGName := getLatticeTGName(targetGroup)
	// check if exists
	tgSummary, err := s.findTGByName(ctx, latticeTGName)
	if err != nil {
		return latticemodel.TargetGroupStatus{TargetGroupARN: "", TargetGroupID: ""}, err
	}
	if tgSummary != nil {
		return latticemodel.TargetGroupStatus{TargetGroupARN: aws.StringValue(tgSummary.Arn), TargetGroupID: aws.StringValue(tgSummary.Id)}, err
	}

	glog.V(6).Infof("create targetgroup API here %v\n", targetGroup)
	port := int64(targetGroup.Spec.Config.Port)
	tgConfig := &vpclattice.TargetGroupConfig{
		Port:            &port,
		Protocol:        &targetGroup.Spec.Config.Protocol,
		ProtocolVersion: &targetGroup.Spec.Config.ProtocolVersion,
		VpcIdentifier:   &targetGroup.Spec.Config.VpcID,
	}

	targetGroupType := string(targetGroup.Spec.Type)

	createTargetGroupInput := vpclattice.CreateTargetGroupInput{
		Config: tgConfig,
		Name:   &latticeTGName,
		Type:   &targetGroupType,
		Tags:   make(map[string]*string),
	}
	createTargetGroupInput.Tags[latticemodel.K8SServiceNameKey] = &targetGroup.Spec.Config.K8SServiceName
	createTargetGroupInput.Tags[latticemodel.K8SServiceNamespaceKey] = &targetGroup.Spec.Config.K8SServiceNamespace
	if targetGroup.Spec.Config.IsServiceExport {
		value := latticemodel.K8SServiceExportType
		createTargetGroupInput.Tags[latticemodel.K8SParentRefTypeKey] = &value
	} else {
		value := latticemodel.K8SHTTPRouteType
		createTargetGroupInput.Tags[latticemodel.K8SParentRefTypeKey] = &value
		createTargetGroupInput.Tags[latticemodel.K8SHTTPRouteNameKey] = &targetGroup.Spec.Config.K8SHTTPRouteName
		createTargetGroupInput.Tags[latticemodel.K8SHTTPRouteNamespaceKey] = &targetGroup.Spec.Config.K8SHTTPRouteNamespace
	}

	vpcLatticeSess := s.cloud.Lattice()
	resp, err := vpcLatticeSess.CreateTargetGroupWithContext(ctx, &createTargetGroupInput)
	glog.V(2).Infof("create target group >>>> req [%v], resp[%v] err[%v]\n", createTargetGroupInput, resp, err)

	if err != nil {
		glog.V(6).Infof("fail to create target group %v \n", err)
		return latticemodel.TargetGroupStatus{TargetGroupARN: "", TargetGroupID: ""}, err
	} else {
		tgArn := aws.StringValue(resp.Arn)
		tgId := aws.StringValue(resp.Id)
		tgStatus := aws.StringValue(resp.Status)
		switch tgStatus {
		case vpclattice.TargetGroupStatusCreateInProgress:
			return latticemodel.TargetGroupStatus{TargetGroupARN: "", TargetGroupID: ""}, errors.New(LATTICE_RETRY)
		case vpclattice.TargetGroupStatusActive:
			return latticemodel.TargetGroupStatus{TargetGroupARN: tgArn, TargetGroupID: tgId}, nil
		case vpclattice.TargetGroupStatusCreateFailed:
			return latticemodel.TargetGroupStatus{TargetGroupARN: "", TargetGroupID: ""}, errors.New(LATTICE_RETRY)
		case vpclattice.TargetGroupStatusDeleteFailed:
			return latticemodel.TargetGroupStatus{TargetGroupARN: "", TargetGroupID: ""}, errors.New(LATTICE_RETRY)
		case vpclattice.TargetGroupStatusDeleteInProgress:
			return latticemodel.TargetGroupStatus{TargetGroupARN: "", TargetGroupID: ""}, errors.New(LATTICE_RETRY)
		}
	}
	return latticemodel.TargetGroupStatus{TargetGroupARN: "", TargetGroupID: ""}, nil
}

func (s *defaultTargetGroupManager) Get(ctx context.Context, targetGroup *latticemodel.TargetGroup) (latticemodel.TargetGroupStatus, error) {
	glog.V(6).Infof("Create Lattice Target Group API call for name %s \n", targetGroup.Spec.Name)

	// check if exists
	tgSummary, err := s.findTGByName(ctx, getLatticeTGName(targetGroup))
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
		glog.V(6).Info("TargetGroupManager: Delete API ignored for empty LatticeID\n")
		return nil
	}

	vpcLatticeSess := s.cloud.Lattice()
	// de-register all targets first
	listTargetsInput := vpclattice.ListTargetsInput{
		TargetGroupIdentifier: &targetGroup.Spec.LatticeID,
	}
	glog.V(6).Infof("TG manager list, listReq %v\n", listTargetsInput)
	listResp, err := vpcLatticeSess.ListTargetsAsList(ctx, &listTargetsInput)
	glog.V(6).Infof("TG manager delete,  listResp %v, err: %v \n", listResp, err)
	if err != nil {
		return err
	}

	var targets []*vpclattice.Target
	for _, t := range listResp {
		if t.Status != nil && *t.Status != vpclattice.TargetStatusUnused {
			// When delete a target group, and it has non-notInUsed status target(e.g, healthy, unhealthy, initial status),
			// it means this target group is still in use (referenced by latticeService's listeners and rules).

			//The caller path of defaultTargetGroupManager.Delete() can be following 2 cases only:
			// 1. delete a HttpRoute trigger the delete of target group (httproute_controller -> model_build_targetgroup -> target_group_synthesizer -> target_group_manager code path)
			// 2. delete the serviceExport trigger the delete of target group (serviceexport_controller -> model_build_targetgroup -> target_group_synthesizer -> target_group_manager code path)
			//In both 2 above cases, listeners and rules that related to this tg should be deleted prior to hitting the defaultTargetGroupManager.Delete().
			//(FYI: if the serivceImport of this serviceExport still in used by a httproute, the controller will forbid delete the serviceExport)
			//In that case, if this tg still has non-notInUsed status targets, it means the listenerRules-to-target_group disassociation still need some time to take effect.
			//We could do a LATTICE_RETRY to wait 20 sec to make the target group (targets) status changed to Unused.

			//FYI: delete k8sService request (service_controller -> model_build_targets -> targets_synthesizer -> targets_manager code path) can still deregister targets for a still-in-use target group,
			//which could cause draining status targets. In that case, HttpRoute deletion request need to retry (5min at maximum) until the draining targets to be removed from the target group by vpc lattice backend, then the controller could delete the target group.

			return errors.New(LATTICE_RETRY)
		}
		targets = append(targets, &vpclattice.Target{
			Id:   t.Id,
			Port: t.Port,
		})
	}

	iftargetsRegistered := len(targets) > 0
	if iftargetsRegistered {

		deRegisterInput := vpclattice.DeregisterTargetsInput{
			TargetGroupIdentifier: &targetGroup.Spec.LatticeID,
			Targets:               targets,
		}
		glog.V(6).Infof("TG manager deregister: Input : %v\n", deRegisterInput)
		deRegResp, err := vpcLatticeSess.DeregisterTargetsWithContext(ctx, &deRegisterInput)
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

	deleteTGInput := vpclattice.DeleteTargetGroupInput{
		TargetGroupIdentifier: &targetGroup.Spec.LatticeID,
	}

	delResp, err := vpcLatticeSess.DeleteTargetGroupWithContext(ctx, &deleteTGInput)

	glog.V(2).Infof("TGManager delTGInput >>>> %v delTGResp :%v, err %v \n", deleteTGInput, delResp, err)

	return err
}

type targetGroupOutput struct {
	getTargetGroupOutput vpclattice.GetTargetGroupOutput
	targetGroupTags      *vpclattice.ListTagsForResourceOutput
}

func (s *defaultTargetGroupManager) List(ctx context.Context) ([]targetGroupOutput, error) {
	vpcLatticeSess := s.cloud.Lattice()
	var tgList []targetGroupOutput
	targetGroupListInput := vpclattice.ListTargetGroupsInput{}
	resp, err := vpcLatticeSess.ListTargetGroupsAsList(ctx, &targetGroupListInput)
	glog.V(6).Infof("ManagerList: %v, err: %v \n", resp, err)
	if err != nil {
		return tgList, err
	}

	for _, tg := range resp {
		tgInput := vpclattice.GetTargetGroupInput{
			TargetGroupIdentifier: tg.Id,
		}

		tgOutput, err := vpcLatticeSess.GetTargetGroupWithContext(ctx, &tgInput)
		//glog.V(6).Infof("MangerTG: tgOUtput %v err %v \n", tgOutput, err)
		if err != nil {
			continue
		}

		//glog.V(6).Infof("Manager-List: tg-vpc %v , config.vpc %v\n", aws.StringValue(tgOutput.Config.VpcId), config.VpcID)

		if tgOutput.Config != nil && aws.StringValue(tgOutput.Config.VpcIdentifier) == config.VpcID {
			// retrieve target group tags
			//ListTagsForResourceWithContext
			tagsInput := vpclattice.ListTagsForResourceInput{
				ResourceArn: tg.Arn,
			}

			tagsOutput, err := vpcLatticeSess.ListTagsForResourceWithContext(ctx, &tagsInput)

			glog.V(6).Infof("tagsOutput %v,  err: %v", tagsOutput, err)

			if err != nil {
				// setting it to nil, so the caller knows there is tag resource associated to this target group
				tagsOutput = nil
			}
			tgOutput := targetGroupOutput{
				getTargetGroupOutput: *tgOutput,
				targetGroupTags:      tagsOutput,
			}
			tgList = append(tgList, tgOutput)
		}
	}
	return tgList, err
}

func (s *defaultTargetGroupManager) findTGByName(ctx context.Context, targetGroup string) (*vpclattice.TargetGroupSummary, error) {
	vpcLatticeSess := s.cloud.Lattice()
	targetGroupListInput := vpclattice.ListTargetGroupsInput{}
	resp, err := vpcLatticeSess.ListTargetGroupsAsList(ctx, &targetGroupListInput)

	if err == nil {
		glog.V(6).Infof("findTGByName: resp %v \n", resp)
		for _, r := range resp {
			if aws.StringValue(r.Name) == targetGroup {
				glog.V(6).Info("targetgroup ", targetGroup, " already exists with arn ", *r.Arn, "\n")
				status := aws.StringValue(r.Status)
				switch status {
				case vpclattice.TargetGroupStatusCreateInProgress:
					return nil, errors.New(LATTICE_RETRY)
				case vpclattice.TargetGroupStatusActive:
					return r, nil
				case vpclattice.TargetGroupStatusCreateFailed:
					return nil, nil
				case vpclattice.TargetGroupStatusDeleteFailed:
					return r, nil
				case vpclattice.TargetGroupStatusDeleteInProgress:
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
