package test

import (
	"github.com/samber/lo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func (env *Framework) NewGRPCRoute(namespace string, parentRefsGateway *gwv1.Gateway, rules []v1alpha2.GRPCRouteRule) *v1alpha2.GRPCRoute {
	grpcRoute := New(&v1alpha2.GRPCRoute{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
		},
		Spec: v1alpha2.GRPCRouteSpec{
			CommonRouteSpec: v1alpha2.CommonRouteSpec{
				ParentRefs: []gwv1.ParentReference{{
					Name:        gwv1.ObjectName(parentRefsGateway.Name),
					Namespace:   (*gwv1.Namespace)(&parentRefsGateway.Namespace),
					SectionName: lo.ToPtr(gwv1.SectionName("https")),
				}},
			},
			Rules: rules,
		},
	})

	return grpcRoute
}
