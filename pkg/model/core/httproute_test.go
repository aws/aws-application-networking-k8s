package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

func TestHTTPRouteSpec_Equals(t *testing.T) {
	name1 := gwv1beta1.ObjectName("name1")
	name2 := gwv1beta1.ObjectName("name2")

	tests := []struct {
		routeSpec1  *HTTPRouteSpec
		routeSpec2  RouteSpec
		expectEqual bool
		description string
	}{
		{
			routeSpec1:  &HTTPRouteSpec{},
			routeSpec2:  &HTTPRouteSpec{},
			expectEqual: true,
			description: "Empty instances are equal",
		},
		{
			routeSpec1: &HTTPRouteSpec{
				s: gwv1beta1.HTTPRouteSpec{
					CommonRouteSpec: gwv1beta1.CommonRouteSpec{
						ParentRefs: []gwv1beta1.ParentReference{
							{},
						},
					},
					Hostnames: []gwv1beta1.Hostname{"example.com"},
					Rules: []gwv1beta1.HTTPRouteRule{
						{},
					},
				},
			},
			routeSpec2: &HTTPRouteSpec{
				s: gwv1beta1.HTTPRouteSpec{
					CommonRouteSpec: gwv1beta1.CommonRouteSpec{
						ParentRefs: []gwv1beta1.ParentReference{
							{},
						},
					},
					Hostnames: []gwv1beta1.Hostname{"example.com"},
					Rules: []gwv1beta1.HTTPRouteRule{
						{},
					},
				},
			},
			expectEqual: true,
			description: "Instances populated with the same values are equal",
		},
		{
			routeSpec1:  &HTTPRouteSpec{},
			routeSpec2:  &GRPCRouteSpec{},
			expectEqual: false,
			description: "Instances of different types are not equal",
		},
		{
			routeSpec1: &HTTPRouteSpec{
				s: gwv1beta1.HTTPRouteSpec{
					CommonRouteSpec: gwv1beta1.CommonRouteSpec{
						ParentRefs: []gwv1beta1.ParentReference{{Name: "parent1"}},
					},
				},
			},
			routeSpec2: &HTTPRouteSpec{
				s: gwv1beta1.HTTPRouteSpec{
					CommonRouteSpec: gwv1beta1.CommonRouteSpec{
						ParentRefs: []gwv1beta1.ParentReference{{Name: "parent2"}},
					},
				},
			},
			expectEqual: false,
			description: "Instances with different ParentRefs are not equal",
		},
		{
			routeSpec1: &HTTPRouteSpec{
				s: gwv1beta1.HTTPRouteSpec{
					Hostnames: []gwv1beta1.Hostname{"example1.com"},
				},
			},
			routeSpec2: &HTTPRouteSpec{
				s: gwv1beta1.HTTPRouteSpec{
					Hostnames: []gwv1beta1.Hostname{"example2.com"},
				},
			},
			expectEqual: false,
			description: "Instances with different HostNames are not equal",
		},
		{
			routeSpec1: &HTTPRouteSpec{
				s: gwv1beta1.HTTPRouteSpec{
					Rules: []gwv1beta1.HTTPRouteRule{
						{},
						{},
					},
				},
			},
			routeSpec2: &HTTPRouteSpec{
				s: gwv1beta1.HTTPRouteSpec{
					Rules: []gwv1beta1.HTTPRouteRule{
						{},
					},
				},
			},
			expectEqual: false,
			description: "Instances with different Rules lengths are not equal",
		},
		{
			routeSpec1: &HTTPRouteSpec{
				s: gwv1beta1.HTTPRouteSpec{
					Rules: []gwv1beta1.HTTPRouteRule{
						{
							BackendRefs: []gwv1beta1.HTTPBackendRef{
								{
									BackendRef: gwv1beta1.BackendRef{
										BackendObjectReference: gwv1beta1.BackendObjectReference{
											Name: name1,
										},
									},
								},
							},
						},
					},
				},
			},
			routeSpec2: &HTTPRouteSpec{
				s: gwv1beta1.HTTPRouteSpec{
					Rules: []gwv1beta1.HTTPRouteRule{
						{
							BackendRefs: []gwv1beta1.HTTPBackendRef{
								{
									BackendRef: gwv1beta1.BackendRef{
										BackendObjectReference: gwv1beta1.BackendObjectReference{
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
			routeSpec1:  &HTTPRouteSpec{},
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

func TestHTTPRouteRule_Equals(t *testing.T) {
	httpMethod1 := gwv1.HTTPMethodPost
	httpMethod2 := gwv1.HTTPMethodGet

	tests := []struct {
		routeRule1  *HTTPRouteRule
		routeRule2  RouteRule
		expectEqual bool
		description string
	}{
		{
			routeRule1:  &HTTPRouteRule{},
			routeRule2:  &HTTPRouteRule{},
			expectEqual: true,
			description: "Empty instances are equal",
		},
		{
			routeRule1: &HTTPRouteRule{
				r: gwv1beta1.HTTPRouteRule{
					BackendRefs: []gwv1beta1.HTTPBackendRef{
						{},
					},
					Matches: []gwv1beta1.HTTPRouteMatch{
						{},
					},
				},
			},
			routeRule2: &HTTPRouteRule{
				r: gwv1beta1.HTTPRouteRule{
					BackendRefs: []gwv1beta1.HTTPBackendRef{
						{},
					},
					Matches: []gwv1beta1.HTTPRouteMatch{
						{},
					},
				},
			},
			expectEqual: true,
			description: "Instances populated with the same values are equal",
		},
		{
			routeRule1:  &HTTPRouteRule{},
			routeRule2:  &GRPCRouteRule{},
			expectEqual: false,
			description: "Instances of different types are not equal",
		},
		{
			routeRule1: &HTTPRouteRule{
				r: gwv1beta1.HTTPRouteRule{
					BackendRefs: []gwv1beta1.HTTPBackendRef{
						{},
						{},
					},
				},
			},
			routeRule2: &HTTPRouteRule{
				r: gwv1beta1.HTTPRouteRule{
					BackendRefs: []gwv1beta1.HTTPBackendRef{
						{},
					},
				},
			},
			expectEqual: false,
			description: "Instances with different BackendRefs lengths are not equal",
		},
		{
			routeRule1: &HTTPRouteRule{
				r: gwv1beta1.HTTPRouteRule{
					BackendRefs: []gwv1beta1.HTTPBackendRef{
						{
							BackendRef: gwv1beta1.BackendRef{
								Weight: ptr.To(int32(1)),
							},
						},
					},
				},
			},
			routeRule2: &HTTPRouteRule{
				r: gwv1beta1.HTTPRouteRule{
					BackendRefs: []gwv1beta1.HTTPBackendRef{
						{
							BackendRef: gwv1beta1.BackendRef{
								Weight: ptr.To(int32(2)),
							},
						},
					},
				},
			},
			expectEqual: false,
			description: "Instances with different BackendRef values are not equal",
		},
		{
			routeRule1: &HTTPRouteRule{
				r: gwv1beta1.HTTPRouteRule{
					Matches: []gwv1beta1.HTTPRouteMatch{
						{},
						{},
					},
				},
			},
			routeRule2: &HTTPRouteRule{
				r: gwv1beta1.HTTPRouteRule{
					Matches: []gwv1beta1.HTTPRouteMatch{
						{},
					},
				},
			},
			expectEqual: false,
			description: "Instances with different Matches lengths are not equal",
		},
		{
			routeRule1: &HTTPRouteRule{
				r: gwv1beta1.HTTPRouteRule{
					Matches: []gwv1beta1.HTTPRouteMatch{
						{
							Method: &httpMethod1,
						},
					},
				},
			},
			routeRule2: &HTTPRouteRule{
				r: gwv1beta1.HTTPRouteRule{
					Matches: []gwv1beta1.HTTPRouteMatch{
						{
							Method: &httpMethod2,
						},
					},
				},
			},
			expectEqual: false,
			description: "Instances with different Match values are not equal",
		},
		{
			routeRule1:  &HTTPRouteRule{},
			routeRule2:  nil,
			expectEqual: false,
			description: "Non-nil instances are not equal to nil",
		},
	}

	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			assert.Equal(t, test.expectEqual, test.routeRule1.Equals(test.routeRule2), test.description)
		})
	}
}

func TestHTTPBackendRef_Equals(t *testing.T) {
	weight1 := ptr.To(int32(1))
	weight2 := ptr.To(int32(2))
	group1 := gwv1beta1.Group("group1")
	group2 := gwv1beta1.Group("group2")
	kind1 := gwv1beta1.Kind("kind1")
	kind2 := gwv1beta1.Kind("kind2")
	name1 := gwv1beta1.ObjectName("name1")
	name2 := gwv1beta1.ObjectName("name2")
	namespace1 := gwv1beta1.Namespace("namespace1")
	namespace2 := gwv1beta1.Namespace("namespace2")
	port1 := gwv1beta1.PortNumber(1)
	port2 := gwv1beta1.PortNumber(2)

	tests := []struct {
		backendRef1 *HTTPBackendRef
		backendRef2 BackendRef
		expectEqual bool
		description string
	}{
		{
			backendRef1: &HTTPBackendRef{},
			backendRef2: &HTTPBackendRef{},
			expectEqual: true,
			description: "Empty instances are equal",
		},
		{
			backendRef1: &HTTPBackendRef{
				r: gwv1beta1.HTTPBackendRef{
					BackendRef: gwv1beta1.BackendRef{
						Weight: weight1,
						BackendObjectReference: gwv1beta1.BackendObjectReference{
							Group:     &group1,
							Kind:      &kind1,
							Name:      name1,
							Namespace: &namespace1,
							Port:      &port1,
						},
					},
				},
			},
			backendRef2: &HTTPBackendRef{
				r: gwv1beta1.HTTPBackendRef{
					BackendRef: gwv1beta1.BackendRef{
						Weight: weight1,
						BackendObjectReference: gwv1beta1.BackendObjectReference{
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
			backendRef1: &HTTPBackendRef{},
			backendRef2: &GRPCBackendRef{},
			expectEqual: false,
			description: "Instances of different types are not equal",
		},
		{
			backendRef1: &HTTPBackendRef{
				r: gwv1beta1.HTTPBackendRef{
					BackendRef: gwv1beta1.BackendRef{
						Weight: weight1,
					},
				},
			},
			backendRef2: &HTTPBackendRef{
				r: gwv1beta1.HTTPBackendRef{
					BackendRef: gwv1beta1.BackendRef{
						Weight: weight2,
					},
				},
			},
			expectEqual: false,
			description: "Instances with different Weights are not equal",
		},
		{
			backendRef1: &HTTPBackendRef{
				r: gwv1beta1.HTTPBackendRef{
					BackendRef: gwv1beta1.BackendRef{
						BackendObjectReference: gwv1beta1.BackendObjectReference{
							Group: &group1,
						},
					},
				},
			},
			backendRef2: &HTTPBackendRef{
				r: gwv1beta1.HTTPBackendRef{
					BackendRef: gwv1beta1.BackendRef{
						BackendObjectReference: gwv1beta1.BackendObjectReference{
							Group: &group2,
						},
					},
				},
			},
			expectEqual: false,
			description: "Instances with different Groups are not equal",
		},
		{
			backendRef1: &HTTPBackendRef{
				r: gwv1beta1.HTTPBackendRef{
					BackendRef: gwv1beta1.BackendRef{
						BackendObjectReference: gwv1beta1.BackendObjectReference{
							Kind: &kind1,
						},
					},
				},
			},
			backendRef2: &HTTPBackendRef{
				r: gwv1beta1.HTTPBackendRef{
					BackendRef: gwv1beta1.BackendRef{
						BackendObjectReference: gwv1beta1.BackendObjectReference{
							Kind: &kind2,
						},
					},
				},
			},
			expectEqual: false,
			description: "Instances with different Kinds are not equal",
		},
		{
			backendRef1: &HTTPBackendRef{
				r: gwv1beta1.HTTPBackendRef{
					BackendRef: gwv1beta1.BackendRef{
						BackendObjectReference: gwv1beta1.BackendObjectReference{
							Name: name1,
						},
					},
				},
			},
			backendRef2: &HTTPBackendRef{
				r: gwv1beta1.HTTPBackendRef{
					BackendRef: gwv1beta1.BackendRef{
						BackendObjectReference: gwv1beta1.BackendObjectReference{
							Name: name2,
						},
					},
				},
			},
			expectEqual: false,
			description: "Instances with different Names are not equal",
		},
		{
			backendRef1: &HTTPBackendRef{
				r: gwv1beta1.HTTPBackendRef{
					BackendRef: gwv1beta1.BackendRef{
						BackendObjectReference: gwv1beta1.BackendObjectReference{
							Namespace: &namespace1,
						},
					},
				},
			},
			backendRef2: &HTTPBackendRef{
				r: gwv1beta1.HTTPBackendRef{
					BackendRef: gwv1beta1.BackendRef{
						BackendObjectReference: gwv1beta1.BackendObjectReference{
							Namespace: &namespace2,
						},
					},
				},
			},
			expectEqual: false,
			description: "Instances with different Namespaces are not equal",
		},
		{
			backendRef1: &HTTPBackendRef{
				r: gwv1beta1.HTTPBackendRef{
					BackendRef: gwv1beta1.BackendRef{
						BackendObjectReference: gwv1beta1.BackendObjectReference{
							Port: &port1,
						},
					},
				},
			},
			backendRef2: &HTTPBackendRef{
				r: gwv1beta1.HTTPBackendRef{
					BackendRef: gwv1beta1.BackendRef{
						BackendObjectReference: gwv1beta1.BackendObjectReference{
							Port: &port2,
						},
					},
				},
			},
			expectEqual: false,
			description: "Instances with different Ports are not equal",
		},
		{
			backendRef1: &HTTPBackendRef{},
			backendRef2: nil,
			expectEqual: false,
			description: "Non-nil instances are not equal to nil",
		},
	}

	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			assert.Equal(t, test.expectEqual, test.backendRef1.Equals(test.backendRef2), test.description)
		})
	}
}

func TestHTTPHeaderMatch_Equals(t *testing.T) {
	headerMatchType1 := gwv1.HeaderMatchExact
	headerMatchType2 := gwv1.HeaderMatchRegularExpression
	name1 := gwv1.HTTPHeaderName("name1")
	name2 := gwv1.HTTPHeaderName("name2")
	value1 := "value1"
	value2 := "value2"

	tests := []struct {
		headerMatch1 *HTTPHeaderMatch
		headerMatch2 HeaderMatch
		expectEqual  bool
		description  string
	}{
		{
			headerMatch1: &HTTPHeaderMatch{},
			headerMatch2: &HTTPHeaderMatch{},
			expectEqual:  true,
			description:  "Empty instances are equal",
		},
		{
			headerMatch1: &HTTPHeaderMatch{
				m: gwv1beta1.HTTPHeaderMatch{
					Type:  &headerMatchType1,
					Name:  name1,
					Value: value1,
				},
			},
			headerMatch2: &HTTPHeaderMatch{
				m: gwv1beta1.HTTPHeaderMatch{
					Type:  &headerMatchType1,
					Name:  name1,
					Value: value1,
				},
			},
			expectEqual: true,
			description: "Instances populated with the same values are equal",
		},
		{
			headerMatch1: &HTTPHeaderMatch{},
			headerMatch2: &GRPCHeaderMatch{},
			expectEqual:  false,
			description:  "Instances of different types are not equal",
		},
		{
			headerMatch1: &HTTPHeaderMatch{
				m: gwv1beta1.HTTPHeaderMatch{
					Type: &headerMatchType1,
				},
			},
			headerMatch2: &HTTPHeaderMatch{
				m: gwv1beta1.HTTPHeaderMatch{
					Type: &headerMatchType2,
				},
			},
			expectEqual: false,
			description: "Instances with different Types are not equal",
		},
		{
			headerMatch1: &HTTPHeaderMatch{
				m: gwv1beta1.HTTPHeaderMatch{
					Name: name1,
				},
			},
			headerMatch2: &HTTPHeaderMatch{
				m: gwv1beta1.HTTPHeaderMatch{
					Name: name2,
				},
			},
			expectEqual: false,
			description: "Instances with different Names are not equal",
		},
		{
			headerMatch1: &HTTPHeaderMatch{
				m: gwv1beta1.HTTPHeaderMatch{
					Value: value1,
				},
			},
			headerMatch2: &HTTPHeaderMatch{
				m: gwv1beta1.HTTPHeaderMatch{
					Value: value2,
				},
			},
			expectEqual: false,
			description: "Instances with different Values are not equal",
		},
		{
			headerMatch1: &HTTPHeaderMatch{},
			headerMatch2: nil,
			expectEqual:  false,
			description:  "Non-nil instances are not equal to nil",
		},
	}

	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			assert.Equal(t, test.expectEqual, test.headerMatch1.Equals(test.headerMatch2), test.description)
		})
	}
}

func TestHTTPRouteMatch_Equals(t *testing.T) {
	httpMethod1 := gwv1.HTTPMethodPost
	httpMethod2 := gwv1.HTTPMethodGet
	pathMatchType1 := gwv1.PathMatchExact
	pathMatchType2 := gwv1.PathMatchPathPrefix
	headerMatchType1 := gwv1.HeaderMatchExact
	headerMatchType2 := gwv1.HeaderMatchRegularExpression
	queryParamMatchType1 := gwv1.QueryParamMatchExact
	queryParamMatchType2 := gwv1.QueryParamMatchRegularExpression

	tests := []struct {
		routeMatch1 *HTTPRouteMatch
		routeMatch2 RouteMatch
		expectEqual bool
		description string
	}{
		{
			routeMatch1: &HTTPRouteMatch{},
			routeMatch2: &HTTPRouteMatch{},
			expectEqual: true,
			description: "Empty instances are equal",
		},
		{
			routeMatch1: &HTTPRouteMatch{
				m: gwv1beta1.HTTPRouteMatch{
					Path: &gwv1beta1.HTTPPathMatch{},
					Headers: []gwv1beta1.HTTPHeaderMatch{
						{},
					},
					QueryParams: []gwv1beta1.HTTPQueryParamMatch{
						{},
					},
					Method: &httpMethod1,
				},
			},
			routeMatch2: &HTTPRouteMatch{
				m: gwv1beta1.HTTPRouteMatch{
					Path: &gwv1beta1.HTTPPathMatch{},
					Headers: []gwv1beta1.HTTPHeaderMatch{
						{},
					},
					QueryParams: []gwv1beta1.HTTPQueryParamMatch{
						{},
					},
					Method: &httpMethod1,
				},
			},
			expectEqual: true,
			description: "Instances populated with the same values are equal",
		},
		{
			routeMatch1: &HTTPRouteMatch{},
			routeMatch2: &GRPCRouteMatch{},
			expectEqual: false,
			description: "Instances of different types are not equal",
		},
		{
			routeMatch1: &HTTPRouteMatch{
				m: gwv1beta1.HTTPRouteMatch{
					Path: &gwv1beta1.HTTPPathMatch{
						Type: &pathMatchType1,
					},
				},
			},
			routeMatch2: &HTTPRouteMatch{
				m: gwv1beta1.HTTPRouteMatch{
					Path: &gwv1beta1.HTTPPathMatch{
						Type: &pathMatchType2,
					},
				},
			},
			expectEqual: false,
			description: "Instances with different Paths are not equal",
		},
		{
			routeMatch1: &HTTPRouteMatch{
				m: gwv1beta1.HTTPRouteMatch{
					Headers: []gwv1beta1.HTTPHeaderMatch{
						{},
						{},
					},
				},
			},
			routeMatch2: &HTTPRouteMatch{
				m: gwv1beta1.HTTPRouteMatch{
					Headers: []gwv1beta1.HTTPHeaderMatch{
						{},
					},
				},
			},
			expectEqual: false,
			description: "Instances with different Headers lengths are not equal",
		},
		{
			routeMatch1: &HTTPRouteMatch{
				m: gwv1beta1.HTTPRouteMatch{
					Headers: []gwv1beta1.HTTPHeaderMatch{
						{
							Type: &headerMatchType1,
						},
					},
				},
			},
			routeMatch2: &HTTPRouteMatch{
				m: gwv1beta1.HTTPRouteMatch{
					Headers: []gwv1beta1.HTTPHeaderMatch{
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
			routeMatch1: &HTTPRouteMatch{
				m: gwv1beta1.HTTPRouteMatch{
					QueryParams: []gwv1beta1.HTTPQueryParamMatch{
						{},
						{},
					},
				},
			},
			routeMatch2: &HTTPRouteMatch{
				m: gwv1beta1.HTTPRouteMatch{
					QueryParams: []gwv1beta1.HTTPQueryParamMatch{
						{},
					},
				},
			},
			expectEqual: false,
			description: "Instances with different QueryParams lengths are not equal",
		},
		{
			routeMatch1: &HTTPRouteMatch{
				m: gwv1beta1.HTTPRouteMatch{
					QueryParams: []gwv1beta1.HTTPQueryParamMatch{
						{
							Type: &queryParamMatchType1,
						},
					},
				},
			},
			routeMatch2: &HTTPRouteMatch{
				m: gwv1beta1.HTTPRouteMatch{
					QueryParams: []gwv1beta1.HTTPQueryParamMatch{
						{
							Type: &queryParamMatchType2,
						},
					},
				},
			},
			expectEqual: false,
			description: "Instances with different QueryParam values are not equal",
		},
		{
			routeMatch1: &HTTPRouteMatch{
				m: gwv1beta1.HTTPRouteMatch{
					Method: &httpMethod1,
				},
			},
			routeMatch2: &HTTPRouteMatch{
				m: gwv1beta1.HTTPRouteMatch{
					Method: &httpMethod2,
				},
			},
			expectEqual: false,
			description: "Instances with different Methods are not equal",
		},
		{
			routeMatch1: &HTTPRouteMatch{},
			routeMatch2: nil,
			expectEqual: false,
			description: "Non-nil instances are not equal to nil",
		},
	}

	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			assert.Equal(t, test.expectEqual, test.routeMatch1.Equals(test.routeMatch2), test.description)
		})
	}
}
