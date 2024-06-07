package controllers

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	policy "github.com/aws/aws-application-networking-k8s/pkg/k8s/policyhelper"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

type (
	TGP = anv1alpha1.TargetGroupPolicy
)

type TargetGroupPolicyController struct {
	log    gwlog.Logger
	client client.Client
	ph     *policy.PolicyHandler[*TGP]
}

func RegisterTargetGroupPolicyController(log gwlog.Logger, mgr ctrl.Manager) error {
	ph := policy.NewTargetGroupPolicyHandler(log, mgr.GetClient())
	controller := &TargetGroupPolicyController{
		log:    log,
		client: mgr.GetClient(),
		ph:     ph,
	}

	b := ctrl.NewControllerManagedBy(mgr).
		For(&TGP{}, builder.WithPredicates(predicate.GenerationChangedPredicate{}))
	ph.AddWatchers(b, &corev1.Service{})
	ph.AddWatchers(b, &anv1alpha1.ServiceExport{})

	return b.Complete(controller)
}

func (c *TargetGroupPolicyController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctx = gwlog.StartReconcileTrace(ctx, c.log, "targetgrouppolicy", req.Name)
	defer func() {
		gwlog.EndReconcileTrace(ctx, c.log)
	}()

	tgPolicy := &TGP{}
	err := c.client.Get(ctx, req.NamespacedName, tgPolicy)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	c.log.Infow(ctx, "reconcile target group policy", "req", req, "targetRef", tgPolicy.Spec.TargetRef)

	_, err = c.ph.ValidateAndUpdateCondition(ctx, tgPolicy)
	if err != nil {
		return ctrl.Result{}, err
	}

	c.log.Infow(ctx, "reconciled target group policy",
		"req", req,
		"targetRef", tgPolicy.Spec.TargetRef,
	)
	return ctrl.Result{}, nil
}
