package core

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

type RouteType string

type Route interface {
	Spec() RouteSpec
	Status() RouteStatus
	Name() string
	Namespace() string
	DeletionTimestamp() *metav1.Time
	DeepCopy() Route
	K8sObject() client.Object
}

func NewRoute(object client.Object) (Route, error) {
	switch obj := object.(type) {
	case *gwv1.HTTPRoute:
		return NewHTTPRoute(gwv1beta1.HTTPRoute(*obj)), nil
	case *gwv1beta1.HTTPRoute:
		return NewHTTPRoute(*obj), nil
	case *gwv1alpha2.GRPCRoute:
		return NewGRPCRoute(*obj), nil
	default:
		return nil, fmt.Errorf("unexpected route type for object %+v", object)
	}
}

func ListAllRoutes(context context.Context, client client.Client) ([]Route, error) {
	httpRoutes, err := ListHTTPRoutes(context, client)
	if err != nil {
		return nil, err
	}

	grpcRoutes, err := ListGRPCRoutes(context, client)
	if err != nil {
		return nil, err
	}

	var routes []Route
	routes = append(routes, httpRoutes...)
	routes = append(routes, grpcRoutes...)

	return routes, nil
}

type RouteSpec interface {
	ParentRefs() []gwv1beta1.ParentReference
	Hostnames() []gwv1beta1.Hostname
	Rules() []RouteRule
	Equals(routeSpec RouteSpec) bool
}

type RouteStatus interface {
	Parents() []gwv1beta1.RouteParentStatus
	SetParents(parents []gwv1beta1.RouteParentStatus)
	UpdateParentRefs(parent gwv1beta1.ParentReference, controllerName gwv1beta1.GatewayController)
	UpdateRouteCondition(condition metav1.Condition)
}

type RouteRule interface {
	BackendRefs() []BackendRef
	Matches() []RouteMatch
	Equals(routeRule RouteRule) bool
}

type BackendRef interface {
	Weight() *int32
	Group() *gwv1beta1.Group
	Kind() *gwv1beta1.Kind
	Name() gwv1beta1.ObjectName
	Namespace() *gwv1beta1.Namespace
	Port() *gwv1beta1.PortNumber
	Equals(backendRef BackendRef) bool
}

type RouteMatch interface {
	Headers() []HeaderMatch
	Equals(routeMatch RouteMatch) bool
}

type HeaderMatch interface {
	Type() *gwv1beta1.HeaderMatchType
	Name() string
	Value() string
	Equals(headerMatch HeaderMatch) bool
}
