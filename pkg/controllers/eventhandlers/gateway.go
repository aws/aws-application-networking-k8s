package eventhandlers

import (
	"context"
	"time"

	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"

	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type enqueueRequestsForGatewayEvent struct {
	log    gwlog.Logger
	client client.Client
}

func NewEnqueueRequestGatewayEvent(log gwlog.Logger, client client.Client) handler.EventHandler {
	return &enqueueRequestsForGatewayEvent{
		log:    log,
		client: client,
	}
}

var ZeroTransitionTime = metav1.NewTime(time.Time{})

func (h *enqueueRequestsForGatewayEvent) Create(ctx context.Context, e event.CreateEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	gwNew := e.Object.(*gwv1.Gateway)

	h.log.Infof(ctx, "Received Create event for Gateway %s-%s", gwNew.Name, gwNew.Namespace)

	// initialize transition time
	gwNew.Status.Conditions[0].LastTransitionTime = ZeroTransitionTime
	h.enqueueImpactedRoutes(ctx, queue)
}

func (h *enqueueRequestsForGatewayEvent) Update(ctx context.Context, e event.UpdateEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	gwOld := e.ObjectOld.(*gwv1.Gateway)
	gwNew := e.ObjectNew.(*gwv1.Gateway)

	h.log.Infof(ctx, "Received Update event for Gateway %s-%s", gwNew.GetName(), gwNew.GetNamespace())

	if !equality.Semantic.DeepEqual(gwOld.Spec, gwNew.Spec) {
		// initialize transition time
		gwNew.Status.Conditions[0].LastTransitionTime = ZeroTransitionTime
		h.enqueueImpactedRoutes(ctx, queue)
	}
}

func (h *enqueueRequestsForGatewayEvent) Delete(ctx context.Context, e event.DeleteEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	// TODO: delete gateway
}

func (h *enqueueRequestsForGatewayEvent) Generic(ctx context.Context, e event.GenericEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {

}

func (h *enqueueRequestsForGatewayEvent) enqueueImpactedRoutes(ctx context.Context, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	routes, err := core.ListAllRoutes(ctx, h.client)
	if err != nil {
		h.log.Errorf(ctx, "Failed to list all routes, %s", err)
		return
	}

	for _, route := range routes {
		parents, err := k8s.FindControlledParents(ctx, h.client, route)
		// If there is one or more parents, even if an error occurs,
		// it is not an error related to the parent controlled by the Lattice Controller, so enqueue the route
		if len(parents) > 0 {
			// parents are controlled by lattice gateway controller, so enqueue the route
			h.log.Debugf(ctx, "Adding Route %s-%s to queue due to Gateway event", route.Name(), route.Namespace())
			queue.Add(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: route.Namespace(),
					Name:      route.Name(),
				},
			})
			continue
		}
		if err != nil {
			h.log.Debugf(ctx, "Ignoring Route with unknown parentRef %s-%s", route.Name(), route.Namespace())
			continue
		}
		h.log.Debugf(ctx, "Ignoring Route %s-%s with no controlled parent", route.Name(), route.Namespace())
	}
}
