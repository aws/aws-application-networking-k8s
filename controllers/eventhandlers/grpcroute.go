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
	gateway_api_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

type enqueueRequestsForGRPCRouteEvent struct {
	client client.Client
}

func NewEnqueueRequestGRPCRouteEvent(client client.Client) handler.EventHandler {
	return &enqueueRequestsForGRPCRouteEvent{
		client: client,
	}
}

func (h *enqueueRequestsForGRPCRouteEvent) Create(e event.CreateEvent, queue workqueue.RateLimitingInterface) {
	glog.V(6).Info("GRPCRoute create")

	newGRPCRoute := e.Object.(*gateway_api_v1alpha2.GRPCRoute)
	h.enqueueImpactedService(queue, newGRPCRoute)
}

func (h *enqueueRequestsForGRPCRouteEvent) Update(e event.UpdateEvent, queue workqueue.RateLimitingInterface) {
	glog.V(6).Info("GRPCRoute update ")

	oldGRPCRoute := e.ObjectOld.(*gateway_api_v1alpha2.GRPCRoute)
	newGRPCRoute := e.ObjectNew.(*gateway_api_v1alpha2.GRPCRoute)

	if !equality.Semantic.DeepEqual(oldGRPCRoute.Spec, newGRPCRoute.Spec) {
		glog.V(6).Infof("--oldGRPCRoute %v \n", oldGRPCRoute.Spec)
		glog.V(6).Infof("--newGRPCRoute %v \n", newGRPCRoute.Spec)
		if newGRPCRoute.Status.RouteStatus.Parents != nil &&
			newGRPCRoute.Status.RouteStatus.Parents[0].Conditions[0].LastTransitionTime != ZeroTransitionTime {
			glog.V(6).Infof("Update Gateway Event -- reset LastTransitionTime for gateway %v\n", newGRPCRoute)
			newGRPCRoute.Status.RouteStatus.Parents[0].Conditions[0].LastTransitionTime = ZeroTransitionTime

		}
		h.enqueueImpactedService(queue, newGRPCRoute)
	}
}

func (h *enqueueRequestsForGRPCRouteEvent) Delete(e event.DeleteEvent, queue workqueue.RateLimitingInterface) {
	glog.V(6).Infof("TODO GRPCRoute Delete \n")
}

func (h *enqueueRequestsForGRPCRouteEvent) Generic(e event.GenericEvent, queue workqueue.RateLimitingInterface) {

}

func (h *enqueueRequestsForGRPCRouteEvent) enqueueImpactedService(queue workqueue.RateLimitingInterface, grpcRoute *gateway_api_v1alpha2.GRPCRoute) {
	glog.V(6).Infof("enqueueImpactedService [%v]\n", grpcRoute)

	for _, grpcRule := range grpcRoute.Spec.Rules {
		for _, grpcBackendRef := range grpcRule.BackendRefs {
			// TODOif grpcBackendRef.Kind == "service" {
			namespaceName := types.NamespacedName{
				Namespace: grpcRoute.Namespace,
				Name:      string(grpcBackendRef.Name),
			}

			svc := &corev1.Service{}
			if err := h.client.Get(context.TODO(), namespaceName, svc); err != nil {
				glog.V(6).Infof("enqueueRequestsForGRPCRouteEvent: unknown svc[%v]\n", namespaceName)
				continue
			}

			glog.V(6).Infof("enqueueRequestsForGRPCRouteEvent for svc[%v]\n", namespaceName)
			queue.Add(reconcile.Request{
				NamespacedName: namespaceName,
			})

			//}
		}
	}
}
