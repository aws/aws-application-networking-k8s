package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/utils/pointer"
	gateway_api_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gateway_api_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

func TestGRPCRouteSpec_Equals(t *testing.T) {
	name1 := gateway_api_v1alpha2.ObjectName("name1")
	name2 := gateway_api_v1alpha2.ObjectName("name2")

	tests := []struct {
		routeSpec1  *GRPCRouteSpec
		routeSpec2  RouteSpec
		expectEqual bool
		description string
	}{
		{
			routeSpec1:  &GRPCRouteSpec{},
			routeSpec2:  &GRPCRouteSpec{},
			expectEqual: true,
			description: "Empty instances are equal",
		},
		{
			routeSpec1: &GRPCRouteSpec{
				s: gateway_api_v1alpha2.GRPCRouteSpec{
					CommonRouteSpec: gateway_api_v1alpha2.CommonRouteSpec{
						ParentRefs: []gateway_api_v1alpha2.ParentReference{
							{},
						},
					},
					Hostnames: []gateway_api_v1alpha2.Hostname{"example.com"},
					Rules: []gateway_api_v1alpha2.GRPCRouteRule{
						{},
					},
				},
			},
			routeSpec2: &GRPCRouteSpec{
				s: gateway_api_v1alpha2.GRPCRouteSpec{
					CommonRouteSpec: gateway_api_v1alpha2.CommonRouteSpec{
						ParentRefs: []gateway_api_v1alpha2.ParentReference{
							{},
						},
					},
					Hostnames: []gateway_api_v1alpha2.Hostname{"example.com"},
					Rules: []gateway_api_v1alpha2.GRPCRouteRule{
						{},
					},
				},
			},
			expectEqual: true,
			description: "Instances populated with the same values are equal",
		},
		{
			routeSpec1:  &GRPCRouteSpec{},
			routeSpec2:  &HTTPRouteSpec{},
			expectEqual: false,
			description: "Instances of different types are not equal",
		},
		{
			routeSpec1: &GRPCRouteSpec{
				s: gateway_api_v1alpha2.GRPCRouteSpec{
					CommonRouteSpec: gateway_api_v1alpha2.CommonRouteSpec{
						ParentRefs: []gateway_api_v1alpha2.ParentReference{{Name: "parent1"}},
					},
				},
			},
			routeSpec2: &GRPCRouteSpec{
				s: gateway_api_v1alpha2.GRPCRouteSpec{
					CommonRouteSpec: gateway_api_v1alpha2.CommonRouteSpec{
						ParentRefs: []gateway_api_v1alpha2.ParentReference{{Name: "parent2"}},
					},
				},
			},
			expectEqual: false,
			description: "Instances with different ParentRefs are not equal",
		},
		{
			routeSpec1: &GRPCRouteSpec{
				s: gateway_api_v1alpha2.GRPCRouteSpec{
					Hostnames: []gateway_api_v1alpha2.Hostname{"example1.com"},
				},
			},
			routeSpec2: &GRPCRouteSpec{
				s: gateway_api_v1alpha2.GRPCRouteSpec{
					Hostnames: []gateway_api_v1alpha2.Hostname{"example2.com"},
				},
			},
			expectEqual: false,
			description: "Instances with different HostNames are not equal",
		},
		{
			routeSpec1: &GRPCRouteSpec{
				s: gateway_api_v1alpha2.GRPCRouteSpec{
					Rules: []gateway_api_v1alpha2.GRPCRouteRule{
						{},
						{},
					},
				},
			},
			routeSpec2: &GRPCRouteSpec{
				s: gateway_api_v1alpha2.GRPCRouteSpec{
					Rules: []gateway_api_v1alpha2.GRPCRouteRule{
						{},
					},
				},
			},
			expectEqual: false,
			description: "Instances with different Rules lengths are not equal",
		},
		{
			routeSpec1: &GRPCRouteSpec{
				s: gateway_api_v1alpha2.GRPCRouteSpec{
					Rules: []gateway_api_v1alpha2.GRPCRouteRule{
						{
							BackendRefs: []gateway_api_v1alpha2.GRPCBackendRef{
								{
									BackendRef: gateway_api_v1alpha2.BackendRef{
										BackendObjectReference: gateway_api_v1alpha2.BackendObjectReference{
											Name: name1,
										},
									},
								},
							},
						},
					},
				},
			},
			routeSpec2: &GRPCRouteSpec{
				s: gateway_api_v1alpha2.GRPCRouteSpec{
					Rules: []gateway_api_v1alpha2.GRPCRouteRule{
						{
							BackendRefs: []gateway_api_v1alpha2.GRPCBackendRef{
								{
									BackendRef: gateway_api_v1alpha2.BackendRef{
										BackendObjectReference: gateway_api_v1alpha2.BackendObjectReference{
											Name: name2,
										},
									},
								},
							},
						},
					},
				},
			},
			expectEqual: false,
			description: "Instances populated with different Rule values are not equal",
		},
		{
			routeSpec1:  &GRPCRouteSpec{},
			routeSpec2:  nil,
			expectEqual: false,
			description: "Non-nil instances are not equal to nil",
		},
	}

	for _, test := range tests {
		assert.Equal(t, test.expectEqual, test.routeSpec1.Equals(test.routeSpec2), test.description)
	}
}

func TestGRPCRouteRule_Equals(t *testing.T) {
	grpcMethodMatchType1 := gateway_api_v1alpha2.GRPCMethodMatchExact
	grpcMethodMatchType2 := gateway_api_v1alpha2.GRPCMethodMatchRegularExpression

	tests := []struct {
		routeRule1  *GRPCRouteRule
		routeRule2  RouteRule
		expectEqual bool
		description string
	}{
		{
			routeRule1:  &GRPCRouteRule{},
			routeRule2:  &GRPCRouteRule{},
			expectEqual: true,
			description: "Empty instances are equal",
		},
		{
			routeRule1: &GRPCRouteRule{
				r: gateway_api_v1alpha2.GRPCRouteRule{
					BackendRefs: []gateway_api_v1alpha2.GRPCBackendRef{
						{},
					},
					Matches: []gateway_api_v1alpha2.GRPCRouteMatch{
						{},
					},
				},
			},
			routeRule2: &GRPCRouteRule{
				r: gateway_api_v1alpha2.GRPCRouteRule{
					BackendRefs: []gateway_api_v1alpha2.GRPCBackendRef{
						{},
					},
					Matches: []gateway_api_v1alpha2.GRPCRouteMatch{
						{},
					},
				},
			},
			expectEqual: true,
			description: "Instances populated with the same values are equal",
		},
		{
			routeRule1:  &GRPCRouteRule{},
			routeRule2:  &HTTPRouteRule{},
			expectEqual: false,
			description: "Instances of different types are not equal",
		},
		{
			routeRule1: &GRPCRouteRule{
				r: gateway_api_v1alpha2.GRPCRouteRule{
					BackendRefs: []gateway_api_v1alpha2.GRPCBackendRef{
						{},
						{},
					},
				},
			},
			routeRule2: &GRPCRouteRule{
				r: gateway_api_v1alpha2.GRPCRouteRule{
					BackendRefs: []gateway_api_v1alpha2.GRPCBackendRef{
						{},
					},
				},
			},
			expectEqual: false,
			description: "Instances with different BackendRefs lengths are not equal",
		},
		{
			routeRule1: &GRPCRouteRule{
				r: gateway_api_v1alpha2.GRPCRouteRule{
					BackendRefs: []gateway_api_v1alpha2.GRPCBackendRef{
						{
							BackendRef: gateway_api_v1alpha2.BackendRef{
								Weight: pointer.Int32(1),
							},
						},
					},
				},
			},
			routeRule2: &GRPCRouteRule{
				r: gateway_api_v1alpha2.GRPCRouteRule{
					BackendRefs: []gateway_api_v1alpha2.GRPCBackendRef{
						{
							BackendRef: gateway_api_v1alpha2.BackendRef{
								Weight: pointer.Int32(2),
							},
						},
					},
				},
			},
			expectEqual: false,
			description: "Instances with different BackendRef values are not equal",
		},
		{
			routeRule1: &GRPCRouteRule{
				r: gateway_api_v1alpha2.GRPCRouteRule{
					Matches: []gateway_api_v1alpha2.GRPCRouteMatch{
						{},
						{},
					},
				},
			},
			routeRule2: &GRPCRouteRule{
				r: gateway_api_v1alpha2.GRPCRouteRule{
					Matches: []gateway_api_v1alpha2.GRPCRouteMatch{
						{},
					},
				},
			},
			expectEqual: false,
			description: "Instances with different Matches lengths are not equal",
		},
		{
			routeRule1: &GRPCRouteRule{
				r: gateway_api_v1alpha2.GRPCRouteRule{
					Matches: []gateway_api_v1alpha2.GRPCRouteMatch{
						{
							Method: &gateway_api_v1alpha2.GRPCMethodMatch{
								Type: &grpcMethodMatchType1,
							},
						},
					},
				},
			},
			routeRule2: &GRPCRouteRule{
				r: gateway_api_v1alpha2.GRPCRouteRule{
					Matches: []gateway_api_v1alpha2.GRPCRouteMatch{
						{
							Method: &gateway_api_v1alpha2.GRPCMethodMatch{
								Type: &grpcMethodMatchType2,
							},
						},
					},
				},
			},
			expectEqual: false,
			description: "Instances with different Match values are not equal",
		},
		{
			routeRule1:  &GRPCRouteRule{},
			routeRule2:  nil,
			expectEqual: false,
			description: "Non-nil instances are not equal to nil",
		},
	}

	for _, test := range tests {
		assert.Equal(t, test.expectEqual, test.routeRule1.Equals(test.routeRule2), test.description)
	}
}

func TestGRPCBackendRef_Equals(t *testing.T) {
	weight1 := pointer.Int32(1)
	weight2 := pointer.Int32(2)
	group1 := gateway_api_v1alpha2.Group("group1")
	group2 := gateway_api_v1alpha2.Group("group2")
	kind1 := gateway_api_v1alpha2.Kind("kind1")
	kind2 := gateway_api_v1alpha2.Kind("kind2")
	name1 := gateway_api_v1alpha2.ObjectName("name1")
	name2 := gateway_api_v1alpha2.ObjectName("name2")
	namespace1 := gateway_api_v1alpha2.Namespace("namespace1")
	namespace2 := gateway_api_v1alpha2.Namespace("namespace2")
	port1 := gateway_api_v1alpha2.PortNumber(1)
	port2 := gateway_api_v1alpha2.PortNumber(2)

	tests := []struct {
		backendRef1 *GRPCBackendRef
		backendRef2 BackendRef
		expectEqual bool
		description string
	}{
		{
			backendRef1: &GRPCBackendRef{},
			backendRef2: &GRPCBackendRef{},
			expectEqual: true,
			description: "Empty instances are equal",
		},
		{
			backendRef1: &GRPCBackendRef{
				r: gateway_api_v1alpha2.GRPCBackendRef{
					BackendRef: gateway_api_v1alpha2.BackendRef{
						Weight: weight1,
						BackendObjectReference: gateway_api_v1alpha2.BackendObjectReference{
							Group:     &group1,
							Kind:      &kind1,
							Name:      name1,
							Namespace: &namespace1,
							Port:      &port1,
						},
					},
				},
			},
			backendRef2: &GRPCBackendRef{
				r: gateway_api_v1alpha2.GRPCBackendRef{
					BackendRef: gateway_api_v1alpha2.BackendRef{
						Weight: weight1,
						BackendObjectReference: gateway_api_v1alpha2.BackendObjectReference{
							Group:     &group1,
							Kind:      &kind1,
							Name:      name1,
							Namespace: &namespace1,
							Port:      &port1,
						},
					},
				},
			},
			expectEqual: true,
			description: "Instances populated with the same values are equal",
		},
		{
			backendRef1: &GRPCBackendRef{},
			backendRef2: &HTTPBackendRef{},
			expectEqual: false,
			description: "Instances of different types are not equal",
		},
		{
			backendRef1: &GRPCBackendRef{
				r: gateway_api_v1alpha2.GRPCBackendRef{
					BackendRef: gateway_api_v1alpha2.BackendRef{
						Weight: weight1,
					},
				},
			},
			backendRef2: &GRPCBackendRef{
				r: gateway_api_v1alpha2.GRPCBackendRef{
					BackendRef: gateway_api_v1alpha2.BackendRef{
						Weight: weight2,
					},
				},
			},
			expectEqual: false,
			description: "Instances with different Weights are not equal",
		},
		{
			backendRef1: &GRPCBackendRef{
				r: gateway_api_v1alpha2.GRPCBackendRef{
					BackendRef: gateway_api_v1alpha2.BackendRef{
						BackendObjectReference: gateway_api_v1alpha2.BackendObjectReference{
							Group: &group1,
						},
					},
				},
			},
			backendRef2: &GRPCBackendRef{
				r: gateway_api_v1alpha2.GRPCBackendRef{
					BackendRef: gateway_api_v1alpha2.BackendRef{
						BackendObjectReference: gateway_api_v1alpha2.BackendObjectReference{
							Group: &group2,
						},
					},
				},
			},
			expectEqual: false,
			description: "Instances with different Groups are not equal",
		},
		{
			backendRef1: &GRPCBackendRef{
				r: gateway_api_v1alpha2.GRPCBackendRef{
					BackendRef: gateway_api_v1alpha2.BackendRef{
						BackendObjectReference: gateway_api_v1alpha2.BackendObjectReference{
							Kind: &kind1,
						},
					},
				},
			},
			backendRef2: &GRPCBackendRef{
				r: gateway_api_v1alpha2.GRPCBackendRef{
					BackendRef: gateway_api_v1alpha2.BackendRef{
						BackendObjectReference: gateway_api_v1alpha2.BackendObjectReference{
							Kind: &kind2,
						},
					},
				},
			},
			expectEqual: false,
			description: "Instances with different Kinds are not equal",
		},
		{
			backendRef1: &GRPCBackendRef{
				r: gateway_api_v1alpha2.GRPCBackendRef{
					BackendRef: gateway_api_v1alpha2.BackendRef{
						BackendObjectReference: gateway_api_v1alpha2.BackendObjectReference{
							Name: name1,
						},
					},
				},
			},
			backendRef2: &GRPCBackendRef{
				r: gateway_api_v1alpha2.GRPCBackendRef{
					BackendRef: gateway_api_v1alpha2.BackendRef{
						BackendObjectReference: gateway_api_v1alpha2.BackendObjectReference{
							Name: name2,
						},
					},
				},
			},
			expectEqual: false,
			description: "Instances with different Names are not equal",
		},
		{
			backendRef1: &GRPCBackendRef{
				r: gateway_api_v1alpha2.GRPCBackendRef{
					BackendRef: gateway_api_v1alpha2.BackendRef{
						BackendObjectReference: gateway_api_v1alpha2.BackendObjectReference{
							Namespace: &namespace1,
						},
					},
				},
			},
			backendRef2: &GRPCBackendRef{
				r: gateway_api_v1alpha2.GRPCBackendRef{
					BackendRef: gateway_api_v1alpha2.BackendRef{
						BackendObjectReference: gateway_api_v1alpha2.BackendObjectReference{
							Namespace: &namespace2,
						},
					},
				},
			},
			expectEqual: false,
			description: "Instances with different Namespaces are not equal",
		},
		{
			backendRef1: &GRPCBackendRef{
				r: gateway_api_v1alpha2.GRPCBackendRef{
					BackendRef: gateway_api_v1alpha2.BackendRef{
						BackendObjectReference: gateway_api_v1alpha2.BackendObjectReference{
							Port: &port1,
						},
					},
				},
			},
			backendRef2: &GRPCBackendRef{
				r: gateway_api_v1alpha2.GRPCBackendRef{
					BackendRef: gateway_api_v1alpha2.BackendRef{
						BackendObjectReference: gateway_api_v1alpha2.BackendObjectReference{
							Port: &port2,
						},
					},
				},
			},
			expectEqual: false,
			description: "Instances with different Ports are not equal",
		},
		{
			backendRef1: &GRPCBackendRef{},
			backendRef2: nil,
			expectEqual: false,
			description: "Non-nil instances are not equal to nil",
		},
	}

	for _, test := range tests {
		assert.Equal(t, test.expectEqual, test.backendRef1.Equals(test.backendRef2), test.description)
	}
}

func TestGRPCHeaderMatch_Equals(t *testing.T) {
	headerMatchType1 := gateway_api_v1beta1.HeaderMatchExact
	headerMatchType2 := gateway_api_v1beta1.HeaderMatchRegularExpression
	name1 := gateway_api_v1alpha2.GRPCHeaderName("name1")
	name2 := gateway_api_v1alpha2.GRPCHeaderName("name2")
	value1 := "value1"
	value2 := "value2"

	tests := []struct {
		headerMatch1 *GRPCHeaderMatch
		headerMatch2 HeaderMatch
		expectEqual  bool
		description  string
	}{
		{
			headerMatch1: &GRPCHeaderMatch{},
			headerMatch2: &GRPCHeaderMatch{},
			expectEqual:  true,
			description:  "Empty instances are equal",
		},
		{
			headerMatch1: &GRPCHeaderMatch{
				m: gateway_api_v1alpha2.GRPCHeaderMatch{
					Type:  &headerMatchType1,
					Name:  name1,
					Value: value1,
				},
			},
			headerMatch2: &GRPCHeaderMatch{
				m: gateway_api_v1alpha2.GRPCHeaderMatch{
					Type:  &headerMatchType1,
					Name:  name1,
					Value: value1,
				},
			},
			expectEqual: true,
			description: "Instances populated with the same values are equal",
		},
		{
			headerMatch1: &GRPCHeaderMatch{},
			headerMatch2: &HTTPHeaderMatch{},
			expectEqual:  false,
			description:  "Instances of different types are not equal",
		},
		{
			headerMatch1: &GRPCHeaderMatch{
				m: gateway_api_v1alpha2.GRPCHeaderMatch{
					Type: &headerMatchType1,
				},
			},
			headerMatch2: &GRPCHeaderMatch{
				m: gateway_api_v1alpha2.GRPCHeaderMatch{
					Type: &headerMatchType2,
				},
			},
			expectEqual: false,
			description: "Instances with different Types are not equal",
		},
		{
			headerMatch1: &GRPCHeaderMatch{
				m: gateway_api_v1alpha2.GRPCHeaderMatch{
					Name: name1,
				},
			},
			headerMatch2: &GRPCHeaderMatch{
				m: gateway_api_v1alpha2.GRPCHeaderMatch{
					Name: name2,
				},
			},
			expectEqual: false,
			description: "Instances with different Names are not equal",
		},
		{
			headerMatch1: &GRPCHeaderMatch{
				m: gateway_api_v1alpha2.GRPCHeaderMatch{
					Value: value1,
				},
			},
			headerMatch2: &GRPCHeaderMatch{
				m: gateway_api_v1alpha2.GRPCHeaderMatch{
					Value: value2,
				},
			},
			expectEqual: false,
			description: "Instances with different Values are not equal",
		},
		{
			headerMatch1: &GRPCHeaderMatch{},
			headerMatch2: nil,
			expectEqual:  false,
			description:  "Non-nil instances are not equal to nil",
		},
	}

	for _, test := range tests {
		assert.Equal(t, test.expectEqual, test.headerMatch1.Equals(test.headerMatch2), test.description)
	}
}

func TestGRPCRouteMatch_Equals(t *testing.T) {
	grpcMethodMatchType1 := gateway_api_v1alpha2.GRPCMethodMatchExact
	grpcMethodMatchType2 := gateway_api_v1alpha2.GRPCMethodMatchRegularExpression
	headerMatchType1 := gateway_api_v1beta1.HeaderMatchExact
	headerMatchType2 := gateway_api_v1beta1.HeaderMatchRegularExpression

	tests := []struct {
		routeMatch1 *GRPCRouteMatch
		routeMatch2 RouteMatch
		expectEqual bool
		description string
	}{
		{
			routeMatch1: &GRPCRouteMatch{},
			routeMatch2: &GRPCRouteMatch{},
			expectEqual: true,
			description: "Empty instances are equal",
		},
		{
			routeMatch1: &GRPCRouteMatch{
				m: gateway_api_v1alpha2.GRPCRouteMatch{
					Method: &gateway_api_v1alpha2.GRPCMethodMatch{},
					Headers: []gateway_api_v1alpha2.GRPCHeaderMatch{
						{},
					},
				},
			},
			routeMatch2: &GRPCRouteMatch{
				m: gateway_api_v1alpha2.GRPCRouteMatch{
					Method: &gateway_api_v1alpha2.GRPCMethodMatch{},
					Headers: []gateway_api_v1alpha2.GRPCHeaderMatch{
						{},
					},
				},
			},
			expectEqual: true,
			description: "Instances populated with the same values are equal",
		},
		{
			routeMatch1: &GRPCRouteMatch{},
			routeMatch2: &HTTPRouteMatch{},
			expectEqual: false,
			description: "Instances of different types are not equal",
		},
		{
			routeMatch1: &GRPCRouteMatch{
				m: gateway_api_v1alpha2.GRPCRouteMatch{
					Method: &gateway_api_v1alpha2.GRPCMethodMatch{
						Type: &grpcMethodMatchType1,
					},
				},
			},
			routeMatch2: &GRPCRouteMatch{
				m: gateway_api_v1alpha2.GRPCRouteMatch{
					Method: &gateway_api_v1alpha2.GRPCMethodMatch{
						Type: &grpcMethodMatchType2,
					},
				},
			},
			expectEqual: false,
			description: "Instances with different Methods are not equal",
		},
		{
			routeMatch1: &GRPCRouteMatch{
				m: gateway_api_v1alpha2.GRPCRouteMatch{
					Headers: []gateway_api_v1alpha2.GRPCHeaderMatch{
						{},
						{},
					},
				},
			},
			routeMatch2: &GRPCRouteMatch{
				m: gateway_api_v1alpha2.GRPCRouteMatch{
					Headers: []gateway_api_v1alpha2.GRPCHeaderMatch{
						{},
					},
				},
			},
			expectEqual: false,
			description: "Instances with different Headers lengths are not equal",
		},
		{
			routeMatch1: &GRPCRouteMatch{
				m: gateway_api_v1alpha2.GRPCRouteMatch{
					Headers: []gateway_api_v1alpha2.GRPCHeaderMatch{
						{
							Type: &headerMatchType1,
						},
					},
				},
			},
			routeMatch2: &GRPCRouteMatch{
				m: gateway_api_v1alpha2.GRPCRouteMatch{
					Headers: []gateway_api_v1alpha2.GRPCHeaderMatch{
						{
							Type: &headerMatchType2,
						},
					},
				},
			},
			expectEqual: false,
			description: "Instances with different Header values are not equal",
		},
		{
			routeMatch1: &GRPCRouteMatch{},
			routeMatch2: nil,
			expectEqual: false,
			description: "Non-nil instances are not equal to nil",
		},
	}

	for _, test := range tests {
		assert.Equal(t, test.expectEqual, test.routeMatch1.Equals(test.routeMatch2), test.description)
	}
}
