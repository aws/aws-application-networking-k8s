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

func (h *enqueueRequestsForGatewayClassEvent) Create(ctx context.Context, e event.CreateEvent, queue workqueue.RateLimitingInterface) {
	gwClassNew := e.Object.(*gateway_api.GatewayClass)
	h.enqueueImpactedGateway(ctx, queue, gwClassNew)
}

func (h *enqueueRequestsForGatewayClassEvent) Update(ctx context.Context, e event.UpdateEvent, queue workqueue.RateLimitingInterface) {
}

func (h *enqueueRequestsForGatewayClassEvent) Delete(ctx context.Context, e event.DeleteEvent, queue workqueue.RateLimitingInterface) {
}

func (h *enqueueRequestsForGatewayClassEvent) Generic(ctx context.Context, e event.GenericEvent, queue workqueue.RateLimitingInterface) {
}

func (h *enqueueRequestsForGatewayClassEvent) enqueueImpactedGateway(
	ctx context.Context,
	queue workqueue.RateLimitingInterface,
	gwClass *gateway_api.GatewayClass,
) {
	gwList := &gateway_api.GatewayList{}
	err := h.client.List(ctx, gwList)
	if err != nil {
		h.log.Errorf("Error listing Gateways during GatewayClass event %s", err)
		return
	}

	for _, gw := range gwList.Items {
		if string(gw.Spec.GatewayClassName) == gwClass.Name {
			if gwClass.Spec.ControllerName == config.LatticeGatewayControllerName {
				h.log.Debugf("Found matching gateway, %s-%s", gw.Name, gw.Namespace)
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
