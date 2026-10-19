package eventhandlers

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

type referenceGrantEventHandler struct {
	log    gwlog.Logger
	client client.Client
	mapper *resourceMapper
}

func NewReferenceGrantEventHandler(log gwlog.Logger, client client.Client) *referenceGrantEventHandler {
	return &referenceGrantEventHandler{
		log:    log,
		client: client,
		mapper: &resourceMapper{log: log, client: client},
	}
}

func (h *referenceGrantEventHandler) MapToRoute(routeType core.RouteType) handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
		return h.mapToRoute(ctx, obj, routeType)
	})
}

func (h *referenceGrantEventHandler) mapToRoute(ctx context.Context, obj client.Object, routeType core.RouteType) []reconcile.Request {
	grant, ok := obj.(*gwv1.ReferenceGrant)
	if !ok {
		return nil
	}

	var requests []reconcile.Request
	for _, route := range h.mapper.ReferenceGrantToRoutes(ctx, grant, routeType) {
		routeName := k8s.NamespacedName(route.K8sObject())
		requests = append(requests, reconcile.Request{NamespacedName: routeName})
		h.log.Infow(ctx, "ReferenceGrant change triggered Route update",
			"referenceGrant", grant.Namespace+"/"+grant.Name, "routeName", routeName, "routeType", routeType)
	}
	return requests
}
