package deploy

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"

	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/types"

	mock_client "github.com/aws/aws-application-networking-k8s/mocks/controller-runtime/client"
	mocks_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/deploy/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"

	"github.com/aws/aws-application-networking-k8s/pkg/deploy/externaldns"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

func Test_latticeServiceStackDeployer_createAllResources(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()

	mockClient := mock_client.NewMockClient(c)
	mockCloud := mocks_aws.NewMockCloud(c)
	mockLattice := services.NewMockLattice(c)
	mockServiceManager := lattice.NewMockServiceManager(c)
	mockTargetGroupManager := lattice.NewMockTargetGroupManager(c)
	mockListenerManager := lattice.NewMockListenerManager(c)
	mockRuleManager := lattice.NewMockRuleManager(c)
	mockDnsManager := externaldns.NewMockDnsEndpointManager(c)
	mockTargetsManager := lattice.NewMockTargetsManager(c)
	mockLatticeDataStore := latticestore.NewLatticeDataStore()

	ctx := context.TODO()

	s := core.NewDefaultStack(core.StackID(types.NamespacedName{Namespace: "tt", Name: "name"}))

	stackService := model.NewLatticeService(s, "fake-service", model.ServiceSpec{})
	model.NewTargetGroup(s, "fake-targetGroup", model.TargetGroupSpec{})
	model.NewTargets(s, "fake-target", model.TargetsSpec{})
	model.NewListener(s, "fake-listener", 8080, "HTTP", "service1", "default", model.DefaultAction{})
	model.NewRule(s, "fake-rule", "fake-rule", "default", 80, "HTTP", model.RuleAction{}, model.RuleSpec{})

	mockLattice.EXPECT().FindService(gomock.Any(), gomock.Any()).Return(
		&vpclattice.ServiceSummary{
			Name: aws.String(stackService.LatticeServiceName()),
			Id:   aws.String("fake-service"),
		}, nil).AnyTimes()

	mockListenerManager.EXPECT().Cloud().Return(mockCloud).AnyTimes()
	mockRuleManager.EXPECT().Cloud().Return(mockCloud).AnyTimes()
	mockCloud.EXPECT().Lattice().Return(mockLattice).AnyTimes()

	mockTargetGroupManager.EXPECT().List(gomock.Any()).AnyTimes()
	mockListenerManager.EXPECT().List(gomock.Any(), gomock.Any()).AnyTimes()

	mockServiceManager.EXPECT().Create(gomock.Any(), gomock.Any())
	mockTargetGroupManager.EXPECT().Create(gomock.Any(), gomock.Any()).AnyTimes()
	mockTargetsManager.EXPECT().Create(gomock.Any(), gomock.Any())
	mockListenerManager.EXPECT().Create(gomock.Any(), gomock.Any())
	mockRuleManager.EXPECT().Create(gomock.Any(), gomock.Any())
	mockDnsManager.EXPECT().Create(gomock.Any(), gomock.Any())

	deployer := &LatticeServiceStackDeployer{
		log:                   gwlog.FallbackLogger,
		cloud:                 mockCloud,
		k8sClient:             mockClient,
		latticeServiceManager: mockServiceManager,
		targetGroupManager:    mockTargetGroupManager,
		listenerManager:       mockListenerManager,
		ruleManager:           mockRuleManager,
		targetsManager:        mockTargetsManager,
		dnsEndpointManager:    mockDnsManager,
		latticeDataStore:      mockLatticeDataStore,
	}

	err := deployer.Deploy(ctx, s)

	assert.Nil(t, err)
}

func Test_latticeServiceStackDeployer_CreateJustService(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()

	mockClient := mock_client.NewMockClient(c)
	mockCloud := mocks_aws.NewMockCloud(c)
	mockLattice := services.NewMockLattice(c)
	mockServiceManager := lattice.NewMockServiceManager(c)
	mockTargetGroupManager := lattice.NewMockTargetGroupManager(c)
	mockTargetsManager := lattice.NewMockTargetsManager(c)
	mockListenerManager := lattice.NewMockListenerManager(c)
	mockRuleManager := lattice.NewMockRuleManager(c)
	mockDnsManager := externaldns.NewMockDnsEndpointManager(c)
	mockLatticeDataStore := latticestore.NewLatticeDataStore()

	ctx := context.TODO()

	mockTargetGroupManager.EXPECT().List(gomock.Any())
	mockListenerManager.EXPECT().List(gomock.Any(), gomock.Any())

	mockServiceManager.EXPECT().Create(gomock.Any(), gomock.Any())
	mockDnsManager.EXPECT().Create(gomock.Any(), gomock.Any())

	s := core.NewDefaultStack(core.StackID(types.NamespacedName{Namespace: "tt", Name: "name"}))

	stackService := model.NewLatticeService(s, "fake-service", model.ServiceSpec{})

	mockLattice.EXPECT().FindService(gomock.Any(), gomock.Any()).Return(
		&vpclattice.ServiceSummary{
			Name: aws.String(stackService.LatticeServiceName()),
			Id:   aws.String("fake-service"),
		}, nil).AnyTimes()

	mockListenerManager.EXPECT().Cloud().Return(mockCloud).AnyTimes()
	mockRuleManager.EXPECT().Cloud().Return(mockCloud).AnyTimes()
	mockCloud.EXPECT().Lattice().Return(mockLattice).AnyTimes()

	deployer := &LatticeServiceStackDeployer{
		log:                   gwlog.FallbackLogger,
		cloud:                 mockCloud,
		k8sClient:             mockClient,
		latticeServiceManager: mockServiceManager,
		targetGroupManager:    mockTargetGroupManager,
		targetsManager:        mockTargetsManager,
		listenerManager:       mockListenerManager,
		ruleManager:           mockRuleManager,
		dnsEndpointManager:    mockDnsManager,
		latticeDataStore:      mockLatticeDataStore,
	}

	err := deployer.Deploy(ctx, s)

	assert.Nil(t, err)
}

func Test_latticeServiceStackDeployer_DeleteService(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()

	mockClient := mock_client.NewMockClient(c)
	mockCloud := mocks_aws.NewMockCloud(c)
	mockLattice := services.NewMockLattice(c)
	mockServiceManager := lattice.NewMockServiceManager(c)
	mockTargetGroupManager := lattice.NewMockTargetGroupManager(c)
	mockListenerManager := lattice.NewMockListenerManager(c)
	mockRuleManager := lattice.NewMockRuleManager(c)
	mockTargetsManager := lattice.NewMockTargetsManager(c)
	mockLatticeDataStore := latticestore.NewLatticeDataStore()

	ctx := context.TODO()

	s := core.NewDefaultStack(core.StackID(types.NamespacedName{Namespace: "tt", Name: "name"}))

	stackService := model.NewLatticeService(s, "fake-service", model.ServiceSpec{
		IsDeleted: true,
	})

	mockLattice.EXPECT().FindService(gomock.Any(), gomock.Any()).Return(
		&vpclattice.ServiceSummary{
			Name: aws.String(stackService.LatticeServiceName()),
			Id:   aws.String("fake-service"),
		}, nil).AnyTimes()

	mockListenerManager.EXPECT().Cloud().Return(mockCloud).AnyTimes()
	mockRuleManager.EXPECT().Cloud().Return(mockCloud).AnyTimes()
	mockCloud.EXPECT().Lattice().Return(mockLattice).AnyTimes()

	mockTargetGroupManager.EXPECT().List(gomock.Any()).AnyTimes()
	mockListenerManager.EXPECT().List(gomock.Any(), gomock.Any()).AnyTimes()

	mockServiceManager.EXPECT().Delete(gomock.Any(), gomock.Any())

	deployer := &LatticeServiceStackDeployer{
		log:                   gwlog.FallbackLogger,
		cloud:                 mockCloud,
		k8sClient:             mockClient,
		latticeServiceManager: mockServiceManager,
		targetGroupManager:    mockTargetGroupManager,
		listenerManager:       mockListenerManager,
		ruleManager:           mockRuleManager,
		targetsManager:        mockTargetsManager,
		latticeDataStore:      mockLatticeDataStore,
	}

	err := deployer.Deploy(ctx, s)

	assert.Nil(t, err)
}

func Test_latticeServiceStackDeployer_DeleteAllResources(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()

	mockClient := mock_client.NewMockClient(c)
	mockCloud := mocks_aws.NewMockCloud(c)
	mockLattice := services.NewMockLattice(c)
	mockServiceManager := lattice.NewMockServiceManager(c)
	mockTargetGroupManager := lattice.NewMockTargetGroupManager(c)
	mockListenerManager := lattice.NewMockListenerManager(c)
	mockRuleManager := lattice.NewMockRuleManager(c)
	mockTargetsManager := lattice.NewMockTargetsManager(c)
	mockLatticeDataStore := latticestore.NewLatticeDataStore()

	ctx := context.TODO()

	s := core.NewDefaultStack(core.StackID(types.NamespacedName{Namespace: "tt", Name: "name"}))

	stackService := model.NewLatticeService(s, "fake-service", model.ServiceSpec{
		IsDeleted: true,
	})
	model.NewTargetGroup(s, "fake-targetGroup", model.TargetGroupSpec{
		IsDeleted: true,
	})

	mockLattice.EXPECT().FindService(gomock.Any(), gomock.Any()).Return(
		&vpclattice.ServiceSummary{
			Name: aws.String(stackService.LatticeServiceName()),
			Id:   aws.String("fake-service"),
		}, nil).AnyTimes()

	mockListenerManager.EXPECT().Cloud().Return(mockCloud).AnyTimes()
	mockRuleManager.EXPECT().Cloud().Return(mockCloud).AnyTimes()
	mockCloud.EXPECT().Lattice().Return(mockLattice).AnyTimes()

	mockTargetGroupManager.EXPECT().List(gomock.Any()).AnyTimes()
	mockListenerManager.EXPECT().List(gomock.Any(), gomock.Any()).AnyTimes()

	mockServiceManager.EXPECT().Delete(gomock.Any(), gomock.Any())
	mockTargetGroupManager.EXPECT().Delete(gomock.Any(), gomock.Any())

	deployer := &LatticeServiceStackDeployer{
		log:                   gwlog.FallbackLogger,
		cloud:                 mockCloud,
		k8sClient:             mockClient,
		latticeServiceManager: mockServiceManager,
		targetGroupManager:    mockTargetGroupManager,
		listenerManager:       mockListenerManager,
		ruleManager:           mockRuleManager,
		targetsManager:        mockTargetsManager,
		latticeDataStore:      mockLatticeDataStore,
	}

	err := deployer.Deploy(ctx, s)

	assert.Nil(t, err)
}
