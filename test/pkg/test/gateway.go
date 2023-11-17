package test

import (
	"github.com/samber/lo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func (env *Framework) NewGateway(name string, namespace string) *gwv1.Gateway {
	gateway := New(
		&gwv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				Annotations: map[string]string{
					"application-networking.k8s.aws/lattice-vpc-association": "true",
				},
			},
			Spec: gwv1.GatewaySpec{
				GatewayClassName: "amazon-vpc-lattice",
				Listeners: []gwv1.Listener{
					{
						Name:     "http",
						Protocol: gwv1.HTTPProtocolType,
						Port:     80,
					},
					{
						Name:     "https",
						Protocol: gwv1.HTTPSProtocolType,
						Port:     443,
						TLS: &gwv1.GatewayTLSConfig{
							Mode: lo.ToPtr(gwv1.TLSModeTerminate),
							CertificateRefs: []gwv1.SecretObjectReference{
								{
									Name: "dummy",
								},
							},
						},
					},
				},
			},
			Status: gwv1.GatewayStatus{},
		},
	)
	return gateway
}
