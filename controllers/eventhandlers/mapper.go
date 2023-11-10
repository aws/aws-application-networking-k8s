package eventhandlers

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gateway_api_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gateway_api "sigs.k8s.io/gateway-api/apis/v1beta1"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

type resourceMapper struct {
	log    gwlog.Logger
	client client.Client
}

const (
	serviceKind       = "Service"
	serviceImportKind = "ServiceImport"
	gatewayKind       = "Gateway"
)

func (r *resourceMapper) ServiceToRoutes(ctx context.Context, svc *corev1.Service, routeType core.RouteType) []core.Route {
	if svc == nil {
		return nil
	}
	return r.backendRefToRoutes(ctx, svc, corev1.GroupName, serviceKind, routeType)
}

func (r *resourceMapper) ServiceImportToRoutes(ctx context.Context, svc *anv1alpha1.ServiceImport, routeType core.RouteType) []core.Route {
	if svc == nil {
		return nil
	}
	return r.backendRefToRoutes(ctx, svc, anv1alpha1.GroupName, serviceImportKind, routeType)
}

func (r *resourceMapper) ServiceToServiceExport(ctx context.Context, svc *corev1.Service) *anv1alpha1.ServiceExport {
	if svc == nil {
		return nil
	}
	svcExport := &anv1alpha1.ServiceExport{}
	if err := r.client.Get(ctx, k8s.NamespacedName(svc), svcExport); err != nil {
		return nil
	}
	return svcExport
}

func (r *resourceMapper) EndpointsToService(ctx context.Context, ep *corev1.Endpoints) *corev1.Service {
	if ep == nil {
		return nil
	}
	svc := &corev1.Service{}
	if err := r.client.Get(ctx, k8s.NamespacedName(ep), svc); err != nil {
		return nil
	}
	return svc
}

func (r *resourceMapper) TargetGroupPolicyToService(ctx context.Context, tgp *anv1alpha1.TargetGroupPolicy) *corev1.Service {
	return policyToTargetRefObj(r, ctx, tgp, &corev1.Service{})
}

func (r *resourceMapper) VpcAssociationPolicyToGateway(ctx context.Context, vap *anv1alpha1.VpcAssociationPolicy) *gateway_api.Gateway {
	return policyToTargetRefObj(r, ctx, vap, &gateway_api.Gateway{})
}

func policyToTargetRefObj[T client.Object](r *resourceMapper, ctx context.Context, policy core.Policy, retObj T) T {
	null := *new(T)
	if policy == nil {
		return null
	}
	policyNamespacedName := policy.GetNamespacedName()

	targetRef := policy.GetTargetRef()
	if targetRef == nil {
		r.log.Infow("Policy does not have targetRef, skipping",
			"policyName", policyNamespacedName)
		return null
	}
	expectedGroup, expectedKind, err := k8sResourceTypeToGroupAndKind(retObj)
	if err != nil {
		r.log.Errorw("Failed to get expected GroupKind for targetRefObj",
			"policyName", policyNamespacedName,
			"targetRef", targetRef,
			"reason", err.Error())
		return null
	}

	if targetRef.Group != expectedGroup || targetRef.Kind != expectedKind {
		r.log.Infow("Detected targetRef GroupKind and expected retObj GroupKind are different, skipping",
			"policyName", policyNamespacedName,
			"targetRef", targetRef,
			"expectedGroup", expectedGroup,
			"expectedKind", expectedKind)
		return null
	}
	if targetRef.Namespace != nil && policyNamespacedName.Namespace != string(*targetRef.Namespace) {
		r.log.Infow("Detected Policy and TargetRef namespace are different, skipping",
			"policyNamespacedName", policyNamespacedName, "targetRef", targetRef,
			"targetRef.Namespace", targetRef.Namespace,
			"policyNamespacedName.Namespace", policyNamespacedName.Namespace)
		return null
	}

	key := types.NamespacedName{
		Namespace: policyNamespacedName.Namespace,
		Name:      string(targetRef.Name),
	}
	if err := r.client.Get(ctx, key, retObj); err != nil {
		if errors.IsNotFound(err) {
			r.log.Debugw("Policy is referring to a non-existent targetRefObj, skipping",
				"policyName", policyNamespacedName, "targetRef", targetRef)
		} else {
			// Still gracefully skipping the event but errors other than NotFound are bad sign.
			r.log.Errorw("Failed to query targetRef of TargetGroupPolicy",
				"policyName", policyNamespacedName, "targetRef", targetRef, "reason", err.Error())
		}
		return null
	}
	r.log.Debugw("Policy change on Service detected",
		"policyName", policyNamespacedName, "targetRef", targetRef)

	return retObj
}

func k8sResourceTypeToGroupAndKind(obj client.Object) (gateway_api.Group, gateway_api.Kind, error) {
	switch obj.(type) {
	case *corev1.Service:
		return corev1.GroupName, serviceKind, nil
	case *gateway_api.Gateway:
		return gateway_api.GroupName, gatewayKind, nil
	default:
		return "", "", fmt.Errorf("un-registered obj type: %T", obj)
	}
}

func (r *resourceMapper) backendRefToRoutes(ctx context.Context, obj client.Object, group, kind string, routeType core.RouteType) []core.Route {
	if obj == nil {
		return nil
	}
	var routes []core.Route
	switch routeType {
	case core.HttpRouteType:
		routeList := &gateway_api.HTTPRouteList{}
		r.client.List(ctx, routeList)
		for _, k8sRoute := range routeList.Items {
			routes = append(routes, core.NewHTTPRoute(k8sRoute))
		}
	case core.GrpcRouteType:
		routeList := &gateway_api_v1alpha2.GRPCRouteList{}
		r.client.List(ctx, routeList)
		for _, k8sRoute := range routeList.Items {
			routes = append(routes, core.NewGRPCRoute(k8sRoute))
		}
	default:
		return nil
	}

	var filteredRoutes []core.Route
	for _, route := range routes {
		if r.isBackendRefUsedByRoute(route, obj, group, kind) {
			filteredRoutes = append(filteredRoutes, route)
		}
	}
	return filteredRoutes
}

func (r *resourceMapper) isBackendRefUsedByRoute(route core.Route, obj k8s.NamespacedAndNamed, group, kind string) bool {
	for _, rule := range route.Spec().Rules() {
		for _, backendRef := range rule.BackendRefs() {
			isGroupEqual := backendRef.Group() != nil && string(*backendRef.Group()) == group
			isKindEqual := backendRef.Kind() != nil && string(*backendRef.Kind()) == kind
			isNameEqual := string(backendRef.Name()) == obj.GetName()

			namespace := route.Namespace()
			if backendRef.Namespace() != nil {
				namespace = string(*backendRef.Namespace())
			}
			isNamespaceEqual := namespace == obj.GetNamespace()

			if isGroupEqual && isKindEqual && isNameEqual && isNamespaceEqual {
				return true
			}
		}
	}
	return false
}
