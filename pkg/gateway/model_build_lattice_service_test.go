package gateway

import (
	"context"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/gateway-api/apis/v1alpha2"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"

	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

func Test_LatticeServiceModelBuild(t *testing.T) {
	now := metav1.Now()
	var httpSectionName v1alpha2.SectionName = "http"
	var serviceKind v1alpha2.Kind = "Service"
	var serviceimportKind v1alpha2.Kind = "ServiceImport"
	var weight1 = int32(10)
	var weight2 = int32(90)
	var namespace = v1alpha2.Namespace("default")

	var backendRef1 = v1alpha2.BackendRef{
		BackendObjectReference: v1alpha2.BackendObjectReference{
			Name:      "targetgroup1",
			Namespace: &namespace,
			Kind:      &serviceKind,
		},
		Weight: &weight1,
	}
	var backendRef2 = v1alpha2.BackendRef{
		BackendObjectReference: v1alpha2.BackendObjectReference{
			Name:      "targetgroup2",
			Namespace: &namespace,
			Kind:      &serviceimportKind,
		},
		Weight: &weight2,
	}

	tests := []struct {
		name          string
		httpRoute     *v1alpha2.HTTPRoute
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
				},
			},

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
								Name:        "gateway2",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []v1alpha2.HTTPRouteRule{
						{
							BackendRefs: []v1alpha2.HTTPBackendRef{
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
			},

			wantError:     nil,
			wantName:      "service2",
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

			stack := core.NewDefaultStack(core.StackID(k8s.NamespacedName(tt.httpRoute)))

			task := &latticeServiceModelBuildTask{
				httpRoute: tt.httpRoute,
				stack:     stack,
				Client:    k8sClient,
				tgByResID: make(map[string]*latticemodel.TargetGroup),
				Datastore: ds,
			}

			err := task.buildLatticeService(ctx)

			fmt.Printf("task.latticeService.Spec %v, err: %v\n", task.latticeService.Spec, err)

			if tt.wantIsDeleted {
				assert.Equal(t, true, task.latticeService.Spec.IsDeleted)
				// make sure no rules and listener are built
				var resRules []*latticemodel.Rule
				stack.ListResources(&resRules)
				assert.Equal(t, len(resRules), 0)

				var resListener []*latticemodel.Listener
				stack.ListResources(&resListener)
				assert.Equal(t, len(resListener), 0)

			} else {
				assert.Equal(t, false, task.latticeService.Spec.IsDeleted)
				assert.Equal(t, tt.httpRoute.Name, task.latticeService.Spec.Name)
				assert.Equal(t, tt.httpRoute.Namespace, task.latticeService.Spec.Namespace)
			}

			if tt.wantErrIsNil {
				assert.Nil(t, err)

			} else {
				assert.NotNil(t, err)
			}

		})
	}
}
