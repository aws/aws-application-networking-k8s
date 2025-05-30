package lattice

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"

	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/pkg/utils"

	"reflect"

	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

//go:generate mockgen -destination target_group_manager_mock.go -package lattice github.com/aws/aws-application-networking-k8s/pkg/deploy/lattice TargetGroupManager

type TargetGroupManager interface {
	Upsert(ctx context.Context, modelTg *model.TargetGroup) (model.TargetGroupStatus, error)
	Delete(ctx context.Context, modelTg *model.TargetGroup) error
	List(ctx context.Context) ([]tgListOutput, error)
	IsTargetGroupMatch(ctx context.Context, modelTg *model.TargetGroup, latticeTg *vpclattice.TargetGroupSummary,
		latticeTags *model.TargetGroupTagFields) (bool, error)
	ResolveRuleTgIds(ctx context.Context, modelRuleAction *model.RuleAction, stack core.Stack) error
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

func (s *defaultTargetGroupManager) Upsert(
	ctx context.Context,
	modelTg *model.TargetGroup,
) (model.TargetGroupStatus, error) {
	// check if exists
	latticeTgSummary, err := s.findTargetGroup(ctx, modelTg)
	if err != nil {
		return model.TargetGroupStatus{}, err
	}

	if latticeTgSummary == nil {
		return s.create(ctx, modelTg)
	} else {
		return s.update(ctx, modelTg, latticeTgSummary)
	}
}

func (s *defaultTargetGroupManager) create(ctx context.Context, modelTg *model.TargetGroup) (model.TargetGroupStatus, error) {
	var ipAddressType, protocolVersion *string
	if modelTg.Spec.IpAddressType != "" {
		ipAddressType = &modelTg.Spec.IpAddressType
	}
	if modelTg.Spec.ProtocolVersion == "" {
		protocolVersion = nil
	} else {
		protocolVersion = &modelTg.Spec.ProtocolVersion
	}

	latticeTgCfg := &vpclattice.TargetGroupConfig{
		Port:            aws.Int64(int64(modelTg.Spec.Port)),
		Protocol:        &modelTg.Spec.Protocol,
		ProtocolVersion: protocolVersion,
		VpcIdentifier:   &modelTg.Spec.VpcId,
		IpAddressType:   ipAddressType,
		HealthCheck:     modelTg.Spec.HealthCheckConfig,
	}

	latticeTgType := string(modelTg.Spec.Type)

	latticeTgName := model.GenerateTgName(modelTg.Spec)
	createInput := vpclattice.CreateTargetGroupInput{
		Config: latticeTgCfg,
		Name:   &latticeTgName,
		Type:   &latticeTgType,
		Tags:   s.cloud.DefaultTags(),
	}
	createInput.Tags[model.K8SClusterNameKey] = &modelTg.Spec.K8SClusterName
	createInput.Tags[model.K8SServiceNameKey] = &modelTg.Spec.K8SServiceName
	createInput.Tags[model.K8SServiceNamespaceKey] = &modelTg.Spec.K8SServiceNamespace
	createInput.Tags[model.K8SSourceTypeKey] = aws.String(string(modelTg.Spec.K8SSourceType))
	createInput.Tags[model.K8SProtocolVersionKey] = &modelTg.Spec.ProtocolVersion

	if modelTg.Spec.IsSourceTypeRoute() {
		createInput.Tags[model.K8SRouteNameKey] = &modelTg.Spec.K8SRouteName
		createInput.Tags[model.K8SRouteNamespaceKey] = &modelTg.Spec.K8SRouteNamespace
	}

	lattice := s.cloud.Lattice()
	resp, err := lattice.CreateTargetGroupWithContext(ctx, &createInput)
	if err != nil {
		return model.TargetGroupStatus{},
			fmt.Errorf("failed CreateTargetGroup %s due to %s", latticeTgName, err)
	}
	s.log.Infof(ctx, "Success CreateTargetGroup %s", latticeTgName)

	latticeTgStatus := aws.StringValue(resp.Status)
	if latticeTgStatus != vpclattice.TargetGroupStatusActive &&
		latticeTgStatus != vpclattice.TargetGroupStatusCreateInProgress {

		s.log.Infof(ctx, "Target group is not in the desired state. State is %s, will retry", latticeTgStatus)
		return model.TargetGroupStatus{}, errors.New(LATTICE_RETRY)
	}

	// create-in-progress is considered success
	// later, target reg may need to retry due to the state, and that's OK
	return model.TargetGroupStatus{
		Name: aws.StringValue(resp.Name),
		Arn:  aws.StringValue(resp.Arn),
		Id:   aws.StringValue(resp.Id)}, nil
}

func (s *defaultTargetGroupManager) update(ctx context.Context, targetGroup *model.TargetGroup, latticeTg *vpclattice.GetTargetGroupOutput) (model.TargetGroupStatus, error) {
	healthCheckConfig := targetGroup.Spec.HealthCheckConfig

	if healthCheckConfig == nil {
		s.log.Debugf(ctx, "HealthCheck is empty. Resetting to default settings")
		healthCheckConfig = &vpclattice.HealthCheckConfig{}
	}
	s.fillDefaultHealthCheckConfig(healthCheckConfig, targetGroup.Spec.Protocol, targetGroup.Spec.ProtocolVersion)

	if !reflect.DeepEqual(healthCheckConfig, latticeTg.Config.HealthCheck) {
		_, err := s.cloud.Lattice().UpdateTargetGroupWithContext(ctx, &vpclattice.UpdateTargetGroupInput{
			HealthCheck:           healthCheckConfig,
			TargetGroupIdentifier: latticeTg.Id,
		})
		if err != nil {
			return model.TargetGroupStatus{},
				fmt.Errorf("failed UpdateTargetGroup %s due to %w", aws.StringValue(latticeTg.Id), err)
		}
	}

	modelTgStatus := model.TargetGroupStatus{
		Name: aws.StringValue(latticeTg.Name),
		Arn:  aws.StringValue(latticeTg.Arn),
		Id:   aws.StringValue(latticeTg.Id),
	}

	return modelTgStatus, nil
}

func (s *defaultTargetGroupManager) Delete(ctx context.Context, modelTg *model.TargetGroup) error {
	if modelTg.Status == nil || modelTg.Status.Id == "" {
		latticeTgSummary, err := s.findTargetGroup(ctx, modelTg)
		if err != nil {
			return err
		}

		if latticeTgSummary == nil {
			// nothing to delete
			s.log.Infof(ctx, "Target group with name prefix %s does not exist, nothing to delete", model.TgNamePrefix(modelTg.Spec))
			return nil
		}

		modelTg.Status = &model.TargetGroupStatus{
			Name: aws.StringValue(latticeTgSummary.Name),
			Arn:  aws.StringValue(latticeTgSummary.Arn),
			Id:   aws.StringValue(latticeTgSummary.Id),
		}
	}
	s.log.Debugf(ctx, "Deleting target group %s", modelTg.Status.Id)

	lattice := s.cloud.Lattice()

	// de-register all targets first
	listTargetsInput := vpclattice.ListTargetsInput{
		TargetGroupIdentifier: &modelTg.Status.Id,
	}

	listResp, err := lattice.ListTargetsAsList(ctx, &listTargetsInput)
	if err != nil {
		if services.IsLatticeAPINotFoundErr(err) {
			s.log.Debugf(ctx, "Target group %s was already deleted", modelTg.Status.Id)
			return nil
		}
		return fmt.Errorf("failed ListTargets %s due to %s", modelTg.Status.Id, err)
	}

	var targetsToDeregister []*vpclattice.Target
	drainCount := 0
	for _, t := range listResp {
		targetsToDeregister = append(targetsToDeregister, &vpclattice.Target{
			Id:   t.Id,
			Port: t.Port,
		})

		if aws.StringValue(t.Status) == vpclattice.TargetStatusDraining {
			drainCount++
		}
	}

	if drainCount > 0 {
		// no point in trying to deregister may as well wait
		return fmt.Errorf("cannot deregister targets for %s as %d targets are DRAINING", modelTg.Status.Id, drainCount)
	}

	if len(targetsToDeregister) > 0 {
		var deregisterTargetsError error
		chunks := utils.Chunks(targetsToDeregister, maxTargetsPerLatticeTargetsApiCall)
		for i, targets := range chunks {
			deregisterInput := vpclattice.DeregisterTargetsInput{
				TargetGroupIdentifier: &modelTg.Status.Id,
				Targets:               targets,
			}
			deregisterResponse, err := lattice.DeregisterTargetsWithContext(ctx, &deregisterInput)
			if err != nil {
				deregisterTargetsError = errors.Join(deregisterTargetsError, fmt.Errorf("failed to deregister targets from VPC Lattice Target Group %s due to %s", modelTg.Status.Id, err))
			}
			if len(deregisterResponse.Unsuccessful) > 0 {
				deregisterTargetsError = errors.Join(deregisterTargetsError, fmt.Errorf("failed to deregister targets from VPC Lattice Target Group %s for chunk %d/%d, unsuccessful targets %v",
					modelTg.Status.Id, i+1, len(chunks), deregisterResponse.Unsuccessful))
			}
			s.log.Debugf(ctx, "Successfully deregistered targets from VPC Lattice Target Group %s for chunk %d/%d", modelTg.Status.Id, i+1, len(chunks))
		}
		if deregisterTargetsError != nil {
			return deregisterTargetsError
		}
	}

	deleteTGInput := vpclattice.DeleteTargetGroupInput{
		TargetGroupIdentifier: &modelTg.Status.Id,
	}
	_, err = lattice.DeleteTargetGroupWithContext(ctx, &deleteTGInput)
	if err != nil {
		if services.IsLatticeAPINotFoundErr(err) {
			s.log.Infof(ctx, "Target group %s was already deleted", modelTg.Status.Id)
			return nil
		} else {
			return fmt.Errorf("failed DeleteTargetGroup %s due to %s", modelTg.Status.Id, err)
		}
	}

	s.log.Infof(ctx, "Success DeleteTargetGroup %s", modelTg.Status.Id)
	return nil
}

type tgListOutput struct {
	tgSummary *vpclattice.TargetGroupSummary
	tags      services.Tags
}

// Retrieve all TGs in the account, including tags. If individual tags fetch fails, tags will be nil for that tg
func (s *defaultTargetGroupManager) List(ctx context.Context) ([]tgListOutput, error) {
	lattice := s.cloud.Lattice()
	var tgList []tgListOutput
	targetGroupListInput := vpclattice.ListTargetGroupsInput{}
	resp, err := lattice.ListTargetGroupsAsList(ctx, &targetGroupListInput)
	if err != nil {
		return nil, err
	}
	if len(resp) == 0 {
		return nil, nil
	}
	tgArns := utils.SliceMap(resp, func(tg *vpclattice.TargetGroupSummary) string {
		return aws.StringValue(tg.Arn)
	})
	tgArnToTagsMap, err := s.cloud.Tagging().GetTagsForArns(ctx, tgArns)

	if err != nil {
		return nil, err
	}
	for _, tg := range resp {
		tgList = append(tgList, tgListOutput{
			tgSummary: tg,
			tags:      tgArnToTagsMap[*tg.Arn],
		})
	}
	return tgList, err
}

func (s *defaultTargetGroupManager) findTargetGroup(
	ctx context.Context,
	modelTargetGroup *model.TargetGroup,
) (*vpclattice.GetTargetGroupOutput, error) {
	arns, err := s.cloud.Tagging().FindResourcesByTags(ctx, services.ResourceTypeTargetGroup,
		model.TagsFromTGTagFields(modelTargetGroup.Spec.TargetGroupTagFields))
	if err != nil {
		return nil, err
	}
	if len(arns) == 0 {
		return nil, nil
	}

	for _, arn := range arns {
		latticeTg, err := s.cloud.Lattice().GetTargetGroupWithContext(ctx, &vpclattice.GetTargetGroupInput{
			TargetGroupIdentifier: &arn,
		})
		if err != nil {
			if services.IsNotFoundError(err) {
				continue
			}
			return nil, err
		}

		// we ignore create failed status, so may as well check for it first
		status := aws.StringValue(latticeTg.Status)
		if status == vpclattice.TargetGroupStatusCreateFailed {
			continue
		}

		// Check the immutable fields to ensure TG is valid
		match, err := s.IsTargetGroupMatch(ctx, modelTargetGroup, &vpclattice.TargetGroupSummary{
			Arn:           latticeTg.Arn,
			Port:          latticeTg.Config.Port,
			Protocol:      latticeTg.Config.Protocol,
			IpAddressType: latticeTg.Config.IpAddressType,
			Type:          latticeTg.Type,
			VpcIdentifier: latticeTg.Config.VpcIdentifier,
		}, nil) // we already know that tags match
		if err != nil {
			return nil, err
		}
		if match {
			switch status {
			case vpclattice.TargetGroupStatusCreateInProgress, vpclattice.TargetGroupStatusDeleteInProgress:
				return nil, errors.New(LATTICE_RETRY)
			case vpclattice.TargetGroupStatusDeleteFailed, vpclattice.TargetGroupStatusActive:
				return latticeTg, nil
			}
		}
	}

	return nil, nil
}

// Skips tag verification if not provided
func (s *defaultTargetGroupManager) IsTargetGroupMatch(ctx context.Context,
	modelTg *model.TargetGroup, latticeTg *vpclattice.TargetGroupSummary,
	latticeTagsAsModelTags *model.TargetGroupTagFields) (bool, error) {

	if aws.Int64Value(latticeTg.Port) != int64(modelTg.Spec.Port) ||
		aws.StringValue(latticeTg.Protocol) != modelTg.Spec.Protocol ||
		aws.StringValue(latticeTg.IpAddressType) != modelTg.Spec.IpAddressType ||
		aws.StringValue(latticeTg.Type) != string(modelTg.Spec.Type) ||
		aws.StringValue(latticeTg.VpcIdentifier) != modelTg.Spec.VpcId {

		return false, nil
	}

	if latticeTagsAsModelTags != nil {
		tagsMatch := model.TagFieldsMatch(modelTg.Spec, *latticeTagsAsModelTags)
		if !tagsMatch {
			return false, nil
		}
	}

	return true, nil
}

// Get default health check configuration according to
// https://docs.aws.amazon.com/vpc-lattice/latest/ug/target-group-health-checks.html#health-check-settings
func (s *defaultTargetGroupManager) getDefaultHealthCheckConfig(targetGroupProtocol string, targetGroupProtocolVersion string) *vpclattice.HealthCheckConfig {
	if targetGroupProtocol == vpclattice.TargetGroupProtocolTcp {
		return &vpclattice.HealthCheckConfig{
			Enabled: aws.Bool(false),
		}
	}

	var (
		defaultHealthCheckIntervalSeconds int64 = 30
		defaultHealthCheckTimeoutSeconds  int64 = 5
		defaultHealthyThresholdCount      int64 = 5
		defaultUnhealthyThresholdCount    int64 = 2
		defaultMatcher                          = vpclattice.Matcher{
			HttpCode: aws.String("200"),
		}
		defaultPath     = "/"
		defaultProtocol = vpclattice.TargetGroupProtocolHttp
	)

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
		HealthyThresholdCount:      &defaultHealthyThresholdCount,
		UnhealthyThresholdCount:    &defaultUnhealthyThresholdCount,
		HealthCheckTimeoutSeconds:  &defaultHealthCheckTimeoutSeconds,
		HealthCheckIntervalSeconds: &defaultHealthCheckIntervalSeconds,
	}
}

func (s *defaultTargetGroupManager) fillDefaultHealthCheckConfig(hc *vpclattice.HealthCheckConfig, targetGroupProtocol string, targetGroupProtocolVersion string) {
	defaultCfg := s.getDefaultHealthCheckConfig(targetGroupProtocol, targetGroupProtocolVersion)
	if hc.Enabled == nil {
		hc.Enabled = defaultCfg.Enabled
	}
	if hc.Protocol == nil {
		hc.Protocol = defaultCfg.Protocol
	}
	if hc.ProtocolVersion == nil {
		hc.ProtocolVersion = defaultCfg.ProtocolVersion
	}
	if hc.Path == nil {
		hc.Path = defaultCfg.Path
	}
	if hc.Matcher == nil {
		hc.Matcher = defaultCfg.Matcher
	}
	if hc.HealthCheckTimeoutSeconds == nil {
		hc.HealthCheckTimeoutSeconds = defaultCfg.HealthCheckTimeoutSeconds
	}
	if hc.HealthCheckIntervalSeconds == nil {
		hc.HealthCheckIntervalSeconds = defaultCfg.HealthCheckIntervalSeconds
	}
	if hc.HealthyThresholdCount == nil {
		hc.HealthyThresholdCount = defaultCfg.HealthyThresholdCount
	}
	if hc.UnhealthyThresholdCount == nil {
		hc.UnhealthyThresholdCount = defaultCfg.UnhealthyThresholdCount
	}
}

func (s *defaultTargetGroupManager) findSvcExportTG(ctx context.Context, svcImportTg model.SvcImportTargetGroup) (string, error) {
	tgs, err := s.List(ctx)
	if err != nil {
		return "", err
	}
	for _, tg := range tgs {
		tgTags := model.TGTagFieldsFromTags(tg.tags)
		svcMatch := tgTags.IsSourceTypeServiceExport() && (tgTags.K8SServiceName == svcImportTg.K8SServiceName) &&
			(tgTags.K8SServiceNamespace == svcImportTg.K8SServiceNamespace)
		clusterMatch := (svcImportTg.K8SClusterName == "") || (tgTags.K8SClusterName == svcImportTg.K8SClusterName)
		vpcMatch := (svcImportTg.VpcId == "") || (svcImportTg.VpcId == aws.StringValue(tg.tgSummary.VpcIdentifier))
		if svcMatch && clusterMatch && vpcMatch {
			return *tg.tgSummary.Id, nil
		}
	}
	return "", errors.New("target group for service import could not be found")
}

// ResolveRuleTgIds populates all target group ids in the rule's actions
func (s *defaultTargetGroupManager) ResolveRuleTgIds(ctx context.Context, ruleAction *model.RuleAction, stack core.Stack) error {
	if len(ruleAction.TargetGroups) == 0 {
		s.log.Debugf(ctx, "no target groups to resolve for rule")
		return nil
	}
	for i, ruleActionTg := range ruleAction.TargetGroups {
		if ruleActionTg.StackTargetGroupId == "" && ruleActionTg.SvcImportTG == nil && ruleActionTg.LatticeTgId == "" {
			return errors.New("rule TG is missing a required target group identifier")
		}
		if ruleActionTg.LatticeTgId != "" {
			s.log.Debugf(ctx, "Rule TG %d already resolved %s", i, ruleActionTg.LatticeTgId)
			continue
		}
		if ruleActionTg.StackTargetGroupId != "" {
			if ruleActionTg.StackTargetGroupId == model.InvalidBackendRefTgId {
				s.log.Debugf(ctx, "Rule TG has an invalid backendref, setting TG id to invalid")
				ruleActionTg.LatticeTgId = model.InvalidBackendRefTgId
				continue
			}
			s.log.Debugf(ctx, "Fetching TG %d from the stack (ID %s)", i, ruleActionTg.StackTargetGroupId)
			stackTg := &model.TargetGroup{}
			err := stack.GetResource(ruleActionTg.StackTargetGroupId, stackTg)
			if err != nil {
				return err
			}
			if stackTg.Status == nil {
				return errors.New("stack target group is missing Status field")
			}
			ruleActionTg.LatticeTgId = stackTg.Status.Id
		}
		if ruleActionTg.SvcImportTG != nil {
			s.log.Debugf(ctx, "Getting target group for service import %s %s (%s, %s)",
				ruleActionTg.SvcImportTG.K8SServiceName, ruleActionTg.SvcImportTG.K8SServiceNamespace,
				ruleActionTg.SvcImportTG.K8SClusterName, ruleActionTg.SvcImportTG.VpcId)
			tgId, err := s.findSvcExportTG(ctx, *ruleActionTg.SvcImportTG)

			if err != nil {
				return err
			}
			ruleActionTg.LatticeTgId = tgId
		}
	}
	return nil
}
