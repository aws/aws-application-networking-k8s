package controllers

import (
	"context"
	"fmt"
	"time"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	deploy "github.com/aws/aws-application-networking-k8s/pkg/deploy/lattice"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

type IAMAuthPolicyController struct {
	log       gwlog.Logger
	client    client.Client
	policyMgr deploy.IAMAuthPolicyManager
}

func RegisterIAMAuthPolicyController(log gwlog.Logger, mgr ctrl.Manager, cloud pkg_aws.Cloud) error {
	controller := &IAMAuthPolicyController{
		log:       log,
		client:    mgr.GetClient(),
		policyMgr: deploy.IAMAuthPolicyManager{Cloud: cloud},
	}
	err := ctrl.NewControllerManagedBy(mgr).
		For(&anv1alpha1.IAMAuthPolicy{}).
		Complete(controller)
	return err
}

func (c *IAMAuthPolicyController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	k8sPolicy := &anv1alpha1.IAMAuthPolicy{}
	err := c.client.Get(ctx, req.NamespacedName, k8sPolicy)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	c.log.Infow("reconcile", "req", req, "targetRef", k8sPolicy.Spec.TargetRef)

	err = c.handleConflicts(ctx, k8sPolicy)
	if err != nil {
		c.log.Error(err)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	c.handleFinalizer(ctx, k8sPolicy)

	isDelete := !k8sPolicy.DeletionTimestamp.IsZero()
	kind := k8sPolicy.Spec.TargetRef.Kind
	var reconcileFunc func(context.Context, *anv1alpha1.IAMAuthPolicy) (string, error)
	switch kind {
	case "Gateway":
		if isDelete {
			reconcileFunc = c.deleteGatewayPolicy
		} else {
			reconcileFunc = c.upsertGatewayPolicy
		}
	case "HTTPRoute", "GRPCRoute":
		if isDelete {
			reconcileFunc = c.deleteRoutePolicy
		} else {
			reconcileFunc = c.upsertRoutePolicy
		}
	default:
		c.log.Errorw("unsupported targetRef", "kind", kind, "req", req)
		return ctrl.Result{RequeueAfter: time.Hour}, nil
	}

	latticeResourceId, err := reconcileFunc(ctx, k8sPolicy)
	if err != nil {
		// ignore Sn/Svc not found when deleting iam policy
		if !(isDelete && services.IsNotFoundError(err)) {
			c.log.Infof("reconcile error, retry in 30 sec: %s", err)
			return ctrl.Result{RequeueAfter: time.Second * 30}, nil
		}
	}

	k8sPolicy.Annotations["application-networking.k8s.aws/resourceId"] = latticeResourceId

	err = c.client.Update(ctx, k8sPolicy)
	if err != nil {
		return ctrl.Result{}, err
	}

	c.log.Infow("reconciled IAM policy",
		"req", req,
		"targetRef", k8sPolicy.Spec.TargetRef,
		"latticeResorceId", latticeResourceId,
		"isDeleted", isDelete,
	)
	return ctrl.Result{}, nil
}

func (c *IAMAuthPolicyController) handleFinalizer(ctx context.Context, k8sPolicy *anv1alpha1.IAMAuthPolicy) {
	authPolicyFinalizer := "iamauthpolicy.k8s.aws/resources"
	if k8sPolicy.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(k8sPolicy, authPolicyFinalizer) {
			controllerutil.AddFinalizer(k8sPolicy, authPolicyFinalizer)
		}
	} else {
		if controllerutil.ContainsFinalizer(k8sPolicy, authPolicyFinalizer) {
			controllerutil.RemoveFinalizer(k8sPolicy, authPolicyFinalizer)
		}
	}
}

func (c *IAMAuthPolicyController) deleteGatewayPolicy(ctx context.Context, k8sPolicy *anv1alpha1.IAMAuthPolicy) (string, error) {
	snId, err := c.findSnId(ctx, k8sPolicy)
	if err != nil {
		return "", err
	}
	err = c.policyMgr.Delete(ctx, snId)
	if err != nil {
		return "", err
	}
	err = c.policyMgr.DisableSnIAMAuth(ctx, snId)
	if err != nil {
		return "", err
	}
	return snId, nil
}

func (c *IAMAuthPolicyController) findSnId(ctx context.Context, k8sPolicy *anv1alpha1.IAMAuthPolicy) (string, error) {
	tr := k8sPolicy.Spec.TargetRef
	snInfo, err := c.policyMgr.Cloud.Lattice().FindServiceNetworkByK8sName(ctx, string(tr.Name))
	if err != nil {
		return "", err
	}
	return *snInfo.SvcNetwork.Id, nil
}

func (c *IAMAuthPolicyController) upsertGatewayPolicy(ctx context.Context, k8sPolicy *anv1alpha1.IAMAuthPolicy) (string, error) {
	snId, err := c.findSnId(ctx, k8sPolicy)
	if err != nil {
		return "", err
	}
	err = c.policyMgr.EnableSnIAMAuth(ctx, snId)
	if err != nil {
		return "", err
	}
	err = c.putPolicy(ctx, snId, k8sPolicy.Spec.Policy)
	if err != nil {
		return "", err
	}

	return snId, nil
}

func (c *IAMAuthPolicyController) findSvcId(ctx context.Context, k8sPolicy *anv1alpha1.IAMAuthPolicy) (string, error) {
	tr := k8sPolicy.Spec.TargetRef
	svcName := utils.LatticeServiceName(string(tr.Name), k8sPolicy.Namespace)
	svcInfo, err := c.policyMgr.Cloud.Lattice().FindServiceByK8sName(ctx, svcName)
	if err != nil {
		return "", err
	}
	return *svcInfo.Id, nil
}

func (c *IAMAuthPolicyController) deleteRoutePolicy(ctx context.Context, k8sPolicy *anv1alpha1.IAMAuthPolicy) (string, error) {
	svcId, err := c.findSvcId(ctx, k8sPolicy)
	if err != nil {
		return "", err
	}
	err = c.policyMgr.Delete(ctx, svcId)
	if err != nil {
		return "", err
	}
	err = c.policyMgr.DisableSvcIAMAuth(ctx, svcId)
	if err != nil {
		return "", err
	}
	return svcId, nil
}

func (c *IAMAuthPolicyController) upsertRoutePolicy(ctx context.Context, k8sPolicy *anv1alpha1.IAMAuthPolicy) (string, error) {
	svcId, err := c.findSvcId(ctx, k8sPolicy)
	if err != nil {
		return "", err
	}
	err = c.policyMgr.EnableSvcIAMAuth(ctx, svcId)
	if err != nil {
		return "", err
	}
	err = c.putPolicy(ctx, svcId, k8sPolicy.Spec.Policy)
	if err != nil {
		return "", err
	}
	return svcId, nil
}

func (c *IAMAuthPolicyController) putPolicy(ctx context.Context, resId, policy string) error {
	modelPolicy := model.IAMAuthPolicy{
		ResourceId: resId,
		Policy:     policy,
	}
	_, err := c.policyMgr.Put(ctx, modelPolicy)
	return err
}

func (c *IAMAuthPolicyController) handleConflicts(ctx context.Context, k8sPolicy *anv1alpha1.IAMAuthPolicy) error {
	if !k8sPolicy.DeletionTimestamp.IsZero() {
		return nil
	}
	conflictingPolicies, err := c.findConflictingPolicies(ctx, k8sPolicy)
	if err != nil {
		return err
	}
	if len(conflictingPolicies) > 0 {
		c.updatePolicyCondition(ctx, k8sPolicy, gwv1alpha2.PolicyReasonConflicted)
		return fmt.Errorf("conflict with other policies for same TargetRef, policy: %s, conflicted with: %v",
			k8sPolicy.Name, conflictingPolicies)
	}
	return nil
}

func (c *IAMAuthPolicyController) findConflictingPolicies(ctx context.Context, k8sPolicy *anv1alpha1.IAMAuthPolicy) ([]string, error) {
	var out []string
	policies := &anv1alpha1.IAMAuthPolicyList{}
	err := c.client.List(ctx, policies, &client.ListOptions{
		Namespace: k8sPolicy.Namespace,
	})
	if err != nil {
		return out, err
	}
	for _, p := range policies.Items {
		if k8sPolicy.Name == p.Name {
			continue
		}
		if *k8sPolicy.Spec.TargetRef == *p.Spec.TargetRef {
			out = append(out, p.Name)
		}
	}
	return out, nil
}

func (c IAMAuthPolicyController) updatePolicyCondition(ctx context.Context, k8sPolicy *anv1alpha1.IAMAuthPolicy, reason gwv1alpha2.PolicyConditionReason) error {
	status := metav1.ConditionTrue
	if reason != gwv1alpha2.PolicyReasonAccepted {
		status = metav1.ConditionFalse
	}
	cnd := metav1.Condition{
		Type:               string(gwv1alpha2.PolicyConditionAccepted),
		Status:             status,
		ObservedGeneration: k8sPolicy.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             string(reason),
	}
	meta.SetStatusCondition(&k8sPolicy.Status.Conditions, cnd)
	err := c.client.Status().Update(ctx, k8sPolicy)
	return err
}
