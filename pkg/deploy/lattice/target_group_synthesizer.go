package lattice

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/gateway"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

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

		fmt.Printf("liwwu >> tg SynthesizeCreate %v \n", resTargetGroup)
		tgStatus, err := t.targetGroupManager.Upsert(ctx, resTargetGroup)

		fmt.Printf("liwwu >> tgStatus %v\n", tgStatus)
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

// result of deletion attempt, if err is nil target group was deleted
type DeleteUnusedResult struct {
	Arn string
	Err error
}

// This method assumes all synthesis. Returns list of deletion results, might include partial
// failures if cannot produce list for deletion will return error.
//
// TODO: we should do parallel deletion calls, preferably with bounded WorkGroup
func (t *TargetGroupSynthesizer) SynthesizeUnusedDelete(ctx context.Context) ([]DeleteUnusedResult, error) {
	tgsToDelete, err := t.calculateTargetGroupsToDelete(ctx)
	if err != nil {
		return nil, err
	}

	results := make([]DeleteUnusedResult, len(tgsToDelete))

	for i, tg := range tgsToDelete {
		modelStatus := model.TargetGroupStatus{
			Name: aws.StringValue(tg.tgSummary.Name),
			Arn:  aws.StringValue(tg.tgSummary.Arn),
			Id:   aws.StringValue(tg.tgSummary.Id),
		}
		modelTg := model.TargetGroup{
			Status:    &modelStatus,
			IsDeleted: true,
		}

		err := t.targetGroupManager.Delete(ctx, &modelTg)
		results[i] = DeleteUnusedResult{
			Arn: modelTg.Status.Arn,
			Err: err,
		}
	}

	return results, nil
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
		tagFields := model.TGTagFieldsFromTags(latticeTg.tags)
		if !t.hasExpectedTags(latticeTg, tagFields) {
			continue
		}

		// most importantly, is the tg in use?
		if len(latticeTg.tgSummary.ServiceArns) > 0 {
			t.log.Debugf("TargetGroup %s (%s) is referenced by lattice service",
				*latticeTg.tgSummary.Arn, *latticeTg.tgSummary.Name)
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
		*latticeTg.tgSummary.Arn, *latticeTg.tgSummary.Name)

	svcExport := &anv1alpha1.ServiceExport{}
	err := t.client.Get(ctx, svcExportName, svcExport)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// if the service export does not exist, we can safely delete
			t.log.Infof("Will delete TargetGroup %s (%s) - ServiceExport is not found",
				*latticeTg.tgSummary.Arn, *latticeTg.tgSummary.Name)
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
			*latticeTg.tgSummary.Arn, *latticeTg.tgSummary.Name)
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

	// the main identifiers are validated, just need to check the other essentials.
	// protocolVersion is not in TG summary so we are bringing it from tags.
	if int64(modelTg.Spec.Port) != aws.Int64Value(latticeTg.tgSummary.Port) ||
		modelTg.Spec.Protocol != aws.StringValue(latticeTg.tgSummary.Protocol) ||
		modelTg.Spec.ProtocolVersion != tagFields.K8SProtocolVersion ||
		modelTg.Spec.IpAddressType != aws.StringValue(latticeTg.tgSummary.IpAddressType) {

		// one or more immutable fields differ from the source, so the TG is out of date
		t.log.Infof("Will delete TargetGroup %s (%s) - fields differ from source service/service export",
			*latticeTg.tgSummary.Arn, *latticeTg.tgSummary.Name)
		return true
	}

	t.log.Debugf("ServiceExport TargetGroup %s (%s) is up to date",
		*latticeTg.tgSummary.Arn, *latticeTg.tgSummary.Name)

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
	if tagFields.K8SProtocolVersion == vpclattice.TargetGroupProtocolVersionGrpc {
		route, err = core.GetGRPCRoute(ctx, t.client, routeName)
	} else {
		route, err = core.GetHTTPRoute(ctx, t.client, routeName)
	}

	if err != nil {
		if apierrors.IsNotFound(err) {
			// if the route does not exist, we can safely delete
			t.log.Debugf("Will delete TargetGroup %s (%s) - Route is not found",
				*latticeTg.tgSummary.Arn, *latticeTg.tgSummary.Name)
			return true
		} else {
			// skip if we have an unknown error
			t.log.Infof("Received unexpected API error getting route %s", err)
			return false
		}
	}

	if !route.DeletionTimestamp().IsZero() {
		t.log.Debugf("Will delete TargetGroup %s (%s) - Route is deleted",
			*latticeTg.tgSummary.Arn, *latticeTg.tgSummary.Name)
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
		match, err := t.targetGroupManager.IsTargetGroupMatch(ctx, modelTg, latticeTg.tgSummary, &tagFields)
		if err != nil {
			t.log.Infof("Received error during tg comparison %s", err)
			continue
		}

		if match {
			t.log.Debugf("Route TargetGroup %s (%s) is up to date",
				*latticeTg.tgSummary.Arn, *latticeTg.tgSummary.Name)

			matchFound = true
			break
		}
	}

	if !matchFound {
		t.log.Debugf("Will delete TargetGroup %s (%s) - TG is not up to date",
			*latticeTg.tgSummary.Arn, *latticeTg.tgSummary.Name)

		return true // safe to delete
	}

	return false
}

func (t *TargetGroupSynthesizer) hasTags(latticeTg tgListOutput) bool {
	if latticeTg.tags == nil {
		t.log.Debugf("Ignoring target group %s (%s) because tag fetch was not successful",
			*latticeTg.tgSummary.Arn, *latticeTg.tgSummary.Name)
		return false
	}
	return true
}

func (t *TargetGroupSynthesizer) vpcMatchesConfig(latticeTg tgListOutput) bool {
	if aws.StringValue(latticeTg.tgSummary.VpcIdentifier) != config.VpcID {
		t.log.Debugf("Ignoring target group %s (%s) because it is not configured for this VPC",
			*latticeTg.tgSummary.Arn, *latticeTg.tgSummary.Name)
		return false
	}
	return true
}

func (t *TargetGroupSynthesizer) hasExpectedTags(latticeTg tgListOutput, tagFields model.TargetGroupTagFields) bool {
	if tagFields.K8SClusterName != config.ClusterName {
		t.log.Debugf("Ignoring target group %s (%s) because it is not configured for this Cluster",
			*latticeTg.tgSummary.Arn, *latticeTg.tgSummary.Name)
		return false
	}

	if tagFields.K8SSourceType == model.SourceTypeInvalid ||
		tagFields.K8SServiceName == "" || tagFields.K8SServiceNamespace == "" {

		t.log.Infof("Ignoring target group %s (%s) as one or more required tags are missing",
			*latticeTg.tgSummary.Arn, *latticeTg.tgSummary.Name)
		return false
	}

	// route-based TGs should have the additional route keys
	if tagFields.IsSourceTypeRoute() && (tagFields.K8SRouteName == "" || tagFields.K8SRouteNamespace == "") {
		t.log.Infof("Ignoring route-based target group %s (%s) as one or more required tags are missing",
			*latticeTg.tgSummary.Arn, *latticeTg.tgSummary.Name)
		return false
	}

	return true
}

func (t *TargetGroupSynthesizer) PostSynthesize(ctx context.Context) error {
	// nothing to do here
	return nil
}
