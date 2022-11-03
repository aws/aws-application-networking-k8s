package eventhandlers

import (
	"context"
	"fmt"
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
	glog.V(6).Info("endpoint create")

	epNew := e.Object.(*corev1.Endpoints)
	h.enqueueImpactedService(queue, epNew)
}

func (h *enqueueRequestsForEndpointsEvent) Update(e event.UpdateEvent, queue workqueue.RateLimitingInterface) {
	glog.V(6).Info("endpoints Update")
	epOld := e.ObjectOld.(*corev1.Endpoints)
	epNew := e.ObjectNew.(*corev1.Endpoints)
	// fmt.Printf("endpoints update epOld [%v]  epNew[%v]\n", epOld, epNew)

	if !equality.Semantic.DeepEqual(epOld.Subsets, epNew.Subsets) {
		h.enqueueImpactedService(queue, epNew)
	}
}

func (h *enqueueRequestsForEndpointsEvent) Delete(e event.DeleteEvent, queue workqueue.RateLimitingInterface) {
	fmt.Printf("TODO endpoints Delete \n")
}

func (h *enqueueRequestsForEndpointsEvent) Generic(e event.GenericEvent, queue workqueue.RateLimitingInterface) {

}

func (h *enqueueRequestsForEndpointsEvent) enqueueImpactedService(queue workqueue.RateLimitingInterface, ep *corev1.Endpoints) {
	glog.V(6).Infof("enqueueImpactedService [%v]", ep)

	var targetIPList []string

	// building a IP list
	for _, endPoint := range ep.Subsets {

		for _, address := range endPoint.Addresses {

			targetIPList = append(targetIPList, address.IP)
		}

	}

	//fmt.Printf("--- targetIPList [%v]\n", targetIPList)

	svc := &corev1.Service{}
	namespaceName := types.NamespacedName{
		Namespace: ep.Namespace,
		Name:      ep.Name,
	}

	if err := h.client.Get(context.TODO(), namespaceName, svc); err != nil {
		glog.V(2).Infof("enqueueImpactedService, service not found %v\n", err)
		return
	}

	queue.Add(reconcile.Request{
		NamespacedName: namespaceName,
	})

	glog.V(6).Infof("Finished enqueueImpactedService [%v] targetIPLIST[%v]\n", ep, targetIPList)

}
