package gateway

import (
	"context"
	"testing"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

type K8sGatewayListenerType int

const (
	HTTP K8sGatewayListenerType = iota
	HTTPS
	TLS_PASSTHROUGH
)

func Test_ListenerModelBuild(t *testing.T) {
	var sectionName gwv1.SectionName = "my-gw-listener"
	var missingSectionName gwv1.SectionName = "miss"
	var serviceKind gwv1.Kind = "Service"
	var serviceImportKind gwv1.Kind = "ServiceImport"
	var backendRef = gwv1.BackendRef{
		BackendObjectReference: gwv1.BackendObjectReference{
			Name: "targetgroup1",
			Kind: &serviceKind,
		},
	}
	vpcLatticeGatewayClass := gwv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gwClass",
		},
		Spec: gwv1.GatewayClassSpec{
			ControllerName: config.LatticeGatewayControllerName,
		},
	}
	vpcLatticeGateway := gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gateway1",
			Namespace: "default",
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: gwv1.ObjectName(vpcLatticeGatewayClass.Name),
		},
	}
	vpcLatticeGatewayWithListeners := func(listeners ...gwv1.Listener) gwv1.Gateway {
		gw := vpcLatticeGateway.DeepCopy()
		gw.Spec.Listeners = listeners
		return *gw
	}

	tlsModePassthrough := gwv1.TLSModePassthrough
	tlsModeTerminate := gwv1.TLSModeTerminate
	serviceImportName := gwv1.ObjectName("k8s-service3")

	tests := []struct {
		name                    string
		gw                      gwv1.Gateway
		route                   core.Route
		wantErrIsNil            bool
		k8sGetGatewayCall       bool
		brTgBuilderBuildCall    bool
		k8sGetServiceImportCall bool
		expectedSpec            []model.ListenerSpec
	}{
		{
			name:              "Build HTTP listener",
			wantErrIsNil:      true,
			k8sGetGatewayCall: true,
			gw: vpcLatticeGatewayWithListeners(
				gwv1.Listener{
					Port:     80,
					Protocol: "HTTP",
					Name:     sectionName,
				}),
			route: core.NewHTTPRoute(gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name:        gwv1.ObjectName(vpcLatticeGateway.Name),
								SectionName: &sectionName,
							},
						},
					},
					Rules: []gwv1.HTTPRouteRule{
						{
							BackendRefs: []gwv1.HTTPBackendRef{
								{
									BackendRef: backendRef,
								},
							},
						},
					},
				},
			}),
			expectedSpec: []model.ListenerSpec{
				{
					StackServiceId:    "svc-id",
					K8SRouteName:      "service1",
					K8SRouteNamespace: "default",
					Port:              80,
					Protocol:          "HTTP",
					DefaultAction: &model.DefaultAction{
						FixedResponseStatusCode: aws.Int64(404),
					},
				},
			},
		},
		{
			name:              "Build HTTPS listener",
			wantErrIsNil:      true,
			k8sGetGatewayCall: true,
			gw: vpcLatticeGatewayWithListeners(
				gwv1.Listener{
					Port:     443,
					Protocol: "HTTPS",
					Name:     sectionName,
					TLS: &gwv1.GatewayTLSConfig{
						Mode: &tlsModeTerminate,
					},
				}),
			route: core.NewHTTPRoute(gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name:        gwv1.ObjectName(vpcLatticeGateway.Name),
								SectionName: &sectionName,
							},
						},
					},
					Rules: []gwv1.HTTPRouteRule{
						{
							BackendRefs: []gwv1.HTTPBackendRef{
								{
									BackendRef: backendRef,
								},
							},
						},
					},
				},
			}),
			expectedSpec: []model.ListenerSpec{
				{
					StackServiceId:    "svc-id",
					K8SRouteName:      "service1",
					K8SRouteNamespace: "default",
					Port:              443,
					Protocol:          "HTTPS",
					DefaultAction: &model.DefaultAction{
						FixedResponseStatusCode: aws.Int64(404),
					},
				},
			},
		},
		{
			name:                    "Build TLS_PASSTHROUGH listener",
			wantErrIsNil:            true,
			k8sGetGatewayCall:       true,
			k8sGetServiceImportCall: true,
			brTgBuilderBuildCall:    true,
			gw: vpcLatticeGatewayWithListeners(
				gwv1.Listener{
					Port:     443,
					Protocol: "TLS",
					Name:     sectionName,
					TLS: &gwv1.GatewayTLSConfig{
						Mode: &tlsModePassthrough,
					},
				}),
			route: core.NewTLSRoute(gwv1alpha2.TLSRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gwv1alpha2.TLSRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name:        gwv1.ObjectName(vpcLatticeGateway.Name),
								SectionName: &sectionName,
							},
						},
					},
					Rules: []gwv1alpha2.TLSRouteRule{
						{
							BackendRefs: []gwv1.BackendRef{
								{
									BackendObjectReference: gwv1.BackendObjectReference{
										Name: "k8s-service1",
										Kind: &serviceKind,
										// No weight specified, default to 1
									},
								},
								{
									BackendObjectReference: gwv1.BackendObjectReference{
										Name: "k8s-service2",
										Kind: &serviceKind,
									},
									Weight: aws.Int32(10),
								},
								{
									BackendObjectReference: gwv1.BackendObjectReference{
										Name: serviceImportName,
										Kind: &serviceImportKind,
									},
									Weight: aws.Int32(90),
								},
							},
						},
					},
				},
			}),
			expectedSpec: []model.ListenerSpec{
				{
					StackServiceId:    "svc-id",
					K8SRouteName:      "service1",
					K8SRouteNamespace: "default",
					Port:              443,
					Protocol:          vpclattice.ListenerProtocolTlsPassthrough,
					DefaultAction: &model.DefaultAction{
						Forward: &model.RuleAction{
							TargetGroups: []*model.RuleTargetGroup{
								{
									StackTargetGroupId: "k8s-service1",
									Weight:             1, // No weight specified, default to 1
								},
								{
									StackTargetGroupId: "k8s-service2",
									Weight:             10,
								},
								{
									SvcImportTG: &model.SvcImportTargetGroup{
										K8SServiceNamespace: "default",
										K8SServiceName:      string(serviceImportName),
										VpcId:               "vpc-123",
										K8SClusterName:      "eks-cluster",
									},
									Weight: 90,
								},
							},
						},
					},
				},
			},
		},
		{
			name:                    "TLSRoute has more than one rule, build TLS_PASSTHROUGH listener failed",
			wantErrIsNil:            false,
			k8sGetGatewayCall:       true,
			k8sGetServiceImportCall: false,
			gw: vpcLatticeGatewayWithListeners(
				gwv1.Listener{
					Port:     443,
					Protocol: "TLS",
					Name:     sectionName,
					TLS: &gwv1.GatewayTLSConfig{
						Mode: &tlsModePassthrough,
					},
				}),
			route: core.NewTLSRoute(gwv1alpha2.TLSRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gwv1alpha2.TLSRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name:        gwv1.ObjectName(vpcLatticeGateway.Name),
								SectionName: &sectionName,
							},
						},
					},
					Rules: []gwv1alpha2.TLSRouteRule{
						{
							BackendRefs: []gwv1.BackendRef{
								{
									BackendObjectReference: gwv1.BackendObjectReference{
										Name: "k8s-service1",
										Kind: &serviceKind,
									},
								},
							},
						},
						{
							BackendRefs: []gwv1.BackendRef{
								{
									BackendObjectReference: gwv1.BackendObjectReference{
										Name: "k8s-service2",
										Kind: &serviceKind,
									},
								},
							},
						},
					},
				},
			}),
			expectedSpec: []model.ListenerSpec{
				{
					StackServiceId:    "svc-id",
					K8SRouteName:      "service1",
					K8SRouteNamespace: "default",
					Port:              443,
					Protocol:          vpclattice.ListenerProtocolTlsPassthrough,
					DefaultAction: &model.DefaultAction{
						Forward: &model.RuleAction{
							TargetGroups: []*model.RuleTargetGroup{
								{
									StackTargetGroupId: "k8s-service1",
									Weight:             10,
								},
								{
									SvcImportTG: &model.SvcImportTargetGroup{
										K8SServiceNamespace: "default",
										K8SServiceName:      "k8s-service2",
										VpcId:               "vpc-123",
										K8SClusterName:      "eks-cluster",
									},
									Weight: 90,
								},
							},
						},
					},
				},
			},
		},
		{
			name:              "no parentref",
			wantErrIsNil:      true,
			k8sGetGatewayCall: false,
			route: core.NewHTTPRoute(gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{},
					},
					Rules: []gwv1.HTTPRouteRule{
						{
							BackendRefs: []gwv1.HTTPBackendRef{
								{
									BackendRef: backendRef,
								},
							},
						},
					},
				},
			}),
			expectedSpec: []model.ListenerSpec{}, // empty list
		},
		{
			name:              "No k8sgateway object",
			wantErrIsNil:      false,
			k8sGetGatewayCall: false,
			route: core.NewHTTPRoute(gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name:        gwv1.ObjectName(vpcLatticeGateway.Name),
								SectionName: &sectionName,
							},
						},
					},
					Rules: []gwv1.HTTPRouteRule{
						{
							BackendRefs: []gwv1.HTTPBackendRef{
								{
									BackendRef: backendRef,
								},
							},
						},
					},
				},
			}),
		},
		{
			name:              "No gateway managed by vpc lattice",
			wantErrIsNil:      false,
			k8sGetGatewayCall: true,
			gw: gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "non-lattice",
					Namespace: "default",
				},
				Spec: gwv1.GatewaySpec{
					GatewayClassName: gwv1.ObjectName("gwClass"),
				},
			},
			route: core.NewHTTPRoute(gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name:        "non-lattice",
								SectionName: &sectionName,
							},
						},
					},
					Rules: []gwv1.HTTPRouteRule{
						{
							BackendRefs: []gwv1.HTTPBackendRef{
								{
									BackendRef: backendRef,
								},
							},
						},
					},
				},
			}),
		},
		{
			name:              "no section name",
			wantErrIsNil:      false,
			k8sGetGatewayCall: true,
			gw: vpcLatticeGatewayWithListeners(
				gwv1.Listener{
					Port:     80,
					Protocol: "HTTP",
					Name:     sectionName,
				}),
			route: core.NewHTTPRoute(gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name:        gwv1.ObjectName(vpcLatticeGateway.Name),
								SectionName: &missingSectionName,
							},
						},
					},
					Rules: []gwv1.HTTPRouteRule{
						{
							BackendRefs: []gwv1.HTTPBackendRef{
								{
									BackendRef: backendRef,
								},
							},
						},
					},
				},
			}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := gomock.NewController(t)
			defer c.Finish()
			ctx := context.TODO()

			k8sSchema := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sSchema)
			gwv1.Install(k8sSchema)
			anv1alpha1.Install(k8sSchema)
			k8sClient := testclient.NewClientBuilder().WithScheme(k8sSchema).Build()

			assert.NoError(t, k8sClient.Create(ctx, vpcLatticeGatewayClass.DeepCopy()))
			if tt.k8sGetGatewayCall {
				assert.NoError(t, k8sClient.Create(ctx, tt.gw.DeepCopy()))
			}

			mockBrTgBuilder := NewMockBackendRefTargetGroupModelBuilder(c)
			stack := core.NewDefaultStack(core.StackID(k8s.NamespacedName(tt.route.K8sObject())))

			if tt.k8sGetServiceImportCall {
				k8sClient.Create(ctx, &anv1alpha1.ServiceImport{
					ObjectMeta: metav1.ObjectMeta{
						Name:      string(serviceImportName),
						Namespace: "default",
						Annotations: map[string]string{
							"application-networking.k8s.aws/aws-vpc":              "vpc-123",
							"application-networking.k8s.aws/aws-eks-cluster-name": "eks-cluster",
						},
					},
				})
			}

			if tt.brTgBuilderBuildCall {
				gomock.InOrder(
					mockBrTgBuilder.EXPECT().Build(ctx, tt.route, gomock.Any(), gomock.Any()).
						Return(nil, &model.TargetGroup{
							ResourceMeta: core.NewResourceMeta(stack, "AWS:VPCServiceNetwork::TargetGroup", "k8s-service1"),
						}, nil),
					mockBrTgBuilder.EXPECT().Build(ctx, tt.route, gomock.Any(), gomock.Any()).
						Return(nil, &model.TargetGroup{
							ResourceMeta: core.NewResourceMeta(stack, "AWS:VPCServiceNetwork::TargetGroup", "k8s-service2"),
						}, nil),
				)
			}
			task := &latticeServiceModelBuildTask{
				log:         gwlog.FallbackLogger,
				route:       tt.route,
				client:      k8sClient,
				stack:       stack,
				brTgBuilder: mockBrTgBuilder,
			}

			err := task.buildListeners(ctx, "svc-id")

			if !tt.wantErrIsNil {
				assert.NotNil(t, err)
				return
			}

			assert.NoError(t, err)

			var resListener []*model.Listener
			stack.ListResources(&resListener)

			assert.Equal(t, len(tt.expectedSpec), len(resListener))

			for i, expected := range tt.expectedSpec {
				actual := resListener[i].Spec

				assert.Equal(t, expected.StackServiceId, actual.StackServiceId)
				assert.Equal(t, expected.K8SRouteName, actual.K8SRouteName)
				assert.Equal(t, expected.K8SRouteNamespace, actual.K8SRouteNamespace)
				assert.Equal(t, expected.Port, actual.Port)
				assert.Equal(t, expected.Protocol, actual.Protocol)
				assert.Equal(t, expected.DefaultAction, actual.DefaultAction)
			}
		})
	}
}

func Test_ListenerModelBuild_HTTPRouteWithAndWithoutAdditionalTagsAnnotation(t *testing.T) {
	var sectionName gwv1.SectionName = "my-gw-listener"
	var serviceKind gwv1.Kind = "Service"
	var backendRef = gwv1.BackendRef{
		BackendObjectReference: gwv1.BackendObjectReference{
			Name: "targetgroup1",
			Kind: &serviceKind,
		},
	}

	vpcLatticeGatewayClass := gwv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gwClass",
		},
		Spec: gwv1.GatewayClassSpec{
			ControllerName: config.LatticeGatewayControllerName,
		},
	}

	vpcLatticeGateway := gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gateway1",
			Namespace: "default",
		},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: gwv1.ObjectName(vpcLatticeGatewayClass.Name),
			Listeners: []gwv1.Listener{
				{
					Port:     80,
					Protocol: "HTTP",
					Name:     sectionName,
				},
			},
		},
	}

	tests := []struct {
		name                   string
		route                  core.Route
		expectedAdditionalTags k8s.Tags
		description            string
	}{
		{
			name: "HTTPRoute with additional tags annotation",
			route: core.NewHTTPRoute(gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "route-with-tags",
					Namespace: "default",
					Annotations: map[string]string{
						k8s.TagsAnnotationKey: "Environment=Prod,Project=ListenerTest,Team=Platform",
					},
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name:        gwv1.ObjectName(vpcLatticeGateway.Name),
								SectionName: &sectionName,
							},
						},
					},
					Rules: []gwv1.HTTPRouteRule{
						{
							BackendRefs: []gwv1.HTTPBackendRef{
								{
									BackendRef: backendRef,
								},
							},
						},
					},
				},
			}),
			expectedAdditionalTags: k8s.Tags{
				"Environment": &[]string{"Prod"}[0],
				"Project":     &[]string{"ListenerTest"}[0],
				"Team":        &[]string{"Platform"}[0],
			},
			description: "should set additional tags from HTTPRoute annotations in listener spec",
		},
		{
			name: "HTTPRoute without additional tags annotation",
			route: core.NewHTTPRoute(gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "route-no-tags",
					Namespace: "default",
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name:        gwv1.ObjectName(vpcLatticeGateway.Name),
								SectionName: &sectionName,
							},
						},
					},
					Rules: []gwv1.HTTPRouteRule{
						{
							BackendRefs: []gwv1.HTTPBackendRef{
								{
									BackendRef: backendRef,
								},
							},
						},
					},
				},
			}),
			expectedAdditionalTags: nil,
			description:            "should have nil additional tags when no annotation present in listener spec",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := gomock.NewController(t)
			defer c.Finish()
			ctx := context.TODO()

			k8sSchema := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sSchema)
			gwv1.Install(k8sSchema)
			anv1alpha1.Install(k8sSchema)
			k8sClient := testclient.NewClientBuilder().WithScheme(k8sSchema).Build()

			assert.NoError(t, k8sClient.Create(ctx, vpcLatticeGatewayClass.DeepCopy()))
			assert.NoError(t, k8sClient.Create(ctx, vpcLatticeGateway.DeepCopy()))

			mockBrTgBuilder := NewMockBackendRefTargetGroupModelBuilder(c)
			stack := core.NewDefaultStack(core.StackID(k8s.NamespacedName(tt.route.K8sObject())))

			task := &latticeServiceModelBuildTask{
				log:         gwlog.FallbackLogger,
				route:       tt.route,
				client:      k8sClient,
				stack:       stack,
				brTgBuilder: mockBrTgBuilder,
			}

			err := task.buildListeners(ctx, "svc-id")
			assert.NoError(t, err, tt.description)

			var resListener []*model.Listener
			stack.ListResources(&resListener)
			assert.Equal(t, 1, len(resListener), "Expected exactly one listener")

			actualListener := resListener[0]
			assert.Equal(t, tt.expectedAdditionalTags, actualListener.Spec.AdditionalTags, tt.description)
		})
	}
}
