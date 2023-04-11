package test

import (
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/samber/lo"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/gateway-api/apis/v1beta1"
	"strconv"
)

func (env *Framework) NewPathMatchHttpRoute(parentRefsGateway *v1beta1.Gateway, services []*v1.Service) *v1beta1.HTTPRoute {
	var rules []v1beta1.HTTPRouteRule
	for i, service := range services {
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
					Path: &v1beta1.HTTPPathMatch{
						Type:  lo.ToPtr(v1beta1.PathMatchPathPrefix),
						Value: lo.ToPtr("/pathmatch" + strconv.Itoa(i)),
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

	env.TestCasesCreatedServiceNames[latticestore.AWSServiceName(httpRoute.Name, httpRoute.Namespace)] = true
	return httpRoute
}
