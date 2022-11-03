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
	"sigs.k8s.io/gateway-api/apis/v1alpha2"

	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

type enqueueRequestsForHTTPRouteEvent struct {
	client client.Client
}

func NewEnqueueRequestHTTPRouteEvent(client client.Client) handler.EventHandler {
	return &enqueueRequestsForHTTPRouteEvent{
		client: client,
	}
}

func (h *enqueueRequestsForHTTPRouteEvent) Create(e event.CreateEvent, queue workqueue.RateLimitingInterface) {
	glog.V(6).Info("HTTPRoute create")

	newHTTPRoute := e.Object.(*v1alpha2.HTTPRoute)
	h.enqueueImpactedService(queue, newHTTPRoute)
}

func (h *enqueueRequestsForHTTPRouteEvent) Update(e event.UpdateEvent, queue workqueue.RateLimitingInterface) {
	glog.V(6).Info("HTTPRoute update ")

	oldHTTPRoute := e.ObjectOld.(*v1alpha2.HTTPRoute)
	newHTTPRoute := e.ObjectNew.(*v1alpha2.HTTPRoute)

	if !equality.Semantic.DeepEqual(oldHTTPRoute.Spec, newHTTPRoute.Spec) {
		glog.V(6).Infof("--oldHTTPRoute %v \n", oldHTTPRoute.Spec)
		glog.V(6).Infof("--newHTTPRoute %v \n", newHTTPRoute.Spec)
		if newHTTPRoute.Status.RouteStatus.Parents != nil &&
			newHTTPRoute.Status.RouteStatus.Parents[0].Conditions[0].LastTransitionTime != ZeroTransitionTime {
			glog.V(6).Infof("Update Gateway Event -- reset LastTransitionTime for gateway %v\n", newHTTPRoute)
			newHTTPRoute.Status.RouteStatus.Parents[0].Conditions[0].LastTransitionTime = ZeroTransitionTime

		}
		h.enqueueImpactedService(queue, newHTTPRoute)
	}
}

func (h *enqueueRequestsForHTTPRouteEvent) Delete(e event.DeleteEvent, queue workqueue.RateLimitingInterface) {
	glog.V(6).Infof("TODO HTTPRoute Delete \n")
}

func (h *enqueueRequestsForHTTPRouteEvent) Generic(e event.GenericEvent, queue workqueue.RateLimitingInterface) {

}

func (h *enqueueRequestsForHTTPRouteEvent) enqueueImpactedService(queue workqueue.RateLimitingInterface, httpRoute *v1alpha2.HTTPRoute) {
	glog.V(6).Infof("enqueueImpactedService [%v]\n", httpRoute)

	for _, httpRule := range httpRoute.Spec.Rules {
		for _, httpBackendRef := range httpRule.BackendRefs {
			// TODOif httpBackendRef.Kind == "service" {
			namespaceName := types.NamespacedName{
				Namespace: httpRoute.Namespace,
				Name:      string(httpBackendRef.Name),
			}

			svc := &corev1.Service{}
			if err := h.client.Get(context.TODO(), namespaceName, svc); err != nil {
				glog.V(6).Infof("enqueueRequestsForHTTPRouteEvent: unknown svc[%v]\n", namespaceName)
				continue
			}

			glog.V(6).Infof("enqueueRequestsForHTTPRouteEvent for svc[%v]\n", namespaceName)
			queue.Add(reconcile.Request{
				NamespacedName: namespaceName,
			})

			//}
		}
	}
}
