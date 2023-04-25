package test

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/gateway-api/apis/v1beta1"
)

func (env *Framework) NewGateway() *v1beta1.Gateway {
	gateway := New(
		&v1beta1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					"application-networking.k8s.aws/lattice-vpc-association": "true",
				},
			},
			Spec: v1beta1.GatewaySpec{
				GatewayClassName: "amazon-vpc-lattice",
				Listeners: []v1beta1.Listener{
					{
						Name:     "http",
						Protocol: v1beta1.HTTPProtocolType,
						Port:     80,
					},
					{
						Name:     "https",
						Protocol: v1beta1.HTTPSProtocolType,
						Port:     443,
					},
				},
			},
			Status: v1beta1.GatewayStatus{},
		},
	)
	env.TestCasesCreatedServiceNetworkNames[gateway.Name] = true
	env.TestCasesCreatedK8sResource = append(env.TestCasesCreatedK8sResource, gateway)
	return gateway
}
