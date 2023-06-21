package test

import (
	"github.com/samber/lo"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
)

func (env *Framework) NewHeaderMatchHttpRoute(parentRefsGateway *v1beta1.Gateway, services []*v1.Service) *v1beta1.HTTPRoute {
	var rules []v1beta1.HTTPRouteRule
	for _, service := range services {
		rule := v1beta1.HTTPRouteRule{
			BackendRefs: []v1beta1.HTTPBackendRef{{
				BackendRef: v1beta1.BackendRef{
					BackendObjectReference: v1beta1.BackendObjectReference{
						Name: v1beta1.ObjectName(service.Name),
						Kind: lo.ToPtr(v1beta1.Kind("Service")),
					},
				},
			}},
			Matches: []v1beta1.HTTPRouteMatch{
				{
					Headers: []v1beta1.HTTPHeaderMatch{
						{
							Type:  lo.ToPtr(v1beta1.HeaderMatchExact),
							Name:  "my-header-name1",
							Value: "my-header-value1",
						},
						{
							Type:  lo.ToPtr(v1beta1.HeaderMatchExact),
							Name:  "my-header-name2",
							Value: "my-header-value2",
						},
					},
				},
			},
		}
		rules = append(rules, rule)
	}
	httpRoute := New(&v1beta1.HTTPRoute{
		Spec: v1beta1.HTTPRouteSpec{
			CommonRouteSpec: v1beta1.CommonRouteSpec{
				ParentRefs: []v1beta1.ParentReference{{
					Name:        v1beta1.ObjectName(parentRefsGateway.Name),
					SectionName: lo.ToPtr(v1beta1.SectionName("http")),
				}},
			},
			Rules: rules,
		},
	})

	env.TestCasesCreatedServiceNames[latticestore.LatticeServiceName(httpRoute.Name, httpRoute.Namespace)] = true
	env.TestCasesCreatedK8sResource = append(env.TestCasesCreatedK8sResource, httpRoute)
	return httpRoute
}
