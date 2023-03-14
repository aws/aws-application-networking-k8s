package lattice

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/gateway-api/apis/v1alpha2"
	mcs_api "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	mock_client "github.com/aws/aws-application-networking-k8s/mocks/controller-runtime/client"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/gateway"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

func Test_SynthesizeTriggeredServiceExport(t *testing.T) {
	now := metav1.Now()
	tests := []struct {
		name           string
		svcExport      *mcs_api.ServiceExport
		tgManagerError bool
		wantErrIsNil   bool
	}{
		{
			name: "Adding a new targetgroup, ok case",
			svcExport: &mcs_api.ServiceExport{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "export1",
					Namespace: "ns1",
				},
			},
			tgManagerError: false,
			wantErrIsNil:   true,
		},
		{
			name: "Adding a new targetgroup, nok case",
			svcExport: &mcs_api.ServiceExport{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "export2",
					Namespace: "ns1",
				},
			},
			tgManagerError: true,
			wantErrIsNil:   false,
		},
		{
			name: "Deleting a targetgroup, ok case",
			svcExport: &mcs_api.ServiceExport{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "export3",
					Namespace:         "ns1",
					Finalizers:        []string{"gateway.k8s.aws/resources"},
					DeletionTimestamp: &now,
				},
			},
			tgManagerError: false,
			wantErrIsNil:   true,
		},
		{
			name: "Deleting a targetgroup, nok case",
			svcExport: &mcs_api.ServiceExport{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "export3",
					Namespace:         "ns1",
					Finalizers:        []string{"gateway.k8s.aws/resources"},
					DeletionTimestamp: &now,
				},
			},
			tgManagerError: true,
			wantErrIsNil:   false,
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

			svc := corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tt.svcExport.Name,
					Namespace: tt.svcExport.Namespace,
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{},
					},
				},
			}

			k8sClient.Create(ctx, svc.DeepCopy())

			mockTGManager := NewMockTargetGroupManager(c)

			ds := latticestore.NewLatticeDataStore()

			builder := gateway.NewTargetGroupBuilder(k8sClient, ds, nil)

			stack, tg, err := builder.Build(ctx, tt.svcExport)
			assert.Nil(t, err)

			synthersizer := NewTargetGroupSynthesizer(nil, nil, mockTGManager, stack, ds)

			tgStatus := latticemodel.TargetGroupStatus{
				TargetGroupARN: "arn123",
				TargetGroupID:  "4567",
			}

			if tg.Spec.IsDeleted {
				if tt.tgManagerError {
					mockTGManager.EXPECT().Delete(ctx, tg).Return(errors.New("ERROR"))
				} else {
					mockTGManager.EXPECT().Delete(ctx, tg).Return(nil)
				}
			} else {
				if tt.tgManagerError {
					mockTGManager.EXPECT().Create(ctx, tg).Return(tgStatus, errors.New("ERROR"))
				} else {
					mockTGManager.EXPECT().Create(ctx, tg).Return(tgStatus, nil)
				}
			}

			err = synthersizer.SynthesizeTriggeredTargetGroup(ctx)

			if !tt.wantErrIsNil {
				assert.NotNil(t, err)
				return
			}

			assert.Nil(t, err)
			tgName := latticestore.TargetGroupName(tt.svcExport.Name, tt.svcExport.Namespace)
			dsTG, err := ds.GetTargetGroup(tgName, false)

			if tg.Spec.IsDeleted {
				assert.NotNil(t, err)

			} else {
				assert.Nil(t, err)
				assert.Equal(t, dsTG.ARN, tgStatus.TargetGroupARN)
				assert.Equal(t, dsTG.ID, tgStatus.TargetGroupID)
			}
		})
	}
}

type svcImportDef struct {
	name    string
	tgARN   string
	tgID    string
	tgExist bool
	mgrErr  bool
}

func Test_SynthersizeTriggeredByServiceImport(t *testing.T) {
	tests := []struct {
		name          string
		svcImportList []svcImportDef
		isDeleted     bool
		wantErrIsNil  bool
	}{
		{
			name: "service import triggered target group",
			svcImportList: []svcImportDef{
				{
					name:    "service-import1",
					tgARN:   "service-import1-arn",
					tgID:    "service-import1-ID",
					tgExist: true,
					mgrErr:  false,
				},
				{
					name:    "service-import2",
					tgARN:   "service-import2-arn",
					tgID:    "service-import2-ID",
					tgExist: true,
					mgrErr:  false,
				},
			},
			isDeleted:    false,
			wantErrIsNil: true,
		},
		{
			name: "service import triggered target group, 1st one return err",
			svcImportList: []svcImportDef{
				{
					name:    "service-import21",
					tgExist: false,
					mgrErr:  true,
				},
				{
					name:    "service-import22",
					tgARN:   "service-import22-arn",
					tgID:    "service-import22-ID",
					tgExist: true,
					mgrErr:  false,
				},
			},
			isDeleted:    false,
			wantErrIsNil: false,
		},
		{
			name: "service import triggered target group, 1st one return err",
			svcImportList: []svcImportDef{
				{
					name:    "service-import31",
					tgExist: false,
					mgrErr:  true,
				},
				{
					name:    "service-import32",
					tgARN:   "service-import32-arn",
					tgID:    "service-import32-ID",
					tgExist: true,
					mgrErr:  false,
				},
			},
			isDeleted:    true,
			wantErrIsNil: true,
		},
	}
	for _, tt := range tests {
		c := gomock.NewController(t)
		defer c.Finish()
		ctx := context.TODO()

		mockTGManager := NewMockTargetGroupManager(c)

		ds := latticestore.NewLatticeDataStore()

		stack := core.NewDefaultStack(core.StackID(types.NamespacedName{Namespace: "tt", Name: "name"}))

		for _, tgImport := range tt.svcImportList {

			tgSpec := latticemodel.TargetGroupSpec{
				Name: tgImport.name,
				Type: latticemodel.TargetGroupTypeIP,
				Config: latticemodel.TargetGroupConfig{
					IsServiceImport: true,
				},
				IsDeleted: tt.isDeleted,
			}

			tg := latticemodel.NewTargetGroup(stack, tgImport.name, tgSpec)
			fmt.Printf("tg : %v\n", tg)

			if tt.isDeleted {
				continue
			}

			if tgImport.mgrErr {
				mockTGManager.EXPECT().Get(ctx, tg).Return(latticemodel.TargetGroupStatus{}, errors.New("tgmgr err"))
			} else {
				mockTGManager.EXPECT().Get(ctx, tg).Return(latticemodel.TargetGroupStatus{TargetGroupARN: tgImport.tgARN, TargetGroupID: tgImport.tgID}, nil)
			}
		}
		synthesizer := NewTargetGroupSynthesizer(nil, nil, mockTGManager, stack, ds)
		err := synthesizer.SynthesizeTriggeredTargetGroup(ctx)
		fmt.Printf("err:%v \n", err)

		if tt.wantErrIsNil {
			assert.Nil(t, err)
		} else {
			assert.NotNil(t, err)
		}

		if !tt.isDeleted {
			// check datastore
			for _, tgImport := range tt.svcImportList {
				if tgImport.mgrErr {
					_, err := ds.GetTargetGroup(tgImport.name, true)
					assert.NotNil(t, err)

				} else {
					tg, err := ds.GetTargetGroup(tgImport.name, true)
					assert.Nil(t, err)
					assert.Equal(t, tgImport.tgARN, tg.ARN)
					assert.Equal(t, tgImport.tgID, tg.ID)

				}
			}
		}

	}
}

type sdkTGDef struct {
	name string
	id   string

	isSameVPC                bool
	hasTags                  bool
	hasServiceExportTypeTag  bool
	hasHTTPRouteTypeTag      bool
	serviceExportExist       bool
	HTTPRouteExist           bool
	refedByHTTPRoute         bool
	serviceNetworkManagerErr error

	expectDelete bool
}

func Test_SynthesizeSDKTargetGroups(t *testing.T) {
	kindPtr := func(k string) *v1alpha2.Kind {
		p := v1alpha2.Kind(k)
		return &p
	}

	config.VpcID = "current-vpc"
	srvname := "test-svc1"
	srvnamespace := "default"
	routename := "test-route1"
	routenamespace := "default"
	tests := []struct {
		name                 string
		sdkTargetGroups      []sdkTGDef
		wantSynthesizerError error
		wantDataStoreError   error
		wantDataStoreStatus  string
	}{

		{
			name: "Delete SDK TargetGroup Successfully(due to not refed by any HTTPRoutes) ",
			sdkTargetGroups: []sdkTGDef{
				{name: "sdkTG1", id: "sdkTG1-id", serviceNetworkManagerErr: nil,
					isSameVPC: true,
					hasTags:   true, hasServiceExportTypeTag: false,
					hasHTTPRouteTypeTag: true, HTTPRouteExist: false,
					expectDelete: true},
			},
			wantSynthesizerError: nil,
			wantDataStoreError:   nil,
			wantDataStoreStatus:  "",
		},

		{
			name: "Delete SDK TargetGroup Successfully(due to not in backend ref of a HTTPRoutes) ",
			sdkTargetGroups: []sdkTGDef{
				{name: "sdkTG1", id: "sdkTG1-id", serviceNetworkManagerErr: nil,
					isSameVPC: true,
					hasTags:   true, hasServiceExportTypeTag: false,
					hasHTTPRouteTypeTag: true, HTTPRouteExist: true, refedByHTTPRoute: false,
					expectDelete: true},
			},
			wantSynthesizerError: nil,
			wantDataStoreError:   nil,
			wantDataStoreStatus:  "",
		},
		{
			name: "No need to delete SDK TargetGroup since it is referenced by a HTTPRoutes) ",
			sdkTargetGroups: []sdkTGDef{
				{name: "sdkTG1", id: "sdkTG1-id", serviceNetworkManagerErr: nil,
					isSameVPC: true,
					hasTags:   true, hasServiceExportTypeTag: false,
					hasHTTPRouteTypeTag: true, HTTPRouteExist: true, refedByHTTPRoute: true,
					expectDelete: false},
			},
			wantSynthesizerError: nil,
			wantDataStoreError:   nil,
			wantDataStoreStatus:  "",
		},
		{
			name: "No need to delete SDK TargetGroup , no K8S tags ",
			sdkTargetGroups: []sdkTGDef{
				{name: "sdkTG1", id: "sdkTG1-id", serviceNetworkManagerErr: nil,
					isSameVPC: true,
					hasTags:   false, hasServiceExportTypeTag: false,
					hasHTTPRouteTypeTag: false, HTTPRouteExist: false, refedByHTTPRoute: false,
					expectDelete: false},
			},
			wantSynthesizerError: nil,
			wantDataStoreError:   nil,
			wantDataStoreStatus:  "",
		},
		{
			name: "No need to delete SDK TargetGroup , different VPC",
			sdkTargetGroups: []sdkTGDef{
				{name: "sdkTG1", id: "sdkTG1-id", serviceNetworkManagerErr: nil,
					isSameVPC: false,
					hasTags:   false, hasServiceExportTypeTag: false,
					hasHTTPRouteTypeTag: false, HTTPRouteExist: false, refedByHTTPRoute: false,
					expectDelete: false},
			},
			wantSynthesizerError: nil,
			wantDataStoreError:   nil,
			wantDataStoreStatus:  "",
		},

		{
			name: "delete SDK TargetGroup due not referenced by any serviceexport",
			sdkTargetGroups: []sdkTGDef{
				{name: "sdkTG1", id: "sdkTG1-id", serviceNetworkManagerErr: nil,
					isSameVPC: true,
					hasTags:   true, hasServiceExportTypeTag: true, serviceExportExist: false,
					hasHTTPRouteTypeTag: false, HTTPRouteExist: false, refedByHTTPRoute: false,
					expectDelete: true},
			},
			wantSynthesizerError: nil,
			wantDataStoreError:   nil,
			wantDataStoreStatus:  "",
		},

		{
			name: "no need to delete SDK TargetGroup since it is referenced by an serviceexport",
			sdkTargetGroups: []sdkTGDef{
				{name: "sdkTG1", id: "sdkTG1-id", serviceNetworkManagerErr: nil,
					isSameVPC: true,
					hasTags:   true, hasServiceExportTypeTag: true, serviceExportExist: true,
					hasHTTPRouteTypeTag: false, HTTPRouteExist: false, refedByHTTPRoute: false,
					expectDelete: false},
			},
			wantSynthesizerError: nil,
			wantDataStoreError:   nil,
			wantDataStoreStatus:  "",
		},
	}

	for _, tt := range tests {
		fmt.Printf(" testcase >>>> %s \n", tt.name)
		c := gomock.NewController(t)
		defer c.Finish()
		ctx := context.Background()

		ds := latticestore.NewLatticeDataStore()

		mockTGManager := NewMockTargetGroupManager(c)

		sdkTGReturned := []targetGroupOutput{}

		k8sClient := mock_client.NewMockClient(c)

		if len(tt.sdkTargetGroups) > 0 {
			for _, sdkTG := range tt.sdkTargetGroups {
				name := sdkTG.name
				id := sdkTG.id
				vpc := ""
				if sdkTG.isSameVPC {
					vpc = config.VpcID

				} else {
					vpc = config.VpcID + "other VPC"
				}

				var tagsOutput *vpclattice.ListTagsForResourceOutput
				var tags = make(map[string]*string)

				tags["non-k8e"] = aws.String("not-k8s-related")

				if sdkTG.hasTags == false {

					tagsOutput = nil
				} else {

					tagsOutput = &vpclattice.ListTagsForResourceOutput{
						Tags: tags,
					}

				}

				if sdkTG.hasServiceExportTypeTag {
					tags[latticemodel.K8SParentRefTypeKey] = aws.String(latticemodel.K8SServiceExportType)
				}

				if sdkTG.hasHTTPRouteTypeTag {
					tags[latticemodel.K8SParentRefTypeKey] = aws.String(latticemodel.K8SHTTPRouteType)
				}

				tags[latticemodel.K8SServiceNameKey] = aws.String(srvname)
				tags[latticemodel.K8SServiceNamespaceKey] = aws.String(srvnamespace)
				tags[latticemodel.K8SHTTPRouteNameKey] = aws.String(routename)
				tags[latticemodel.K8SHTTPRouteNamespaceKey] = aws.String(routenamespace)

				sdkTGReturned = append(sdkTGReturned,
					targetGroupOutput{
						getTargetGroupOutput: vpclattice.GetTargetGroupOutput{
							Name: &name,
							Id:   &id,
							Arn:  aws.String("tg-ARN"),
							Config: &vpclattice.TargetGroupConfig{
								VpcIdentifier: &vpc,
							}},
						targetGroupTags: tagsOutput,
					},
				)

				fmt.Printf("sdkTGReturned :%v \n", sdkTGReturned[0].targetGroupTags)

				tgSpec := latticemodel.TargetGroup{
					Spec: latticemodel.TargetGroupSpec{
						Name:      sdkTG.name,
						LatticeID: sdkTG.id,
					},
				}

				if sdkTG.HTTPRouteExist {

					if sdkTG.refedByHTTPRoute {
						k8sClient.EXPECT().Get(ctx, gomock.Any(), gomock.Any()).DoAndReturn(
							func(ctx context.Context, name types.NamespacedName, httpRoute *v1alpha2.HTTPRoute, arg3 ...interface{}) error {
								httpRoute.Name = routename
								httpRoute.Namespace = routenamespace

								httpRoute.Spec.Rules = []v1alpha2.HTTPRouteRule{
									{
										BackendRefs: []v1alpha2.HTTPBackendRef{
											{
												BackendRef: v1alpha2.BackendRef{
													BackendObjectReference: v1alpha2.BackendObjectReference{
														Kind: kindPtr("Service"),
														Name: v1alpha2.ObjectName(srvname),
													},
												},
											},
										},
									},
								}

								return nil
							},
						)

					} else {
						k8sClient.EXPECT().Get(ctx, gomock.Any(), gomock.Any()).DoAndReturn(
							func(ctx context.Context, name types.NamespacedName, httpRoute *v1alpha2.HTTPRoute, arg3 ...interface{}) error {
								httpRoute.Name = routename
								httpRoute.Namespace = routenamespace
								return nil

							},
						)

					}
				} else {
					if sdkTG.hasHTTPRouteTypeTag {
						k8sClient.EXPECT().Get(ctx, gomock.Any(), gomock.Any()).DoAndReturn(
							func(ctx context.Context, name types.NamespacedName, httpRoute *v1alpha2.HTTPRoute, arg3 ...interface{}) error {

								return errors.New("no httproute")

							},
						)
					}
				}

				if sdkTG.hasServiceExportTypeTag {
					if sdkTG.serviceExportExist {
						k8sClient.EXPECT().Get(ctx, gomock.Any(), gomock.Any()).DoAndReturn(
							func(ctx context.Context, name types.NamespacedName, svcexport *mcs_api.ServiceExport, arg3 ...interface{}) error {

								return nil

							},
						)

					} else {
						k8sClient.EXPECT().Get(ctx, gomock.Any(), gomock.Any()).DoAndReturn(
							func(ctx context.Context, name types.NamespacedName, svcexport *mcs_api.ServiceExport, arg3 ...interface{}) error {

								return errors.New("no serviceexport")

							},
						)

					}
				}

				if sdkTG.expectDelete {
					mockTGManager.EXPECT().Delete(ctx, &tgSpec).Return(sdkTG.serviceNetworkManagerErr)
				}

			}
		}
		fmt.Printf("sdkTGReturnd %v len %v\n", sdkTGReturned, len(sdkTGReturned))

		mockTGManager.EXPECT().List(ctx).Return(sdkTGReturned, nil)

		tgSynthesizer := NewTargetGroupSynthesizer(nil, k8sClient, mockTGManager, nil, ds)

		err := tgSynthesizer.SynthesizeSDKTargetGroups(ctx)

		assert.Equal(t, tt.wantSynthesizerError, err)
	}
}

type svcDef struct {
	name   string
	tgARN  string
	tgID   string
	mgrErr bool
}

func Test_SynthesizeTriggeredService(t *testing.T) {
	tests := []struct {
		name         string
		svcList      []svcDef
		isDeleted    bool
		wantErrIsNil bool
	}{
		{
			name: "service triggered target group",
			svcList: []svcDef{
				{
					name:   "service11",
					tgARN:  "service11-arn",
					tgID:   "service11-ID",
					mgrErr: false,
				},
				{
					name:   "service12",
					tgARN:  "service12-arn",
					tgID:   "service12-ID",
					mgrErr: false,
				},
			},
			isDeleted:    false,
			wantErrIsNil: true,
		},
		{
			name: "service triggered target group",
			svcList: []svcDef{
				{
					name:   "service21",
					tgARN:  "service21-arn",
					tgID:   "service21-ID",
					mgrErr: true,
				},
				{
					name:   "service22",
					tgARN:  "service22-arn",
					tgID:   "service22-ID",
					mgrErr: false,
				},
			},
			isDeleted:    false,
			wantErrIsNil: false,
		},
		{
			name: "service triggered target group",
			svcList: []svcDef{
				{
					name:   "service31",
					tgARN:  "service31-arn",
					tgID:   "service31-ID",
					mgrErr: true,
				},
				{
					name:   "service32",
					tgARN:  "service32-arn",
					tgID:   "service32-ID",
					mgrErr: false,
				},
			},
			isDeleted:    true,
			wantErrIsNil: false,
		},
	}

	for _, tt := range tests {
		c := gomock.NewController(t)
		defer c.Finish()
		ctx := context.TODO()

		mockTGManager := NewMockTargetGroupManager(c)

		ds := latticestore.NewLatticeDataStore()

		stack := core.NewDefaultStack(core.StackID(types.NamespacedName{Namespace: "tt", Name: "name"}))

		for _, svc := range tt.svcList {
			tgSpec := latticemodel.TargetGroupSpec{
				Name: svc.name,
				Type: latticemodel.TargetGroupTypeIP,
				Config: latticemodel.TargetGroupConfig{
					IsServiceImport: false,
				},
				IsDeleted: tt.isDeleted,
			}

			tg := latticemodel.NewTargetGroup(stack, svc.name, tgSpec)
			fmt.Printf("tg : %v\n", tg)

			if !tt.isDeleted {

				if svc.mgrErr {
					mockTGManager.EXPECT().Create(ctx, tg).Return(latticemodel.TargetGroupStatus{}, errors.New("tgmgr err"))
				} else {
					mockTGManager.EXPECT().Create(ctx, tg).Return(latticemodel.TargetGroupStatus{TargetGroupARN: svc.tgARN, TargetGroupID: svc.tgID}, nil)
				}
			} else {
				if svc.mgrErr {
					mockTGManager.EXPECT().Delete(ctx, tg).Return(errors.New("tgmgr err"))
				} else {
					mockTGManager.EXPECT().Delete(ctx, tg).Return(nil)
				}
			}
		}

		synthesizer := NewTargetGroupSynthesizer(nil, nil, mockTGManager, stack, ds)
		err := synthesizer.SynthesizeTriggeredTargetGroup(ctx)
		fmt.Printf("err:%v \n", err)

		if tt.wantErrIsNil {
			assert.Nil(t, err)
		} else {
			assert.NotNil(t, err)
		}

		if !tt.isDeleted {
			// check datastore
			for _, tg := range tt.svcList {
				if tg.mgrErr {
					_, err := ds.GetTargetGroup(tg.name, false)
					assert.NotNil(t, err)

				} else {
					dsTG, err := ds.GetTargetGroup(tg.name, false)
					assert.Nil(t, err)
					assert.Equal(t, tg.tgARN, dsTG.ARN)
					assert.Equal(t, tg.tgID, dsTG.ID)

				}
			}
		}

	}
}
