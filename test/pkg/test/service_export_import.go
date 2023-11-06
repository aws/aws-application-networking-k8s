package test

import (
	"strconv"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
)

func (env *Framework) CreateServiceExportAndServiceImportByService(service *v1.Service) (*anv1alpha1.ServiceExport, *anv1alpha1.ServiceImport) {
	serviceExport := New(&anv1alpha1.ServiceExport{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "application-networking.k8s.aws/v1alpha1",
			Kind:       "ServiceExport",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      service.Name,
			Namespace: service.Namespace,
			Annotations: map[string]string{
				"application-networking.k8s.aws/federation": "amazon-vpc-lattice",
			},
		},
	})
	serviceImport := New(&anv1alpha1.ServiceImport{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "application-networking.k8s.aws/v1alpha1",
			Kind:       "ServiceImport",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      service.Name,
			Namespace: service.Namespace,
		},
		Spec: anv1alpha1.ServiceImportSpec{
			Type: anv1alpha1.ClusterSetIP,
			Ports: []anv1alpha1.ServicePort{
				{
					Port:     80,
					Protocol: v1.Protocol("TCP"),
				},
			},
		},
	})
	return serviceExport, serviceImport
}

func (env *Framework) CreateServiceExport(service *v1.Service) *anv1alpha1.ServiceExport {
	serviceExport := New(&anv1alpha1.ServiceExport{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "application-networking.k8s.aws/v1alpha1",
			Kind:       "ServiceExport",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      service.Name,
			Namespace: service.Namespace,
			Annotations: map[string]string{
				"application-networking.k8s.aws/federation": "amazon-vpc-lattice",
				"application-networking.k8s.aws/port":       strconv.FormatInt(int64(service.Spec.Ports[0].Port), 10),
			},
		},
	})
	return serviceExport
}

func (env *Framework) CreateServiceImport(service *v1.Service) *anv1alpha1.ServiceImport {
	serviceImport := New(&anv1alpha1.ServiceImport{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "application-networking.k8s.aws/v1alpha1",
			Kind:       "ServiceImport",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      service.Name,
			Namespace: service.Namespace,
		},
		Spec: anv1alpha1.ServiceImportSpec{
			Type: anv1alpha1.ClusterSetIP,
			Ports: []anv1alpha1.ServicePort{
				{
					Port:     service.Spec.Ports[0].Port,
					Protocol: service.Spec.Ports[0].Protocol,
				},
			},
		},
	})
	return serviceImport
}
