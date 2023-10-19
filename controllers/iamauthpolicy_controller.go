package controllers

import (
	"context"
	"fmt"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"

	k8serr "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type IAMAuthPolicyController struct {
	log    gwlog.Logger
	client client.Client
}

func RegisterIAMAuthPolicyController(log gwlog.Logger, mgr ctrl.Manager) error {
	controller := &IAMAuthPolicyController{
		log:    log,
		client: mgr.GetClient(),
	}
	err := ctrl.NewControllerManagedBy(mgr).
		For(&anv1alpha1.IAMAuthPolicy{}).
		Complete(controller)
	return err
}

func (c *IAMAuthPolicyController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	c.log.Infow("reconcile", "req", req)

	policy := &anv1alpha1.IAMAuthPolicy{}
	err := c.client.Get(ctx, req.NamespacedName, policy)
	if !k8serr.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	switch policy.Spec.TargetRef.Kind {
	case "Gateway":
		err = c.reconcileGateway(ctx, policy)
		break
	case "HTTPRoute":
	case "GRPCRoute":
		err = c.reconcileRoute(ctx, policy)
		break
	default:
		err = fmt.Errorf("unsupported targetRef type, req=%s, kind=%s",
			req, policy.Spec.TargetRef.Kind)
	}
	if err != nil {
		return ctrl.Result{}, err
	}

	c.log.Infow("successfully reconciled", "req", req)
	return ctrl.Result{}, nil
}

func (c *IAMAuthPolicyController) reconcileGateway(ctx context.Context, policy *anv1alpha1.IAMAuthPolicy) error {
	c.log.Debugw("reconcile gateway iam policy", "policy", policy)
	return nil
}

func (c IAMAuthPolicyController) reconcileRoute(ctx context.Context, policy *anv1alpha1.IAMAuthPolicy) error {
	c.log.Debugw("reconcile route iam policy", "policy", policy)
	return nil
}
