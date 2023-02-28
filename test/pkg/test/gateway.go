package test

import (
	"sigs.k8s.io/gateway-api/apis/v1alpha2"
)

var Gateway = New(&v1alpha2.Gateway{
	Spec: v1alpha2.GatewaySpec{
		GatewayClassName: "amazon-vpc-lattice",
		Listeners: []v1alpha2.Listener{{
			Name:     "http",
			Protocol: v1alpha2.HTTPProtocolType,
			Port:     80,
		}},
	},
})
