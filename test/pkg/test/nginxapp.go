package test

import (
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type ElasticSearchOptions struct {
	Name                string
	Namespace           string // the object will be created in this namespace
	Port                int
	TargetPort          int
	Port2               int
	TargetPort2         int
	MergeFromDeployment []*appsv1.Deployment
	MergeFromService    []*v1.Service
}

func (env *Framework) NewNginxApp(options ElasticSearchOptions) (*appsv1.Deployment, *v1.Service) {
	if options.Port == 0 {
		options.Port = 80
	}
	if options.Port2 == 0 {
		options.Port2 = 9114
	}
	if options.TargetPort == 0 {
		options.TargetPort = 80
	}
	if options.TargetPort2 == 0 {
		options.TargetPort2 = 9114
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
									Name:          "http",
									ContainerPort: int32(options.TargetPort),
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
									ContainerPort: int32(options.TargetPort2),
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
					Name:       options.Name,
					Protocol:   v1.ProtocolTCP,
					Port:       int32(options.Port),
					TargetPort: intstr.FromInt(options.TargetPort),
				},
				{
					Name:       "prometheus",
					Protocol:   v1.ProtocolTCP,
					Port:       int32(options.Port2),
					TargetPort: intstr.FromInt(options.TargetPort2),
				}},
		},
	}, options.MergeFromService...)
	return deployment, service

}

func (env *Framework) NewHttpRoute(parentRefsGateway *gwv1.Gateway, service *v1.Service, kind string) *gwv1.HTTPRoute {
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
	httpRoute := New(&gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: service.Namespace,
			Name:      service.Name,
		},
		Spec: gwv1.HTTPRouteSpec{
			CommonRouteSpec: gwv1.CommonRouteSpec{
				ParentRefs: []gwv1.ParentReference{{
					Name: gwv1.ObjectName(parentRefsGateway.Name),
				}},
			},
			Rules: rules,
		},
	})
	return httpRoute
}
