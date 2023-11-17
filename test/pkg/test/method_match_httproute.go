package test

import (
	"github.com/samber/lo"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// creates a route sending GET to getService and POST to postService
func (env *Framework) NewMethodMatchHttpRoute(parentRefsGateway *gwv1.Gateway, getService *v1.Service, postService *v1.Service,
	httpRouteName string, namespace string) *gwv1.HTTPRoute {
	getRule := gwv1.HTTPRouteRule{
		BackendRefs: []gwv1.HTTPBackendRef{{
			BackendRef: gwv1.BackendRef{
				BackendObjectReference: gwv1.BackendObjectReference{
					Name: gwv1.ObjectName(getService.Name),
					Kind: lo.ToPtr(gwv1.Kind("Service")),
					Port: (*gwv1.PortNumber)(&postService.Spec.Ports[0].Port),
				},
			},
		}},
		Matches: []gwv1.HTTPRouteMatch{
			{
				Method: lo.ToPtr(gwv1.HTTPMethodGet),
			},
		},
	}

	postRule := gwv1.HTTPRouteRule{
		BackendRefs: []gwv1.HTTPBackendRef{{
			BackendRef: gwv1.BackendRef{
				BackendObjectReference: gwv1.BackendObjectReference{
					Name: gwv1.ObjectName(postService.Name),
					Kind: lo.ToPtr(gwv1.Kind("Service")),
					Port: (*gwv1.PortNumber)(&postService.Spec.Ports[0].Port),
				},
			},
		}},
		Matches: []gwv1.HTTPRouteMatch{
			{
				Method: lo.ToPtr(gwv1.HTTPMethodPost),
			},
		},
	}

	httpRoute := New(&gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      httpRouteName,
			Namespace: namespace,
		},
		Spec: gwv1.HTTPRouteSpec{
			CommonRouteSpec: gwv1.CommonRouteSpec{
				ParentRefs: []gwv1.ParentReference{{
					Name:        gwv1.ObjectName(parentRefsGateway.Name),
					SectionName: lo.ToPtr(gwv1.SectionName("http")),
				}},
			},
			Rules: []gwv1.HTTPRouteRule{getRule, postRule},
		},
	})

	return httpRoute
}
