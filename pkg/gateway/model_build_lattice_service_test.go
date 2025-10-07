package gateway

import (
	"context"
	"testing"

	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func Test_LatticeServiceModelBuild(t *testing.T) {
	now := metav1.Now()
	var httpSectionName gwv1.SectionName = "http"
	var serviceKind gwv1.Kind = "Service"
	var serviceimportKind gwv1.Kind = "ServiceImport"
	var weight1 = int32(10)
	var weight2 = int32(90)
	var namespace = gwv1.Namespace("default")

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

	namespacePtr := func(ns string) *gwv1.Namespace {
		p := gwv1.Namespace(ns)
		return &p
	}

	var backendRef1 = gwv1.BackendRef{
		BackendObjectReference: gwv1.BackendObjectReference{
			Name:      "targetgroup1",
			Namespace: &namespace,
			Kind:      &serviceKind,
		},
		Weight: &weight1,
	}
	var backendRef2 = gwv1.BackendRef{
		BackendObjectReference: gwv1.BackendObjectReference{
			Name:      "targetgroup2",
			Namespace: &namespace,
			Kind:      &serviceimportKind,
		},
		Weight: &weight2,
	}

	tlsSectionName := gwv1.SectionName("tls")
	tlsModeTerminate := gwv1.TLSModeTerminate

	tests := []struct {
		name          string
		gwClass       gwv1.GatewayClass
		gws           []gwv1.Gateway
		route         core.Route
		wantErrIsNil  bool
		wantIsDeleted bool
		expected      model.ServiceSpec
	}{
		{
			name:          "Add LatticeService with hostname",
			wantIsDeleted: false,
			wantErrIsNil:  true,
			gwClass:       vpcLatticeGatewayClass,
			gws:           []gwv1.Gateway{vpcLatticeGateway},
			route: core.NewHTTPRoute(gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "test",
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name:      gwv1.ObjectName(vpcLatticeGateway.Name),
								Namespace: namespacePtr(vpcLatticeGateway.Namespace),
							},
						},
					},
					Hostnames: []gwv1.Hostname{
						"test1.test.com",
						"test2.test.com",
					},
				},
			}),
			expected: model.ServiceSpec{
				ServiceTagFields: model.ServiceTagFields{
					RouteName:      "service1",
					RouteNamespace: "test",
					RouteType:      core.HttpRouteType,
				},
				CustomerDomainName:  "test1.test.com",
				ServiceNetworkNames: []string{vpcLatticeGateway.Name},
			},
		},
		{
			name:          "Add LatticeService",
			wantIsDeleted: false,
			wantErrIsNil:  true,
			gwClass:       vpcLatticeGatewayClass,
			gws: []gwv1.Gateway{
				vpcLatticeGateway,
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
								Name:      gwv1.ObjectName(vpcLatticeGateway.Name),
								Namespace: namespacePtr(vpcLatticeGateway.Namespace),
							},
						},
					},
				},
			}),
			expected: model.ServiceSpec{
				ServiceTagFields: model.ServiceTagFields{
					RouteName:      "service1",
					RouteNamespace: "default",
					RouteType:      core.HttpRouteType,
				},
				ServiceNetworkNames: []string{vpcLatticeGateway.Name},
			},
		},
		{
			name:          "Add LatticeService with GRPCRoute",
			wantIsDeleted: false,
			wantErrIsNil:  true,
			gwClass:       vpcLatticeGatewayClass,
			gws: []gwv1.Gateway{
				vpcLatticeGateway,
			},
			route: core.NewGRPCRoute(gwv1.GRPCRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "test",
				},
				Spec: gwv1.GRPCRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name:      gwv1.ObjectName(vpcLatticeGateway.Name),
								Namespace: namespacePtr(vpcLatticeGateway.Namespace),
							},
						},
					},
				},
			}),
			expected: model.ServiceSpec{
				ServiceTagFields: model.ServiceTagFields{
					RouteName:      "service1",
					RouteNamespace: "test",
					RouteType:      core.GrpcRouteType,
				},
				ServiceNetworkNames: []string{vpcLatticeGateway.Name},
			},
		},
		{
			name:          "Delete LatticeService",
			wantIsDeleted: true,
			wantErrIsNil:  true,
			gwClass:       vpcLatticeGatewayClass,
			gws: []gwv1.Gateway{
				vpcLatticeGateway,
			},
			route: core.NewHTTPRoute(gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "service2",
					Namespace:         "ns1",
					Finalizers:        []string{"gateway.k8s.aws/resources"},
					DeletionTimestamp: &now, // <- the important bit
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name:        gwv1.ObjectName(vpcLatticeGateway.Name),
								Namespace:   namespacePtr(vpcLatticeGateway.Namespace),
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []gwv1.HTTPRouteRule{
						{
							BackendRefs: []gwv1.HTTPBackendRef{
								{
									BackendRef: backendRef1,
								},
								{
									BackendRef: backendRef2,
								},
							},
						},
					},
				},
			}),
			expected: model.ServiceSpec{
				ServiceTagFields: model.ServiceTagFields{
					RouteName:      "service2",
					RouteNamespace: "ns1",
					RouteType:      core.HttpRouteType,
				},
				ServiceNetworkNames: []string{vpcLatticeGateway.Name},
			},
		},
		{
			name:          "Service with customer Cert ARN",
			wantIsDeleted: false,
			wantErrIsNil:  true,
			gwClass:       vpcLatticeGatewayClass,
			gws: []gwv1.Gateway{
				{
					ObjectMeta: vpcLatticeGateway.ObjectMeta,
					Spec: gwv1.GatewaySpec{
						GatewayClassName: gwv1.ObjectName(vpcLatticeGatewayClass.Name),
						Listeners: []gwv1.Listener{
							{
								Name:     "tls",
								Port:     443,
								Protocol: "HTTPS",
								TLS: &gwv1.GatewayTLSConfig{
									Mode:            &tlsModeTerminate,
									CertificateRefs: nil,
									Options: map[gwv1.AnnotationKey]gwv1.AnnotationValue{
										"application-networking.k8s.aws/certificate-arn": "cert-arn",
									},
								},
							},
						},
					},
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
								Name:        gwv1.ObjectName(vpcLatticeGateway.Name),
								Namespace:   namespacePtr(vpcLatticeGateway.Namespace),
								SectionName: &tlsSectionName,
							},
						},
					},
				},
			}),
			expected: model.ServiceSpec{
				ServiceTagFields: model.ServiceTagFields{
					RouteName:      "service1",
					RouteNamespace: "default",
					RouteType:      core.HttpRouteType,
				},
				CustomerCertARN:     "cert-arn",
				ServiceNetworkNames: []string{vpcLatticeGateway.Name},
			},
		},
		{
			name:    "GW does not exist",
			gwClass: vpcLatticeGatewayClass,
			gws: []gwv1.Gateway{
				vpcLatticeGateway,
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
								Name:      "not-a-real-gateway",
								Namespace: namespacePtr("default"),
							},
						},
					},
				},
			}),
			wantErrIsNil: false,
		},
		{
			name:          "Service with TLS section but no cert arn",
			wantIsDeleted: false,
			wantErrIsNil:  true,
			gwClass:       vpcLatticeGatewayClass,
			gws: []gwv1.Gateway{
				{
					ObjectMeta: vpcLatticeGateway.ObjectMeta,
					Spec: gwv1.GatewaySpec{
						GatewayClassName: gwv1.ObjectName(vpcLatticeGatewayClass.Name),
						Listeners: []gwv1.Listener{
							{
								Name:     "tls",
								Port:     443,
								Protocol: "HTTPS",
								TLS: &gwv1.GatewayTLSConfig{
									Mode:            &tlsModeTerminate,
									CertificateRefs: nil,
								},
							},
						},
					},
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
								Name:        gwv1.ObjectName(vpcLatticeGateway.Name),
								Namespace:   namespacePtr(vpcLatticeGateway.Namespace),
								SectionName: &tlsSectionName,
							},
						},
					},
				},
			}),
			expected: model.ServiceSpec{
				ServiceTagFields: model.ServiceTagFields{
					RouteName:      "service1",
					RouteNamespace: "default",
					RouteType:      core.HttpRouteType,
				},
				ServiceNetworkNames: []string{vpcLatticeGateway.Name},
			},
		},
		{
			name:          "Multiple service networks",
			wantIsDeleted: false,
			wantErrIsNil:  true,
			gwClass:       vpcLatticeGatewayClass,
			gws: []gwv1.Gateway{
				vpcLatticeGateway,
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gateway2",
						Namespace: "ns2",
					},
					Spec: gwv1.GatewaySpec{
						GatewayClassName: gwv1.ObjectName(vpcLatticeGatewayClass.Name),
					},
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
								Name:      gwv1.ObjectName(vpcLatticeGateway.Name),
								Namespace: namespacePtr(vpcLatticeGateway.Namespace),
							},
							{
								Name:      "gateway2",
								Namespace: namespacePtr("ns2"),
							},
						},
					},
				},
			}),
			expected: model.ServiceSpec{
				ServiceTagFields: model.ServiceTagFields{
					RouteName:      "service1",
					RouteNamespace: "default",
					RouteType:      core.HttpRouteType,
				},
				ServiceNetworkNames: []string{vpcLatticeGateway.Name, "gateway2"},
			},
		},
		{
			name:          "TLSRoute without hostname should fail",
			wantIsDeleted: false,
			wantErrIsNil:  false,
			gwClass:       vpcLatticeGatewayClass,
			gws: []gwv1.Gateway{
				vpcLatticeGateway,
			},
			route: core.NewTLSRoute(gwv1alpha2.TLSRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gwv1alpha2.TLSRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name:      gwv1.ObjectName(vpcLatticeGateway.Name),
								Namespace: namespacePtr(vpcLatticeGateway.Namespace),
							},
						},
					},
				},
			}),
		},
		{
			name:          "Multiple service networks with one different controller",
			wantIsDeleted: false,
			wantErrIsNil:  true,
			gwClass:       vpcLatticeGatewayClass,
			gws: []gwv1.Gateway{
				vpcLatticeGateway,
				// managed by different controller gateway
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "not-lattice",
						Namespace: "ns2",
					},
					Spec: gwv1.GatewaySpec{
						GatewayClassName: gwv1.ObjectName("not-lattice-gwClass"),
					},
				},
			},
			route: core.NewHTTPRoute(gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						// has two parent refs and one is not managed by lattice
						ParentRefs: []gwv1.ParentReference{
							{
								Name:      gwv1.ObjectName(vpcLatticeGateway.Name),
								Namespace: namespacePtr(vpcLatticeGateway.Namespace),
							},
							{
								Name:      "not-lattice",
								Namespace: namespacePtr("ns2"),
							},
						},
					},
				},
			}),
			expected: model.ServiceSpec{
				ServiceTagFields: model.ServiceTagFields{
					RouteName:      "service1",
					RouteNamespace: "default",
					RouteType:      core.HttpRouteType,
				},
				// only the lattice gateway is added
				ServiceNetworkNames: []string{vpcLatticeGateway.Name},
			},
		},
		{
			name:          "Standalone mode via route annotation",
			wantIsDeleted: false,
			wantErrIsNil:  true,
			gwClass:       vpcLatticeGatewayClass,
			gws: []gwv1.Gateway{
				vpcLatticeGateway,
			},
			route: core.NewHTTPRoute(gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "standalone-service",
					Namespace: "default",
					Annotations: map[string]string{
						k8s.StandaloneAnnotation: "true",
					},
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name:      gwv1.ObjectName(vpcLatticeGateway.Name),
								Namespace: namespacePtr(vpcLatticeGateway.Namespace),
							},
						},
					},
				},
			}),
			expected: model.ServiceSpec{
				ServiceTagFields: model.ServiceTagFields{
					RouteName:      "standalone-service",
					RouteNamespace: "default",
					RouteType:      core.HttpRouteType,
				},
				ServiceNetworkNames: []string{}, // Empty for standalone mode
			},
		},
		{
			name:          "Standalone mode via gateway annotation",
			wantIsDeleted: false,
			wantErrIsNil:  true,
			gwClass:       vpcLatticeGatewayClass,
			gws: []gwv1.Gateway{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "standalone-gateway",
						Namespace: "default",
						Annotations: map[string]string{
							k8s.StandaloneAnnotation: "true",
						},
					},
					Spec: gwv1.GatewaySpec{
						GatewayClassName: gwv1.ObjectName(vpcLatticeGatewayClass.Name),
					},
				},
			},
			route: core.NewHTTPRoute(gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service-inherits-standalone",
					Namespace: "default",
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name:      "standalone-gateway",
								Namespace: namespacePtr("default"),
							},
						},
					},
				},
			}),
			expected: model.ServiceSpec{
				ServiceTagFields: model.ServiceTagFields{
					RouteName:      "service-inherits-standalone",
					RouteNamespace: "default",
					RouteType:      core.HttpRouteType,
				},
				ServiceNetworkNames: []string{}, // Empty for standalone mode
			},
		},
		{
			name:          "Route annotation overrides gateway annotation",
			wantIsDeleted: false,
			wantErrIsNil:  true,
			gwClass:       vpcLatticeGatewayClass,
			gws: []gwv1.Gateway{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "standalone-gateway",
						Namespace: "default",
						Annotations: map[string]string{
							k8s.StandaloneAnnotation: "true",
						},
					},
					Spec: gwv1.GatewaySpec{
						GatewayClassName: gwv1.ObjectName(vpcLatticeGatewayClass.Name),
					},
				},
			},
			route: core.NewHTTPRoute(gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service-overrides-gateway",
					Namespace: "default",
					Annotations: map[string]string{
						k8s.StandaloneAnnotation: "false",
					},
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name:      "standalone-gateway",
								Namespace: namespacePtr("default"),
							},
						},
					},
				},
			}),
			expected: model.ServiceSpec{
				ServiceTagFields: model.ServiceTagFields{
					RouteName:      "service-overrides-gateway",
					RouteNamespace: "default",
					RouteType:      core.HttpRouteType,
				},
				ServiceNetworkNames: []string{"standalone-gateway"}, // Route annotation overrides gateway
			},
		},
		{
			name:          "Standalone mode with service network override enabled",
			wantIsDeleted: false,
			wantErrIsNil:  true,
			gwClass:       vpcLatticeGatewayClass,
			gws: []gwv1.Gateway{
				vpcLatticeGateway,
			},
			route: core.NewHTTPRoute(gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "standalone-with-override",
					Namespace: "default",
					Annotations: map[string]string{
						k8s.StandaloneAnnotation: "true",
					},
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name:      gwv1.ObjectName(vpcLatticeGateway.Name),
								Namespace: namespacePtr(vpcLatticeGateway.Namespace),
							},
						},
					},
				},
			}),
			expected: model.ServiceSpec{
				ServiceTagFields: model.ServiceTagFields{
					RouteName:      "standalone-with-override",
					RouteNamespace: "default",
					RouteType:      core.HttpRouteType,
				},
				ServiceNetworkNames: []string{}, // Standalone mode overrides service network override
			},
		},
		{
			name:          "Standalone mode with hostname",
			wantIsDeleted: false,
			wantErrIsNil:  true,
			gwClass:       vpcLatticeGatewayClass,
			gws: []gwv1.Gateway{
				vpcLatticeGateway,
			},
			route: core.NewHTTPRoute(gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "standalone-with-hostname",
					Namespace: "default",
					Annotations: map[string]string{
						k8s.StandaloneAnnotation: "true",
					},
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name:      gwv1.ObjectName(vpcLatticeGateway.Name),
								Namespace: namespacePtr(vpcLatticeGateway.Namespace),
							},
						},
					},
					Hostnames: []gwv1.Hostname{
						"standalone.example.com",
					},
				},
			}),
			expected: model.ServiceSpec{
				ServiceTagFields: model.ServiceTagFields{
					RouteName:      "standalone-with-hostname",
					RouteNamespace: "default",
					RouteType:      core.HttpRouteType,
				},
				CustomerDomainName:  "standalone.example.com",
				ServiceNetworkNames: []string{}, // Empty for standalone mode
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := gomock.NewController(t)
			defer c.Finish()
			ctx := context.TODO()

			// Handle service network override mode test case
			if tt.name == "Standalone mode with service network override enabled" {
				// Save original config values
				originalOverrideMode := config.ServiceNetworkOverrideMode
				originalDefaultServiceNetwork := config.DefaultServiceNetwork

				// Set override mode for this test
				config.ServiceNetworkOverrideMode = true
				config.DefaultServiceNetwork = "default-service-network"

				// Restore original values after test
				defer func() {
					config.ServiceNetworkOverrideMode = originalOverrideMode
					config.DefaultServiceNetwork = originalDefaultServiceNetwork
				}()
			}

			k8sSchema := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sSchema)
			gwv1.Install(k8sSchema)
			gwv1alpha2.Install(k8sSchema)
			k8sClient := testclient.NewClientBuilder().WithScheme(k8sSchema).Build()

			assert.NoError(t, k8sClient.Create(ctx, tt.gwClass.DeepCopy()))
			for _, gw := range tt.gws {
				assert.NoError(t, k8sClient.Create(ctx, gw.DeepCopy()))
			}
			stack := core.NewDefaultStack(core.StackID(k8s.NamespacedName(tt.route.K8sObject())))

			task := &latticeServiceModelBuildTask{
				log:    gwlog.FallbackLogger,
				route:  tt.route,
				stack:  stack,
				client: k8sClient,
			}

			svc, err := task.buildLatticeService(ctx)
			if !tt.wantErrIsNil {
				assert.NotNil(t, err)
				return
			}
			assert.Nil(t, err)

			assert.Equal(t, tt.wantIsDeleted, svc.IsDeleted)

			assert.Equal(t, tt.expected.RouteName, svc.Spec.RouteName)
			assert.Equal(t, tt.expected.RouteNamespace, svc.Spec.RouteNamespace)
			assert.Equal(t, tt.expected.CustomerCertARN, svc.Spec.CustomerCertARN)
			assert.Equal(t, tt.expected.CustomerDomainName, svc.Spec.CustomerDomainName)
			assert.Equal(t, tt.expected.RouteType, svc.Spec.RouteType)
			assert.Equal(t, tt.expected.ServiceNetworkNames, svc.Spec.ServiceNetworkNames)
		})
	}
}

func Test_latticeServiceModelBuildTask_isStandaloneMode(t *testing.T) {
	vpcLatticeGatewayClass := gwv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "gwClass",
		},
		Spec: gwv1.GatewayClassSpec{
			ControllerName: config.LatticeGatewayControllerName,
		},
	}

	tests := []struct {
		name           string
		route          core.Route
		gateway        *gwv1.Gateway
		gatewayClass   *gwv1.GatewayClass
		wantStandalone bool
		wantErr        bool
		errContains    string
	}{
		{
			name: "route with standalone annotation true",
			route: core.NewHTTPRoute(gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "default",
					Annotations: map[string]string{
						k8s.StandaloneAnnotation: "true",
					},
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name: "test-gateway",
							},
						},
					},
				},
			}),
			gateway: &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway",
					Namespace: "default",
				},
				Spec: gwv1.GatewaySpec{
					GatewayClassName: "gwClass",
				},
			},
			gatewayClass:   &vpcLatticeGatewayClass,
			wantStandalone: true,
			wantErr:        false,
		},
		{
			name: "route with standalone annotation false",
			route: core.NewHTTPRoute(gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "default",
					Annotations: map[string]string{
						k8s.StandaloneAnnotation: "false",
					},
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name: "test-gateway",
							},
						},
					},
				},
			}),
			gateway: &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway",
					Namespace: "default",
				},
				Spec: gwv1.GatewaySpec{
					GatewayClassName: "gwClass",
				},
			},
			gatewayClass:   &vpcLatticeGatewayClass,
			wantStandalone: false,
			wantErr:        false,
		},
		{
			name: "route with invalid standalone annotation",
			route: core.NewHTTPRoute(gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "default",
					Annotations: map[string]string{
						k8s.StandaloneAnnotation: "invalid",
					},
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name: "test-gateway",
							},
						},
					},
				},
			}),
			gateway: &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway",
					Namespace: "default",
				},
				Spec: gwv1.GatewaySpec{
					GatewayClassName: "gwClass",
				},
			},
			gatewayClass:   &vpcLatticeGatewayClass,
			wantStandalone: false,
			wantErr:        false,
		},
		{
			name: "gateway with standalone annotation true, route without annotation",
			route: core.NewHTTPRoute(gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "default",
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name: "test-gateway",
							},
						},
					},
				},
			}),
			gateway: &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway",
					Namespace: "default",
					Annotations: map[string]string{
						k8s.StandaloneAnnotation: "true",
					},
				},
				Spec: gwv1.GatewaySpec{
					GatewayClassName: "gwClass",
				},
			},
			gatewayClass:   &vpcLatticeGatewayClass,
			wantStandalone: true,
			wantErr:        false,
		},
		{
			name: "route annotation overrides gateway annotation",
			route: core.NewHTTPRoute(gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "default",
					Annotations: map[string]string{
						k8s.StandaloneAnnotation: "false",
					},
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name: "test-gateway",
							},
						},
					},
				},
			}),
			gateway: &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway",
					Namespace: "default",
					Annotations: map[string]string{
						k8s.StandaloneAnnotation: "true",
					},
				},
				Spec: gwv1.GatewaySpec{
					GatewayClassName: "gwClass",
				},
			},
			gatewayClass:   &vpcLatticeGatewayClass,
			wantStandalone: false, // Route annotation takes precedence
			wantErr:        false,
		},
		{
			name: "no annotations anywhere",
			route: core.NewHTTPRoute(gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "default",
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name: "test-gateway",
							},
						},
					},
				},
			}),
			gateway: &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway",
					Namespace: "default",
				},
				Spec: gwv1.GatewaySpec{
					GatewayClassName: "gwClass",
				},
			},
			gatewayClass:   &vpcLatticeGatewayClass,
			wantStandalone: false,
			wantErr:        false,
		},
		{
			name: "gateway not found error",
			route: core.NewHTTPRoute(gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "default",
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name: "nonexistent-gateway",
							},
						},
					},
				},
			}),
			gateway:        nil, // Gateway not created
			gatewayClass:   &vpcLatticeGatewayClass,
			wantStandalone: false,
			wantErr:        true,
			errContains:    "failed to find controlled parent gateways",
		},
		{
			name: "case insensitive true annotation",
			route: core.NewHTTPRoute(gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "default",
					Annotations: map[string]string{
						k8s.StandaloneAnnotation: "True",
					},
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name: "test-gateway",
							},
						},
					},
				},
			}),
			gateway: &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway",
					Namespace: "default",
				},
				Spec: gwv1.GatewaySpec{
					GatewayClassName: "gwClass",
				},
			},
			gatewayClass:   &vpcLatticeGatewayClass,
			wantStandalone: true,
			wantErr:        false,
		},
		{
			name: "route with empty annotation value",
			route: core.NewHTTPRoute(gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "default",
					Annotations: map[string]string{
						k8s.StandaloneAnnotation: "",
					},
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name: "test-gateway",
							},
						},
					},
				},
			}),
			gateway: &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway",
					Namespace: "default",
				},
				Spec: gwv1.GatewaySpec{
					GatewayClassName: "gwClass",
				},
			},
			gatewayClass:   &vpcLatticeGatewayClass,
			wantStandalone: false, // Empty value treated as false
			wantErr:        false,
		},
		{
			name: "route with whitespace annotation value",
			route: core.NewHTTPRoute(gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "default",
					Annotations: map[string]string{
						k8s.StandaloneAnnotation: "   ",
					},
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name: "test-gateway",
							},
						},
					},
				},
			}),
			gateway: &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway",
					Namespace: "default",
				},
				Spec: gwv1.GatewaySpec{
					GatewayClassName: "gwClass",
				},
			},
			gatewayClass:   &vpcLatticeGatewayClass,
			wantStandalone: false, // Whitespace-only value treated as false
			wantErr:        false,
		},
		{
			name: "route with true annotation with whitespace",
			route: core.NewHTTPRoute(gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "default",
					Annotations: map[string]string{
						k8s.StandaloneAnnotation: "  true  ",
					},
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name: "test-gateway",
							},
						},
					},
				},
			}),
			gateway: &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway",
					Namespace: "default",
				},
				Spec: gwv1.GatewaySpec{
					GatewayClassName: "gwClass",
				},
			},
			gatewayClass:   &vpcLatticeGatewayClass,
			wantStandalone: true, // Whitespace around valid value is handled
			wantErr:        false,
		},
		{
			name: "gateway with invalid annotation value",
			route: core.NewHTTPRoute(gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "default",
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name: "test-gateway",
							},
						},
					},
				},
			}),
			gateway: &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway",
					Namespace: "default",
					Annotations: map[string]string{
						k8s.StandaloneAnnotation: "invalid",
					},
				},
				Spec: gwv1.GatewaySpec{
					GatewayClassName: "gwClass",
				},
			},
			gatewayClass:   &vpcLatticeGatewayClass,
			wantStandalone: false, // Invalid gateway annotation treated as false
			wantErr:        false,
		},
		{
			name: "route being deleted with missing gateway",
			route: func() core.Route {
				now := metav1.Now()
				return core.NewHTTPRoute(gwv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test-route",
						Namespace:         "default",
						DeletionTimestamp: &now,
					},
					Spec: gwv1.HTTPRouteSpec{
						CommonRouteSpec: gwv1.CommonRouteSpec{
							ParentRefs: []gwv1.ParentReference{
								{
									Name: "nonexistent-gateway",
								},
							},
						},
					},
				})
			}(),
			gateway:        nil, // Gateway not created
			gatewayClass:   &vpcLatticeGatewayClass,
			wantStandalone: false, // Should handle gracefully during deletion
			wantErr:        false,
		},
		{
			name: "numeric annotation value",
			route: core.NewHTTPRoute(gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: "default",
					Annotations: map[string]string{
						k8s.StandaloneAnnotation: "1",
					},
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name: "test-gateway",
							},
						},
					},
				},
			}),
			gateway: &gwv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway",
					Namespace: "default",
				},
				Spec: gwv1.GatewaySpec{
					GatewayClassName: "gwClass",
				},
			},
			gatewayClass:   &vpcLatticeGatewayClass,
			wantStandalone: false, // Numeric value treated as false
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.TODO()

			k8sSchema := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sSchema)
			gwv1.Install(k8sSchema)
			k8sClient := testclient.NewClientBuilder().WithScheme(k8sSchema).Build()

			// Create gateway class
			assert.NoError(t, k8sClient.Create(ctx, tt.gatewayClass.DeepCopy()))

			// Create gateway if provided
			if tt.gateway != nil {
				assert.NoError(t, k8sClient.Create(ctx, tt.gateway.DeepCopy()))
			}

			stack := core.NewDefaultStack(core.StackID(k8s.NamespacedName(tt.route.K8sObject())))
			task := &latticeServiceModelBuildTask{
				log:    gwlog.FallbackLogger,
				route:  tt.route,
				stack:  stack,
				client: k8sClient,
			}

			standalone, err := task.isStandaloneMode(ctx)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantStandalone, standalone)
			}
		})
	}
}

func Test_LatticeServiceModelBuild_HTTPRouteWithAndWithoutAdditionalTagsAnnotation(t *testing.T) {
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

	namespacePtr := func(ns string) *gwv1.Namespace {
		p := gwv1.Namespace(ns)
		return &p
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
					Name:      "service-with-tags",
					Namespace: "default",
					Annotations: map[string]string{
						k8s.TagsAnnotationKey: "Environment=Prod,Project=ServiceTest,Team=Platform",
					},
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name:      gwv1.ObjectName(vpcLatticeGateway.Name),
								Namespace: namespacePtr(vpcLatticeGateway.Namespace),
							},
						},
					},
				},
			}),
			expectedAdditionalTags: k8s.Tags{
				"Environment": &[]string{"Prod"}[0],
				"Project":     &[]string{"ServiceTest"}[0],
				"Team":        &[]string{"Platform"}[0],
			},
			description: "should set additional tags from HTTPRoute annotations in service spec",
		},
		{
			name: "HTTPRoute without additional tags annotation",
			route: core.NewHTTPRoute(gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service-no-tags",
					Namespace: "default",
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name:      gwv1.ObjectName(vpcLatticeGateway.Name),
								Namespace: namespacePtr(vpcLatticeGateway.Namespace),
							},
						},
					},
				},
			}),
			expectedAdditionalTags: nil,
			description:            "should have nil additional tags when no annotation present in service spec",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.TODO()

			k8sSchema := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sSchema)
			gwv1.Install(k8sSchema)
			gwv1alpha2.Install(k8sSchema)
			k8sClient := testclient.NewClientBuilder().WithScheme(k8sSchema).Build()

			assert.NoError(t, k8sClient.Create(ctx, vpcLatticeGatewayClass.DeepCopy()))
			assert.NoError(t, k8sClient.Create(ctx, vpcLatticeGateway.DeepCopy()))

			stack := core.NewDefaultStack(core.StackID(k8s.NamespacedName(tt.route.K8sObject())))

			task := &latticeServiceModelBuildTask{
				log:    gwlog.FallbackLogger,
				route:  tt.route,
				stack:  stack,
				client: k8sClient,
			}

			svc, err := task.buildLatticeService(ctx)
			assert.NoError(t, err, tt.description)

			assert.Equal(t, tt.expectedAdditionalTags, svc.Spec.AdditionalTags, tt.description)
		})
	}
}
