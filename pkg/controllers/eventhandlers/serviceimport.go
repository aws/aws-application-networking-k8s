package eventhandlers

import (
	"context"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
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
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
		return h.mapToRoute(ctx, obj, routeType)
	})
}

func (h *serviceImportEventHandler) mapToRoute(ctx context.Context, obj client.Object, routeType core.RouteType) []reconcile.Request {
	routes := h.mapper.ServiceImportToRoutes(ctx, obj.(*anv1alpha1.ServiceImport), routeType)

	var requests []reconcile.Request
	for _, route := range routes {
		routeName := k8s.NamespacedName(route.K8sObject())
		requests = append(requests, reconcile.Request{NamespacedName: routeName})
		h.log.Infow("ServiceImport resource change triggered Route update",
			"serviceName", obj.GetNamespace()+"/"+obj.GetName(), "routeName", routeName, "routeType", routeType)
	}
	return requests
}
