package gateway

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/gateway-api/apis/v1alpha2"
	mcs_api "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	mock_client "github.com/aws/aws-application-networking-k8s/mocks/controller-runtime/client"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

func Test_TGModelByServicexportBuild(t *testing.T) {
	now := metav1.Now()
	tests := []struct {
		name          string
		svcExport     *mcs_api.ServiceExport
		svc           *corev1.Service
		endPoints     []corev1.Endpoints
		wantErrIsNil  bool
		wantIsDeleted bool
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
			wantErrIsNil:  true,
			wantIsDeleted: false,
		},
		{
			name: "Adding ServieExport where service object does NOT exist",
			svcExport: &mcs_api.ServiceExport{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "export2",
					Namespace: "ns1",
				},
			},

			wantErrIsNil:  false,
			wantIsDeleted: false,
		},
		{
			name: "Deleting ServieExport where service object does NOT exist",
			svcExport: &mcs_api.ServiceExport{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "export3",
					Namespace:         "ns1",
					Finalizers:        []string{"gateway.k8s.aws/resources"},
					DeletionTimestamp: &now,
				},
			},

			wantErrIsNil:  false,
			wantIsDeleted: true,
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
			wantErrIsNil:  true,
			wantIsDeleted: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := gomock.NewController(t)
			defer c.Finish()
			ctx := context.TODO()

			k8sSchema := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sSchema)
			k8sClient := testclient.NewFakeClientWithScheme(k8sSchema)

			if tt.svc != nil {
				assert.NoError(t, k8sClient.Create(ctx, tt.svc.DeepCopy()))

			}

			if tt.endPoints != nil {
				assert.NoError(t, k8sClient.Create(ctx, tt.endPoints[0].DeepCopy()))
			}

			ds := latticestore.NewLatticeDataStore()

			builder := NewTargetGroupBuilder(k8sClient, ds, nil)

			stack, tg, err := builder.Build(ctx, tt.svcExport)

			fmt.Printf("stack %v tg %v err %v\n", stack, tg, err)

			if !tt.wantErrIsNil {
				assert.NotNil(t, err)
				return
			}

			assert.Nil(t, err)
			tgName := latticestore.TargetGroupName(tt.svcExport.Name, tt.svcExport.Namespace)
			assert.Equal(t, tgName, tg.Spec.Name)

			dsTG, err := ds.GetTargetGroup(tgName, false)
			assert.Nil(t, err)
			if tt.wantIsDeleted {
				assert.Equal(t, false, dsTG.ByServiceExport)
				assert.Equal(t, true, tg.Spec.IsDeleted)
			} else {
				assert.Equal(t, true, dsTG.ByServiceExport)
			}

		})
	}
}

func Test_TGModelByHTTPRouteBuild(t *testing.T) {
	now := metav1.Now()

	namespacePtr := func(ns string) *v1alpha2.Namespace {
		p := v1alpha2.Namespace(ns)
		return &p
	}

	kindPtr := func(k string) *v1alpha2.Kind {
		p := v1alpha2.Kind(k)
		return &p
	}

	tests := []struct {
		name          string
		httpRoute     *v1alpha2.HTTPRoute
		svcExist      bool
		wantError     error
		wantErrIsNil  bool
		wantName      string
		wantIsDeleted bool
	}{
		{
			name: "Add LatticeService",
			httpRoute: &v1alpha2.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name: "service1",
				},
				Spec: v1alpha2.HTTPRouteSpec{
					CommonRouteSpec: v1alpha2.CommonRouteSpec{
						ParentRefs: []v1alpha2.ParentRef{
							{
								Name: "gateway1",
							},
						},
					},
					Rules: []v1alpha2.HTTPRouteRule{
						{
							BackendRefs: []v1alpha2.HTTPBackendRef{
								{
									BackendRef: v1alpha2.BackendRef{
										BackendObjectReference: v1alpha2.BackendObjectReference{
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
			},
			svcExist:      true,
			wantError:     nil,
			wantName:      "service1",
			wantIsDeleted: false,
			wantErrIsNil:  true,
		},
		{
			name: "Delete LatticeService",
			httpRoute: &v1alpha2.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "service2",
					Finalizers:        []string{"gateway.k8s.aws/resources"},
					DeletionTimestamp: &now,
				},
				Spec: v1alpha2.HTTPRouteSpec{
					CommonRouteSpec: v1alpha2.CommonRouteSpec{
						ParentRefs: []v1alpha2.ParentRef{
							{
								Name: "gateway1",
							},
						},
					},
					Rules: []v1alpha2.HTTPRouteRule{
						{
							BackendRefs: []v1alpha2.HTTPBackendRef{
								{
									BackendRef: v1alpha2.BackendRef{
										BackendObjectReference: v1alpha2.BackendObjectReference{
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
			},
			svcExist:      true,
			wantError:     nil,
			wantName:      "service1",
			wantIsDeleted: true,
			wantErrIsNil:  true,
		},
		{
			name: "Create LatticeService where backend K8S service does NOT exist",
			httpRoute: &v1alpha2.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "service3",
					Finalizers:        []string{"gateway.k8s.aws/resources"},
					DeletionTimestamp: &now,
				},
				Spec: v1alpha2.HTTPRouteSpec{
					CommonRouteSpec: v1alpha2.CommonRouteSpec{
						ParentRefs: []v1alpha2.ParentRef{
							{
								Name: "gateway1",
							},
						},
					},
					Rules: []v1alpha2.HTTPRouteRule{
						{
							BackendRefs: []v1alpha2.HTTPBackendRef{
								{
									BackendRef: v1alpha2.BackendRef{
										BackendObjectReference: v1alpha2.BackendObjectReference{
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
			},
			svcExist:      false,
			wantError:     nil,
			wantName:      "service1",
			wantIsDeleted: false,
			wantErrIsNil:  false,
		},
		{
			name: "Create LatticeService where backend mcs serviceimport does NOT exist",
			httpRoute: &v1alpha2.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "service4",
					Finalizers:        []string{"gateway.k8s.aws/resources"},
					DeletionTimestamp: &now,
				},
				Spec: v1alpha2.HTTPRouteSpec{
					CommonRouteSpec: v1alpha2.CommonRouteSpec{
						ParentRefs: []v1alpha2.ParentRef{
							{
								Name: "gateway1",
							},
						},
					},
					Rules: []v1alpha2.HTTPRouteRule{
						{
							BackendRefs: []v1alpha2.HTTPBackendRef{
								{
									BackendRef: v1alpha2.BackendRef{
										BackendObjectReference: v1alpha2.BackendObjectReference{
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
			},
			svcExist:      false,
			wantError:     nil,
			wantName:      "service1",
			wantIsDeleted: false,
			wantErrIsNil:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := gomock.NewController(t)
			defer c.Finish()
			ctx := context.TODO()

			k8sSchema := runtime.NewScheme()
			clientgoscheme.AddToScheme(k8sSchema)
			k8sClient := testclient.NewFakeClientWithScheme(k8sSchema)
			ds := latticestore.NewLatticeDataStore()

			//builder := NewLatticeServiceBuilder(k8sClient, ds, nil)

			stack := core.NewDefaultStack(core.StackID(k8s.NamespacedName(tt.httpRoute)))

			task := &latticeServiceModelBuildTask{
				httpRoute: tt.httpRoute,
				stack:     stack,
				Client:    k8sClient,
				tgByResID: make(map[string]*latticemodel.TargetGroup),
				Datastore: ds,
			}

			if tt.svcExist {
				// populate K8S service
				for _, httpRules := range tt.httpRoute.Spec.Rules {
					for _, httpBackendRef := range httpRules.BackendRefs {
						if tt.svcExist {
							svc := corev1.Service{
								ObjectMeta: metav1.ObjectMeta{
									Name:      string(httpBackendRef.Name),
									Namespace: string(*httpBackendRef.Namespace),
								},
							}
							fmt.Printf("create K8S service %v\n", svc)
							assert.NoError(t, k8sClient.Create(ctx, svc.DeepCopy()))
						}

					}
				}
			}

			_, err := task.buildTargetGroup(ctx, k8sClient)

			if !tt.wantErrIsNil {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
			}

			if tt.wantErrIsNil {
				// verify data store
				for _, httpRules := range tt.httpRoute.Spec.Rules {
					for _, httpBackendRef := range httpRules.BackendRefs {
						tgName := latticestore.TargetGroupName(string(httpBackendRef.Name), string(*httpBackendRef.Namespace))

						fmt.Printf("httpBacndendRef %s\n", *httpBackendRef.BackendObjectReference.Kind)
						if "Service" == *httpBackendRef.BackendObjectReference.Kind {
							if tt.wantIsDeleted {
								tg := task.tgByResID[tgName]
								fmt.Printf("--task.tgByResID[tgName] %v \n", tg)
								assert.Equal(t, true, tg.Spec.IsDeleted)
							} else {
								dsTG, err := ds.GetTargetGroup(tgName, false)
								assert.Equal(t, true, dsTG.ByBackendRef)
								fmt.Printf("--dsTG %v\n", dsTG)
								assert.Nil(t, err)
							}
						} else {
							dsTG, err := ds.GetTargetGroup(tgName, true)
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
	t.Skip("model_build_targetgroup.go:292: wrong number of arguments in DoAndReturn func for *mock_client.MockClient.Get: got 3, want 4")
	now := metav1.Now()
	namespacePtr := func(ns string) *v1alpha2.Namespace {
		p := v1alpha2.Namespace(ns)
		return &p
	}

	kindPtr := func(k string) *v1alpha2.Kind {
		p := v1alpha2.Kind(k)
		return &p
	}

	tests := []struct {
		name           string
		httpRoute      *v1alpha2.HTTPRoute
		svcImportExist bool
		wantError      error
		wantErrIsNil   bool
		wantName       string
		wantIsDeleted  bool
	}{
		{
			name: "Add LatticeService",
			httpRoute: &v1alpha2.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name: "serviceimport1",
				},
				Spec: v1alpha2.HTTPRouteSpec{
					CommonRouteSpec: v1alpha2.CommonRouteSpec{
						ParentRefs: []v1alpha2.ParentRef{
							{
								Name: "gateway1",
							},
						},
					},
					Rules: []v1alpha2.HTTPRouteRule{

						{
							BackendRefs: []v1alpha2.HTTPBackendRef{
								{
									BackendRef: v1alpha2.BackendRef{
										BackendObjectReference: v1alpha2.BackendObjectReference{
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
			},
			svcImportExist: true,
			wantError:      nil,
			wantName:       "service1",
			wantIsDeleted:  false,
			wantErrIsNil:   true,
		},
		{
			name: "Delete LatticeService",
			httpRoute: &v1alpha2.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "serviceimport2",
					Finalizers:        []string{"gateway.k8s.aws/resources"},
					DeletionTimestamp: &now,
				},
				Spec: v1alpha2.HTTPRouteSpec{
					CommonRouteSpec: v1alpha2.CommonRouteSpec{
						ParentRefs: []v1alpha2.ParentRef{
							{
								Name: "gateway1",
							},
						},
					},
					Rules: []v1alpha2.HTTPRouteRule{

						{
							BackendRefs: []v1alpha2.HTTPBackendRef{
								{
									BackendRef: v1alpha2.BackendRef{
										BackendObjectReference: v1alpha2.BackendObjectReference{
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
			},
			svcImportExist: true,
			wantError:      nil,
			wantName:       "service1",
			wantIsDeleted:  true,
			wantErrIsNil:   true,
		},
	}

	for _, tt := range tests {
		c := gomock.NewController(t)
		defer c.Finish()
		ctx := context.Background()

		k8sClient := mock_client.NewMockClient(c)

		ds := latticestore.NewLatticeDataStore()

		//builder := NewLatticeServiceBuilder(k8sClient, ds, nil)

		stack := core.NewDefaultStack(core.StackID(k8s.NamespacedName(tt.httpRoute)))

		task := &latticeServiceModelBuildTask{
			httpRoute: tt.httpRoute,
			stack:     stack,
			Client:    k8sClient,
			tgByResID: make(map[string]*latticemodel.TargetGroup),
			Datastore: ds,
		}

		for _, httpRules := range tt.httpRoute.Spec.Rules {
			for _, httpBackendRef := range httpRules.BackendRefs {
				if tt.svcImportExist {
					k8sClient.EXPECT().Get(ctx, gomock.Any(), gomock.Any()).DoAndReturn(
						func(ctx context.Context, name types.NamespacedName, svcImport *mcs_api.ServiceImport) error {
							//TODO add more
							svcImport.ObjectMeta.Name = string(httpBackendRef.Name)
							return nil
						},
					)
				} else {
					k8sClient.EXPECT().Get(ctx, gomock.Any(), gomock.Any()).Return(errors.New("serviceimport not exist"))
				}
			}
		}
		_, err := task.buildTargetGroup(ctx, k8sClient)

		fmt.Printf("err %v\n", err)

		if !tt.wantErrIsNil {
			assert.NotNil(t, err)
		} else {
			assert.Nil(t, err)
		}

		if tt.wantErrIsNil {
			// verify data store
			for _, httpRules := range tt.httpRoute.Spec.Rules {
				for _, httpBackendRef := range httpRules.BackendRefs {
					tgName := latticestore.TargetGroupName(string(httpBackendRef.Name), string(*httpBackendRef.Namespace))

					fmt.Printf("httpBacndendRef %s\n", *httpBackendRef.BackendObjectReference.Kind)
					if "Service" == *httpBackendRef.BackendObjectReference.Kind {
						if tt.wantIsDeleted {
							tg := task.tgByResID[tgName]
							fmt.Printf("--task.tgByResID[tgName] %v \n", tg)
							assert.Equal(t, true, tg.Spec.IsDeleted)
						} else {
							dsTG, err := ds.GetTargetGroup(tgName, false)
							assert.Equal(t, true, dsTG.ByBackendRef)
							fmt.Printf("--dsTG %v\n", dsTG)
							assert.Nil(t, err)
						}
					} else {
						dsTG, err := ds.GetTargetGroup(tgName, true)
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
