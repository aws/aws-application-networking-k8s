package eventhandlers

import (
	"context"
	"github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gateway_api_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gateway_api "sigs.k8s.io/gateway-api/apis/v1beta1"
	mcs_api "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

type targetGroupPolicyEventHandler struct {
	log    gwlog.Logger
	client client.Client
}

func NewTargetGroupPolicyEventHandler(log gwlog.Logger, client client.Client) *targetGroupPolicyEventHandler {
	return &targetGroupPolicyEventHandler{log: log, client: client}
}

func (h *targetGroupPolicyEventHandler) getTargetRef(obj client.Object) *corev1.Service {
	tgp := obj.(*v1alpha1.TargetGroupPolicy)
	policyName := tgp.Namespace + "/" + tgp.Name

	targetRef := tgp.Spec.TargetRef
	if targetRef.Group != "" || targetRef.Kind != "Service" {
		h.log.Warnw("Detected non-Service TargetGroupPolicy attachment, skipping",
			"policyName", policyName, "targetRef", targetRef)
		return nil
	}
	namespace := tgp.Namespace
	if targetRef.Namespace != nil && namespace != string(*targetRef.Namespace) {
		h.log.Warnw("Detected cross namespace TargetGroupPolicy attachment, skipping",
			"policyName", policyName, "targetRef", targetRef)
		return nil
	}

	svcName := types.NamespacedName{
		Namespace: namespace,
		Name:      string(targetRef.Name),
	}
	svc := &corev1.Service{}
	if err := h.client.Get(context.TODO(), svcName, svc); err != nil {
		return nil
	}
	h.log.Debugw("TargetGroupPolicy change on Service detected",
		"policyName", policyName, "serviceName", svcName.String())

	return svc
}

func (h *targetGroupPolicyEventHandler) MapToHTTPRoute(obj client.Object) []reconcile.Request {
	var requests []reconcile.Request

	svc := h.getTargetRef(obj)
	if svc == nil {
		return nil
	}

	routeList := &gateway_api.HTTPRouteList{}
	h.client.List(context.TODO(), routeList)

	for _, httpRoute := range routeList.Items {
		if isServiceUsedByRoute(core.NewHTTPRoute(httpRoute), svc) {
			routeName := types.NamespacedName{
				Namespace: httpRoute.Namespace,
				Name:      httpRoute.Name,
			}
			requests = append(requests, reconcile.Request{NamespacedName: routeName})
			h.log.Infow("Service TargetGroupPolicy change triggering HTTPRoute update",
				"serviceName", svc.Namespace+"/"+svc.Name, "routeName", routeName.String())
		}
	}
	return requests
}

func (h *targetGroupPolicyEventHandler) MapToGRPCRoute(obj client.Object) []reconcile.Request {
	var requests []reconcile.Request

	svc := h.getTargetRef(obj)
	if svc == nil {
		return nil
	}

	routeList := &gateway_api_v1alpha2.GRPCRouteList{}
	h.client.List(context.TODO(), routeList)

	for _, grpcRoute := range routeList.Items {
		if isServiceUsedByRoute(core.NewGRPCRoute(grpcRoute), svc) {
			routeName := types.NamespacedName{
				Namespace: grpcRoute.Namespace,
				Name:      grpcRoute.Name,
			}
			requests = append(requests, reconcile.Request{NamespacedName: routeName})
			h.log.Infow("Service TargetGroupPolicy change triggering GRPCRoute update",
				"serviceName", svc.Namespace+"/"+svc.Name, "routeName", routeName.String())
		}
	}
	return requests
}

func (h *targetGroupPolicyEventHandler) MapToServiceExport(obj client.Object) []reconcile.Request {
	var requests []reconcile.Request

	svc := h.getTargetRef(obj)
	if svc == nil {
		return nil
	}
	svcName := types.NamespacedName{
		Namespace: svc.Namespace,
		Name:      svc.Name,
	}
	svcExport := &mcs_api.ServiceExport{}
	if err := h.client.Get(context.TODO(), svcName, svcExport); err != nil {
		return nil
	}
	requests = append(requests, reconcile.Request{
		NamespacedName: svcName,
	})
	return requests
}

func isServiceUsedByRoute(route core.Route, svc *corev1.Service) bool {
	for _, rule := range route.Spec().Rules() {
		for _, backendRef := range rule.BackendRefs() {
			if backendRef.Kind() != nil && string(*backendRef.Kind()) != "Service" {
				continue
			}
			if string(backendRef.Name()) != svc.Name {
				continue
			}

			namespace := route.Namespace()
			if backendRef.Namespace() != nil {
				namespace = string(*backendRef.Namespace())
			}
			if namespace != svc.Namespace {
				continue
			}
			return true
		}
	}
	return false
}
