package eventhandlers

import (
	"context"
	"github.com/golang/glog"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	mcs_api "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

type enqueueRequestsForServiceExportEvent struct {
	client client.Client
}

func NewEqueueRequestServiceExportEvent(client client.Client) handler.EventHandler {
	return &enqueueRequestsForServiceExportEvent{
		client: client,
	}
}

func (h *enqueueRequestsForServiceExportEvent) Create(e event.CreateEvent, queue workqueue.RateLimitingInterface) {
	newServiceExport := e.Object.(*mcs_api.ServiceExport)
	h.enqueueImpactedService(queue, newServiceExport)
}

func (h *enqueueRequestsForServiceExportEvent) Update(e event.UpdateEvent, queue workqueue.RateLimitingInterface) {
	oldServiceExport := e.ObjectOld.(*mcs_api.ServiceExport)
	newServiceExport := e.ObjectNew.(*mcs_api.ServiceExport)

	if !equality.Semantic.DeepEqual(oldServiceExport, newServiceExport) {
		h.enqueueImpactedService(queue, newServiceExport)
	}
}

func (h *enqueueRequestsForServiceExportEvent) Delete(e event.DeleteEvent, queue workqueue.RateLimitingInterface) {
	glog.V(6).Infof("TODO serviceExport Delete \n")
}

func (h *enqueueRequestsForServiceExportEvent) Generic(e event.GenericEvent, queue workqueue.RateLimitingInterface) {

}

func (h *enqueueRequestsForServiceExportEvent) enqueueImpactedService(queue workqueue.RateLimitingInterface, serviceExport *mcs_api.ServiceExport) {
	glog.V(6).Infof("enqueueImpactedService, serviceExport[%v]\n", serviceExport)

	namespaceName := types.NamespacedName{
		Namespace: serviceExport.Namespace,
		Name:      serviceExport.Name,
	}

	svc := &corev1.Service{}

	if err := h.client.Get(context.TODO(), namespaceName, svc); err != nil {
		glog.V(6).Infof("enqueueImpactedService, unknown service [%v]\n", namespaceName)
		return
	}

	queue.Add(reconcile.Request{
		NamespacedName: namespaceName,
	})
}
