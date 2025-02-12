package core

import (
	"context"
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/aws/aws-application-networking-k8s/pkg/utils"
)

const (
	TlsRouteType RouteType = "tls"
	TlsRouteKind string    = "TLSRoute"
)

type TLSRoute struct {
	r gwv1alpha2.TLSRoute
}

func NewTLSRoute(route gwv1alpha2.TLSRoute) *TLSRoute {
	return &TLSRoute{r: route}
}

func GetTLSRoute(ctx context.Context, client client.Client, routeNamespacedName types.NamespacedName) (Route, error) {
	tlsRoute := &gwv1alpha2.TLSRoute{}
	err := client.Get(ctx, routeNamespacedName, tlsRoute)
	if err != nil {
		return nil, err
	}
	return NewTLSRoute(*tlsRoute), nil
}

func ListTLSRoutes(context context.Context, client client.Client) ([]Route, error) {
	routeList := &gwv1alpha2.TLSRouteList{}
	if err := client.List(context, routeList); err != nil {
		return nil, err
	}

	var routes []Route
	for _, route := range routeList.Items {
		routes = append(routes, NewTLSRoute(route))
	}
	return routes, nil
}

func (r *TLSRoute) Spec() RouteSpec {
	return &TLSRouteSpec{r.r.Spec}
}

func (r *TLSRoute) Status() RouteStatus {
	return &TLSRouteStatus{&r.r.Status}
}

func (r *TLSRoute) Name() string {
	return r.r.Name
}

func (r *TLSRoute) Namespace() string {
	return r.r.Namespace
}

func (r *TLSRoute) DeletionTimestamp() *metav1.Time {
	return r.r.DeletionTimestamp
}

func (r *TLSRoute) DeepCopy() Route {
	return &TLSRoute{r: *r.r.DeepCopy()}
}

func (r *TLSRoute) K8sObject() client.Object {
	return &r.r
}

func (r *TLSRoute) Inner() *gwv1alpha2.TLSRoute {
	return &r.r
}

func (r *TLSRoute) GroupKind() metav1.GroupKind {
	return metav1.GroupKind{
		Group: gwv1.GroupName,
		Kind:  TlsRouteKind,
	}
}

type TLSRouteSpec struct {
	s gwv1alpha2.TLSRouteSpec
}

func (s *TLSRouteSpec) ParentRefs() []gwv1.ParentReference {
	return s.s.ParentRefs
}

func (s *TLSRouteSpec) Hostnames() []gwv1.Hostname {
	return s.s.Hostnames
}

func (s *TLSRouteSpec) Rules() []RouteRule {
	var rules []RouteRule
	for _, rule := range s.s.Rules {
		rules = append(rules, &TLSRouteRule{rule})
	}
	return rules
}

func (s *TLSRouteSpec) Equals(routeSpec RouteSpec) bool {
	_, ok := routeSpec.(*TLSRouteSpec)
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

type TLSRouteStatus struct {
	s *gwv1alpha2.TLSRouteStatus
}

func (s *TLSRouteStatus) Parents() []gwv1.RouteParentStatus {
	return s.s.Parents
}

func (s *TLSRouteStatus) SetParents(parents []gwv1.RouteParentStatus) {
	s.s.Parents = parents
}

func (s *TLSRouteStatus) UpdateParentRefs(parent gwv1.ParentReference, controllerName gwv1.GatewayController) {
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

func (s *TLSRouteStatus) UpdateRouteCondition(parent gwv1.ParentReference, condition metav1.Condition) {
	for i, p := range s.Parents() {
		if p.ParentRef.Name == parent.Name {
			s.Parents()[i].Conditions = utils.GetNewConditions(p.Conditions, condition)
			return
		}
	}
}

type TLSRouteRule struct {
	r gwv1alpha2.TLSRouteRule
}

func (r *TLSRouteRule) BackendRefs() []BackendRef {
	var backendRefs []BackendRef
	for _, backendRef := range r.r.BackendRefs {
		backendRefs = append(backendRefs, &TLSBackendRef{backendRef})
	}
	return backendRefs
}

func (r *TLSRouteRule) Matches() []RouteMatch {
	var matches []RouteMatch
	// TLSRoute does not have any RouteMatch
	return matches
}

func (r *TLSRouteRule) Equals(routeRule RouteRule) bool {
	other, ok := routeRule.(*TLSRouteRule)
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

type TLSBackendRef struct {
	r gwv1.BackendRef
}

func (r *TLSBackendRef) Weight() *int32 {
	return r.r.Weight
}

func (r *TLSBackendRef) Group() *gwv1.Group {
	return r.r.Group
}

func (r *TLSBackendRef) Kind() *gwv1.Kind {
	return r.r.Kind
}

func (r *TLSBackendRef) Name() gwv1.ObjectName {
	return r.r.Name
}

func (r *TLSBackendRef) Namespace() *gwv1.Namespace {
	return r.r.Namespace
}

func (r *TLSBackendRef) Port() *gwv1.PortNumber {
	return r.r.Port
}

func (r *TLSBackendRef) Equals(backendRef BackendRef) bool {
	other, ok := backendRef.(*TLSBackendRef)
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
