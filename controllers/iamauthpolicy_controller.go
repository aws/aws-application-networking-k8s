package controllers

import (
	"context"
	"errors"
	"fmt"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	deploy "github.com/aws/aws-application-networking-k8s/pkg/deploy/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"

	"golang.org/x/exp/slices"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

const (
	IAMAuthPolicyAnnotation      = "iam-auth-policy"
	IAMAuthPolicyAnnotationResId = k8s.AnnotationPrefix + IAMAuthPolicyAnnotation + "-resource-id"
	IAMAuthPolicyAnnotationType  = k8s.AnnotationPrefix + IAMAuthPolicyAnnotation + "-resource-type"
	IAMAuthPolicyFinalizer       = k8s.AnnotationPrefix + IAMAuthPolicyAnnotation
)

type IAMAuthPolicyController struct {
	log       gwlog.Logger
	client    client.Client
	policyMgr *deploy.IAMAuthPolicyManager
	cloud     pkg_aws.Cloud
}

func RegisterIAMAuthPolicyController(log gwlog.Logger, mgr ctrl.Manager, cloud pkg_aws.Cloud) error {
	controller := &IAMAuthPolicyController{
		log:       log,
		client:    mgr.GetClient(),
		policyMgr: deploy.NewIAMAuthPolicyManager(cloud),
		cloud:     cloud,
	}
	mapfn := iamAuthPolicyMapFunc(mgr.GetClient(), log)
	err := ctrl.NewControllerManagedBy(mgr).
		For(&anv1alpha1.IAMAuthPolicy{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(&source.Kind{Type: &gwv1beta1.Gateway{}},
			handler.EnqueueRequestsFromMapFunc(mapfn)).
		Watches(&source.Kind{Type: &gwv1beta1.HTTPRoute{}},
			handler.EnqueueRequestsFromMapFunc(mapfn)).
		Watches(&source.Kind{Type: &gwv1alpha2.GRPCRoute{}},
			handler.EnqueueRequestsFromMapFunc(mapfn)).
		Complete(controller)
	return err
}

// Reconciles IAMAuthPolicy CRD.
//
// IAMAuthPolicy has a plain text policy field and targetRef.Content of policy is not validated by
// controller, but Lattice API.
//
// TargetRef Kind can be Gatbeway, HTTPRoute, or GRPCRoute. Other Kinds will result in Invalid
// status.  Policy can be attached to single targetRef only. Attempt to attach more than 1 policy
// will result in Policy Conflict.  If policies created in sequence, the first one will be in
// Accepted status, and second in Conflict.  Any following updates to accepted policy will put it
// into conflicting status, and requires manual resolution - delete conflicting policy.
//
// Lattice side. Gateway attaches to Lattice ServiceNetwork, and HTTP/GRPCRoute to Service.  Policy
// attachment changes ServiceNetowrk and Service auth-type to IAM, and detachment to
// NONE. Successful creation of lattice policy updates k8s policy annotation with ARN/Id of Lattice
// Resouce
//
// Policy Attachment Spec is defined in [GEP-713]: https://gateway-api.sigs.k8s.io/geps/gep-713/.
func (c *IAMAuthPolicyController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	k8sPolicy := &anv1alpha1.IAMAuthPolicy{}
	err := c.client.Get(ctx, req.NamespacedName, k8sPolicy)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	c.log.Infow("reconcile IAM policy", "req", req, "targetRef", k8sPolicy.Spec.TargetRef)
	isDelete := !k8sPolicy.DeletionTimestamp.IsZero()
	var res ctrl.Result
	if isDelete {
		res, err = c.reconcileDelete(ctx, k8sPolicy)
	} else {
		res, err = c.reconcileUpsert(ctx, k8sPolicy)
	}
	if err != nil {
		return ctrl.Result{}, err
	}
	err = c.client.Update(ctx, k8sPolicy)
	if err != nil {
		return reconcile.Result{}, err
	}
	c.log.Infow("reconciled IAM policy",
		"req", req,
		"targetRef", k8sPolicy.Spec.TargetRef,
		"isDeleted", isDelete,
	)
	return res, nil
}

func (c *IAMAuthPolicyController) reconcileDelete(ctx context.Context, k8sPolicy *anv1alpha1.IAMAuthPolicy) (ctrl.Result, error) {
	err := c.validateSpec(ctx, k8sPolicy)
	if err == nil {
		modelPolicy := model.NewIAMAuthPolicy(k8sPolicy)
		_, err := c.policyMgr.Delete(ctx, modelPolicy)
		if err != nil {
			return ctrl.Result{}, services.IgnoreNotFound(err)
		}
	}
	err = c.handleLatticeResourceChange(ctx, k8sPolicy, model.IAMAuthPolicyStatus{})
	if err != nil {
		return ctrl.Result{}, err
	}
	c.removeFinalizer(k8sPolicy)
	return ctrl.Result{}, nil
}

func (c *IAMAuthPolicyController) reconcileUpsert(ctx context.Context, k8sPolicy *anv1alpha1.IAMAuthPolicy) (ctrl.Result, error) {
	validationErr := c.validateSpec(ctx, k8sPolicy)
	err := c.updateStatus(ctx, k8sPolicy, validationErr)
	if err != nil {
		return ctrl.Result{}, err
	}
	var statusPolicy model.IAMAuthPolicyStatus
	if validationErr == nil {
		modelPolicy := model.NewIAMAuthPolicy(k8sPolicy)
		c.addFinalizer(k8sPolicy)
		err = c.client.Update(ctx, k8sPolicy)
		if err != nil {
			return reconcile.Result{}, err
		}
		statusPolicy, err = c.policyMgr.Put(ctx, modelPolicy)
		if err != nil {
			return reconcile.Result{}, services.IgnoreNotFound(err)
		}
		c.updateLatticeAnnotaion(k8sPolicy, statusPolicy.ResourceId, modelPolicy.Type)
	}
	err = c.handleLatticeResourceChange(ctx, k8sPolicy, statusPolicy)
	if err != nil {
		return reconcile.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (c *IAMAuthPolicyController) validateSpec(ctx context.Context, k8sPolicy *anv1alpha1.IAMAuthPolicy) error {
	tr := k8sPolicy.Spec.TargetRef
	if tr.Group != gwv1beta1.GroupName {
		return fmt.Errorf("%w: %s", GroupNameError, tr.Group)
	}
	if !slices.Contains([]string{"Gateway", "HTTPRoute", "GRPCRoute"}, string(tr.Kind)) {
		return fmt.Errorf("%w: %s", KindError, tr.Kind)
	}
	refExists, err := c.targetRefExists(ctx, k8sPolicy)
	if err != nil {
		return err
	}
	if !refExists {
		return fmt.Errorf("%w: %s", TargetRefNotFound, tr.Name)
	}
	conflictingPolicies, err := c.findConflictingPolicies(ctx, k8sPolicy)
	if err != nil {
		return err
	}
	if len(conflictingPolicies) > 0 {
		return fmt.Errorf("%w, policies: %v", TargetRefConflict, conflictingPolicies)
	}
	return nil
}

func (c *IAMAuthPolicyController) updateStatus(ctx context.Context, k8sPolicy *anv1alpha1.IAMAuthPolicy, validationErr error) error {
	reason := validationErrToStatusReason(validationErr)
	msg := ""
	if validationErr != nil {
		msg = validationErr.Error()
	}
	c.updatePolicyCondition(k8sPolicy, reason, msg)
	err := c.client.Status().Update(ctx, k8sPolicy)
	return err
}

func validationErrToStatusReason(validationErr error) gwv1alpha2.PolicyConditionReason {
	var reason gwv1alpha2.PolicyConditionReason
	switch {
	case validationErr == nil:
		reason = gwv1alpha2.PolicyReasonAccepted
	case errors.Is(validationErr, GroupNameError) || errors.Is(validationErr, KindError):
		reason = gwv1alpha2.PolicyReasonInvalid
	case errors.Is(validationErr, TargetRefNotFound):
		reason = gwv1alpha2.PolicyReasonTargetNotFound
	case errors.Is(validationErr, TargetRefConflict):
		reason = gwv1alpha2.PolicyReasonConflicted
	default:
		panic("unexpected validation error: " + validationErr.Error())
	}
	return reason
}

func (c *IAMAuthPolicyController) targetRefExists(ctx context.Context, k8sPolicy *anv1alpha1.IAMAuthPolicy) (bool, error) {
	tr := k8sPolicy.Spec.TargetRef
	var obj client.Object
	switch tr.Kind {
	case "Gateway":
		obj = &gwv1beta1.Gateway{}
	case "HTTPRoute":
		obj = &gwv1beta1.HTTPRoute{}
	case "GRPCRoute":
		obj = &gwv1alpha2.GRPCRoute{}
	default:
		panic("unexpected targetRef Kind=" + tr.Kind)
	}
	return k8s.ObjExists(ctx, c.client, types.NamespacedName{
		Namespace: k8sPolicy.Namespace,
		Name:      string(tr.Name),
	}, obj)
}

func (c *IAMAuthPolicyController) removeFinalizer(k8sPolicy *anv1alpha1.IAMAuthPolicy) {
	if controllerutil.ContainsFinalizer(k8sPolicy, IAMAuthPolicyFinalizer) {
		controllerutil.RemoveFinalizer(k8sPolicy, IAMAuthPolicyFinalizer)
	}
}

func (c *IAMAuthPolicyController) addFinalizer(k8sPolicy *anv1alpha1.IAMAuthPolicy) {
	if !controllerutil.ContainsFinalizer(k8sPolicy, IAMAuthPolicyFinalizer) {
		controllerutil.AddFinalizer(k8sPolicy, IAMAuthPolicyFinalizer)
	}
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

// cleanup lattice resources after targetRef changes
func (c *IAMAuthPolicyController) handleLatticeResourceChange(ctx context.Context, k8sPolicy *anv1alpha1.IAMAuthPolicy, statusPolicy model.IAMAuthPolicyStatus) error {
	prevModel, ok := c.getLatticeAnnotation(k8sPolicy)
	if !ok {
		return nil
	}
	if prevModel.ResourceId != statusPolicy.ResourceId {
		_, err := c.policyMgr.Delete(ctx, prevModel)
		if err != nil {
			return services.IgnoreNotFound(err)
		}
	}
	return nil
}

func (c *IAMAuthPolicyController) updatePolicyCondition(k8sPolicy *anv1alpha1.IAMAuthPolicy, reason gwv1alpha2.PolicyConditionReason, msg string) {
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
	meta.SetStatusCondition(&k8sPolicy.Status.Conditions, cnd)
}

func iamAuthPolicyMapFunc(c client.Client, log gwlog.Logger) handler.MapFunc {
	return func(obj client.Object) []ctrl.Request {
		requests := []ctrl.Request{}
		policies := &anv1alpha1.IAMAuthPolicyList{}
		err := c.List(context.Background(), policies, &client.ListOptions{Namespace: obj.GetNamespace()})
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

func (c *IAMAuthPolicyController) updateLatticeAnnotaion(k8sPolicy *anv1alpha1.IAMAuthPolicy, resId, resType string) {
	if k8sPolicy.Annotations == nil {
		k8sPolicy.Annotations = make(map[string]string)
	}
	k8sPolicy.Annotations[IAMAuthPolicyAnnotationResId] = resId
	k8sPolicy.Annotations[IAMAuthPolicyAnnotationType] = resType
}

func (c *IAMAuthPolicyController) getLatticeAnnotation(k8sPolicy *anv1alpha1.IAMAuthPolicy) (model.IAMAuthPolicy, bool) {
	if k8sPolicy.Annotations == nil {
		return model.IAMAuthPolicy{}, false
	}
	resourceId := k8sPolicy.Annotations[IAMAuthPolicyAnnotationResId]
	resourceType := k8sPolicy.Annotations[IAMAuthPolicyAnnotationType]
	if resourceId == "" || resourceType == "" {
		return model.IAMAuthPolicy{}, false
	}
	return model.IAMAuthPolicy{
		Type:       resourceType,
		ResourceId: resourceId,
	}, true
}
