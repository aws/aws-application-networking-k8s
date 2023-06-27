package deploy

import (
	"context"
	"testing"

	mock_client "github.com/aws/aws-application-networking-k8s/mocks/controller-runtime/client"
	mocks_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/deploy/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/golang/mock/gomock"
	"k8s.io/apimachinery/pkg/types"

	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

func Test_latticeServiceStackDeployer_createAllResources(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()

	mockClient := mock_client.NewMockClient(c)

	mockCloud := mocks_aws.NewMockCloud(c)

	mockServiceManager := lattice.NewMockServiceManager(c)

	mockTargetGroupManager := lattice.NewMockTargetGroupManager(c)

	mockListenerManager := lattice.NewMockListenerManager(c)

	mockRuleManager := lattice.NewMockRuleManager(c)

	mockTargetsManager := lattice.NewMockTargetsManager(c)

	mockLatticeDataStore := latticestore.NewLatticeDataStore()

	ctx := context.TODO()

	s := core.NewDefaultStack(core.StackID(types.NamespacedName{Namespace: "tt", Name: "name"}))

	latticemodel.NewLatticeService(s, "fake-service", latticemodel.ServiceSpec{})
	latticemodel.NewTargetGroup(s, "fake-targetGroup", latticemodel.TargetGroupSpec{})
	latticemodel.NewTargets(s, "fake-target", latticemodel.TargetsSpec{})
	latticemodel.NewListener(s, "fake-listener", 8080, "HTTP", "service1", "default", latticemodel.DefaultAction{})
	latticemodel.NewRule(s, "fake-rule", "fake-rule", "default", 80, "HTTP", latticemodel.RuleAction{}, latticemodel.RuleSpec{})

	mockTargetGroupManager.EXPECT().List(gomock.Any()).AnyTimes()
	mockListenerManager.EXPECT().List(gomock.Any(), gomock.Any()).AnyTimes()

	mockServiceManager.EXPECT().Create(gomock.Any(), gomock.Any())
	mockTargetGroupManager.EXPECT().Create(gomock.Any(), gomock.Any()).AnyTimes()
	mockTargetsManager.EXPECT().Create(gomock.Any(), gomock.Any())
	mockListenerManager.EXPECT().Create(gomock.Any(), gomock.Any())
	mockRuleManager.EXPECT().Create(gomock.Any(), gomock.Any())

	deployer := &latticeServiceStackDeployer{
		cloud:                 mockCloud,
		k8sclient:             mockClient,
		latticeServiceManager: mockServiceManager,
		targetGroupManager:    mockTargetGroupManager,
		listenerManager:       mockListenerManager,
		ruleManager:           mockRuleManager,
		targetsManager:        mockTargetsManager,
		latticeDataStore:      mockLatticeDataStore,
	}

	deployer.Deploy(ctx, s)
}

func Test_latticeServiceStackDeployer_CreateJustService(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()

	mockClient := mock_client.NewMockClient(c)

	mockCloud := mocks_aws.NewMockCloud(c)

	mockServiceManager := lattice.NewMockServiceManager(c)

	mockTargetGroupManager := lattice.NewMockTargetGroupManager(c)

	mockTargetsManager := lattice.NewMockTargetsManager(c)

	mockListenerManager := lattice.NewMockListenerManager(c)

	mockRuleManager := lattice.NewMockRuleManager(c)

	mockLatticeDataStore := latticestore.NewLatticeDataStore()

	ctx := context.TODO()

	mockTargetGroupManager.EXPECT().List(gomock.Any())
	mockListenerManager.EXPECT().List(gomock.Any(), gomock.Any())

	mockServiceManager.EXPECT().Create(gomock.Any(), gomock.Any())

	s := core.NewDefaultStack(core.StackID(types.NamespacedName{Namespace: "tt", Name: "name"}))

	latticemodel.NewLatticeService(s, "fake-service", latticemodel.ServiceSpec{})

	deployer := &latticeServiceStackDeployer{
		cloud:                 mockCloud,
		k8sclient:             mockClient,
		latticeServiceManager: mockServiceManager,
		targetGroupManager:    mockTargetGroupManager,
		targetsManager:        mockTargetsManager,
		listenerManager:       mockListenerManager,
		ruleManager:           mockRuleManager,
		latticeDataStore:      mockLatticeDataStore,
	}

	deployer.Deploy(ctx, s)
}

func Test_latticeServiceStackDeployer_DeleteService(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()

	mockClient := mock_client.NewMockClient(c)

	mockCloud := mocks_aws.NewMockCloud(c)

	mockServiceManager := lattice.NewMockServiceManager(c)

	mockTargetGroupManager := lattice.NewMockTargetGroupManager(c)

	mockListenerManager := lattice.NewMockListenerManager(c)

	mockRuleManager := lattice.NewMockRuleManager(c)

	mockTargetsManager := lattice.NewMockTargetsManager(c)

	mockLatticeDataStore := latticestore.NewLatticeDataStore()

	ctx := context.TODO()

	s := core.NewDefaultStack(core.StackID(types.NamespacedName{Namespace: "tt", Name: "name"}))

	latticemodel.NewLatticeService(s, "fake-service", latticemodel.ServiceSpec{
		IsDeleted: true,
	})

	mockTargetGroupManager.EXPECT().List(gomock.Any()).AnyTimes()
	mockListenerManager.EXPECT().List(gomock.Any(), gomock.Any()).AnyTimes()

	mockServiceManager.EXPECT().Delete(gomock.Any(), gomock.Any())

	deployer := &latticeServiceStackDeployer{
		cloud:                 mockCloud,
		k8sclient:             mockClient,
		latticeServiceManager: mockServiceManager,
		targetGroupManager:    mockTargetGroupManager,
		listenerManager:       mockListenerManager,
		ruleManager:           mockRuleManager,
		targetsManager:        mockTargetsManager,
		latticeDataStore:      mockLatticeDataStore,
	}

	deployer.Deploy(ctx, s)
}

func Test_latticeServiceStackDeployer_DeleteAllResources(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()

	mockClient := mock_client.NewMockClient(c)

	mockCloud := mocks_aws.NewMockCloud(c)

	mockServiceManager := lattice.NewMockServiceManager(c)

	mockTargetGroupManager := lattice.NewMockTargetGroupManager(c)

	mockListenerManager := lattice.NewMockListenerManager(c)

	mockRuleManager := lattice.NewMockRuleManager(c)

	mockTargetsManager := lattice.NewMockTargetsManager(c)

	mockLatticeDataStore := latticestore.NewLatticeDataStore()

	ctx := context.TODO()

	s := core.NewDefaultStack(core.StackID(types.NamespacedName{Namespace: "tt", Name: "name"}))

	latticemodel.NewLatticeService(s, "fake-service", latticemodel.ServiceSpec{
		IsDeleted: true,
	})
	latticemodel.NewTargetGroup(s, "fake-targetGroup", latticemodel.TargetGroupSpec{
		IsDeleted: true,
	})

	mockTargetGroupManager.EXPECT().List(gomock.Any()).AnyTimes()
	mockListenerManager.EXPECT().List(gomock.Any(), gomock.Any()).AnyTimes()

	mockServiceManager.EXPECT().Delete(gomock.Any(), gomock.Any())
	mockTargetGroupManager.EXPECT().Delete(gomock.Any(), gomock.Any())

	deployer := &latticeServiceStackDeployer{
		cloud:                 mockCloud,
		k8sclient:             mockClient,
		latticeServiceManager: mockServiceManager,
		targetGroupManager:    mockTargetGroupManager,
		listenerManager:       mockListenerManager,
		ruleManager:           mockRuleManager,
		targetsManager:        mockTargetsManager,
		latticeDataStore:      mockLatticeDataStore,
	}

	deployer.Deploy(ctx, s)
}
