package core

import (
	"context"
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/aws/aws-application-networking-k8s/pkg/utils"
)

const (
	HttpRouteType RouteType = "http"
)

type HTTPRoute struct {
	r gwv1.HTTPRoute
}

func NewHTTPRoute(route gwv1.HTTPRoute) *HTTPRoute {
	return &HTTPRoute{r: route}
}

func GetHTTPRoute(ctx context.Context, client client.Client, routeNamespacedName types.NamespacedName) (Route, error) {
	httpRoute := &gwv1.HTTPRoute{}
	err := client.Get(ctx, routeNamespacedName, httpRoute)
	if err != nil {
		return nil, err
	}
	return NewHTTPRoute(*httpRoute), nil
}

func ListHTTPRoutes(context context.Context, client client.Client) ([]Route, error) {
	routeList := &gwv1.HTTPRouteList{}
	if err := client.List(context, routeList); err != nil {
		return nil, err
	}

	var routes []Route
	for _, route := range routeList.Items {
		routes = append(routes, NewHTTPRoute(route))
	}
	return routes, nil
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

func (r *HTTPRoute) DeletionTimestamp() *metav1.Time {
	return r.r.DeletionTimestamp
}

func (r *HTTPRoute) DeepCopy() Route {
	return &HTTPRoute{r: *r.r.DeepCopy()}
}

func (r *HTTPRoute) K8sObject() client.Object {
	return &r.r
}

func (r *HTTPRoute) Inner() *gwv1.HTTPRoute {
	return &r.r
}

func (r *HTTPRoute) GroupKind() metav1.GroupKind {
	return metav1.GroupKind{
		Group: gwv1.GroupName,
		Kind:  "HTTPRoute",
	}
}

type HTTPRouteSpec struct {
	s gwv1.HTTPRouteSpec
}

func (s *HTTPRouteSpec) ParentRefs() []gwv1.ParentReference {
	return s.s.ParentRefs
}

func (s *HTTPRouteSpec) Hostnames() []gwv1.Hostname {
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
		if !rule.Equals(otherRule) {
			return false
		}
	}

	return true
}

type HTTPRouteStatus struct {
	s *gwv1.HTTPRouteStatus
}

func (s *HTTPRouteStatus) Parents() []gwv1.RouteParentStatus {
	return s.s.Parents
}

func (s *HTTPRouteStatus) SetParents(parents []gwv1.RouteParentStatus) {
	s.s.Parents = parents
}

func (s *HTTPRouteStatus) UpdateParentRefs(parent gwv1.ParentReference, controllerName gwv1.GatewayController) {
	for i, p := range s.Parents() {
		if p.ParentRef.Name == parent.Name {
			s.Parents()[i].ParentRef = parent
			s.Parents()[i].ControllerName = controllerName
			return
		}
	}
	s.SetParents(append(s.Parents(), gwv1.RouteParentStatus{
		ParentRef:      parent,
		ControllerName: controllerName,
	}))
}

func (s *HTTPRouteStatus) UpdateRouteCondition(parent gwv1.ParentReference, condition metav1.Condition) {
	for i, p := range s.Parents() {
		if p.ParentRef.Name == parent.Name {
			s.Parents()[i].Conditions = utils.GetNewConditions(p.Conditions, condition)
			return
		}
	}
}

type HTTPRouteRule struct {
	r gwv1.HTTPRouteRule
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

type HTTPBackendRef struct {
	r gwv1.HTTPBackendRef
}

func NewHTTPBackendRef(r gwv1.HTTPBackendRef) HTTPBackendRef {
	return HTTPBackendRef{r: r}
}

func (r *HTTPBackendRef) Weight() *int32 {
	return r.r.Weight
}

func (r *HTTPBackendRef) Group() *gwv1.Group {
	return r.r.Group
}

func (r *HTTPBackendRef) Kind() *gwv1.Kind {
	return r.r.Kind
}

func (r *HTTPBackendRef) Name() gwv1.ObjectName {
	return r.r.Name
}

func (r *HTTPBackendRef) Namespace() *gwv1.Namespace {
	return r.r.Namespace
}

func (r *HTTPBackendRef) Port() *gwv1.PortNumber {
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

type HTTPRouteMatch struct {
	m gwv1.HTTPRouteMatch
}

func (m *HTTPRouteMatch) Headers() []HeaderMatch {
	var headerMatches []HeaderMatch
	for _, headerMatch := range m.m.Headers {
		headerMatches = append(headerMatches, &HTTPHeaderMatch{headerMatch})
	}
	return headerMatches
}

func (m *HTTPRouteMatch) Path() *gwv1.HTTPPathMatch {
	return m.m.Path
}

func (m *HTTPRouteMatch) QueryParams() []gwv1.HTTPQueryParamMatch {
	return m.m.QueryParams
}

func (m *HTTPRouteMatch) Method() *gwv1.HTTPMethod {
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

type HTTPHeaderMatch struct {
	m gwv1.HTTPHeaderMatch
}

func (m *HTTPHeaderMatch) Type() *gwv1.HeaderMatchType {
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
