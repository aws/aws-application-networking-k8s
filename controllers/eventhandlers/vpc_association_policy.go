package eventhandlers

import (
	"context"

	"github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type vpcAssociationPolicyEventHandler struct {
	log    gwlog.Logger
	client client.Client
	mapper *resourceMapper
}

func NewVpcAssociationPolicyEventHandler(log gwlog.Logger, client client.Client) *vpcAssociationPolicyEventHandler {
	return &vpcAssociationPolicyEventHandler{log: log, client: client,
		mapper: &resourceMapper{log: log, client: client}}
}

func (h *vpcAssociationPolicyEventHandler) MapToGateway() handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
		if vap, ok := obj.(*v1alpha1.VpcAssociationPolicy); ok {
			if gw := h.mapper.VpcAssociationPolicyToGateway(ctx, vap); gw != nil {
				return []reconcile.Request{{NamespacedName: k8s.NamespacedName(gw)}}
			}
		}
		return nil
	})
}
