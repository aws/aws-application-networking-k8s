package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func TestTLSRouteSpec_Equals(t *testing.T) {

	tests := []struct {
		routeSpec1  *TLSRouteSpec
		routeSpec2  RouteSpec
		expectEqual bool
		description string
	}{
		{
			description: "Empty instance are equal",
			routeSpec1:  &TLSRouteSpec{},
			routeSpec2:  &TLSRouteSpec{},
			expectEqual: true,
		},
		{
			description: "Instance populated with the same values are equal",
			routeSpec1: &TLSRouteSpec{
				s: gwv1alpha2.TLSRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{},
						},
					},
					Hostnames: []gwv1alpha2.Hostname{"example.com"},
					Rules: []gwv1alpha2.TLSRouteRule{
						{},
					},
				},
			},
			routeSpec2: &TLSRouteSpec{
				s: gwv1alpha2.TLSRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{},
						},
					},
					Hostnames: []gwv1alpha2.Hostname{"example.com"},
					Rules: []gwv1alpha2.TLSRouteRule{
						{},
					},
				},
			},
			expectEqual: true,
		},
		{
			description: "Instances of different types are not equal",
			routeSpec1:  &TLSRouteSpec{},
			routeSpec2:  &HTTPRouteSpec{},
			expectEqual: false,
		},
		{
			description: "Instance with different ParentRefs are not equal",
			routeSpec1: &TLSRouteSpec{
				s: gwv1alpha2.TLSRouteSpec{
					CommonRouteSpec: gwv1alpha2.CommonRouteSpec{
						ParentRefs: []gwv1alpha2.ParentReference{{Name: "parent1"}},
					},
				},
			},
			routeSpec2: &TLSRouteSpec{
				s: gwv1alpha2.TLSRouteSpec{
					CommonRouteSpec: gwv1alpha2.CommonRouteSpec{
						ParentRefs: []gwv1alpha2.ParentReference{{Name: "parent2"}},
					},
				},
			},
			expectEqual: false,
		},
		{
			description: "Instance with different Hostnames are not equal",
			routeSpec1: &TLSRouteSpec{
				s: gwv1alpha2.TLSRouteSpec{
					Hostnames: []gwv1alpha2.Hostname{"example1.com"},
				},
			},
			routeSpec2: &TLSRouteSpec{
				s: gwv1alpha2.TLSRouteSpec{
					Hostnames: []gwv1alpha2.Hostname{"example2.com"},
				},
			},
			expectEqual: false,
		},
		{
			routeSpec1:  &TLSRouteSpec{},
			routeSpec2:  nil,
			expectEqual: false,
			description: "Non-nil instances are not equal to nil",
		},
	}

	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			assert.Equal(t, test.expectEqual, test.routeSpec1.Equals(test.routeSpec2), test.description)
		})
	}
}

func TestTLSRouteBackendRef_Equals(t *testing.T) {
	weight1 := ptr.To(int32(1))
	weight2 := ptr.To(int32(2))
	group1 := gwv1alpha2.Group("group1")
	group2 := gwv1alpha2.Group("group2")
	kind1 := gwv1alpha2.Kind("kind1")
	kind2 := gwv1alpha2.Kind("kind2")
	name1 := gwv1alpha2.ObjectName("name1")
	name2 := gwv1alpha2.ObjectName("name2")
	namespace1 := gwv1alpha2.Namespace("namespace1")
	namespace2 := gwv1alpha2.Namespace("namespace2")
	port1 := gwv1alpha2.PortNumber(1)
	//port2 := gwv1alpha2.PortNumber(2)
	tests := []struct {
		description string
		backendRef1 *TLSBackendRef
		backendRef2 BackendRef
		expectEqual bool
	}{
		{
			description: "Empty instance are equal",
			backendRef1: &TLSBackendRef{},
			backendRef2: &TLSBackendRef{},
			expectEqual: true,
		},
		{
			description: "Instances populatd with the same values are equal",
			backendRef1: &TLSBackendRef{
				r: gwv1alpha2.BackendRef{
					Weight: weight1,
					BackendObjectReference: gwv1alpha2.BackendObjectReference{
						Group:     &group1,
						Kind:      &kind1,
						Name:      name1,
						Namespace: &namespace1,
						Port:      &port1,
					},
				},
			},

			backendRef2: &TLSBackendRef{
				r: gwv1alpha2.BackendRef{
					Weight: weight1,
					BackendObjectReference: gwv1alpha2.BackendObjectReference{
						Group:     &group1,
						Kind:      &kind1,
						Name:      name1,
						Namespace: &namespace1,
						Port:      &port1,
					},
				},
			},
			expectEqual: true,
		},
		{
			description: "Instance of different types are not equal",
			backendRef1: &TLSBackendRef{},
			backendRef2: &HTTPBackendRef{},
			expectEqual: false,
		},
		{
			description: "Instances with different weights are not equal",
			backendRef1: &TLSBackendRef{
				r: gwv1.BackendRef{
					Weight: weight1,
				},
			},
			backendRef2: &TLSBackendRef{
				r: gwv1.BackendRef{
					Weight: weight2,
				},
			},
			expectEqual: false,
		},
		{
			description: "Instances with different groups are not equal",
			backendRef1: &TLSBackendRef{
				r: gwv1alpha2.BackendRef{
					BackendObjectReference: gwv1alpha2.BackendObjectReference{
						Group: &group1,
					},
				},
			},
			backendRef2: &TLSBackendRef{
				r: gwv1alpha2.BackendRef{
					BackendObjectReference: gwv1alpha2.BackendObjectReference{
						Group: &group2,
					},
				},
			},
			expectEqual: false,
		},
		{
			description: "Instances with different kinds are not equal",
			backendRef1: &TLSBackendRef{
				r: gwv1alpha2.BackendRef{
					BackendObjectReference: gwv1alpha2.BackendObjectReference{
						Kind: &kind1,
					},
				},
			},
			backendRef2: &TLSBackendRef{
				r: gwv1alpha2.BackendRef{
					BackendObjectReference: gwv1alpha2.BackendObjectReference{
						Kind: &kind2,
					},
				},
			},
			expectEqual: false,
		},
		{
			description: "Instance with different Names are not equal",
			backendRef1: &TLSBackendRef{
				r: gwv1alpha2.BackendRef{
					BackendObjectReference: gwv1alpha2.BackendObjectReference{
						Name: name1,
					},
				},
			},
			backendRef2: &TLSBackendRef{
				r: gwv1alpha2.BackendRef{
					BackendObjectReference: gwv1alpha2.BackendObjectReference{
						Name: name2,
					},
				},
			},
			expectEqual: false,
		},
		{
			description: "Instance with different Namespaces are not equal",
			backendRef1: &TLSBackendRef{
				r: gwv1alpha2.BackendRef{
					BackendObjectReference: gwv1alpha2.BackendObjectReference{
						Name: gwv1.ObjectName(namespace1),
					},
				},
			},
			backendRef2: &TLSBackendRef{
				r: gwv1alpha2.BackendRef{
					BackendObjectReference: gwv1alpha2.BackendObjectReference{
						Name: gwv1.ObjectName(namespace2),
					},
				},
			},
			expectEqual: false,
		},
		{
			description: "non-nil instance are not equal to nil",
			backendRef1: &TLSBackendRef{},
			backendRef2: nil,
			expectEqual: false,
		},
	}

	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			assert.Equal(t, test.expectEqual, test.backendRef1.Equals(test.backendRef2), test.description)
		})
	}
}
