package test

import (
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type GrpcAppOptions struct {
	AppName   string
	Namespace string
}

// https://github.com/moul/grpcbin
func (env *Framework) NewGrpcBin(options GrpcAppOptions) (*appsv1.Deployment, *v1.Service) {

	deployment := New(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: options.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: lo.ToPtr(int32(1)),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": options.AppName,
				},
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: options.Namespace,
					Labels: map[string]string{
						"app":          options.AppName,
						DiscoveryLabel: "true",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{{
						Name:  options.AppName,
						Image: "moul/grpcbin:latest",
					}},
				},
			},
		},
	})

	service := New(&v1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind: "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: options.Namespace,
		},
		Spec: v1.ServiceSpec{
			Selector: map[string]string{
				"app": options.AppName,
			},
			Ports: []v1.ServicePort{
				{
					Name:       "grpcbin-over-http",
					Protocol:   v1.ProtocolTCP,
					Port:       int32(19000),
					TargetPort: intstr.FromInt(9000),
				},
				{
					Name:       "grpcbin-over-https",
					Protocol:   v1.ProtocolTCP,
					Port:       int32(19001),
					TargetPort: intstr.FromInt(9001),
				},
			},
		},
	})
	return deployment, service
}
