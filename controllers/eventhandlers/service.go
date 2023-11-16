package eventhandlers

import (
	"context"
	"github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type serviceEventHandler struct {
	log    gwlog.Logger
	client client.Client
	mapper *resourceMapper
}

func NewServiceEventHandler(log gwlog.Logger, client client.Client) *serviceEventHandler {
	return &serviceEventHandler{log: log, client: client,
		mapper: &resourceMapper{log: log, client: client}}
}

func (h *serviceEventHandler) MapToRoute(routeType core.RouteType) handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
		return h.mapToRoute(ctx, obj, routeType)
	})
}

func (h *serviceEventHandler) MapToServiceExport() handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
		return h.mapToServiceExport(ctx, obj)
	})
}

func (h *serviceEventHandler) mapToServiceExport(ctx context.Context, obj client.Object) []reconcile.Request {
	var requests []reconcile.Request

	svc := h.mapToService(ctx, obj)
	svcExport := h.mapper.ServiceToServiceExport(ctx, svc)
	if svcExport != nil {
		requests = append(requests, reconcile.Request{
			NamespacedName: k8s.NamespacedName(svcExport),
		})
		h.log.Infow("Service impacting resource change triggered ServiceExport update",
			"serviceName", svc.Namespace+"/"+svc.Name)
	}
	return requests
}

func (h *serviceEventHandler) mapToService(ctx context.Context, obj client.Object) *corev1.Service {
	switch typed := obj.(type) {
	case *corev1.Service:
		return typed
	case *v1alpha1.TargetGroupPolicy:
		return h.mapper.TargetGroupPolicyToService(ctx, typed)
	case *corev1.Endpoints:
		return h.mapper.EndpointsToService(ctx, typed)
	}
	return nil
}

func (h *serviceEventHandler) mapToRoute(ctx context.Context, obj client.Object, routeType core.RouteType) []reconcile.Request {
	svc := h.mapToService(ctx, obj)
	routes := h.mapper.ServiceToRoutes(ctx, svc, routeType)

	var requests []reconcile.Request
	for _, route := range routes {
		routeName := k8s.NamespacedName(route.K8sObject())
		requests = append(requests, reconcile.Request{NamespacedName: routeName})
		h.log.Infow("Service impacting resource change triggered Route update",
			"serviceName", svc.Namespace+"/"+svc.Name, "routeName", routeName, "routeType", routeType)
	}
	return requests
}
