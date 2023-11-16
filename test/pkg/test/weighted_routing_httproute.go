package test

import (
	"github.com/samber/lo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type ObjectAndWeight struct {
	Object client.Object
	Weight int32
}

func (env *Framework) NewWeightedRoutingHttpRoute(parentRefsGateway *gwv1.Gateway, backendRefObjectAndWeights []*ObjectAndWeight,
	gwListenerSectionNames []string) *gwv1.HTTPRoute {

	var backendRefs []gwv1.HTTPBackendRef
	for _, objectAndWeight := range backendRefObjectAndWeights {
		backendRefs = append(backendRefs, gwv1.HTTPBackendRef{
			BackendRef: gwv1.BackendRef{
				BackendObjectReference: gwv1.BackendObjectReference{
					Name: gwv1.ObjectName(objectAndWeight.Object.GetName()),
					Kind: lo.ToPtr(gwv1.Kind(objectAndWeight.Object.GetObjectKind().GroupVersionKind().Kind)),
				},
				Weight: lo.ToPtr(objectAndWeight.Weight),
			},
		})
	}
	var parentRefs []gwv1.ParentReference
	for _, gwListenerSectionName := range gwListenerSectionNames {
		parentRefs = append(parentRefs, gwv1.ParentReference{
			Name:        gwv1.ObjectName(parentRefsGateway.Name),
			SectionName: lo.ToPtr(gwv1.SectionName(gwListenerSectionName)),
		})
	}
	httpRoute := New(&gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: parentRefsGateway.Namespace,
		},
		Spec: gwv1.HTTPRouteSpec{
			CommonRouteSpec: gwv1.CommonRouteSpec{
				ParentRefs: parentRefs,
			},
			Rules: []gwv1.HTTPRouteRule{
				{
					BackendRefs: backendRefs,
				},
			},
		},
	})
	return httpRoute
}
