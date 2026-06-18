package controllers

import (
	"context"
	"fmt"
	"testing"
	"time"

	mock_client "github.com/aws/aws-application-networking-k8s/mocks/controller-runtime/client"
	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	aws2 "github.com/aws/aws-application-networking-k8s/pkg/aws"
	mocks "github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/deploy"
	"github.com/aws/aws-application-networking-k8s/pkg/gateway"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/vpclattice"
	"github.com/aws/aws-sdk-go-v2/service/vpclattice/types"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
		}, nil)
	mockLattice.EXPECT().CreateServiceNetworkServiceAssociation(gomock.Any(), gomock.Any()).Return(
		&vpclattice.CreateServiceNetworkServiceAssociationOutput{
			Arn:    aws.String("sns-assoc-arn"),
			Id:     aws.String("sns-assoc-id"),
			Status: types.ServiceNetworkServiceAssociationStatusActive,
		}, nil)
	mockLattice.EXPECT().FindService(gomock.Any(), gomock.Any()).Return(
		&types.ServiceSummary{
			Arn:    aws.String("svc-arn"),
			Id:     aws.String("svc-id"),
			Name:   aws.String("svc-name"),
			Status: types.ServiceStatusActive,
			DnsEntry: &types.DnsEntry{
				DomainName:   aws.String("my-fqdn.lattice.on.aws"),
				HostedZoneId: aws.String("my-hosted-zone"),
			},
		}, nil) // will trigger DNS Update

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

}

func TestRouteReconciler_UpdateRouteStatusWithServiceInfo(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()

	k8sScheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(k8sScheme)
	gwv1.Install(k8sScheme)

	mockCloud := aws2.NewMockCloud(c)
	mockLattice := mocks.NewMockLattice(c)
	mockCloud.EXPECT().Lattice().Return(mockLattice).AnyTimes()

	t.Run("updates route with service ARN and DNS when both available", func(t *testing.T) {
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
			cloud:     mockCloud,
		}

		// Mock service with both ARN and DNS
		mockLattice.EXPECT().FindService(gomock.Any(), gomock.Any()).Return(
			&types.ServiceSummary{
				Arn:  aws.String("arn:aws:vpc-lattice:us-west-2:123456789012:service/svc-12345"),
				Name: aws.String("test-service"),
				DnsEntry: &types.DnsEntry{
					DomainName: aws.String("test-service.lattice.amazonaws.com"),
				},
			}, nil)

		coreRoute, _ := core.GetHTTPRoute(ctx, k8sClient, k8s.NamespacedName(route))
		err := rc.updateRouteStatusWithServiceInfo(ctx, coreRoute)
		assert.Nil(t, err)

		// Verify annotations were set
		updatedRoute := &gwv1.HTTPRoute{}
		k8sClient.Get(ctx, k8s.NamespacedName(route), updatedRoute)
		annotations := updatedRoute.GetAnnotations()
		assert.Equal(t, "arn:aws:vpc-lattice:us-west-2:123456789012:service/svc-12345", annotations[LatticeServiceArn])
		assert.Equal(t, "test-service.lattice.amazonaws.com", annotations[LatticeAssignedDomainName])
	})

	t.Run("updates route with only DNS when ARN not available", func(t *testing.T) {
		k8sClient := testclient.NewClientBuilder().WithScheme(k8sScheme).Build()

		route := &gwv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-route-2",
				Namespace: "test-ns",
			},
			Spec: gwv1.HTTPRouteSpec{},
		}
		k8sClient.Create(ctx, route)

		rc := routeReconciler{
			routeType: core.HttpRouteType,
			log:       gwlog.FallbackLogger,
			client:    k8sClient,
			cloud:     mockCloud,
		}

		// Mock service with only DNS
		mockLattice.EXPECT().FindService(gomock.Any(), gomock.Any()).Return(
			&types.ServiceSummary{
				Name: aws.String("test-service"),
				DnsEntry: &types.DnsEntry{
					DomainName: aws.String("test-service.lattice.amazonaws.com"),
				},
			}, nil)

		coreRoute, _ := core.GetHTTPRoute(ctx, k8sClient, k8s.NamespacedName(route))
		err := rc.updateRouteStatusWithServiceInfo(ctx, coreRoute)
		assert.Nil(t, err)

		// Verify only DNS annotation was set
		updatedRoute := &gwv1.HTTPRoute{}
		k8sClient.Get(ctx, k8s.NamespacedName(route), updatedRoute)
		annotations := updatedRoute.GetAnnotations()
		_, arnExists := annotations[LatticeServiceArn]
		assert.False(t, arnExists, "ARN annotation should not exist when ARN is not available")
		assert.Equal(t, "test-service.lattice.amazonaws.com", annotations[LatticeAssignedDomainName])
	})

	t.Run("updates route with only ARN when DNS not available", func(t *testing.T) {
		k8sClient := testclient.NewClientBuilder().WithScheme(k8sScheme).Build()

		route := &gwv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-route-3",
				Namespace: "test-ns",
			},
			Spec: gwv1.HTTPRouteSpec{},
		}
		k8sClient.Create(ctx, route)

		rc := routeReconciler{
			routeType: core.HttpRouteType,
			log:       gwlog.FallbackLogger,
			client:    k8sClient,
			cloud:     mockCloud,
		}

		// Mock service with only ARN
		mockLattice.EXPECT().FindService(gomock.Any(), gomock.Any()).Return(
			&types.ServiceSummary{
				Arn:  aws.String("arn:aws:vpc-lattice:us-west-2:123456789012:service/svc-12345"),
				Name: aws.String("test-service"),
			}, nil)

		coreRoute, _ := core.GetHTTPRoute(ctx, k8sClient, k8s.NamespacedName(route))
		err := rc.updateRouteStatusWithServiceInfo(ctx, coreRoute)
		assert.Nil(t, err)

		// Verify only ARN annotation was set
		updatedRoute := &gwv1.HTTPRoute{}
		k8sClient.Get(ctx, k8s.NamespacedName(route), updatedRoute)
		annotations := updatedRoute.GetAnnotations()
		assert.Equal(t, "arn:aws:vpc-lattice:us-west-2:123456789012:service/svc-12345", annotations[LatticeServiceArn])
		_, dnsExists := annotations[LatticeAssignedDomainName]
		assert.False(t, dnsExists, "DNS annotation should not exist when DNS is not available")
	})

	t.Run("handles service not found gracefully", func(t *testing.T) {
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
			cloud:     mockCloud,
		}

		// Mock service not found
		mockLattice.EXPECT().FindService(gomock.Any(), gomock.Any()).Return(
			nil, mocks.NewNotFoundError("Service", "test-service"))

		coreRoute, _ := core.GetHTTPRoute(ctx, k8sClient, k8s.NamespacedName(route))
		err := rc.updateRouteStatusWithServiceInfo(ctx, coreRoute)
		assert.Nil(t, err)

		// Verify no annotations were set
		updatedRoute := &gwv1.HTTPRoute{}
		k8sClient.Get(ctx, k8s.NamespacedName(route), updatedRoute)
		annotations := updatedRoute.GetAnnotations()
		assert.Nil(t, annotations)
	})

	t.Run("handles service lookup error", func(t *testing.T) {
		k8sClient := testclient.NewClientBuilder().WithScheme(k8sScheme).Build()

		route := &gwv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-route-5",
				Namespace: "test-ns",
			},
			Spec: gwv1.HTTPRouteSpec{},
		}
		k8sClient.Create(ctx, route)

		rc := routeReconciler{
			routeType: core.HttpRouteType,
			log:       gwlog.FallbackLogger,
			client:    k8sClient,
			cloud:     mockCloud,
		}

		// Mock service lookup error
		mockLattice.EXPECT().FindService(gomock.Any(), gomock.Any()).Return(
			nil, assert.AnError)

		coreRoute, _ := core.GetHTTPRoute(ctx, k8sClient, k8s.NamespacedName(route))
		err := rc.updateRouteStatusWithServiceInfo(ctx, coreRoute)
		assert.NotNil(t, err)
		assert.Equal(t, assert.AnError, err)
	})

	t.Run("handles nil service gracefully", func(t *testing.T) {
		k8sClient := testclient.NewClientBuilder().WithScheme(k8sScheme).Build()

		route := &gwv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-route-6",
				Namespace: "test-ns",
			},
			Spec: gwv1.HTTPRouteSpec{},
		}
		k8sClient.Create(ctx, route)

		rc := routeReconciler{
			routeType: core.HttpRouteType,
			log:       gwlog.FallbackLogger,
			client:    k8sClient,
			cloud:     mockCloud,
		}

		// Mock nil service
		mockLattice.EXPECT().FindService(gomock.Any(), gomock.Any()).Return(nil, nil)

		coreRoute, _ := core.GetHTTPRoute(ctx, k8sClient, k8s.NamespacedName(route))
		err := rc.updateRouteStatusWithServiceInfo(ctx, coreRoute)
		assert.Nil(t, err)

		// Verify no annotations were set
		updatedRoute := &gwv1.HTTPRoute{}
		k8sClient.Get(ctx, k8s.NamespacedName(route), updatedRoute)
		annotations := updatedRoute.GetAnnotations()
		assert.Nil(t, annotations)
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
