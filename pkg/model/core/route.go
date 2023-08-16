package core

import (
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gateway_api_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gateway_api_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

type Route interface {
	Spec() RouteSpec
	Status() RouteStatus
	Name() string
	Namespace() string
	DeletionTimestamp() *v1.Time
	K8sObject() client.Object
}

type HTTPRoute struct {
	r gateway_api_v1beta1.HTTPRoute
}

func NewHTTPRoute(route gateway_api_v1beta1.HTTPRoute) *HTTPRoute {
	return &HTTPRoute{r: route}
}

func (r *HTTPRoute) Spec() RouteSpec {
	return &HTTPRouteSpec{r.r.Spec}
}

func (r *HTTPRoute) Status() RouteStatus {
	return &HTTPRouteStatus{r.r.Status}
}

func (r *HTTPRoute) Name() string {
	return r.r.Name
}

func (r *HTTPRoute) Namespace() string {
	return r.r.Namespace
}

func (r *HTTPRoute) DeletionTimestamp() *v1.Time {
	return r.r.DeletionTimestamp
}

func (r *HTTPRoute) K8sObject() client.Object {
	return &r.r
}

func (r *HTTPRoute) Inner() *gateway_api_v1beta1.HTTPRoute {
	return &r.r
}

type GRPCRoute struct {
	r gateway_api_v1alpha2.GRPCRoute
}

func NewGRPCRoute(route gateway_api_v1alpha2.GRPCRoute) *GRPCRoute {
	return &GRPCRoute{r: route}
}

func (r *GRPCRoute) Spec() RouteSpec {
	return &GRPCRouteSpec{r.r.Spec}
}

func (r *GRPCRoute) Status() RouteStatus {
	return &GRPCRouteStatus{r.r.Status}
}

func (r *GRPCRoute) Name() string {
	return r.r.Name
}

func (r *GRPCRoute) Namespace() string {
	return r.r.Namespace
}

func (r *GRPCRoute) DeletionTimestamp() *v1.Time {
	return r.r.DeletionTimestamp
}

func (r *GRPCRoute) K8sObject() client.Object {
	return &r.r
}

func (r *GRPCRoute) Inner() *gateway_api_v1alpha2.GRPCRoute {
	return &r.r
}

type RouteSpec interface {
	ParentRefs() []gateway_api_v1beta1.ParentReference
	Hostnames() []gateway_api_v1beta1.Hostname
	Rules() []RouteRule
}

type HTTPRouteSpec struct {
	s gateway_api_v1beta1.HTTPRouteSpec
}

func (s *HTTPRouteSpec) ParentRefs() []gateway_api_v1beta1.ParentReference {
	return s.s.ParentRefs
}

func (s *HTTPRouteSpec) Hostnames() []gateway_api_v1beta1.Hostname {
	return s.s.Hostnames
}

func (s *HTTPRouteSpec) Rules() []RouteRule {
	var rules []RouteRule
	for _, rule := range s.s.Rules {
		rules = append(rules, &HTTPRouteRule{rule})
	}
	return rules
}

type GRPCRouteSpec struct {
	s gateway_api_v1alpha2.GRPCRouteSpec
}

func (s *GRPCRouteSpec) ParentRefs() []gateway_api_v1beta1.ParentReference {
	return s.s.ParentRefs
}

func (s *GRPCRouteSpec) Hostnames() []gateway_api_v1beta1.Hostname {
	return s.s.Hostnames
}

func (s *GRPCRouteSpec) Rules() []RouteRule {
	var rules []RouteRule
	for _, rule := range s.s.Rules {
		rules = append(rules, &GRPCRouteRule{rule})
	}
	return rules
}

type RouteStatus interface {
	Parents() []gateway_api_v1beta1.RouteParentStatus
}

type HTTPRouteStatus struct {
	s gateway_api_v1beta1.HTTPRouteStatus
}

func (s *HTTPRouteStatus) Parents() []gateway_api_v1beta1.RouteParentStatus {
	return s.s.Parents
}

type GRPCRouteStatus struct {
	s gateway_api_v1alpha2.GRPCRouteStatus
}

func (s *GRPCRouteStatus) Parents() []gateway_api_v1beta1.RouteParentStatus {
	return s.s.Parents
}

type RouteRule interface {
	BackendRefs() []BackendRef
	Matches() []RouteMatch
}

type HTTPRouteRule struct {
	r gateway_api_v1beta1.HTTPRouteRule
}

func (r *HTTPRouteRule) BackendRefs() []BackendRef {
	var backendRefs []BackendRef
	for _, backendRef := range r.r.BackendRefs {
		backendRefs = append(backendRefs, &HTTPBackendRef{backendRef})
	}
	return backendRefs
}

func (r *HTTPRouteRule) Matches() []RouteMatch {
	var routeMatches []RouteMatch
	for _, routeMatch := range r.r.Matches {
		routeMatches = append(routeMatches, &HTTPRouteMatch{routeMatch})
	}
	return routeMatches
}

type GRPCRouteRule struct {
	r gateway_api_v1alpha2.GRPCRouteRule
}

func (r *GRPCRouteRule) BackendRefs() []BackendRef {
	var backendRefs []BackendRef
	for _, backendRef := range r.r.BackendRefs {
		backendRefs = append(backendRefs, &GRPCBackendRef{backendRef})
	}
	return backendRefs
}

func (r *GRPCRouteRule) Matches() []RouteMatch {
	var routeMatches []RouteMatch
	for _, routeMatch := range r.r.Matches {
		routeMatches = append(routeMatches, &GRPCRouteMatch{routeMatch})
	}
	return routeMatches
}

type BackendRef interface {
	Weight() *int32
	Group() *gateway_api_v1beta1.Group
	Kind() *gateway_api_v1beta1.Kind
	Name() gateway_api_v1beta1.ObjectName
	Namespace() *gateway_api_v1beta1.Namespace
	Port() *gateway_api_v1beta1.PortNumber
}

type HTTPBackendRef struct {
	r gateway_api_v1beta1.HTTPBackendRef
}

func (r *HTTPBackendRef) Weight() *int32 {
	return r.r.Weight
}

func (r *HTTPBackendRef) Group() *gateway_api_v1beta1.Group {
	return r.r.Group
}

func (r *HTTPBackendRef) Kind() *gateway_api_v1beta1.Kind {
	return r.r.Kind
}

func (r *HTTPBackendRef) Name() gateway_api_v1beta1.ObjectName {
	return r.r.Name
}

func (r *HTTPBackendRef) Namespace() *gateway_api_v1beta1.Namespace {
	return r.r.Namespace
}

func (r *HTTPBackendRef) Port() *gateway_api_v1beta1.PortNumber {
	return r.r.Port
}

type GRPCBackendRef struct {
	r gateway_api_v1alpha2.GRPCBackendRef
}

func (r *GRPCBackendRef) Weight() *int32 {
	return r.r.Weight
}

func (r *GRPCBackendRef) Group() *gateway_api_v1beta1.Group {
	return r.r.Group
}

func (r *GRPCBackendRef) Kind() *gateway_api_v1beta1.Kind {
	return r.r.Kind
}

func (r *GRPCBackendRef) Name() gateway_api_v1beta1.ObjectName {
	return r.r.Name
}

func (r *GRPCBackendRef) Namespace() *gateway_api_v1beta1.Namespace {
	return r.r.Namespace
}

func (r *GRPCBackendRef) Port() *gateway_api_v1beta1.PortNumber {
	return r.r.Port
}

type RouteMatch interface {
	Headers() []HeaderMatch
}

type HTTPRouteMatch struct {
	m gateway_api_v1beta1.HTTPRouteMatch
}

func (m *HTTPRouteMatch) Headers() []HeaderMatch {
	var headerMatches []HeaderMatch
	for _, headerMatch := range m.m.Headers {
		headerMatches = append(headerMatches, &HTTPHeaderMatch{headerMatch})
	}
	return headerMatches
}

func (m *HTTPRouteMatch) Path() *gateway_api_v1beta1.HTTPPathMatch {
	return m.m.Path
}

func (m *HTTPRouteMatch) QueryParams() []gateway_api_v1beta1.HTTPQueryParamMatch {
	return m.m.QueryParams
}

func (m *HTTPRouteMatch) Method() *gateway_api_v1beta1.HTTPMethod {
	return m.m.Method
}

type GRPCRouteMatch struct {
	m gateway_api_v1alpha2.GRPCRouteMatch
}

func (m *GRPCRouteMatch) Headers() []HeaderMatch {
	var headerMatches []HeaderMatch
	for _, headerMatch := range m.m.Headers {
		headerMatches = append(headerMatches, &GRPCHeaderMatch{headerMatch})
	}
	return headerMatches
}

func (m *GRPCRouteMatch) Method() *gateway_api_v1alpha2.GRPCMethodMatch {
	return m.m.Method
}

type HeaderMatch interface {
	Type() *gateway_api_v1beta1.HeaderMatchType
	Name() string
	Value() string
}

type HTTPHeaderMatch struct {
	m gateway_api_v1beta1.HTTPHeaderMatch
}

func (m *HTTPHeaderMatch) Type() *gateway_api_v1beta1.HeaderMatchType {
	return m.m.Type
}

func (m *HTTPHeaderMatch) Name() string {
	return string(m.m.Name)
}

func (m *HTTPHeaderMatch) Value() string {
	return m.m.Value
}

type GRPCHeaderMatch struct {
	m gateway_api_v1alpha2.GRPCHeaderMatch
}

func (m *GRPCHeaderMatch) Type() *gateway_api_v1beta1.HeaderMatchType {
	return m.m.Type
}

func (m *GRPCHeaderMatch) Name() string {
	return string(m.m.Name)
}

func (m *GRPCHeaderMatch) Value() string {
	return m.m.Value
}
