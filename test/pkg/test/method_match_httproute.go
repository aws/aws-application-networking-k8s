package test

import (
	"github.com/samber/lo"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/gateway-api/apis/v1beta1"
)

// creates a route sending GET to getService and POST to postService
func (env *Framework) NewMethodMatchHttpRoute(parentRefsGateway *v1beta1.Gateway, getService *v1.Service, postService *v1.Service,
	httpRouteName string, namespace string) *v1beta1.HTTPRoute {
	getRule := v1beta1.HTTPRouteRule{
		BackendRefs: []v1beta1.HTTPBackendRef{{
			BackendRef: v1beta1.BackendRef{
				BackendObjectReference: v1beta1.BackendObjectReference{
					Name: v1beta1.ObjectName(getService.Name),
					Kind: lo.ToPtr(v1beta1.Kind("Service")),
				},
			},
		}},
		Matches: []v1beta1.HTTPRouteMatch{
			{
				Method: lo.ToPtr(v1beta1.HTTPMethodGet),
			},
		},
	}

	postRule := v1beta1.HTTPRouteRule{
		BackendRefs: []v1beta1.HTTPBackendRef{{
			BackendRef: v1beta1.BackendRef{
				BackendObjectReference: v1beta1.BackendObjectReference{
					Name: v1beta1.ObjectName(postService.Name),
					Kind: lo.ToPtr(v1beta1.Kind("Service")),
				},
			},
		}},
		Matches: []v1beta1.HTTPRouteMatch{
			{
				Method: lo.ToPtr(v1beta1.HTTPMethodPost),
			},
		},
	}

	httpRoute := New(&v1beta1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      httpRouteName,
			Namespace: namespace,
		},
		Spec: v1beta1.HTTPRouteSpec{
			CommonRouteSpec: v1beta1.CommonRouteSpec{
				ParentRefs: []v1beta1.ParentReference{{
					Name:        v1beta1.ObjectName(parentRefsGateway.Name),
					SectionName: lo.ToPtr(v1beta1.SectionName("http")),
				}},
			},
			Rules: []v1beta1.HTTPRouteRule{getRule, postRule},
		},
	})

	return httpRoute
}
