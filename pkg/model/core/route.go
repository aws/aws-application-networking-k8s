package core

import (
	"fmt"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"reflect"
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
	DeepCopy() Route
	K8sObject() client.Object
}

func NewRoute(object client.Object) (Route, error) {
	switch obj := object.(type) {
	case *gateway_api_v1beta1.HTTPRoute:
		return NewHTTPRoute(*obj), nil
	case *gateway_api_v1alpha2.GRPCRoute:
		return NewGRPCRoute(*obj), nil
	default:
		return nil, fmt.Errorf("unexpected route type for object %+v", object)
	}
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
	return &HTTPRouteStatus{&r.r.Status}
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

func (r *HTTPRoute) DeepCopy() Route {
	return &HTTPRoute{r: *r.r.DeepCopy()}
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

type RouteSpec interface {
	ParentRefs() []gateway_api_v1beta1.ParentReference
	Hostnames() []gateway_api_v1beta1.Hostname
	Rules() []RouteRule
	Equals(routeSpec RouteSpec) bool
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

func (s *HTTPRouteSpec) Equals(routeSpec RouteSpec) bool {
	_, ok := routeSpec.(*HTTPRouteSpec)
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
		// Assuming RouteRule also has an Equals method
		if !rule.Equals(otherRule) {
			return false
		}
	}

	return true
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
		// Assuming RouteRule also has an Equals method
		if !rule.Equals(otherRule) {
			return false
		}
	}

	return true
}

type RouteStatus interface {
	Parents() []gateway_api_v1beta1.RouteParentStatus
	SetParents(parents []gateway_api_v1beta1.RouteParentStatus)
}

type HTTPRouteStatus struct {
	s *gateway_api_v1beta1.HTTPRouteStatus
}

func (s *HTTPRouteStatus) Parents() []gateway_api_v1beta1.RouteParentStatus {
	return s.s.Parents
}

func (s *HTTPRouteStatus) SetParents(parents []gateway_api_v1beta1.RouteParentStatus) {
	s.s.Parents = parents
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

type RouteRule interface {
	BackendRefs() []BackendRef
	Matches() []RouteMatch
	Equals(routeRule RouteRule) bool
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

func (r *HTTPRouteRule) Equals(routeRule RouteRule) bool {
	other, ok := routeRule.(*HTTPRouteRule)
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

	// Compare Matches
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

	// Compare Matches
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

type BackendRef interface {
	Weight() *int32
	Group() *gateway_api_v1beta1.Group
	Kind() *gateway_api_v1beta1.Kind
	Name() gateway_api_v1beta1.ObjectName
	Namespace() *gateway_api_v1beta1.Namespace
	Port() *gateway_api_v1beta1.PortNumber
	Equals(backendRef BackendRef) bool
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

func (r *HTTPBackendRef) Equals(backendRef BackendRef) bool {
	other, ok := backendRef.(*HTTPBackendRef)
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

type RouteMatch interface {
	Headers() []HeaderMatch
	Equals(routeMatch RouteMatch) bool
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

func (m *HTTPRouteMatch) Equals(routeMatch RouteMatch) bool {
	other, ok := routeMatch.(*HTTPRouteMatch)
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

	if !reflect.DeepEqual(m.Path(), other.Path()) {
		return false
	}

	if !reflect.DeepEqual(m.QueryParams(), other.QueryParams()) {
		return false
	}

	if !reflect.DeepEqual(m.Method(), other.Method()) {
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

type HeaderMatch interface {
	Type() *gateway_api_v1beta1.HeaderMatchType
	Name() string
	Value() string
	Equals(headerMatch HeaderMatch) bool
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

func (m *HTTPHeaderMatch) Equals(headerMatch HeaderMatch) bool {
	other, ok := headerMatch.(*HTTPHeaderMatch)
	if !ok {
		return false
	}

	return m.Type() == other.Type() &&
		m.Name() == other.Name() &&
		m.Value() == other.Value()
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
