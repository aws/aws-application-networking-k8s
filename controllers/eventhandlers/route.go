package eventhandlers

import (
	"context"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type enqueueRequestsForRouteEvent struct {
	log    gwlog.Logger
	client client.Client
}

func NewEnqueueRequestRouteEvent(log gwlog.Logger, client client.Client) handler.EventHandler {
	return &enqueueRequestsForRouteEvent{
		log:    log,
		client: client,
	}
}

func (h *enqueueRequestsForRouteEvent) Create(e event.CreateEvent, queue workqueue.RateLimitingInterface) {
	h.log.Infof("Received CreateRoute event for %s", e.Object.GetName())
	h.log.Debugf("CreateRoute event %+v", e)

	newRoute, err := core.NewRoute(e.Object)
	if err != nil {
		h.log.Errorf("Error while reading route in CreateRouteEvent %s", err)
	}

	h.enqueueImpactedService(queue, newRoute)
}

func (h *enqueueRequestsForRouteEvent) Update(e event.UpdateEvent, queue workqueue.RateLimitingInterface) {
	h.log.Infof("Received UpdateRoute event for %s", e.ObjectOld.GetName())
	h.log.Debugf("UpdateRoute event %+v", e)

	oldRoute, err := core.NewRoute(e.ObjectOld)
	if err != nil {
		h.log.Errorf("Error while reading old route in UpdateRouteEvent %s", err)
	}

	newRoute, err := core.NewRoute(e.ObjectNew)
	if err != nil {
		h.log.Errorf("Error while reading new route UpdateRouteEvent %s", err)
	}

	if !oldRoute.Spec().Equals(newRoute.Spec()) {
		h.log.Debugf("New and old route are different. Old: %+v, New: %+v", oldRoute.Spec(), newRoute.Spec())
		parents := newRoute.Status().Parents()
		if parents != nil && parents[0].Conditions[0].LastTransitionTime != ZeroTransitionTime {
			h.log.Info("Update Gateway Event, reset LastTransitionTime for gateway")
			parents[0].Conditions[0].LastTransitionTime = ZeroTransitionTime

		}
		h.enqueueImpactedService(queue, newRoute)
	}
}

func (h *enqueueRequestsForRouteEvent) Delete(e event.DeleteEvent, queue workqueue.RateLimitingInterface) {
	// TODO
}

func (h *enqueueRequestsForRouteEvent) Generic(e event.GenericEvent, queue workqueue.RateLimitingInterface) {

}

func (h *enqueueRequestsForRouteEvent) enqueueImpactedService(queue workqueue.RateLimitingInterface, route core.Route) {
	h.log.Infof("enqueueImpactedService [%v]", route)

	for _, rule := range route.Spec().Rules() {
		for _, backendRef := range rule.BackendRefs() {
			// TODOif backendRef.Kind == "service" {
			namespaceName := types.NamespacedName{
				Namespace: route.Namespace(),
				Name:      string(backendRef.Name()),
			}

			svc := &corev1.Service{}
			if err := h.client.Get(context.TODO(), namespaceName, svc); err != nil {
				h.log.Infof("Unknown service %+v", namespaceName)
				continue
			}

			h.log.Infof("Adding to service %+v to queue", namespaceName)
			queue.Add(reconcile.Request{
				NamespacedName: namespaceName,
			})

			//}
		}
	}
}
