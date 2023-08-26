package eventhandlers

import (
	"context"

	corev1 "k8s.io/api/core/v1"
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

type enqueueRequestForServiceWithExportEvent struct {
	log    gwlog.Logger
	client client.Client
}

func NewEqueueRequestServiceWithExportEvent(log gwlog.Logger, client client.Client) handler.EventHandler {
	return &enqueueRequestForServiceWithExportEvent{
		log:    log,
		client: client,
	}
}

func (h *enqueueRequestForServiceWithExportEvent) Create(e event.CreateEvent, queue workqueue.RateLimitingInterface) {
	h.log.Info("Event: service create")
	service := e.Object.(*corev1.Service)
	h.enqueueImpactedService(queue, service)
	h.enqueueImpactedServiceExport(queue, service)
}

func (h *enqueueRequestForServiceWithExportEvent) Update(e event.UpdateEvent, queue workqueue.RateLimitingInterface) {
}

func (h *enqueueRequestForServiceWithExportEvent) Delete(e event.DeleteEvent, queue workqueue.RateLimitingInterface) {
	h.log.Info("Event: service delete")
	service := e.Object.(*corev1.Service)
	h.enqueueImpactedService(queue, service)
	h.enqueueImpactedServiceExport(queue, service)
}

func (h *enqueueRequestForServiceWithExportEvent) Generic(e event.GenericEvent, queue workqueue.RateLimitingInterface) {

}

func (h *enqueueRequestForServiceWithExportEvent) enqueueImpactedService(queue workqueue.RateLimitingInterface, ep *corev1.Service) {
	h.log.Infof("Event: enqueueImpactedService: service name: %s, service namespace: %s", ep.Name, ep.Namespace)

	srv := &corev1.Service{}
	namespacedName := types.NamespacedName{
		Namespace: ep.Namespace,
		Name:      ep.Name,
	}

	if err := h.client.Get(context.TODO(), namespacedName, srv); err != nil {
		h.log.Infof("Event: enqueueImpactedService, service not found %v\n", err)
		return
	}

	queue.Add(reconcile.Request{
		NamespacedName: namespacedName,
	})

}

func (h *enqueueRequestForServiceWithExportEvent) enqueueImpactedServiceExport(queue workqueue.RateLimitingInterface, ep *corev1.Service) {
	h.log.Infof("Event: enqueueImpactedServiceExport: service name %s, service namespace %s", ep.Name, ep.Namespace)

	srvExport := &mcs_api.ServiceExport{}
	namespacedName := types.NamespacedName{
		Namespace: ep.Namespace,
		Name:      ep.Name,
	}

	if err := h.client.Get(context.TODO(), namespacedName, srvExport); err != nil {
		h.log.Infof("Event: enqueueImpactedServiceExport, serviceexport not found %v\n", err)
		return
	}

	queue.Add(reconcile.Request{
		NamespacedName: namespacedName,
	})
}

type enqueueRequestForServiceWithRoutesEvent struct {
	log    gwlog.Logger
	client client.Client
}

func NewEnqueueRequestForServiceWithRoutesEvent(log gwlog.Logger, client client.Client) handler.EventHandler {
	return &enqueueRequestForServiceWithRoutesEvent{
		log:    log,
		client: client,
	}
}

func (h *enqueueRequestForServiceWithRoutesEvent) Create(e event.CreateEvent, queue workqueue.RateLimitingInterface) {
	service := e.Object.(*corev1.Service)
	h.enqueueImpactedRoutes(queue, service)
}

func (h *enqueueRequestForServiceWithRoutesEvent) Update(e event.UpdateEvent, queue workqueue.RateLimitingInterface) {
}

func (h *enqueueRequestForServiceWithRoutesEvent) Delete(e event.DeleteEvent, queue workqueue.RateLimitingInterface) {
	service := e.Object.(*corev1.Service)
	h.enqueueImpactedRoutes(queue, service)
}

func (h *enqueueRequestForServiceWithRoutesEvent) Generic(e event.GenericEvent, queue workqueue.RateLimitingInterface) {

}

func (h *enqueueRequestForServiceWithRoutesEvent) enqueueImpactedRoutes(queue workqueue.RateLimitingInterface, ep *corev1.Service) {
	h.log.Infof("Event: enqueueImpactedRoutes for service name %s, namespace %s", ep.Name, ep.Namespace)

	routes, err := core.ListAllRoutes(context.TODO(), h.client)
	if err != nil {
		h.log.Errorf("Failed to list all routes, %s", err)
		return
	}
	for _, route := range routes {
		if !isServiceUsedByRoute(route, ep) {
			continue
		}
		h.log.Infof("Event: enqueueImpactedRoutes --> route %v", route)
		namespacedName := types.NamespacedName{
			Namespace: route.Namespace(),
			Name:      route.Name(),
		}
		queue.Add(reconcile.Request{
			NamespacedName: namespacedName,
		})
	}
}

func isServiceUsedByRoute(route core.Route, ep *corev1.Service) bool {
	for _, rule := range route.Spec().Rules() {
		for _, backendRef := range rule.BackendRefs() {
			if string(*backendRef.Kind()) != "service" {
				continue
			}

			if string(backendRef.Name()) != ep.Name {
				continue
			}

			namespace := route.Namespace()
			if backendRef.Namespace() != nil {
				namespace = string(*backendRef.Namespace())
			}

			if namespace != ep.Namespace {
				continue
			}

			return true
		}
	}

	return false
}
