package test

import (
	"github.com/samber/lo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/gateway-api/apis/v1alpha2"
	"sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
)

func (env *Framework) NewGRPCRoute(namespace string, parentRefsGateway *v1beta1.Gateway, rules []v1alpha2.GRPCRouteRule) *v1alpha2.GRPCRoute {
	grpcRoute := New(&v1alpha2.GRPCRoute{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
		},
		Spec: v1alpha2.GRPCRouteSpec{
			CommonRouteSpec: v1alpha2.CommonRouteSpec{
				ParentRefs: []v1beta1.ParentReference{{
					Name:        v1beta1.ObjectName(parentRefsGateway.Name),
					Namespace:   (*v1beta1.Namespace)(&parentRefsGateway.Namespace),
					SectionName: lo.ToPtr(v1beta1.SectionName("https")),
				}},
			},
			Rules: rules,
		},
	})

	env.TestCasesCreatedServiceNames[latticestore.LatticeServiceName(grpcRoute.Name, grpcRoute.Namespace)] = true
	env.TestCasesCreatedK8sResource = append(env.TestCasesCreatedK8sResource, grpcRoute)
	return grpcRoute
}
