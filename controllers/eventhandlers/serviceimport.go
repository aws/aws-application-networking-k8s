package eventhandlers

import (
	"context"

	"github.com/golang/glog"

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	gateway_api "sigs.k8s.io/gateway-api/apis/v1beta1"
	mcs_api "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

type enqueueRequestsForServiceImportEvent struct {
	client client.Client
}

func NewEqueueRequestServiceImportEvent(client client.Client) handler.EventHandler {
	return &enqueueRequestsForServiceImportEvent{
		client: client,
	}
}

func (h *enqueueRequestsForServiceImportEvent) Create(e event.CreateEvent, queue workqueue.RateLimitingInterface) {
	newServiceImport := e.Object.(*mcs_api.ServiceImport)
	h.enqueueImpactedService(queue, newServiceImport)
}

func (h *enqueueRequestsForServiceImportEvent) Update(e event.UpdateEvent, queue workqueue.RateLimitingInterface) {
	oldServiceImport := e.ObjectOld.(*mcs_api.ServiceImport)
	newServiceImport := e.ObjectNew.(*mcs_api.ServiceImport)

	if !equality.Semantic.DeepEqual(oldServiceImport, newServiceImport) {
		h.enqueueImpactedService(queue, newServiceImport)
	}
}

func (h *enqueueRequestsForServiceImportEvent) Delete(e event.DeleteEvent, queue workqueue.RateLimitingInterface) {
	glog.V(6).Infof("TODO serviceExport Delete \n")
	oldServiceImport := e.Object.(*mcs_api.ServiceImport)
	h.enqueueImpactedService(queue, oldServiceImport)

}

func (h *enqueueRequestsForServiceImportEvent) Generic(e event.GenericEvent, queue workqueue.RateLimitingInterface) {

}

func (h *enqueueRequestsForServiceImportEvent) enqueueImpactedService(queue workqueue.RateLimitingInterface, serviceImport *mcs_api.ServiceImport) {
	glog.V(6).Infof("enqueueImpactedHTTPRoute, serviceImport[%v]\n", serviceImport)

	httpRouteList := &gateway_api.HTTPRouteList{}

	h.client.List(context.TODO(), httpRouteList)

	for _, httpRoute := range httpRouteList.Items {
		if !isServiceImportUsedByHTTPRoute(httpRoute, serviceImport) {
			continue
		}

		glog.V(6).Infof("enqueueRequestsForServiceImportEvent --> httproute %v\n", httpRoute)
		namespacedName := types.NamespacedName{
			Namespace: httpRoute.Namespace,
			Name:      httpRoute.Name,
		}

		queue.Add(reconcile.Request{
			NamespacedName: namespacedName,
		})

	}

}

func isServiceImportUsedByHTTPRoute(httpRoute gateway_api.HTTPRoute, serviceImport *mcs_api.ServiceImport) bool {
	for _, httpRule := range httpRoute.Spec.Rules {
		for _, httpBackendRef := range httpRule.BackendRefs {
			if string(*httpBackendRef.BackendObjectReference.Kind) != "ServiceImport" {
				continue
			}

			if string(httpBackendRef.BackendObjectReference.Name) != serviceImport.Name {
				continue
			}

			namespace := httpRoute.Namespace
			if httpBackendRef.BackendObjectReference.Namespace != nil {
				namespace = string(*httpBackendRef.BackendObjectReference.Namespace)
			}

			if namespace != serviceImport.Namespace {
				continue
			}
			return true
		}
	}
	return false

}
