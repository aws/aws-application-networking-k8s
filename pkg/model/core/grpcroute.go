package core

import (
	"context"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gateway_api_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gateway_api_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

type GRPCRoute struct {
	r gateway_api_v1alpha2.GRPCRoute
}

func NewGRPCRoute(route gateway_api_v1alpha2.GRPCRoute) *GRPCRoute {
	return &GRPCRoute{r: route}
}

func GetGRPCRoute(client client.Client, ctx context.Context, routeNamespacedName types.NamespacedName) (Route, error) {
	grpcRoute := &gateway_api_v1alpha2.GRPCRoute{}
	err := client.Get(ctx, routeNamespacedName, grpcRoute)
	if err != nil {
		return nil, err
	}
	return NewGRPCRoute(*grpcRoute), nil
}

func ListGRPCRoutes(client client.Client, context context.Context) []Route {
	routeList := &gateway_api_v1alpha2.GRPCRouteList{}
	client.List(context, routeList)
	var routes []Route
	for _, route := range routeList.Items {
		routes = append(routes, NewGRPCRoute(route))
	}
	return routes
}

func (r *GRPCRoute) Spec() RouteSpec {
	return &GRPCRouteSpec{r.r.Spec}
}

func (r *GRPCRoute) Status() RouteStatus {
	return &GRPCRouteStatus{&r.r.Status}
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

func (r *GRPCRoute) DeepCopy() Route {
	return &GRPCRoute{r: *r.r.DeepCopy()}
}

func (r *GRPCRoute) K8sObject() client.Object {
	return &r.r
}

func (r *GRPCRoute) Inner() *gateway_api_v1alpha2.GRPCRoute {
	return &r.r
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

func (s *GRPCRouteSpec) Equals(routeSpec RouteSpec) bool {
	_, ok := routeSpec.(*GRPCRouteSpec)
	if !ok {
		return false
	}

	if !reflect.DeepEqual(s.ParentRefs(), routeSpec.ParentRefs()) {
		return false
	}

	if !reflect.DeepEqual(s.Hostnames(), routeSpec.Hostnames()) {
		return false
	}

	if len(s.Rules()) != len(routeSpec.Rules()) {
		return false
	}

	for i, rule := range s.Rules() {
		otherRule := routeSpec.Rules()[i]
		if !rule.Equals(otherRule) {
			return false
		}
	}

	return true
}

type GRPCRouteStatus struct {
	s *gateway_api_v1alpha2.GRPCRouteStatus
}

func (s *GRPCRouteStatus) Parents() []gateway_api_v1beta1.RouteParentStatus {
	return s.s.Parents
}

func (s *GRPCRouteStatus) SetParents(parents []gateway_api_v1beta1.RouteParentStatus) {
	s.s.Parents = parents
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

func (r *GRPCRouteRule) Equals(routeRule RouteRule) bool {
	other, ok := routeRule.(*GRPCRouteRule)
	if !ok {
		return false
	}

	if len(r.BackendRefs()) != len(other.BackendRefs()) {
		return false
	}
	for i, backendRef := range r.BackendRefs() {
		otherBackendRef := other.BackendRefs()[i]
		if !backendRef.Equals(otherBackendRef) {
			return false
		}
	}

	if len(r.Matches()) != len(other.Matches()) {
		return false
	}
	for i, match := range r.Matches() {
		otherMatch := other.Matches()[i]
		if !match.Equals(otherMatch) {
			return false
		}
	}

	return true
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

func (r *GRPCBackendRef) Equals(backendRef BackendRef) bool {
	other, ok := backendRef.(*GRPCBackendRef)
	if !ok {
		return false
	}

	if (r.Weight() == nil && other.Weight() != nil) || (r.Weight() != nil && other.Weight() == nil) {
		return false
	}

	if r.Weight() != nil && other.Weight() != nil && *r.Weight() != *other.Weight() {
		return false
	}

	if !reflect.DeepEqual(r.Group(), other.Group()) ||
		!reflect.DeepEqual(r.Kind(), other.Kind()) ||
		!reflect.DeepEqual(r.Name(), other.Name()) ||
		!reflect.DeepEqual(r.Namespace(), other.Namespace()) ||
		!reflect.DeepEqual(r.Port(), other.Port()) {
		return false
	}

	return true
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

func (m *GRPCRouteMatch) Equals(routeMatch RouteMatch) bool {
	other, ok := routeMatch.(*GRPCRouteMatch)
	if !ok {
		return false
	}

	if len(m.Headers()) != len(other.Headers()) {
		return false
	}

	for i, header := range m.Headers() {
		if !header.Equals(other.Headers()[i]) {
			return false
		}
	}

	if !reflect.DeepEqual(m.Method(), other.Method()) {
		return false
	}

	return true
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

func (m *GRPCHeaderMatch) Equals(headerMatch HeaderMatch) bool {
	other, ok := headerMatch.(*GRPCHeaderMatch)
	if !ok {
		return false
	}

	return m.Type() == other.Type() &&
		m.Name() == other.Name() &&
		m.Value() == other.Value()
}
