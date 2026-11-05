package controllers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	mock_client "github.com/aws/aws-application-networking-k8s/mocks/controller-runtime/client"
	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	aws2 "github.com/aws/aws-application-networking-k8s/pkg/aws"
	mocks "github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/deploy"
	"github.com/aws/aws-application-networking-k8s/pkg/deploy/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/gateway"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/vpclattice"
	"github.com/aws/aws-sdk-go-v2/service/vpclattice/types"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/external-dns/endpoint"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func TestRouteReconciler_ReconcileCreates(t *testing.T) {
	config.VpcID = "my-vpc"
	config.ClusterName = "my-cluster"

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()

	k8sScheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(k8sScheme)
	gwv1.Install(k8sScheme)
	discoveryv1.AddToScheme(k8sScheme)
	addOptionalCRDs(k8sScheme)

	k8sClient := testclient.
		NewClientBuilder().
		WithScheme(k8sScheme).
		WithStatusSubresource(&gwv1.HTTPRoute{}).
		Build()

	gwClass := &gwv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "amazon-vpc-lattice",
		},
		Spec: gwv1.GatewayClassSpec{
			ControllerName: config.LatticeGatewayControllerName,
		},
		Status: gwv1.GatewayClassStatus{},
	}
	k8sClient.Create(ctx, gwClass.DeepCopy())

	// here we have a gateway, service, and route
	gw := &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-gateway",
			Namespace: "ns1",
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: "amazon-vpc-lattice",
			Listeners: []gwv1.Listener{
				{
					Name:     "http",
					Protocol: "HTTP",
					Port:     80,
				},
			},
		},
	}

	notVpcLattice := &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "not-vpc-lattice",
			Namespace: "ns1",
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: "not-amazon-vpc-lattice",
			Listeners: []gwv1.Listener{
				{
					Name:     "http",
					Protocol: "HTTP",
					Port:     80,
				},
			},
		},
	}

	k8sClient.Create(ctx, notVpcLattice.DeepCopy())
	k8sClient.Create(ctx, gw.DeepCopy())

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-service",
			Namespace: "ns1",
		},
		Spec: corev1.ServiceSpec{
			IPFamilies: []corev1.IPFamily{
				"IPv4",
			},
			Ports: []corev1.ServicePort{
				{
					Protocol:   "TCP",
					Port:       80,
					TargetPort: intstr.FromInt(8090),
				},
			},
		},
	}
	k8sClient.Create(ctx, svc.DeepCopy())

	epSlice := discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-service",
			Namespace: "ns1",
			Labels:    map[string]string{discoveryv1.LabelServiceName: "my-service"},
		},
		Ports: []discoveryv1.EndpointPort{
			{Port: aws.Int32(8090)},
		},
		Endpoints: []discoveryv1.Endpoint{
			{
				Addresses: []string{"192.0.2.22", "192.0.2.33"},
				Conditions: discoveryv1.EndpointConditions{
					Ready: aws.Bool(true),
				},
			},
		},
	}
	k8sClient.Create(ctx, epSlice.DeepCopy())

	kind := gwv1.Kind("Service")
	port := gwv1.PortNumber(80)
	route := gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-route",
			Namespace: "ns1",
		},
		Spec: gwv1.HTTPRouteSpec{
			CommonRouteSpec: gwv1.CommonRouteSpec{
				ParentRefs: []gwv1.ParentReference{
					// if route has multiple parents, we'll only use the managed vpc lattice gateway
					{
						Name: "not-vpc-lattice",
					},
					{
						Name: "my-gateway",
					},
				},
			},
			Rules: []gwv1.HTTPRouteRule{
				{
					BackendRefs: []gwv1.HTTPBackendRef{
						{
							BackendRef: gwv1.BackendRef{
								BackendObjectReference: gwv1.BackendObjectReference{
									Kind: &kind,
									Name: "my-service",
									Port: &port,
								},
								Weight: aws.Int32(10),
							},
						},
					},
				},
			},
		},
	}
	k8sClient.Create(ctx, route.DeepCopy())

	mockCloud := aws2.NewMockCloud(c)
	mockLattice := mocks.NewMockLattice(c)
	mockTagging := mocks.NewMockTagging(c)
	mockCloud.EXPECT().Lattice().Return(mockLattice).AnyTimes()
	mockCloud.EXPECT().Tagging().Return(mockTagging).AnyTimes()
	mockCloud.EXPECT().Config().Return(
		aws2.CloudConfig{
			VpcId:       config.VpcID,
			AccountId:   "account-id",
			Region:      "ep-imagine-1",
			ClusterName: config.ClusterName,
		}).AnyTimes()
	mockCloud.EXPECT().DefaultTags().Return(mocks.Tags{}).AnyTimes()
	mockCloud.EXPECT().DefaultTagsMergedWith(gomock.Any()).Return(mocks.Tags{}).AnyTimes()
	mockCloud.EXPECT().MergeTags(gomock.Any(), gomock.Any()).Return(mocks.Tags{}).AnyTimes()

	// we expect a fair number of lattice calls
	mockLattice.EXPECT().ListTargetsAsList(gomock.Any(), gomock.Any()).Return(
		[]types.TargetSummary{}, nil)
	mockLattice.EXPECT().ListTargetsAsList(gomock.Any(), gomock.Any()).Return(
		[]types.TargetSummary{
			{
				Id:   aws.String("192.0.2.22"),
				Port: aws.Int32(8090),
			},
			{
				Id:   aws.String("192.0.2.33"),
				Port: aws.Int32(8090),
			},
		}, nil)
	mockLattice.EXPECT().RegisterTargets(gomock.Any(), gomock.Any()).Return(
		&vpclattice.RegisterTargetsOutput{
			Successful: []types.Target{
				{
					Id:   aws.String("192.0.2.22"),
					Port: aws.Int32(8090),
				},
				{
					Id:   aws.String("192.0.2.33"),
					Port: aws.Int32(8090),
				},
			},
		}, nil)
	mockLattice.EXPECT().FindServiceNetwork(gomock.Any(), gomock.Any()).Return(
		&mocks.ServiceNetworkInfo{
			SvcNetwork: types.ServiceNetworkSummary{
				Arn:  aws.String("sn-arn"),
				Id:   aws.String("sn-id"),
				Name: aws.String("sn-name"),
			},
		}, nil)
	mockLattice.EXPECT().FindService(gomock.Any(), gomock.Any()).Return(
		nil, mocks.NewNotFoundError("Service", "svc-name")) // will trigger create
	mockLattice.EXPECT().CreateService(gomock.Any(), gomock.Any()).Return(
		&vpclattice.CreateServiceOutput{
			Arn:    aws.String("svc-arn"),
			Id:     aws.String("svc-id"),
			Name:   aws.String("svc-name"),
			Status: types.ServiceStatusActive,
			DnsEntry: &types.DnsEntry{
				DomainName: aws.String("svc-name.7d67968.vpc-lattice-svcs.us-west-2.on.aws"),
			},
		}, nil)
	mockLattice.EXPECT().CreateServiceNetworkServiceAssociation(gomock.Any(), gomock.Any()).Return(
		&vpclattice.CreateServiceNetworkServiceAssociationOutput{
			Arn:    aws.String("sns-assoc-arn"),
			Id:     aws.String("sns-assoc-id"),
			Status: types.ServiceNetworkServiceAssociationStatusActive,
		}, nil)

	mockTagging.EXPECT().FindResourcesByTags(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil)
	mockLattice.EXPECT().ListTargetGroupsAsList(gomock.Any(), gomock.Any()).Return(
		[]types.TargetGroupSummary{}, nil).AnyTimes() // this will cause us to skip "unused delete" step
	mockLattice.EXPECT().CreateTargetGroup(gomock.Any(), gomock.Any()).Return(
		&vpclattice.CreateTargetGroupOutput{
			Arn:    aws.String("tg-arn"),
			Id:     aws.String("tg-id"),
			Name:   aws.String("tg-name"),
			Status: types.TargetGroupStatusActive,
		}, nil)

	mockLattice.EXPECT().ListListenersAsList(gomock.Any(), gomock.Any()).Return(
		[]types.ListenerSummary{}, nil).AnyTimes()
	mockLattice.EXPECT().CreateListener(gomock.Any(), gomock.Any()).Return(
		&vpclattice.CreateListenerOutput{
			Arn:        aws.String("listener-arn"),
			Id:         aws.String("listener-id"),
			Name:       aws.String("listener-name"),
			ServiceArn: aws.String("svc-arn"),
			ServiceId:  aws.String("svc-id"),
		}, nil)

	mockLattice.EXPECT().GetRulesAsList(gomock.Any(), gomock.Any()).Return(
		[]*vpclattice.GetRuleOutput{}, nil)
	mockLattice.EXPECT().CreateRule(gomock.Any(), gomock.Any()).Return(
		&vpclattice.CreateRuleOutput{
			Arn:      aws.String("rule-arn"),
			Id:       aws.String("rule-id"),
			Name:     aws.String("rule-name"),
			Priority: aws.Int32(1),
		}, nil)
	// List is called after create, so we'll return what we have
	mockLattice.EXPECT().ListRulesAsList(gomock.Any(), gomock.Any()).Return(
		[]types.RuleSummary{
			{
				Arn:       aws.String("rule-arn"),
				Id:        aws.String("rule-id"),
				IsDefault: aws.Bool(false),
				Name:      aws.String("rule-name"),
				Priority:  aws.Int32(1),
			},
		}, nil)

	mockEventRecorder := mock_client.NewMockEventRecorder(c)
	mockEventRecorder.EXPECT().Event(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	mockFinalizer := k8s.NewMockFinalizerManager(c)
	mockFinalizer.EXPECT().AddFinalizers(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	mockFinalizer.EXPECT().RemoveFinalizers(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	brTgBuilder := gateway.NewBackendRefTargetGroupBuilder(gwlog.FallbackLogger, k8sClient)
	rc := routeReconciler{
		routeType:        core.HttpRouteType,
		log:              gwlog.FallbackLogger,
		client:           k8sClient,
		scheme:           k8sScheme,
		finalizerManager: mockFinalizer,
		eventRecorder:    mockEventRecorder,
		modelBuilder:     gateway.NewLatticeServiceBuilder(gwlog.FallbackLogger, k8sClient, brTgBuilder, nil, nil),
		stackDeployer:    deploy.NewLatticeServiceStackDeploy(gwlog.FallbackLogger, mockCloud, k8sClient),
		stackMarshaller:  deploy.NewDefaultStackMarshaller(),
		cloud:            mockCloud,
	}

	routeName := k8s.NamespacedName(&route)
	result, err := rc.Reconcile(ctx, reconcile.Request{NamespacedName: routeName})
	assert.Nil(t, err)
	assert.Equal(t, time.Duration(0), result.RequeueAfter)

	// Verify service annotations were set from CreateService response
	updatedRoute := &gwv1.HTTPRoute{}
	k8sClient.Get(ctx, routeName, updatedRoute)
	annotations := updatedRoute.GetAnnotations()
	assert.Equal(t, "svc-arn", annotations[LatticeServiceArn])
	assert.Equal(t, "svc-name.7d67968.vpc-lattice-svcs.us-west-2.on.aws", annotations[LatticeAssignedDomainName])
}

func TestRouteReconciler_UpdateRouteStatusWithServiceInfo(t *testing.T) {
	ctx := context.TODO()

	k8sScheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(k8sScheme)
	gwv1.Install(k8sScheme)

	t.Run("sets both ARN and DNS annotations", func(t *testing.T) {
		k8sClient := testclient.NewClientBuilder().WithScheme(k8sScheme).Build()

		route := &gwv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-route-1",
				Namespace: "test-ns",
			},
			Spec: gwv1.HTTPRouteSpec{},
		}
		k8sClient.Create(ctx, route)

		rc := routeReconciler{
			routeType: core.HttpRouteType,
			log:       gwlog.FallbackLogger,
			client:    k8sClient,
		}

		svcStatus := &latticemodel.ServiceStatus{
			Arn: "arn:aws:vpc-lattice:us-west-2:123456789012:service/svc-12345",
			Dns: "test-service.lattice.amazonaws.com",
		}

		coreRoute, _ := core.GetHTTPRoute(ctx, k8sClient, k8s.NamespacedName(route))
		err := rc.updateRouteStatusWithServiceInfo(ctx, coreRoute, svcStatus)
		assert.Nil(t, err)

		updatedRoute := &gwv1.HTTPRoute{}
		k8sClient.Get(ctx, k8s.NamespacedName(route), updatedRoute)
		annotations := updatedRoute.GetAnnotations()
		assert.Equal(t, "arn:aws:vpc-lattice:us-west-2:123456789012:service/svc-12345", annotations[LatticeServiceArn])
		assert.Equal(t, "test-service.lattice.amazonaws.com", annotations[LatticeAssignedDomainName])
	})

	t.Run("returns nil and sets no annotations when svcStatus is nil", func(t *testing.T) {
		k8sClient := testclient.NewClientBuilder().WithScheme(k8sScheme).Build()

		route := &gwv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-route-4",
				Namespace: "test-ns",
			},
			Spec: gwv1.HTTPRouteSpec{},
		}
		k8sClient.Create(ctx, route)

		rc := routeReconciler{
			routeType: core.HttpRouteType,
			log:       gwlog.FallbackLogger,
			client:    k8sClient,
		}

		coreRoute, _ := core.GetHTTPRoute(ctx, k8sClient, k8s.NamespacedName(route))
		err := rc.updateRouteStatusWithServiceInfo(ctx, coreRoute, nil)
		assert.Nil(t, err)

		updatedRoute := &gwv1.HTTPRoute{}
		k8sClient.Get(ctx, k8s.NamespacedName(route), updatedRoute)
		assert.Nil(t, updatedRoute.GetAnnotations())
	})
}

func TestUpdateRouteListenerStatus_UpdatesGatewayAttachedRoutes(t *testing.T) {
	ctx := context.Background()

	k8sScheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(k8sScheme)
	gwv1.Install(k8sScheme)
	gwv1alpha2.Install(k8sScheme)
	addOptionalCRDs(k8sScheme)

	t.Run("gateway attachedRoutes increments when route references it", func(t *testing.T) {
		gwClass := &gwv1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: "amazon-vpc-lattice",
			},
			Spec: gwv1.GatewayClassSpec{
				ControllerName: config.LatticeGatewayControllerName,
			},
		}

		gw := &gwv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-gateway",
				Namespace: "default",
			},
			Spec: gwv1.GatewaySpec{
				GatewayClassName: "amazon-vpc-lattice",
				Listeners: []gwv1.Listener{
					{
						Name:     "http",
						Port:     80,
						Protocol: gwv1.HTTPProtocolType,
						AllowedRoutes: &gwv1.AllowedRoutes{
							Kinds: []gwv1.RouteGroupKind{{Kind: "HTTPRoute"}},
						},
					},
				},
			},
		}

		sectionName := gwv1.SectionName("http")
		route := &gwv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-route",
				Namespace: "default",
			},
			Spec: gwv1.HTTPRouteSpec{
				CommonRouteSpec: gwv1.CommonRouteSpec{
					ParentRefs: []gwv1.ParentReference{
						{
							Name:        "test-gateway",
							SectionName: &sectionName,
						},
					},
				},
			},
		}

		k8sClient := testclient.NewClientBuilder().
			WithScheme(k8sScheme).
			WithObjects(gwClass, gw, route).
			WithStatusSubresource(gw).
			Build()

		coreRoute, err := core.GetHTTPRoute(ctx, k8sClient, k8s.NamespacedName(route))
		assert.NoError(t, err)

		err = updateRouteListenerStatus(ctx, k8sClient, coreRoute)
		assert.NoError(t, err)

		// Verify gateway status was updated
		updatedGw := &gwv1.Gateway{}
		err = k8sClient.Get(ctx, k8s.NamespacedName(gw), updatedGw)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(updatedGw.Status.Listeners))
		assert.Equal(t, int32(1), updatedGw.Status.Listeners[0].AttachedRoutes)
	})
}

func addOptionalCRDs(scheme *runtime.Scheme) {
	dnsEndpoint := schema.GroupVersion{
		Group:   "externaldns.k8s.io",
		Version: "v1alpha1",
	}
	scheme.AddKnownTypes(dnsEndpoint, &endpoint.DNSEndpoint{}, &endpoint.DNSEndpointList{})
	metav1.AddToGroupVersion(scheme, dnsEndpoint)

	awsGatewayControllerCRDGroupVersion := schema.GroupVersion{
		Group:   anv1alpha1.GroupName,
		Version: "v1alpha1",
	}
	scheme.AddKnownTypes(awsGatewayControllerCRDGroupVersion, &anv1alpha1.TargetGroupPolicy{}, &anv1alpha1.TargetGroupPolicyList{})
	metav1.AddToGroupVersion(scheme, awsGatewayControllerCRDGroupVersion)

	scheme.AddKnownTypes(awsGatewayControllerCRDGroupVersion, &anv1alpha1.VpcAssociationPolicy{}, &anv1alpha1.VpcAssociationPolicyList{})
	metav1.AddToGroupVersion(scheme, awsGatewayControllerCRDGroupVersion)
}

func TestRouteReconciler_CertificateNotFound(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()

	k8sScheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(k8sScheme)
	gwv1.Install(k8sScheme)
	gwv1alpha2.Install(k8sScheme)
	discoveryv1.AddToScheme(k8sScheme)
	addOptionalCRDs(k8sScheme)

	gwClass := &gwv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "amazon-vpc-lattice",
		},
		Spec: gwv1.GatewayClassSpec{
			ControllerName: config.LatticeGatewayControllerName,
		},
	}

	gw := &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-gateway",
			Namespace: "ns1",
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: "amazon-vpc-lattice",
			Listeners: []gwv1.Listener{
				{
					Name:     "http",
					Protocol: "HTTP",
					Port:     80,
				},
			},
		},
	}

	route := &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-route",
			Namespace: "ns1",
		},
		Spec: gwv1.HTTPRouteSpec{
			CommonRouteSpec: gwv1.CommonRouteSpec{
				ParentRefs: []gwv1.ParentReference{
					{
						Name: "my-gateway",
					},
				},
			},
		},
	}

	k8sClient := testclient.NewClientBuilder().
		WithScheme(k8sScheme).
		WithObjects(gwClass, gw, route).
		WithStatusSubresource(&gwv1.HTTPRoute{}).
		Build()

	mockBuilder := gateway.NewMockLatticeServiceBuilder(c)
	mockBuilder.EXPECT().Build(gomock.Any(), gomock.Any()).Return(
		nil, fmt.Errorf("%w for hostname app.example.com", gateway.ErrCertificateNotFound))

	mockEventRecorder := mock_client.NewMockEventRecorder(c)
	mockEventRecorder.EXPECT().Event(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	mockFinalizer := k8s.NewMockFinalizerManager(c)
	mockFinalizer.EXPECT().AddFinalizers(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	rc := routeReconciler{
		routeType:        core.HttpRouteType,
		log:              gwlog.FallbackLogger,
		client:           k8sClient,
		scheme:           k8sScheme,
		finalizerManager: mockFinalizer,
		eventRecorder:    mockEventRecorder,
		modelBuilder:     mockBuilder,
	}

	routeName := k8s.NamespacedName(route)
	result, err := rc.Reconcile(ctx, reconcile.Request{NamespacedName: routeName})
	assert.Nil(t, err)
	assert.Equal(t, 1*time.Minute, result.RequeueAfter)

	// Verify route status was updated with Accepted: False
	updatedRoute := &gwv1.HTTPRoute{}
	k8sClient.Get(ctx, routeName, updatedRoute)

	var acceptedCond *metav1.Condition
	for _, parent := range updatedRoute.Status.Parents {
		for i, cond := range parent.Conditions {
			if cond.Type == string(gwv1.RouteConditionAccepted) {
				acceptedCond = &parent.Conditions[i]
				break
			}
		}
	}
	assert.NotNil(t, acceptedCond)
	assert.Equal(t, metav1.ConditionFalse, acceptedCond.Status)
	assert.Equal(t, "CertificateNotFound", acceptedCond.Reason)
	assert.Contains(t, acceptedCond.Message, "no matching ACM certificate found")
}

func TestRouteReconciler_ExternalTargetGroupStatusSurfacing(t *testing.T) {
	const arn = "arn:aws:vpc-lattice:us-west-2:123456789012:targetgroup/tg-0df85aff983932f06"

	tests := []struct {
		name           string
		buildErr       error
		expectedReason string
		outcome        string
		expectedErr    error
	}{
		{
			name:           "malformed ARN -> InvalidExternalTargetGroup, error backoff",
			buildErr:       k8s.NewInvalidExternalTargetGroupError("not-an-arn", fmt.Errorf("not a valid ARN")),
			expectedReason: "InvalidExternalTargetGroup",
			outcome:        "errorBackoff",
			expectedErr:    ErrExternalTargetGroupInvalid,
		},
		{
			name:           "conflicting annotations -> ConflictingServiceImportAnnotations, error backoff",
			buildErr:       k8s.NewConflictingServiceImportAnnotationsError("target-group-arn together with export-name"),
			expectedReason: "ConflictingServiceImportAnnotations",
			outcome:        "errorBackoff",
			expectedErr:    ErrConflictingServiceImportConfig,
		},
		{
			name:           "TG not found -> ExternalTargetGroupNotFound, fixed 1-min requeue",
			buildErr:       k8s.NewExternalTargetGroupNotFoundError(arn, "us-west-2", fmt.Errorf("not found")),
			expectedReason: "ExternalTargetGroupNotFound",
			outcome:        "fixedRequeue",
		},
		{
			name:           "transient error -> no condition, bare return",
			buildErr:       fmt.Errorf("throttled"),
			expectedReason: "",
			outcome:        "errorNoCond",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := gomock.NewController(t)
			defer c.Finish()
			ctx := context.TODO()

			k8sScheme := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sScheme)
			gwv1.Install(k8sScheme)
			gwv1alpha2.Install(k8sScheme)
			discoveryv1.AddToScheme(k8sScheme)
			addOptionalCRDs(k8sScheme)

			gwClass := &gwv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "amazon-vpc-lattice"},
				Spec:       gwv1.GatewayClassSpec{ControllerName: config.LatticeGatewayControllerName},
			}
			gw := &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "my-gateway", Namespace: "ns1"},
				Spec: gwv1.GatewaySpec{
					GatewayClassName: "amazon-vpc-lattice",
					Listeners:        []gwv1.Listener{{Name: "http", Protocol: "HTTP", Port: 80}},
				},
			}
			route := &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "my-route", Namespace: "ns1"},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{{Name: "my-gateway"}},
					},
				},
			}

			k8sClient := testclient.NewClientBuilder().
				WithScheme(k8sScheme).
				WithObjects(gwClass, gw, route).
				WithStatusSubresource(&gwv1.HTTPRoute{}).
				Build()

			mockBuilder := gateway.NewMockLatticeServiceBuilder(c)
			mockBuilder.EXPECT().Build(gomock.Any(), gomock.Any()).Return(nil, tt.buildErr)

			mockEventRecorder := mock_client.NewMockEventRecorder(c)
			mockEventRecorder.EXPECT().Event(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

			mockFinalizer := k8s.NewMockFinalizerManager(c)
			mockFinalizer.EXPECT().AddFinalizers(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

			rc := routeReconciler{
				routeType:        core.HttpRouteType,
				log:              gwlog.FallbackLogger,
				client:           k8sClient,
				scheme:           k8sScheme,
				finalizerManager: mockFinalizer,
				eventRecorder:    mockEventRecorder,
				modelBuilder:     mockBuilder,
			}

			routeName := k8s.NamespacedName(route)
			result, err := rc.Reconcile(ctx, reconcile.Request{NamespacedName: routeName})

			updatedRoute := &gwv1.HTTPRoute{}
			assert.NoError(t, k8sClient.Get(ctx, routeName, updatedRoute))

			var resolvedRefsCond, acceptedCond *metav1.Condition
			for _, parent := range updatedRoute.Status.Parents {
				for i, cond := range parent.Conditions {
					switch cond.Type {
					case string(gwv1.RouteConditionResolvedRefs):
						resolvedRefsCond = &parent.Conditions[i]
					case string(gwv1.RouteConditionAccepted):
						acceptedCond = &parent.Conditions[i]
					}
				}
			}

			switch tt.outcome {
			case "errorNoCond":
				assert.Error(t, err)
				assert.Equal(t, time.Duration(0), result.RequeueAfter)
				if resolvedRefsCond != nil {
					assert.Equal(t, metav1.ConditionTrue, resolvedRefsCond.Status,
						"transient error must not flip ResolvedRefs to False")
				}
				return
			case "errorBackoff":
				assert.ErrorIs(t, err, tt.expectedErr)
				assert.Equal(t, time.Duration(0), result.RequeueAfter, "sentinel error must not set a fixed RequeueAfter")
			case "fixedRequeue":
				assert.NoError(t, err)
				assert.Equal(t, 1*time.Minute, result.RequeueAfter, "not-found must requeue after a fixed 1 min")
			default:
				t.Fatalf("unknown outcome %q", tt.outcome)
			}

			assert.NotNil(t, resolvedRefsCond, "ResolvedRefs condition must be persisted")
			assert.Equal(t, metav1.ConditionFalse, resolvedRefsCond.Status)
			assert.Equal(t, tt.expectedReason, resolvedRefsCond.Reason)
			assert.Equal(t, route.GetGeneration(), resolvedRefsCond.ObservedGeneration)

			if acceptedCond != nil {
				assert.NotEqual(t, metav1.ConditionFalse, acceptedCond.Status,
					"external-TG failure must leave Accepted untouched (not False)")
			}
		})
	}
}

func TestRouteReconciler_ExternalTargetGroupStatusWriteFailureNotMasked(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()

	k8sScheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(k8sScheme)
	gwv1.Install(k8sScheme)
	gwv1alpha2.Install(k8sScheme)
	discoveryv1.AddToScheme(k8sScheme)
	addOptionalCRDs(k8sScheme)

	gwClass := &gwv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{Name: "amazon-vpc-lattice"},
		Spec:       gwv1.GatewayClassSpec{ControllerName: config.LatticeGatewayControllerName},
	}
	gw := &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "my-gateway", Namespace: "ns1"},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: "amazon-vpc-lattice",
			Listeners:        []gwv1.Listener{{Name: "http", Protocol: "HTTP", Port: 80}},
		},
	}
	route := &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "my-route", Namespace: "ns1"},
		Spec: gwv1.HTTPRouteSpec{
			CommonRouteSpec: gwv1.CommonRouteSpec{
				ParentRefs: []gwv1.ParentReference{{Name: "my-gateway"}},
			},
		},
	}

	statusUpdateErr := fmt.Errorf("simulated status update failure")
	k8sClient := testclient.NewClientBuilder().
		WithScheme(k8sScheme).
		WithObjects(gwClass, gw, route).
		WithStatusSubresource(&gwv1.HTTPRoute{}).
		WithInterceptorFuncs(interceptor.Funcs{
			SubResourceUpdate: func(ctx context.Context, cl client.Client, subResourceName string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
				if subResourceName == "status" {
					return statusUpdateErr
				}
				return cl.Status().Update(ctx, obj, opts...)
			},
		}).
		Build()

	mockBuilder := gateway.NewMockLatticeServiceBuilder(c)
	mockBuilder.EXPECT().Build(gomock.Any(), gomock.Any()).Return(
		nil, k8s.NewExternalTargetGroupNotFoundError("arn:aws:vpc-lattice:us-west-2:123456789012:targetgroup/tg-0df85aff983932f06", "us-west-2", fmt.Errorf("not found"))).AnyTimes()

	mockEventRecorder := mock_client.NewMockEventRecorder(c)
	mockEventRecorder.EXPECT().Event(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	mockFinalizer := k8s.NewMockFinalizerManager(c)
	mockFinalizer.EXPECT().AddFinalizers(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	rc := routeReconciler{
		routeType:        core.HttpRouteType,
		log:              gwlog.FallbackLogger,
		client:           k8sClient,
		scheme:           k8sScheme,
		finalizerManager: mockFinalizer,
		eventRecorder:    mockEventRecorder,
		modelBuilder:     mockBuilder,
	}

	routeName := k8s.NamespacedName(route)
	_, err := rc.Reconcile(ctx, reconcile.Request{NamespacedName: routeName})

	assert.Error(t, err)
}

type noopStackDeployer struct{ deployed bool }

func (d *noopStackDeployer) Deploy(ctx context.Context, stack core.Stack) error {
	d.deployed = true
	return nil
}

func TestRouteReconciler_DeleteRoute_ExternalTgFinalizerNotStranded(t *testing.T) {
	config.VpcID = "my-vpc"
	config.ClusterName = "my-cluster"

	const (
		finalizer = "httproute.k8s.aws/resources"
		svcImport = "external-import"
	)

	tests := []struct {
		name        string
		annotations map[string]string
	}{
		{
			name:        "malformed external-TG ARN on delete -> finalizer removed",
			annotations: map[string]string{k8s.TargetGroupArnAnnotation: "not-an-arn"},
		},
		{
			name: "conflicting external-TG annotations on delete -> finalizer removed",
			annotations: map[string]string{
				k8s.TargetGroupArnAnnotation: "arn:aws:vpc-lattice:us-west-2:123456789012:targetgroup/tg-0df85aff983932f06",
				k8s.ExportNameAnnotation:     "some-export",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := gomock.NewController(t)
			defer c.Finish()
			ctx := context.TODO()

			k8sScheme := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sScheme)
			gwv1.Install(k8sScheme)
			gwv1alpha2.Install(k8sScheme)
			discoveryv1.AddToScheme(k8sScheme)
			anv1alpha1.Install(k8sScheme)
			addOptionalCRDs(k8sScheme)

			gwClass := &gwv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "amazon-vpc-lattice"},
				Spec:       gwv1.GatewayClassSpec{ControllerName: config.LatticeGatewayControllerName},
			}
			gw := &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "my-gateway", Namespace: "ns1"},
				Spec: gwv1.GatewaySpec{
					GatewayClassName: "amazon-vpc-lattice",
					Listeners:        []gwv1.Listener{{Name: "http", Protocol: "HTTP", Port: 80}},
				},
			}
			si := &anv1alpha1.ServiceImport{
				ObjectMeta: metav1.ObjectMeta{Name: svcImport, Namespace: "ns1", Annotations: tt.annotations},
			}
			kind := gwv1.Kind("ServiceImport")
			now := metav1.Now()
			route := &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "my-route",
					Namespace:         "ns1",
					DeletionTimestamp: &now,
					Finalizers:        []string{finalizer},
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{{Name: "my-gateway"}},
					},
					Rules: []gwv1.HTTPRouteRule{
						{
							BackendRefs: []gwv1.HTTPBackendRef{
								{BackendRef: gwv1.BackendRef{
									BackendObjectReference: gwv1.BackendObjectReference{
										Kind: &kind,
										Name: svcImport,
									},
									Weight: aws.Int32(90),
								}},
							},
						},
					},
				},
			}

			k8sClient := testclient.NewClientBuilder().
				WithScheme(k8sScheme).
				WithObjects(gwClass, gw, si, route).
				WithStatusSubresource(&gwv1.HTTPRoute{}, &gwv1.Gateway{}).
				Build()

			mockLattice := mocks.NewMockLattice(c)
			mockLattice.EXPECT().GetTargetGroup(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, _ interface{}, _ ...interface{}) (interface{}, error) {
					t.Fatalf("GetTargetGroup must not be called on the delete path")
					return nil, nil
				}).AnyTimes()

			mockEventRecorder := mock_client.NewMockEventRecorder(c)
			mockEventRecorder.EXPECT().Event(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

			brTgBuilder := gateway.NewBackendRefTargetGroupBuilder(gwlog.FallbackLogger, k8sClient)
			deployer := &noopStackDeployer{}

			rc := routeReconciler{
				routeType:        core.HttpRouteType,
				log:              gwlog.FallbackLogger,
				client:           k8sClient,
				scheme:           k8sScheme,
				finalizerManager: k8s.NewDefaultFinalizerManager(k8sClient),
				eventRecorder:    mockEventRecorder,
				modelBuilder:     gateway.NewLatticeServiceBuilder(gwlog.FallbackLogger, k8sClient, brTgBuilder, nil, mockLattice),
				stackDeployer:    deployer,
				stackMarshaller:  deploy.NewDefaultStackMarshaller(),
			}

			routeName := k8s.NamespacedName(route)
			result, err := rc.Reconcile(ctx, reconcile.Request{NamespacedName: routeName})

			assert.NoError(t, err)
			assert.Equal(t, time.Duration(0), result.RequeueAfter)

			updated := &gwv1.HTTPRoute{}
			getErr := k8sClient.Get(ctx, routeName, updated)
			if getErr == nil {
				assert.NotContains(t, updated.Finalizers, finalizer,
					"route finalizer must be removed on delete (not stuck Terminating)")
			} else {
				assert.True(t, apierrors.IsNotFound(getErr),
					"route should be gone after its last finalizer is removed; got: %v", getErr)
			}
		})
	}
}

func TestRouteReconciler_UpsertRoute_ExternalTgClientReachesGetTargetGroup(t *testing.T) {
	config.VpcID = "my-vpc"
	config.ClusterName = "my-cluster"

	const (
		svcImport = "external-import"
		arn       = "arn:aws:vpc-lattice:us-west-2:123456789012:targetgroup/tg-0df85aff983932f06"
	)

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()

	k8sScheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(k8sScheme)
	gwv1.Install(k8sScheme)
	gwv1alpha2.Install(k8sScheme)
	discoveryv1.AddToScheme(k8sScheme)
	anv1alpha1.Install(k8sScheme)
	addOptionalCRDs(k8sScheme)

	gwClass := &gwv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{Name: "amazon-vpc-lattice"},
		Spec:       gwv1.GatewayClassSpec{ControllerName: config.LatticeGatewayControllerName},
	}
	gw := &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "my-gateway", Namespace: "ns1"},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: "amazon-vpc-lattice",
			Listeners:        []gwv1.Listener{{Name: "http", Protocol: "HTTP", Port: 80}},
		},
	}
	si := &anv1alpha1.ServiceImport{
		ObjectMeta: metav1.ObjectMeta{
			Name:        svcImport,
			Namespace:   "ns1",
			Annotations: map[string]string{k8s.TargetGroupArnAnnotation: arn},
		},
	}
	kind := gwv1.Kind("ServiceImport")
	route := &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "my-route", Namespace: "ns1"},
		Spec: gwv1.HTTPRouteSpec{
			CommonRouteSpec: gwv1.CommonRouteSpec{
				ParentRefs: []gwv1.ParentReference{{Name: "my-gateway"}},
			},
			Rules: []gwv1.HTTPRouteRule{
				{
					BackendRefs: []gwv1.HTTPBackendRef{
						{BackendRef: gwv1.BackendRef{
							BackendObjectReference: gwv1.BackendObjectReference{Kind: &kind, Name: svcImport},
							Weight:                 aws.Int32(90),
						}},
					},
				},
			},
		},
	}

	k8sClient := testclient.NewClientBuilder().
		WithScheme(k8sScheme).
		WithObjects(gwClass, gw, si, route).
		WithStatusSubresource(&gwv1.HTTPRoute{}, &gwv1.Gateway{}).
		Build()

	var gotIdentifier string
	gtgCalls := 0
	mockLattice := mocks.NewMockLattice(c)
	mockLattice.EXPECT().GetTargetGroup(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, in *vpclattice.GetTargetGroupInput, _ ...func(*vpclattice.Options)) (*vpclattice.GetTargetGroupOutput, error) {
			gtgCalls++
			gotIdentifier = aws.ToString(in.TargetGroupIdentifier)
			return nil, mocks.NewNotFoundError("TargetGroup", arn)
		}).AnyTimes()

	mockEventRecorder := mock_client.NewMockEventRecorder(c)
	mockEventRecorder.EXPECT().Event(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	mockFinalizer := k8s.NewMockFinalizerManager(c)
	mockFinalizer.EXPECT().AddFinalizers(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	brTgBuilder := gateway.NewBackendRefTargetGroupBuilder(gwlog.FallbackLogger, k8sClient)
	rc := routeReconciler{
		routeType:        core.HttpRouteType,
		log:              gwlog.FallbackLogger,
		client:           k8sClient,
		scheme:           k8sScheme,
		finalizerManager: mockFinalizer,
		eventRecorder:    mockEventRecorder,
		modelBuilder:     gateway.NewLatticeServiceBuilder(gwlog.FallbackLogger, k8sClient, brTgBuilder, nil, mockLattice),
		stackDeployer:    &noopStackDeployer{},
		stackMarshaller:  deploy.NewDefaultStackMarshaller(),
	}

	routeName := k8s.NamespacedName(route)
	result, err := rc.Reconcile(ctx, reconcile.Request{NamespacedName: routeName})

	assert.Positive(t, gtgCalls, "GetTargetGroup must be called on the real upsert path")
	assert.Equal(t, arn, gotIdentifier, "GetTargetGroup must be called with the annotation ARN")
	assert.NoError(t, err)
	assert.Equal(t, 1*time.Minute, result.RequeueAfter)

	updated := &gwv1.HTTPRoute{}
	assert.NoError(t, k8sClient.Get(ctx, routeName, updated))
	var resolvedRefsCond *metav1.Condition
	for _, parent := range updated.Status.Parents {
		for i, cond := range parent.Conditions {
			if cond.Type == string(gwv1.RouteConditionResolvedRefs) {
				resolvedRefsCond = &parent.Conditions[i]
			}
		}
	}
	assert.NotNil(t, resolvedRefsCond, "ResolvedRefs condition must be persisted")
	assert.Equal(t, metav1.ConditionFalse, resolvedRefsCond.Status)
	assert.Equal(t, "ExternalTargetGroupNotFound", resolvedRefsCond.Reason)
}

type errStackDeployer struct{ err error }

func (d *errStackDeployer) Deploy(ctx context.Context, stack core.Stack) error { return d.err }

// newQuotaTestReconciler returns a reconciler and fake client holding a Lattice Gateway "my-gateway" in ns1
// with one HTTP listener, plus the given route.
func newQuotaTestReconciler(t *testing.T, c *gomock.Controller, route *gwv1.HTTPRoute, builder gateway.LatticeServiceBuilder,
	deployer deploy.StackDeployer, recorder *mock_client.MockEventRecorder) (routeReconciler, client.Client) {
	k8sScheme := runtime.NewScheme()
	assert.NoError(t, clientgoscheme.AddToScheme(k8sScheme))
	assert.NoError(t, gwv1.Install(k8sScheme))
	assert.NoError(t, gwv1alpha2.Install(k8sScheme))
	assert.NoError(t, discoveryv1.AddToScheme(k8sScheme))
	addOptionalCRDs(k8sScheme)

	gwClass := &gwv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{Name: "amazon-vpc-lattice"},
		Spec:       gwv1.GatewayClassSpec{ControllerName: config.LatticeGatewayControllerName},
	}
	gw := &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "my-gateway", Namespace: "ns1"},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: "amazon-vpc-lattice",
			Listeners:        []gwv1.Listener{{Name: "http", Protocol: "HTTP", Port: 80}},
		},
	}
	k8sClient := testclient.NewClientBuilder().
		WithScheme(k8sScheme).
		WithObjects(gwClass, gw, route).
		WithStatusSubresource(&gwv1.HTTPRoute{}).
		Build()

	if recorder == nil {
		recorder = mock_client.NewMockEventRecorder(c)
		recorder.EXPECT().Event(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	}
	finalizer := k8s.NewMockFinalizerManager(c)
	finalizer.EXPECT().AddFinalizers(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	return routeReconciler{
		routeType:        core.HttpRouteType,
		log:              gwlog.FallbackLogger,
		client:           k8sClient,
		scheme:           k8sScheme,
		finalizerManager: finalizer,
		eventRecorder:    recorder,
		modelBuilder:     builder,
		stackDeployer:    deployer,
		stackMarshaller:  deploy.NewDefaultStackMarshaller(),
	}, k8sClient
}

// routeWithPartiallyInvalid returns a route whose only parent already carries a PartiallyInvalid condition.
func routeWithPartiallyInvalid(parentRef gwv1.ParentReference) *gwv1.HTTPRoute {
	return &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "my-route", Namespace: "ns1"},
		Spec: gwv1.HTTPRouteSpec{
			CommonRouteSpec: gwv1.CommonRouteSpec{ParentRefs: []gwv1.ParentReference{parentRef}},
		},
		Status: gwv1.HTTPRouteStatus{RouteStatus: gwv1.RouteStatus{Parents: []gwv1.RouteParentStatus{{
			ParentRef:      parentRef,
			ControllerName: config.LatticeGatewayControllerName,
			Conditions: []metav1.Condition{{
				Type:               string(gwv1.RouteConditionPartiallyInvalid),
				Status:             metav1.ConditionTrue,
				Reason:             k8s.RouteEventReasonServiceQuotaExceeded,
				LastTransitionTime: metav1.Now(),
			}},
		}}}},
	}
}

func TestRouteReconciler_ServiceQuotaExceeded(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()

	route := &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "my-route", Namespace: "ns1"},
		Spec: gwv1.HTTPRouteSpec{
			CommonRouteSpec: gwv1.CommonRouteSpec{ParentRefs: []gwv1.ParentReference{{Name: "my-gateway"}}},
		},
	}

	mockBuilder := gateway.NewMockLatticeServiceBuilder(c)
	mockBuilder.EXPECT().Build(gomock.Any(), gomock.Any()).
		DoAndReturn(func(context.Context, core.Route) (core.Stack, error) {
			stack := core.NewDefaultStack(core.StackID{Name: "my-route", Namespace: "ns1"})
			svc, err := latticemodel.NewLatticeService(stack, latticemodel.ServiceSpec{
				ServiceTagFields: latticemodel.ServiceTagFields{RouteName: "my-route", RouteNamespace: "ns1"},
			})
			assert.NoError(t, err)
			svc.Status = &latticemodel.ServiceStatus{Arn: "svc-arn", Dns: "svc.dns"}
			return stack, nil
		}).AnyTimes()

	quotaEvents, deployFailedEvents := 0, 0
	mockEventRecorder := mock_client.NewMockEventRecorder(c)
	mockEventRecorder.EXPECT().Event(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Do(func(_ runtime.Object, _, reason, _ string) {
			switch reason {
			case k8s.RouteEventReasonServiceQuotaExceeded:
				quotaEvents++
			case k8s.RouteEventReasonFailedDeployModel:
				deployFailedEvents++
			}
		}).AnyTimes()

	quotaErr := &types.ServiceQuotaExceededException{Message: aws.String("rules per listener limit reached")}
	quotaDeployErr := fmt.Errorf("error during rule synthesis %w",
		&lattice.RuleQuotaExceededError{
			Dropped: []lattice.DroppedRule{{RuleIndex: 10, RuleName: "checkout", Priority: 11, ListenerPort: 80, ListenerProtocol: "HTTP"}},
			Applied: 10,
			Err:     fmt.Errorf("failed CreateRule due to %w", quotaErr),
		})
	deployer := &errStackDeployer{err: quotaDeployErr}
	rc, k8sClient := newQuotaTestReconciler(t, c, route, mockBuilder, deployer, mockEventRecorder)

	findCond := func(condType gwv1.RouteConditionType) *metav1.Condition {
		got := &gwv1.HTTPRoute{}
		assert.NoError(t, k8sClient.Get(ctx, k8s.NamespacedName(route), got))
		for _, parent := range got.Status.Parents {
			if cnd := meta.FindStatusCondition(parent.Conditions, string(condType)); cnd != nil {
				return cnd
			}
		}
		return nil
	}

	req := reconcile.Request{NamespacedName: k8s.NamespacedName(route)}
	result, err := rc.Reconcile(ctx, req)
	assert.NoError(t, err)
	assert.Equal(t, 1*time.Minute, result.RequeueAfter)

	// the service is live, so its ARN/DNS are still published from the partially deployed stack
	published := &gwv1.HTTPRoute{}
	assert.NoError(t, k8sClient.Get(ctx, k8s.NamespacedName(route), published))
	assert.Equal(t, "svc-arn", published.Annotations[LatticeServiceArn])
	assert.Equal(t, "svc.dns", published.Annotations[LatticeAssignedDomainName])

	partial := findCond(gwv1.RouteConditionPartiallyInvalid)
	if assert.NotNil(t, partial) {
		assert.Equal(t, metav1.ConditionTrue, partial.Status)
		assert.Equal(t, k8s.RouteEventReasonServiceQuotaExceeded, partial.Reason)
		assert.True(t, strings.HasSuffix(partial.Message, ". VPC Lattice error: rules per listener limit reached"))
	}
	// the route still serves its last programmed state, so Accepted must not be flipped
	accepted := findCond(gwv1.RouteConditionAccepted)
	if assert.NotNil(t, accepted) {
		assert.Equal(t, metav1.ConditionTrue, accepted.Status)
	}

	// the same failure again is logged but not re-emitted as an event, and the generic event is not used
	_, err = rc.Reconcile(ctx, req)
	assert.NoError(t, err)
	assert.Equal(t, 1, quotaEvents)
	assert.Equal(t, 0, deployFailedEvents)

	// an unrelated failure must not hide that rules are still missing
	deployer.err = errors.New("throttled")
	_, err = rc.Reconcile(ctx, req)
	assert.Error(t, err)
	assert.NotNil(t, findCond(gwv1.RouteConditionPartiallyInvalid))

	// a quota error from a step other than rule creation is not reported as dropped rules
	deployFailedEvents = 0
	deployer.err = fmt.Errorf("error during service synthesis %w", quotaErr)
	_, err = rc.Reconcile(ctx, req)
	assert.Error(t, err)
	assert.Equal(t, 1, quotaEvents)
	assert.Equal(t, 1, deployFailedEvents)

	// once the deploy succeeds the condition goes away
	deployer.err = nil
	_, err = rc.Reconcile(ctx, req)
	assert.NoError(t, err)
	assert.Nil(t, findCond(gwv1.RouteConditionPartiallyInvalid))

	// a new failure after that is reported again
	deployer.err = quotaDeployErr
	_, err = rc.Reconcile(ctx, req)
	assert.NoError(t, err)
	assert.Equal(t, 2, quotaEvents)
}

// The spec forbids PartiallyInvalid on a parent that did not accept the route.
func TestRouteReconciler_PartiallyInvalidNeverOnRejectedParent(t *testing.T) {
	quotaDeployer := &errStackDeployer{err: &lattice.RuleQuotaExceededError{
		Dropped: []lattice.DroppedRule{{RuleIndex: 0, Priority: 1, ListenerPort: 80, ListenerProtocol: "HTTP"}},
		Err:     &types.ServiceQuotaExceededException{},
	}}
	noSuchListener := gwv1.SectionName("no-such-listener")

	tests := []struct {
		name      string
		parentRef gwv1.ParentReference
		build     func(c *gomock.Controller) gateway.LatticeServiceBuilder
		deployer  deploy.StackDeployer
	}{
		{
			// a rejected parent neither keeps the condition from an earlier reconcile nor gets it from a new failure
			name:      "listener does not match",
			parentRef: gwv1.ParentReference{Name: "my-gateway", SectionName: &noSuchListener},
			build: func(c *gomock.Controller) gateway.LatticeServiceBuilder {
				b := gateway.NewMockLatticeServiceBuilder(c)
				b.EXPECT().Build(gomock.Any(), gomock.Any()).
					Return(core.NewDefaultStack(core.StackID{Name: "my-route", Namespace: "ns1"}), nil)
				return b
			},
			deployer: quotaDeployer,
		},
		{
			name:      "conflict sets Accepted=False",
			parentRef: gwv1.ParentReference{Name: "my-gateway"},
			build: func(c *gomock.Controller) gateway.LatticeServiceBuilder {
				b := gateway.NewMockLatticeServiceBuilder(c)
				b.EXPECT().Build(gomock.Any(), gomock.Any()).
					Return(nil, mocks.NewConflictError("service", "ns1/my-route", "owned elsewhere"))
				return b
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := gomock.NewController(t)
			defer c.Finish()
			ctx := context.TODO()
			route := routeWithPartiallyInvalid(tt.parentRef)
			rc, k8sClient := newQuotaTestReconciler(t, c, route, tt.build(c), tt.deployer, nil)

			_, err := rc.Reconcile(ctx, reconcile.Request{NamespacedName: k8s.NamespacedName(route)})
			assert.NoError(t, err)

			got := &gwv1.HTTPRoute{}
			assert.NoError(t, k8sClient.Get(ctx, k8s.NamespacedName(route), got))
			if assert.Len(t, got.Status.Parents, 1) {
				conds := got.Status.Parents[0].Conditions
				assert.False(t, meta.IsStatusConditionTrue(conds, string(gwv1.RouteConditionAccepted)))
				assert.Nil(t, meta.FindStatusCondition(conds, string(gwv1.RouteConditionPartiallyInvalid)))
			}
		})
	}
}

func TestDroppedRulesMessage(t *testing.T) {
	tests := []struct {
		name        string
		ruleErr     *lattice.RuleQuotaExceededError
		contains    []string
		notContains []string
		suffix      string
	}{
		{
			name: "rules listed once each, in spec order",
			ruleErr: &lattice.RuleQuotaExceededError{Applied: 1, Dropped: []lattice.DroppedRule{
				{RuleIndex: 12, Priority: 13, ListenerPort: 80, ListenerProtocol: "HTTP"},
				{RuleIndex: 3, RuleName: "api", Priority: 4, ListenerPort: 80, ListenerProtocol: "HTTP"},
				{RuleIndex: 3, RuleName: "api", Priority: 4, ListenerPort: 443, ListenerProtocol: "HTTPS"},
			}},
			contains: []string{`Not created: rules[3] "api" (priority 4, listener HTTP:80, HTTPS:443), ` +
				"rules[12] (priority 13, listener HTTP:80)."},
			notContains: []string{"Not updated"},
			suffix:      "All other rules were applied",
		},
		{
			name: "not created and not updated listed separately",
			ruleErr: &lattice.RuleQuotaExceededError{Applied: 1, Dropped: []lattice.DroppedRule{
				{RuleIndex: 4, Priority: 5, ListenerPort: 80, ListenerProtocol: "HTTP", NotUpdated: true},
				{RuleIndex: 10, Priority: 11, ListenerPort: 80, ListenerProtocol: "HTTP"},
			}},
			contains: []string{
				"Not created: rules[10] (priority 11, listener HTTP:80).",
				"Not updated, still using their previous backends: rules[4] (priority 5, listener HTTP:80).",
			},
			suffix: "All other rules were applied",
		},
		{
			name: "no claim that other rules were applied when none were",
			ruleErr: &lattice.RuleQuotaExceededError{Dropped: []lattice.DroppedRule{
				{RuleIndex: 0, Priority: 1, ListenerPort: 80, ListenerProtocol: "HTTP"},
			}},
			notContains: []string{"All other rules were applied"},
			suffix:      "Not created: rules[0] (priority 1, listener HTTP:80)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := droppedRulesMessage(tt.ruleErr)
			assert.True(t, strings.HasPrefix(msg, "Dropped Rule"), msg)
			assert.True(t, strings.HasSuffix(msg, tt.suffix), msg)
			for _, s := range tt.contains {
				assert.Contains(t, msg, s)
			}
			for _, s := range tt.notContains {
				assert.NotContains(t, msg, s)
			}
		})
	}
}
