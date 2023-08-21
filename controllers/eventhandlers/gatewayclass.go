package eventhandlers

import (
	"context"

	"github.com/golang/glog"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gateway_api "sigs.k8s.io/gateway-api/apis/v1beta1"
)

func NewEnqueueRequestsForGatewayClassEvent(client client.Client) handler.EventHandler {
	return &enqueueRequestsForGatewayClassEvent{
		client: client,
	}
}

type enqueueRequestsForGatewayClassEvent struct {
	client client.Client
}

func (h *enqueueRequestsForGatewayClassEvent) Create(e event.CreateEvent, queue workqueue.RateLimitingInterface) {
	gwClassNew := e.Object.(*gateway_api.GatewayClass)
	h.enqueueImpactedGateway(queue, gwClassNew)
}

func (h *enqueueRequestsForGatewayClassEvent) Update(e event.UpdateEvent, queue workqueue.RateLimitingInterface) {
	glog.V(6).Info("GatwayClass, Update ")
}

func (h *enqueueRequestsForGatewayClassEvent) Delete(e event.DeleteEvent, queue workqueue.RateLimitingInterface) {
	glog.V(6).Info("GatewayClass, Delete")
}

func (h *enqueueRequestsForGatewayClassEvent) Generic(e event.GenericEvent, queue workqueue.RateLimitingInterface) {
}

func (h *enqueueRequestsForGatewayClassEvent) enqueueImpactedGateway(queue workqueue.RateLimitingInterface, gwclass *gateway_api.GatewayClass) {

	gwList := &gateway_api.GatewayList{}

	h.client.List(context.TODO(), gwList)

	for _, gw := range gwList.Items {

		if string(gw.Spec.GatewayClassName) == string(gwclass.Name) {

			if gwclass.Spec.ControllerName == LatticeGatewayControllerName {
				glog.V(6).Infof("Found matching gateway, %s\n", gw.Name)

				queue.Add(reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: gw.Namespace,
						Name:      gw.Name,
					},
				})

			}

		}

	}

}
