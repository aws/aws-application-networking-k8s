package gateway

import (
	"context"
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
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	"testing"
)

func Test_LatticeServiceModelBuild(t *testing.T) {
	now := metav1.Now()
	var httpSectionName gwv1beta1.SectionName = "http"
	var serviceKind gwv1beta1.Kind = "Service"
	var serviceimportKind gwv1beta1.Kind = "ServiceImport"
	var weight1 = int32(10)
	var weight2 = int32(90)
	var namespace = gwv1beta1.Namespace("default")

	namespacePtr := func(ns string) *gwv1beta1.Namespace {
		p := gwv1beta1.Namespace(ns)
		return &p
	}

	var backendRef1 = gwv1beta1.BackendRef{
		BackendObjectReference: gwv1beta1.BackendObjectReference{
			Name:      "targetgroup1",
			Namespace: &namespace,
			Kind:      &serviceKind,
		},
		Weight: &weight1,
	}
	var backendRef2 = gwv1beta1.BackendRef{
		BackendObjectReference: gwv1beta1.BackendObjectReference{
			Name:      "targetgroup2",
			Namespace: &namespace,
			Kind:      &serviceimportKind,
		},
		Weight: &weight2,
	}

	tlsSectionName := gwv1beta1.SectionName("tls")
	tlsModeTerminate := gwv1beta1.TLSModeTerminate

	tests := []struct {
		name          string
		gw            gwv1beta1.Gateway
		route         core.Route
		wantErrIsNil  bool
		wantIsDeleted bool
		expected      model.ServiceSpec
	}{
		{
			name:          "Add LatticeService with hostname",
			wantIsDeleted: false,
			wantErrIsNil:  true,
			gw: gwv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gateway1",
					Namespace: "default",
				},
			},
			route: core.NewHTTPRoute(gwv1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "test",
				},
				Spec: gwv1beta1.HTTPRouteSpec{
					CommonRouteSpec: gwv1beta1.CommonRouteSpec{
						ParentRefs: []gwv1beta1.ParentReference{
							{
								Name:      "gateway1",
								Namespace: namespacePtr("default"),
							},
						},
					},
					Hostnames: []gwv1beta1.Hostname{
						"test1.test.com",
						"test2.test.com",
					},
				},
			}),
			expected: model.ServiceSpec{
				Name:                "service1",
				Namespace:           "test",
				CustomerDomainName:  "test1.test.com",
				RouteType:           core.HttpRouteType,
				ServiceNetworkNames: []string{"gateway1"},
			},
		},
		{
			name:          "Add LatticeService",
			wantIsDeleted: false,
			wantErrIsNil:  true,
			gw: gwv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gateway1",
					Namespace: "default",
				},
			},
			route: core.NewHTTPRoute(gwv1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gwv1beta1.HTTPRouteSpec{
					CommonRouteSpec: gwv1beta1.CommonRouteSpec{
						ParentRefs: []gwv1beta1.ParentReference{
							{
								Name:      "gateway1",
								Namespace: namespacePtr("default"),
							},
						},
					},
				},
			}),
			expected: model.ServiceSpec{
				Name:                "service1",
				Namespace:           "default",
				RouteType:           core.HttpRouteType,
				ServiceNetworkNames: []string{"gateway1"},
			},
		},
		{
			name:          "Add LatticeService with GRPCRoute",
			wantIsDeleted: false,
			wantErrIsNil:  true,
			gw: gwv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gateway1",
					Namespace: "test",
				},
			},
			route: core.NewGRPCRoute(gwv1alpha2.GRPCRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "test",
				},
				Spec: gwv1alpha2.GRPCRouteSpec{
					CommonRouteSpec: gwv1beta1.CommonRouteSpec{
						ParentRefs: []gwv1beta1.ParentReference{
							{
								Name: "gateway1",
							},
						},
					},
				},
			}),
			expected: model.ServiceSpec{
				Name:                "service1",
				Namespace:           "test",
				RouteType:           core.GrpcRouteType,
				ServiceNetworkNames: []string{"gateway1"},
			},
		},
		{
			name:          "Delete LatticeService",
			wantIsDeleted: true,
			wantErrIsNil:  true,
			gw: gwv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gateway2",
					Namespace: "ns1",
				},
				Spec: gwv1beta1.GatewaySpec{
					Listeners: []gwv1beta1.Listener{
						{
							Name:     httpSectionName,
							Port:     80,
							Protocol: "HTTP",
						},
					},
				},
			},
			route: core.NewHTTPRoute(gwv1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "service2",
					Namespace:         "ns1",
					Finalizers:        []string{"gateway.k8s.aws/resources"},
					DeletionTimestamp: &now, // <- the important bit
				},
				Spec: gwv1beta1.HTTPRouteSpec{
					CommonRouteSpec: gwv1beta1.CommonRouteSpec{
						ParentRefs: []gwv1beta1.ParentReference{
							{
								Name:        "gateway2",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []gwv1beta1.HTTPRouteRule{
						{
							BackendRefs: []gwv1beta1.HTTPBackendRef{
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
				Name:                "service2",
				Namespace:           "ns1",
				RouteType:           core.HttpRouteType,
				ServiceNetworkNames: []string{"gateway2"},
			},
		},
		{
			name:          "Service with customer Cert ARN",
			wantIsDeleted: false,
			wantErrIsNil:  true,
			gw: gwv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gateway1",
					Namespace: "default",
				},
				Spec: gwv1beta1.GatewaySpec{
					Listeners: []gwv1beta1.Listener{
						{
							Name:     "tls",
							Port:     443,
							Protocol: "HTTPS",
							TLS: &gwv1beta1.GatewayTLSConfig{
								Mode:            &tlsModeTerminate,
								CertificateRefs: nil,
								Options: map[gwv1beta1.AnnotationKey]gwv1beta1.AnnotationValue{
									"application-networking.k8s.aws/certificate-arn": "cert-arn",
								},
							},
						},
					},
				},
			},
			route: core.NewHTTPRoute(gwv1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gwv1beta1.HTTPRouteSpec{
					CommonRouteSpec: gwv1beta1.CommonRouteSpec{
						ParentRefs: []gwv1beta1.ParentReference{
							{
								Name:        "gateway1",
								Namespace:   namespacePtr("default"),
								SectionName: &tlsSectionName,
							},
						},
					},
				},
			}),
			expected: model.ServiceSpec{
				Name:                "service1",
				Namespace:           "default",
				RouteType:           core.HttpRouteType,
				CustomerCertARN:     "cert-arn",
				ServiceNetworkNames: []string{"gateway1"},
			},
		},
		{
			name: "GW does not exist",
			gw: gwv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gateway1",
					Namespace: "default",
				},
			},
			route: core.NewHTTPRoute(gwv1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gwv1beta1.HTTPRouteSpec{
					CommonRouteSpec: gwv1beta1.CommonRouteSpec{
						ParentRefs: []gwv1beta1.ParentReference{
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
			gw: gwv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gateway1",
					Namespace: "default",
				},
				Spec: gwv1beta1.GatewaySpec{
					Listeners: []gwv1beta1.Listener{
						{
							Name:     "tls",
							Port:     443,
							Protocol: "HTTPS",
							TLS: &gwv1beta1.GatewayTLSConfig{
								Mode:            &tlsModeTerminate,
								CertificateRefs: nil,
							},
						},
					},
				},
			},
			route: core.NewHTTPRoute(gwv1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gwv1beta1.HTTPRouteSpec{
					CommonRouteSpec: gwv1beta1.CommonRouteSpec{
						ParentRefs: []gwv1beta1.ParentReference{
							{
								Name:        "gateway1",
								Namespace:   namespacePtr("default"),
								SectionName: &tlsSectionName,
							},
						},
					},
				},
			}),
			expected: model.ServiceSpec{
				Name:                "service1",
				Namespace:           "default",
				RouteType:           core.HttpRouteType,
				ServiceNetworkNames: []string{"gateway1"},
			},
		},
		{
			name:          "Multiple service networks",
			wantIsDeleted: false,
			wantErrIsNil:  true,
			gw: gwv1beta1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gateway1",
					Namespace: "default",
				},
			},
			route: core.NewHTTPRoute(gwv1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gwv1beta1.HTTPRouteSpec{
					CommonRouteSpec: gwv1beta1.CommonRouteSpec{
						ParentRefs: []gwv1beta1.ParentReference{
							{
								Name:      "gateway1",
								Namespace: namespacePtr("default"),
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
				Name:                "service1",
				Namespace:           "default",
				RouteType:           core.HttpRouteType,
				ServiceNetworkNames: []string{"gateway1", "gateway2"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := gomock.NewController(t)
			defer c.Finish()
			ctx := context.TODO()

			k8sSchema := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sSchema)
			gwv1beta1.AddToScheme(k8sSchema)
			k8sClient := testclient.NewFakeClientWithScheme(k8sSchema)

			assert.NoError(t, k8sClient.Create(ctx, tt.gw.DeepCopy()))
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

			assert.Equal(t, tt.expected.Name, svc.Spec.Name)
			assert.Equal(t, tt.expected.Namespace, svc.Spec.Namespace)
			assert.Equal(t, tt.expected.CustomerCertARN, svc.Spec.CustomerCertARN)
			assert.Equal(t, tt.expected.CustomerDomainName, svc.Spec.CustomerDomainName)
			assert.Equal(t, tt.expected.RouteType, svc.Spec.RouteType)
			assert.Equal(t, tt.expected.ServiceNetworkNames, svc.Spec.ServiceNetworkNames)
		})
	}
}
