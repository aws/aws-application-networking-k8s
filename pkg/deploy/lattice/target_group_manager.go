package lattice

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/vpclattice"

	"fmt"
	"strings"

	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

type TargetGroupManager interface {
	Create(ctx context.Context, targetGroup *model.TargetGroup) (model.TargetGroupStatus, error)
	Delete(ctx context.Context, targetGroup *model.TargetGroup) error
	List(ctx context.Context) ([]targetGroupOutput, error)
	Get(tx context.Context, targetGroup *model.TargetGroup) (model.TargetGroupStatus, error)
}

type defaultTargetGroupManager struct {
	log   gwlog.Logger
	cloud pkg_aws.Cloud
}

func NewTargetGroupManager(log gwlog.Logger, cloud pkg_aws.Cloud) *defaultTargetGroupManager {
	return &defaultTargetGroupManager{
		log:   log,
		cloud: cloud,
	}
}

// Determines the "actual" target group name used in VPC Lattice.
func getLatticeTGName(targetGroup *model.TargetGroup) string {
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
func (s *defaultTargetGroupManager) Create(
	ctx context.Context,
	targetGroup *model.TargetGroup,
) (model.TargetGroupStatus, error) {
	s.log.Debugf("Creating VPC Lattice Target Group %s", targetGroup.Spec.Name)

	latticeTGName := getLatticeTGName(targetGroup)
	// check if exists
	tgSummary, err := s.findTargetGroup(ctx, targetGroup)

	if err != nil {
		return model.TargetGroupStatus{TargetGroupARN: "", TargetGroupID: ""}, err
	}

	vpcLatticeSess := s.cloud.Lattice()

	// this means Target Group already existed, so this is an update request
	if tgSummary != nil {
		return s.update(ctx, targetGroup, tgSummary)
	}

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
	createTargetGroupInput.Tags[model.K8SServiceNameKey] = &targetGroup.Spec.Config.K8SServiceName
	createTargetGroupInput.Tags[model.K8SServiceNamespaceKey] = &targetGroup.Spec.Config.K8SServiceNamespace
	if targetGroup.Spec.Config.IsServiceExport {
		value := model.K8SServiceExportType
		createTargetGroupInput.Tags[model.K8SParentRefTypeKey] = &value
	} else {
		value := model.K8SHTTPRouteType
		createTargetGroupInput.Tags[model.K8SParentRefTypeKey] = &value
		createTargetGroupInput.Tags[model.K8SHTTPRouteNameKey] = &targetGroup.Spec.Config.K8SHTTPRouteName
		createTargetGroupInput.Tags[model.K8SHTTPRouteNamespaceKey] = &targetGroup.Spec.Config.K8SHTTPRouteNamespace
	}

	resp, err := vpcLatticeSess.CreateTargetGroupWithContext(ctx, &createTargetGroupInput)
	if err != nil {
		return model.TargetGroupStatus{TargetGroupARN: "", TargetGroupID: ""}, err
	} else {
		tgArn := aws.StringValue(resp.Arn)
		tgId := aws.StringValue(resp.Id)
		tgStatus := aws.StringValue(resp.Status)
		switch tgStatus {
		case vpclattice.TargetGroupStatusCreateInProgress:
			return model.TargetGroupStatus{TargetGroupARN: "", TargetGroupID: ""}, errors.New(LATTICE_RETRY)
		case vpclattice.TargetGroupStatusActive:
			return model.TargetGroupStatus{TargetGroupARN: tgArn, TargetGroupID: tgId}, nil
		case vpclattice.TargetGroupStatusCreateFailed:
			return model.TargetGroupStatus{TargetGroupARN: "", TargetGroupID: ""}, errors.New(LATTICE_RETRY)
		case vpclattice.TargetGroupStatusDeleteFailed:
			return model.TargetGroupStatus{TargetGroupARN: "", TargetGroupID: ""}, errors.New(LATTICE_RETRY)
		case vpclattice.TargetGroupStatusDeleteInProgress:
			return model.TargetGroupStatus{TargetGroupARN: "", TargetGroupID: ""}, errors.New(LATTICE_RETRY)
		}
	}
	return model.TargetGroupStatus{TargetGroupARN: "", TargetGroupID: ""}, nil
}

func (s *defaultTargetGroupManager) Get(ctx context.Context, targetGroup *model.TargetGroup) (model.TargetGroupStatus, error) {
	s.log.Debugf("Getting VPC Lattice Target Group %s", targetGroup.Spec.Name)

	// check if exists
	tgSummary, err := s.findTargetGroup(ctx, targetGroup)
	if err != nil {
		return model.TargetGroupStatus{TargetGroupARN: "", TargetGroupID: ""}, err
	}
	if tgSummary != nil {
		return model.TargetGroupStatus{TargetGroupARN: aws.StringValue(tgSummary.Arn), TargetGroupID: aws.StringValue(tgSummary.Id)}, err
	}

	return model.TargetGroupStatus{TargetGroupARN: "", TargetGroupID: ""}, errors.New("Non existing Target Group")
}

func (s *defaultTargetGroupManager) update(ctx context.Context, targetGroup *model.TargetGroup, tgSummary *vpclattice.TargetGroupSummary) (model.TargetGroupStatus, error) {
	s.log.Debugf("Updating VPC Lattice Target Group %s", targetGroup.Spec.Name)

	vpcLatticeSess := s.cloud.Lattice()
	healthCheckConfig := targetGroup.Spec.Config.HealthCheckConfig
	targetGroupStatus := model.TargetGroupStatus{
		TargetGroupARN: aws.StringValue(tgSummary.Arn),
		TargetGroupID:  aws.StringValue(tgSummary.Id),
	}

	if healthCheckConfig == nil {
		s.log.Debugf("HealthCheck is empty. Resetting to default settings")
		targetGroupProtocolVersion := targetGroup.Spec.Config.ProtocolVersion
		healthCheckConfig = s.getDefaultHealthCheckConfig(targetGroupProtocolVersion)
	}

	_, err := vpcLatticeSess.UpdateTargetGroupWithContext(ctx, &vpclattice.UpdateTargetGroupInput{
		HealthCheck:           healthCheckConfig,
		TargetGroupIdentifier: tgSummary.Id,
	})

	if err != nil {
		return model.TargetGroupStatus{}, err
	}

	return targetGroupStatus, nil
}

func (s *defaultTargetGroupManager) Delete(ctx context.Context, targetGroup *model.TargetGroup) error {
	s.log.Debugf("Deleting VPC Lattice Target Group %s", targetGroup.Spec.Name)

	if targetGroup.Spec.LatticeID == "" {
		s.log.Debugf("No ID found for target group, ignoring.")
		return nil
	}

	vpcLatticeSess := s.cloud.Lattice()
	// de-register all targets first
	listTargetsInput := vpclattice.ListTargetsInput{
		TargetGroupIdentifier: &targetGroup.Spec.LatticeID,
	}

	listResp, err := vpcLatticeSess.ListTargetsAsList(ctx, &listTargetsInput)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			if aerr.Code() == vpclattice.ErrCodeResourceNotFoundException {
				// already deleted in lattice, this is OK
				s.log.Debugf("Target group %s was already deleted", targetGroup.Spec.LatticeID)
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
				s.log.Debugf("Target Group %s has non-unused status target(s), which means this targetGroup"+
					" is still in use by a VPC Lattice Service, so it cannot be deleted now", targetGroup.Spec.LatticeID)
				// Before call the defaultTargetGroupManager.Delete(), we always call the latticeServiceManager.Delete() first,
				//  *t.Status != vpclattice.TargetStatusUnused means previous delete latticeService still in the progress, we could wait for 20 seconds and then retry
				return errors.New(LATTICE_RETRY)
			}
			targets = append(targets, &vpclattice.Target{
				Id:   t.Id,
				Port: t.Port,
			})
		}

		targetsAreRegistered := len(targets) > 0
		if targetsAreRegistered {
			deRegisterInput := vpclattice.DeregisterTargetsInput{
				TargetGroupIdentifier: &targetGroup.Spec.LatticeID,
				Targets:               targets,
			}

			deRegResp, err := vpcLatticeSess.DeregisterTargetsWithContext(ctx, &deRegisterInput)
			if err != nil {
				return err
			}

			isDeRegRespUnsuccessful := len(deRegResp.Unsuccessful) > 0
			if isDeRegRespUnsuccessful {
				s.log.Debugf("Target deregistration was unsuccessful, will retry later")
				return errors.New(LATTICE_RETRY)
			}
		}
	}

	deleteTGInput := vpclattice.DeleteTargetGroupInput{
		TargetGroupIdentifier: &targetGroup.Spec.LatticeID,
	}
	_, err = vpcLatticeSess.DeleteTargetGroupWithContext(ctx, &deleteTGInput)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			if aerr.Code() == vpclattice.ErrCodeResourceNotFoundException {
				s.log.Debugf("Target group %s was already deleted", targetGroup.Spec.LatticeID)
				err = nil
			}
		}
	}

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
	if err != nil {
		return nil, err
	}

	for _, tg := range resp {
		tgInput := vpclattice.GetTargetGroupInput{
			TargetGroupIdentifier: tg.Id,
		}

		tgOutput, err := vpcLatticeSess.GetTargetGroupWithContext(ctx, &tgInput)
		if err != nil {
			continue
		}

		if tgOutput.Config != nil && aws.StringValue(tgOutput.Config.VpcIdentifier) == config.VpcID {
			// retrieve target group tags
			//ListTagsForResourceWithContext
			tagsInput := vpclattice.ListTagsForResourceInput{
				ResourceArn: tg.Arn,
			}

			tagsOutput, err := vpcLatticeSess.ListTagsForResourceWithContext(ctx, &tagsInput)
			if err != nil {
				s.log.Debugf("Error listing tags for target group %s: %s", *tg.Arn, err)
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

func isNameOfTargetGroup(targetGroup *model.TargetGroup, name string) bool {
	if targetGroup.Spec.Config.IsServiceImport {
		// We are missing protocol info for ServiceImport, but we do know the RouteType.
		// Relying on the assumption that we have one TG per (RouteType, Service),
		// do a simple guess to find the matching TG.
		validProtocols := []string{
			vpclattice.TargetGroupProtocolHttp,
			vpclattice.TargetGroupProtocolHttps,
		}
		validProtocolVersions := []string{
			vpclattice.TargetGroupProtocolVersionHttp1,
			vpclattice.TargetGroupProtocolVersionHttp2,
		}
		if targetGroup.Spec.Config.ProtocolVersion == vpclattice.TargetGroupProtocolVersionGrpc {
			validProtocolVersions = []string{vpclattice.TargetGroupProtocolVersionGrpc}
		}

		for _, p := range validProtocols {
			for _, pv := range validProtocolVersions {
				candidate := &model.TargetGroup{
					Spec: model.TargetGroupSpec{
						Name: targetGroup.Spec.Name,
						Config: model.TargetGroupConfig{
							Protocol:        p,
							ProtocolVersion: pv,
						},
					},
				}
				if name == getLatticeTGName(candidate) {
					return true
				}
			}
		}
		return false
	} else {
		return name == getLatticeTGName(targetGroup)
	}
}

func (s *defaultTargetGroupManager) findTargetGroup(
	ctx context.Context,
	targetGroup *model.TargetGroup,
) (*vpclattice.TargetGroupSummary, error) {
	vpcLatticeSess := s.cloud.Lattice()
	targetGroupListInput := vpclattice.ListTargetGroupsInput{}
	resp, err := vpcLatticeSess.ListTargetGroupsAsList(ctx, &targetGroupListInput)
	if err != nil {
		return nil, err
	}

	for _, r := range resp {
		if isNameOfTargetGroup(targetGroup, *r.Name) {
			s.log.Debugf("Target group %s already exists with arn %s", *r.Name, *r.Arn)
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

	return nil, nil
}

// Get default health check configuration according to
// https://docs.aws.amazon.com/vpc-lattice/latest/ug/target-group-health-checks.html#health-check-settings
func (s *defaultTargetGroupManager) getDefaultHealthCheckConfig(targetGroupProtocolVersion string) *vpclattice.HealthCheckConfig {
	var intResetValue int64 = 0

	defaultMatcher := vpclattice.Matcher{
		HttpCode: aws.String("200"),
	}

	defaultPath := "/"
	defaultProtocol := vpclattice.TargetGroupProtocolHttp

	if targetGroupProtocolVersion == "" {
		targetGroupProtocolVersion = vpclattice.TargetGroupProtocolVersionHttp1
	}

	enabled := targetGroupProtocolVersion == vpclattice.TargetGroupProtocolVersionHttp1
	healthCheckProtocolVersion := targetGroupProtocolVersion

	if targetGroupProtocolVersion == vpclattice.TargetGroupProtocolVersionGrpc {
		healthCheckProtocolVersion = vpclattice.HealthCheckProtocolVersionHttp1
	}

	return &vpclattice.HealthCheckConfig{
		Enabled:                    &enabled,
		Protocol:                   &defaultProtocol,
		ProtocolVersion:            &healthCheckProtocolVersion,
		Path:                       &defaultPath,
		Matcher:                    &defaultMatcher,
		Port:                       nil, // Use target port
		HealthCheckIntervalSeconds: &intResetValue,
		HealthyThresholdCount:      &intResetValue,
		UnhealthyThresholdCount:    &intResetValue,
		HealthCheckTimeoutSeconds:  &intResetValue,
	}
}
