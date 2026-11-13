package deploy

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/deploy/externaldns"
	"github.com/aws/aws-application-networking-k8s/pkg/deploy/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

func TestTgGc(t *testing.T) {

	type test struct {
		name string
		gcFn TgGcCycleFn
	}

	tests := []test{
		{
			name: "empty cycle",
			gcFn: func(context.Context) (TgGcResult, error) { return TgGcResult{}, nil },
		},
		{
			name: "cycle with results",
			gcFn: func(context.Context) (TgGcResult, error) {
				return TgGcResult{
					att:      10,
					succ:     10,
					duration: time.Second,
				}, nil
			},
		},
		{
			name: "cycle panic",
			gcFn: func(context.Context) (TgGcResult, error) { panic("") },
		},
		{
			name: "cycle error",
			gcFn: func(context.Context) (TgGcResult, error) { return TgGcResult{}, errors.New("") },
		},
	}

	nCycles := 10 // run each test at least 10 cycles

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			n := 0
			f := func(ctx context.Context) (TgGcResult, error) {
				defer func() {
					n += 1
					if n >= nCycles {
						cancel()
					}
				}()
				return tt.gcFn(ctx)
			}
			ivl := time.Millisecond * 10
			tgGc := &TgGc{
				log:     gwlog.FallbackLogger,
				ctx:     ctx,
				ivl:     ivl,
				cycleFn: f,
			}
			tgGc.start()
			time.Sleep(ivl * (time.Duration(nCycles) + 2)) // sleep enough cycles to terminate
			assert.Equal(t, nCycles, n, fmt.Sprintf("should run only %d cycles", nCycles))
			assert.True(t, tgGc.isDone.Load(), "gc must terminate")
		})
	}
}

func newTestRouteStack(t *testing.T) core.Stack {
	if tgGc == nil {
		tgGc = &TgGc{}
	}
	stack := core.NewDefaultStack(core.StackID{Name: "route", Namespace: "ns"})
	_, err := model.NewLatticeService(stack, model.ServiceSpec{
		ServiceTagFields: model.ServiceTagFields{RouteName: "route", RouteNamespace: "ns", RouteType: core.HttpRouteType},
	})
	assert.NoError(t, err)
	tg, err := model.NewTargetGroup(stack, model.TargetGroupSpec{
		Type: model.TargetGroupTypeIP, Port: 80, Protocol: "HTTP", ProtocolVersion: "HTTP1",
		IpAddressType: "IPV4", VpcId: "vpc-id",
		TargetGroupTagFields: model.TargetGroupTagFields{
			K8SClusterName: "cluster", K8SSourceType: model.SourceTypeHTTPRoute,
			K8SServiceName: "svc", K8SServiceNamespace: "ns", K8SRouteName: "route", K8SRouteNamespace: "ns",
		},
	})
	assert.NoError(t, err)
	_, err = model.NewTargets(stack, model.TargetsSpec{StackTargetGroupId: tg.ID()})
	assert.NoError(t, err)
	return stack
}

func TestLatticeServiceStackDeploy_ServiceErrors(t *testing.T) {
	ctx := context.TODO()
	k8sScheme := runtime.NewScheme()
	assert.NoError(t, clientgoscheme.AddToScheme(k8sScheme))
	assert.NoError(t, anv1alpha1.Install(k8sScheme))
	k8sClient := testclient.NewClientBuilder().WithScheme(k8sScheme).Build()

	tests := []struct {
		name              string
		svcErr            error
		expectTgsAndTargs bool
		expectConflict    bool
	}{
		{
			name:              "service owned by another route stops before target groups are created",
			svcErr:            services.NewConflictError("service", "route/ns", "conflicting service name"),
			expectTgsAndTargs: false,
			expectConflict:    true,
		},
		{
			name:              "other service errors still update target groups and targets",
			svcErr:            errors.New("throttled"),
			expectTgsAndTargs: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := gomock.NewController(t)
			svcMgr := lattice.NewMockServiceManager(c)
			tgMgr := lattice.NewMockTargetGroupManager(c)
			targetsMgr := lattice.NewMockTargetsManager(c)
			listenerMgr := lattice.NewMockListenerManager(c)
			ruleMgr := lattice.NewMockRuleManager(c)

			svcCall := svcMgr.EXPECT().Upsert(ctx, gomock.Any()).Return(model.ServiceStatus{}, tt.svcErr)
			if tt.expectTgsAndTargs {
				tgCall := tgMgr.EXPECT().Upsert(ctx, gomock.Any()).
					Return(model.TargetGroupStatus{Id: "tg-id"}, nil).After(svcCall)
				targetsMgr.EXPECT().Update(ctx, gomock.Any(), gomock.Any()).Return(nil).After(tgCall)
			}

			d := &latticeServiceStackDeployer{
				log:                   gwlog.FallbackLogger,
				k8sClient:             k8sClient,
				latticeServiceManager: svcMgr,
				targetGroupManager:    tgMgr,
				targetsManager:        targetsMgr,
				listenerManager:       listenerMgr,
				ruleManager:           ruleMgr,
				dnsEndpointManager:    externaldns.NewMockDnsEndpointManager(c),
			}
			err := d.Deploy(ctx, newTestRouteStack(t))
			assert.ErrorIs(t, err, tt.svcErr)
			assert.Equal(t, tt.expectConflict, services.IsConflictError(err))
		})
	}
}

func TestLatticeServiceStackDeploy_SynthesizesServiceBeforeTargetGroups(t *testing.T) {
	ctx := context.TODO()
	k8sScheme := runtime.NewScheme()
	assert.NoError(t, clientgoscheme.AddToScheme(k8sScheme))
	assert.NoError(t, anv1alpha1.Install(k8sScheme))
	k8sClient := testclient.NewClientBuilder().WithScheme(k8sScheme).Build()

	c := gomock.NewController(t)
	svcMgr := lattice.NewMockServiceManager(c)
	tgMgr := lattice.NewMockTargetGroupManager(c)
	targetsMgr := lattice.NewMockTargetsManager(c)
	listenerMgr := lattice.NewMockListenerManager(c)
	dnsMgr := externaldns.NewMockDnsEndpointManager(c)
	listErr := errors.New("stop after targets")

	gomock.InOrder(
		svcMgr.EXPECT().Upsert(ctx, gomock.Any()).Return(model.ServiceStatus{Id: "svc-id"}, nil),
		dnsMgr.EXPECT().Create(ctx, gomock.Any()).Return(nil),
		tgMgr.EXPECT().Upsert(ctx, gomock.Any()).Return(model.TargetGroupStatus{Id: "tg-id"}, nil),
		targetsMgr.EXPECT().Update(ctx, gomock.Any(), gomock.Any()).Return(nil),
		listenerMgr.EXPECT().List(ctx, "svc-id").Return(nil, nil),
		targetsMgr.EXPECT().List(ctx, gomock.Any()).Return(nil, listErr),
	)

	d := &latticeServiceStackDeployer{
		log:                   gwlog.FallbackLogger,
		k8sClient:             k8sClient,
		latticeServiceManager: svcMgr,
		targetGroupManager:    tgMgr,
		targetsManager:        targetsMgr,
		listenerManager:       listenerMgr,
		ruleManager:           lattice.NewMockRuleManager(c),
		dnsEndpointManager:    dnsMgr,
	}
	assert.ErrorIs(t, d.Deploy(ctx, newTestRouteStack(t)), listErr)
}

func TestLatticeServiceStackDeploy_DeletedRoute(t *testing.T) {
	ctx := context.TODO()
	k8sScheme := runtime.NewScheme()
	assert.NoError(t, clientgoscheme.AddToScheme(k8sScheme))
	k8sClient := testclient.NewClientBuilder().WithScheme(k8sScheme).Build()

	if tgGc == nil {
		tgGc = &TgGc{}
	}
	stack := core.NewDefaultStack(core.StackID{Name: "route", Namespace: "ns"})
	svc, err := model.NewLatticeService(stack, model.ServiceSpec{
		ServiceTagFields: model.ServiceTagFields{RouteName: "route", RouteNamespace: "ns", RouteType: core.HttpRouteType},
	})
	assert.NoError(t, err)
	svc.IsDeleted = true
	tg, err := model.NewTargetGroup(stack, model.TargetGroupSpec{
		Type: model.TargetGroupTypeIP, Port: 80, Protocol: "HTTP", ProtocolVersion: "HTTP1",
		IpAddressType: "IPV4", VpcId: "vpc-id",
		TargetGroupTagFields: model.TargetGroupTagFields{
			K8SClusterName: "cluster", K8SSourceType: model.SourceTypeHTTPRoute,
			K8SServiceName: "svc", K8SServiceNamespace: "ns", K8SRouteName: "route", K8SRouteNamespace: "ns",
		},
	})
	assert.NoError(t, err)
	tg.IsDeleted = true

	c := gomock.NewController(t)
	svcMgr := lattice.NewMockServiceManager(c)
	tgMgr := lattice.NewMockTargetGroupManager(c)
	gomock.InOrder(
		svcMgr.EXPECT().Delete(ctx, svc).Return(nil),
		tgMgr.EXPECT().Delete(ctx, tg).Return(nil),
	)

	d := &latticeServiceStackDeployer{
		log:                   gwlog.FallbackLogger,
		k8sClient:             k8sClient,
		latticeServiceManager: svcMgr,
		targetGroupManager:    tgMgr,
		targetsManager:        lattice.NewMockTargetsManager(c),
		listenerManager:       lattice.NewMockListenerManager(c),
		ruleManager:           lattice.NewMockRuleManager(c),
		dnsEndpointManager:    externaldns.NewMockDnsEndpointManager(c),
	}
	assert.NoError(t, d.Deploy(ctx, stack))
}
