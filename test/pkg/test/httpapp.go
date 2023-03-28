package test

import (
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type HTTPAppOptions struct {
	Name                string
	Port                int
	TargetPort          int
	MergeFromDeployment []*appsv1.Deployment
	MergeFromService    []*v1.Service
}

func HTTPApp(options HTTPAppOptions) (*appsv1.Deployment, *v1.Service) {
	if options.Port == 0 {
		options.Port = 80
	}
	if options.TargetPort == 0 {
		options.TargetPort = 8090
	}
	return New(&appsv1.Deployment{
			Spec: appsv1.DeploymentSpec{
				Replicas: lo.ToPtr(int32(1)),
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app": options.Name,
					},
				},
				Template: v1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"app": options.Name,
						},
					},
					Spec: v1.PodSpec{
						Containers: []v1.Container{{
							Name:  options.Name,
							Image: "public.ecr.aws/x2j8p8w7/http-server:latest",
							Env: []v1.EnvVar{{
								Name:  "PodName",
								Value: options.Name,
							}},
						}},
					},
				},
			},
		}, options.MergeFromDeployment...),
		New(&v1.Service{
			Spec: v1.ServiceSpec{
				Selector: map[string]string{
					"app": options.Name,
				},
				Ports: []v1.ServicePort{{
					Protocol:   v1.ProtocolTCP,
					Port:       int32(options.Port),
					TargetPort: intstr.FromInt(options.TargetPort),
				}},
			},
		}, options.MergeFromService...)
}
