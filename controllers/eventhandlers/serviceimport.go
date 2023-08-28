package eventhandlers

import (
	"context"

	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	mcs_api "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

type serviceImportEventHandler struct {
	log    gwlog.Logger
	client client.Client
	mapper *resourceMapper
}

func NewServiceImportEventHandler(log gwlog.Logger, client client.Client) *serviceImportEventHandler {
	return &serviceImportEventHandler{
		log:    log,
		client: client,
		mapper: &resourceMapper{log: log, client: client},
	}
}

func (h *serviceImportEventHandler) MapToRoute(routeType core.RouteType) handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
		return h.mapToRoute(obj, routeType)
	})
}

func (h *serviceImportEventHandler) mapToRoute(obj client.Object, routeType core.RouteType) []reconcile.Request {
	ctx := context.Background()
	routes := h.mapper.ServiceImportToRoutes(ctx, obj.(*mcs_api.ServiceImport), routeType)

	var requests []reconcile.Request
	for _, route := range routes {
		routeName := k8s.NamespacedName(route.K8sObject())
		requests = append(requests, reconcile.Request{NamespacedName: routeName})
		h.log.Infow("ServiceImport resource change triggered Route update",
			"serviceName", obj.GetNamespace()+"/"+obj.GetName(), "routeName", routeName, "routeType", routeType)
	}
	return requests
}
