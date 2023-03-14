package test

import (
	"sigs.k8s.io/gateway-api/apis/v1beta1"
)

var Gateway = New(&v1beta1.Gateway{
	Spec: v1beta1.GatewaySpec{
		GatewayClassName: "amazon-vpc-lattice",
		Listeners: []v1beta1.Listener{{
			Name:     "http",
			Protocol: v1beta1.HTTPProtocolType,
			Port:     80,
		}},
	},
})
