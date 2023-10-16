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

	gateway_api "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/aws/aws-application-networking-k8s/pkg/config"
)

// TODO: Remove `enqueueRequestsForGatewayEvent`, and use `gatewayEventHandler` only
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

func (h *enqueueRequestsForGatewayEvent) Create(e event.CreateEvent, queue workqueue.RateLimitingInterface) {
	gwNew := e.Object.(*gateway_api.Gateway)

	h.log.Infof("Received Create event for Gateway %s-%s", gwNew.Name, gwNew.Namespace)

	// initialize transition time
	gwNew.Status.Conditions[0].LastTransitionTime = ZeroTransitionTime
	h.enqueueImpactedRoutes(queue)
}

func (h *enqueueRequestsForGatewayEvent) Update(e event.UpdateEvent, queue workqueue.RateLimitingInterface) {
	gwOld := e.ObjectOld.(*gateway_api.Gateway)
	gwNew := e.ObjectNew.(*gateway_api.Gateway)

	h.log.Infof("Received Update event for Gateway %s-%s", gwNew.GetName(), gwNew.GetNamespace())

	if !equality.Semantic.DeepEqual(gwOld.Spec, gwNew.Spec) {
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
	routes, err := core.ListAllRoutes(context.TODO(), h.client)
	if err != nil {
		h.log.Errorf("Failed to list all routes, %s", err)
		return
	}

	for _, route := range routes {
		if len(route.Spec().ParentRefs()) <= 0 {
			h.log.Debugf("Ignoring Route with no parentRef %s-%s", route.Name(), route.Namespace())
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
			h.log.Debugf("Ignoring Route with unknown parentRef %s-%s", route.Name(), route.Namespace())
			continue
		}

		// find the parent gateway class name
		gwClass := &gateway_api.GatewayClass{}
		gwClassName := types.NamespacedName{
			Namespace: "default",
			Name:      string(gw.Spec.GatewayClassName),
		}

		if err := h.client.Get(context.TODO(), gwClassName, gwClass); err != nil {
			h.log.Debugf("Ignoring Route with unknown Gateway %s-%s", route.Name(), route.Namespace())
			continue
		}

		if gwClass.Spec.ControllerName == config.LatticeGatewayControllerName {
			h.log.Debugf("Adding Route %s-%s to queue due to Gateway event", route.Name(), route.Namespace())
			queue.Add(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: route.Namespace(),
					Name:      route.Name(),
				},
			})
		}
	}
}

type gatewayEventHandler struct {
	log    gwlog.Logger
	client client.Client
	mapper *resourceMapper
}

func NewGatewayEventHandler(log gwlog.Logger, client client.Client) *gatewayEventHandler {
	return &gatewayEventHandler{log: log, client: client,
		mapper: &resourceMapper{log: log, client: client}}
}

func (h *gatewayEventHandler) MapToIAMAuthPolicies() handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
		var requests []reconcile.Request
		if gw, ok := obj.(*gateway_api.Gateway); ok {
			policies := h.mapper.GatewayToIAMAuthPolicies(context.Background(), gw)
			for _, p := range policies {
				h.log.Infof("Gateway [%s/%s] resource change triggers IAMAuthPolicy [%s/%s] resource change", gw.Namespace, gw.Name, p.Namespace, p.Name)
				requests = append(requests, reconcile.Request{NamespacedName: k8s.NamespacedName(p)})
			}
		}
		return requests
	})
}

func (h *gatewayEventHandler) MapToAccessLogPolicies() handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
		var requests []reconcile.Request
		if gw, ok := obj.(*gateway_api.Gateway); ok {
			policies := h.mapper.GatewayToAccessLogPolicies(context.Background(), gw)
			for _, p := range policies {
				h.log.Infof("Gateway [%s/%s] resource change triggers AccessLogPolicy [%s/%s] resource change", gw.Namespace, gw.Name, p.Namespace, p.Name)
				requests = append(requests, reconcile.Request{NamespacedName: k8s.NamespacedName(p)})
			}
		}
		return requests
	})
}
