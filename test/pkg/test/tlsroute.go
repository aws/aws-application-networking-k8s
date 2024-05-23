package test

import (
	"github.com/samber/lo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func (env *Framework) NewTLSRoute(namespace string, parentRefsGateway *gwv1.Gateway, rules []v1alpha2.TLSRouteRule) *v1alpha2.TLSRoute {
	tlsRoute := New(&v1alpha2.TLSRoute{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
		},
		Spec: v1alpha2.TLSRouteSpec{
			CommonRouteSpec: v1alpha2.CommonRouteSpec{
				ParentRefs: []gwv1.ParentReference{{
					Name:        gwv1.ObjectName(parentRefsGateway.Name),
					Namespace:   (*gwv1.Namespace)(&parentRefsGateway.Namespace),
					SectionName: lo.ToPtr(gwv1.SectionName("tls")),
				}},
			},
			Hostnames: []gwv1.Hostname{"tls.test.com"},
			Rules:     rules,
		},
	})

	return tlsRoute
}
