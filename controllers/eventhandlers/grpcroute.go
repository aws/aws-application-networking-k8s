package eventhandlers

import (
	"context"

	"sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type grpcRouteEventHandler struct {
	log    gwlog.Logger
	client client.Client
	mapper *resourceMapper
}

func NewGRPCRouteEventHandler(log gwlog.Logger, client client.Client) *grpcRouteEventHandler {
	return &grpcRouteEventHandler{log: log, client: client,
		mapper: &resourceMapper{log: log, client: client}}
}

func (h *grpcRouteEventHandler) MapToIAMAuthPolicies() handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
		var requests []reconcile.Request
		if route, ok := obj.(*v1alpha2.GRPCRoute); ok {
			policies := h.mapper.GRPCRouteToIAMAuthPolicies(context.Background(), route)
			for _, p := range policies {
				h.log.Infof("GRPCRoute [%s/%s] resource change triggers IAMAuthPolicy [%s/%s] resource change", route.Namespace, route.Name, p.Namespace, p.Name)
				requests = append(requests, reconcile.Request{NamespacedName: k8s.NamespacedName(p)})
			}
		}
		return requests
	})
}

func (h *grpcRouteEventHandler) MapToAccessLogPolicies() handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
		var requests []reconcile.Request
		if route, ok := obj.(*v1alpha2.GRPCRoute); ok {
			policies := h.mapper.GRPCRouteToAccessLogPolicies(context.Background(), route)
			for _, p := range policies {
				h.log.Infof("GRPCRoute [%s/%s] resource change triggers AccessLogPolicy [%s/%s] resource change", route.Namespace, route.Name, p.Namespace, p.Name)
				requests = append(requests, reconcile.Request{NamespacedName: k8s.NamespacedName(p)})
			}
		}
		return requests
	})
}
