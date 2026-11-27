package controllers

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	deploy "github.com/aws/aws-application-networking-k8s/pkg/deploy/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	policy "github.com/aws/aws-application-networking-k8s/pkg/k8s/policyhelper"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	lattice_runtime "github.com/aws/aws-application-networking-k8s/pkg/runtime"
	"github.com/aws/aws-application-networking-k8s/pkg/utils"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/vpclattice"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const (
	IAMAuthPolicyAnnotation        = "iam-auth-policy"
	IAMAuthPolicyAnnotationResId   = k8s.AnnotationPrefix + IAMAuthPolicyAnnotation + "-resource-id"
	IAMAuthPolicyAnnotationType    = k8s.AnnotationPrefix + IAMAuthPolicyAnnotation + "-resource-type"
	IAMAuthPolicyFinalizer         = k8s.AnnotationPrefix + IAMAuthPolicyAnnotation
	IAMAuthPolicyAnnotationResName = k8s.AnnotationPrefix + IAMAuthPolicyAnnotation + "-resource-name"
)

type (
	IAP = anv1alpha1.IAMAuthPolicy
)

type IAMAuthPolicyController struct {
	log           gwlog.Logger
	client        client.Client
	pm            *deploy.IAMAuthPolicyManager
	ph            *policy.PolicyHandler[*IAP]
	cloud         pkg_aws.Cloud
	eventRecorder record.EventRecorder
}

func RegisterIAMAuthPolicyController(log gwlog.Logger, mgr ctrl.Manager, cloud pkg_aws.Cloud) error {
	ph := policy.NewIAMAuthPolicyHandler(log, mgr.GetClient())

	controller := &IAMAuthPolicyController{
		log:           log,
		client:        mgr.GetClient(),
		pm:            deploy.NewIAMAuthPolicyManager(cloud),
		ph:            ph,
		cloud:         cloud,
		eventRecorder: mgr.GetEventRecorderFor("iam-auth-policy-controller"),
	}

	b := ctrl.
		NewControllerManagedBy(mgr).
		For(&anv1alpha1.IAMAuthPolicy{}, builder.WithPredicates(predicate.GenerationChangedPredicate{}))
	ph.AddWatchers(b, &gwv1.Gateway{}, &gwv1.HTTPRoute{}, &gwv1.GRPCRoute{})
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
	ctx = gwlog.StartReconcileTrace(ctx, c.log, "iamauthpolicy", req.Name, req.Namespace)
	defer func() {
		gwlog.EndReconcileTrace(ctx, c.log)
	}()

	recErr := c.reconcile(ctx, req)
	if recErr != nil {
		c.log.Errorw(ctx, "reconcile error", "name", req.Name, "message", recErr.Error())
	}
	res, retryErr := lattice_runtime.HandleReconcileError(recErr)
	if res.RequeueAfter != 0 {
		c.log.Infow(ctx, "requeue request", "name", req.Name, "requeueAfter", res.RequeueAfter)
	} else if retryErr != nil && !errors.Is(retryErr, reconcile.TerminalError(nil)) {
		c.log.Infow(ctx, "requeue request using exponential backoff", "name", req.Name)
	} else if retryErr == nil {
		c.log.Infow(ctx, "reconciled", "name", req.Name)
	}
	return res, retryErr
}

func (c *IAMAuthPolicyController) reconcile(ctx context.Context, req ctrl.Request) error {
	k8sPolicy := &anv1alpha1.IAMAuthPolicy{}
	err := c.client.Get(ctx, req.NamespacedName, k8sPolicy)
	if err != nil {
		return client.IgnoreNotFound(err)
	}

	c.eventRecorder.Event(k8sPolicy, corev1.EventTypeNormal, k8s.ReconcilingEvent, "Started reconciling")

	c.log.Infow(ctx, "reconcile IAM policy", "req", req, "targetRef", k8sPolicy.Spec.TargetRef)
	isDelete := !k8sPolicy.DeletionTimestamp.IsZero()

	if isDelete {
		err = c.reconcileDelete(ctx, k8sPolicy)
	} else {
		err = c.reconcileUpsert(ctx, k8sPolicy)
	}
	if err != nil {
		c.eventRecorder.Event(k8sPolicy, corev1.EventTypeWarning, k8s.FailedReconcileEvent, fmt.Sprintf("Reconcile failed: %s", err))
		return err
	}

	err = c.client.Update(ctx, k8sPolicy)
	if err != nil {
		return err
	}

	c.eventRecorder.Event(k8sPolicy, corev1.EventTypeNormal, k8s.ReconciledEvent, "Successfully reconciled")

	c.log.Infow(ctx, "reconciled IAM policy",
		"req", req,
		"targetRef", k8sPolicy.Spec.TargetRef,
		"isDeleted", isDelete,
	)
	return nil
}

func (c *IAMAuthPolicyController) reconcileDelete(ctx context.Context, k8sPolicy *anv1alpha1.IAMAuthPolicy) error {
	if err := c.handleLatticeResourceChange(ctx, k8sPolicy, model.IAMAuthPolicyStatus{}); err != nil {
		return err
	}
	c.removeFinalizer(k8sPolicy)
	return nil
}

func (c *IAMAuthPolicyController) reconcileUpsert(ctx context.Context, k8sPolicy *anv1alpha1.IAMAuthPolicy) error {
	reason, err := c.ph.ValidateAndUpdateCondition(ctx, k8sPolicy)
	if err != nil {
		return err
	}
	if reason != policy.ReasonAccepted {
		return nil
	}

	resourceName, err := c.targetRefToResourceName(ctx, k8sPolicy)
	if err != nil {
		if k8s.IsInvalidServiceNameOverrideError(err) {
			if statusErr := c.ph.UpdateAcceptedCondition(ctx, k8sPolicy, gwv1.PolicyReasonInvalid, err.Error()); statusErr != nil {
				return statusErr
			}
			return nil
		}
		return err
	}

	modelPolicy := model.NewIAMAuthPolicy(k8sPolicy, resourceName)

	c.addFinalizer(k8sPolicy)
	err = c.client.Update(ctx, k8sPolicy)
	if err != nil {
		return err
	}
	statusPolicy, err := c.pm.Put(ctx, modelPolicy)
	if err != nil {
		// The underlying VPC Lattice resource (Service or ServiceNetwork) does
		// not exist. This can happen if it was deleted out-of-band, or — for
		// Gateway-targeted policies — if the ServiceNetwork has not yet been
		// created (the gateway controller does not create ServiceNetworks).
		// Surface this as a non-Accepted condition rather than silently
		// succeeding; preserve the existing resource-id annotation so any
		// previously-tracked resource can still be cleaned up if it returns.
		if services.IsNotFoundError(err) {
			msg := fmt.Sprintf("VPC Lattice %s %q not found", modelPolicy.Type, resourceName)
			if statusErr := c.ph.UpdateAcceptedCondition(ctx, k8sPolicy, gwv1.PolicyReasonInvalid, msg); statusErr != nil {
				return statusErr
			}
			return nil
		}
		// Foreign-owned target: mark Conflicted and stop retrying (return nil).
		if services.IsConflictError(err) {
			if statusErr := c.ph.UpdateAcceptedCondition(ctx, k8sPolicy, gwv1.PolicyReasonConflicted, err.Error()); statusErr != nil {
				return statusErr
			}
			return nil
		}
		return err
	}
	err = c.handleLatticeResourceChange(ctx, k8sPolicy, statusPolicy)
	if err != nil {
		return err
	}
	c.updateLatticeAnnotaion(k8sPolicy, statusPolicy.ResourceId, modelPolicy.Type, modelPolicy.Name)
	return nil
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

// handleLatticeResourceChange removes auth from the previously recorded
// resource when it differs from the current one (retarget or delete).
func (c *IAMAuthPolicyController) handleLatticeResourceChange(ctx context.Context, k8sPolicy *anv1alpha1.IAMAuthPolicy, statusPolicy model.IAMAuthPolicyStatus) error {
	prevModel, ok := c.getLatticeAnnotation(k8sPolicy)
	if !ok || prevModel.ResourceId == statusPolicy.ResourceId {
		return nil
	}
	if prevModel.Type == model.ServiceNetworkType {
		allowed, err := c.snCleanupAllowed(ctx, k8sPolicy, prevModel.ResourceId)
		if err != nil || !allowed {
			return err
		}
	}
	_, err := c.pm.Delete(ctx, prevModel)
	return services.IgnoreNotFound(err)
}

var snIdPattern = regexp.MustCompile(`^sn-[0-9a-z]{17}$`)

// snCleanupAllowed checks a recorded service network ID before its auth is
// removed. The annotations are user-editable, so cleanup is limited to SNs a
// Gateway in the policy's namespace could also target with Put.
func (c *IAMAuthPolicyController) snCleanupAllowed(ctx context.Context, k8sPolicy *anv1alpha1.IAMAuthPolicy, snId string) (bool, error) {
	skip := func(reason string) (bool, error) {
		c.eventRecorder.Event(k8sPolicy, corev1.EventTypeWarning, k8s.IAMAuthPolicyEventReasonSkippedCleanup,
			fmt.Sprintf("Skipped removing IAM auth from VPC Lattice service network %s: %s. "+
				"If it exists, it keeps AuthType AWS_IAM and its auth policy.", snId, reason))
		return false, nil
	}
	if !snIdPattern.MatchString(snId) {
		return skip("not a service network ID")
	}
	sn, err := c.cloud.Lattice().GetServiceNetwork(ctx, &vpclattice.GetServiceNetworkInput{ServiceNetworkIdentifier: &snId})
	if err != nil {
		if services.IsNotFoundError(err) {
			c.log.Infow(ctx, "previously recorded service network not found, nothing to clean up", "resourceId", snId)
			return false, nil
		}
		return false, err
	}
	allowed, err := c.hasControlledGateway(ctx, k8sPolicy.Namespace, aws.ToString(sn.Name), aws.ToString(sn.Id))
	if err != nil || allowed {
		return allowed, err
	}
	return skip(fmt.Sprintf("no Gateway in namespace %s managed by this controller maps to %q, for example because "+
		"it was deleted first", k8sPolicy.Namespace, aws.ToString(sn.Name)))
}

// hasControlledGateway reports whether a controller-managed Gateway in the
// namespace maps to the SN.
func (c *IAMAuthPolicyController) hasControlledGateway(ctx context.Context, namespace, name, id string) (bool, error) {
	// In override mode every Gateway maps to the default SN. The name rule below
	// still covers SNs recorded before override mode was enabled.
	if config.ServiceNetworkOverrideMode && (name == config.DefaultServiceNetwork || id == config.DefaultServiceNetwork) {
		gws := &gwv1.GatewayList{}
		if err := c.client.List(ctx, gws, client.InNamespace(namespace)); err != nil {
			return false, err
		}
		for i := range gws.Items {
			if controlled, err := k8s.IsControlledByLatticeGatewayController(ctx, c.client, &gws.Items[i]); controlled || err != nil {
				return controlled, err
			}
		}
	}

	// FindServiceNetwork matches a Gateway name against both SN name and ID.
	for _, gwName := range []string{name, id} {
		if gwName == "" {
			continue
		}
		gw := &gwv1.Gateway{}
		err := c.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: gwName}, gw)
		if apierrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return false, err
		}
		if controlled, err := k8s.IsControlledByLatticeGatewayController(ctx, c.client, gw); controlled || err != nil {
			return controlled, err
		}
	}
	return false, nil
}

// iamAuthPolicyNeedsGateway reports whether the policy may still need the
// Gateway to exist to clean up, so the Gateway's deletion waits for it.
func iamAuthPolicyNeedsGateway(p *anv1alpha1.IAMAuthPolicy, gwName string) bool {
	return controllerutil.ContainsFinalizer(p, IAMAuthPolicyFinalizer) &&
		slices.Contains(iamAuthPolicyGatewayNames(p), gwName)
}

// iamAuthPolicyGatewayNames returns the Gateways the policy targets or last
// applied auth through. A forged annotation can only delay deleting a Gateway
// in the policy's own namespace.
func iamAuthPolicyGatewayNames(p *anv1alpha1.IAMAuthPolicy) []string {
	var names []string
	if tr := p.Spec.TargetRef; tr != nil && policy.TargetRefGroupKind(tr) == policy.ObjToGroupKind(&gwv1.Gateway{}) {
		names = append(names, string(tr.Name))
	}
	if p.Annotations[IAMAuthPolicyAnnotationType] == model.ServiceNetworkType {
		if name := p.Annotations[IAMAuthPolicyAnnotationResName]; name != "" {
			names = append(names, name)
		}
	}
	return names
}

func (c *IAMAuthPolicyController) updateLatticeAnnotaion(k8sPolicy *anv1alpha1.IAMAuthPolicy, resId, resType string, resName string) {
	if k8sPolicy.Annotations == nil {
		k8sPolicy.Annotations = make(map[string]string)
	}
	k8sPolicy.Annotations[IAMAuthPolicyAnnotationResId] = resId
	k8sPolicy.Annotations[IAMAuthPolicyAnnotationType] = resType
	k8sPolicy.Annotations[IAMAuthPolicyAnnotationResName] = resName
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
	// Cleanup targets resources by ID only; the name annotation is user-editable.
	return model.IAMAuthPolicy{
		Type:       resourceType,
		ResourceId: resourceId,
	}, true
}

func (c *IAMAuthPolicyController) targetRefToResourceName(
	ctx context.Context,
	k8sPolicy *anv1alpha1.IAMAuthPolicy,
) (string, error) {
	targetRef := k8sPolicy.Spec.TargetRef

	if targetRef.Kind == "Gateway" {
		return string(targetRef.Name), nil
	}

	if targetRef.Kind == "HTTPRoute" || targetRef.Kind == "GRPCRoute" {
		namespace := k8sPolicy.Namespace
		if targetRef.Namespace != nil {
			namespace = string(*targetRef.Namespace)
		}

		targetRefObj, err := c.ph.GetTargetRefObj(ctx, k8sPolicy)
		if err != nil {
			return "", fmt.Errorf("failed to get target route: %w", err)
		}

		serviceNameOverride, err := k8s.GetServiceNameOverrideWithValidation(targetRefObj)
		if err != nil {
			return "", fmt.Errorf("route '%s' has %w", targetRef.Name, err)
		}

		return utils.LatticeServiceName(string(targetRef.Name), namespace, serviceNameOverride), nil
	}

	return "", fmt.Errorf("unsupported targetRef kind: %s", targetRef.Kind)
}
