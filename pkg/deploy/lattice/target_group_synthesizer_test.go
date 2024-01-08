package lattice

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mock_client "github.com/aws/aws-application-networking-k8s/mocks/controller-runtime/client"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/gateway"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

func Test_Synthesize(t *testing.T) {
	// all synthesize does is delegate to the manager
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockTGManager := NewMockTargetGroupManager(c)

	stack := core.NewDefaultStack(core.StackID{Name: "foo", Namespace: "bar"})
	tgToDelete := &model.TargetGroup{
		ResourceMeta: core.NewResourceMeta(stack, "AWS:VPCServiceNetwork::TargetGroup", "tg-delete"),
		IsDeleted:    true,
	}
	tgToCreate := &model.TargetGroup{
		ResourceMeta: core.NewResourceMeta(stack, "AWS:VPCServiceNetwork::TargetGroup", "tg-create"),
		IsDeleted:    false,
	}
	assert.NoError(t, stack.AddResource(tgToDelete))
	assert.NoError(t, stack.AddResource(tgToCreate))

	mockTGManager.EXPECT().Delete(ctx, tgToDelete).Return(nil)
	mockTGManager.EXPECT().Upsert(ctx, tgToCreate).Return(model.TargetGroupStatus{Name: "create-name"}, nil)

	synthesizer := NewTargetGroupSynthesizer(gwlog.FallbackLogger, nil, nil, mockTGManager, nil, nil, stack)

	err := synthesizer.Synthesize(ctx)
	assert.Nil(t, err)
	assert.Equal(t, "create-name", tgToCreate.Status.Name)
}

func copy(src tgListOutput) tgListOutput {
	srcSummary := src.tgSummary
	cp := tgListOutput{
		tgSummary: &vpclattice.TargetGroupSummary{
			Arn:           aws.String(aws.StringValue(srcSummary.Arn)),
			Id:            aws.String(aws.StringValue(srcSummary.Id)),
			Name:          aws.String(aws.StringValue(srcSummary.Name)),
			Type:          aws.String(aws.StringValue(srcSummary.Type)),
			CreatedAt:     aws.Time(aws.TimeValue(srcSummary.CreatedAt)),
			IpAddressType: aws.String(aws.StringValue(srcSummary.IpAddressType)),
			Port:          aws.Int64(aws.Int64Value(srcSummary.Port)),
			Protocol:      aws.String(aws.StringValue(srcSummary.Protocol)),
			VpcIdentifier: aws.String(aws.StringValue(srcSummary.VpcIdentifier)),
		},
	}

	srctags := src.tags
	if srctags != nil {
		cp.tags = make(map[string]*string)
		for k, v := range srctags {
			cp.tags[k] = aws.String(aws.StringValue(v))
		}
	}

	return cp
}

// we have a list of target groups with varying properties
// TGs that are not managed by the controller
//   - tag fetch was unsuccessful (tags nil)
//   - vpc id does not match
//   - cluster name does not match
//   - parent ref type is invalid
//   - K8SServiceName missing
//   - K8SServiceNamespace missing
//   - parent ref type is a route, but K8SRouteName missing
//   - parent ref type is a route, but K8SRouteNamespace missing
func Test_SynthesizeUnusedDeleteIgnoreNotManagedByController(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockTGManager := NewMockTargetGroupManager(c)

	config.VpcID = "vpc-id"
	config.ClusterName = "cluster-name"

	var nonManagedTgs []tgListOutput

	tgTagFetchUnsuccessful := tgListOutput{
		tgSummary: &vpclattice.TargetGroupSummary{
			Arn:  aws.String("tg-arn"),
			Id:   aws.String("tg-id"),
			Name: aws.String("tg-name"),
			Type: aws.String("IP"),
		},
		tags: nil,
	}
	nonManagedTgs = append(nonManagedTgs, tgTagFetchUnsuccessful)

	tgWrongVpc := copy(tgTagFetchUnsuccessful)
	tgWrongVpc.tags = make(map[string]*string)
	tgWrongVpc.tgSummary.VpcIdentifier = aws.String("another-vpc")
	nonManagedTgs = append(nonManagedTgs, tgWrongVpc)

	tgWrongCluster := copy(tgWrongVpc)
	tgWrongCluster.tgSummary.VpcIdentifier = aws.String("vpc-id")
	tgWrongCluster.tags[model.K8SClusterNameKey] = aws.String("another-cluster")
	nonManagedTgs = append(nonManagedTgs, tgWrongCluster)

	tgInvalidParentRef := copy(tgWrongCluster)
	tgInvalidParentRef.tags[model.K8SClusterNameKey] = aws.String("cluster-name")
	tgInvalidParentRef.tags[model.K8SSourceTypeKey] = aws.String(string(model.SourceTypeInvalid))
	nonManagedTgs = append(nonManagedTgs, tgInvalidParentRef)

	tgMissingK8SServiceName := copy(tgInvalidParentRef)
	tgMissingK8SServiceName.tags[model.K8SSourceTypeKey] = aws.String(string(model.SourceTypeSvcExport))
	nonManagedTgs = append(nonManagedTgs, tgMissingK8SServiceName)

	tgMissingK8SServiceNamespace := copy(tgMissingK8SServiceName)
	tgMissingK8SServiceNamespace.tags[model.K8SServiceNameKey] = aws.String("my-service")
	nonManagedTgs = append(nonManagedTgs, tgMissingK8SServiceNamespace)

	tgMissingRouteName := copy(tgMissingK8SServiceNamespace)
	tgMissingRouteName.tags[model.K8SSourceTypeKey] = aws.String(string(model.SourceTypeHTTPRoute))
	tgMissingRouteName.tags[model.K8SServiceNamespaceKey] = aws.String("ns-1")
	nonManagedTgs = append(nonManagedTgs, tgMissingRouteName)

	tgMissingRouteNamespace := copy(tgMissingRouteName)
	tgMissingRouteNamespace.tags[model.K8SRouteNameKey] = aws.String("route-name")
	nonManagedTgs = append(nonManagedTgs, tgMissingRouteNamespace)

	mockTGManager.EXPECT().List(ctx).Return(nonManagedTgs, nil)
	synthesizer := NewTargetGroupSynthesizer(gwlog.FallbackLogger, nil, nil, mockTGManager, nil, nil, nil)
	_, err := synthesizer.SynthesizeUnusedDelete(ctx)
	assert.Nil(t, err)
}

func getBaseTg() tgListOutput {
	baseTg := tgListOutput{
		tgSummary: &vpclattice.TargetGroupSummary{
			Arn:           aws.String("tg-arn"),
			Id:            aws.String("tg-id"),
			Name:          aws.String("tg-name"),
			CreatedAt:     aws.Time(time.Now()),
			VpcIdentifier: aws.String("vpc-id"),
			Port:          aws.Int64(80),
			Protocol:      aws.String("HTTP"),
			IpAddressType: aws.String("IPV4"),
		},
		tags: make(map[string]*string),
	}
	baseTg.tags[model.K8SClusterNameKey] = aws.String("cluster-name")
	baseTg.tags[model.K8SServiceNameKey] = aws.String("svc")
	baseTg.tags[model.K8SServiceNamespaceKey] = aws.String("ns")
	baseTg.tags[model.K8SProtocolVersionKey] = aws.String("HTTP")
	return baseTg
}

// Do not delete cases
// TG has service arns
// TG is service export
//   - port, protocol, protocolVersion, ipaddressType, all match
//
// TG is route
//   - TG matches a current TG for the route
func Test_DoNotDeleteCases(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockTGManager := NewMockTargetGroupManager(c)
	mockClient := mock_client.NewMockClient(c)
	mockSvcExportTgBuilder := gateway.NewMockSvcExportTargetGroupModelBuilder(c)
	mockSvcBuilder := gateway.NewMockLatticeServiceBuilder(c)

	config.VpcID = "vpc-id"
	config.ClusterName = "cluster-name"

	baseTg := getBaseTg()

	var noDeleteTgs []tgListOutput

	tgWithSvcArns := copy(baseTg)
	tgWithSvcArns.tgSummary.Arn = aws.String("tg-with-svcs-arn") // useful for reading logs
	tgWithSvcArns.tgSummary.ServiceArns = []*string{aws.String("svc-arn")}
	noDeleteTgs = append(noDeleteTgs, tgWithSvcArns)

	tgSvcExportUpToDate := copy(baseTg)
	tgSvcExportUpToDate.tgSummary.Arn = aws.String("tg-svc-export-arn")
	tgSvcExportUpToDate.tags[model.K8SSourceTypeKey] = aws.String(string(model.SourceTypeSvcExport))
	tgSvcExportUpToDate.tags[model.K8SProtocolVersionKey] = aws.String("HTTP1")
	noDeleteTgs = append(noDeleteTgs, tgSvcExportUpToDate)

	tgSvcUpToDate := copy(baseTg)
	tgSvcUpToDate.tgSummary.Arn = aws.String("tg-svc-arn")
	tgSvcUpToDate.tags[model.K8SSourceTypeKey] = aws.String(string(model.SourceTypeHTTPRoute))
	tgSvcUpToDate.tags[model.K8SRouteNameKey] = aws.String("route")
	tgSvcUpToDate.tags[model.K8SRouteNamespaceKey] = aws.String("route-ns")
	tgSvcUpToDate.tags[model.K8SProtocolVersionKey] = aws.String("HTTP1")
	noDeleteTgs = append(noDeleteTgs, tgSvcUpToDate)

	mockTGManager.EXPECT().List(ctx).Return(noDeleteTgs, nil)

	mockClient.EXPECT().Get(ctx, gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, routeOrSvcExport client.Object, _ ...interface{}) error {
			routeOrSvcExport.SetName("ignored-name")
			routeOrSvcExport.SetNamespace("ignored-ns")
			return nil
		},
	).AnyTimes()

	baseModelTg := model.TargetGroup{
		Spec: model.TargetGroupSpec{
			VpcId:           "vpc-id",
			Type:            "IP",
			Port:            80,
			Protocol:        "HTTP",
			ProtocolVersion: "HTTP1",
			IpAddressType:   "IPV4",
			TargetGroupTagFields: model.TargetGroupTagFields{
				K8SClusterName:      "cluster-name",
				K8SServiceName:      "svc",
				K8SServiceNamespace: "ns",
			},
		},
	}
	httpTg := baseModelTg
	grpcTg := baseModelTg
	grpcTg.Spec.Protocol = "HTTPS"
	grpcTg.Spec.ProtocolVersion = "GRPC"
	httpTg.Spec.TargetGroupTagFields.K8SSourceType = model.SourceTypeSvcExport

	mockSvcExportTgBuilder.EXPECT().BuildTargetGroups(ctx, gomock.Any()).Return(&httpTg, &grpcTg, nil)

	stack := core.NewDefaultStack(core.StackID{Name: "foo", Namespace: "bar"})
	svcModelTg := baseModelTg
	svcModelTg.ResourceMeta = core.NewResourceMeta(stack, "AWS:VPCServiceNetwork::TargetGroup", "tg-id")
	svcModelTg.Spec.TargetGroupTagFields.K8SSourceType = model.SourceTypeHTTPRoute
	svcModelTg.Spec.TargetGroupTagFields.K8SRouteName = "route"
	svcModelTg.Spec.TargetGroupTagFields.K8SRouteNamespace = "route-ns"
	stack.AddResource(&svcModelTg)

	mockTGManager.EXPECT().IsTargetGroupMatch(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)

	mockSvcBuilder.EXPECT().Build(ctx, gomock.Any()).Return(stack, nil)

	synthesizer := NewTargetGroupSynthesizer(
		gwlog.FallbackLogger, nil, mockClient, mockTGManager, mockSvcExportTgBuilder, mockSvcBuilder, stack)

	_, err := synthesizer.SynthesizeUnusedDelete(ctx)
	assert.Nil(t, err)
}

func Test_DeleteServiceExport_DeleteCases(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockTGManager := NewMockTargetGroupManager(c)
	mockClient := mock_client.NewMockClient(c)
	mockSvcExportTgBuilder := gateway.NewMockSvcExportTargetGroupModelBuilder(c)

	config.VpcID = "vpc-id"
	config.ClusterName = "cluster-name"

	baseTg := getBaseTg()

	var deleteTgs []tgListOutput
	tgSvcExport := copy(baseTg)
	tgSvcExport.tgSummary.Arn = aws.String("tg-svc-export-arn")
	tgSvcExport.tags[model.K8SSourceTypeKey] = aws.String(string(model.SourceTypeSvcExport))
	tgSvcExport.tags[model.K8SProtocolVersionKey] = aws.String("HTTP1")
	deleteTgs = append(deleteTgs, tgSvcExport)

	t.Run("Service Export does not exist", func(t *testing.T) {
		mockTGManager.EXPECT().List(ctx).Return(deleteTgs, nil)

		// the important bit below - svc export get returns does-not-exist
		mockClient.EXPECT().Get(ctx, gomock.Any(), gomock.Any()).Return(
			&apierrors.StatusError{
				ErrStatus: metav1.Status{
					Code:   http.StatusNotFound,
					Reason: metav1.StatusReasonNotFound,
				},
			})
		mockTGManager.EXPECT().Delete(ctx, gomock.Any()).Return(nil)

		synthesizer := NewTargetGroupSynthesizer(
			gwlog.FallbackLogger, nil, mockClient, mockTGManager, mockSvcExportTgBuilder, nil, nil)

		_, err := synthesizer.SynthesizeUnusedDelete(ctx)
		assert.Nil(t, err)
	})

	t.Run("Service Export deleted", func(t *testing.T) {
		mockTGManager.EXPECT().List(ctx).Return(deleteTgs, nil)

		// the important bit below - svc export get returns deleted
		mockClient.EXPECT().Get(ctx, gomock.Any(), gomock.Any()).DoAndReturn(
			func(ctx context.Context, name types.NamespacedName, svcExport client.Object, _ ...interface{}) error {
				now := metav1.Now()
				svcExport.SetName("svc")
				svcExport.SetNamespace("ns")
				svcExport.SetDeletionTimestamp(&now)
				return nil
			},
		)
		mockTGManager.EXPECT().Delete(ctx, gomock.Any()).Return(nil)

		synthesizer := NewTargetGroupSynthesizer(
			gwlog.FallbackLogger, nil, mockClient, mockTGManager, mockSvcExportTgBuilder, nil, nil)

		_, err := synthesizer.SynthesizeUnusedDelete(ctx)
		assert.Nil(t, err)
	})

	t.Run("Service Export model differs", func(t *testing.T) {
		modelTg := model.TargetGroup{
			Spec: model.TargetGroupSpec{
				VpcId:           "vpc-id",
				Type:            "IP",
				Port:            8080, // <-- important bit, port has changed
				Protocol:        "HTTP",
				ProtocolVersion: "HTTP1",
				IpAddressType:   "IPV4",
				TargetGroupTagFields: model.TargetGroupTagFields{
					K8SClusterName:      "cluster-name",
					K8SServiceName:      "svc",
					K8SServiceNamespace: "ns",
					K8SSourceType:       model.SourceTypeSvcExport,
				},
			},
		}

		mockClient.EXPECT().Get(ctx, gomock.Any(), gomock.Any()).DoAndReturn(
			func(ctx context.Context, name types.NamespacedName, svcExport client.Object, _ ...interface{}) error {
				svcExport.SetName("svc")
				svcExport.SetNamespace("ns")
				return nil
			},
		)

		mockSvcExportTgBuilder.EXPECT().BuildTargetGroups(ctx, gomock.Any()).Return(&modelTg, &modelTg, nil)

		mockTGManager.EXPECT().List(ctx).Return(deleteTgs, nil)
		mockTGManager.EXPECT().Delete(ctx, gomock.Any()).Return(nil)

		synthesizer := NewTargetGroupSynthesizer(
			gwlog.FallbackLogger, nil, mockClient, mockTGManager, mockSvcExportTgBuilder, nil, nil)

		_, err := synthesizer.SynthesizeUnusedDelete(ctx)
		assert.Nil(t, err)
	})
}

func Test_DeleteRoute_DeleteCases(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	mockTGManager := NewMockTargetGroupManager(c)
	mockClient := mock_client.NewMockClient(c)
	mockSvcBuilder := gateway.NewMockLatticeServiceBuilder(c)

	config.VpcID = "vpc-id"
	config.ClusterName = "cluster-name"

	baseTg := getBaseTg()

	var deleteTgs []tgListOutput
	tgSvc := copy(baseTg)
	tgSvc.tgSummary.Arn = aws.String("tg-svc-arn")
	tgSvc.tags[model.K8SSourceTypeKey] = aws.String(string(model.SourceTypeHTTPRoute))
	tgSvc.tags[model.K8SRouteNameKey] = aws.String("route")
	tgSvc.tags[model.K8SRouteNamespaceKey] = aws.String("route-ns")
	tgSvc.tags[model.K8SProtocolVersionKey] = aws.String("HTTP1")
	deleteTgs = append(deleteTgs, tgSvc)

	t.Run("Route does not exist", func(t *testing.T) {
		mockTGManager.EXPECT().List(ctx).Return(deleteTgs, nil)

		// the important bit below - svc export get returns does-not-exist
		mockClient.EXPECT().Get(ctx, gomock.Any(), gomock.Any()).Return(
			&apierrors.StatusError{
				ErrStatus: metav1.Status{
					Code:   http.StatusNotFound,
					Reason: metav1.StatusReasonNotFound,
				},
			})
		mockTGManager.EXPECT().Delete(ctx, gomock.Any()).Return(nil)

		synthesizer := NewTargetGroupSynthesizer(
			gwlog.FallbackLogger, nil, mockClient, mockTGManager, nil, mockSvcBuilder, nil)

		_, err := synthesizer.SynthesizeUnusedDelete(ctx)
		assert.Nil(t, err)
	})

	t.Run("Route deleted", func(t *testing.T) {
		mockTGManager.EXPECT().List(ctx).Return(deleteTgs, nil)

		// the important bit below - svc export get returns deleted
		mockClient.EXPECT().Get(ctx, gomock.Any(), gomock.Any()).DoAndReturn(
			func(ctx context.Context, name types.NamespacedName, route client.Object, _ ...interface{}) error {
				now := metav1.Now()
				route.SetName("route-name")
				route.SetNamespace("route-ns")
				route.SetDeletionTimestamp(&now)
				return nil
			},
		)
		mockTGManager.EXPECT().Delete(ctx, gomock.Any()).Return(nil)

		synthesizer := NewTargetGroupSynthesizer(
			gwlog.FallbackLogger, nil, mockClient, mockTGManager, nil, mockSvcBuilder, nil)

		_, err := synthesizer.SynthesizeUnusedDelete(ctx)
		assert.Nil(t, err)
	})

	t.Run("Route model differs", func(t *testing.T) {
		// current logic requires we return at least one tg
		// but match logic is deferred to tgManager.IsTargetGroupMatch
		svcModelTg := model.TargetGroup{
			Spec: model.TargetGroupSpec{
				VpcId:           "vpc-id",
				Type:            "IP",
				Port:            8080,
				Protocol:        "HTTP",
				ProtocolVersion: "HTTP1",
				IpAddressType:   "IPV4",
				TargetGroupTagFields: model.TargetGroupTagFields{
					K8SClusterName:      "cluster-name",
					K8SServiceName:      "svc",
					K8SServiceNamespace: "ns",
					K8SRouteName:        "route-name",
					K8SRouteNamespace:   "route-ns",
					K8SSourceType:       model.SourceTypeSvcExport,
				},
			},
		}

		stack := core.NewDefaultStack(core.StackID{Name: "foo", Namespace: "bar"})
		svcModelTg.ResourceMeta = core.NewResourceMeta(stack, "AWS:VPCServiceNetwork::TargetGroup", "tg-id")
		stack.AddResource(&svcModelTg)

		mockSvcBuilder.EXPECT().Build(ctx, gomock.Any()).Return(stack, nil)

		mockClient.EXPECT().Get(ctx, gomock.Any(), gomock.Any()).DoAndReturn(
			func(ctx context.Context, name types.NamespacedName, svcOrSvcExport client.Object, _ ...interface{}) error {
				svcOrSvcExport.SetName("svc")
				svcOrSvcExport.SetNamespace("ns")
				return nil
			},
		)

		mockTGManager.EXPECT().List(ctx).Return(deleteTgs, nil)
		// important bit below, return false for match
		// this is actually what decides if the tgs are a match or not
		mockTGManager.EXPECT().IsTargetGroupMatch(ctx, gomock.Any(), gomock.Any(), gomock.Any()).
			Return(false, nil)
		mockTGManager.EXPECT().Delete(ctx, gomock.Any()).Return(nil)

		synthesizer := NewTargetGroupSynthesizer(
			gwlog.FallbackLogger, nil, mockClient, mockTGManager, nil, mockSvcBuilder, nil)

		_, err := synthesizer.SynthesizeUnusedDelete(ctx)
		assert.Nil(t, err)
	})
}

// TODO: Error cases should not delete
