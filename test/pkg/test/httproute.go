package test

import (
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func (env *Framework) NewHttpRoute(parentRefsGateway *gwv1.Gateway, service *corev1.Service, kind string) *gwv1.HTTPRoute {
	var rules []gwv1.HTTPRouteRule
	rule := gwv1.HTTPRouteRule{
		BackendRefs: []gwv1.HTTPBackendRef{{
			BackendRef: gwv1.BackendRef{
				BackendObjectReference: gwv1.BackendObjectReference{
					Name:      gwv1.ObjectName(service.Name),
					Namespace: (*gwv1.Namespace)(&service.Namespace),
					Kind:      lo.ToPtr(gwv1.Kind(kind)),
					Port:      (*gwv1.PortNumber)(&service.Spec.Ports[0].Port),
				},
			},
		}},
	}
	rules = append(rules, rule)
	parentNS := gwv1.Namespace(parentRefsGateway.Namespace)
	httpRoute := New(&gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: service.Namespace,
			Name:      service.Name,
		},
		Spec: gwv1.HTTPRouteSpec{
			CommonRouteSpec: gwv1.CommonRouteSpec{
				ParentRefs: []gwv1.ParentReference{{
					Name:      gwv1.ObjectName(parentRefsGateway.Name),
					Namespace: &parentNS,
				}},
			},
			Rules: rules,
		},
	})
	return httpRoute
}
