package eventhandlers

import (
	"context"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"time"

	"github.com/golang/glog"

	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	gateway_api "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/aws/aws-application-networking-k8s/pkg/config"
)

type enqueueRequestsForGatewayEvent struct {
	client client.Client
}

func NewEnqueueRequestGatewayEvent(client client.Client) handler.EventHandler {
	return &enqueueRequestsForGatewayEvent{
		client: client,
	}
}

var ZeroTransitionTime = metav1.NewTime(time.Time{})

func (h *enqueueRequestsForGatewayEvent) Create(e event.CreateEvent, queue workqueue.RateLimitingInterface) {
	gwNew := e.Object.(*gateway_api.Gateway)
	glog.V(2).Infof("Gateway Create and Spec is %v", gwNew.Spec)

	// initialize transition time
	gwNew.Status.Conditions[0].LastTransitionTime = ZeroTransitionTime
	h.enqueueImpactedRoutes(queue)
}

func (h *enqueueRequestsForGatewayEvent) Update(e event.UpdateEvent, queue workqueue.RateLimitingInterface) {
	glog.V(6).Info("Gateway Update ")

	gwOld := e.ObjectOld.(*gateway_api.Gateway)
	gwNew := e.ObjectNew.(*gateway_api.Gateway)

	if !equality.Semantic.DeepEqual(gwOld.Spec, gwNew.Spec) {
		glog.V(2).Infof("Gateway Update old spec %v to new spec %v",
			gwOld.Spec, gwNew.Spec)
		// initialize transition time
		gwNew.Status.Conditions[0].LastTransitionTime = ZeroTransitionTime
		h.enqueueImpactedRoutes(queue)
	}
}

func (h *enqueueRequestsForGatewayEvent) Delete(e event.DeleteEvent, queue workqueue.RateLimitingInterface) {
	// TODO: delete gateway
}

func (h *enqueueRequestsForGatewayEvent) Generic(e event.GenericEvent, queue workqueue.RateLimitingInterface) {

}

func (h *enqueueRequestsForGatewayEvent) enqueueImpactedRoutes(queue workqueue.RateLimitingInterface) {
	routes := core.ListAllRoutes(h.client, context.TODO())
	for _, route := range routes {

		if len(route.Spec().ParentRefs()) <= 0 {
			glog.V(6).Infof("Ignore route no parentRefs %s", route.Name())
			continue
		}

		// find the parent gw object
		var gwNamespace = route.Namespace()
		if route.Spec().ParentRefs()[0].Namespace != nil {
			gwNamespace = string(*route.Spec().ParentRefs()[0].Namespace)
		}

		gwName := types.NamespacedName{
			Namespace: gwNamespace,
			Name:      string(route.Spec().ParentRefs()[0].Name),
		}

		gw := &gateway_api.Gateway{}
		if err := h.client.Get(context.TODO(), gwName, gw); err != nil {
			glog.V(6).Infof("Ignore Route with unknown parentRef %s", route.Name())
			continue
		}

		// find the parent gateway class name
		gwClass := &gateway_api.GatewayClass{}
		gwClassName := types.NamespacedName{
			Namespace: "default",
			Name:      string(gw.Spec.GatewayClassName),
		}

		if err := h.client.Get(context.TODO(), gwClassName, gwClass); err != nil {
			glog.V(6).Infof("Ignore Route with unknown Gateway %s", route.Name())
			continue
		}

		if gwClass.Spec.ControllerName == config.LatticeGatewayControllerName {
			glog.V(2).Infof("Trigger Route from Gateway event, route %s", route.Name())
			queue.Add(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: route.Namespace(),
					Name:      route.Name(),
				},
			})
		}

	}
}
