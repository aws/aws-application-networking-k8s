package controllers

import (
	"context"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	deploy "github.com/aws/aws-application-networking-k8s/pkg/deploy/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	policy "github.com/aws/aws-application-networking-k8s/pkg/k8s/policyhelper"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

const (
	IAMAuthPolicyAnnotation      = "iam-auth-policy"
	IAMAuthPolicyAnnotationResId = k8s.AnnotationPrefix + IAMAuthPolicyAnnotation + "-resource-id"
	IAMAuthPolicyAnnotationType  = k8s.AnnotationPrefix + IAMAuthPolicyAnnotation + "-resource-type"
	IAMAuthPolicyFinalizer       = k8s.AnnotationPrefix + IAMAuthPolicyAnnotation
)

type (
	IAP = anv1alpha1.IAMAuthPolicy
)

type IAMAuthPolicyController struct {
	log    gwlog.Logger
	client client.Client
	pm     *deploy.IAMAuthPolicyManager
	ph     *policy.PolicyHandler[*IAP]
	cloud  pkg_aws.Cloud
}

func RegisterIAMAuthPolicyController(log gwlog.Logger, mgr ctrl.Manager, cloud pkg_aws.Cloud) error {
	ph := policy.NewIAMAuthPolicyHandler(log, mgr.GetClient())

	controller := &IAMAuthPolicyController{
		log:    log,
		client: mgr.GetClient(),
		pm:     deploy.NewIAMAuthPolicyManager(cloud),
		ph:     ph,
		cloud:  cloud,
	}

	b := ctrl.
		NewControllerManagedBy(mgr).
		For(&anv1alpha1.IAMAuthPolicy{}, builder.WithPredicates(predicate.GenerationChangedPredicate{}))
	ph.AddWatchers(b, &gwv1beta1.Gateway{}, &gwv1beta1.HTTPRoute{}, &gwv1alpha2.GRPCRoute{})
	err := b.Complete(controller)
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
	ctx = gwlog.NewTrace(ctx)
	gwlog.AddMetadata(ctx, "type", "iamauthpolicy")
	gwlog.AddMetadata(ctx, "name", req.Name)

	c.log.Infow(ctx, "reconcile starting", gwlog.GetMetadata(ctx)...)
	defer func() {
		c.log.Infow(ctx, "reconcile completed", gwlog.GetMetadata(ctx)...)
	}()

	k8sPolicy := &anv1alpha1.IAMAuthPolicy{}
	err := c.client.Get(ctx, req.NamespacedName, k8sPolicy)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	c.log.Infow(ctx, "reconcile IAM policy", "req", req, "targetRef", k8sPolicy.Spec.TargetRef)
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

	c.log.Infow(ctx, "reconciled IAM policy",
		"req", req,
		"targetRef", k8sPolicy.Spec.TargetRef,
		"isDeleted", isDelete,
	)
	return res, nil
}

func (c *IAMAuthPolicyController) reconcileDelete(ctx context.Context, k8sPolicy *anv1alpha1.IAMAuthPolicy) (ctrl.Result, error) {
	err := c.ph.ValidateTargetRef(ctx, k8sPolicy)
	if err == nil {
		modelPolicy := model.NewIAMAuthPolicy(k8sPolicy)
		_, err := c.pm.Delete(ctx, modelPolicy)
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
	reason, err := c.ph.ValidateAndUpdateCondition(ctx, k8sPolicy)
	if err != nil {
		return ctrl.Result{}, err
	}
	if reason != policy.ReasonAccepted {
		return ctrl.Result{}, nil
	}
	modelPolicy := model.NewIAMAuthPolicy(k8sPolicy)
	c.addFinalizer(k8sPolicy)
	err = c.client.Update(ctx, k8sPolicy)
	if err != nil {
		return reconcile.Result{}, err
	}
	statusPolicy, err := c.pm.Put(ctx, modelPolicy)
	if err != nil {
		return reconcile.Result{}, services.IgnoreNotFound(err)
	}
	c.updateLatticeAnnotaion(k8sPolicy, statusPolicy.ResourceId, modelPolicy.Type)
	err = c.handleLatticeResourceChange(ctx, k8sPolicy, statusPolicy)
	if err != nil {
		return reconcile.Result{}, err
	}
	return ctrl.Result{}, nil
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

// cleanup lattice resources after targetRef changes
func (c *IAMAuthPolicyController) handleLatticeResourceChange(ctx context.Context, k8sPolicy *anv1alpha1.IAMAuthPolicy, statusPolicy model.IAMAuthPolicyStatus) error {
	prevModel, ok := c.getLatticeAnnotation(k8sPolicy)
	if !ok {
		return nil
	}
	if prevModel.ResourceId != statusPolicy.ResourceId {
		_, err := c.pm.Delete(ctx, prevModel)
		if err != nil {
			return services.IgnoreNotFound(err)
		}
	}
	return nil
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
