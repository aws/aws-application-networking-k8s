package gateway

import (
	"context"
	"fmt"
	"strings"
	"testing"

	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	mock_client "github.com/aws/aws-application-networking-k8s/mocks/controller-runtime/client"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"

	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"

	"github.com/aws/aws-sdk-go/service/vpclattice"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

func Test_TGModelByServiceExportBuild(t *testing.T) {
	config.VpcID = "vpc-id"
	config.ClusterName = "cluster-name"

	now := metav1.Now()
	tests := []struct {
		name                string
		svcExport           *anv1alpha1.ServiceExport
		svc                 *corev1.Service
		endPoints           []corev1.Endpoints
		wantErrIsNil        bool
		wantIsDeleted       bool
		wantIPv6TargetGroup bool
	}{
		{
			name: "Adding ServiceExport where service object exist",
			svcExport: &anv1alpha1.ServiceExport{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "export1",
					Namespace: "ns1",
				},
			},
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "export1",
					Namespace: "ns1",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{},
					},
					IPFamilies: []corev1.IPFamily{
						corev1.IPv4Protocol,
					},
				},
			},
			endPoints: []corev1.Endpoints{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns1",
						Name:      "export1",
					},
				},
			},
			wantErrIsNil:        true,
			wantIsDeleted:       false,
			wantIPv6TargetGroup: false,
		},
		{
			name: "Adding ServiceExport where service object does NOT exist",
			svcExport: &anv1alpha1.ServiceExport{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "export2",
					Namespace: "ns1",
				},
			},

			wantErrIsNil:        false,
			wantIsDeleted:       false,
			wantIPv6TargetGroup: false,
		},
		{
			name: "Deleting ServiceExport where service object does NOT exist",
			svcExport: &anv1alpha1.ServiceExport{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "export3",
					Namespace:         "ns1",
					Finalizers:        []string{"gateway.k8s.aws/resources"},
					DeletionTimestamp: &now,
				},
			},
			wantIsDeleted: true,
			wantErrIsNil:  true,
		},
		{
			name: "Deleting ServiceExport where service object exist",
			svcExport: &anv1alpha1.ServiceExport{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "export4",
					Namespace:         "ns1",
					Finalizers:        []string{"gateway.k8s.aws/resources"},
					DeletionTimestamp: &now,
				},
			},
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "export4",
					Namespace: "ns1",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{},
					},
					IPFamilies: []corev1.IPFamily{
						corev1.IPv4Protocol,
					},
				},
			},
			endPoints: []corev1.Endpoints{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns1",
						Name:      "export4",
					},
				},
			},
			wantErrIsNil:        true,
			wantIsDeleted:       true,
			wantIPv6TargetGroup: false,
		},
		{
			name: "Creating IPv6 ServiceExport where service object with IpFamilies IPv6 exists",
			svcExport: &anv1alpha1.ServiceExport{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "export5",
					Namespace:  "ns1",
					Finalizers: []string{"gateway.k8s.aws/resources"},
				},
			},
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "export5",
					Namespace: "ns1",
				},
				Spec: corev1.ServiceSpec{
					IPFamilies: []corev1.IPFamily{corev1.IPv6Protocol},
					Ports: []corev1.ServicePort{
						{},
					},
				},
			},
			endPoints: []corev1.Endpoints{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns1",
						Name:      "export5",
					},
				},
			},
			wantErrIsNil:        true,
			wantIsDeleted:       false,
			wantIPv6TargetGroup: true,
		},
		{
			name: "Failed to create IPv6 ServiceExport where service object with dual stack IpFamilies exists",
			svcExport: &anv1alpha1.ServiceExport{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "export6",
					Namespace:  "ns1",
					Finalizers: []string{"gateway.k8s.aws/resources"},
				},
			},
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "export6",
					Namespace: "ns1",
				},
				Spec: corev1.ServiceSpec{
					IPFamilies: []corev1.IPFamily{corev1.IPv4Protocol, corev1.IPv6Protocol},
					Ports: []corev1.ServicePort{
						{},
					},
				},
			},
			endPoints: []corev1.Endpoints{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns1",
						Name:      "export6",
					},
				},
			},
			wantErrIsNil:  false,
			wantIsDeleted: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := gomock.NewController(t)
			defer c.Finish()
			ctx := context.TODO()

			k8sSchema := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sSchema)
			anv1alpha1.AddToScheme(k8sSchema)
			k8sClient := testclient.NewClientBuilder().WithScheme(k8sSchema).Build()

			if tt.svc != nil {
				assert.NoError(t, k8sClient.Create(ctx, tt.svc.DeepCopy()))
			}

			if tt.endPoints != nil {
				assert.NoError(t, k8sClient.Create(ctx, tt.endPoints[0].DeepCopy()))
			}

			builder := NewSvcExportTargetGroupBuilder(gwlog.FallbackLogger, k8sClient)

			stack, err := builder.Build(ctx, tt.svcExport)
			fmt.Printf("stack %v err %v\n", stack, err)
			if !tt.wantErrIsNil {
				assert.NotNil(t, err)
				return
			}
			assert.Nil(t, err)

			var resTargetGroups []*model.TargetGroup
			err = stack.ListResources(&resTargetGroups)
			assert.Nil(t, err)
			assert.Equal(t, 1, len(resTargetGroups))

			stackTg := resTargetGroups[0]
			spec := model.TargetGroupSpec{}
			spec.K8SServiceName = tt.svcExport.Name
			spec.K8SServiceNamespace = tt.svcExport.Namespace
			tgNamePrefix := model.TgNamePrefix(spec)
			generatedName := model.GenerateTgName(stackTg.Spec)
			assert.True(t, strings.HasPrefix(generatedName, tgNamePrefix))

			if tt.wantIsDeleted {
				assert.True(t, stackTg.IsDeleted)
				return
			}
			assert.False(t, stackTg.IsDeleted)

			if tt.wantIPv6TargetGroup {
				assert.Equal(t, vpclattice.IpAddressTypeIpv6, stackTg.Spec.IpAddressType)
			} else {
				assert.Equal(t, vpclattice.IpAddressTypeIpv4, stackTg.Spec.IpAddressType)
			}

			assert.Equal(t, config.ClusterName, stackTg.Spec.K8SClusterName)
			assert.Equal(t, config.VpcID, stackTg.Spec.VpcId)
			assert.Equal(t, model.SourceTypeSvcExport, stackTg.Spec.K8SSourceType)
			assert.Equal(t, tt.svc.Name, stackTg.Spec.K8SServiceName)
			assert.Equal(t, tt.svc.Namespace, stackTg.Spec.K8SServiceNamespace)
			assert.Equal(t, "", stackTg.Spec.K8SRouteName)
			assert.Equal(t, "", stackTg.Spec.K8SRouteNamespace)
		})
	}
}

func Test_TGModelByHTTPRouteBuild(t *testing.T) {
	config.VpcID = "vpc-id"
	config.ClusterName = "cluster-name"
	now := metav1.Now()

	namespacePtr := func(ns string) *gwv1.Namespace {
		p := gwv1.Namespace(ns)
		return &p
	}

	kindPtr := func(k string) *gwv1.Kind {
		p := gwv1.Kind(k)
		return &p
	}

	tests := []struct {
		name                string
		route               core.Route
		svcExist            bool
		wantError           error
		wantErrIsNil        bool
		wantName            string
		wantIsDeleted       bool
		wantIPv6TargetGroup bool
	}{
		{
			name: "Add LatticeService",
			route: core.NewHTTPRoute(gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "ns1",
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name:      "gateway1",
								Namespace: namespacePtr("ns1"),
							},
						},
					},
					Rules: []gwv1.HTTPRouteRule{
						{
							BackendRefs: []gwv1.HTTPBackendRef{
								{
									BackendRef: gwv1.BackendRef{
										BackendObjectReference: gwv1.BackendObjectReference{
											Name:      "service1-tg1",
											Namespace: namespacePtr("ns11"),
											Kind:      kindPtr("Service"),
										},
									},
								},
							},
						},
					},
				},
			}),
			svcExist:            true,
			wantError:           nil,
			wantName:            "service1",
			wantIsDeleted:       false,
			wantErrIsNil:        true,
			wantIPv6TargetGroup: false,
		},
		{
			name: "Delete LatticeService",
			route: core.NewHTTPRoute(gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "service2",
					Namespace:         "ns1",
					Finalizers:        []string{"gateway.k8s.aws/resources"},
					DeletionTimestamp: &now,
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name:      "gateway1",
								Namespace: namespacePtr("ns1"),
							},
						},
					},
					Rules: []gwv1.HTTPRouteRule{
						{
							BackendRefs: []gwv1.HTTPBackendRef{
								{
									BackendRef: gwv1.BackendRef{
										BackendObjectReference: gwv1.BackendObjectReference{
											Name:      "service2-tg1",
											Namespace: namespacePtr("ns21"),
											Kind:      kindPtr("Service"),
										},
									},
								},
							},
						},
					},
				},
			}),
			svcExist:            true,
			wantError:           nil,
			wantName:            "service1",
			wantIsDeleted:       true,
			wantErrIsNil:        true,
			wantIPv6TargetGroup: false,
		},
		{
			name: "Create LatticeService where backend K8S service does NOT exist",
			route: core.NewHTTPRoute(gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "service3",
					Namespace:  "ns1",
					Finalizers: []string{"gateway.k8s.aws/resources"},
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name:      "gateway1",
								Namespace: namespacePtr("ns1"),
							},
						},
					},
					Rules: []gwv1.HTTPRouteRule{
						{
							BackendRefs: []gwv1.HTTPBackendRef{
								{
									BackendRef: gwv1.BackendRef{
										BackendObjectReference: gwv1.BackendObjectReference{
											Name:      "service3-tg1",
											Namespace: namespacePtr("ns31"),
											Kind:      kindPtr("Service"),
										},
									},
								},
							},
						},
					},
				},
			}),
			svcExist:            false,
			wantError:           nil,
			wantName:            "service1",
			wantIsDeleted:       false,
			wantErrIsNil:        false,
			wantIPv6TargetGroup: false,
		},
		{
			name: "Lattice Service with IPv6 Target Group",
			route: core.NewHTTPRoute(gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "service5",
					Namespace:  "ns1",
					Finalizers: []string{"gateway.k8s.aws/resources"},
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name:      "gateway1",
								Namespace: namespacePtr("ns1"),
							},
						},
					},
					Rules: []gwv1.HTTPRouteRule{
						{
							BackendRefs: []gwv1.HTTPBackendRef{
								{
									BackendRef: gwv1.BackendRef{
										BackendObjectReference: gwv1.BackendObjectReference{
											Name:      "service5-tg1",
											Namespace: namespacePtr("ns31"),
											Kind:      kindPtr("Service"),
										},
									},
								},
							},
						},
					},
				},
			}),
			svcExist:            true,
			wantError:           nil,
			wantIsDeleted:       false,
			wantErrIsNil:        true,
			wantIPv6TargetGroup: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := gomock.NewController(t)
			defer c.Finish()
			ctx := context.TODO()

			k8sSchema := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sSchema)
			anv1alpha1.AddToScheme(k8sSchema)
			gwv1.AddToScheme(k8sSchema)
			k8sClient := testclient.NewClientBuilder().WithScheme(k8sSchema).Build()

			stack := core.NewDefaultStack(core.StackID(k8s.NamespacedName(tt.route.K8sObject())))

			if tt.svcExist {
				// populate K8S service
				for _, httpRules := range tt.route.Spec().Rules() {
					for _, httpBackendRef := range httpRules.BackendRefs() {
						svc := corev1.Service{
							ObjectMeta: metav1.ObjectMeta{
								Name:      string(httpBackendRef.Name()),
								Namespace: string(*httpBackendRef.Namespace()),
							},
						}

						if tt.wantIPv6TargetGroup {
							svc.Spec.IPFamilies = []corev1.IPFamily{corev1.IPv6Protocol}
						} else {
							svc.Spec.IPFamilies = []corev1.IPFamily{corev1.IPv4Protocol}
						}

						assert.NoError(t, k8sClient.Create(ctx, svc.DeepCopy()))
					}
				}
			}

			// this test assumes a single rule and single backendRef, which means we
			// should always just get one target group
			assert.Equal(t, 1, len(tt.route.Spec().Rules()))
			rule := tt.route.Spec().Rules()[0]

			assert.Equal(t, 1, len(rule.BackendRefs()))
			httpBackendRef := rule.BackendRefs()[0]

			// we just want to test the target group logic, not service, listener, etc
			// this is done on a per backend-ref basis
			builder := NewBackendRefTargetGroupBuilder(gwlog.FallbackLogger, k8sClient)

			_, stackTg, err := builder.Build(ctx, tt.route, httpBackendRef, stack)
			if !tt.wantErrIsNil {
				ibre := &InvalidBackendRefError{}
				if !tt.svcExist {
					assert.ErrorAs(t, err, &ibre)
				}
				assert.NotNil(t, err)
				return
			}
			assert.Nil(t, err)

			var stackTgs []*model.TargetGroup
			_ = stack.ListResources(&stackTgs)
			assert.Equal(t, 1, len(stackTgs))

			// make sure the name is in the format we expect - based off the backend ref/svc name/ns
			spec := model.TargetGroupSpec{}
			spec.K8SServiceName = string(httpBackendRef.Name())
			spec.K8SServiceNamespace = string(*httpBackendRef.Namespace())
			tgNamePrefix := model.TgNamePrefix(spec)
			generatedName := model.GenerateTgName(stackTg.Spec)
			assert.True(t, strings.HasPrefix(generatedName, tgNamePrefix))

			assert.Equal(t, tt.wantIsDeleted, stackTg.IsDeleted)
			if tt.wantIsDeleted {
				return
			}

			if tt.wantIPv6TargetGroup {
				assert.Equal(t, vpclattice.IpAddressTypeIpv6, stackTg.Spec.IpAddressType)
			} else {
				assert.Equal(t, vpclattice.IpAddressTypeIpv4, stackTg.Spec.IpAddressType)
			}

			assert.Equal(t, config.ClusterName, stackTg.Spec.K8SClusterName)
			assert.Equal(t, config.VpcID, stackTg.Spec.VpcId)
			assert.Equal(t, model.SourceTypeHTTPRoute, stackTg.Spec.K8SSourceType)
			assert.Equal(t, spec.K8SServiceName, stackTg.Spec.K8SServiceName)
			assert.Equal(t, spec.K8SServiceNamespace, stackTg.Spec.K8SServiceNamespace)
			assert.Equal(t, tt.route.Name(), stackTg.Spec.K8SRouteName)
			assert.Equal(t, tt.route.Namespace(), stackTg.Spec.K8SRouteNamespace)
		})
	}
}

// service imports do not do a full TG build, just a reference
// see model_build_rule.go#getTargetGroupsForRuleAction
func Test_ServiceImportToTGBuildReturnsError(t *testing.T) {
	config.VpcID = "vpc-id"
	config.ClusterName = "cluster-name"

	namespacePtr := func(ns string) *gwv1.Namespace {
		p := gwv1.Namespace(ns)
		return &p
	}

	kindPtr := func(k string) *gwv1.Kind {
		p := gwv1.Kind(k)
		return &p
	}

	tests := []struct {
		name  string
		route core.Route
	}{
		{
			name: "Service import does not create target group - returns error",
			route: core.NewHTTPRoute(gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name: "serviceimport1",
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name: "gateway1",
							},
						},
					},
					Rules: []gwv1.HTTPRouteRule{
						{
							BackendRefs: []gwv1.HTTPBackendRef{
								{
									BackendRef: gwv1.BackendRef{
										BackendObjectReference: gwv1.BackendObjectReference{
											Name:      "service1-tg2",
											Namespace: namespacePtr("tg1-ns1"),
											Kind:      kindPtr("ServiceImport"),
										},
									},
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
			ctx := context.Background()

			mockK8sClient := mock_client.NewMockClient(c)

			stack := core.NewDefaultStack(core.StackID(k8s.NamespacedName(tt.route.K8sObject())))

			// these tests only support a single rule and backend ref
			rule := tt.route.Spec().Rules()[0]
			httpBackendRef := rule.BackendRefs()[0]

			builder := NewBackendRefTargetGroupBuilder(gwlog.FallbackLogger, mockK8sClient)
			_, _, err := builder.Build(ctx, tt.route, httpBackendRef, stack)
			assert.NotNil(t, err)
		})
	}
}

func Test_buildTargetGroupIpAddressType(t *testing.T) {
	type args struct {
		svc *corev1.Service
	}

	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "IpFamilies [IPv4] get corev1.IPv4Protocol",
			args: args{
				svc: &corev1.Service{
					Spec: corev1.ServiceSpec{
						IPFamilies: []corev1.IPFamily{corev1.IPv4Protocol},
					},
				},
			},
			want:    vpclattice.IpAddressTypeIpv4,
			wantErr: false,
		},
		{
			name: "IpFamilies [IPv4] get corev1.IPv6Protocol",
			args: args{
				svc: &corev1.Service{
					Spec: corev1.ServiceSpec{
						IPFamilies: []corev1.IPFamily{corev1.IPv6Protocol},
					},
				},
			},
			want:    vpclattice.IpAddressTypeIpv6,
			wantErr: false,
		},
		{
			name: "IpFamilies empty get error",
			args: args{
				svc: &corev1.Service{
					Spec: corev1.ServiceSpec{
						IPFamilies: []corev1.IPFamily{},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "IpFamilies contain invalid value get error",
			args: args{
				svc: &corev1.Service{
					Spec: corev1.ServiceSpec{
						IPFamilies: []corev1.IPFamily{"IPv5"},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildTargetGroupIpAddressType(tt.args.svc)
			if (err != nil) != tt.wantErr {
				t.Errorf("buildTargetGroupIpAddressType() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("buildTargetGroupIpAddressType() = %v, want %v", got, tt.want)
			}
		})
	}
}
