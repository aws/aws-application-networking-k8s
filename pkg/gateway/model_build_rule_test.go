package gateway

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/service/vpclattice"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gateway_api "sigs.k8s.io/gateway-api/apis/v1beta1"

	"k8s.io/apimachinery/pkg/types"

	mock_client "github.com/aws/aws-application-networking-k8s/mocks/controller-runtime/client"

	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"

	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

func Test_RuleModelBuild(t *testing.T) {
	var httpSectionName gateway_api.SectionName = "http"
	var serviceKind gateway_api.Kind = "Service"
	var serviceimportKind gateway_api.Kind = "ServiceImport"
	var weight1 = int32(10)
	var weight2 = int32(90)
	var namespace = gateway_api.Namespace("default")
	var path1 = string("/ver1")
	var path2 = string("/ver2")
	var k8sPathMatchExactType = gateway_api.PathMatchExact
	var backendRef1 = gateway_api.BackendRef{
		BackendObjectReference: gateway_api.BackendObjectReference{
			Name:      "targetgroup1",
			Namespace: &namespace,
			Kind:      &serviceKind,
		},
		Weight: &weight1,
	}
	var backendRef2 = gateway_api.BackendRef{
		BackendObjectReference: gateway_api.BackendObjectReference{
			Name:      "targetgroup2",
			Namespace: &namespace,
			Kind:      &serviceimportKind,
		},
		Weight: &weight2,
	}
	var backendServiceImportRef = gateway_api.BackendRef{
		BackendObjectReference: gateway_api.BackendObjectReference{
			Name: "targetgroup1",
			Kind: &serviceimportKind,
		},
	}

	tests := []struct {
		name               string
		gwListenerPort     gateway_api.PortNumber
		httpRoute          *gateway_api.HTTPRoute
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
			httpRoute: &gateway_api.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gateway_api.HTTPRouteSpec{
					CommonRouteSpec: gateway_api.CommonRouteSpec{
						ParentRefs: []gateway_api.ParentReference{
							{
								Name:        "mesh1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []gateway_api.HTTPRouteRule{
						{
							BackendRefs: []gateway_api.HTTPBackendRef{
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
			httpRoute: &gateway_api.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gateway_api.HTTPRouteSpec{
					CommonRouteSpec: gateway_api.CommonRouteSpec{
						ParentRefs: []gateway_api.ParentReference{
							{
								Name:        "mesh1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []gateway_api.HTTPRouteRule{
						{
							BackendRefs: []gateway_api.HTTPBackendRef{
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
			httpRoute: &gateway_api.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gateway_api.HTTPRouteSpec{
					CommonRouteSpec: gateway_api.CommonRouteSpec{
						ParentRefs: []gateway_api.ParentReference{
							{
								Name:        "mesh1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []gateway_api.HTTPRouteRule{
						{
							BackendRefs: []gateway_api.HTTPBackendRef{
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
			httpRoute: &gateway_api.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gateway_api.HTTPRouteSpec{
					CommonRouteSpec: gateway_api.CommonRouteSpec{
						ParentRefs: []gateway_api.ParentReference{
							{
								Name:        "mesh1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []gateway_api.HTTPRouteRule{
						{
							Matches: []gateway_api.HTTPRouteMatch{
								{

									Path: &gateway_api.HTTPPathMatch{
										Type:  &k8sPathMatchExactType,
										Value: &path1,
									},
								},
							},
							BackendRefs: []gateway_api.HTTPBackendRef{
								{
									BackendRef: backendRef1,
								},
							},
						},
						{
							Matches: []gateway_api.HTTPRouteMatch{
								{

									Path: &gateway_api.HTTPPathMatch{

										Value: &path2,
									},
								},
							},
							BackendRefs: []gateway_api.HTTPBackendRef{
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
		fmt.Printf("Testing >>> %v\n", tt.name)
		c := gomock.NewController(t)
		defer c.Finish()
		ctx := context.TODO()

		k8sClient := mock_client.NewMockClient(c)

		if tt.k8sGetGatewayCall {

			k8sClient.EXPECT().Get(ctx, gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
				func(ctx context.Context, gwName types.NamespacedName, gw *gateway_api.Gateway, arg3 ...interface{}) error {

					if tt.k8sGatewayReturnOK {
						gw.Spec.Listeners = append(gw.Spec.Listeners, gateway_api.Listener{
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

		if len(resRules) > 0 {
			fmt.Printf("resRules :%v \n", *resRules[0])
		}

		var i = 1
		for _, resRule := range resRules {

			fmt.Sscanf(resRule.Spec.RuleID, "rule-%d", &i)

			assert.Equal(t, resRule.Spec.ListenerPort, int64(tt.gwListenerPort))
			// Defer this to dedicate rule check			assert.Equal(t, resRule.Spec.PathMatchValue, tt.httpRoute.)
			assert.Equal(t, resRule.Spec.ServiceName, tt.httpRoute.Name)
			assert.Equal(t, resRule.Spec.ServiceNamespace, tt.httpRoute.Namespace)

			var j = 0
			for _, tg := range resRule.Spec.Action.TargetGroups {

				assert.Equal(t, gateway_api.ObjectName(tg.Name), tt.httpRoute.Spec.Rules[i-1].BackendRefs[j].Name)
				if tt.httpRoute.Spec.Rules[i-1].BackendRefs[j].Namespace != nil {
					assert.Equal(t, gateway_api.Namespace(tg.Namespace), *tt.httpRoute.Spec.Rules[i-1].BackendRefs[j].Namespace)
				}

				if *tt.httpRoute.Spec.Rules[i-1].BackendRefs[j].Kind == "ServiceImport" {
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
	var httpSectionName gateway_api.SectionName = "http"
	var serviceKind gateway_api.Kind = "Service"

	var namespace = gateway_api.Namespace("default")
	var path1 = string("/ver1")
	//var path2 = string("/ver2")
	var k8sPathMatchExactType = gateway_api.PathMatchExact
	var k8sPathMatchPrefixType = gateway_api.PathMatchPathPrefix
	var k8sMethod = gateway_api.HTTPMethodGet

	var k8sHeaderExactType = gateway_api.HeaderMatchExact
	var hdr1 = "env1"
	var hdr1Value = "test1"
	var hdr2 = "env2"
	var hdr2Value = "test2"

	var backendRef1 = gateway_api.BackendRef{
		BackendObjectReference: gateway_api.BackendObjectReference{
			Name:      "targetgroup1",
			Namespace: &namespace,
			Kind:      &serviceKind,
		},
	}

	tests := []struct {
		name             string
		gwListenerPort   gateway_api.PortNumber
		httpRoute        *gateway_api.HTTPRoute
		expectedRuleSpec latticemodel.RuleSpec
		wantErrIsNil     bool
		samerule         bool
	}{
		{
			name:           "PathMatchExact Match",
			gwListenerPort: *PortNumberPtr(80),
			wantErrIsNil:   false,
			samerule:       true,

			httpRoute: &gateway_api.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gateway_api.HTTPRouteSpec{
					CommonRouteSpec: gateway_api.CommonRouteSpec{
						ParentRefs: []gateway_api.ParentReference{
							{
								Name:        "mesh1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []gateway_api.HTTPRouteRule{
						{
							Matches: []gateway_api.HTTPRouteMatch{
								{

									Path: &gateway_api.HTTPPathMatch{
										Type:  &k8sPathMatchExactType,
										Value: &path1,
									},
								},
							},
							BackendRefs: []gateway_api.HTTPBackendRef{
								{
									BackendRef: backendRef1,
								},
							},
						},
					},
				},
			},
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

			httpRoute: &gateway_api.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gateway_api.HTTPRouteSpec{
					CommonRouteSpec: gateway_api.CommonRouteSpec{
						ParentRefs: []gateway_api.ParentReference{
							{
								Name:        "mesh1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []gateway_api.HTTPRouteRule{
						{
							Matches: []gateway_api.HTTPRouteMatch{
								{

									Path: &gateway_api.HTTPPathMatch{
										Type:  &k8sPathMatchPrefixType,
										Value: &path1,
									},
								},
							},
							BackendRefs: []gateway_api.HTTPBackendRef{
								{
									BackendRef: backendRef1,
								},
							},
						},
					},
				},
			},
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

			httpRoute: &gateway_api.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gateway_api.HTTPRouteSpec{
					CommonRouteSpec: gateway_api.CommonRouteSpec{
						ParentRefs: []gateway_api.ParentReference{
							{
								Name:        "mesh1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []gateway_api.HTTPRouteRule{
						{
							Matches: []gateway_api.HTTPRouteMatch{
								{

									//	Path: &gateway_api.HTTPPathMatch{
									//		Type:  &k8sPathMatchPrefixType,
									//		Value: &path1,
									//	},
									Headers: []gateway_api.HTTPHeaderMatch{
										{
											Type:  &k8sHeaderExactType,
											Name:  gateway_api.HTTPHeaderName(hdr1),
											Value: hdr1Value,
										},
									},
								},
							},
							BackendRefs: []gateway_api.HTTPBackendRef{
								{
									BackendRef: backendRef1,
								},
							},
						},
					},
				},
			},
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

			httpRoute: &gateway_api.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gateway_api.HTTPRouteSpec{
					CommonRouteSpec: gateway_api.CommonRouteSpec{
						ParentRefs: []gateway_api.ParentReference{
							{
								Name:        "mesh1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []gateway_api.HTTPRouteRule{
						{
							Matches: []gateway_api.HTTPRouteMatch{
								{
									Headers: []gateway_api.HTTPHeaderMatch{
										{
											Type:  &k8sHeaderExactType,
											Name:  gateway_api.HTTPHeaderName(hdr1),
											Value: hdr1Value,
										},
										{
											Type:  &k8sHeaderExactType,
											Name:  gateway_api.HTTPHeaderName(hdr2),
											Value: hdr2Value,
										},
									},
								},
							},
							BackendRefs: []gateway_api.HTTPBackendRef{
								{
									BackendRef: backendRef1,
								},
							},
						},
					},
				},
			},
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

			httpRoute: &gateway_api.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gateway_api.HTTPRouteSpec{
					CommonRouteSpec: gateway_api.CommonRouteSpec{
						ParentRefs: []gateway_api.ParentReference{
							{
								Name:        "mesh1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []gateway_api.HTTPRouteRule{
						{
							Matches: []gateway_api.HTTPRouteMatch{
								{

									Path: &gateway_api.HTTPPathMatch{
										Type:  &k8sPathMatchExactType,
										Value: &path1,
									},
									Headers: []gateway_api.HTTPHeaderMatch{
										{
											Type:  &k8sHeaderExactType,
											Name:  gateway_api.HTTPHeaderName(hdr1),
											Value: hdr1Value,
										},
										{
											Type:  &k8sHeaderExactType,
											Name:  gateway_api.HTTPHeaderName(hdr2),
											Value: hdr2Value,
										},
									},
								},
							},
							BackendRefs: []gateway_api.HTTPBackendRef{
								{
									BackendRef: backendRef1,
								},
							},
						},
					},
				},
			},
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

			httpRoute: &gateway_api.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gateway_api.HTTPRouteSpec{
					CommonRouteSpec: gateway_api.CommonRouteSpec{
						ParentRefs: []gateway_api.ParentReference{
							{
								Name:        "mesh1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []gateway_api.HTTPRouteRule{
						{
							Matches: []gateway_api.HTTPRouteMatch{
								{

									Path: &gateway_api.HTTPPathMatch{
										Type:  &k8sPathMatchPrefixType,
										Value: &path1,
									},
									Headers: []gateway_api.HTTPHeaderMatch{
										{
											Type:  &k8sHeaderExactType,
											Name:  gateway_api.HTTPHeaderName(hdr1),
											Value: hdr1Value,
										},
										{
											Type:  &k8sHeaderExactType,
											Name:  gateway_api.HTTPHeaderName(hdr2),
											Value: hdr2Value,
										},
									},
								},
							},
							BackendRefs: []gateway_api.HTTPBackendRef{
								{
									BackendRef: backendRef1,
								},
							},
						},
					},
				},
			},
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

			httpRoute: &gateway_api.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gateway_api.HTTPRouteSpec{
					CommonRouteSpec: gateway_api.CommonRouteSpec{
						ParentRefs: []gateway_api.ParentReference{
							{
								Name:        "mesh1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []gateway_api.HTTPRouteRule{
						{
							Matches: []gateway_api.HTTPRouteMatch{
								{
									Headers: []gateway_api.HTTPHeaderMatch{
										{
											Type:  &k8sHeaderExactType,
											Name:  gateway_api.HTTPHeaderName(hdr1),
											Value: hdr1Value,
										},
										{
											Type:  &k8sHeaderExactType,
											Name:  gateway_api.HTTPHeaderName(hdr2),
											Value: hdr2Value,
										},
									},
								},
							},
							BackendRefs: []gateway_api.HTTPBackendRef{
								{
									BackendRef: backendRef1,
								},
							},
						},
					},
				},
			},
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

			httpRoute: &gateway_api.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gateway_api.HTTPRouteSpec{
					CommonRouteSpec: gateway_api.CommonRouteSpec{
						ParentRefs: []gateway_api.ParentReference{
							{
								Name:        "mesh1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []gateway_api.HTTPRouteRule{
						{
							Matches: []gateway_api.HTTPRouteMatch{
								{

									Path: &gateway_api.HTTPPathMatch{
										Type:  &k8sPathMatchExactType,
										Value: &path1,
									},
									Headers: []gateway_api.HTTPHeaderMatch{
										{
											Type:  &k8sHeaderExactType,
											Name:  gateway_api.HTTPHeaderName(hdr1),
											Value: hdr1Value,
										},
										{
											Type:  &k8sHeaderExactType,
											Name:  gateway_api.HTTPHeaderName(hdr2),
											Value: hdr2Value,
										},
										{
											Type:  &k8sHeaderExactType,
											Name:  gateway_api.HTTPHeaderName(hdr1),
											Value: hdr1Value,
										},
										{
											Type:  &k8sHeaderExactType,
											Name:  gateway_api.HTTPHeaderName(hdr2),
											Value: hdr2Value,
										},
										{
											Type:  &k8sHeaderExactType,
											Name:  gateway_api.HTTPHeaderName(hdr1),
											Value: hdr1Value,
										},
										{
											Type:  &k8sHeaderExactType,
											Name:  gateway_api.HTTPHeaderName(hdr2),
											Value: hdr2Value,
										},
									},
								},
							},
							BackendRefs: []gateway_api.HTTPBackendRef{
								{
									BackendRef: backendRef1,
								},
							},
						},
					},
				},
			},
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

			httpRoute: &gateway_api.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gateway_api.HTTPRouteSpec{
					CommonRouteSpec: gateway_api.CommonRouteSpec{
						ParentRefs: []gateway_api.ParentReference{
							{
								Name:        "mesh1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []gateway_api.HTTPRouteRule{
						{
							Matches: []gateway_api.HTTPRouteMatch{
								{

									Path: &gateway_api.HTTPPathMatch{
										Type:  &k8sPathMatchExactType,
										Value: &path1,
									},
								},
								{

									Path: &gateway_api.HTTPPathMatch{
										Type:  &k8sPathMatchExactType,
										Value: &path1,
									},
								},
							},
							BackendRefs: []gateway_api.HTTPBackendRef{
								{
									BackendRef: backendRef1,
								},
							},
						},
					},
				},
			},
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

			httpRoute: &gateway_api.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gateway_api.HTTPRouteSpec{
					CommonRouteSpec: gateway_api.CommonRouteSpec{
						ParentRefs: []gateway_api.ParentReference{
							{
								Name:        "mesh1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []gateway_api.HTTPRouteRule{
						{
							Matches: []gateway_api.HTTPRouteMatch{
								{

									Method: &k8sMethod,
								},
							},

							BackendRefs: []gateway_api.HTTPBackendRef{
								{
									BackendRef: backendRef1,
								},
							},
						},
					},
				},
			},
			expectedRuleSpec: latticemodel.RuleSpec{
				PathMatchExact: true,
				PathMatchValue: path1,
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
			func(ctx context.Context, gwName types.NamespacedName, gw *gateway_api.Gateway, arg3 ...interface{}) error {

				gw.Spec.Listeners = append(gw.Spec.Listeners, gateway_api.Listener{
					Port: tt.gwListenerPort,
					Name: *tt.httpRoute.Spec.ParentRefs[0].SectionName,
				})
				return nil

			},
		)

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

			if *rule1Hdr.Match.Exact == *rule2Hdr.Match.Exact &&
				*rule1Hdr.Name == *rule2Hdr.Name {
				found = true
				break
			}

		}
		if !found {
			return false
		}
	}
	return true
}
