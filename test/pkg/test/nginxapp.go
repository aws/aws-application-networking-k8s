package test

import (
	"github.com/aws/aws-application-networking-k8s/pkg/deploy/lattice"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
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
	ReadinessGate       bool
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
	var readinessGates []v1.PodReadinessGate
	if options.ReadinessGate {
		readinessGates = append(readinessGates, v1.PodReadinessGate{
			ConditionType: lattice.LatticeReadinessGateConditionType,
		})
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
					ReadinessGates: readinessGates,
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
