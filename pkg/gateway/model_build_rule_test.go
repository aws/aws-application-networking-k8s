package gateway

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	apimachineryv1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/pointer"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type dummyTgBuilder struct {
	i int
}

func (d *dummyTgBuilder) Build(ctx context.Context, route core.Route, backendRef core.BackendRef, stack core.Stack) (core.Stack, *model.TargetGroup, error) {
	if backendRef.Name() == "invalid" {
		return stack, nil, &InvalidBackendRefError{
			BackendRef: backendRef,
			Reason:     "not valid",
		}
	}

	// just need to provide a TG with an ID
	id := fmt.Sprintf("tg-%d", d.i)
	d.i++

	tg := &model.TargetGroup{
		ResourceMeta: core.NewResourceMeta(stack, "AWS:VPCServiceNetwork::TargetGroup", id),
	}
	stack.AddResource(tg)
	return stack, tg, nil
}

func Test_RuleModelBuild(t *testing.T) {
	var httpSectionName gwv1beta1.SectionName = "http"
	var serviceKind gwv1beta1.Kind = "Service"
	var serviceImportKind gwv1beta1.Kind = "ServiceImport"
	var weight1 = int32(10)
	var weight2 = int32(90)
	var namespace = gwv1beta1.Namespace("testnamespace")
	var namespace2 = gwv1beta1.Namespace("testnamespace2")
	var path1 = "/ver1"
	var path2 = "/ver2"
	var path3 = "/ver3"
	var httpGet = gwv1.HTTPMethodGet
	var httpPost = gwv1.HTTPMethodPost
	var k8sPathMatchExactType = gwv1.PathMatchExact
	var k8sPathMatchPrefix = gwv1.PathMatchPathPrefix
	var k8sMethodMatchExactType = gwv1alpha2.GRPCMethodMatchExact
	var k8sHeaderExactType = gwv1.HeaderMatchExact
	var hdr1 = "env1"
	var hdr1Value = "test1"
	var hdr2 = "env2"
	var hdr2Value = "test2"

	var backendRef1 = gwv1beta1.BackendRef{
		BackendObjectReference: gwv1beta1.BackendObjectReference{
			Name: "targetgroup1",
			Kind: &serviceKind,
		},
		Weight: &weight1,
	}
	var backendRef2 = gwv1beta1.BackendRef{
		BackendObjectReference: gwv1beta1.BackendObjectReference{
			Name: "targetgroup2",
			Kind: &serviceImportKind,
		},
		Weight: &weight2,
	}
	var invalidBackendRef = gwv1beta1.BackendRef{
		BackendObjectReference: gwv1beta1.BackendObjectReference{
			Name: "invalid",
			Kind: &serviceKind,
		},
		Weight: &weight2,
	}
	var backendRef1Namespace1 = gwv1beta1.BackendRef{
		BackendObjectReference: gwv1beta1.BackendObjectReference{
			Name:      "targetgroup2",
			Namespace: &namespace,
			Kind:      &serviceImportKind,
		},
		Weight: &weight2,
	}
	var backendRef1Namespace2 = gwv1beta1.BackendRef{
		BackendObjectReference: gwv1beta1.BackendObjectReference{
			Name:      "targetgroup2",
			Namespace: &namespace2,
			Kind:      &serviceImportKind,
		},
		Weight: &weight2,
	}
	var backendServiceImportRef = gwv1beta1.BackendRef{
		BackendObjectReference: gwv1beta1.BackendObjectReference{
			Name: "targetgroup1",
			Kind: &serviceImportKind,
		},
	}

	tests := []struct {
		name         string
		route        core.Route
		wantErrIsNil bool
		expectedSpec []model.RuleSpec
	}{
		{
			name:         "rule, default service action",
			wantErrIsNil: true,
			route: core.NewHTTPRoute(gwv1beta1.HTTPRoute{
				ObjectMeta: apimachineryv1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gwv1beta1.HTTPRouteSpec{
					CommonRouteSpec: gwv1beta1.CommonRouteSpec{
						ParentRefs: []gwv1beta1.ParentReference{
							{
								Name:        "gw1",
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
							},
						},
					},
				},
			}),
			expectedSpec: []model.RuleSpec{ // note priority is only calculated at synthesis b/c it requires access to existing rules
				{
					StackListenerId: "listener-id",
					PathMatchPrefix: true,
					PathMatchValue:  "/",
					Action: model.RuleAction{
						TargetGroups: []*model.RuleTargetGroup{
							{
								StackTargetGroupId: "tg-0",
								Weight:             int64(weight1),
							},
						},
					},
				},
			},
		},
		{
			name:         "rule, default serviceimport action",
			wantErrIsNil: true,
			route: core.NewHTTPRoute(gwv1beta1.HTTPRoute{
				ObjectMeta: apimachineryv1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gwv1beta1.HTTPRouteSpec{
					CommonRouteSpec: gwv1beta1.CommonRouteSpec{
						ParentRefs: []gwv1beta1.ParentReference{
							{
								Name:        "gw1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []gwv1beta1.HTTPRouteRule{
						{
							BackendRefs: []gwv1beta1.HTTPBackendRef{
								{
									BackendRef: backendServiceImportRef,
								},
							},
						},
					},
				},
			}),
			expectedSpec: []model.RuleSpec{
				{
					StackListenerId: "listener-id",
					PathMatchPrefix: true,
					PathMatchValue:  "/",
					Action: model.RuleAction{
						TargetGroups: []*model.RuleTargetGroup{
							{
								SvcImportTG: &model.SvcImportTargetGroup{
									K8SServiceName:      string(backendServiceImportRef.Name),
									K8SServiceNamespace: "default",
								},
								Weight: 1,
							},
						},
					},
				},
			},
		},
		{
			name:         "rule, weighted target group",
			wantErrIsNil: true,
			route: core.NewHTTPRoute(gwv1beta1.HTTPRoute{
				ObjectMeta: apimachineryv1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gwv1beta1.HTTPRouteSpec{
					CommonRouteSpec: gwv1beta1.CommonRouteSpec{
						ParentRefs: []gwv1beta1.ParentReference{
							{
								Name:        "gw1",
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
			expectedSpec: []model.RuleSpec{
				{
					StackListenerId: "listener-id",
					PathMatchPrefix: true,
					PathMatchValue:  "/",
					Action: model.RuleAction{
						TargetGroups: []*model.RuleTargetGroup{
							{
								StackTargetGroupId: "tg-0",
								Weight:             int64(weight1),
							},
							{
								SvcImportTG: &model.SvcImportTargetGroup{
									K8SServiceName:      string(backendRef2.Name),
									K8SServiceNamespace: "default",
								},
								Weight: int64(weight2),
							},
						},
					},
				},
			},
		},
		{
			name:         "rule, path based target group",
			wantErrIsNil: true,
			route: core.NewHTTPRoute(gwv1beta1.HTTPRoute{
				ObjectMeta: apimachineryv1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gwv1beta1.HTTPRouteSpec{
					CommonRouteSpec: gwv1beta1.CommonRouteSpec{
						ParentRefs: []gwv1beta1.ParentReference{
							{
								Name:        "gw1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []gwv1beta1.HTTPRouteRule{
						{
							Matches: []gwv1beta1.HTTPRouteMatch{
								{
									Path: &gwv1beta1.HTTPPathMatch{
										Type:  &k8sPathMatchExactType,
										Value: &path1,
									},
								},
							},
							BackendRefs: []gwv1beta1.HTTPBackendRef{
								{
									BackendRef: backendRef1,
								},
							},
						},
						{
							Matches: []gwv1beta1.HTTPRouteMatch{
								{
									Path: &gwv1beta1.HTTPPathMatch{
										Type:  &k8sPathMatchPrefix,
										Value: &path2,
									},
								},
							},
							BackendRefs: []gwv1beta1.HTTPBackendRef{
								{
									BackendRef: backendRef2,
								},
							},
						},
					},
				},
			}),
			expectedSpec: []model.RuleSpec{
				{
					StackListenerId: "listener-id",
					PathMatchExact:  true,
					PathMatchValue:  path1,
					Action: model.RuleAction{
						TargetGroups: []*model.RuleTargetGroup{
							{
								StackTargetGroupId: "tg-0",
								Weight:             int64(weight1),
							},
						},
					},
				},
				{
					StackListenerId: "listener-id",
					PathMatchPrefix: true,
					PathMatchValue:  path2,
					Action: model.RuleAction{
						TargetGroups: []*model.RuleTargetGroup{
							{
								SvcImportTG: &model.SvcImportTargetGroup{
									K8SServiceName:      string(backendRef2.Name),
									K8SServiceNamespace: "default",
								},
								Weight: int64(weight2),
							},
						},
					},
				},
			},
		},
		{
			name:         "rule, method based",
			wantErrIsNil: true,
			route: core.NewHTTPRoute(gwv1beta1.HTTPRoute{
				ObjectMeta: apimachineryv1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gwv1beta1.HTTPRouteSpec{
					CommonRouteSpec: gwv1beta1.CommonRouteSpec{
						ParentRefs: []gwv1beta1.ParentReference{
							{
								Name:        "gw1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []gwv1beta1.HTTPRouteRule{
						{
							Matches: []gwv1beta1.HTTPRouteMatch{
								{
									Method: &httpGet,
								},
							},
							BackendRefs: []gwv1beta1.HTTPBackendRef{
								{
									BackendRef: backendRef1,
								},
							},
						},
						{
							Matches: []gwv1beta1.HTTPRouteMatch{
								{
									Method: &httpPost,
								},
							},
							BackendRefs: []gwv1beta1.HTTPBackendRef{
								{
									BackendRef: backendRef2,
								},
							},
						},
					},
				},
			}),
			expectedSpec: []model.RuleSpec{
				{
					StackListenerId: "listener-id",
					Method:          string(httpGet),
					Action: model.RuleAction{
						TargetGroups: []*model.RuleTargetGroup{
							{
								StackTargetGroupId: "tg-0",
								Weight:             int64(weight1),
							},
						},
					},
				},
				{
					StackListenerId: "listener-id",
					Method:          string(httpPost),
					Action: model.RuleAction{
						TargetGroups: []*model.RuleTargetGroup{
							{
								SvcImportTG: &model.SvcImportTargetGroup{
									K8SServiceName:      string(backendRef2.Name),
									K8SServiceNamespace: "default",
								},
								Weight: int64(weight2),
							},
						},
					},
				},
			},
		},
		{
			name:         "rule, different namespace combination",
			wantErrIsNil: true,
			route: core.NewHTTPRoute(gwv1beta1.HTTPRoute{
				ObjectMeta: apimachineryv1.ObjectMeta{
					Name:      "service1",
					Namespace: "non-default",
				},
				Spec: gwv1beta1.HTTPRouteSpec{
					CommonRouteSpec: gwv1beta1.CommonRouteSpec{
						ParentRefs: []gwv1beta1.ParentReference{
							{
								Name:        "gw1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []gwv1beta1.HTTPRouteRule{
						{
							Matches: []gwv1beta1.HTTPRouteMatch{
								{
									Path: &gwv1beta1.HTTPPathMatch{
										Value: &path1,
										Type:  &k8sPathMatchExactType,
									},
								},
							},
							BackendRefs: []gwv1beta1.HTTPBackendRef{
								{
									BackendRef: backendRef1,
								},
							},
						},
						{
							Matches: []gwv1beta1.HTTPRouteMatch{
								{
									Path: &gwv1beta1.HTTPPathMatch{
										Value: &path2,
										Type:  &k8sPathMatchExactType,
									},
								},
							},
							BackendRefs: []gwv1beta1.HTTPBackendRef{
								{
									BackendRef: backendRef1Namespace1,
								},
							},
						},
						{
							Matches: []gwv1beta1.HTTPRouteMatch{
								{
									Path: &gwv1beta1.HTTPPathMatch{
										Value: &path3,
										Type:  &k8sPathMatchExactType,
									},
								},
							},
							BackendRefs: []gwv1beta1.HTTPBackendRef{
								{
									BackendRef: backendRef1Namespace2,
								},
							},
						},
					},
				},
			}),
			expectedSpec: []model.RuleSpec{
				{
					StackListenerId: "listener-id",
					PathMatchExact:  true,
					PathMatchValue:  path1,
					Action: model.RuleAction{
						TargetGroups: []*model.RuleTargetGroup{
							{
								StackTargetGroupId: "tg-0",
								Weight:             int64(weight1),
							},
						},
					},
				},
				{
					StackListenerId: "listener-id",
					PathMatchExact:  true,
					PathMatchValue:  path2,
					Action: model.RuleAction{
						TargetGroups: []*model.RuleTargetGroup{
							{
								SvcImportTG: &model.SvcImportTargetGroup{
									K8SServiceName:      string(backendRef1Namespace1.Name),
									K8SServiceNamespace: string(*backendRef1Namespace1.Namespace),
								},
								Weight: int64(weight2),
							},
						},
					},
				},
				{
					StackListenerId: "listener-id",
					PathMatchExact:  true,
					PathMatchValue:  path3,
					Action: model.RuleAction{
						TargetGroups: []*model.RuleTargetGroup{
							{
								SvcImportTG: &model.SvcImportTargetGroup{
									K8SServiceName:      string(backendRef1Namespace2.Name),
									K8SServiceNamespace: string(*backendRef1Namespace2.Namespace),
								},
								Weight: int64(weight2),
							},
						},
					},
				},
			},
		},
		{
			name:         "rule, default service import action for GRPCRoute",
			wantErrIsNil: true,
			route: core.NewGRPCRoute(gwv1alpha2.GRPCRoute{
				ObjectMeta: apimachineryv1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gwv1alpha2.GRPCRouteSpec{
					CommonRouteSpec: gwv1alpha2.CommonRouteSpec{
						ParentRefs: []gwv1alpha2.ParentReference{
							{
								Name:        "gw1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []gwv1alpha2.GRPCRouteRule{
						{
							BackendRefs: []gwv1alpha2.GRPCBackendRef{
								{
									BackendRef: backendServiceImportRef,
								},
							},
						},
					},
				},
			}),
			expectedSpec: []model.RuleSpec{
				{
					StackListenerId: "listener-id",
					Method:          string(httpPost),
					PathMatchPrefix: true,
					PathMatchValue:  "/",
					Action: model.RuleAction{
						TargetGroups: []*model.RuleTargetGroup{
							{
								SvcImportTG: &model.SvcImportTargetGroup{
									K8SServiceName:      string(backendServiceImportRef.Name),
									K8SServiceNamespace: "default",
								},
								Weight: 1,
							},
						},
					},
				},
			},
		},
		{
			name:         "rule, gRPC routes with methods and multiple namespaces",
			wantErrIsNil: true,
			route: core.NewGRPCRoute(gwv1alpha2.GRPCRoute{
				ObjectMeta: apimachineryv1.ObjectMeta{
					Name:      "service1",
					Namespace: "non-default",
				},
				Spec: gwv1alpha2.GRPCRouteSpec{
					CommonRouteSpec: gwv1beta1.CommonRouteSpec{
						ParentRefs: []gwv1beta1.ParentReference{
							{
								Name:        "gw1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []gwv1alpha2.GRPCRouteRule{
						{
							Matches: []gwv1alpha2.GRPCRouteMatch{
								{
									Method: &gwv1alpha2.GRPCMethodMatch{
										Type:    &k8sMethodMatchExactType,
										Service: pointer.String("service"),
										Method:  pointer.String("method1"),
									},
								},
							},
							BackendRefs: []gwv1alpha2.GRPCBackendRef{
								{
									BackendRef: backendRef1,
								},
							},
						},
						{
							Matches: []gwv1alpha2.GRPCRouteMatch{
								{
									Method: &gwv1alpha2.GRPCMethodMatch{
										Type:    &k8sMethodMatchExactType,
										Service: pointer.String("service"),
										Method:  pointer.String("method2"),
									},
								},
							},
							BackendRefs: []gwv1alpha2.GRPCBackendRef{
								{
									BackendRef: backendRef1Namespace1,
								},
							},
						},
						{
							Matches: []gwv1alpha2.GRPCRouteMatch{
								{
									Method: &gwv1alpha2.GRPCMethodMatch{
										Type:    &k8sMethodMatchExactType,
										Service: pointer.String("service"),
										Method:  pointer.String("method3"),
									},
								},
							},
							BackendRefs: []gwv1alpha2.GRPCBackendRef{
								{
									BackendRef: backendRef1Namespace2,
								},
							},
						},
					},
				},
			}),
			expectedSpec: []model.RuleSpec{
				{
					StackListenerId: "listener-id",
					PathMatchExact:  true,
					PathMatchValue:  "/service/method1",
					Method:          string(httpPost),
					Action: model.RuleAction{
						TargetGroups: []*model.RuleTargetGroup{
							{
								StackTargetGroupId: "tg-0",
								Weight:             int64(weight1),
							},
						},
					},
				},
				{
					StackListenerId: "listener-id",
					PathMatchExact:  true,
					PathMatchValue:  "/service/method2",
					Method:          string(httpPost),
					Action: model.RuleAction{
						TargetGroups: []*model.RuleTargetGroup{
							{
								SvcImportTG: &model.SvcImportTargetGroup{
									K8SServiceName:      string(backendRef1Namespace1.Name),
									K8SServiceNamespace: string(*backendRef1Namespace1.Namespace),
								},
								Weight: int64(weight2),
							},
						},
					},
				},
				{
					StackListenerId: "listener-id",
					PathMatchExact:  true,
					PathMatchValue:  "/service/method3",
					Method:          string(httpPost),
					Action: model.RuleAction{
						TargetGroups: []*model.RuleTargetGroup{
							{
								SvcImportTG: &model.SvcImportTargetGroup{
									K8SServiceName:      string(backendRef1Namespace2.Name),
									K8SServiceNamespace: string(*backendRef1Namespace2.Namespace),
								},
								Weight: int64(weight2),
							},
						},
					},
				},
			},
		},
		{
			name:         "1 header match",
			wantErrIsNil: true,
			route: core.NewHTTPRoute(gwv1beta1.HTTPRoute{
				ObjectMeta: apimachineryv1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gwv1beta1.HTTPRouteSpec{
					CommonRouteSpec: gwv1beta1.CommonRouteSpec{
						ParentRefs: []gwv1beta1.ParentReference{
							{
								Name:        "gw1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []gwv1beta1.HTTPRouteRule{
						{
							Matches: []gwv1beta1.HTTPRouteMatch{
								{
									Headers: []gwv1beta1.HTTPHeaderMatch{
										{
											Type:  &k8sHeaderExactType,
											Name:  gwv1.HTTPHeaderName(hdr1),
											Value: hdr1Value,
										},
									},
								},
							},
							BackendRefs: []gwv1beta1.HTTPBackendRef{
								{
									BackendRef: backendRef1,
								},
							},
						},
					},
				},
			}),
			expectedSpec: []model.RuleSpec{
				{
					StackListenerId: "listener-id",
					MatchedHeaders: []vpclattice.HeaderMatch{
						{
							Name: &hdr1,
							Match: &vpclattice.HeaderMatchType{
								Exact: &hdr1Value,
							},
						},
					},
					Action: model.RuleAction{
						TargetGroups: []*model.RuleTargetGroup{
							{
								StackTargetGroupId: "tg-0",
								Weight:             int64(*backendRef1.Weight),
							},
						},
					},
				},
			},
		},
		{
			name:         "2 header match",
			wantErrIsNil: true,
			route: core.NewHTTPRoute(gwv1beta1.HTTPRoute{
				ObjectMeta: apimachineryv1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gwv1beta1.HTTPRouteSpec{
					CommonRouteSpec: gwv1beta1.CommonRouteSpec{
						ParentRefs: []gwv1beta1.ParentReference{
							{
								Name:        "gw1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []gwv1beta1.HTTPRouteRule{
						{
							Matches: []gwv1beta1.HTTPRouteMatch{
								{
									Headers: []gwv1beta1.HTTPHeaderMatch{
										{
											Type:  &k8sHeaderExactType,
											Name:  gwv1.HTTPHeaderName(hdr1),
											Value: hdr1Value,
										},
										{
											Type:  &k8sHeaderExactType,
											Name:  gwv1.HTTPHeaderName(hdr2),
											Value: hdr2Value,
										},
									},
								},
							},
							BackendRefs: []gwv1beta1.HTTPBackendRef{
								{
									BackendRef: backendRef1,
								},
							},
						},
					},
				},
			}),
			expectedSpec: []model.RuleSpec{
				{
					StackListenerId: "listener-id",
					MatchedHeaders: []vpclattice.HeaderMatch{
						{
							Name: &hdr1,
							Match: &vpclattice.HeaderMatchType{
								Exact: &hdr1Value,
							},
						},
						{
							Name: &hdr2,
							Match: &vpclattice.HeaderMatchType{
								Exact: &hdr2Value,
							},
						},
					},
					Action: model.RuleAction{
						TargetGroups: []*model.RuleTargetGroup{
							{
								StackTargetGroupId: "tg-0",
								Weight:             int64(*backendRef1.Weight),
							},
						},
					},
				},
			},
		},
		{
			name:         "2 header match with path exact",
			wantErrIsNil: true,
			route: core.NewHTTPRoute(gwv1beta1.HTTPRoute{
				ObjectMeta: apimachineryv1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gwv1beta1.HTTPRouteSpec{
					CommonRouteSpec: gwv1beta1.CommonRouteSpec{
						ParentRefs: []gwv1beta1.ParentReference{
							{
								Name:        "gw1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []gwv1beta1.HTTPRouteRule{
						{
							Matches: []gwv1beta1.HTTPRouteMatch{
								{

									Path: &gwv1beta1.HTTPPathMatch{
										Type:  &k8sPathMatchExactType,
										Value: &path1,
									},
									Headers: []gwv1beta1.HTTPHeaderMatch{
										{
											Type:  &k8sHeaderExactType,
											Name:  gwv1.HTTPHeaderName(hdr1),
											Value: hdr1Value,
										},
										{
											Type:  &k8sHeaderExactType,
											Name:  gwv1.HTTPHeaderName(hdr2),
											Value: hdr2Value,
										},
									},
								},
							},
							BackendRefs: []gwv1beta1.HTTPBackendRef{
								{
									BackendRef: backendRef1,
								},
							},
						},
					},
				},
			}),
			expectedSpec: []model.RuleSpec{
				{
					StackListenerId: "listener-id",
					PathMatchExact:  true,
					PathMatchValue:  path1,
					MatchedHeaders: []vpclattice.HeaderMatch{
						{
							Name: &hdr1,
							Match: &vpclattice.HeaderMatchType{
								Exact: &hdr1Value,
							},
						},
						{
							Name: &hdr2,
							Match: &vpclattice.HeaderMatchType{
								Exact: &hdr2Value,
							},
						},
					},
					Action: model.RuleAction{
						TargetGroups: []*model.RuleTargetGroup{
							{
								StackTargetGroupId: "tg-0",
								Weight:             int64(*backendRef1.Weight),
							},
						},
					},
				},
			},
		},
		{
			name:         "2 header match with path prefix",
			wantErrIsNil: true,
			route: core.NewHTTPRoute(gwv1beta1.HTTPRoute{
				ObjectMeta: apimachineryv1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gwv1beta1.HTTPRouteSpec{
					CommonRouteSpec: gwv1beta1.CommonRouteSpec{
						ParentRefs: []gwv1beta1.ParentReference{
							{
								Name:        "gw1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []gwv1beta1.HTTPRouteRule{
						{
							Matches: []gwv1beta1.HTTPRouteMatch{
								{

									Path: &gwv1beta1.HTTPPathMatch{
										Type:  &k8sPathMatchPrefix,
										Value: &path1,
									},
									Headers: []gwv1beta1.HTTPHeaderMatch{
										{
											Type:  &k8sHeaderExactType,
											Name:  gwv1.HTTPHeaderName(hdr1),
											Value: hdr1Value,
										},
										{
											Type:  &k8sHeaderExactType,
											Name:  gwv1.HTTPHeaderName(hdr2),
											Value: hdr2Value,
										},
									},
								},
							},
							BackendRefs: []gwv1beta1.HTTPBackendRef{
								{
									BackendRef: backendRef1,
								},
							},
						},
					},
				},
			}),
			expectedSpec: []model.RuleSpec{
				{
					StackListenerId: "listener-id",
					PathMatchPrefix: true,
					PathMatchValue:  path1,
					MatchedHeaders: []vpclattice.HeaderMatch{
						{
							Name: &hdr1,
							Match: &vpclattice.HeaderMatchType{
								Exact: &hdr1Value,
							},
						},
						{
							Name: &hdr2,
							Match: &vpclattice.HeaderMatchType{
								Exact: &hdr2Value,
							},
						},
					},
					Action: model.RuleAction{
						TargetGroups: []*model.RuleTargetGroup{
							{
								StackTargetGroupId: "tg-0",
								Weight:             int64(*backendRef1.Weight),
							},
						},
					},
				},
			},
		},
		{
			name:         " negative 6 header match (max headers is 5)",
			wantErrIsNil: false,
			route: core.NewHTTPRoute(gwv1beta1.HTTPRoute{
				ObjectMeta: apimachineryv1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gwv1beta1.HTTPRouteSpec{
					CommonRouteSpec: gwv1beta1.CommonRouteSpec{
						ParentRefs: []gwv1beta1.ParentReference{
							{
								Name:        "gw1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []gwv1beta1.HTTPRouteRule{
						{
							Matches: []gwv1beta1.HTTPRouteMatch{
								{

									Path: &gwv1beta1.HTTPPathMatch{
										Type:  &k8sPathMatchExactType,
										Value: &path1,
									},
									Headers: []gwv1beta1.HTTPHeaderMatch{
										{
											Type:  &k8sHeaderExactType,
											Name:  gwv1.HTTPHeaderName(hdr1),
											Value: hdr1Value,
										},
										{
											Type:  &k8sHeaderExactType,
											Name:  gwv1.HTTPHeaderName(hdr2),
											Value: hdr2Value,
										},
										{
											Type:  &k8sHeaderExactType,
											Name:  gwv1.HTTPHeaderName(hdr1),
											Value: hdr1Value,
										},
										{
											Type:  &k8sHeaderExactType,
											Name:  gwv1.HTTPHeaderName(hdr2),
											Value: hdr2Value,
										},
										{
											Type:  &k8sHeaderExactType,
											Name:  gwv1.HTTPHeaderName(hdr1),
											Value: hdr1Value,
										},
										{
											Type:  &k8sHeaderExactType,
											Name:  gwv1.HTTPHeaderName(hdr2),
											Value: hdr2Value,
										},
									},
								},
							},
							BackendRefs: []gwv1beta1.HTTPBackendRef{
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
			name:         "Negative, multiple methods",
			wantErrIsNil: false,
			route: core.NewHTTPRoute(gwv1beta1.HTTPRoute{
				ObjectMeta: apimachineryv1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gwv1beta1.HTTPRouteSpec{
					CommonRouteSpec: gwv1beta1.CommonRouteSpec{
						ParentRefs: []gwv1beta1.ParentReference{
							{
								Name:        "gw1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []gwv1beta1.HTTPRouteRule{
						{
							Matches: []gwv1beta1.HTTPRouteMatch{
								{

									Path: &gwv1beta1.HTTPPathMatch{
										Type:  &k8sPathMatchExactType,
										Value: &path1,
									},
								},
								{

									Path: &gwv1beta1.HTTPPathMatch{
										Type:  &k8sPathMatchExactType,
										Value: &path1,
									},
								},
							},
							BackendRefs: []gwv1beta1.HTTPBackendRef{
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
			name:         "GRPC match on service and method",
			wantErrIsNil: true,
			route: core.NewGRPCRoute(gwv1alpha2.GRPCRoute{
				ObjectMeta: apimachineryv1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gwv1alpha2.GRPCRouteSpec{
					CommonRouteSpec: gwv1beta1.CommonRouteSpec{
						ParentRefs: []gwv1beta1.ParentReference{
							{
								Name:        "gw1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []gwv1alpha2.GRPCRouteRule{
						{
							Matches: []gwv1alpha2.GRPCRouteMatch{
								{
									Method: &gwv1alpha2.GRPCMethodMatch{
										Type:    &k8sMethodMatchExactType,
										Service: pointer.String("service"),
										Method:  pointer.String("method"),
									},
								},
							},
							BackendRefs: []gwv1alpha2.GRPCBackendRef{
								{
									BackendRef: backendRef1,
								},
							},
						},
					},
				},
			}),
			expectedSpec: []model.RuleSpec{
				{
					StackListenerId: "listener-id",
					PathMatchExact:  true,
					PathMatchValue:  "/service/method",
					Method:          "POST",
					Action: model.RuleAction{
						TargetGroups: []*model.RuleTargetGroup{
							{
								StackTargetGroupId: "tg-0",
								Weight:             int64(*backendRef1.Weight),
							},
						},
					},
				},
			},
		},
		{
			name:         "GRPC match on service",
			wantErrIsNil: true,
			route: core.NewGRPCRoute(gwv1alpha2.GRPCRoute{
				ObjectMeta: apimachineryv1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gwv1alpha2.GRPCRouteSpec{
					CommonRouteSpec: gwv1beta1.CommonRouteSpec{
						ParentRefs: []gwv1beta1.ParentReference{
							{
								Name:        "gw1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []gwv1alpha2.GRPCRouteRule{
						{
							Matches: []gwv1alpha2.GRPCRouteMatch{
								{
									Method: &gwv1alpha2.GRPCMethodMatch{
										Type:    &k8sMethodMatchExactType,
										Service: pointer.String("service"),
									},
								},
							},
							BackendRefs: []gwv1alpha2.GRPCBackendRef{
								{
									BackendRef: backendRef1,
								},
							},
						},
					},
				},
			}),
			expectedSpec: []model.RuleSpec{
				{
					StackListenerId: "listener-id",
					PathMatchPrefix: true,
					PathMatchValue:  "/service/",
					Method:          "POST",
					Action: model.RuleAction{
						TargetGroups: []*model.RuleTargetGroup{
							{
								StackTargetGroupId: "tg-0",
								Weight:             int64(*backendRef1.Weight),
							},
						},
					},
				},
			},
		},
		{
			name:         "GRPC match on all",
			wantErrIsNil: true,
			route: core.NewGRPCRoute(gwv1alpha2.GRPCRoute{
				ObjectMeta: apimachineryv1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gwv1alpha2.GRPCRouteSpec{
					CommonRouteSpec: gwv1beta1.CommonRouteSpec{
						ParentRefs: []gwv1beta1.ParentReference{
							{
								Name:        "gw1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []gwv1alpha2.GRPCRouteRule{
						{
							Matches: []gwv1alpha2.GRPCRouteMatch{
								{
									Method: &gwv1alpha2.GRPCMethodMatch{
										Type: &k8sMethodMatchExactType,
									},
								},
							},

							BackendRefs: []gwv1alpha2.GRPCBackendRef{
								{
									BackendRef: backendRef1,
								},
							},
						},
					},
				},
			}),
			expectedSpec: []model.RuleSpec{
				{
					StackListenerId: "listener-id",
					PathMatchPrefix: true,
					PathMatchValue:  "/",
					Method:          "POST",
					Action: model.RuleAction{
						TargetGroups: []*model.RuleTargetGroup{
							{
								StackTargetGroupId: "tg-0",
								Weight:             int64(*backendRef1.Weight),
							},
						},
					},
				},
			},
		},
		{
			name:         "GRPC match with 5 headers",
			wantErrIsNil: true,
			route: core.NewGRPCRoute(gwv1alpha2.GRPCRoute{
				ObjectMeta: apimachineryv1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gwv1alpha2.GRPCRouteSpec{
					CommonRouteSpec: gwv1beta1.CommonRouteSpec{
						ParentRefs: []gwv1beta1.ParentReference{
							{
								Name:        "gw1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []gwv1alpha2.GRPCRouteRule{
						{
							Matches: []gwv1alpha2.GRPCRouteMatch{
								{
									Method: &gwv1alpha2.GRPCMethodMatch{
										Type:    &k8sMethodMatchExactType,
										Service: pointer.String("service"),
									},
									Headers: []gwv1alpha2.GRPCHeaderMatch{
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
							BackendRefs: []gwv1alpha2.GRPCBackendRef{
								{
									BackendRef: backendRef1,
								},
							},
						},
					},
				},
			}),
			expectedSpec: []model.RuleSpec{
				{
					StackListenerId: "listener-id",
					PathMatchPrefix: true,
					PathMatchValue:  "/service/",
					Method:          "POST",
					Action: model.RuleAction{
						TargetGroups: []*model.RuleTargetGroup{
							{
								StackTargetGroupId: "tg-0",
								Weight:             int64(*backendRef1.Weight),
							},
						},
					},
					MatchedHeaders: []vpclattice.HeaderMatch{
						{
							Name: aws.String("foo1"),
							Match: &vpclattice.HeaderMatchType{
								Exact: aws.String("bar1"),
							},
						},
						{
							Name: aws.String("foo2"),
							Match: &vpclattice.HeaderMatchType{
								Exact: aws.String("bar2"),
							},
						},
						{
							Name: aws.String("foo3"),
							Match: &vpclattice.HeaderMatchType{
								Exact: aws.String("bar3"),
							},
						},
						{
							Name: aws.String("foo4"),
							Match: &vpclattice.HeaderMatchType{
								Exact: aws.String("bar4"),
							},
						},
						{
							Name: aws.String("foo5"),
							Match: &vpclattice.HeaderMatchType{
								Exact: aws.String("bar5"),
							},
						},
					},
				},
			},
		},
		{
			name:         "invalid backendRef",
			wantErrIsNil: true,
			route: core.NewHTTPRoute(gwv1beta1.HTTPRoute{
				ObjectMeta: apimachineryv1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gwv1beta1.HTTPRouteSpec{
					CommonRouteSpec: gwv1beta1.CommonRouteSpec{
						ParentRefs: []gwv1beta1.ParentReference{
							{
								Name:        "gw1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []gwv1beta1.HTTPRouteRule{
						{
							BackendRefs: []gwv1beta1.HTTPBackendRef{
								{
									BackendRef: invalidBackendRef,
								},
							},
						},
					},
				},
			}),
			expectedSpec: []model.RuleSpec{
				{
					StackListenerId: "listener-id",
					PathMatchPrefix: true,
					PathMatchValue:  "/",
					Action: model.RuleAction{
						TargetGroups: []*model.RuleTargetGroup{
							{
								StackTargetGroupId: model.InvalidBackendRefTgId,
								Weight:             int64(*invalidBackendRef.Weight),
							},
						},
					},
				},
			},
		},

		{
			name:         "valid and invalid backendRef",
			wantErrIsNil: true,
			route: core.NewHTTPRoute(gwv1beta1.HTTPRoute{
				ObjectMeta: apimachineryv1.ObjectMeta{
					Name:      "service1",
					Namespace: "default",
				},
				Spec: gwv1beta1.HTTPRouteSpec{
					CommonRouteSpec: gwv1beta1.CommonRouteSpec{
						ParentRefs: []gwv1beta1.ParentReference{
							{
								Name:        "gw1",
								SectionName: &httpSectionName,
							},
						},
					},
					Rules: []gwv1beta1.HTTPRouteRule{
						{
							BackendRefs: []gwv1beta1.HTTPBackendRef{
								{
									BackendRef: invalidBackendRef,
								},
								{
									BackendRef: backendRef1,
								},
							},
						},
					},
				},
			}),
			expectedSpec: []model.RuleSpec{
				{
					StackListenerId: "listener-id",
					PathMatchPrefix: true,
					PathMatchValue:  "/",
					Action: model.RuleAction{
						TargetGroups: []*model.RuleTargetGroup{
							{
								StackTargetGroupId: model.InvalidBackendRefTgId,
								Weight:             int64(*invalidBackendRef.Weight),
							},
							{
								StackTargetGroupId: "tg-0",
								Weight:             int64(*backendRef1.Weight),
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := gomock.NewController(t)
			defer c.Finish()
			ctx := context.TODO()

			k8sSchema := runtime.NewScheme()
			k8sSchema.AddKnownTypes(anv1alpha1.SchemeGroupVersion, &anv1alpha1.ServiceImport{})
			clientgoscheme.AddToScheme(k8sSchema)
			k8sClient := testclient.NewClientBuilder().WithScheme(k8sSchema).Build()

			svc := corev1.Service{
				ObjectMeta: apimachineryv1.ObjectMeta{
					Name:      string(backendRef1.Name),
					Namespace: "default",
				},
				Status: corev1.ServiceStatus{},
			}
			assert.NoError(t, k8sClient.Create(ctx, svc.DeepCopy()))
			stack := core.NewDefaultStack(core.StackID(k8s.NamespacedName(tt.route.K8sObject())))

			task := &latticeServiceModelBuildTask{
				log:         gwlog.FallbackLogger,
				route:       tt.route,
				stack:       stack,
				client:      k8sClient,
				brTgBuilder: &dummyTgBuilder{},
			}

			err := task.buildRules(ctx, "listener-id")
			if tt.wantErrIsNil {
				assert.NoError(t, err)
			} else {
				assert.NotNil(t, err)
				return
			}

			var resRules []*model.Rule
			stack.ListResources(&resRules)

			validateEqual(t, tt.expectedSpec, resRules)
		})
	}
}

func validateEqual(t *testing.T, expectedRules []model.RuleSpec, actualRules []*model.Rule) {
	assert.Equal(t, len(expectedRules), len(actualRules))
	assert.Equal(t, len(expectedRules), len(actualRules))

	for i, expectedSpec := range expectedRules {
		actualRule := actualRules[i]

		assert.Equal(t, expectedSpec.StackListenerId, actualRule.Spec.StackListenerId)
		assert.Equal(t, expectedSpec.PathMatchValue, actualRule.Spec.PathMatchValue)
		assert.Equal(t, expectedSpec.PathMatchPrefix, actualRule.Spec.PathMatchPrefix)
		assert.Equal(t, expectedSpec.PathMatchExact, actualRule.Spec.PathMatchExact)
		assert.Equal(t, expectedSpec.Method, actualRule.Spec.Method)

		// priority is not determined by model building, but in synthesis, so we don't
		// validate priority here

		assert.True(t, reflect.DeepEqual(expectedSpec.MatchedHeaders, actualRule.Spec.MatchedHeaders))

		assert.Equal(t, len(expectedSpec.Action.TargetGroups), len(actualRule.Spec.Action.TargetGroups))
		for j, etg := range expectedSpec.Action.TargetGroups {
			atg := actualRule.Spec.Action.TargetGroups[j]

			assert.Equal(t, etg.Weight, atg.Weight)
			assert.Equal(t, etg.StackTargetGroupId, atg.StackTargetGroupId)
			assert.Equal(t, etg.SvcImportTG, etg.SvcImportTG)
		}
	}
}
