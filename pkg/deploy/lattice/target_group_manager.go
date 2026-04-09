package lattice

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/vpclattice"
	"github.com/aws/aws-sdk-go-v2/service/vpclattice/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/pkg/utils"

	"reflect"

	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	lattice_runtime "github.com/aws/aws-application-networking-k8s/pkg/runtime"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

//go:generate mockgen -destination target_group_manager_mock.go -package lattice github.com/aws/aws-application-networking-k8s/pkg/deploy/lattice TargetGroupManager

type TargetGroupManager interface {
	Upsert(ctx context.Context, modelTg *model.TargetGroup) (model.TargetGroupStatus, error)
	Delete(ctx context.Context, modelTg *model.TargetGroup) error
	List(ctx context.Context) ([]tgListOutput, error)
	IsTargetGroupMatch(ctx context.Context, modelTg *model.TargetGroup, latticeTg *types.TargetGroupSummary,
		latticeTags *model.TargetGroupTagFields) (bool, error)
	ResolveRuleTgIds(ctx context.Context, modelRuleAction *model.RuleAction, stack core.Stack) error
}

type defaultTargetGroupManager struct {
	log       gwlog.Logger
	awsCloud  pkg_aws.Cloud
	k8sClient client.Client
}

func NewTargetGroupManager(log gwlog.Logger, awsCloud pkg_aws.Cloud, k8sClient client.Client) *defaultTargetGroupManager {
	return &defaultTargetGroupManager{
		log:       log,
		awsCloud:  awsCloud,
		k8sClient: k8sClient,
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

	latticeTgCfg := &types.TargetGroupConfig{
		Port:            aws.Int32(modelTg.Spec.Port),
		Protocol:        types.TargetGroupProtocol(modelTg.Spec.Protocol),
		ProtocolVersion: types.TargetGroupProtocolVersion(aws.ToString(protocolVersion)),
		VpcIdentifier:   &modelTg.Spec.VpcId,
		IpAddressType:   types.IpAddressType(aws.ToString(ipAddressType)),
		HealthCheck:     modelTg.Spec.HealthCheckConfig,
	}

	latticeTgName := model.GenerateTgName(modelTg.Spec)
	createInput := vpclattice.CreateTargetGroupInput{
		Config: latticeTgCfg,
		Name:   &latticeTgName,
		Type:   types.TargetGroupType(modelTg.Spec.Type),
		Tags:   s.awsCloud.DefaultTags(),
	}
	createInput.Tags[model.K8SClusterNameKey] = modelTg.Spec.K8SClusterName
	createInput.Tags[model.K8SServiceNameKey] = modelTg.Spec.K8SServiceName
	createInput.Tags[model.K8SServiceNamespaceKey] = modelTg.Spec.K8SServiceNamespace
	createInput.Tags[model.K8SSourceTypeKey] = string(modelTg.Spec.K8SSourceType)
	createInput.Tags[model.K8SProtocolVersionKey] = modelTg.Spec.ProtocolVersion

	if modelTg.Spec.IsSourceTypeRoute() {
		createInput.Tags[model.K8SRouteNameKey] = modelTg.Spec.K8SRouteName
		createInput.Tags[model.K8SRouteNamespaceKey] = modelTg.Spec.K8SRouteNamespace
	}

	createInput.Tags = s.awsCloud.MergeTags(createInput.Tags, modelTg.Spec.AdditionalTags)

	lattice := s.awsCloud.Lattice()
	resp, err := lattice.CreateTargetGroup(ctx, &createInput)
	if err != nil {
		return model.TargetGroupStatus{},
			fmt.Errorf("failed CreateTargetGroup %s due to %s", latticeTgName, err)
	}
	s.log.Infof(ctx, "Success CreateTargetGroup %s", latticeTgName)

	latticeTgStatus := string(resp.Status)
	if latticeTgStatus != string(types.TargetGroupStatusActive) &&
		latticeTgStatus != string(types.TargetGroupStatusCreateInProgress) {

		s.log.Infof(ctx, "Target group is not in the desired state. State is %s, will retry", latticeTgStatus)
		return model.TargetGroupStatus{}, lattice_runtime.NewRetryError()
	}

	// create-in-progress is considered success
	// later, target reg may need to retry due to the state, and that's OK
	return model.TargetGroupStatus{
		Name: aws.ToString(resp.Name),
		Arn:  aws.ToString(resp.Arn),
		Id:   aws.ToString(resp.Id)}, nil
}

func (s *defaultTargetGroupManager) update(ctx context.Context, targetGroup *model.TargetGroup, latticeTg *vpclattice.GetTargetGroupOutput) (model.TargetGroupStatus, error) {
	healthCheckConfig := targetGroup.Spec.HealthCheckConfig

	err := s.awsCloud.Tagging().UpdateTags(ctx, aws.ToString(latticeTg.Arn), targetGroup.Spec.AdditionalTags, nil)
	if err != nil {
		return model.TargetGroupStatus{}, fmt.Errorf("failed to update tags for target group %s: %w", aws.ToString(latticeTg.Id), err)
	}

	if healthCheckConfig == nil {
		s.log.Debugf(ctx, "HealthCheck is empty. Resetting to default settings")
		healthCheckConfig = &types.HealthCheckConfig{}
	}

	// Try to resolve health check configuration from TargetGroupPolicy using centralized resolver
	resolver := NewHealthCheckConfigResolver(s.log, s.k8sClient)
	policyHealthCheckConfig, err := resolver.ResolveHealthCheckConfig(ctx, targetGroup)
	if err != nil {
		s.log.Debugf(ctx, "Failed to resolve health check config from policy: %v", err)
		// Continue with existing behavior - use provided config or defaults
	} else if policyHealthCheckConfig != nil {
		s.log.Debugf(ctx, "Using health check configuration from TargetGroupPolicy")
		healthCheckConfig = policyHealthCheckConfig
	}

	s.fillDefaultHealthCheckConfig(healthCheckConfig, targetGroup.Spec.Protocol, targetGroup.Spec.ProtocolVersion)

	if !reflect.DeepEqual(healthCheckConfig, latticeTg.Config.HealthCheck) {
		_, err := s.awsCloud.Lattice().UpdateTargetGroup(ctx, &vpclattice.UpdateTargetGroupInput{
			HealthCheck:           healthCheckConfig,
			TargetGroupIdentifier: latticeTg.Id,
		})
		if err != nil {
			return model.TargetGroupStatus{},
				fmt.Errorf("failed UpdateTargetGroup %s due to %w", aws.ToString(latticeTg.Id), err)
		}
	}

	modelTgStatus := model.TargetGroupStatus{
		Name: aws.ToString(latticeTg.Name),
		Arn:  aws.ToString(latticeTg.Arn),
		Id:   aws.ToString(latticeTg.Id),
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
			Name: aws.ToString(latticeTgSummary.Name),
			Arn:  aws.ToString(latticeTgSummary.Arn),
			Id:   aws.ToString(latticeTgSummary.Id),
		}
	}
	s.log.Debugf(ctx, "Deleting target group %s", modelTg.Status.Id)

	lattice := s.awsCloud.Lattice()

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

	var targetsToDeregister []types.Target
	drainCount := 0
	for _, t := range listResp {
		targetsToDeregister = append(targetsToDeregister, types.Target{
			Id:   t.Id,
			Port: t.Port,
		})

		if string(t.Status) == string(types.TargetStatusDraining) {
			drainCount++
		}
	}

	if drainCount > 0 {
		// no point in trying to deregister may as well wait
		return fmt.Errorf("%w: cannot deregister targets for %s as %d targets are DRAINING", lattice_runtime.NewRetryError(), modelTg.Status.Id, drainCount)
	}

	if len(targetsToDeregister) > 0 {
		var deregisterTargetsError error
		chunks := utils.Chunks(targetsToDeregister, maxTargetsPerLatticeTargetsApiCall)
		for i, targets := range chunks {
			deregisterInput := vpclattice.DeregisterTargetsInput{
				TargetGroupIdentifier: &modelTg.Status.Id,
				Targets:               targets,
			}
			deregisterResponse, err := lattice.DeregisterTargets(ctx, &deregisterInput)
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
	_, err = lattice.DeleteTargetGroup(ctx, &deleteTGInput)
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
	tgSummary types.TargetGroupSummary
	tags      services.Tags
}

// Retrieve all TGs in the account, including tags. If individual tags fetch fails, tags will be nil for that tg
func (s *defaultTargetGroupManager) List(ctx context.Context) ([]tgListOutput, error) {
	lattice := s.awsCloud.Lattice()
	var tgList []tgListOutput
	targetGroupListInput := vpclattice.ListTargetGroupsInput{}
	resp, err := lattice.ListTargetGroupsAsList(ctx, &targetGroupListInput)
	if err != nil {
		return nil, err
	}
	if len(resp) == 0 {
		return nil, nil
	}
	tgArns := utils.SliceMap(resp, func(tg types.TargetGroupSummary) string {
		return aws.ToString(tg.Arn)
	})
	tgArnToTagsMap, err := s.awsCloud.Tagging().GetTagsForArns(ctx, tgArns)

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
	arns, err := s.awsCloud.Tagging().FindResourcesByTags(ctx, services.ResourceTypeTargetGroup,
		model.TagsFromTGTagFields(modelTargetGroup.Spec.TargetGroupTagFields))
	if err != nil {
		return nil, err
	}
	if len(arns) == 0 {
		return nil, nil
	}

	for _, arn := range arns {
		latticeTg, err := s.awsCloud.Lattice().GetTargetGroup(ctx, &vpclattice.GetTargetGroupInput{
			TargetGroupIdentifier: &arn,
		})
		if err != nil {
			if services.IsNotFoundError(err) {
				continue
			}
			return nil, err
		}

		// we ignore create failed status, so may as well check for it first
		status := string(latticeTg.Status)
		if status == string(types.TargetGroupStatusCreateFailed) {
			continue
		}

		// Check the immutable fields to ensure TG is valid
		match, err := s.IsTargetGroupMatch(ctx, modelTargetGroup, &types.TargetGroupSummary{
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
			case string(types.TargetGroupStatusCreateInProgress), string(types.TargetGroupStatusDeleteInProgress):
				return nil, lattice_runtime.NewRetryError()
			case string(types.TargetGroupStatusDeleteFailed), string(types.TargetGroupStatusActive):
				return latticeTg, nil
			}
		}
	}

	return nil, nil
}

// Skips tag verification if not provided
func (s *defaultTargetGroupManager) IsTargetGroupMatch(ctx context.Context,
	modelTg *model.TargetGroup, latticeTg *types.TargetGroupSummary,
	latticeTagsAsModelTags *model.TargetGroupTagFields) (bool, error) {

	if int64(aws.ToInt32(latticeTg.Port)) != int64(modelTg.Spec.Port) ||
		string(latticeTg.Protocol) != modelTg.Spec.Protocol ||
		string(latticeTg.IpAddressType) != modelTg.Spec.IpAddressType ||
		string(latticeTg.Type) != string(modelTg.Spec.Type) ||
		aws.ToString(latticeTg.VpcIdentifier) != modelTg.Spec.VpcId {

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
func (s *defaultTargetGroupManager) getDefaultHealthCheckConfig(targetGroupProtocol string, targetGroupProtocolVersion string) *types.HealthCheckConfig {
	if targetGroupProtocol == string(types.TargetGroupProtocolTcp) {
		return &types.HealthCheckConfig{
			Enabled: aws.Bool(false),
		}
	}

	var (
		defaultHealthCheckIntervalSeconds int32         = 30
		defaultHealthCheckTimeoutSeconds  int32         = 5
		defaultHealthyThresholdCount      int32         = 5
		defaultUnhealthyThresholdCount    int32         = 2
		defaultMatcher                    types.Matcher = &types.MatcherMemberHttpCode{Value: "200"}
		defaultPath                                     = "/"
		defaultProtocol                                 = types.TargetGroupProtocolHttp
	)

	if targetGroupProtocolVersion == "" {
		targetGroupProtocolVersion = string(types.TargetGroupProtocolVersionHttp1)
	}

	enabled := targetGroupProtocolVersion == string(types.TargetGroupProtocolVersionHttp1)
	healthCheckProtocolVersion := types.HealthCheckProtocolVersion(targetGroupProtocolVersion)

	if targetGroupProtocolVersion == string(types.TargetGroupProtocolVersionGrpc) {
		healthCheckProtocolVersion = types.HealthCheckProtocolVersionHttp1
	}

	return &types.HealthCheckConfig{
		Enabled:                    &enabled,
		Protocol:                   defaultProtocol,
		ProtocolVersion:            healthCheckProtocolVersion,
		Path:                       &defaultPath,
		Matcher:                    defaultMatcher,
		Port:                       nil, // Use target port
		HealthyThresholdCount:      &defaultHealthyThresholdCount,
		UnhealthyThresholdCount:    &defaultUnhealthyThresholdCount,
		HealthCheckTimeoutSeconds:  &defaultHealthCheckTimeoutSeconds,
		HealthCheckIntervalSeconds: &defaultHealthCheckIntervalSeconds,
	}
}

func (s *defaultTargetGroupManager) fillDefaultHealthCheckConfig(hc *types.HealthCheckConfig, targetGroupProtocol string, targetGroupProtocolVersion string) {
	defaultCfg := s.getDefaultHealthCheckConfig(targetGroupProtocol, targetGroupProtocolVersion)
	if hc.Enabled == nil {
		hc.Enabled = defaultCfg.Enabled
	}
	if hc.Protocol == "" {
		hc.Protocol = defaultCfg.Protocol
	}
	if hc.ProtocolVersion == "" {
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
		vpcMatch := (svcImportTg.VpcId == "") || (svcImportTg.VpcId == aws.ToString(tg.tgSummary.VpcIdentifier))
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
