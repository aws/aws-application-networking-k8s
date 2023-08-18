package test

import (
	"github.com/samber/lo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
)

type ObjectAndWeight struct {
	Object client.Object
	Weight int32
}

func (env *Framework) NewWeightedRoutingHttpRoute(parentRefsGateway *v1beta1.Gateway, backendRefObjectAndWeights []*ObjectAndWeight,
	gwListenerSectionNames []string) *v1beta1.HTTPRoute {

	var backendRefs []v1beta1.HTTPBackendRef
	for _, objectAndWeight := range backendRefObjectAndWeights {
		backendRefs = append(backendRefs, v1beta1.HTTPBackendRef{
			BackendRef: v1beta1.BackendRef{
				BackendObjectReference: v1beta1.BackendObjectReference{
					Name: v1beta1.ObjectName(objectAndWeight.Object.GetName()),
					Kind: lo.ToPtr(v1beta1.Kind(objectAndWeight.Object.GetObjectKind().GroupVersionKind().Kind)),
				},
				Weight: lo.ToPtr(objectAndWeight.Weight),
			},
		})
	}
	var parentRefs []v1beta1.ParentReference
	for _, gwListenerSectionName := range gwListenerSectionNames {
		parentRefs = append(parentRefs, v1beta1.ParentReference{
			Name:        v1beta1.ObjectName(parentRefsGateway.Name),
			SectionName: lo.ToPtr(v1beta1.SectionName(gwListenerSectionName)),
		})
	}
	httpRoute := New(&v1beta1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: parentRefsGateway.Namespace,
		},
		Spec: v1beta1.HTTPRouteSpec{
			CommonRouteSpec: v1beta1.CommonRouteSpec{
				ParentRefs: parentRefs,
			},
			Rules: []v1beta1.HTTPRouteRule{
				{
					BackendRefs: backendRefs,
				},
			},
		},
	})
	env.TestCasesCreatedServiceNames[latticestore.LatticeServiceName(httpRoute.Name, httpRoute.Namespace)] = true
	env.TestCasesCreatedK8sResource = append(env.TestCasesCreatedK8sResource, httpRoute)
	return httpRoute
}
