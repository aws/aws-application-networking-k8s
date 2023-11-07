package controllers

import (
	"context"
	"time"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	deploy "github.com/aws/aws-application-networking-k8s/pkg/deploy/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"

	"github.com/aws/aws-application-networking-k8s/controllers/eventhandlers"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

type vpcAssociationPolicyReconciler struct {
	log     gwlog.Logger
	client  client.Client
	cloud   pkg_aws.Cloud
	manager deploy.ServiceNetworkManager
}

func RegisterVpcAssociationPolicyController(log gwlog.Logger, mgr ctrl.Manager, cloud pkg_aws.Cloud) error {
	controller := &vpcAssociationPolicyReconciler{
		log:     log,
		client:  mgr.GetClient(),
		cloud:   cloud,
		manager: deploy.NewDefaultServiceNetworkManager(log, cloud),
	}
	eh := eventhandlers.NewPolicyEventHandler(log, mgr.GetClient(), &anv1alpha1.VpcAssociationPolicy{})
	err := ctrl.NewControllerManagedBy(mgr).
		For(&anv1alpha1.VpcAssociationPolicy{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(&source.Kind{Type: &gwv1beta1.Gateway{}}, eh.MapObjectToPolicy()).
		Complete(controller)
	return err
}

func (c *vpcAssociationPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	k8sPolicy := &anv1alpha1.VpcAssociationPolicy{}
	err := c.client.Get(ctx, req.NamespacedName, k8sPolicy)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	c.log.Infow("reconcile", "req", req, "targetRef", k8sPolicy.Spec.TargetRef)

	isDelete := !k8sPolicy.DeletionTimestamp.IsZero()
	isAssociation := k8sPolicy.Spec.AssociateWithVpc == nil || *k8sPolicy.Spec.AssociateWithVpc

	kind := k8sPolicy.Spec.TargetRef.Kind
	switch kind {
	case "Gateway":
		if isDelete || !isAssociation {
			err = c.delete(ctx, k8sPolicy)
		} else {
			err = c.upsert(ctx, k8sPolicy)
		}
	default:
		err = c.updatePolicyCondition(ctx, k8sPolicy, gwv1alpha2.PolicyReasonTargetNotFound)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	if err != nil {
		c.log.Infof("reconcile error, retry in 30 sec: %s", err)
		return ctrl.Result{RequeueAfter: time.Second * 30}, nil
	}

	err = c.handleFinalizer(ctx, k8sPolicy)
	if err != nil {
		return ctrl.Result{}, err
	}

	c.log.Infow("reconciled vpc association policy",
		"req", req,
		"targetRef", k8sPolicy.Spec.TargetRef,
		"isDeleted", isDelete,
	)
	return ctrl.Result{}, nil
}

func (c *vpcAssociationPolicyReconciler) handleFinalizer(ctx context.Context, k8sPolicy *anv1alpha1.VpcAssociationPolicy) error {
	finalizer := "vpcassociationpolicies.application-networking.k8s.aws/resources"
	if k8sPolicy.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(k8sPolicy, finalizer) {
			controllerutil.AddFinalizer(k8sPolicy, finalizer)
		}
	} else {
		if controllerutil.ContainsFinalizer(k8sPolicy, finalizer) {
			controllerutil.RemoveFinalizer(k8sPolicy, finalizer)
		}
	}
	return c.client.Update(ctx, k8sPolicy)
}

func (c *vpcAssociationPolicyReconciler) delete(ctx context.Context, k8sPolicy *anv1alpha1.VpcAssociationPolicy) error {
	snName := string(k8sPolicy.Spec.TargetRef.Name)
	err := c.manager.DeleteVpcAssociation(ctx, snName)
	if err != nil {
		return ignoreTargetRefNotFound(err)
	}
	return nil
}

func (c *vpcAssociationPolicyReconciler) upsert(ctx context.Context, k8sPolicy *anv1alpha1.VpcAssociationPolicy) error {
	snName := string(k8sPolicy.Spec.TargetRef.Name)
	var sgIds []*string
	for _, id := range k8sPolicy.Spec.SecurityGroupIds {
		strId := string(id)
		sgIds = append(sgIds, &strId)
	}
	snva, err := c.manager.UpsertVpcAssociation(ctx, snName, sgIds)
	if err != nil {
		return c.handleUpsertError(ctx, k8sPolicy, err)
	}
	err = c.updateLatticeAnnotation(ctx, k8sPolicy, snva)
	if err != nil {
		return err
	}
	err = c.updatePolicyCondition(ctx, k8sPolicy, gwv1alpha2.PolicyReasonAccepted)
	if err != nil {
		c.log.Debugf("seems like an error: %s, %s", k8sPolicy.GetNamespacedName(), k8sPolicy.GroupVersionKind().String())
		return err
	}
	return nil
}

func (c *vpcAssociationPolicyReconciler) updatePolicyCondition(ctx context.Context, k8sPolicy *anv1alpha1.VpcAssociationPolicy, reason gwv1alpha2.PolicyConditionReason) error {
	status := metav1.ConditionTrue
	if reason != gwv1alpha2.PolicyReasonAccepted {
		status = metav1.ConditionFalse
	}
	cnd := metav1.Condition{
		Type:   string(gwv1alpha2.PolicyConditionAccepted),
		Status: status,
		Reason: string(reason),
	}
	meta.SetStatusCondition(&k8sPolicy.Status.Conditions, cnd)
	err := c.client.Status().Update(ctx, k8sPolicy)
	return err
}

func (c *vpcAssociationPolicyReconciler) handleUpsertError(ctx context.Context, k8sPolicy *anv1alpha1.VpcAssociationPolicy, err error) error {
	switch {
	case services.IsNotFoundError(err):
		err = c.updatePolicyCondition(ctx, k8sPolicy, gwv1alpha2.PolicyReasonTargetNotFound)
	case services.IsConflictError(err):
		err = c.updatePolicyCondition(ctx, k8sPolicy, gwv1alpha2.PolicyReasonConflicted)
	}
	return err
}

func (c *vpcAssociationPolicyReconciler) updateLatticeAnnotation(ctx context.Context, k8sPolicy *anv1alpha1.VpcAssociationPolicy, resArn string) error {
	k8sPolicy.Annotations["application-networking.k8s.aws/resourceArn"] = resArn
	err := c.client.Update(ctx, k8sPolicy)
	return err
}
