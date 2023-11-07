package gateway

import (
	"context"
	"errors"
	mock_client "github.com/aws/aws-application-networking-k8s/mocks/controller-runtime/client"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	"testing"
)

// PortNumberPtr translates an int to a *PortNumber
func PortNumberPtr(p int) *gwv1beta1.PortNumber {
	result := gwv1beta1.PortNumber(p)
	return &result
}

func Test_ListenerModelBuild(t *testing.T) {
	var httpSectionName gwv1beta1.SectionName = "http"
	var missingSectionName gwv1beta1.SectionName = "miss"
	var serviceKind gwv1beta1.Kind = "Service"
	var backendRef = gwv1beta1.BackendRef{
		BackendObjectReference: gwv1beta1.BackendObjectReference{
			Name: "targetgroup1",
			Kind: &serviceKind,
		},
	}

	tests := []struct {
		name               string
		gwListenerPort     gwv1beta1.PortNumber
		route              core.Route
		wantErrIsNil       bool
		k8sGetGatewayCall  bool
		k8sGatewayReturnOK bool
		tlsTerminate       bool
		expectedSpec       []model.ListenerSpec
	}{
		{
			name:               "listener, default service action",
			gwListenerPort:     *PortNumberPtr(80),
			wantErrIsNil:       true,
			k8sGetGatewayCall:  true,
			k8sGatewayReturnOK: true,
			tlsTerminate:       false,
			route: core.NewHTTPRoute(gwv1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gwv1beta1.HTTPRouteSpec{
					CommonRouteSpec: gwv1beta1.CommonRouteSpec{
						ParentRefs: []gwv1beta1.ParentReference{
							{
								Name:        "gw1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []gwv1beta1.HTTPRouteRule{
						{
							BackendRefs: []gwv1beta1.HTTPBackendRef{
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
				},
			},
		},
		{
			name:               "tls listener",
			gwListenerPort:     *PortNumberPtr(443),
			wantErrIsNil:       true,
			k8sGetGatewayCall:  true,
			k8sGatewayReturnOK: true,
			tlsTerminate:       true,
			route: core.NewHTTPRoute(gwv1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gwv1beta1.HTTPRouteSpec{
					CommonRouteSpec: gwv1beta1.CommonRouteSpec{
						ParentRefs: []gwv1beta1.ParentReference{
							{
								Name:        "gw1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []gwv1beta1.HTTPRouteRule{
						{
							BackendRefs: []gwv1beta1.HTTPBackendRef{
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
				},
			},
		},
		{
			name:              "no parentref",
			gwListenerPort:    *PortNumberPtr(80),
			wantErrIsNil:      true,
			k8sGetGatewayCall: false,
			route: core.NewHTTPRoute(gwv1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gwv1beta1.HTTPRouteSpec{
					CommonRouteSpec: gwv1beta1.CommonRouteSpec{
						ParentRefs: []gwv1beta1.ParentReference{},
					},
					Rules: []gwv1beta1.HTTPRouteRule{
						{
							BackendRefs: []gwv1beta1.HTTPBackendRef{
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
			route: core.NewHTTPRoute(gwv1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gwv1beta1.HTTPRouteSpec{
					CommonRouteSpec: gwv1beta1.CommonRouteSpec{
						ParentRefs: []gwv1beta1.ParentReference{
							{
								Name:        "gw1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []gwv1beta1.HTTPRouteRule{
						{
							BackendRefs: []gwv1beta1.HTTPBackendRef{
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
			route: core.NewHTTPRoute(gwv1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gwv1beta1.HTTPRouteSpec{
					CommonRouteSpec: gwv1beta1.CommonRouteSpec{
						ParentRefs: []gwv1beta1.ParentReference{
							{
								Name:        "gw1",
								SectionName: &missingSectionName,
							},
						},
					},
					Rules: []gwv1beta1.HTTPRouteRule{
						{
							BackendRefs: []gwv1beta1.HTTPBackendRef{
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

			if tt.k8sGetGatewayCall {
				mockK8sClient.EXPECT().Get(ctx, gomock.Any(), gomock.Any()).DoAndReturn(
					func(ctx context.Context, gwName types.NamespacedName, gw *gwv1beta1.Gateway, arg3 ...interface{}) error {
						if !tt.k8sGatewayReturnOK {
							return errors.New("unknown k8s object")
						}
						listener := gwv1beta1.Listener{
							Port:     tt.gwListenerPort,
							Protocol: "HTTP",
							Name:     httpSectionName,
						}

						if tt.tlsTerminate {
							listener.Protocol = "HTTPS"
							mode := gwv1beta1.TLSModeTerminate
							listener.TLS = &gwv1beta1.GatewayTLSConfig{
								Mode: &mode,
							}
						}

						gw.Spec.Listeners = append(gw.Spec.Listeners, listener)
						return nil
					},
				)
			}

			stack := core.NewDefaultStack(core.StackID(k8s.NamespacedName(tt.route.K8sObject())))

			task := &latticeServiceModelBuildTask{
				log:    gwlog.FallbackLogger,
				route:  tt.route,
				client: mockK8sClient,
				stack:  stack,
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
			}
		})
	}
}
