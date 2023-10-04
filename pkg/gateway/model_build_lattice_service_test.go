package gateway

import (
	"context"
	"fmt"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"

	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func Test_LatticeServiceModelBuild(t *testing.T) {
	now := metav1.Now()
	var httpSectionName gwv1beta1.SectionName = "http"
	var serviceKind gwv1beta1.Kind = "Service"
	var serviceimportKind gwv1beta1.Kind = "ServiceImport"
	var weight1 = int32(10)
	var weight2 = int32(90)
	var namespace = gwv1beta1.Namespace("default")

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

	tests := []struct {
		name          string
		route         core.Route
		wantError     error
		wantErrIsNil  bool
		wantName      string
		wantRouteType core.RouteType
		wantIsDeleted bool
	}{
		{
			name: "Add LatticeService with hostname",
			route: core.NewHTTPRoute(gwv1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name: "service1",
				},
				Spec: gwv1beta1.HTTPRouteSpec{
					CommonRouteSpec: gwv1beta1.CommonRouteSpec{
						ParentRefs: []gwv1beta1.ParentReference{
							{
								Name: "gateway1",
							},
						},
					},
					Hostnames: []gwv1beta1.Hostname{
						"test1.test.com",
						"test2.test.com",
					},
				},
			}),

			wantError:     nil,
			wantName:      "service1",
			wantRouteType: core.HttpRouteType,
			wantIsDeleted: false,
			wantErrIsNil:  true,
		},
		{
			name: "Add LatticeService",
			route: core.NewHTTPRoute(gwv1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name: "service1",
				},
				Spec: gwv1beta1.HTTPRouteSpec{
					CommonRouteSpec: gwv1beta1.CommonRouteSpec{
						ParentRefs: []gwv1beta1.ParentReference{
							{
								Name: "gateway1",
							},
						},
					},
				},
			}),

			wantError:     nil,
			wantName:      "service1",
			wantRouteType: core.HttpRouteType,
			wantIsDeleted: false,
			wantErrIsNil:  true,
		},
		{
			name: "Add LatticeService with GRPCRoute",
			route: core.NewGRPCRoute(gwv1alpha2.GRPCRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name: "service1",
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
			wantError:     nil,
			wantName:      "service1",
			wantRouteType: core.GrpcRouteType,
			wantIsDeleted: false,
			wantErrIsNil:  true,
		},
		{
			name: "Delete LatticeService",
			route: core.NewHTTPRoute(gwv1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "service2",
					Finalizers:        []string{"gateway.k8s.aws/resources"},
					DeletionTimestamp: &now,
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

			wantError:     nil,
			wantName:      "service2",
			wantRouteType: core.HttpRouteType,
			wantIsDeleted: true,
			wantErrIsNil:  true,
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

			stack := core.NewDefaultStack(core.StackID(k8s.NamespacedName(tt.route.K8sObject())))

			task := &latticeServiceModelBuildTask{
				log:       gwlog.FallbackLogger,
				route:     tt.route,
				stack:     stack,
				client:    k8sClient,
				tgByResID: make(map[string]*model.TargetGroup),
				datastore: ds,
			}

			err := task.buildLatticeService(ctx)

			fmt.Printf("task.latticeService.Spec %v, err: %v\n", task.latticeService.Spec, err)

			if tt.wantIsDeleted {
				assert.Equal(t, true, task.latticeService.Spec.IsDeleted)
				// make sure no rules and listener are built
				var resRules []*model.Rule
				stack.ListResources(&resRules)
				assert.Equal(t, len(resRules), 0)

				var resListener []*model.Listener
				stack.ListResources(&resListener)
				assert.Equal(t, len(resListener), 0)

			} else {
				assert.Equal(t, false, task.latticeService.Spec.IsDeleted)
				assert.Equal(t, tt.route.Name(), task.latticeService.Spec.Name)
				assert.Equal(t, tt.route.Namespace(), task.latticeService.Spec.Namespace)

				if len(tt.route.Spec().Hostnames()) > 0 {
					assert.Equal(t, string(tt.route.Spec().Hostnames()[0]), task.latticeService.Spec.CustomerDomainName)
				} else {
					assert.Equal(t, "", task.latticeService.Spec.CustomerDomainName)
				}
			}

			if tt.wantErrIsNil {
				assert.Nil(t, err)

			} else {
				assert.NotNil(t, err)
			}
		})
	}
}
