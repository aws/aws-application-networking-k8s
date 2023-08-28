package eventhandlers

import (
	"context"
	"github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gateway_api_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gateway_api "sigs.k8s.io/gateway-api/apis/v1beta1"
	mcs_api "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

type resourceMapper struct {
	log    gwlog.Logger
	client client.Client
}

const (
	coreGroupName     = "" // empty means core by definition
	serviceKind       = "Service"
	serviceImportKind = "ServiceImport"
)

func (r *resourceMapper) ServiceToRoutes(ctx context.Context, svc *corev1.Service, routeType core.RouteType) []core.Route {
	if svc == nil {
		return nil
	}
	return r.backendRefToRoutes(ctx, svc, coreGroupName, serviceKind, routeType)
}

func (r *resourceMapper) ServiceImportToRoutes(ctx context.Context, svc *mcs_api.ServiceImport, routeType core.RouteType) []core.Route {
	if svc == nil {
		return nil
	}
	return r.backendRefToRoutes(ctx, svc, mcs_api.GroupName, serviceImportKind, routeType)
}

func (r *resourceMapper) ServiceToServiceExport(ctx context.Context, svc *corev1.Service) *mcs_api.ServiceExport {
	if svc == nil {
		return nil
	}
	svcExport := &mcs_api.ServiceExport{}
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

func (r *resourceMapper) TargetGroupPolicyToService(ctx context.Context, tgp *v1alpha1.TargetGroupPolicy) *corev1.Service {
	if tgp == nil {
		return nil
	}
	policyName := k8s.NamespacedName(tgp).String()

	targetRef := tgp.Spec.TargetRef
	if targetRef == nil {
		r.log.Infow("TargetGroupPolicy does not have targetRef, skipping",
			"policyName", policyName)
		return nil
	}
	if targetRef.Group != coreGroupName || targetRef.Kind != serviceKind {
		r.log.Infow("Detected non-Service TargetGroupPolicy attachment, skipping",
			"policyName", policyName, "targetRef", targetRef)
		return nil
	}
	namespace := tgp.Namespace
	if targetRef.Namespace != nil && namespace != string(*targetRef.Namespace) {
		r.log.Infow("Detected cross namespace TargetGroupPolicy attachment, skipping",
			"policyName", policyName, "targetRef", targetRef)
		return nil
	}

	svcName := types.NamespacedName{
		Namespace: namespace,
		Name:      string(targetRef.Name),
	}
	svc := &corev1.Service{}
	if err := r.client.Get(ctx, svcName, svc); err != nil {
		if errors.IsNotFound(err) {
			r.log.Debugw("TargetGroupPolicy is referring to non-existent service, skipping",
				"policyName", policyName, "serviceName", svcName.String())
		} else {
			// Still gracefully skipping the event but errors other than NotFound are bad sign.
			r.log.Errorw("Failed to query targetRef of TargetGroupPolicy",
				"policyName", policyName, "serviceName", svcName.String(), "reason", err.Error())
		}
		return nil
	}
	r.log.Debugw("TargetGroupPolicy change on Service detected",
		"policyName", policyName, "serviceName", svcName.String())

	return svc
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
