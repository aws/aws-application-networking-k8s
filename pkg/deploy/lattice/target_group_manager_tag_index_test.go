package lattice

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/vpclattice"
	"github.com/aws/aws-sdk-go-v2/service/vpclattice/types"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	mocks "github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	lattice_runtime "github.com/aws/aws-application-networking-k8s/pkg/runtime"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

// These tests cover findTargetGroup with the Resource Groups Tagging API
// (DISABLE_TAGGING_SERVICE_API=false). Its index is eventually consistent and
// can omit a target group for a while after it is created; a miss there must
// not lead to a duplicate CreateTargetGroup.
// https://github.com/aws/aws-application-networking-k8s/issues/698

func tagIndexTestTg() *model.TargetGroup {
	spec := model.TargetGroupSpec{
		VpcId:           TestCloudConfig.VpcId,
		Type:            model.TargetGroupTypeIP,
		Port:            8080,
		Protocol:        string(types.TargetGroupProtocolHttp),
		ProtocolVersion: string(types.TargetGroupProtocolVersionHttp1),
		IpAddressType:   string(types.IpAddressTypeIpv4),
	}
	spec.K8SClusterName = TestCloudConfig.ClusterName
	spec.K8SSourceType = model.SourceTypeHTTPRoute
	spec.K8SServiceName = "backend"
	spec.K8SServiceNamespace = "ns1"
	spec.K8SRouteName = "route1"
	spec.K8SRouteNamespace = "ns1"
	spec.K8SProtocolVersion = string(types.TargetGroupProtocolVersionHttp1)
	return &model.TargetGroup{Spec: spec}
}

func tagIndexTestSummary(tg *model.TargetGroup, id string) types.TargetGroupSummary {
	return types.TargetGroupSummary{
		Arn:           aws.String("arn:" + id),
		Id:            aws.String(id),
		Name:          aws.String(model.GenerateTgName(tg.Spec)),
		VpcIdentifier: aws.String(tg.Spec.VpcId),
		Port:          aws.Int32(tg.Spec.Port),
		Protocol:      types.TargetGroupProtocol(tg.Spec.Protocol),
		IpAddressType: types.IpAddressType(tg.Spec.IpAddressType),
		Type:          types.TargetGroupType(tg.Spec.Type),
		Status:        types.TargetGroupStatusActive,
	}
}

func tagIndexTestGetOutput(sum types.TargetGroupSummary, status types.TargetGroupStatus) *vpclattice.GetTargetGroupOutput {
	return &vpclattice.GetTargetGroupOutput{
		Arn:    sum.Arn,
		Id:     sum.Id,
		Name:   sum.Name,
		Status: status,
		Type:   sum.Type,
		Config: &types.TargetGroupConfig{
			Port:          sum.Port,
			Protocol:      sum.Protocol,
			IpAddressType: sum.IpAddressType,
			VpcIdentifier: sum.VpcIdentifier,
		},
	}
}

func tagIndexTestTags(tg *model.TargetGroup) map[string]string {
	return model.TagsFromTGTagFields(tg.Spec.TargetGroupTagFields)
}

func tagIndexTestCreateOutput() *vpclattice.CreateTargetGroupOutput {
	return &vpclattice.CreateTargetGroupOutput{
		Arn:    aws.String("arn:tg-new"),
		Id:     aws.String("tg-new"),
		Name:   aws.String("new"),
		Status: types.TargetGroupStatusActive,
	}
}

func TestFindTargetGroup_TagIndexMiss_UsesTargetGroupFromLattice(t *testing.T) {
	routeTg := tagIndexTestTg()

	svcExportTg := tagIndexTestTg()
	svcExportTg.Spec.K8SSourceType = model.SourceTypeSvcExport
	svcExportTg.Spec.K8SRouteName = ""
	svcExportTg.Spec.K8SRouteNamespace = ""

	for name, tg := range map[string]*model.TargetGroup{"HTTPRoute": routeTg, "ServiceExport": svcExportTg} {
		t.Run(name, func(t *testing.T) {
			c := gomock.NewController(t)
			ctx := context.TODO()
			mockLattice := mocks.NewMockLattice(c)
			mockTagging := mocks.NewMockTagging(c)
			cloud := pkg_aws.NewDefaultCloudWithTagging(mockLattice, mockTagging, TestCloudConfig)

			existing := tagIndexTestSummary(tg, "tg-existing")

			// The tag index has not caught up with the target group created by the previous reconcile.
			mockTagging.EXPECT().FindResourcesByTags(ctx, mocks.ResourceTypeTargetGroup, tagIndexTestTags(tg)).Return(nil, nil)
			mockLattice.EXPECT().ListTargetGroupsAsList(ctx, &vpclattice.ListTargetGroupsInput{
				VpcIdentifier: aws.String(tg.Spec.VpcId),
			}).Return([]types.TargetGroupSummary{existing}, nil)
			mockLattice.EXPECT().ListTagsForResource(ctx, &vpclattice.ListTagsForResourceInput{
				ResourceArn: existing.Arn,
			}).Return(&vpclattice.ListTagsForResourceOutput{Tags: tagIndexTestTags(tg)}, nil)
			mockLattice.EXPECT().GetTargetGroup(ctx, &vpclattice.GetTargetGroupInput{TargetGroupIdentifier: existing.Arn}).
				Return(tagIndexTestGetOutput(existing, types.TargetGroupStatusActive), nil)
			// update path; no CreateTargetGroup is expected, so a create fails the test
			mockTagging.EXPECT().UpdateTags(ctx, aws.ToString(existing.Arn), gomock.Any(), nil).Return(nil)
			mockLattice.EXPECT().UpdateTargetGroup(ctx, gomock.Any()).Return(&vpclattice.UpdateTargetGroupOutput{}, nil)

			status, err := NewTargetGroupManager(gwlog.FallbackLogger, cloud, nil).Upsert(ctx, tg)
			assert.NoError(t, err)
			assert.Equal(t, "tg-existing", status.Id)
			assert.Equal(t, aws.ToString(existing.Arn), status.Arn)
		})
	}
}

func TestFindTargetGroup_TagIndexMiss_RejectsNonMatchingLatticeTargetGroups(t *testing.T) {
	c := gomock.NewController(t)
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	mockTagging := mocks.NewMockTagging(c)
	cloud := pkg_aws.NewDefaultCloudWithTagging(mockLattice, mockTagging, TestCloudConfig)

	tg := tagIndexTestTg()

	otherName := tagIndexTestSummary(tg, "tg-other-name")
	otherName.Name = aws.String("k8s-ns1-other-abcdefghij")

	otherPort := tagIndexTestSummary(tg, "tg-other-port")
	otherPort.Port = aws.Int32(9090)

	otherRoute := tagIndexTestSummary(tg, "tg-other-route")
	otherRouteTg := tagIndexTestTg()
	otherRouteTg.Spec.K8SRouteName = "route2"

	gone := tagIndexTestSummary(tg, "tg-gone")

	mockTagging.EXPECT().FindResourcesByTags(ctx, gomock.Any(), gomock.Any()).Return(nil, nil)
	mockLattice.EXPECT().ListTargetGroupsAsList(ctx, gomock.Any()).
		Return([]types.TargetGroupSummary{otherName, otherPort, otherRoute, gone}, nil)
	// Tags are read only for target groups whose name prefix and immutable fields match.
	mockLattice.EXPECT().ListTagsForResource(ctx, &vpclattice.ListTagsForResourceInput{
		ResourceArn: otherRoute.Arn,
	}).Return(&vpclattice.ListTagsForResourceOutput{Tags: tagIndexTestTags(otherRouteTg)}, nil)
	mockLattice.EXPECT().ListTagsForResource(ctx, &vpclattice.ListTagsForResourceInput{
		ResourceArn: gone.Arn,
	}).Return(nil, &types.ResourceNotFoundException{Message: aws.String("gone")})
	mockLattice.EXPECT().CreateTargetGroup(ctx, gomock.Any()).Return(tagIndexTestCreateOutput(), nil)

	status, err := NewTargetGroupManager(gwlog.FallbackLogger, cloud, nil).Upsert(ctx, tg)
	assert.NoError(t, err)
	assert.Equal(t, "tg-new", status.Id)
}

func TestFindTargetGroup_TagIndexMiss_LatticeTargetGroupCreating_Retries(t *testing.T) {
	c := gomock.NewController(t)
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	mockTagging := mocks.NewMockTagging(c)
	cloud := pkg_aws.NewDefaultCloudWithTagging(mockLattice, mockTagging, TestCloudConfig)

	tg := tagIndexTestTg()
	creating := tagIndexTestSummary(tg, "tg-creating")
	creating.Status = types.TargetGroupStatusCreateInProgress

	mockTagging.EXPECT().FindResourcesByTags(ctx, gomock.Any(), gomock.Any()).Return(nil, nil)
	mockLattice.EXPECT().ListTargetGroupsAsList(ctx, gomock.Any()).Return([]types.TargetGroupSummary{creating}, nil)
	mockLattice.EXPECT().ListTagsForResource(ctx, gomock.Any()).
		Return(&vpclattice.ListTagsForResourceOutput{Tags: tagIndexTestTags(tg)}, nil)
	mockLattice.EXPECT().GetTargetGroup(ctx, gomock.Any()).
		Return(tagIndexTestGetOutput(creating, types.TargetGroupStatusCreateInProgress), nil)

	_, err := NewTargetGroupManager(gwlog.FallbackLogger, cloud, nil).Upsert(ctx, tg)
	var retry *lattice_runtime.RequeueNeededAfter
	assert.True(t, errors.As(err, &retry), "expected a retry, got %v", err)
}

func TestFindTargetGroup_TagIndexMiss_LatticeListError_DoesNotCreate(t *testing.T) {
	c := gomock.NewController(t)
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	mockTagging := mocks.NewMockTagging(c)
	cloud := pkg_aws.NewDefaultCloudWithTagging(mockLattice, mockTagging, TestCloudConfig)

	mockTagging.EXPECT().FindResourcesByTags(ctx, gomock.Any(), gomock.Any()).Return(nil, nil)
	mockLattice.EXPECT().ListTargetGroupsAsList(ctx, gomock.Any()).Return(nil, errors.New("throttled"))

	_, err := NewTargetGroupManager(gwlog.FallbackLogger, cloud, nil).Upsert(ctx, tagIndexTestTg())
	assert.ErrorContains(t, err, "throttled")
}

func TestFindTargetGroup_TagIndexReturnsOnlyFailedTg_SkipsItInLattice(t *testing.T) {
	c := gomock.NewController(t)
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	mockTagging := mocks.NewMockTagging(c)
	cloud := pkg_aws.NewDefaultCloudWithTagging(mockLattice, mockTagging, TestCloudConfig)

	tg := tagIndexTestTg()
	failed := tagIndexTestSummary(tg, "tg-failed")
	failed.Status = types.TargetGroupStatusCreateFailed
	fresh := tagIndexTestSummary(tg, "tg-fresh")

	mockTagging.EXPECT().FindResourcesByTags(ctx, gomock.Any(), gomock.Any()).Return([]string{aws.ToString(failed.Arn)}, nil)
	mockLattice.EXPECT().GetTargetGroup(ctx, &vpclattice.GetTargetGroupInput{TargetGroupIdentifier: failed.Arn}).
		Return(tagIndexTestGetOutput(failed, types.TargetGroupStatusCreateFailed), nil)
	mockLattice.EXPECT().ListTargetGroupsAsList(ctx, gomock.Any()).Return([]types.TargetGroupSummary{failed, fresh}, nil)
	// the failed target group was already checked, so only the fresh one is read
	mockLattice.EXPECT().ListTagsForResource(ctx, &vpclattice.ListTagsForResourceInput{ResourceArn: fresh.Arn}).
		Return(&vpclattice.ListTagsForResourceOutput{Tags: tagIndexTestTags(tg)}, nil)
	mockLattice.EXPECT().GetTargetGroup(ctx, &vpclattice.GetTargetGroupInput{TargetGroupIdentifier: fresh.Arn}).
		Return(tagIndexTestGetOutput(fresh, types.TargetGroupStatusActive), nil)
	mockTagging.EXPECT().UpdateTags(ctx, aws.ToString(fresh.Arn), gomock.Any(), nil).Return(nil)
	mockLattice.EXPECT().UpdateTargetGroup(ctx, gomock.Any()).Return(&vpclattice.UpdateTargetGroupOutput{}, nil)

	status, err := NewTargetGroupManager(gwlog.FallbackLogger, cloud, nil).Upsert(ctx, tg)
	assert.NoError(t, err)
	assert.Equal(t, "tg-fresh", status.Id)
}

func TestFindTargetGroup_TaggingAPIDisabled_NoSecondLookup(t *testing.T) {
	c := gomock.NewController(t)
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	mockTagging := mocks.NewMockTagging(c)
	cfg := TestCloudConfig
	cfg.TaggingServiceAPIDisabled = true
	cloud := pkg_aws.NewDefaultCloudWithTagging(mockLattice, mockTagging, cfg)

	// With DISABLE_TAGGING_SERVICE_API=true the first lookup already reads
	// VPC Lattice, so a miss creates without a second listing.
	mockTagging.EXPECT().FindResourcesByTags(ctx, gomock.Any(), gomock.Any()).Return(nil, nil)
	mockLattice.EXPECT().CreateTargetGroup(ctx, gomock.Any()).Return(tagIndexTestCreateOutput(), nil)

	status, err := NewTargetGroupManager(gwlog.FallbackLogger, cloud, nil).Upsert(ctx, tagIndexTestTg())
	assert.NoError(t, err)
	assert.Equal(t, "tg-new", status.Id)
}

func TestDeleteTargetGroup_TagIndexMiss_DeletesTargetGroupFoundInLattice(t *testing.T) {
	c := gomock.NewController(t)
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	mockTagging := mocks.NewMockTagging(c)
	cloud := pkg_aws.NewDefaultCloudWithTagging(mockLattice, mockTagging, TestCloudConfig)

	tg := tagIndexTestTg()
	tg.IsDeleted = true
	existing := tagIndexTestSummary(tg, "tg-existing")

	mockTagging.EXPECT().FindResourcesByTags(ctx, gomock.Any(), gomock.Any()).Return(nil, nil)
	mockLattice.EXPECT().ListTargetGroupsAsList(ctx, gomock.Any()).Return([]types.TargetGroupSummary{existing}, nil)
	mockLattice.EXPECT().ListTagsForResource(ctx, gomock.Any()).
		Return(&vpclattice.ListTagsForResourceOutput{Tags: tagIndexTestTags(tg)}, nil)
	mockLattice.EXPECT().GetTargetGroup(ctx, gomock.Any()).
		Return(tagIndexTestGetOutput(existing, types.TargetGroupStatusActive), nil)
	mockLattice.EXPECT().ListTargetsAsList(ctx, &vpclattice.ListTargetsInput{TargetGroupIdentifier: existing.Id}).Return(nil, nil)
	mockLattice.EXPECT().DeleteTargetGroup(ctx, &vpclattice.DeleteTargetGroupInput{
		TargetGroupIdentifier: existing.Id,
	}).Return(&vpclattice.DeleteTargetGroupOutput{}, nil)

	assert.NoError(t, NewTargetGroupManager(gwlog.FallbackLogger, cloud, nil).Delete(ctx, tg))
}
