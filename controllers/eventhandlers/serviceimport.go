package eventhandlers

import (
	"context"

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/types"

	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"

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
	var err error

	switch h.routeType {
	case core.HttpRouteType:
		routes, err = core.ListHTTPRoutes(context.TODO(), h.client)
	case core.GrpcRouteType:
		routes, err = core.ListGRPCRoutes(context.TODO(), h.client)
	default:
		h.log.Errorf("Invalid routeType %s", h.routeType)
		return
	}

	if err != nil {
		h.log.Errorf("Error while listing routes, %s", err)
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
			if string(*backendRef.Kind()) != "ServiceImport" {
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
