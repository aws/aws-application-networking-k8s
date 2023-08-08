package core

import (
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	gateway_api_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gateway_api_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

type Route interface {
	GetSpec() RouteSpec
	GetStatus() RouteStatus
	GetName() string
	GetNamespace() string
	GetDeletionTimestamp() *v1.Time
	GetRuntimeObject() runtime.Object
}

type HTTPRoute struct {
	gateway_api_v1beta1.HTTPRoute
}

func (r *HTTPRoute) GetSpec() RouteSpec {
	return &HTTPRouteSpec{r.Spec}
}

func (r *HTTPRoute) GetStatus() RouteStatus {
	return &HTTPRouteStatus{r.Status}
}

func (r *HTTPRoute) GetName() string {
	return r.Name
}

func (r *HTTPRoute) GetNamespace() string {
	return r.Namespace
}

func (r *HTTPRoute) GetDeletionTimestamp() *v1.Time {
	return r.DeletionTimestamp
}

func (r *HTTPRoute) GetRuntimeObject() runtime.Object {
	return &r.HTTPRoute
}

type GRPCRoute struct {
	gateway_api_v1alpha2.GRPCRoute
}

func (r *GRPCRoute) GetSpec() RouteSpec {
	return &GRPCRouteSpec{r.Spec}
}

func (r *GRPCRoute) GetStatus() RouteStatus {
	return &GRPCRouteStatus{r.Status}
}

func (r *GRPCRoute) GetName() string {
	return r.Name
}

func (r *GRPCRoute) GetNamespace() string {
	return r.Namespace
}

func (r *GRPCRoute) GetDeletionTimestamp() *v1.Time {
	return r.DeletionTimestamp
}

func (r *GRPCRoute) GetRuntimeObject() runtime.Object {
	return &r.GRPCRoute
}

type RouteSpec interface {
	GetParentRefs() []gateway_api_v1beta1.ParentReference
	GetHostnames() []gateway_api_v1beta1.Hostname
	GetRules() []RouteRule
}

type HTTPRouteSpec struct {
	gateway_api_v1beta1.HTTPRouteSpec
}

func (s *HTTPRouteSpec) GetParentRefs() []gateway_api_v1beta1.ParentReference {
	return s.ParentRefs
}

func (s *HTTPRouteSpec) GetHostnames() []gateway_api_v1beta1.Hostname {
	return s.Hostnames
}

func (s *HTTPRouteSpec) GetRules() []RouteRule {
	var rules []RouteRule
	for _, rule := range s.Rules {
		rules = append(rules, &HTTPRouteRule{rule})
	}
	return rules
}

type GRPCRouteSpec struct {
	gateway_api_v1alpha2.GRPCRouteSpec
}

func (s *GRPCRouteSpec) GetParentRefs() []gateway_api_v1beta1.ParentReference {
	return s.ParentRefs
}

func (s *GRPCRouteSpec) GetHostnames() []gateway_api_v1beta1.Hostname {
	return s.Hostnames
}

func (s *GRPCRouteSpec) GetRules() []RouteRule {
	var rules []RouteRule
	for _, rule := range s.Rules {
		rules = append(rules, &GRPCRouteRule{rule})
	}
	return rules
}

type RouteStatus interface {
	GetParents() []gateway_api_v1beta1.RouteParentStatus
}

type HTTPRouteStatus struct {
	gateway_api_v1beta1.HTTPRouteStatus
}

func (s *HTTPRouteStatus) GetParents() []gateway_api_v1beta1.RouteParentStatus {
	return s.Parents
}

type GRPCRouteStatus struct {
	gateway_api_v1alpha2.GRPCRouteStatus
}

func (s *GRPCRouteStatus) GetParents() []gateway_api_v1beta1.RouteParentStatus {
	return s.Parents
}

type RouteRule interface {
	GetBackendRefs() []BackendRef
	GetMatches() []RouteMatch
}

type HTTPRouteRule struct {
	gateway_api_v1beta1.HTTPRouteRule
}

func (r *HTTPRouteRule) GetBackendRefs() []BackendRef {
	var backendRefs []BackendRef
	for _, backendRef := range r.BackendRefs {
		backendRefs = append(backendRefs, &HTTPBackendRef{backendRef})
	}
	return backendRefs
}

func (r *HTTPRouteRule) GetMatches() []RouteMatch {
	var routeMatches []RouteMatch
	for _, routeMatch := range r.Matches {
		routeMatches = append(routeMatches, &HTTPRouteMatch{routeMatch})
	}
	return routeMatches
}

type GRPCRouteRule struct {
	gateway_api_v1alpha2.GRPCRouteRule
}

func (r *GRPCRouteRule) GetBackendRefs() []BackendRef {
	var backendRefs []BackendRef
	for _, backendRef := range r.BackendRefs {
		backendRefs = append(backendRefs, &GRPCBackendRef{backendRef})
	}
	return backendRefs
}

func (r *GRPCRouteRule) GetMatches() []RouteMatch {
	var routeMatches []RouteMatch
	for _, routeMatch := range r.Matches {
		routeMatches = append(routeMatches, &GRPCRouteMatch{routeMatch})
	}
	return routeMatches
}

type BackendRef interface {
	GetWeight() *int32
	GetGroup() *gateway_api_v1beta1.Group
	GetKind() *gateway_api_v1beta1.Kind
	GetName() gateway_api_v1beta1.ObjectName
	GetNamespace() *gateway_api_v1beta1.Namespace
	GetPort() *gateway_api_v1beta1.PortNumber
}

type HTTPBackendRef struct {
	gateway_api_v1beta1.HTTPBackendRef
}

func (r *HTTPBackendRef) GetWeight() *int32 {
	return r.Weight
}

func (r *HTTPBackendRef) GetGroup() *gateway_api_v1beta1.Group {
	return r.Group
}

func (r *HTTPBackendRef) GetKind() *gateway_api_v1beta1.Kind {
	return r.Kind
}

func (r *HTTPBackendRef) GetName() gateway_api_v1beta1.ObjectName {
	return r.Name
}

func (r *HTTPBackendRef) GetNamespace() *gateway_api_v1beta1.Namespace {
	return r.Namespace
}

func (r *HTTPBackendRef) GetPort() *gateway_api_v1beta1.PortNumber {
	return r.Port
}

type GRPCBackendRef struct {
	gateway_api_v1alpha2.GRPCBackendRef
}

func (r *GRPCBackendRef) GetWeight() *int32 {
	return r.Weight
}

func (r *GRPCBackendRef) GetGroup() *gateway_api_v1beta1.Group {
	return r.Group
}

func (r *GRPCBackendRef) GetKind() *gateway_api_v1beta1.Kind {
	return r.Kind
}

func (r *GRPCBackendRef) GetName() gateway_api_v1beta1.ObjectName {
	return r.Name
}

func (r *GRPCBackendRef) GetNamespace() *gateway_api_v1beta1.Namespace {
	return r.Namespace
}

func (r *GRPCBackendRef) GetPort() *gateway_api_v1beta1.PortNumber {
	return r.Port
}

type RouteMatch interface {
	GetHeaders() []HeaderMatch
}

type HTTPRouteMatch struct {
	gateway_api_v1beta1.HTTPRouteMatch
}

func (r *HTTPRouteMatch) GetHeaders() []HeaderMatch {
	var headerMatches []HeaderMatch
	for _, headerMatch := range r.Headers {
		headerMatches = append(headerMatches, &HTTPHeaderMatch{headerMatch})
	}
	return headerMatches
}

func (r *HTTPRouteMatch) GetPath() *gateway_api_v1beta1.HTTPPathMatch {
	return r.Path
}

func (r *HTTPRouteMatch) GetQueryParams() []gateway_api_v1beta1.HTTPQueryParamMatch {
	return r.QueryParams
}

func (r *HTTPRouteMatch) GetMethod() *gateway_api_v1beta1.HTTPMethod {
	return r.Method
}

type GRPCRouteMatch struct {
	gateway_api_v1alpha2.GRPCRouteMatch
}

func (r *GRPCRouteMatch) GetHeaders() []HeaderMatch {
	var headerMatches []HeaderMatch
	for _, headerMatch := range r.Headers {
		headerMatches = append(headerMatches, &GRPCHeaderMatch{headerMatch})
	}
	return headerMatches
}

func (r *GRPCRouteMatch) GetMethod() *gateway_api_v1alpha2.GRPCMethodMatch {
	return r.Method
}

type HeaderMatch interface {
	GetType() *gateway_api_v1beta1.HeaderMatchType
	GetName() string
	GetValue() string
}

type HTTPHeaderMatch struct {
	gateway_api_v1beta1.HTTPHeaderMatch
}

func (r *HTTPHeaderMatch) GetType() *gateway_api_v1beta1.HeaderMatchType {
	return r.Type
}

func (r *HTTPHeaderMatch) GetName() string {
	return string(r.Name)
}

func (r *HTTPHeaderMatch) GetValue() string {
	return r.Value
}

type GRPCHeaderMatch struct {
	gateway_api_v1alpha2.GRPCHeaderMatch
}

func (r *GRPCHeaderMatch) GetType() *gateway_api_v1beta1.HeaderMatchType {
	return r.Type
}

func (r *GRPCHeaderMatch) GetName() string {
	return string(r.Name)
}

func (r *GRPCHeaderMatch) GetValue() string {
	return r.Value
}
