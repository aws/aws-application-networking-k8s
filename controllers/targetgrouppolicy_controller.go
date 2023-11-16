package controllers

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

type TargetGroupPolicyController struct {
	log    gwlog.Logger
	client client.Client
}

func RegisterTargetGroupPolicyController(log gwlog.Logger, mgr ctrl.Manager) error {
	controller := &TargetGroupPolicyController{
		log:    log,
		client: mgr.GetClient(),
	}
	mapfn := targetGroupPolicyMapFunc(mgr.GetClient(), log)
	return ctrl.NewControllerManagedBy(mgr).
		For(&anv1alpha1.TargetGroupPolicy{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(&corev1.Service{}, handler.EnqueueRequestsFromMapFunc(mapfn)).
		Complete(controller)
}

func (c *TargetGroupPolicyController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	tgPolicy := &anv1alpha1.TargetGroupPolicy{}
	err := c.client.Get(ctx, req.NamespacedName, tgPolicy)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	c.log.Infow("reconcile target group policy", "req", req, "targetRef", tgPolicy.Spec.TargetRef)

	validationErr := c.validateSpec(ctx, tgPolicy)
	reason := validationErrToStatusReason(validationErr)
	msg := ""
	if validationErr != nil {
		msg = validationErr.Error()
	}
	c.updatePolicyCondition(tgPolicy, reason, msg)
	err = c.client.Status().Update(ctx, tgPolicy)
	if err != nil {
		return ctrl.Result{}, err
	}

	c.log.Infow("reconciled target group policy",
		"req", req,
		"targetRef", tgPolicy.Spec.TargetRef,
	)
	return ctrl.Result{}, nil
}

func (c *TargetGroupPolicyController) validateSpec(ctx context.Context, tgPolicy *anv1alpha1.TargetGroupPolicy) error {
	tr := tgPolicy.Spec.TargetRef
	if tr.Group != corev1.GroupName {
		return fmt.Errorf("%w: %s", GroupNameError, tr.Group)
	}
	if string(tr.Kind) != "Service" {
		return fmt.Errorf("%w: %s", KindError, tr.Kind)
	}
	conflictingPolicies, err := c.findConflictingPolicies(ctx, tgPolicy)
	if err != nil {
		return err
	}
	if len(conflictingPolicies) > 0 {
		return fmt.Errorf("%w, policies: %v", TargetRefConflict, conflictingPolicies)
	}
	refExists, err := c.targetRefExists(ctx, tgPolicy)
	if err != nil {
		return err
	}
	if !refExists {
		return fmt.Errorf("%w: %s", TargetRefNotFound, tr.Name)
	}
	return nil
}

func (c *TargetGroupPolicyController) targetRefExists(ctx context.Context, tgPolicy *anv1alpha1.TargetGroupPolicy) (bool, error) {
	tr := tgPolicy.Spec.TargetRef
	var obj client.Object
	switch tr.Kind {
	case "Service":
		obj = &corev1.Service{}
	default:
		panic("unexpected targetRef Kind=" + tr.Kind)
	}
	return k8s.ObjExists(ctx, c.client, types.NamespacedName{
		Namespace: tgPolicy.Namespace,
		Name:      string(tr.Name),
	}, obj)
}

func (c *TargetGroupPolicyController) findConflictingPolicies(ctx context.Context, tgPolicy *anv1alpha1.TargetGroupPolicy) ([]string, error) {
	var out []string
	policies := &anv1alpha1.TargetGroupPolicyList{}
	err := c.client.List(ctx, policies, &client.ListOptions{
		Namespace: tgPolicy.Namespace,
	})
	if err != nil {
		return out, err
	}
	for _, p := range policies.Items {
		if tgPolicy.Name == p.Name {
			continue
		}
		if *tgPolicy.Spec.TargetRef == *p.Spec.TargetRef {
			out = append(out, p.Name)
		}
	}
	return out, nil
}

func (c *TargetGroupPolicyController) updatePolicyCondition(tgPolicy *anv1alpha1.TargetGroupPolicy, reason gwv1alpha2.PolicyConditionReason, msg string) {
	status := metav1.ConditionTrue
	if reason != gwv1alpha2.PolicyReasonAccepted {
		status = metav1.ConditionFalse
	}
	cnd := metav1.Condition{
		Type:    string(gwv1alpha2.PolicyConditionAccepted),
		Status:  status,
		Reason:  string(reason),
		Message: msg,
	}
	meta.SetStatusCondition(&tgPolicy.Status.Conditions, cnd)
}

func targetGroupPolicyMapFunc(c client.Client, log gwlog.Logger) handler.MapFunc {
	return func(ctx context.Context, obj client.Object) []ctrl.Request {
		requests := []ctrl.Request{}
		policies := &anv1alpha1.TargetGroupPolicyList{}
		err := c.List(ctx, policies, &client.ListOptions{Namespace: obj.GetNamespace()})
		if err != nil {
			log.Error(err)
			return requests
		}
		for _, policy := range policies.Items {
			if obj.GetName() == string(policy.Spec.TargetRef.Name) {
				requests = append(requests, ctrl.Request{NamespacedName: policy.GetNamespacedName()})
			}
		}
		return requests
	}
}
