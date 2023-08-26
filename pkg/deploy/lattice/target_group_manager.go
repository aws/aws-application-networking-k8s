package lattice

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/golang/glog"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"

	"fmt"
	lattice_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"strings"
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

// Determines the "actual" target group name used in VPC Lattice.
func getLatticeTGName(targetGroup *latticemodel.TargetGroup) string {
	var (
		namePrefix      = targetGroup.Spec.Name
		protocol        = strings.ToLower(targetGroup.Spec.Config.Protocol)
		protocolVersion = strings.ToLower(targetGroup.Spec.Config.ProtocolVersion)
	)
	if config.UseLongTGName {
		namePrefix = latticestore.TargetGroupLongName(namePrefix,
			targetGroup.Spec.Config.K8SHTTPRouteName, config.VpcID)
	}
	return fmt.Sprintf("%s-%s-%s", namePrefix, protocol, protocolVersion)
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
	tgSummary, err := s.findTargetGroup(ctx, targetGroup)
	if err != nil {
		return latticemodel.TargetGroupStatus{TargetGroupARN: "", TargetGroupID: ""}, err
	}

	vpcLatticeSess := s.cloud.Lattice()
	if tgSummary != nil {
		if targetGroup.Spec.Config.HealthCheckConfig != nil {
			_, err := vpcLatticeSess.UpdateTargetGroupWithContext(ctx, &vpclattice.UpdateTargetGroupInput{
				HealthCheck:           targetGroup.Spec.Config.HealthCheckConfig,
				TargetGroupIdentifier: tgSummary.Id,
			})
			if err != nil {
				return latticemodel.TargetGroupStatus{TargetGroupARN: "", TargetGroupID: ""}, err
			}
		}
		return latticemodel.TargetGroupStatus{TargetGroupARN: aws.StringValue(tgSummary.Arn), TargetGroupID: aws.StringValue(tgSummary.Id)}, nil
	}

	glog.V(6).Infof("create targetgroup API here %v\n", targetGroup)
	port := int64(targetGroup.Spec.Config.Port)

	ipAddressType := &targetGroup.Spec.Config.IpAddressType

	// if IpAddressTypeIpv4 is not set, then default to nil
	if targetGroup.Spec.Config.IpAddressType == "" {
		ipAddressType = nil
	}

	tgConfig := &vpclattice.TargetGroupConfig{
		Port:            &port,
		Protocol:        &targetGroup.Spec.Config.Protocol,
		ProtocolVersion: &targetGroup.Spec.Config.ProtocolVersion,
		VpcIdentifier:   &targetGroup.Spec.Config.VpcID,
		IpAddressType:   ipAddressType,
		HealthCheck:     targetGroup.Spec.Config.HealthCheckConfig,
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
	tgSummary, err := s.findTargetGroup(ctx, targetGroup)
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
		if aerr, ok := err.(awserr.Error); ok {
			if aerr.Code() == vpclattice.ErrCodeResourceNotFoundException {
				// already deleted in lattice, this is OK
				glog.V(6).Infof("Target group already deleted %v \n", targetGroup.Spec.LatticeID)
				err = nil
			}
		}

		if err != nil {
			return err
		}
	} else {
		// deregister targets
		var targets []*vpclattice.Target
		for _, t := range listResp {
			if t.Status != nil && *t.Status != vpclattice.TargetStatusUnused {
				glog.V(6).Infof("TargetGroupManager: The targetGroup [%v] has non-Unused status target(s), which means this targetGroup is still in use by latticeService, it cannot be deleted now\n", targetGroup.Spec.LatticeID)
				// Before call the defaultTargetGroupManager.Delete(), we always call the latticeServiceManager.Delete() first,
				//  *t.Status != vpclattice.TargetStatusUnused means previous delete latticeService still in the progress, we could wait for 20 seconds and then retry
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
	}

	deleteTGInput := vpclattice.DeleteTargetGroupInput{
		TargetGroupIdentifier: &targetGroup.Spec.LatticeID,
	}
	delResp, err := vpcLatticeSess.DeleteTargetGroupWithContext(ctx, &deleteTGInput)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case vpclattice.ErrCodeResourceNotFoundException:
				glog.V(6).Infof("Target group already deleted %v \n", targetGroup.Spec.LatticeID)
				err = nil
				break
			default:
				glog.V(6).Infof("vpcLatticeSess.DeleteTargetGroupWithContext() return error %v \n", err)
			}
		}
	}
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

func isNameOfTargetGroup(targetGroup *latticemodel.TargetGroup, name string) bool {
	// We are missing protocol info for ServiceImport, but we do know if it is GRPCRoute or not.
	// We have two choices (GRPC/non-GRPC) anyway, so just do prefix matching and pick GRPC when it is.
	if targetGroup.Spec.Config.IsServiceImport {
		match := strings.HasPrefix(name, targetGroup.Spec.Name)
		if targetGroup.Spec.Config.ProtocolVersion == vpclattice.TargetGroupProtocolVersionGrpc {
			return match && strings.HasSuffix(name, vpclattice.TargetGroupProtocolVersionGrpc)
		}
		return match
	} else {
		tgName := getLatticeTGName(targetGroup)
		return name == tgName
	}
}

func (s *defaultTargetGroupManager) findTargetGroup(ctx context.Context, targetGroup *latticemodel.TargetGroup) (*vpclattice.TargetGroupSummary, error) {

	vpcLatticeSess := s.cloud.Lattice()
	targetGroupListInput := vpclattice.ListTargetGroupsInput{}
	resp, err := vpcLatticeSess.ListTargetGroupsAsList(ctx, &targetGroupListInput)

	if err == nil {
		glog.V(6).Infof("findTargetGroup: resp %v \n", resp)
		for _, r := range resp {
			if isNameOfTargetGroup(targetGroup, *r.Name) {
				glog.V(6).Info("targetgroup ", *r.Name, " already exists with arn ", *r.Arn, "\n")
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
		glog.V(6).Infof("findTargetGroup, listTargetGroupsAsList failed err %v\n", err)
		return nil, err
	}
	return nil, nil
}
