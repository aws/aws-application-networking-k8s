package gateway

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"

	"github.com/aws/aws-sdk-go/service/vpclattice"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	gateway_api "sigs.k8s.io/gateway-api/apis/v1beta1"
	mcs_api "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	mock_client "github.com/aws/aws-application-networking-k8s/mocks/controller-runtime/client"
	"github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

func Test_TGModelByServicexportBuild(t *testing.T) {
	now := metav1.Now()
	tests := []struct {
		name                string
		svcExport           *mcs_api.ServiceExport
		svc                 *corev1.Service
		endPoints           []corev1.Endpoints
		wantErrIsNil        bool
		wantIsDeleted       bool
		wantIPv6TargetGroup bool
	}{
		{
			name: "Adding ServieExport where service object exist",
			svcExport: &mcs_api.ServiceExport{
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
			name: "Adding ServieExport where service object does NOT exist",
			svcExport: &mcs_api.ServiceExport{
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
			svcExport: &mcs_api.ServiceExport{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "export3",
					Namespace:         "ns1",
					Finalizers:        []string{"gateway.k8s.aws/resources"},
					DeletionTimestamp: &now,
				},
			},

			wantErrIsNil:        true,
			wantIsDeleted:       true,
			wantIPv6TargetGroup: false,
		},
		{
			name: "Deleting ServieExport where service object exist",
			svcExport: &mcs_api.ServiceExport{
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
			svcExport: &mcs_api.ServiceExport{
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
			svcExport: &mcs_api.ServiceExport{
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
			v1alpha1.AddToScheme(k8sSchema)
			k8sClient := testclient.NewFakeClientWithScheme(k8sSchema)

			if tt.svc != nil {
				assert.NoError(t, k8sClient.Create(ctx, tt.svc.DeepCopy()))

			}

			if tt.endPoints != nil {
				assert.NoError(t, k8sClient.Create(ctx, tt.endPoints[0].DeepCopy()))
			}

			ds := latticestore.NewLatticeDataStore()
			if !tt.svcExport.DeletionTimestamp.IsZero() {
				// When test serviceExport deletion, we expect latticeDataStore already had this tg entry
				tgName := latticestore.TargetGroupName(tt.svcExport.Name, tt.svcExport.Namespace)
				ds.AddTargetGroup(tgName, "vpc-123456789", "123456789", "tg-123", false, "")
			}
			builder := NewSvcExportTargetGroupBuilder(gwlog.FallbackLogger, k8sClient, ds, nil)

			stack, tg, err := builder.Build(ctx, tt.svcExport)

			fmt.Printf("stack %v tg %v err %v\n", stack, tg, err)

			if !tt.wantErrIsNil {
				assert.NotNil(t, err)
				return
			}

			assert.Nil(t, err)
			tgName := latticestore.TargetGroupName(tt.svcExport.Name, tt.svcExport.Namespace)
			assert.Equal(t, tgName, tg.Spec.Name)

			// for serviceexport, the routeName is ""
			dsTG, err := ds.GetTargetGroup(tgName, "", false)

			assert.Nil(t, err)
			if tt.wantIsDeleted {
				assert.Equal(t, false, dsTG.ByServiceExport)
				assert.Equal(t, true, tg.Spec.IsDeleted)
				assert.Equal(t, "tg-123", tg.Spec.LatticeID)
			} else {
				assert.Equal(t, true, dsTG.ByServiceExport)
				assert.Equal(t, "", tg.Spec.LatticeID)
				if tt.wantIPv6TargetGroup {
					assert.Equal(t, vpclattice.IpAddressTypeIpv6, tg.Spec.Config.IpAddressType)
				} else {
					assert.Equal(t, vpclattice.IpAddressTypeIpv4, tg.Spec.Config.IpAddressType)
				}
			}

		})
	}
}

func Test_TGModelByHTTPRouteBuild(t *testing.T) {
	now := metav1.Now()

	namespacePtr := func(ns string) *gateway_api.Namespace {
		p := gateway_api.Namespace(ns)
		return &p
	}

	kindPtr := func(k string) *gateway_api.Kind {
		p := gateway_api.Kind(k)
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
			route: core.NewHTTPRoute(gateway_api.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name: "service1",
				},
				Spec: gateway_api.HTTPRouteSpec{
					CommonRouteSpec: gateway_api.CommonRouteSpec{
						ParentRefs: []gateway_api.ParentReference{
							{
								Name: "gateway1",
							},
						},
					},
					Rules: []gateway_api.HTTPRouteRule{
						{
							BackendRefs: []gateway_api.HTTPBackendRef{
								{
									BackendRef: gateway_api.BackendRef{
										BackendObjectReference: gateway_api.BackendObjectReference{
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
			route: core.NewHTTPRoute(gateway_api.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "service2",
					Finalizers:        []string{"gateway.k8s.aws/resources"},
					DeletionTimestamp: &now,
				},
				Spec: gateway_api.HTTPRouteSpec{
					CommonRouteSpec: gateway_api.CommonRouteSpec{
						ParentRefs: []gateway_api.ParentReference{
							{
								Name: "gateway1",
							},
						},
					},
					Rules: []gateway_api.HTTPRouteRule{
						{
							BackendRefs: []gateway_api.HTTPBackendRef{
								{
									BackendRef: gateway_api.BackendRef{
										BackendObjectReference: gateway_api.BackendObjectReference{
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
			route: core.NewHTTPRoute(gateway_api.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "service3",
					Finalizers: []string{"gateway.k8s.aws/resources"},
				},
				Spec: gateway_api.HTTPRouteSpec{
					CommonRouteSpec: gateway_api.CommonRouteSpec{
						ParentRefs: []gateway_api.ParentReference{
							{
								Name: "gateway1",
							},
						},
					},
					Rules: []gateway_api.HTTPRouteRule{
						{
							BackendRefs: []gateway_api.HTTPBackendRef{
								{
									BackendRef: gateway_api.BackendRef{
										BackendObjectReference: gateway_api.BackendObjectReference{
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
			name: "Create LatticeService where backend mcs serviceimport does NOT exist",
			route: core.NewHTTPRoute(gateway_api.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "service4",
					Finalizers: []string{"gateway.k8s.aws/resources"},
				},
				Spec: gateway_api.HTTPRouteSpec{
					CommonRouteSpec: gateway_api.CommonRouteSpec{
						ParentRefs: []gateway_api.ParentReference{
							{
								Name: "gateway1",
							},
						},
					},
					Rules: []gateway_api.HTTPRouteRule{
						{
							BackendRefs: []gateway_api.HTTPBackendRef{
								{
									BackendRef: gateway_api.BackendRef{
										BackendObjectReference: gateway_api.BackendObjectReference{
											Name:      "service4-tg1",
											Namespace: namespacePtr("ns31"),
											Kind:      kindPtr("ServiceImport"),
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
			route: core.NewHTTPRoute(gateway_api.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "service5",
					Finalizers: []string{"gateway.k8s.aws/resources"},
				},
				Spec: gateway_api.HTTPRouteSpec{
					CommonRouteSpec: gateway_api.CommonRouteSpec{
						ParentRefs: []gateway_api.ParentReference{
							{
								Name: "gateway1",
							},
						},
					},
					Rules: []gateway_api.HTTPRouteRule{
						{
							BackendRefs: []gateway_api.HTTPBackendRef{
								{
									BackendRef: gateway_api.BackendRef{
										BackendObjectReference: gateway_api.BackendObjectReference{
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
			v1alpha1.AddToScheme(k8sSchema)
			k8sClient := testclient.NewFakeClientWithScheme(k8sSchema)
			ds := latticestore.NewLatticeDataStore()

			//builder := NewLatticeServiceBuilder(k8sClient, ds, nil)

			stack := core.NewDefaultStack(core.StackID(k8s.NamespacedName(tt.route.K8sObject())))

			task := &latticeServiceModelBuildTask{
				log:       gwlog.FallbackLogger,
				route:     tt.route,
				stack:     stack,
				client:    k8sClient,
				tgByResID: make(map[string]*latticemodel.TargetGroup),
				datastore: ds,
			}

			if tt.svcExist {
				// populate K8S service
				for _, httpRules := range tt.route.Spec().Rules() {
					for _, httpBackendRef := range httpRules.BackendRefs() {
						if tt.svcExist {
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

							fmt.Printf("create K8S service %v\n", svc)

							assert.NoError(t, k8sClient.Create(ctx, svc.DeepCopy()))
						}

					}
				}
			}

			err := task.buildTargetGroupsForRoute(ctx, k8sClient)

			if !tt.wantErrIsNil {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
			}

			if tt.wantErrIsNil {
				// verify data store
				for _, httpRules := range tt.route.Spec().Rules() {
					for _, httpBackendRef := range httpRules.BackendRefs() {
						tgName := latticestore.TargetGroupName(string(httpBackendRef.Name()), string(*httpBackendRef.Namespace()))

						fmt.Printf("httpBackendRef %s\n", *httpBackendRef.Kind())
						if "Service" == *httpBackendRef.Kind() {
							if tt.wantIsDeleted {
								tg := task.tgByResID[tgName]
								fmt.Printf("--task.tgByResID[tgName] %v \n", tg)
								assert.Equal(t, true, tg.Spec.IsDeleted)
							} else {
								dsTG, err := ds.GetTargetGroup(tgName, tt.route.Name(), false)
								assert.Equal(t, true, dsTG.ByBackendRef)
								fmt.Printf("--dsTG %v\n", dsTG)
								assert.Nil(t, err)
							}

							// Verify IpAddressType of Target Group
							tg := task.tgByResID[tgName]

							ipAddressType := tg.Spec.Config.IpAddressType

							if tt.wantIPv6TargetGroup {
								assert.Equal(t, vpclattice.IpAddressTypeIpv6, ipAddressType)
							} else {
								assert.Equal(t, vpclattice.IpAddressTypeIpv4, ipAddressType)
							}
						} else {
							// the routeName for serviceimport is ""
							dsTG, err := ds.GetTargetGroup(tgName, "", true)
							fmt.Printf("dsTG %v\n", dsTG)
							assert.Nil(t, err)
						}
						assert.Nil(t, err)

					}
				}

			}

		})
	}
}

func Test_TGModelByHTTPRouteImportBuild(t *testing.T) {
	now := metav1.Now()
	namespacePtr := func(ns string) *gateway_api.Namespace {
		p := gateway_api.Namespace(ns)
		return &p
	}

	kindPtr := func(k string) *gateway_api.Kind {
		p := gateway_api.Kind(k)
		return &p
	}

	tests := []struct {
		name           string
		route          core.Route
		svcImportExist bool
		wantError      error
		wantErrIsNil   bool
		wantName       string
		wantIsDeleted  bool
	}{
		{
			name: "Add LatticeService",
			route: core.NewHTTPRoute(gateway_api.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name: "serviceimport1",
				},
				Spec: gateway_api.HTTPRouteSpec{
					CommonRouteSpec: gateway_api.CommonRouteSpec{
						ParentRefs: []gateway_api.ParentReference{
							{
								Name: "gateway1",
							},
						},
					},
					Rules: []gateway_api.HTTPRouteRule{
						{
							BackendRefs: []gateway_api.HTTPBackendRef{
								{
									BackendRef: gateway_api.BackendRef{
										BackendObjectReference: gateway_api.BackendObjectReference{
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
			svcImportExist: true,
			wantError:      nil,
			wantName:       "service1",
			wantIsDeleted:  false,
			wantErrIsNil:   true,
		},
		{
			name: "Add LatticeService, implicit namespace",
			route: core.NewHTTPRoute(gateway_api.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "serviceimport1",
					Namespace: "tg1-ns2",
				},
				Spec: gateway_api.HTTPRouteSpec{
					CommonRouteSpec: gateway_api.CommonRouteSpec{
						ParentRefs: []gateway_api.ParentReference{
							{
								Name: "gateway1",
							},
						},
					},
					Rules: []gateway_api.HTTPRouteRule{
						{
							BackendRefs: []gateway_api.HTTPBackendRef{
								{
									BackendRef: gateway_api.BackendRef{
										BackendObjectReference: gateway_api.BackendObjectReference{
											Name: "service1-tg2",
											Kind: kindPtr("ServiceImport"),
										},
									},
								},
							},
						},
					},
				},
			}),
			svcImportExist: true,
			wantError:      nil,
			wantName:       "service1",
			wantIsDeleted:  false,
			wantErrIsNil:   true,
		},
		{
			name: "Delete LatticeService",
			route: core.NewHTTPRoute(gateway_api.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "serviceimport2",
					Finalizers:        []string{"gateway.k8s.aws/resources"},
					DeletionTimestamp: &now,
				},
				Spec: gateway_api.HTTPRouteSpec{
					CommonRouteSpec: gateway_api.CommonRouteSpec{
						ParentRefs: []gateway_api.ParentReference{
							{
								Name: "gateway1",
							},
						},
					},
					Rules: []gateway_api.HTTPRouteRule{

						{
							BackendRefs: []gateway_api.HTTPBackendRef{
								{
									BackendRef: gateway_api.BackendRef{
										BackendObjectReference: gateway_api.BackendObjectReference{
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
			svcImportExist: true,
			wantError:      nil,
			wantName:       "service1",
			wantIsDeleted:  true,
			wantErrIsNil:   true,
		},
	}

	for _, tt := range tests {
		fmt.Printf("Test >>>> %v\n", tt.name)
		c := gomock.NewController(t)
		defer c.Finish()
		ctx := context.Background()

		k8sClient := mock_client.NewMockClient(c)

		ds := latticestore.NewLatticeDataStore()

		//builder := NewLatticeServiceBuilder(k8sClient, ds, nil)

		stack := core.NewDefaultStack(core.StackID(k8s.NamespacedName(tt.route.K8sObject())))

		task := &latticeServiceModelBuildTask{
			log:       gwlog.FallbackLogger,
			route:     tt.route,
			stack:     stack,
			client:    k8sClient,
			tgByResID: make(map[string]*latticemodel.TargetGroup),
			datastore: ds,
		}

		for _, httpRules := range tt.route.Spec().Rules() {
			for _, httpBackendRef := range httpRules.BackendRefs() {
				if tt.svcImportExist {
					k8sClient.EXPECT().Get(ctx, gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
						func(ctx context.Context, name types.NamespacedName, svcImport *mcs_api.ServiceImport, arg3 ...interface{}) error {
							//TODO add more
							svcImport.ObjectMeta.Name = string(httpBackendRef.Name())
							return nil
						},
					)
				} else {
					k8sClient.EXPECT().Get(ctx, gomock.Any(), gomock.Any()).Return(errors.New("serviceimport not exist"))
				}
			}
		}
		k8sClient.EXPECT().List(ctx, gomock.Any(), gomock.Any()).Return(nil)

		err := task.buildTargetGroupsForRoute(ctx, k8sClient)

		fmt.Printf("err %v\n", err)

		if !tt.wantErrIsNil {
			assert.NotNil(t, err)
		} else {
			assert.Nil(t, err)
		}

		if tt.wantErrIsNil {
			// verify data store
			for _, httpRules := range tt.route.Spec().Rules() {
				for _, httpBackendRef := range httpRules.BackendRefs() {
					ns := tt.route.Namespace()
					if httpBackendRef.Namespace() != nil {
						ns = string(*httpBackendRef.Namespace())
					}
					tgName := latticestore.TargetGroupName(string(httpBackendRef.Name()), ns)

					fmt.Printf("httpBackendRef %s\n", *httpBackendRef.Kind())
					if "Service" == *httpBackendRef.Kind() {
						if tt.wantIsDeleted {
							tg := task.tgByResID[tgName]
							fmt.Printf("--task.tgByResID[tgName] %v \n", tg)
							assert.Equal(t, true, tg.Spec.IsDeleted)
						} else {
							dsTG, err := ds.GetTargetGroup(tgName, tt.route.Name(), false)
							assert.Equal(t, true, dsTG.ByBackendRef)
							fmt.Printf("--dsTG %v\n", dsTG)
							assert.Nil(t, err)
						}
					} else {
						dsTG, err := ds.GetTargetGroup(tgName, "", true)
						fmt.Printf("dsTG %v\n", dsTG)
						if tt.wantIsDeleted {
							tg := task.tgByResID[tgName]
							assert.Equal(t, true, tg.Spec.IsDeleted)
						} else {
							assert.Equal(t, false, dsTG.ByBackendRef)
							assert.Equal(t, false, dsTG.ByServiceExport)
							assert.Nil(t, err)
						}
					}
					assert.Nil(t, err)
				}
			}

		}
	}
}

func Test_buildTargetGroupIpAdressType(t *testing.T) {
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
			got, err := buildTargetGroupIpAdressType(tt.args.svc)
			if (err != nil) != tt.wantErr {
				t.Errorf("buildTargetGroupIpAdressType() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("buildTargetGroupIpAdressType() = %v, want %v", got, tt.want)
			}
		})
	}
}
