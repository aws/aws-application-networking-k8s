package controllers

import (
	"context"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	deploy "github.com/aws/aws-application-networking-k8s/pkg/deploy/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	policy "github.com/aws/aws-application-networking-k8s/pkg/k8s/policyhelper"
	"github.com/aws/aws-application-networking-k8s/pkg/utils"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

type (
	VAP = anv1alpha1.VpcAssociationPolicy
)

const (
	finalizer = "vpcassociationpolicies.application-networking.k8s.aws/resources"
)

type vpcAssociationPolicyReconciler struct {
	log              gwlog.Logger
	client           client.Client
	cloud            pkg_aws.Cloud
	finalizerManager k8s.FinalizerManager
	manager          deploy.ServiceNetworkManager
	ph               *policy.PolicyHandler[*VAP]
}

func RegisterVpcAssociationPolicyController(log gwlog.Logger, cloud pkg_aws.Cloud, finalizerManager k8s.FinalizerManager, mgr ctrl.Manager) error {
	ph := policy.NewVpcAssociationPolicyHandler(log, mgr.GetClient())
	controller := &vpcAssociationPolicyReconciler{
		log:              log,
		client:           mgr.GetClient(),
		cloud:            cloud,
		finalizerManager: finalizerManager,
		manager:          deploy.NewDefaultServiceNetworkManager(log, cloud),
		ph:               ph,
	}

	b := ctrl.NewControllerManagedBy(mgr).
		For(&anv1alpha1.VpcAssociationPolicy{}, builder.WithPredicates(predicate.GenerationChangedPredicate{}))
	ph.AddWatchers(b, &gwv1beta1.Gateway{})
	return b.Complete(controller)
}

func (c *vpcAssociationPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctx = gwlog.NewTrace(ctx)
	gwlog.AddMetadata(ctx, "type", "vpcassociationpolicy")
	gwlog.AddMetadata(ctx, "name", req.Name)

	c.log.Infow(ctx, "reconcile starting", gwlog.GetMetadata(ctx)...)
	defer func() {
		c.log.Infow(ctx, "reconcile completed", gwlog.GetMetadata(ctx)...)
	}()

	k8sPolicy := &anv1alpha1.VpcAssociationPolicy{}
	err := c.client.Get(ctx, req.NamespacedName, k8sPolicy)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	c.log.Infow(ctx, "reconcile", "req", req, "targetRef", k8sPolicy.Spec.TargetRef)

	isDelete := !k8sPolicy.DeletionTimestamp.IsZero()
	isAssociation := k8sPolicy.Spec.AssociateWithVpc == nil || *k8sPolicy.Spec.AssociateWithVpc

	if isDelete || !isAssociation {
		err = c.delete(ctx, k8sPolicy)
	} else {
		err = c.upsert(ctx, k8sPolicy)
	}
	if err != nil {
		c.log.Infof(ctx, "reconcile error, retry in 30 sec: %s", err)
		return ctrl.Result{RequeueAfter: time.Second * 30}, nil
	}

	c.log.Infow(ctx, "reconciled vpc association policy",
		"req", req,
		"targetRef", k8sPolicy.Spec.TargetRef,
		"isDeleted", isDelete,
	)
	return ctrl.Result{}, nil
}

func (c *vpcAssociationPolicyReconciler) upsert(ctx context.Context, k8sPolicy *anv1alpha1.VpcAssociationPolicy) error {
	reason, err := c.ph.ValidateAndUpdateCondition(ctx, k8sPolicy)
	if err != nil {
		return err
	}
	if reason != policy.ReasonAccepted {
		return nil
	}

	err = c.finalizerManager.AddFinalizers(ctx, k8sPolicy, finalizer)
	if err != nil {
		return err
	}
	snName := string(k8sPolicy.Spec.TargetRef.Name)
	sgIds := utils.SliceMap(k8sPolicy.Spec.SecurityGroupIds, func(sg anv1alpha1.SecurityGroupId) *string {
		str := string(sg)
		return &str
	})
	snva, err := c.manager.UpsertVpcAssociation(ctx, snName, sgIds)
	if err != nil {
		return err
	}
	err = c.updateLatticeAnnotation(ctx, k8sPolicy, snva)
	if err != nil {
		return err
	}
	return nil
}

func (c *vpcAssociationPolicyReconciler) delete(ctx context.Context, k8sPolicy *anv1alpha1.VpcAssociationPolicy) error {
	snName := string(k8sPolicy.Spec.TargetRef.Name)
	err := c.manager.DeleteVpcAssociation(ctx, snName)
	if err != nil {
		return c.handleDeleteError(err)
	}
	err = c.finalizerManager.RemoveFinalizers(ctx, k8sPolicy, finalizer)
	if err != nil {
		return err
	}
	return nil
}

func (c *vpcAssociationPolicyReconciler) handleDeleteError(err error) error {
	switch {
	case services.IsNotFoundError(err):
		return nil
	}
	return err
}

func (c *vpcAssociationPolicyReconciler) updateLatticeAnnotation(ctx context.Context, k8sPolicy *anv1alpha1.VpcAssociationPolicy, resArn string) error {
	if k8sPolicy.Annotations == nil {
		k8sPolicy.Annotations = make(map[string]string)
	}
	k8sPolicy.Annotations["application-networking.k8s.aws/resourceArn"] = resArn
	err := c.client.Update(ctx, k8sPolicy)
	return err
}
