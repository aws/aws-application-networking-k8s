package eventhandlers

import (
	"context"

	"github.com/golang/glog"

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
	client client.Client
}

func NewEnqueueRequestEndpointEvent(client client.Client) handler.EventHandler {
	return &enqueueRequestsForEndpointsEvent{
		client: client,
	}
}

func (h *enqueueRequestsForEndpointsEvent) Create(e event.CreateEvent, queue workqueue.RateLimitingInterface) {
	glog.V(6).Info("Event: endpoint create")

	epNew := e.Object.(*corev1.Endpoints)
	h.enqueueImpactedService(queue, epNew)
}

func (h *enqueueRequestsForEndpointsEvent) Update(e event.UpdateEvent, queue workqueue.RateLimitingInterface) {
	glog.V(6).Info("Event: endpoints update")
	epOld := e.ObjectOld.(*corev1.Endpoints)
	epNew := e.ObjectNew.(*corev1.Endpoints)
	if !equality.Semantic.DeepEqual(epOld.Subsets, epNew.Subsets) {
		h.enqueueImpactedService(queue, epNew)
	}
}

func (h *enqueueRequestsForEndpointsEvent) Delete(e event.DeleteEvent, queue workqueue.RateLimitingInterface) {
	glog.V(6).Infof("Event: endpoints delete")
	// service event handler handles this event here
}

func (h *enqueueRequestsForEndpointsEvent) Generic(e event.GenericEvent, queue workqueue.RateLimitingInterface) {

}

func (h *enqueueRequestsForEndpointsEvent) enqueueImpactedService(queue workqueue.RateLimitingInterface, ep *corev1.Endpoints) {
	glog.V(6).Infof("Event: enqueueImpactedService [%v]", ep)

	var targetIPList []string

	// building a IP list
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
		glog.V(6).Infof("Event: enqueueImpactedService, service not found %v\n", err)
		return
	}

	queue.Add(reconcile.Request{
		NamespacedName: namespaceName,
	})

	glog.V(6).Infof("Finished enqueueImpactedService [%v] targetIPLIST[%v]\n", ep, targetIPList)

}
