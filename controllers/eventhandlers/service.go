package eventhandlers

import (
	"context"
	"github.com/golang/glog"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	gateway_api "sigs.k8s.io/gateway-api/apis/v1beta1"
	mcs_api "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

type enqueueRequetsForServiceEvent struct {
	client client.Client
}

func NewEqueueRequestServiceEvent(client client.Client) handler.EventHandler {
	return &enqueueRequetsForServiceEvent{
		client: client,
	}
}

func (h *enqueueRequetsForServiceEvent) Create(e event.CreateEvent, queue workqueue.RateLimitingInterface) {
	service := e.Object.(*corev1.Service)
	h.enqueueImpactedServiceExport(queue, service)
}

func (h *enqueueRequetsForServiceEvent) Update(e event.UpdateEvent, queue workqueue.RateLimitingInterface) {
}

func (h *enqueueRequetsForServiceEvent) Delete(e event.DeleteEvent, queue workqueue.RateLimitingInterface) {
	service := e.Object.(*corev1.Service)
	h.enqueueImpactedServiceExport(queue, service)
}

func (h *enqueueRequetsForServiceEvent) Generic(e event.GenericEvent, queue workqueue.RateLimitingInterface) {

}

func (h *enqueueRequetsForServiceEvent) enqueueImpactedServiceExport(queue workqueue.RateLimitingInterface, ep *corev1.Service) {
	glog.V(6).Infof("enqueueImpactedServiceExport: %v\n", ep)

	srvExport := &mcs_api.ServiceExport{}
	namespacedName := types.NamespacedName{
		Namespace: ep.Namespace,
		Name:      ep.Name,
	}

	if err := h.client.Get(context.TODO(), namespacedName, srvExport); err != nil {
		glog.V(6).Infof("enqueueImpactedServiceExport, serviceexport not found %v\n", err)
		return
	}

	queue.Add(reconcile.Request{
		NamespacedName: namespacedName,
	})
}

type enqueueHTTPRequetsForServiceEvent struct {
	client client.Client
}

func NewEqueueHTTPRequestServiceEvent(client client.Client) handler.EventHandler {
	return &enqueueHTTPRequetsForServiceEvent{
		client: client,
	}
}

func (h *enqueueHTTPRequetsForServiceEvent) Create(e event.CreateEvent, queue workqueue.RateLimitingInterface) {
	service := e.Object.(*corev1.Service)
	h.enqueueImpactedHTTPRoute(queue, service)
}

func (h *enqueueHTTPRequetsForServiceEvent) Update(e event.UpdateEvent, queue workqueue.RateLimitingInterface) {
}

func (h *enqueueHTTPRequetsForServiceEvent) Delete(e event.DeleteEvent, queue workqueue.RateLimitingInterface) {
	service := e.Object.(*corev1.Service)
	h.enqueueImpactedHTTPRoute(queue, service)
}

func (h *enqueueHTTPRequetsForServiceEvent) Generic(e event.GenericEvent, queue workqueue.RateLimitingInterface) {

}

func (h *enqueueHTTPRequetsForServiceEvent) enqueueImpactedHTTPRoute(queue workqueue.RateLimitingInterface, ep *corev1.Service) {
	glog.V(6).Infof("enqueueImpactedHTTPRoute: %v\n", ep)

	httpRouteList := &gateway_api.HTTPRouteList{}

	h.client.List(context.TODO(), httpRouteList)

	for _, httpRoute := range httpRouteList.Items {
		if !isServiceUsedByHTTPRoute(httpRoute, ep) {
			continue
		}
		glog.V(6).Infof("enqueueImpactedHTTPRoute --> httproute %v \n", httpRoute)
		namespacedName := types.NamespacedName{
			Namespace: httpRoute.Namespace,
			Name:      httpRoute.Name,
		}
		queue.Add(reconcile.Request{
			NamespacedName: namespacedName,
		})

	}

}

func isServiceUsedByHTTPRoute(httpRoute gateway_api.HTTPRoute, ep *corev1.Service) bool {
	for _, httpRule := range httpRoute.Spec.Rules {
		for _, httpBackendRef := range httpRule.BackendRefs {
			//glog.V(6).Infof("isServiceUsedByHTTPRoute httpBackendRef %v, %v\n", httpBackendRef.BackendObjectReference, ep.Name)
			if string(*httpBackendRef.BackendObjectReference.Kind) != "service" {
				glog.V(6).Infof("isServiceUsedByHTTPRoute: kind %v\n", string(*httpBackendRef.BackendObjectReference.Kind))
				continue
			}

			if string(httpBackendRef.BackendObjectReference.Name) != ep.Name {
				//glog.V(6).Infof("isServiceUsedByHTTPRoute, name %v\n", string(httpBackendRef.BackendObjectReference.Name))
				continue
			}

			namespace := "default"

			if httpBackendRef.BackendObjectReference.Namespace != nil {
				namespace = string(*httpBackendRef.BackendObjectReference.Namespace)
			}

			if namespace != ep.Namespace {
				//glog.V(6).Infof("isServiceUsedByHTTPRoute, namespace %v\n", namespace)
				continue
			}

			return true

		}
	}
	return false

}
