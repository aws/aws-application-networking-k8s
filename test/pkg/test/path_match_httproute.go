package test

import (
	"strconv"

	"github.com/samber/lo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func (env *Framework) NewPathMatchHttpRoute(parentRefsGateway *gwv1.Gateway, backendRefObjects []client.Object,
	gwListenerSectionName string, name string, namespace string) *gwv1.HTTPRoute {
	var rules []gwv1.HTTPRouteRule
	var httpns *string
	if namespace == "" {
		httpns = nil

	} else {
		httpns = &namespace
	}
	for i, object := range backendRefObjects {
		rule := gwv1.HTTPRouteRule{
			BackendRefs: []gwv1.HTTPBackendRef{{
				BackendRef: gwv1.BackendRef{
					BackendObjectReference: gwv1.BackendObjectReference{
						Name:      gwv1.ObjectName(object.GetName()),
						Namespace: (*gwv1.Namespace)(httpns),
						Kind:      lo.ToPtr(gwv1.Kind(object.GetObjectKind().GroupVersionKind().Kind)),
					},
				},
			}},
			Matches: []gwv1.HTTPRouteMatch{
				{
					Path: &gwv1.HTTPPathMatch{
						Type:  lo.ToPtr(gwv1.PathMatchPathPrefix),
						Value: lo.ToPtr("/pathmatch" + strconv.Itoa(i)),
					},
				},
			},
		}
		rules = append(rules, rule)
	}

	httpRoute := New(&gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: gwv1.HTTPRouteSpec{
			CommonRouteSpec: gwv1.CommonRouteSpec{
				ParentRefs: []gwv1.ParentReference{{
					Name:        gwv1.ObjectName(parentRefsGateway.Name),
					Namespace:   (*gwv1.Namespace)(httpns),
					SectionName: lo.ToPtr(gwv1.SectionName(gwListenerSectionName)),
				}},
			},
			Rules: rules,
		},
	})
	return httpRoute
}
