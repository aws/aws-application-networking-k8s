package eventhandlers

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gateway_api "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

func NewEnqueueRequestsForGatewayClassEvent(log gwlog.Logger, client client.Client) handler.EventHandler {
	return &enqueueRequestsForGatewayClassEvent{
		log:    log,
		client: client,
	}
}

type enqueueRequestsForGatewayClassEvent struct {
	log    gwlog.Logger
	client client.Client
}

func (h *enqueueRequestsForGatewayClassEvent) Create(e event.CreateEvent, queue workqueue.RateLimitingInterface) {
	gwClassNew := e.Object.(*gateway_api.GatewayClass)
	h.enqueueImpactedGateway(queue, gwClassNew)
}

func (h *enqueueRequestsForGatewayClassEvent) Update(e event.UpdateEvent, queue workqueue.RateLimitingInterface) {
	h.log.Info("GatewayClass, Update")
}

func (h *enqueueRequestsForGatewayClassEvent) Delete(e event.DeleteEvent, queue workqueue.RateLimitingInterface) {
	h.log.Info("GatewayClass, Delete")
}

func (h *enqueueRequestsForGatewayClassEvent) Generic(e event.GenericEvent, queue workqueue.RateLimitingInterface) {
}

func (h *enqueueRequestsForGatewayClassEvent) enqueueImpactedGateway(queue workqueue.RateLimitingInterface, gwclass *gateway_api.GatewayClass) {

	gwList := &gateway_api.GatewayList{}

	h.client.List(context.TODO(), gwList)

	for _, gw := range gwList.Items {

		if string(gw.Spec.GatewayClassName) == string(gwclass.Name) {

			if gwclass.Spec.ControllerName == config.LatticeGatewayControllerName {
				h.log.Infof("Found matching gateway, %s", gw.Name)

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
