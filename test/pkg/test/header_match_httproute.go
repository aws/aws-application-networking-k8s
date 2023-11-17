package test

import (
	"github.com/samber/lo"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func (env *Framework) NewHeaderMatchHttpRoute(parentRefsGateway *gwv1.Gateway, services []*v1.Service) *gwv1.HTTPRoute {
	var rules []gwv1.HTTPRouteRule
	for _, service := range services {
		rule := gwv1.HTTPRouteRule{
			BackendRefs: []gwv1.HTTPBackendRef{{
				BackendRef: gwv1.BackendRef{
					BackendObjectReference: gwv1.BackendObjectReference{
						Name: gwv1.ObjectName(service.Name),
						Kind: lo.ToPtr(gwv1.Kind("Service")),
					},
				},
			}},
			Matches: []gwv1.HTTPRouteMatch{
				{
					Headers: []gwv1.HTTPHeaderMatch{
						{
							Type:  lo.ToPtr(gwv1.HeaderMatchExact),
							Name:  "my-header-name1",
							Value: "my-header-value1",
						},
						{
							Type:  lo.ToPtr(gwv1.HeaderMatchExact),
							Name:  "my-header-name2",
							Value: "my-header-value2",
						},
					},
				},
			},
		}
		rules = append(rules, rule)
	}
	httpRoute := New(&gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: env.namespace,
		},
		Spec: gwv1.HTTPRouteSpec{
			CommonRouteSpec: gwv1.CommonRouteSpec{
				ParentRefs: []gwv1.ParentReference{{
					Name:        gwv1.ObjectName(parentRefsGateway.Name),
					SectionName: lo.ToPtr(gwv1.SectionName("http")),
				}},
			},
			Rules: rules,
		},
	})

	return httpRoute
}
