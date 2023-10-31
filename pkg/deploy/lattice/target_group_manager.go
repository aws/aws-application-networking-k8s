package lattice

import (
	"context"
	"errors"
	"fmt"
	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

type TargetGroupManager interface {
	Upsert(ctx context.Context, modelTg *model.TargetGroup) (model.TargetGroupStatus, error)
	Delete(ctx context.Context, modelTg *model.TargetGroup) error
	List(ctx context.Context) ([]tgListOutput, error)
	IsTargetGroupMatch(ctx context.Context, modelTg *model.TargetGroup, latticeTg *vpclattice.TargetGroupSummary,
		latticeTags *model.TargetGroupTagFields) (bool, error)
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
	var ipAddressType *string
	if modelTg.Spec.IpAddressType != "" {
		ipAddressType = &modelTg.Spec.IpAddressType
	}

	latticeTgCfg := &vpclattice.TargetGroupConfig{
		Port:            aws.Int64(int64(modelTg.Spec.Port)),
		Protocol:        &modelTg.Spec.Protocol,
		ProtocolVersion: &modelTg.Spec.ProtocolVersion,
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
	createInput.Tags[model.EKSClusterNameKey] = &modelTg.Spec.ClusterName
	createInput.Tags[model.K8SServiceNameKey] = &modelTg.Spec.K8SServiceName
	createInput.Tags[model.K8SServiceNamespaceKey] = &modelTg.Spec.K8SServiceNamespace
	createInput.Tags[model.K8SSourceTypeKey] = aws.String(string(modelTg.Spec.K8SSourceType))

	if modelTg.Spec.IsSourceTypeRoute() {
		createInput.Tags[model.K8SRouteNameKey] = &modelTg.Spec.K8SRouteName
		createInput.Tags[model.K8SRouteNamespaceKey] = &modelTg.Spec.K8SRouteNamespace
	}

	lattice := s.cloud.Lattice()
	resp, err := lattice.CreateTargetGroupWithContext(ctx, &createInput)
	if err != nil {
		return model.TargetGroupStatus{},
			fmt.Errorf("Failed CreateTargetGroup %s due to %s", latticeTgName, err)
	}
	s.log.Infof("Success CreateTargetGroup %s", latticeTgName)

	latticeTgStatus := aws.StringValue(resp.Status)
	if latticeTgStatus != vpclattice.TargetGroupStatusActive &&
		latticeTgStatus != vpclattice.TargetGroupStatusCreateInProgress {

		s.log.Infof("Target group is not in the desired state. State is %s, will retry", latticeTgStatus)
		return model.TargetGroupStatus{}, errors.New(LATTICE_RETRY)
	}

	// create-in-progress is considered success
	// later, target reg may need to retry due to the state, and that's OK
	return model.TargetGroupStatus{
		Name: aws.StringValue(resp.Name),
		Arn:  aws.StringValue(resp.Arn),
		Id:   aws.StringValue(resp.Id)}, nil
}

func (s *defaultTargetGroupManager) update(ctx context.Context, targetGroup *model.TargetGroup, latticeTgSummary *vpclattice.TargetGroupSummary) (model.TargetGroupStatus, error) {
	healthCheckConfig := targetGroup.Spec.HealthCheckConfig

	if healthCheckConfig == nil {
		s.log.Debugf("HealthCheck is empty. Resetting to default settings")
		protocolVersion := targetGroup.Spec.ProtocolVersion
		healthCheckConfig = s.getDefaultHealthCheckConfig(protocolVersion)
	}

	_, err := s.cloud.Lattice().UpdateTargetGroupWithContext(ctx, &vpclattice.UpdateTargetGroupInput{
		HealthCheck:           healthCheckConfig,
		TargetGroupIdentifier: latticeTgSummary.Id,
	})

	if err != nil {
		return model.TargetGroupStatus{},
			fmt.Errorf("Failed UpdateTargetGroup %s due to %s", aws.StringValue(latticeTgSummary.Id), err)
	}
	s.log.Infof("Success UpdateTargetGroup %s", aws.StringValue(latticeTgSummary.Id))

	modelTgStatus := model.TargetGroupStatus{
		Name: aws.StringValue(latticeTgSummary.Name),
		Arn:  aws.StringValue(latticeTgSummary.Arn),
		Id:   aws.StringValue(latticeTgSummary.Id),
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
			s.log.Infof("Target group with name prefix %s does not exist, nothing to delete", model.TgNamePrefix(modelTg.Spec))
			return nil
		}

		modelTg.Status = &model.TargetGroupStatus{
			Name: aws.StringValue(latticeTgSummary.Name),
			Arn:  aws.StringValue(latticeTgSummary.Arn),
			Id:   aws.StringValue(latticeTgSummary.Id),
		}
	}
	s.log.Debugf("Deleting target group %s", modelTg.Status.Id)

	lattice := s.cloud.Lattice()

	// de-register all targets first
	listTargetsInput := vpclattice.ListTargetsInput{
		TargetGroupIdentifier: &modelTg.Status.Id,
	}

	listResp, err := lattice.ListTargetsAsList(ctx, &listTargetsInput)
	if err != nil {
		if services.IsLatticeAPINotFoundErr(err) {
			s.log.Debugf("Target group %s was already deleted", modelTg.Status.Id)
			return nil
		}
		return fmt.Errorf("Failed ListTargets %s due to %s", modelTg.Status.Id, err)
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
		return fmt.Errorf("Cannot deregister targets for %s as %d targets are DRAINING", modelTg.Status.Id, drainCount)
	}

	if len(targetsToDeregister) > 0 {
		deRegisterInput := vpclattice.DeregisterTargetsInput{
			TargetGroupIdentifier: &modelTg.Status.Id,
			Targets:               targetsToDeregister,
		}

		deRegResp, err := lattice.DeregisterTargetsWithContext(ctx, &deRegisterInput)
		if err != nil {
			return fmt.Errorf("Failed DeregisterTargets %s due to %s", modelTg.Status.Id, err)
		}

		isDeRegRespUnsuccessful := len(deRegResp.Unsuccessful) > 0
		if isDeRegRespUnsuccessful {
			return fmt.Errorf("Unsuccessful (%d total) DeregisterTargets %s (%s), will retry",
				len(deRegResp.Unsuccessful), modelTg.Status.Id, aws.StringValue(deRegResp.Unsuccessful[0].FailureMessage))
		}
		s.log.Infof("Success DeregisterTargets %s", modelTg.Status.Id)
	}

	deleteTGInput := vpclattice.DeleteTargetGroupInput{
		TargetGroupIdentifier: &modelTg.Status.Id,
	}
	_, err = lattice.DeleteTargetGroupWithContext(ctx, &deleteTGInput)
	if err != nil {
		if services.IsLatticeAPINotFoundErr(err) {
			s.log.Infof("Target group %s was already deleted", modelTg.Status.Id)
			return nil
		} else {
			return fmt.Errorf("failed DeleteTargetGroup %s due to %s", modelTg.Status.Id, err)
		}
	}

	s.log.Infof("Success DeleteTargetGroup %s", modelTg.Status.Id)
	return nil
}

type tgListOutput struct {
	getTargetGroupOutput vpclattice.GetTargetGroupOutput
	targetGroupTags      *vpclattice.ListTagsForResourceOutput
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

	for _, tg := range resp {
		tgInput := vpclattice.GetTargetGroupInput{
			TargetGroupIdentifier: tg.Id,
		}

		tgOutput, err := lattice.GetTargetGroupWithContext(ctx, &tgInput)
		if err != nil {
			continue
		}

		if tgOutput.Config != nil && aws.StringValue(tgOutput.Config.VpcIdentifier) == config.VpcID {
			tagsInput := vpclattice.ListTagsForResourceInput{
				ResourceArn: tg.Arn,
			}

			tagsOutput, err := lattice.ListTagsForResourceWithContext(ctx, &tagsInput)
			if err != nil {
				s.log.Infof("Failed ListTags %s: %s", aws.StringValue(tg.Arn), err)
				// setting it to nil, so the caller knows this failed
				tagsOutput = nil
			}
			tgOutput := tgListOutput{
				getTargetGroupOutput: *tgOutput,
				targetGroupTags:      tagsOutput,
			}
			tgList = append(tgList, tgOutput)
		}
	}
	return tgList, err
}

func (s *defaultTargetGroupManager) findTargetGroup(
	ctx context.Context,
	modelTargetGroup *model.TargetGroup,
) (*vpclattice.TargetGroupSummary, error) {
	listInput := vpclattice.ListTargetGroupsInput{
		VpcIdentifier: aws.String(modelTargetGroup.Spec.VpcId),
	}
	resp, err := s.cloud.Lattice().ListTargetGroupsAsList(ctx, &listInput)
	if err != nil {
		return nil, err
	}

	for _, latticeTg := range resp {
		// we ignore create failed status, so may as well check for it first
		status := aws.StringValue(latticeTg.Status)
		if status == vpclattice.TargetGroupStatusCreateFailed {
			continue
		}

		isMatch, err := s.IsTargetGroupMatch(ctx, modelTargetGroup, latticeTg, nil)
		if err != nil {
			return nil, err
		}
		if isMatch {
			s.log.Debugf("Target group %s already exists with arn %s", *latticeTg.Name, *latticeTg.Arn)
			switch status {
			case vpclattice.TargetGroupStatusCreateInProgress:
				return nil, errors.New(LATTICE_RETRY)
			case vpclattice.TargetGroupStatusActive:
				return latticeTg, nil
			case vpclattice.TargetGroupStatusDeleteFailed:
				return latticeTg, nil
			case vpclattice.TargetGroupStatusDeleteInProgress:
				return nil, errors.New(LATTICE_RETRY)
			}
		}
	}

	return nil, nil
}

// latticeTags will be fetched if nil
func (s *defaultTargetGroupManager) IsTargetGroupMatch(ctx context.Context,
	modelTg *model.TargetGroup, latticeTg *vpclattice.TargetGroupSummary,
	latticeTagsAsModelTags *model.TargetGroupTagFields) (bool, error) {

	// first check fields we have before we try tags
	if aws.Int64Value(latticeTg.Port) != int64(modelTg.Spec.Port) ||
		aws.StringValue(latticeTg.Protocol) != modelTg.Spec.Protocol ||
		aws.StringValue(latticeTg.IpAddressType) != modelTg.Spec.IpAddressType ||
		aws.StringValue(latticeTg.Type) != string(modelTg.Spec.Type) ||
		aws.StringValue(latticeTg.VpcIdentifier) != modelTg.Spec.VpcId {

		return false, nil
	}

	// so far so good, now we check tags
	if latticeTagsAsModelTags == nil {
		// fetch the tags if we don't have them already
		req := vpclattice.ListTagsForResourceInput{ResourceArn: latticeTg.Arn}
		res, err := s.cloud.Lattice().ListTagsForResourceWithContext(ctx, &req)
		if err != nil {
			if apierrors.IsNotFound(err) {
				// may have been deleted in the meantime, that's OK
				return false, nil
			}

			return false, err
		}

		tags := model.TGTagFieldsFromTags(res.Tags)
		latticeTagsAsModelTags = &tags
	}

	tagsMatch := model.TagFieldsMatch(modelTg.Spec, *latticeTagsAsModelTags)
	if !tagsMatch {
		return false, nil
	}

	// one last check - ProtocolVersion is not present on TargetGroupSummary, so we have to do a Get
	gtgInput := vpclattice.GetTargetGroupInput{TargetGroupIdentifier: latticeTg.Id}
	gtgOutput, err := s.cloud.Lattice().GetTargetGroupWithContext(ctx, &gtgInput)
	if err != nil {
		return false, err
	}

	pvMatch := aws.StringValue(gtgOutput.Config.ProtocolVersion) == modelTg.Spec.ProtocolVersion
	return pvMatch, nil
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
