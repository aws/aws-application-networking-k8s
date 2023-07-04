package test

import (
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/gateway-api/apis/v1beta1"
)

type ElasticSearchOptions struct {
	Name                string
	Namespace           string // the object will be created in this namespace
	Port                int
	Port2               int
	TargetPort          int
	MergeFromDeployment []*appsv1.Deployment
	MergeFromService    []*v1.Service
}

func (env *Framework) NewNginxeApp(options ElasticSearchOptions) (*appsv1.Deployment, *v1.Service) {
	if options.Port == 0 {
		options.Port = 80
	}
	if options.Port2 == 0 {
		options.Port2 = 9114
	}
	if options.TargetPort == 0 {
		options.TargetPort = 80
	}
	deployment := New(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: options.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: lo.ToPtr(int32(2)),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": options.Name,
				},
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: options.Namespace,
					Labels: map[string]string{
						"app":          options.Name,
						DiscoveryLabel: "true",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  options.Name,
							Image: "nginx",
							Ports: []v1.ContainerPort{
								{
									Name:          options.Name,
									ContainerPort: int32(options.Port),
									Protocol:      "TCP",
								},
							},
							Env: []v1.EnvVar{{
								Name:  "PodName",
								Value: options.Name + " handler pod",
							}},
						},
						{
							Name:  "prometheus-exporter",
							Image: "justwatch/elasticsearch_exporter:1.1.0",
							Ports: []v1.ContainerPort{
								{
									Name:          "http-prometheus",
									ContainerPort: int32(options.Port2),
									Protocol:      "TCP",
								},
							},
							Env: []v1.EnvVar{{
								Name:  "PodName",
								Value: options.Name + " handler pod",
							}},
						},
					},
				},
			},
		},
	}, options.MergeFromDeployment...)

	service := New(&v1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind: "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: options.Namespace,
		},
		Spec: v1.ServiceSpec{
			Selector: map[string]string{
				"app": options.Name,
			},
			Ports: []v1.ServicePort{
				{
					Name:     options.Name,
					Protocol: v1.ProtocolTCP,
					Port:     int32(options.Port),
				},
				{
					Name:     "http-prometheus",
					Protocol: v1.ProtocolTCP,
					Port:     int32(options.Port2),
				}},
		},
	}, options.MergeFromService...)
	env.TestCasesCreatedTargetGroupNames[latticestore.TargetGroupName(service.Name, service.Namespace)] = true
	env.TestCasesCreatedK8sResource = append(env.TestCasesCreatedK8sResource, service, deployment)
	return deployment, service

}

func (env *Framework) NewHttpRoute(parentRefsGateway *v1beta1.Gateway, service *v1.Service, kind string) *v1beta1.HTTPRoute {
	var rules []v1beta1.HTTPRouteRule
	rule := v1beta1.HTTPRouteRule{
		BackendRefs: []v1beta1.HTTPBackendRef{{
			BackendRef: v1beta1.BackendRef{
				BackendObjectReference: v1beta1.BackendObjectReference{
					Name:      v1beta1.ObjectName(service.Name),
					Namespace: (*v1beta1.Namespace)(&service.Namespace),
					Kind:      lo.ToPtr(v1beta1.Kind(kind)),
					Port:      (*v1beta1.PortNumber)(&service.Spec.Ports[0].Port),
				},
			},
		}},
	}
	rules = append(rules, rule)
	httpRoute := New(&v1beta1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: service.Namespace,
		},
		Spec: v1beta1.HTTPRouteSpec{
			CommonRouteSpec: v1beta1.CommonRouteSpec{
				ParentRefs: []v1beta1.ParentReference{{
					Name: v1beta1.ObjectName(parentRefsGateway.Name),
					//SectionName: lo.ToPtr(v1beta1.SectionName("http")),
				}},
			},
			Rules: rules,
		},
	})
	env.TestCasesCreatedServiceNames[latticestore.LatticeServiceName(httpRoute.Name, httpRoute.Namespace)] = true
	env.TestCasesCreatedK8sResource = append(env.TestCasesCreatedK8sResource, httpRoute)
	return httpRoute
}
