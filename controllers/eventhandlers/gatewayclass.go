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
	"sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/aws/aws-application-networking-k8s/pkg/config"
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
	gwClassNew := e.Object.(*v1alpha2.GatewayClass)
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

func (h *enqueueRequestsForGatewayClassEvent) enqueueImpactedGateway(queue workqueue.RateLimitingInterface, gwclass *v1alpha2.GatewayClass) {

	gwList := &v1alpha2.GatewayList{}

	h.client.List(context.TODO(), gwList)

	for _, gw := range gwList.Items {

		if string(gw.Spec.GatewayClassName) == string(gwclass.Name) {

			if gwclass.Spec.ControllerName == config.LatticeGatewayControllerName {
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
