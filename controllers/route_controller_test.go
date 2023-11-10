package controllers

import (
	"context"
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
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/external-dns/endpoint"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	"testing"
)

func TestRouteReconciler_ReconcileCreates(t *testing.T) {
	config.VpcID = "my-vpc"
	config.ClusterName = "my-cluster"

	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()

	k8sScheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(k8sScheme)
	gwv1beta1.AddToScheme(k8sScheme)
	addOptionalCRDs(k8sScheme)

	k8sClient := testclient.NewFakeClientWithScheme(k8sScheme)

	gwClass := &gwv1beta1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "amazon-vpc-lattice",
			Namespace: defaultNamespace,
		},
		Spec: gwv1beta1.GatewayClassSpec{
			ControllerName: config.LatticeGatewayControllerName,
		},
		Status: gwv1beta1.GatewayClassStatus{},
	}
	k8sClient.Create(ctx, gwClass.DeepCopy())

	// here we have a gateway, service, and route
	gw := &gwv1beta1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-gateway",
			Namespace: "ns1",
		},
		Spec: gwv1beta1.GatewaySpec{
			GatewayClassName: "amazon-vpc-lattice",
			Listeners: []gwv1beta1.Listener{
				{
					Name:     "http",
					Protocol: "HTTP",
					Port:     80,
				},
			},
		},
	}
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

	endPoints := corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "my-service",
			Name:      "ns1",
		},
		Subsets: []corev1.EndpointSubset{
			{
				Addresses: []corev1.EndpointAddress{
					{
						IP: "192.0.2.22",
					},
					{
						IP: "192.0.2.33",
					},
				},
				Ports: []corev1.EndpointPort{
					{
						Port: 8090,
					},
				},
			},
		},
	}
	k8sClient.Create(ctx, endPoints.DeepCopy())

	kind := gwv1beta1.Kind("Service")
	port := gwv1beta1.PortNumber(80)
	route := gwv1beta1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-route",
			Namespace: "ns1",
		},
		Spec: gwv1beta1.HTTPRouteSpec{
			CommonRouteSpec: gwv1beta1.CommonRouteSpec{
				ParentRefs: []gwv1beta1.ParentReference{
					{
						Name: "my-gateway",
					},
				},
			},
			Rules: []gwv1beta1.HTTPRouteRule{
				{
					BackendRefs: []gwv1beta1.HTTPBackendRef{
						{
							BackendRef: gwv1beta1.BackendRef{
								BackendObjectReference: gwv1beta1.BackendObjectReference{
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
	mockCloud.EXPECT().Lattice().Return(mockLattice).AnyTimes()
	mockCloud.EXPECT().Config().Return(
		aws2.CloudConfig{
			VpcId:       config.VpcID,
			AccountId:   "account-id",
			Region:      "ep-imagine-1",
			ClusterName: config.ClusterName,
		}).AnyTimes()
	mockCloud.EXPECT().DefaultTags().Return(mocks.Tags{}).AnyTimes()
	mockCloud.EXPECT().DefaultTagsMergedWith(gomock.Any()).Return(mocks.Tags{}).AnyTimes()

	// we expect a fair number of lattice calls
	mockLattice.EXPECT().FindServiceNetwork(ctx, gomock.Any(), gomock.Any()).Return(
		&mocks.ServiceNetworkInfo{
			SvcNetwork: vpclattice.ServiceNetworkSummary{
				Arn:  aws.String("sn-arn"),
				Id:   aws.String("sn-id"),
				Name: aws.String("sn-name"),
			},
		}, nil)
	mockLattice.EXPECT().FindService(ctx, gomock.Any()).Return(
		nil, mocks.NewNotFoundError("Service", "svc-name")) // will trigger create
	mockLattice.EXPECT().CreateServiceWithContext(ctx, gomock.Any()).Return(
		&vpclattice.CreateServiceOutput{
			Arn:    aws.String("svc-arn"),
			Id:     aws.String("svc-id"),
			Name:   aws.String("svc-name"),
			Status: aws.String(vpclattice.ServiceStatusActive),
		}, nil)
	mockLattice.EXPECT().CreateServiceNetworkServiceAssociationWithContext(ctx, gomock.Any()).Return(
		&vpclattice.CreateServiceNetworkServiceAssociationOutput{
			Arn:    aws.String("sns-assoc-arn"),
			Id:     aws.String("sns-assoc-id"),
			Status: aws.String(vpclattice.ServiceNetworkServiceAssociationStatusActive),
		}, nil)
	mockLattice.EXPECT().FindService(ctx, gomock.Any()).Return(
		&vpclattice.ServiceSummary{
			Arn:    aws.String("svc-arn"),
			Id:     aws.String("svc-id"),
			Name:   aws.String("svc-name"),
			Status: aws.String(vpclattice.ServiceStatusActive),
			DnsEntry: &vpclattice.DnsEntry{
				DomainName:   aws.String("my-fqdn.lattice.on.aws"),
				HostedZoneId: aws.String("my-hosted-zone"),
			},
		}, nil) // will trigger DNS Update

	mockLattice.EXPECT().ListTargetGroupsAsList(ctx, gomock.Any()).Return(
		[]*vpclattice.TargetGroupSummary{}, nil).AnyTimes() // this will cause us to skip "unused delete" step
	mockLattice.EXPECT().CreateTargetGroupWithContext(ctx, gomock.Any()).Return(
		&vpclattice.CreateTargetGroupOutput{
			Arn:    aws.String("tg-arn"),
			Id:     aws.String("tg-id"),
			Name:   aws.String("tg-name"),
			Status: aws.String(vpclattice.TargetGroupStatusActive),
		}, nil)

	mockLattice.EXPECT().ListListenersWithContext(ctx, gomock.Any()).Return(
		&vpclattice.ListListenersOutput{
			Items:     []*vpclattice.ListenerSummary{},
			NextToken: nil,
		}, nil).AnyTimes()
	mockLattice.EXPECT().CreateListenerWithContext(ctx, gomock.Any()).Return(
		&vpclattice.CreateListenerOutput{
			Arn:        aws.String("listener-arn"),
			Id:         aws.String("listener-id"),
			Name:       aws.String("listener-name"),
			ServiceArn: aws.String("svc-arn"),
			ServiceId:  aws.String("svc-id"),
		}, nil)

	mockLattice.EXPECT().GetRulesAsList(ctx, gomock.Any()).Return(
		[]*vpclattice.GetRuleOutput{}, nil)
	mockLattice.EXPECT().CreateRuleWithContext(ctx, gomock.Any()).Return(
		&vpclattice.CreateRuleOutput{
			Arn:      aws.String("rule-arn"),
			Id:       aws.String("rule-id"),
			Name:     aws.String("rule-name"),
			Priority: aws.Int64(1),
		}, nil)
	// List is called after create, so we'll return what we have
	mockLattice.EXPECT().ListRulesAsList(ctx, gomock.Any()).Return(
		[]*vpclattice.RuleSummary{
			{
				Arn:       aws.String("rule-arn"),
				Id:        aws.String("rule-id"),
				IsDefault: aws.Bool(false),
				Name:      aws.String("rule-name"),
				Priority:  aws.Int64(1),
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
		modelBuilder:     gateway.NewLatticeServiceBuilder(gwlog.FallbackLogger, k8sClient, brTgBuilder),
		stackDeployer:    deploy.NewLatticeServiceStackDeploy(gwlog.FallbackLogger, mockCloud, k8sClient),
		stackMarshaller:  deploy.NewDefaultStackMarshaller(),
		cloud:            mockCloud,
	}

	routeName := k8s.NamespacedName(&route)
	result, err := rc.Reconcile(ctx, reconcile.Request{NamespacedName: routeName})
	assert.Nil(t, err)
	assert.False(t, result.Requeue)

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
