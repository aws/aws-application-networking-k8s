package eventhandlers

import (
	"context"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type enqueueRequestsForEndpointsEvent struct {
	log    gwlog.Logger
	client client.Client
}

func NewEnqueueRequestEndpointEvent(log gwlog.Logger, client client.Client) handler.EventHandler {
	return &enqueueRequestsForEndpointsEvent{
		log:    log,
		client: client,
	}
}

func (h *enqueueRequestsForEndpointsEvent) Create(e event.CreateEvent, queue workqueue.RateLimitingInterface) {
	h.log.Info("Event: endpoint create")

	epNew := e.Object.(*corev1.Endpoints)
	h.enqueueImpactedService(queue, epNew)
}

func (h *enqueueRequestsForEndpointsEvent) Update(e event.UpdateEvent, queue workqueue.RateLimitingInterface) {
	h.log.Info("Event: endpoints update")
	epOld := e.ObjectOld.(*corev1.Endpoints)
	epNew := e.ObjectNew.(*corev1.Endpoints)
	if !equality.Semantic.DeepEqual(epOld.Subsets, epNew.Subsets) {
		h.enqueueImpactedService(queue, epNew)
	}
}

func (h *enqueueRequestsForEndpointsEvent) Delete(e event.DeleteEvent, queue workqueue.RateLimitingInterface) {
	h.log.Infof("Event: endpoints delete")
	// service event handler handles this event here
}

func (h *enqueueRequestsForEndpointsEvent) Generic(e event.GenericEvent, queue workqueue.RateLimitingInterface) {

}

func (h *enqueueRequestsForEndpointsEvent) enqueueImpactedService(queue workqueue.RateLimitingInterface, ep *corev1.Endpoints) {
	h.log.Infof("Event: enqueueImpactedService for service name %s, namespace %s", ep.Name, ep.Namespace)

	var targetIPList []string
	for _, endPoint := range ep.Subsets {
		for _, address := range endPoint.Addresses {
			targetIPList = append(targetIPList, address.IP)
		}
	}

	svc := &corev1.Service{}
	namespaceName := types.NamespacedName{
		Namespace: ep.Namespace,
		Name:      ep.Name,
	}

	if err := h.client.Get(context.TODO(), namespaceName, svc); err != nil {
		h.log.Infof("Event: enqueueImpactedService, service not found %v\n", err)
		return
	}

	queue.Add(reconcile.Request{
		NamespacedName: namespaceName,
	})

	h.log.Infof("Finished enqueueImpactedService for service name %s, namespace %s targetIPLIST[%v]",
		ep.Name, ep.Namespace, targetIPList)
}
