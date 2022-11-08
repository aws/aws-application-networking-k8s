package gateway

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"k8s.io/apimachinery/pkg/types"

	mock_client "github.com/aws/aws-application-networking-k8s/mocks/controller-runtime/client"

	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"

	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

func Test_RuleModelBuild(t *testing.T) {
	var httpSectionName v1alpha2.SectionName = "http"
	var serviceKind v1alpha2.Kind = "Service"
	var serviceimportKind v1alpha2.Kind = "ServiceImport"
	var weight1 = int32(10)
	var weight2 = int32(90)
	var namespace = v1alpha2.Namespace("default")
	var path1 = string("/ver1")
	var path2 = string("/ver2")
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
	var backendServiceImportRef = v1alpha2.BackendRef{
		BackendObjectReference: v1alpha2.BackendObjectReference{
			Name: "targetgroup1",
			Kind: &serviceimportKind,
		},
	}

	tests := []struct {
		name               string
		gwListenerPort     v1alpha2.PortNumber
		httpRoute          *v1alpha2.HTTPRoute
		wantErrIsNil       bool
		k8sGetGatewayCall  bool
		k8sGatewayReturnOK bool
	}{
		{
			name:               "rule, default service action",
			gwListenerPort:     *PortNumberPtr(80),
			wantErrIsNil:       true,
			k8sGetGatewayCall:  true,
			k8sGatewayReturnOK: true,
			httpRoute: &v1alpha2.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: v1alpha2.HTTPRouteSpec{
					CommonRouteSpec: v1alpha2.CommonRouteSpec{
						ParentRefs: []v1alpha2.ParentRef{
							{
								Name:        "mesh1",
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
							},
						},
					},
				},
			},
		},
		{
			name:               "rule, default serviceimport action",
			gwListenerPort:     *PortNumberPtr(80),
			wantErrIsNil:       true,
			k8sGetGatewayCall:  true,
			k8sGatewayReturnOK: true,
			httpRoute: &v1alpha2.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: v1alpha2.HTTPRouteSpec{
					CommonRouteSpec: v1alpha2.CommonRouteSpec{
						ParentRefs: []v1alpha2.ParentRef{
							{
								Name:        "mesh1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []v1alpha2.HTTPRouteRule{
						{
							BackendRefs: []v1alpha2.HTTPBackendRef{
								{
									BackendRef: backendServiceImportRef,
								},
							},
						},
					},
				},
			},
		},
		{
			name:               "rule, weighted target group",
			gwListenerPort:     *PortNumberPtr(80),
			wantErrIsNil:       true,
			k8sGetGatewayCall:  true,
			k8sGatewayReturnOK: true,
			httpRoute: &v1alpha2.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: v1alpha2.HTTPRouteSpec{
					CommonRouteSpec: v1alpha2.CommonRouteSpec{
						ParentRefs: []v1alpha2.ParentRef{
							{
								Name:        "mesh1",
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
		},
		{
			name:               "rule, path based target group",
			gwListenerPort:     *PortNumberPtr(80),
			wantErrIsNil:       true,
			k8sGetGatewayCall:  true,
			k8sGatewayReturnOK: true,
			httpRoute: &v1alpha2.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: v1alpha2.HTTPRouteSpec{
					CommonRouteSpec: v1alpha2.CommonRouteSpec{
						ParentRefs: []v1alpha2.ParentRef{
							{
								Name:        "mesh1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []v1alpha2.HTTPRouteRule{
						{
							Matches: []v1alpha2.HTTPRouteMatch{
								{

									Path: &v1alpha2.HTTPPathMatch{

										Value: &path1,
									},
								},
							},
							BackendRefs: []v1alpha2.HTTPBackendRef{
								{
									BackendRef: backendRef1,
								},
							},
						},
						{
							Matches: []v1alpha2.HTTPRouteMatch{
								{

									Path: &v1alpha2.HTTPPathMatch{

										Value: &path2,
									},
								},
							},
							BackendRefs: []v1alpha2.HTTPBackendRef{
								{
									BackendRef: backendRef2,
								},
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		c := gomock.NewController(t)
		defer c.Finish()
		ctx := context.TODO()

		k8sClient := mock_client.NewMockClient(c)

		if tt.k8sGetGatewayCall {

			k8sClient.EXPECT().Get(ctx, gomock.Any(), gomock.Any()).DoAndReturn(
				func(ctx context.Context, gwName types.NamespacedName, gw *v1alpha2.Gateway) error {

					if tt.k8sGatewayReturnOK {
						gw.Spec.Listeners = append(gw.Spec.Listeners, v1alpha2.Listener{
							Port: tt.gwListenerPort,
							Name: *tt.httpRoute.Spec.ParentRefs[0].SectionName,
						})
						return nil
					} else {
						return errors.New("unknown k8s object")
					}
				},
			)
		}

		ds := latticestore.NewLatticeDataStore()

		stack := core.NewDefaultStack(core.StackID(k8s.NamespacedName(tt.httpRoute)))

		task := &latticeServiceModelBuildTask{
			httpRoute:       tt.httpRoute,
			stack:           stack,
			Client:          k8sClient,
			listenerByResID: make(map[string]*latticemodel.Listener),
			Datastore:       ds,
		}

		err := task.buildRules(ctx)

		assert.NoError(t, err)

		var resRules []*latticemodel.Rule

		stack.ListResources(&resRules)

		fmt.Printf("resRules :%v \n", *resRules[0])

		var i = 1
		for _, resRule := range resRules {

			ruleID := fmt.Sprintf("rule-%d", i)
			assert.Equal(t, resRule.Spec.RuleID, ruleID)
			assert.Equal(t, resRule.Spec.ListenerPort, int64(tt.gwListenerPort))
			assert.Equal(t, resRule.Spec.RuleType, "HTTPRouteMatch")
			assert.Equal(t, resRule.Spec.ServiceName, tt.httpRoute.Name)
			assert.Equal(t, resRule.Spec.ServiceNamespace, tt.httpRoute.Namespace)

			var j = 0
			for _, tg := range resRule.Spec.Action.TargetGroups {

				assert.Equal(t, v1alpha2.ObjectName(tg.Name), tt.httpRoute.Spec.Rules[i-1].BackendRefs[j].Name)
				if tt.httpRoute.Spec.Rules[i-1].BackendRefs[j].Namespace != nil {
					assert.Equal(t, v1alpha2.Namespace(tg.Namespace), *tt.httpRoute.Spec.Rules[i-1].BackendRefs[j].Namespace)
				}

				if *tt.httpRoute.Spec.Rules[i-1].BackendRefs[j].Kind == "ServiceImport" {
					assert.Equal(t, tg.IsServiceImport, true)
				} else {
					assert.Equal(t, tg.IsServiceImport, false)
				}
				j++
			}
			i++

		}

	}
}
