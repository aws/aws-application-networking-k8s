package eventhandlers

import (
	"context"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	mcs_api "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

type enqueueRequestsForServiceImportEvent struct {
	log       gwlog.Logger
	client    client.Client
	routeType core.RouteType
}

func NewEqueueRequestServiceImportEvent(log gwlog.Logger, client client.Client, routeType core.RouteType) handler.EventHandler {
	return &enqueueRequestsForServiceImportEvent{
		log:       log,
		client:    client,
		routeType: routeType,
	}
}

func (h *enqueueRequestsForServiceImportEvent) Create(e event.CreateEvent, queue workqueue.RateLimitingInterface) {
	newServiceImport := e.Object.(*mcs_api.ServiceImport)
	h.enqueueImpactedService(queue, newServiceImport)
}

func (h *enqueueRequestsForServiceImportEvent) Update(e event.UpdateEvent, queue workqueue.RateLimitingInterface) {
	oldServiceImport := e.ObjectOld.(*mcs_api.ServiceImport)
	newServiceImport := e.ObjectNew.(*mcs_api.ServiceImport)

	if !equality.Semantic.DeepEqual(oldServiceImport, newServiceImport) {
		h.enqueueImpactedService(queue, newServiceImport)
	}
}

func (h *enqueueRequestsForServiceImportEvent) Delete(e event.DeleteEvent, queue workqueue.RateLimitingInterface) {
	// TODO
	oldServiceImport := e.Object.(*mcs_api.ServiceImport)
	h.enqueueImpactedService(queue, oldServiceImport)

}

func (h *enqueueRequestsForServiceImportEvent) Generic(e event.GenericEvent, queue workqueue.RateLimitingInterface) {

}

func (h *enqueueRequestsForServiceImportEvent) enqueueImpactedService(queue workqueue.RateLimitingInterface, serviceImport *mcs_api.ServiceImport) {
	h.log.Infof("enqueueImpactedRoute, serviceImport[%v]", serviceImport)

	var routes []core.Route

	switch h.routeType {
	case core.HttpRouteType:
		routes = core.ListHTTPRoutes(h.client, context.TODO())
	case core.GrpcRouteType:
		routes = core.ListGRPCRoutes(h.client, context.TODO())
	default:
		h.log.Errorf("Invalid routeType %s", h.routeType)
	}

	for _, route := range routes {
		if !isServiceImportUsedByRoute(route, serviceImport) {
			continue
		}

		h.log.Infof("enqueueRequestsForServiceImportEvent: route name %s, namespace %s",
			route.Name(), route.Namespace())

		namespacedName := types.NamespacedName{
			Namespace: route.Namespace(),
			Name:      route.Name(),
		}

		queue.Add(reconcile.Request{
			NamespacedName: namespacedName,
		})
	}
}

func isServiceImportUsedByRoute(route core.Route, serviceImport *mcs_api.ServiceImport) bool {
	for _, rule := range route.Spec().Rules() {
		for _, backendRef := range rule.BackendRefs() {
			if string(*backendRef.Kind()) != "serviceimport" {
				continue
			}

			if string(backendRef.Name()) != serviceImport.Name {
				continue
			}

			namespace := route.Namespace()
			if backendRef.Namespace() != nil {
				namespace = string(*backendRef.Namespace())
			}

			if namespace != serviceImport.Namespace {
				continue
			}

			return true
		}
	}
	return false
}
