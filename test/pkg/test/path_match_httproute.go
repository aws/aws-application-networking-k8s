package test

import (
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/samber/lo"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/gateway-api/apis/v1beta1"
	"strconv"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (env *Framework) NewPathMatchHttpRoute(parentRefsGateway *v1beta1.Gateway, backendRefObjects []client.Object,
	gwListenerSectionName string,name string, namespace string) *v1beta1.HTTPRoute {
	var rules []v1beta1.HTTPRouteRule
	var httpns *string
	if namespace == "" {
		httpns = nil

	} else {
		httpns = &namespace
	}
	for i, object := range backendRefObjects {
		rule := v1beta1.HTTPRouteRule{
			BackendRefs: []v1beta1.HTTPBackendRef{{
				BackendRef: v1beta1.BackendRef{
					BackendObjectReference: v1beta1.BackendObjectReference{
						Name: v1beta1.ObjectName(object.GetName()),
						Namespace: (*v1beta1.Namespace)(httpns),
						Kind: lo.ToPtr(v1beta1.Kind(object.GetObjectKind().GroupVersionKind().Kind)),
					},
				},
			}},
			Matches: []v1beta1.HTTPRouteMatch{
				{
					Path: &v1beta1.HTTPPathMatch{
						Type:  lo.ToPtr(v1beta1.PathMatchPathPrefix),
						Value: lo.ToPtr("/pathmatch" + strconv.Itoa(i)),
					},
				},
			},
		}
		rules = append(rules, rule)
	}
	
	httpRoute := New(&v1beta1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Namespace: namespace,

		},
		Spec: v1beta1.HTTPRouteSpec{
			CommonRouteSpec: v1beta1.CommonRouteSpec{
				ParentRefs: []v1beta1.ParentReference{{
					Name:        v1beta1.ObjectName(parentRefsGateway.Name),
					Namespace:(*v1beta1.Namespace)(httpns),
					SectionName: lo.ToPtr(v1beta1.SectionName(gwListenerSectionName)),
				}},
			},
			Rules: rules,
		},
	})
	env.TestCasesCreatedServiceNames[latticestore.AWSServiceName(httpRoute.Name, httpRoute.Namespace)] = true
	env.TestCasesCreatedK8sResource = append(env.TestCasesCreatedK8sResource, httpRoute)
	return httpRoute
}
