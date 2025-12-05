package core

import (
	"context"
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
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
	GroupKind() metav1.GroupKind
}

func NewRoute(object client.Object) (Route, error) {
	switch obj := object.(type) {
	case *gwv1.HTTPRoute:
		return NewHTTPRoute(*obj), nil
	case *gwv1.GRPCRoute:
		return NewGRPCRoute(*obj), nil
	case *gwv1alpha2.TLSRoute:
		return NewTLSRoute((*obj)), nil
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
	tlsRoutes, err := ListTLSRoutes(context, client)
	if err != nil {
		return nil, err
	}
	var routes []Route
	routes = append(routes, httpRoutes...)
	routes = append(routes, grpcRoutes...)
	routes = append(routes, tlsRoutes...)
	return routes, nil
}

type RouteSpec interface {
	ParentRefs() []gwv1.ParentReference
	Hostnames() []gwv1.Hostname
	Rules() []RouteRule
	Equals(routeSpec RouteSpec) bool
}

type RouteStatus interface {
	Parents() []gwv1.RouteParentStatus
	SetParents(parents []gwv1.RouteParentStatus)
	UpdateParentRefs(parent gwv1.ParentReference, controllerName gwv1.GatewayController)
	UpdateRouteCondition(parent gwv1.ParentReference, condition metav1.Condition)
}

type RouteRule interface {
	BackendRefs() []BackendRef
	Matches() []RouteMatch
	Equals(routeRule RouteRule) bool
}

type BackendRef interface {
	Weight() *int32
	Group() *gwv1.Group
	Kind() *gwv1.Kind
	Name() gwv1.ObjectName
	Namespace() *gwv1.Namespace
	Port() *gwv1.PortNumber
	Equals(backendRef BackendRef) bool
}

type RouteMatch interface {
	Headers() []HeaderMatch
	Equals(routeMatch RouteMatch) bool
}

type HeaderMatch interface {
	Type() *gwv1.HeaderMatchType
	Name() string
	Value() string
	Equals(headerMatch HeaderMatch) bool
}

// HasAllParentRefsRejected checks if all parentRefs are rejected
func HasAllParentRefsRejected(route Route) bool {
	rps := route.Status().Parents()
	if len(rps) == 0 {
		return true
	}

	for _, ps := range rps {
		for _, cnd := range ps.Conditions {
			if cnd.Type == string(gwv1.RouteConditionAccepted) && cnd.Status == metav1.ConditionTrue {
				return false
			}
		}
	}
	return true
}

// IsRouteAllowedByListener checks if route is allowed by listener's namespace and kind policies
// checks allowedRoutes.namespaces (Same, All, Selector) and allowedRoutes.kinds
func IsRouteAllowedByListener(ctx context.Context, k8sClient client.Client, route Route, gw *gwv1.Gateway, listener gwv1.Listener) (bool, error) {
	if !isRouteKindAllowedByListener(route, listener) {
		return false, nil
	}

	if listener.AllowedRoutes != nil && listener.AllowedRoutes.Namespaces != nil && listener.AllowedRoutes.Namespaces.From != nil {
		switch *listener.AllowedRoutes.Namespaces.From {
		case gwv1.NamespacesFromSame:
			return route.Namespace() == gw.Namespace, nil
		case gwv1.NamespacesFromAll:
			return true, nil
		case gwv1.NamespacesFromSelector:
			selector, err := metav1.LabelSelectorAsSelector(listener.AllowedRoutes.Namespaces.Selector)
			if err != nil {
				return false, fmt.Errorf("invalid label selector for listener %s: %w", listener.Name, err)
			}

			routeNs := &corev1.Namespace{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: route.Namespace()}, routeNs); err != nil {
				return false, fmt.Errorf("failed to get namespace %s for route %s/%s: %w",
					route.Namespace(), route.Namespace(), route.Name(), err)
			}
			return selector.Matches(labels.Set(routeNs.Labels)), nil
		default:
			// Unknown policy, default to same namespace
			return route.Namespace() == gw.Namespace, nil
		}
	}
	return route.Namespace() == gw.Namespace, nil
}

func isRouteKindAllowedByListener(route Route, listener gwv1.Listener) bool {
	routeKind := route.GroupKind().Kind

	if listener.AllowedRoutes != nil && len(listener.AllowedRoutes.Kinds) > 0 {
		for _, allowedKind := range listener.AllowedRoutes.Kinds {
			if string(allowedKind.Kind) == routeKind {
				return true
			}
		}
		return false
	}

	// No explicit kinds, use protocol-based defaults
	switch listener.Protocol {
	case gwv1.HTTPProtocolType:
		return routeKind == "HTTPRoute"
	case gwv1.HTTPSProtocolType:
		return routeKind == "HTTPRoute" || routeKind == "GRPCRoute"
	case gwv1.TLSProtocolType:
		return routeKind == "TLSRoute"
	default:
		return false
	}
}

func IsParentRefAccepted(route Route, parentRef gwv1.ParentReference) bool {
	for _, parent := range route.Status().Parents() {
		if reflect.DeepEqual(parent.ParentRef, parentRef) {
			for _, condition := range parent.Conditions {
				if condition.Type == string(gwv1.RouteConditionAccepted) {
					return condition.Status == metav1.ConditionTrue
				}
			}
		}
	}
	return false
}
