package eventhandlers

import (
	"context"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s/policyhelper"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type policyEventHandler[T core.Policy] struct {
	log    gwlog.Logger
	client client.Client
	policy T
}

func NewPolicyEventHandler[T core.Policy](log gwlog.Logger, client client.Client, policy T) *policyEventHandler[T] {
	return &policyEventHandler[T]{log: log, client: client, policy: policy}
}

func (h *policyEventHandler[T]) MapObjectToPolicy() handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(h.mapObjectToPolicy)
}

func (h *policyEventHandler[T]) mapObjectToPolicy(eventObj client.Object) []reconcile.Request {
	var requests []reconcile.Request
	policies, err := policyhelper.GetAttachedPolicies(context.Background(), h.client, k8s.NamespacedName(eventObj), *new(T))
	if err != nil {
		h.log.Errorf("Failed calling k8s operation: %s", err.Error())
		return requests
	}
	for _, p := range policies {
		requests = append(requests, reconcile.Request{
			NamespacedName: p.GetNamespacedName(),
		})
	}
	return requests
}
