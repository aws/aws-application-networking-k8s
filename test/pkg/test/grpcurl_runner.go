package test

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// https://github.com/fullstorydev/grpcurl
func NewGrpcurlRunnerPod() *v1.Pod {
	grpcurlPod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "grpcurl-runner",
			Labels: map[string]string{
				"app": "grpcurl-runner",
			},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name: "grpcurl-runner-container",
					// https://gallery.ecr.aws/a0j4q9e4/grpcurl-runner
					Image: "public.ecr.aws/a0j4q9e4/grpcurl-runner:latest",
					Command: []string{
						"/bin/sh",
						"-c",
						"while true; do sleep 5; done;",
					},
				},
			},
		},
	}
	return grpcurlPod
}
