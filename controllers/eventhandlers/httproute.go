package eventhandlers

import (
	"context"

	"sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type httpRouteEventHandler struct {
	log    gwlog.Logger
	client client.Client
	mapper *resourceMapper
}

func NewHTTPRouteEventHandler(log gwlog.Logger, client client.Client) *httpRouteEventHandler {
	return &httpRouteEventHandler{log: log, client: client,
		mapper: &resourceMapper{log: log, client: client}}
}

func (h *httpRouteEventHandler) MapToIAMAuthPolicies() handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
		var requests []reconcile.Request
		if route, ok := obj.(*v1beta1.HTTPRoute); ok {
			policies := h.mapper.HTTPRouteToIAMAuthPolicies(context.Background(), route)
			for _, p := range policies {
				h.log.Infof("HTTPRoute [%s/%s] resource change triggers IAMAuthPolicy [%s/%s] resource change", route.Namespace, route.Name, p.Namespace, p.Name)
				requests = append(requests, reconcile.Request{NamespacedName: k8s.NamespacedName(p)})
			}
		}
		return requests
	})
}

func (h *httpRouteEventHandler) MapToAccessLogPolicies() handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
		var requests []reconcile.Request
		if route, ok := obj.(*v1beta1.HTTPRoute); ok {
			policies := h.mapper.HTTPRouteToAccessLogPolicies(context.Background(), route)
			for _, p := range policies {
				h.log.Infof("HTTPRoute [%s/%s] resource change triggers AccessLogPolicy [%s/%s] resource change", route.Namespace, route.Name, p.Namespace, p.Name)
				requests = append(requests, reconcile.Request{NamespacedName: k8s.NamespacedName(p)})
			}
		}
		return requests
	})
}
