package core

import (
	"context"
	"fmt"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
	gateway_api_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gateway_api_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

type RouteType string

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

func ListAllRoutes(client client.Client, context context.Context) []Route {
	var routes []Route
	for _, route := range ListHTTPRoutes(client, context) {
		routes = append(routes, route)
	}
	for _, route := range ListGRPCRoutes(client, context) {
		routes = append(routes, route)
	}
	return routes
}

type RouteSpec interface {
	ParentRefs() []gateway_api_v1beta1.ParentReference
	Hostnames() []gateway_api_v1beta1.Hostname
	Rules() []RouteRule
	Equals(routeSpec RouteSpec) bool
}

type RouteStatus interface {
	Parents() []gateway_api_v1beta1.RouteParentStatus
	SetParents(parents []gateway_api_v1beta1.RouteParentStatus)
	UpdateParentRefs(parent gateway_api_v1beta1.ParentReference, controllerName gateway_api_v1beta1.GatewayController)
	UpdateRouteCondition(condition v1.Condition)
}

type RouteRule interface {
	BackendRefs() []BackendRef
	Matches() []RouteMatch
	Equals(routeRule RouteRule) bool
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

type RouteMatch interface {
	Headers() []HeaderMatch
	Equals(routeMatch RouteMatch) bool
}

type HeaderMatch interface {
	Type() *gateway_api_v1beta1.HeaderMatchType
	Name() string
	Value() string
	Equals(headerMatch HeaderMatch) bool
}
