package lattice

import (
	"context"
	"errors"
	"fmt"
	"github.com/aws/aws-application-networking-k8s/pkg/gateway"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"time"

	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

// helpful for testing/mocking
func NewTargetGroupSynthesizer(
	log gwlog.Logger,
	cloud pkg_aws.Cloud,
	client client.Client,
	tgManager TargetGroupManager,
	svcExportTgBuilder gateway.SvcExportTargetGroupModelBuilder,
	svcBuilder gateway.LatticeServiceBuilder,
	stack core.Stack,
) *TargetGroupSynthesizer {
	return &TargetGroupSynthesizer{
		log:                log,
		cloud:              cloud,
		client:             client,
		targetGroupManager: tgManager,
		svcExportTgBuilder: svcExportTgBuilder,
		svcBuilder:         svcBuilder,
		stack:              stack,
	}
}

type TargetGroupSynthesizer struct {
	log                gwlog.Logger
	cloud              pkg_aws.Cloud
	client             client.Client
	targetGroupManager TargetGroupManager
	stack              core.Stack
	svcExportTgBuilder gateway.SvcExportTargetGroupModelBuilder
	svcBuilder         gateway.LatticeServiceBuilder
}

func (t *TargetGroupSynthesizer) Synthesize(ctx context.Context) error {
	err1 := t.SynthesizeCreate(ctx)
	err2 := t.SynthesizeDelete(ctx)
	return errors.Join(err1, err2)
}
func (t *TargetGroupSynthesizer) SynthesizeCreate(ctx context.Context) error {
	var resTargetGroups []*model.TargetGroup
	var returnErr = false

	err := t.stack.ListResources(&resTargetGroups)
	if err != nil {
		return err
	}

	for _, resTargetGroup := range resTargetGroups {
		if resTargetGroup.IsDeleted {
			continue
		}

		prefix := model.TgNamePrefix(resTargetGroup.Spec)

		tgStatus, err := t.targetGroupManager.Upsert(ctx, resTargetGroup)
		if err == nil {
			resTargetGroup.Status = &tgStatus
		} else {
			t.log.Debugf("Failed TargetGroupManager.Upsert %s due to %s", prefix, err)
			returnErr = true
		}
	}

	if returnErr {
		return fmt.Errorf("error during target group synthesis, will retry")
	}

	return nil
}
func (t *TargetGroupSynthesizer) SynthesizeDelete(ctx context.Context) error {
	var resTargetGroups []*model.TargetGroup

	err := t.stack.ListResources(&resTargetGroups)
	if err != nil {
		return err
	}

	var retErr error
	for _, resTargetGroup := range resTargetGroups {
		if !resTargetGroup.IsDeleted {
			continue
		}

		err := t.targetGroupManager.Delete(ctx, resTargetGroup)
		if err != nil {
			prefix := model.TgNamePrefix(resTargetGroup.Spec)
			retErr = errors.Join(retErr, fmt.Errorf("failed TargetGroupManager.Delete %s due to %s", prefix, err))
		}
	}

	if retErr != nil {
		return retErr
	}
	return nil
}

// this method assumes all synthesis
func (t *TargetGroupSynthesizer) SynthesizeUnusedDelete(ctx context.Context) error {
	tgsToDelete, err := t.calculateTargetGroupsToDelete(ctx)
	if err != nil {
		return err
	}

	var retErr error
	for _, tg := range tgsToDelete {
		modelStatus := model.TargetGroupStatus{
			Name: aws.StringValue(tg.getTargetGroupOutput.Name),
			Arn:  aws.StringValue(tg.getTargetGroupOutput.Arn),
			Id:   aws.StringValue(tg.getTargetGroupOutput.Id),
		}
		modelTg := model.TargetGroup{
			Status:    &modelStatus,
			IsDeleted: true,
		}

		err := t.targetGroupManager.Delete(ctx, &modelTg)
		if err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("failed TargetGroupManager.Delete %s due to %s", modelStatus.Id, err))
		}
	}

	if retErr != nil {
		return retErr
	}

	return nil
}

func (t *TargetGroupSynthesizer) calculateTargetGroupsToDelete(ctx context.Context) ([]tgListOutput, error) {
	latticeTgs, err := t.targetGroupManager.List(ctx)
	if err != nil {
		return latticeTgs, fmt.Errorf("failed TargetGroupManager.List due to %s", err)
	}

	var tgsToDelete []tgListOutput

	// we check existing target groups to see if they are still in use - this is necessary as
	// some changes to existing service exports or routes will simply create new target groups,
	// for example on protocol changes
	for _, latticeTg := range latticeTgs {
		if !t.hasTags(latticeTg) || !t.vpcMatchesConfig(latticeTg) {
			continue
		}

		// TGs from earlier releases will require 1-time manual cleanup
		// this method of validation only covers TGs created by this build
		// of the controller forward
		tagFields := model.TGTagFieldsFromTags(latticeTg.targetGroupTags.Tags)
		if !t.hasExpectedTags(latticeTg, tagFields) {
			continue
		}

		// most importantly, is the tg in use?
		if len(latticeTg.getTargetGroupOutput.ServiceArns) > 0 {
			t.log.Debugf("TargetGroup %s (%s) is referenced by lattice service",
				*latticeTg.getTargetGroupOutput.Arn, *latticeTg.getTargetGroupOutput.Name)
			continue
		}

		if tagFields.K8SSourceType == model.SourceTypeSvcExport {
			if t.shouldDeleteSvcExportTg(ctx, latticeTg, tagFields) {
				tgsToDelete = append(tgsToDelete, latticeTg)
			}
		} else {
			if t.shouldDeleteRouteTg(ctx, latticeTg, tagFields) {
				tgsToDelete = append(tgsToDelete, latticeTg)
			}
		}
	}
	return tgsToDelete, nil
}

func (t *TargetGroupSynthesizer) shouldDeleteSvcExportTg(
	ctx context.Context, latticeTg tgListOutput, tagFields model.TargetGroupTagFields) bool {

	svcExportName := types.NamespacedName{
		Namespace: tagFields.K8SServiceNamespace,
		Name:      tagFields.K8SServiceName,
	}

	t.log.Debugf("TargetGroup %s (%s) is referenced by ServiceExport",
		*latticeTg.getTargetGroupOutput.Arn, *latticeTg.getTargetGroupOutput.Name)

	svcExport := &mcsv1alpha1.ServiceExport{}
	err := t.client.Get(ctx, svcExportName, svcExport)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// if the service export does not exist, we can safely delete
			t.log.Infof("Will delete TargetGroup %s (%s) - ServiceExport is not found",
				*latticeTg.getTargetGroupOutput.Arn, *latticeTg.getTargetGroupOutput.Name)
			return true
		} else {
			// skip if we have an unknown error
			t.log.Infof("Received unexpected API error getting service export %s", err)
			return false
		}
	}

	if !svcExport.DeletionTimestamp.IsZero() {
		// backing object is deleted, we can delete too
		t.log.Infof("Will delete TargetGroup %s (%s) - ServiceExport has been deleted",
			*latticeTg.getTargetGroupOutput.Arn, *latticeTg.getTargetGroupOutput.Name)
		return true
	}

	// now we get to the tricky business of seeing if our unused target group actually matches
	// the current state of the service and service export - the most correct way to do this is to
	// reconstruct the target group spec from the service export itself, then compare fields
	modelTg, err := t.svcExportTgBuilder.BuildTargetGroup(ctx, svcExport)
	if err != nil {
		t.log.Infof("Received error building svc export target group model %s", err)
		return false
	}

	// tags are already validated, just need to check the other essentials
	ltg := latticeTg.getTargetGroupOutput
	if int64(modelTg.Spec.Port) != aws.Int64Value(ltg.Config.Port) ||
		modelTg.Spec.Protocol != aws.StringValue(ltg.Config.Protocol) ||
		modelTg.Spec.ProtocolVersion != aws.StringValue(ltg.Config.ProtocolVersion) ||
		modelTg.Spec.IpAddressType != aws.StringValue(ltg.Config.IpAddressType) {

		// one or more immutable fields differ from the source, so the TG is out of date
		t.log.Infof("Will delete TargetGroup %s (%s) - fields differ from source service/service export",
			*latticeTg.getTargetGroupOutput.Arn, *latticeTg.getTargetGroupOutput.Name)
		return true
	}

	t.log.Debugf("ServiceExport TargetGroup %s (%s) is up to date",
		*latticeTg.getTargetGroupOutput.Arn, *latticeTg.getTargetGroupOutput.Name)

	return false
}

func (t *TargetGroupSynthesizer) shouldDeleteRouteTg(
	ctx context.Context, latticeTg tgListOutput, tagFields model.TargetGroupTagFields) bool {

	routeName := types.NamespacedName{
		Namespace: tagFields.K8SRouteNamespace,
		Name:      tagFields.K8SRouteName,
	}

	var err error
	var route core.Route
	if *latticeTg.getTargetGroupOutput.Config.ProtocolVersion == vpclattice.TargetGroupProtocolVersionGrpc {
		route, err = core.GetGRPCRoute(ctx, t.client, routeName)
	} else {
		route, err = core.GetHTTPRoute(ctx, t.client, routeName)
	}

	if err != nil {
		if apierrors.IsNotFound(err) {
			// if the route does not exist, we can safely delete
			t.log.Debugf("Will delete TargetGroup %s (%s) - Route is not found",
				*latticeTg.getTargetGroupOutput.Arn, *latticeTg.getTargetGroupOutput.Name)
			return true
		} else {
			// skip if we have an unknown error
			t.log.Infof("Received unexpected API error getting route %s", err)
			return false
		}
	}

	if !route.DeletionTimestamp().IsZero() {
		t.log.Debugf("Will delete TargetGroup %s (%s) - Route is deleted",
			*latticeTg.getTargetGroupOutput.Arn, *latticeTg.getTargetGroupOutput.Name)
		return true
	}

	// basically rebuild everything for the route and see if one of the TGs matches
	routeStack, err := t.svcBuilder.Build(ctx, route)
	if err != nil {
		t.log.Infof("Received error building route model %s", err)
		return false
	}

	var resTargetGroups []*model.TargetGroup
	err = routeStack.ListResources(&resTargetGroups)
	if err != nil {
		t.log.Infof("Error listing stack target groups %s", err)
		return false
	}

	var matchFound bool
	for _, modelTg := range resTargetGroups {
		ltg := latticeTg.getTargetGroupOutput
		latticeTgSummary := vpclattice.TargetGroupSummary{
			Arn:           ltg.Arn,
			CreatedAt:     ltg.CreatedAt,
			Id:            ltg.Id,
			IpAddressType: ltg.Config.IpAddressType,
			LastUpdatedAt: ltg.LastUpdatedAt,
			Name:          ltg.Name,
			Port:          ltg.Config.Port,
			Protocol:      ltg.Config.Protocol,
			ServiceArns:   ltg.ServiceArns,
			Status:        ltg.Status,
			Type:          ltg.Type,
			VpcIdentifier: ltg.Config.VpcIdentifier,
		}

		match, err := t.targetGroupManager.IsTargetGroupMatch(ctx, modelTg, &latticeTgSummary, &tagFields)
		if err != nil {
			t.log.Infof("Received error during tg comparison %s", err)
			continue
		}

		if match {
			t.log.Debugf("Route TargetGroup %s (%s) is up to date",
				*latticeTg.getTargetGroupOutput.Arn, *latticeTg.getTargetGroupOutput.Name)

			matchFound = true
			break
		}
	}

	if !matchFound {
		t.log.Debugf("Will delete TargetGroup %s (%s) - TG is not up to date",
			*latticeTg.getTargetGroupOutput.Arn, *latticeTg.getTargetGroupOutput.Name)

		return true // safe to delete
	}

	// here we just delete anything more than X minutes old - worst case we'll have to recreate
	// the target group - note this case is only theoretically possible at this point
	fiveMinsAgo := time.Now().Add(-time.Minute * 5)
	if fiveMinsAgo.After(aws.TimeValue(latticeTg.getTargetGroupOutput.CreatedAt)) {
		t.log.Debugf("Will delete TargetGroup %s (%s) - TG is more than 5 minutes old",
			*latticeTg.getTargetGroupOutput.Arn, *latticeTg.getTargetGroupOutput.Name)
		return true
	}

	return false
}

func (t *TargetGroupSynthesizer) hasTags(latticeTg tgListOutput) bool {
	if latticeTg.targetGroupTags == nil {
		t.log.Debugf("Ignoring target group %s (%s) because tag fetch was not successful",
			*latticeTg.getTargetGroupOutput.Arn, *latticeTg.getTargetGroupOutput.Name)
		return false
	}
	return true
}

func (t *TargetGroupSynthesizer) vpcMatchesConfig(latticeTg tgListOutput) bool {
	if aws.StringValue(latticeTg.getTargetGroupOutput.Config.VpcIdentifier) != config.VpcID {
		t.log.Debugf("Ignoring target group %s (%s) because it is not configured for this VPC",
			*latticeTg.getTargetGroupOutput.Arn, *latticeTg.getTargetGroupOutput.Name)
		return false
	}
	return true
}

func (t *TargetGroupSynthesizer) hasExpectedTags(latticeTg tgListOutput, tagFields model.TargetGroupTagFields) bool {
	if tagFields.K8SClusterName != config.ClusterName {
		t.log.Debugf("Ignoring target group %s (%s) because it is not configured for this Cluster",
			*latticeTg.getTargetGroupOutput.Arn, *latticeTg.getTargetGroupOutput.Name)
		return false
	}

	if tagFields.K8SSourceType == model.SourceTypeInvalid ||
		tagFields.K8SServiceName == "" || tagFields.K8SServiceNamespace == "" {

		t.log.Infof("Ignoring target group %s (%s) as one or more required tags are missing",
			*latticeTg.getTargetGroupOutput.Arn, *latticeTg.getTargetGroupOutput.Name)
		return false
	}

	// route-based TGs should have the additional route keys
	if tagFields.IsSourceTypeRoute() && (tagFields.K8SRouteName == "" || tagFields.K8SRouteNamespace == "") {
		t.log.Infof("Ignoring route-based target group %s (%s) as one or more required tags are missing",
			*latticeTg.getTargetGroupOutput.Arn, *latticeTg.getTargetGroupOutput.Name)
		return false
	}

	return true
}

func (t *TargetGroupSynthesizer) PostSynthesize(ctx context.Context) error {
	// nothing to do here
	return nil
}
