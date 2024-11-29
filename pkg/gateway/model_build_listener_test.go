package gateway

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	mock_client "github.com/aws/aws-application-networking-k8s/mocks/controller-runtime/client"
	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

type K8sGatewayListenerType int

const (
	HTTP K8sGatewayListenerType = iota
	HTTPS
	TLS_PASSTHROUGH
)

// PortNumberPtr translates an int to a *PortNumber
func PortNumberPtr(p int) *gwv1.PortNumber {
	result := gwv1.PortNumber(p)
	return &result
}

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

	tests := []struct {
		name                    string
		gwListenerPort          gwv1.PortNumber
		route                   core.Route
		wantErrIsNil            bool
		k8sGetGatewayCall       bool
		brTgBuilderBuildCall    bool
		k8sGetServiceImportCall bool
		k8sGatewayReturnOK      bool
		k8sGatewayListenerType  K8sGatewayListenerType
		expectedSpec            []model.ListenerSpec
	}{
		{
			name:                   "Build HTTP listener",
			gwListenerPort:         *PortNumberPtr(80),
			wantErrIsNil:           true,
			k8sGetGatewayCall:      true,
			k8sGatewayReturnOK:     true,
			k8sGatewayListenerType: HTTP,
			route: core.NewHTTPRoute(gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name:        "gw1",
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
			name:                   "Build HTTPS listener",
			gwListenerPort:         *PortNumberPtr(443),
			wantErrIsNil:           true,
			k8sGetGatewayCall:      true,
			k8sGatewayReturnOK:     true,
			k8sGatewayListenerType: HTTPS,
			route: core.NewHTTPRoute(gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name:        "gw1",
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
			gwListenerPort:          *PortNumberPtr(443),
			wantErrIsNil:            true,
			k8sGetGatewayCall:       true,
			k8sGetServiceImportCall: true,
			k8sGatewayReturnOK:      true,
			brTgBuilderBuildCall:    true,
			k8sGatewayListenerType:  TLS_PASSTHROUGH,
			route: core.NewTLSRoute(gwv1alpha2.TLSRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gwv1alpha2.TLSRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name:        "gw1",
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
										Name: "k8s-service3",
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
										K8SServiceName:      "k8s-service3",
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
			gwListenerPort:          *PortNumberPtr(443),
			wantErrIsNil:            false,
			k8sGetGatewayCall:       true,
			k8sGetServiceImportCall: false,
			k8sGatewayReturnOK:      true,
			k8sGatewayListenerType:  TLS_PASSTHROUGH,
			route: core.NewTLSRoute(gwv1alpha2.TLSRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gwv1alpha2.TLSRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name:        "gw1",
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
			gwListenerPort:    *PortNumberPtr(80),
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
			name:               "No k8sgateway object",
			gwListenerPort:     *PortNumberPtr(80),
			wantErrIsNil:       false,
			k8sGetGatewayCall:  true,
			k8sGatewayReturnOK: false,
			route: core.NewHTTPRoute(gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name:        "gw1",
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
			name:               "no section name",
			gwListenerPort:     *PortNumberPtr(80),
			wantErrIsNil:       false,
			k8sGetGatewayCall:  true,
			k8sGatewayReturnOK: true,
			route: core.NewHTTPRoute(gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name:        "gw1",
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

			mockK8sClient := mock_client.NewMockClient(c)
			mockBrTgBuilder := NewMockBackendRefTargetGroupModelBuilder(c)
			stack := core.NewDefaultStack(core.StackID(k8s.NamespacedName(tt.route.K8sObject())))

			if tt.k8sGetGatewayCall {
				mockK8sClient.EXPECT().Get(ctx, gomock.Any(), gomock.AssignableToTypeOf(&gwv1.Gateway{})).DoAndReturn(
					func(ctx context.Context, gwName types.NamespacedName, gw *gwv1.Gateway, arg3 ...interface{}) error {
						if !tt.k8sGatewayReturnOK {
							return errors.New("unknown k8s object")
						}
						var gwListener gwv1.Listener
						switch tt.k8sGatewayListenerType {
						case HTTP:
							gwListener = gwv1.Listener{
								Port:     tt.gwListenerPort,
								Protocol: "HTTP",
								Name:     sectionName,
							}
						case HTTPS:
							mode := gwv1.TLSModeTerminate
							gwListener = gwv1.Listener{
								Port:     tt.gwListenerPort,
								Protocol: "HTTPS",
								Name:     sectionName,
								TLS: &gwv1.GatewayTLSConfig{
									Mode: &mode,
								},
							}
						case TLS_PASSTHROUGH:
							mode := gwv1.TLSModePassthrough
							gwListener = gwv1.Listener{
								Port:     tt.gwListenerPort,
								Protocol: "TLS",
								Name:     sectionName,
								TLS: &gwv1.GatewayTLSConfig{
									Mode: &mode,
								},
							}
						}
						gw.Spec.Listeners = append(gw.Spec.Listeners, gwListener)
						return nil
					},
				)
			}
			if tt.k8sGetServiceImportCall {
				mockK8sClient.EXPECT().Get(ctx, gomock.Any(), gomock.AssignableToTypeOf(&anv1alpha1.ServiceImport{})).DoAndReturn(
					func(ctx context.Context, svcName types.NamespacedName, svcImport *anv1alpha1.ServiceImport, arg3 ...interface{}) error {
						svcImport.Annotations = make(map[string]string)
						svcImport.Annotations["application-networking.k8s.aws/aws-vpc"] = "vpc-123"
						svcImport.Annotations["application-networking.k8s.aws/aws-eks-cluster-name"] = "eks-cluster"
						return nil
					},
				)
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
				client:      mockK8sClient,
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
