package test

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

func (env *Framework) GetServiceExportAndServiceImportByService(service *v1.Service) (*v1alpha1.ServiceExport, *v1alpha1.ServiceImport) {
	serviceExport := New(&v1alpha1.ServiceExport{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "multicluster.x-k8s.io/v1alpha1",
			Kind:       "ServiceExport",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: service.Name,
			Annotations: map[string]string{
				"multicluster.x-k8s.io/federation": "amazon-vpc-lattice",
			},
		},
	})
	serviceImport := New(&v1alpha1.ServiceImport{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "multicluster.x-k8s.io/v1alpha1",
			Kind:       "ServiceImport",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: service.Name,
		},
		Spec: v1alpha1.ServiceImportSpec{
			Type: v1alpha1.ClusterSetIP,
			Ports: []v1alpha1.ServicePort{
				{
					Port:     80,
					Protocol: v1.Protocol("TCP"),
				},
			},
		},
	})
	env.TestCasesCreatedK8sResource = append(env.TestCasesCreatedK8sResource, serviceExport, serviceImport)
	return serviceExport, serviceImport
}
