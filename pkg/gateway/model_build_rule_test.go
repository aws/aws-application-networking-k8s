package gateway

import (
	"context"
	"errors"
	"fmt"
	"k8s.io/utils/pointer"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/aws/aws-sdk-go/service/vpclattice"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"k8s.io/apimachinery/pkg/types"

	mock_client "github.com/aws/aws-application-networking-k8s/mocks/controller-runtime/client"

	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"

	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

func Test_RuleModelBuild(t *testing.T) {
	var httpSectionName v1beta1.SectionName = "http"
	var serviceKind v1beta1.Kind = "Service"
	var serviceimportKind v1beta1.Kind = "ServiceImport"
	var weight1 = int32(10)
	var weight2 = int32(90)
	var namespace = v1beta1.Namespace("testnamespace")
	var namespace2 = v1beta1.Namespace("testnamespace2")
	var path1 = "/ver1"
	var path2 = "/ver2"
	var path3 = "/ver3"
	var k8sPathMatchExactType = v1beta1.PathMatchExact
	var k8sMethodMatchExactType = v1alpha2.GRPCMethodMatchExact
	var backendRef1 = v1beta1.BackendRef{
		BackendObjectReference: v1beta1.BackendObjectReference{
			Name: "targetgroup1",
			Kind: &serviceKind,
		},
		Weight: &weight1,
	}
	var backendRef2 = v1beta1.BackendRef{
		BackendObjectReference: v1beta1.BackendObjectReference{
			Name: "targetgroup2",
			Kind: &serviceimportKind,
		},
		Weight: &weight2,
	}
	var backendRef1Namespace1 = v1beta1.BackendRef{
		BackendObjectReference: v1beta1.BackendObjectReference{
			Name:      "targetgroup2",
			Namespace: &namespace,
			Kind:      &serviceimportKind,
		},
		Weight: &weight2,
	}
	var backendRef1Namespace2 = v1beta1.BackendRef{
		BackendObjectReference: v1beta1.BackendObjectReference{
			Name:      "targetgroup2",
			Namespace: &namespace2,
			Kind:      &serviceimportKind,
		},
		Weight: &weight2,
	}
	var backendServiceImportRef = v1beta1.BackendRef{
		BackendObjectReference: v1beta1.BackendObjectReference{
			Name: "targetgroup1",
			Kind: &serviceimportKind,
		},
	}

	tests := []struct {
		name               string
		gwListenerPort     v1beta1.PortNumber
		route              core.Route
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
			route: core.NewHTTPRoute(v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: v1beta1.HTTPRouteSpec{
					CommonRouteSpec: v1beta1.CommonRouteSpec{
						ParentRefs: []v1beta1.ParentReference{
							{
								Name:        "mesh1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []v1beta1.HTTPRouteRule{
						{
							BackendRefs: []v1beta1.HTTPBackendRef{
								{
									BackendRef: backendRef1,
								},
							},
						},
					},
				},
			}),
		},
		{
			name:               "rule, default serviceimport action",
			gwListenerPort:     *PortNumberPtr(80),
			wantErrIsNil:       true,
			k8sGetGatewayCall:  true,
			k8sGatewayReturnOK: true,
			route: core.NewHTTPRoute(v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: v1beta1.HTTPRouteSpec{
					CommonRouteSpec: v1beta1.CommonRouteSpec{
						ParentRefs: []v1beta1.ParentReference{
							{
								Name:        "mesh1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []v1beta1.HTTPRouteRule{
						{
							BackendRefs: []v1beta1.HTTPBackendRef{
								{
									BackendRef: backendServiceImportRef,
								},
							},
						},
					},
				},
			}),
		},
		{
			name:               "rule, weighted target group",
			gwListenerPort:     *PortNumberPtr(80),
			wantErrIsNil:       true,
			k8sGetGatewayCall:  true,
			k8sGatewayReturnOK: true,
			route: core.NewHTTPRoute(v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: v1beta1.HTTPRouteSpec{
					CommonRouteSpec: v1beta1.CommonRouteSpec{
						ParentRefs: []v1beta1.ParentReference{
							{
								Name:        "mesh1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []v1beta1.HTTPRouteRule{
						{
							BackendRefs: []v1beta1.HTTPBackendRef{
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
		},
		{
			name:               "rule, path based target group",
			gwListenerPort:     *PortNumberPtr(80),
			wantErrIsNil:       true,
			k8sGetGatewayCall:  true,
			k8sGatewayReturnOK: true,
			route: core.NewHTTPRoute(v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: v1beta1.HTTPRouteSpec{
					CommonRouteSpec: v1beta1.CommonRouteSpec{
						ParentRefs: []v1beta1.ParentReference{
							{
								Name:        "mesh1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []v1beta1.HTTPRouteRule{
						{
							Matches: []v1beta1.HTTPRouteMatch{
								{

									Path: &v1beta1.HTTPPathMatch{
										Type:  &k8sPathMatchExactType,
										Value: &path1,
									},
								},
							},
							BackendRefs: []v1beta1.HTTPBackendRef{
								{
									BackendRef: backendRef1,
								},
							},
						},
						{
							Matches: []v1beta1.HTTPRouteMatch{
								{

									Path: &v1beta1.HTTPPathMatch{

										Value: &path2,
									},
								},
							},
							BackendRefs: []v1beta1.HTTPBackendRef{
								{
									BackendRef: backendRef2,
								},
							},
						},
					},
				},
			}),
		},
		{
			name:               "rule, different namespace combination",
			gwListenerPort:     *PortNumberPtr(80),
			wantErrIsNil:       true,
			k8sGetGatewayCall:  true,
			k8sGatewayReturnOK: true,
			route: core.NewHTTPRoute(v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "non-default",
				},
				Spec: v1beta1.HTTPRouteSpec{
					CommonRouteSpec: v1beta1.CommonRouteSpec{
						ParentRefs: []v1beta1.ParentReference{
							{
								Name:        "mesh1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []v1beta1.HTTPRouteRule{
						{
							Matches: []v1beta1.HTTPRouteMatch{
								{
									Path: &v1beta1.HTTPPathMatch{
										Value: &path1,
									},
								},
							},
							BackendRefs: []v1beta1.HTTPBackendRef{
								{
									BackendRef: backendRef1,
								},
							},
						},
						{
							Matches: []v1beta1.HTTPRouteMatch{
								{
									Path: &v1beta1.HTTPPathMatch{
										Value: &path2,
									},
								},
							},
							BackendRefs: []v1beta1.HTTPBackendRef{
								{
									BackendRef: backendRef1Namespace1,
								},
							},
						},
						{
							Matches: []v1beta1.HTTPRouteMatch{
								{
									Path: &v1beta1.HTTPPathMatch{
										Value: &path3,
									},
								},
							},
							BackendRefs: []v1beta1.HTTPBackendRef{
								{
									BackendRef: backendRef1Namespace2,
								},
							},
						},
					},
				},
			}),
		},
		{
			name:               "rule, default service import action for GRPCRoute",
			gwListenerPort:     *PortNumberPtr(80),
			wantErrIsNil:       true,
			k8sGetGatewayCall:  true,
			k8sGatewayReturnOK: true,
			route: core.NewGRPCRoute(v1alpha2.GRPCRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: v1alpha2.GRPCRouteSpec{
					CommonRouteSpec: v1alpha2.CommonRouteSpec{
						ParentRefs: []v1alpha2.ParentReference{
							{
								Name:        "mesh1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []v1alpha2.GRPCRouteRule{
						{
							BackendRefs: []v1alpha2.GRPCBackendRef{
								{
									BackendRef: backendServiceImportRef,
								},
							},
						},
					},
				},
			}),
		},
		{
			name:               "rule, gRPC routes with methods and multiple namespaces",
			gwListenerPort:     *PortNumberPtr(80),
			wantErrIsNil:       true,
			k8sGetGatewayCall:  true,
			k8sGatewayReturnOK: true,
			route: core.NewGRPCRoute(v1alpha2.GRPCRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "non-default",
				},
				Spec: v1alpha2.GRPCRouteSpec{
					CommonRouteSpec: v1beta1.CommonRouteSpec{
						ParentRefs: []v1beta1.ParentReference{
							{
								Name:        "mesh1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []v1alpha2.GRPCRouteRule{
						{
							Matches: []v1alpha2.GRPCRouteMatch{
								{
									Method: &v1alpha2.GRPCMethodMatch{
										Type:    &k8sMethodMatchExactType,
										Service: pointer.String("service"),
										Method:  pointer.String("method1"),
									},
								},
							},
							BackendRefs: []v1alpha2.GRPCBackendRef{
								{
									BackendRef: backendRef1,
								},
							},
						},
						{
							Matches: []v1alpha2.GRPCRouteMatch{
								{
									Method: &v1alpha2.GRPCMethodMatch{
										Type:    &k8sMethodMatchExactType,
										Service: pointer.String("service"),
										Method:  pointer.String("method2"),
									},
								},
							},
							BackendRefs: []v1alpha2.GRPCBackendRef{
								{
									BackendRef: backendRef1Namespace1,
								},
							},
						},
						{
							Matches: []v1alpha2.GRPCRouteMatch{
								{
									Method: &v1alpha2.GRPCMethodMatch{
										Type:    &k8sMethodMatchExactType,
										Service: pointer.String("service"),
										Method:  pointer.String("method3"),
									},
								},
							},
							BackendRefs: []v1alpha2.GRPCBackendRef{
								{
									BackendRef: backendRef1Namespace2,
								},
							},
						},
					},
				},
			}),
		},
	}
	for _, tt := range tests {
		fmt.Printf("Testing >>> %v\n", tt.name)
		c := gomock.NewController(t)
		defer c.Finish()
		ctx := context.TODO()

		k8sClient := mock_client.NewMockClient(c)

		if tt.k8sGetGatewayCall {

			k8sClient.EXPECT().Get(ctx, gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
				func(ctx context.Context, gwName types.NamespacedName, gw *v1beta1.Gateway, arg3 ...interface{}) error {

					if tt.k8sGatewayReturnOK {
						gw.Spec.Listeners = append(gw.Spec.Listeners, v1beta1.Listener{
							Port: tt.gwListenerPort,
							Name: *tt.route.Spec().ParentRefs()[0].SectionName,
						})
						return nil
					} else {
						return errors.New("unknown k8s object")
					}
				},
			)
		}

		ds := latticestore.NewLatticeDataStore()

		stack := core.NewDefaultStack(core.StackID(k8s.NamespacedName(tt.route.K8sObject())))

		task := &latticeServiceModelBuildTask{
			route:           tt.route,
			stack:           stack,
			Client:          k8sClient,
			listenerByResID: make(map[string]*latticemodel.Listener),
			Datastore:       ds,
		}

		err := task.buildRules(ctx)

		assert.NoError(t, err)

		var resRules []*latticemodel.Rule

		stack.ListResources(&resRules)

		if len(resRules) > 0 {
			fmt.Printf("resRules :%v \n", *resRules[0])
		}

		var i = 1
		for _, resRule := range resRules {

			fmt.Sscanf(resRule.Spec.RuleID, "rule-%d", &i)

			assert.Equal(t, resRule.Spec.ListenerPort, int64(tt.gwListenerPort))
			// Defer this to dedicate rule check			assert.Equal(t, resRule.Spec.PathMatchValue, tt.route.)
			assert.Equal(t, resRule.Spec.ServiceName, tt.route.Name())
			assert.Equal(t, resRule.Spec.ServiceNamespace, tt.route.Namespace())

			var j = 0
			for _, tg := range resRule.Spec.Action.TargetGroups {

				assert.Equal(t, v1beta1.ObjectName(tg.Name), tt.route.Spec().Rules()[i-1].BackendRefs()[j].Name())
				if tt.route.Spec().Rules()[i-1].BackendRefs()[j].Namespace() != nil {
					assert.Equal(t, v1beta1.Namespace(tg.Namespace), *tt.route.Spec().Rules()[i-1].BackendRefs()[j].Namespace())
				} else {
					assert.Equal(t, tg.Namespace, tt.route.Namespace())
				}

				if *tt.route.Spec().Rules()[i-1].BackendRefs()[j].Kind() == "ServiceImport" {
					assert.Equal(t, tg.IsServiceImport, true)
				} else {
					assert.Equal(t, tg.IsServiceImport, false)
				}
				j++
			}

		}

	}
}

func Test_HeadersRuleBuild(t *testing.T) {
	var httpSectionName v1beta1.SectionName = "http"
	var serviceKind v1beta1.Kind = "Service"

	var namespace = v1beta1.Namespace("default")
	var path1 = "/ver1"
	//var path2 = string("/ver2")
	var k8sPathMatchExactType = v1beta1.PathMatchExact
	var k8sPathMatchPrefixType = v1beta1.PathMatchPathPrefix
	var k8sMethod = v1beta1.HTTPMethodGet
	var k8sMethodMatchExactType = v1alpha2.GRPCMethodMatchExact

	var k8sHeaderExactType = v1beta1.HeaderMatchExact
	var hdr1 = "env1"
	var hdr1Value = "test1"
	var hdr2 = "env2"
	var hdr2Value = "test2"

	var backendRef1 = v1beta1.BackendRef{
		BackendObjectReference: v1beta1.BackendObjectReference{
			Name:      "targetgroup1",
			Namespace: &namespace,
			Kind:      &serviceKind,
		},
	}

	tests := []struct {
		name             string
		gwListenerPort   v1beta1.PortNumber
		route            core.Route
		expectedRuleSpec latticemodel.RuleSpec
		wantErrIsNil     bool
		samerule         bool
	}{
		{
			name:           "PathMatchExact Match",
			gwListenerPort: *PortNumberPtr(80),
			wantErrIsNil:   false,
			samerule:       true,

			route: core.NewHTTPRoute(v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: v1beta1.HTTPRouteSpec{
					CommonRouteSpec: v1beta1.CommonRouteSpec{
						ParentRefs: []v1beta1.ParentReference{
							{
								Name:        "mesh1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []v1beta1.HTTPRouteRule{
						{
							Matches: []v1beta1.HTTPRouteMatch{
								{

									Path: &v1beta1.HTTPPathMatch{
										Type:  &k8sPathMatchExactType,
										Value: &path1,
									},
								},
							},
							BackendRefs: []v1beta1.HTTPBackendRef{
								{
									BackendRef: backendRef1,
								},
							},
						},
					},
				},
			}),
			expectedRuleSpec: latticemodel.RuleSpec{
				PathMatchExact: true,
				PathMatchValue: path1,
			},
		},

		{
			name:           "PathMatchPrefix",
			gwListenerPort: *PortNumberPtr(80),
			wantErrIsNil:   false,
			samerule:       true,

			route: core.NewHTTPRoute(v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: v1beta1.HTTPRouteSpec{
					CommonRouteSpec: v1beta1.CommonRouteSpec{
						ParentRefs: []v1beta1.ParentReference{
							{
								Name:        "mesh1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []v1beta1.HTTPRouteRule{
						{
							Matches: []v1beta1.HTTPRouteMatch{
								{

									Path: &v1beta1.HTTPPathMatch{
										Type:  &k8sPathMatchPrefixType,
										Value: &path1,
									},
								},
							},
							BackendRefs: []v1beta1.HTTPBackendRef{
								{
									BackendRef: backendRef1,
								},
							},
						},
					},
				},
			}),
			expectedRuleSpec: latticemodel.RuleSpec{
				PathMatchPrefix: true,
				PathMatchValue:  path1,
			},
		},
		{
			name:           "1 header match",
			gwListenerPort: *PortNumberPtr(80),
			wantErrIsNil:   false,
			samerule:       true,

			route: core.NewHTTPRoute(v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: v1beta1.HTTPRouteSpec{
					CommonRouteSpec: v1beta1.CommonRouteSpec{
						ParentRefs: []v1beta1.ParentReference{
							{
								Name:        "mesh1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []v1beta1.HTTPRouteRule{
						{
							Matches: []v1beta1.HTTPRouteMatch{
								{

									//	Path: &v1beta1.HTTPPathMatch{
									//		Type:  &k8sPathMatchPrefixType,
									//		Value: &path1,
									//	},
									Headers: []v1beta1.HTTPHeaderMatch{
										{
											Type:  &k8sHeaderExactType,
											Name:  v1beta1.HTTPHeaderName(hdr1),
											Value: hdr1Value,
										},
									},
								},
							},
							BackendRefs: []v1beta1.HTTPBackendRef{
								{
									BackendRef: backendRef1,
								},
							},
						},
					},
				},
			}),
			expectedRuleSpec: latticemodel.RuleSpec{
				NumOfHeaderMatches: 1,
				MatchedHeaders: [5]vpclattice.HeaderMatch{

					{
						Match: &vpclattice.HeaderMatchType{
							Exact: &hdr1Value},
						Name: &hdr1,
					},

					{},
					{},
					{},
					{},
				},
			},
		},
		{
			name:           "2 header match",
			gwListenerPort: *PortNumberPtr(80),
			wantErrIsNil:   false,
			samerule:       true,

			route: core.NewHTTPRoute(v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: v1beta1.HTTPRouteSpec{
					CommonRouteSpec: v1beta1.CommonRouteSpec{
						ParentRefs: []v1beta1.ParentReference{
							{
								Name:        "mesh1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []v1beta1.HTTPRouteRule{
						{
							Matches: []v1beta1.HTTPRouteMatch{
								{
									Headers: []v1beta1.HTTPHeaderMatch{
										{
											Type:  &k8sHeaderExactType,
											Name:  v1beta1.HTTPHeaderName(hdr1),
											Value: hdr1Value,
										},
										{
											Type:  &k8sHeaderExactType,
											Name:  v1beta1.HTTPHeaderName(hdr2),
											Value: hdr2Value,
										},
									},
								},
							},
							BackendRefs: []v1beta1.HTTPBackendRef{
								{
									BackendRef: backendRef1,
								},
							},
						},
					},
				},
			}),
			expectedRuleSpec: latticemodel.RuleSpec{
				NumOfHeaderMatches: 2,
				MatchedHeaders: [5]vpclattice.HeaderMatch{

					{
						Match: &vpclattice.HeaderMatchType{
							Exact: &hdr1Value},
						Name: &hdr1,
					},
					{
						Match: &vpclattice.HeaderMatchType{
							Exact: &hdr2Value},
						Name: &hdr2,
					},

					{},
					{},
					{},
				},
			},
		},
		{
			name:           "2 header match , path exact",
			gwListenerPort: *PortNumberPtr(80),
			wantErrIsNil:   false,
			samerule:       true,

			route: core.NewHTTPRoute(v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: v1beta1.HTTPRouteSpec{
					CommonRouteSpec: v1beta1.CommonRouteSpec{
						ParentRefs: []v1beta1.ParentReference{
							{
								Name:        "mesh1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []v1beta1.HTTPRouteRule{
						{
							Matches: []v1beta1.HTTPRouteMatch{
								{

									Path: &v1beta1.HTTPPathMatch{
										Type:  &k8sPathMatchExactType,
										Value: &path1,
									},
									Headers: []v1beta1.HTTPHeaderMatch{
										{
											Type:  &k8sHeaderExactType,
											Name:  v1beta1.HTTPHeaderName(hdr1),
											Value: hdr1Value,
										},
										{
											Type:  &k8sHeaderExactType,
											Name:  v1beta1.HTTPHeaderName(hdr2),
											Value: hdr2Value,
										},
									},
								},
							},
							BackendRefs: []v1beta1.HTTPBackendRef{
								{
									BackendRef: backendRef1,
								},
							},
						},
					},
				},
			}),
			expectedRuleSpec: latticemodel.RuleSpec{
				PathMatchExact:     true,
				PathMatchValue:     path1,
				NumOfHeaderMatches: 2,
				MatchedHeaders: [5]vpclattice.HeaderMatch{

					{
						Match: &vpclattice.HeaderMatchType{
							Exact: &hdr1Value},
						Name: &hdr1,
					},
					{
						Match: &vpclattice.HeaderMatchType{
							Exact: &hdr2Value},
						Name: &hdr2,
					},

					{},
					{},
					{},
				},
			},
		},
		{
			name:           "2 header match , path prefix",
			gwListenerPort: *PortNumberPtr(80),
			wantErrIsNil:   false,
			samerule:       true,

			route: core.NewHTTPRoute(v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: v1beta1.HTTPRouteSpec{
					CommonRouteSpec: v1beta1.CommonRouteSpec{
						ParentRefs: []v1beta1.ParentReference{
							{
								Name:        "mesh1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []v1beta1.HTTPRouteRule{
						{
							Matches: []v1beta1.HTTPRouteMatch{
								{

									Path: &v1beta1.HTTPPathMatch{
										Type:  &k8sPathMatchPrefixType,
										Value: &path1,
									},
									Headers: []v1beta1.HTTPHeaderMatch{
										{
											Type:  &k8sHeaderExactType,
											Name:  v1beta1.HTTPHeaderName(hdr1),
											Value: hdr1Value,
										},
										{
											Type:  &k8sHeaderExactType,
											Name:  v1beta1.HTTPHeaderName(hdr2),
											Value: hdr2Value,
										},
									},
								},
							},
							BackendRefs: []v1beta1.HTTPBackendRef{
								{
									BackendRef: backendRef1,
								},
							},
						},
					},
				},
			}),
			expectedRuleSpec: latticemodel.RuleSpec{
				PathMatchPrefix:    true,
				PathMatchValue:     path1,
				NumOfHeaderMatches: 2,
				MatchedHeaders: [5]vpclattice.HeaderMatch{

					{
						Match: &vpclattice.HeaderMatchType{
							Exact: &hdr1Value},
						Name: &hdr1,
					},
					{
						Match: &vpclattice.HeaderMatchType{
							Exact: &hdr2Value},
						Name: &hdr2,
					},

					{},
					{},
					{},
				},
			},
		},
		{
			name:           "2 header match",
			gwListenerPort: *PortNumberPtr(80),
			wantErrIsNil:   false,
			samerule:       true,

			route: core.NewHTTPRoute(v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: v1beta1.HTTPRouteSpec{
					CommonRouteSpec: v1beta1.CommonRouteSpec{
						ParentRefs: []v1beta1.ParentReference{
							{
								Name:        "mesh1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []v1beta1.HTTPRouteRule{
						{
							Matches: []v1beta1.HTTPRouteMatch{
								{
									Headers: []v1beta1.HTTPHeaderMatch{
										{
											Type:  &k8sHeaderExactType,
											Name:  v1beta1.HTTPHeaderName(hdr1),
											Value: hdr1Value,
										},
										{
											Type:  &k8sHeaderExactType,
											Name:  v1beta1.HTTPHeaderName(hdr2),
											Value: hdr2Value,
										},
									},
								},
							},
							BackendRefs: []v1beta1.HTTPBackendRef{
								{
									BackendRef: backendRef1,
								},
							},
						},
					},
				},
			}),
			expectedRuleSpec: latticemodel.RuleSpec{
				NumOfHeaderMatches: 2,
				MatchedHeaders: [5]vpclattice.HeaderMatch{

					{
						Match: &vpclattice.HeaderMatchType{
							Exact: &hdr1Value},
						Name: &hdr1,
					},
					{
						Match: &vpclattice.HeaderMatchType{
							Exact: &hdr2Value},
						Name: &hdr2,
					},

					{},
					{},
					{},
				},
			},
		},
		{
			name:           " negative 6 header match ",
			gwListenerPort: *PortNumberPtr(80),
			wantErrIsNil:   true,
			samerule:       true,

			route: core.NewHTTPRoute(v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: v1beta1.HTTPRouteSpec{
					CommonRouteSpec: v1beta1.CommonRouteSpec{
						ParentRefs: []v1beta1.ParentReference{
							{
								Name:        "mesh1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []v1beta1.HTTPRouteRule{
						{
							Matches: []v1beta1.HTTPRouteMatch{
								{

									Path: &v1beta1.HTTPPathMatch{
										Type:  &k8sPathMatchExactType,
										Value: &path1,
									},
									Headers: []v1beta1.HTTPHeaderMatch{
										{
											Type:  &k8sHeaderExactType,
											Name:  v1beta1.HTTPHeaderName(hdr1),
											Value: hdr1Value,
										},
										{
											Type:  &k8sHeaderExactType,
											Name:  v1beta1.HTTPHeaderName(hdr2),
											Value: hdr2Value,
										},
										{
											Type:  &k8sHeaderExactType,
											Name:  v1beta1.HTTPHeaderName(hdr1),
											Value: hdr1Value,
										},
										{
											Type:  &k8sHeaderExactType,
											Name:  v1beta1.HTTPHeaderName(hdr2),
											Value: hdr2Value,
										},
										{
											Type:  &k8sHeaderExactType,
											Name:  v1beta1.HTTPHeaderName(hdr1),
											Value: hdr1Value,
										},
										{
											Type:  &k8sHeaderExactType,
											Name:  v1beta1.HTTPHeaderName(hdr2),
											Value: hdr2Value,
										},
									},
								},
							},
							BackendRefs: []v1beta1.HTTPBackendRef{
								{
									BackendRef: backendRef1,
								},
							},
						},
					},
				},
			}),
			expectedRuleSpec: latticemodel.RuleSpec{
				PathMatchExact:     true,
				PathMatchValue:     path1,
				NumOfHeaderMatches: 2,
				MatchedHeaders: [5]vpclattice.HeaderMatch{

					{
						Match: &vpclattice.HeaderMatchType{
							Exact: &hdr1Value},
						Name: &hdr1,
					},
					{
						Match: &vpclattice.HeaderMatchType{
							Exact: &hdr2Value},
						Name: &hdr2,
					},

					{},
					{},
					{},
				},
			},
		},
		{
			name:           "Negative, multiple methods",
			gwListenerPort: *PortNumberPtr(80),
			wantErrIsNil:   true,
			samerule:       true,

			route: core.NewHTTPRoute(v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: v1beta1.HTTPRouteSpec{
					CommonRouteSpec: v1beta1.CommonRouteSpec{
						ParentRefs: []v1beta1.ParentReference{
							{
								Name:        "mesh1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []v1beta1.HTTPRouteRule{
						{
							Matches: []v1beta1.HTTPRouteMatch{
								{

									Path: &v1beta1.HTTPPathMatch{
										Type:  &k8sPathMatchExactType,
										Value: &path1,
									},
								},
								{

									Path: &v1beta1.HTTPPathMatch{
										Type:  &k8sPathMatchExactType,
										Value: &path1,
									},
								},
							},
							BackendRefs: []v1beta1.HTTPBackendRef{
								{
									BackendRef: backendRef1,
								},
							},
						},
					},
				},
			}),
			expectedRuleSpec: latticemodel.RuleSpec{
				PathMatchExact: true,
				PathMatchValue: path1,
			},
		},
		{
			name:           "Negative, reject method based",
			gwListenerPort: *PortNumberPtr(80),
			wantErrIsNil:   true,
			samerule:       true,

			route: core.NewHTTPRoute(v1beta1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: v1beta1.HTTPRouteSpec{
					CommonRouteSpec: v1beta1.CommonRouteSpec{
						ParentRefs: []v1beta1.ParentReference{
							{
								Name:        "mesh1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []v1beta1.HTTPRouteRule{
						{
							Matches: []v1beta1.HTTPRouteMatch{
								{

									Method: &k8sMethod,
								},
							},

							BackendRefs: []v1beta1.HTTPBackendRef{
								{
									BackendRef: backendRef1,
								},
							},
						},
					},
				},
			}),
			expectedRuleSpec: latticemodel.RuleSpec{
				PathMatchExact: true,
				PathMatchValue: path1,
			},
		},
		{
			name:           "GRPC match on service and method",
			gwListenerPort: *PortNumberPtr(80),
			wantErrIsNil:   false,
			samerule:       true,

			route: core.NewGRPCRoute(v1alpha2.GRPCRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: v1alpha2.GRPCRouteSpec{
					CommonRouteSpec: v1beta1.CommonRouteSpec{
						ParentRefs: []v1beta1.ParentReference{
							{
								Name:        "mesh1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []v1alpha2.GRPCRouteRule{
						{
							Matches: []v1alpha2.GRPCRouteMatch{
								{
									Method: &v1alpha2.GRPCMethodMatch{
										Type:    &k8sMethodMatchExactType,
										Service: pointer.String("service"),
										Method:  pointer.String("method"),
									},
								},
							},

							BackendRefs: []v1alpha2.GRPCBackendRef{
								{
									BackendRef: backendRef1,
								},
							},
						},
					},
				},
			}),
			expectedRuleSpec: latticemodel.RuleSpec{
				Method:             "POST",
				NumOfHeaderMatches: 0,
				PathMatchExact:     true,
				PathMatchValue:     "/service/method",
			},
		},
		{
			name:           "GRPC match on service",
			gwListenerPort: *PortNumberPtr(80),
			wantErrIsNil:   false,
			samerule:       true,

			route: core.NewGRPCRoute(v1alpha2.GRPCRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: v1alpha2.GRPCRouteSpec{
					CommonRouteSpec: v1beta1.CommonRouteSpec{
						ParentRefs: []v1beta1.ParentReference{
							{
								Name:        "mesh1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []v1alpha2.GRPCRouteRule{
						{
							Matches: []v1alpha2.GRPCRouteMatch{
								{
									Method: &v1alpha2.GRPCMethodMatch{
										Type:    &k8sMethodMatchExactType,
										Service: pointer.String("service"),
									},
								},
							},

							BackendRefs: []v1alpha2.GRPCBackendRef{
								{
									BackendRef: backendRef1,
								},
							},
						},
					},
				},
			}),
			expectedRuleSpec: latticemodel.RuleSpec{
				Method:             "POST",
				NumOfHeaderMatches: 0,
				PathMatchPrefix:    true,
				PathMatchValue:     "/service/",
			},
		},
		{
			name:           "GRPC match on all",
			gwListenerPort: *PortNumberPtr(80),
			wantErrIsNil:   false,
			samerule:       true,

			route: core.NewGRPCRoute(v1alpha2.GRPCRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: v1alpha2.GRPCRouteSpec{
					CommonRouteSpec: v1beta1.CommonRouteSpec{
						ParentRefs: []v1beta1.ParentReference{
							{
								Name:        "mesh1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []v1alpha2.GRPCRouteRule{
						{
							Matches: []v1alpha2.GRPCRouteMatch{
								{
									Method: &v1alpha2.GRPCMethodMatch{
										Type: &k8sMethodMatchExactType,
									},
								},
							},

							BackendRefs: []v1alpha2.GRPCBackendRef{
								{
									BackendRef: backendRef1,
								},
							},
						},
					},
				},
			}),
			expectedRuleSpec: latticemodel.RuleSpec{
				Method:             "POST",
				NumOfHeaderMatches: 0,
				PathMatchPrefix:    true,
				PathMatchValue:     "/",
			},
		},
		{
			name:           "GRPC match with 5 headers",
			gwListenerPort: *PortNumberPtr(80),
			wantErrIsNil:   false,
			samerule:       true,

			route: core.NewGRPCRoute(v1alpha2.GRPCRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: v1alpha2.GRPCRouteSpec{
					CommonRouteSpec: v1beta1.CommonRouteSpec{
						ParentRefs: []v1beta1.ParentReference{
							{
								Name:        "mesh1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []v1alpha2.GRPCRouteRule{
						{
							Matches: []v1alpha2.GRPCRouteMatch{
								{
									Method: &v1alpha2.GRPCMethodMatch{
										Type:    &k8sMethodMatchExactType,
										Service: pointer.String("service"),
									},
									Headers: []v1alpha2.GRPCHeaderMatch{
										{
											Name:  "foo1",
											Value: "bar1",
											Type:  &k8sHeaderExactType,
										},
										{
											Name:  "foo2",
											Value: "bar2",
											Type:  &k8sHeaderExactType,
										},
										{
											Name:  "foo3",
											Value: "bar3",
											Type:  &k8sHeaderExactType,
										},
										{
											Name:  "foo4",
											Value: "bar4",
											Type:  &k8sHeaderExactType,
										},
										{
											Name:  "foo5",
											Value: "bar5",
											Type:  &k8sHeaderExactType,
										},
									},
								},
							},
							BackendRefs: []v1alpha2.GRPCBackendRef{
								{
									BackendRef: backendRef1,
								},
							},
						},
					},
				},
			}),
			expectedRuleSpec: latticemodel.RuleSpec{
				Method:             "POST",
				NumOfHeaderMatches: 5,
				PathMatchPrefix:    true,
				PathMatchValue:     "/service/",
				MatchedHeaders: [latticemodel.MAX_NUM_OF_MATCHED_HEADERS]vpclattice.HeaderMatch{
					{
						Name: pointer.String("foo1"),
						Match: &vpclattice.HeaderMatchType{
							Exact: pointer.String("bar1"),
						},
					},
					{
						Name: pointer.String("foo2"),
						Match: &vpclattice.HeaderMatchType{
							Exact: pointer.String("bar2"),
						},
					},
					{
						Name: pointer.String("foo3"),
						Match: &vpclattice.HeaderMatchType{
							Exact: pointer.String("bar3"),
						},
					},
					{
						Name: pointer.String("foo4"),
						Match: &vpclattice.HeaderMatchType{
							Exact: pointer.String("bar4"),
						},
					},
					{
						Name: pointer.String("foo5"),
						Match: &vpclattice.HeaderMatchType{
							Exact: pointer.String("bar5"),
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {

		fmt.Printf("Testing >>> %v\n", tt.name)
		c := gomock.NewController(t)
		defer c.Finish()
		ctx := context.TODO()

		k8sClient := mock_client.NewMockClient(c)

		k8sClient.EXPECT().Get(ctx, gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
			func(ctx context.Context, gwName types.NamespacedName, gw *v1beta1.Gateway, arg3 ...interface{}) error {

				gw.Spec.Listeners = append(gw.Spec.Listeners, v1beta1.Listener{
					Port: tt.gwListenerPort,
					Name: *tt.route.Spec().ParentRefs()[0].SectionName,
				})
				return nil

			},
		)

		ds := latticestore.NewLatticeDataStore()

		stack := core.NewDefaultStack(core.StackID(k8s.NamespacedName(tt.route.K8sObject())))

		task := &latticeServiceModelBuildTask{
			route:           tt.route,
			stack:           stack,
			Client:          k8sClient,
			listenerByResID: make(map[string]*latticemodel.Listener),
			Datastore:       ds,
		}

		err := task.buildRules(ctx)

		if tt.wantErrIsNil {
			assert.Error(t, err)
			continue
		}
		assert.NoError(t, err)

		var resRules []*latticemodel.Rule
		stack.ListResources(&resRules)

		if len(resRules) > 0 {
			// for debug fmt.Printf("resRules :%v \n", *resRules[0])
		}

		// we are unit- testing various combination of one rule for now
		var i = 1
		for _, resRule := range resRules {

			// for debugging, fmt.Printf("i = %d resRule :%v \n, expected rule: %v\n", i, resRule, tt.expectedRuleSpec)

			sameRule := isRuleSpecSame(&tt.expectedRuleSpec, &resRule.Spec)

			if tt.samerule {
				assert.True(t, sameRule)
			} else {
				assert.False(t, sameRule)
			}
			i++

		}

	}
}

func isRuleSpecSame(rule1 *latticemodel.RuleSpec, rule2 *latticemodel.RuleSpec) bool {

	// debug fmt.Printf("rule1 :%v \n", rule1)
	// debug fmt.Printf("rule2: %v \n", rule2)
	// Path Exact Match
	if rule1.PathMatchExact || rule2.PathMatchExact {
		if rule1.PathMatchExact != rule2.PathMatchExact {
			return false
		}

		if rule1.PathMatchValue != rule2.PathMatchValue {
			return false
		}
	}

	// Path Prefix Match
	if rule1.PathMatchPrefix || rule2.PathMatchPrefix {
		if rule1.PathMatchPrefix != rule2.PathMatchPrefix {
			return false
		}
		if rule1.PathMatchValue != rule2.PathMatchValue {
			return false
		}
	}

	// Header Match
	if rule1.NumOfHeaderMatches != rule2.NumOfHeaderMatches {
		return false
	}

	// Verify each header
	for i := 0; i < rule1.NumOfHeaderMatches; i++ {
		// verify rule1 header is in rule2
		rule1Hdr := rule1.MatchedHeaders[i]

		found := false
		for j := 0; j < rule2.NumOfHeaderMatches; j++ {

			// fmt.Printf("rule1 match :%v\n", rule1Hdr)
			rule2Hdr := rule2.MatchedHeaders[j]
			// fmt.Printf("rule2 match: %v\n", rule2Hdr)

			if *rule1Hdr.Name == *rule2Hdr.Name {
				if rule1Hdr.Match.Exact != nil && rule2Hdr.Match.Exact != nil {
					if *rule1Hdr.Match.Exact == *rule2Hdr.Match.Exact {
						found = true
						break
					}
				} else if rule1Hdr.Match.Prefix != nil && rule2Hdr.Match.Prefix != nil {
					if *rule1Hdr.Match.Prefix == *rule2Hdr.Match.Prefix {
						found = true
						break
					}
				}
			}

		}
		if !found {
			return false
		}
	}
	return true
}
