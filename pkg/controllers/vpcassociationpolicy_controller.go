package controllers

import (
	"context"
	"errors"
	"fmt"

	vpclatticetypes "github.com/aws/aws-sdk-go-v2/service/vpclattice/types"
	"github.com/aws/smithy-go"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/controllers/predicates"
	deploy "github.com/aws/aws-application-networking-k8s/pkg/deploy/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	policy "github.com/aws/aws-application-networking-k8s/pkg/k8s/policyhelper"
	lattice_runtime "github.com/aws/aws-application-networking-k8s/pkg/runtime"
	"github.com/aws/aws-application-networking-k8s/pkg/utils"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

type (
	VAP = anv1alpha1.VpcAssociationPolicy
)

const (
	finalizer = "vpcassociationpolicies.application-networking.k8s.aws/resources"

	recreateSteps = "VPC Lattice only accepts privateDnsEnabled and dnsOptions when an association is created. " +
		"To apply the policy, set spec.associateWithVpc to false, wait until the association is deleted, then set it back to true; " +
		"traffic from this VPC to the service network is interrupted while the association is recreated."

	recreateStepsRamShared = "VPC Lattice only accepts privateDnsEnabled and dnsOptions when an association is created, " +
		"and the controller never deletes an association it did not create on a RAM-shared service network. " +
		"If this association was created by the controller, set spec.associateWithVpc to false, wait until the association is deleted, then set it back to true; " +
		"otherwise delete and recreate the association with the desired private DNS settings, or align the policy with the existing association."
)

type vpcAssociationPolicyReconciler struct {
	log              gwlog.Logger
	client           client.Client
	cloud            pkg_aws.Cloud
	finalizerManager k8s.FinalizerManager
	manager          deploy.ServiceNetworkManager
	eventRecorder    record.EventRecorder
	ph               *policy.PolicyHandler[*VAP]
}

func RegisterVpcAssociationPolicyController(log gwlog.Logger, cloud pkg_aws.Cloud, finalizerManager k8s.FinalizerManager, mgr ctrl.Manager) error {
	warnIfPrivateDnsNotServed(log, mgr.GetConfig())
	ph := policy.NewVpcAssociationPolicyHandler(log, mgr.GetClient())
	controller := &vpcAssociationPolicyReconciler{
		log:              log,
		client:           mgr.GetClient(),
		cloud:            cloud,
		finalizerManager: finalizerManager,
		manager:          deploy.NewDefaultServiceNetworkManager(log, cloud),
		eventRecorder:    mgr.GetEventRecorderFor("vpc-association-policy-controller"),
		ph:               ph,
	}

	b := ctrl.NewControllerManagedBy(mgr).
		For(&anv1alpha1.VpcAssociationPolicy{}, builder.WithPredicates(predicate.Or(predicate.GenerationChangedPredicate{}, predicates.AdditionalTagsAnnotationChangedPredicate)))
	ph.AddWatchers(b, &gwv1.Gateway{})
	return b.Complete(controller)
}

// warnIfPrivateDnsNotServed warns when the installed CRD predates the private DNS fields (Helm does not
// upgrade CRDs): the API server then drops them from every object and the controller cannot see them.
func warnIfPrivateDnsNotServed(log gwlog.Logger, cfg *rest.Config) {
	ctx := context.TODO()
	gvk := anv1alpha1.SchemeGroupVersion.WithKind(anv1alpha1.VpcAssociationPolicyKind)
	served, err := k8s.IsCRDSpecFieldServed(cfg, gvk, "privateDnsEnabled")
	if err != nil {
		log.Debugf(ctx, "could not verify the served VpcAssociationPolicy schema: %s", err)
		return
	}
	if !served {
		log.Warnf(ctx, "The installed VpcAssociationPolicy CRD does not define spec.privateDnsEnabled: "+
			"privateDnsEnabled and dnsOptions are dropped by the API server until the CRD is upgraded "+
			"(apply config/crds/bases/application-networking.k8s.aws_vpcassociationpolicies.yaml from the release)")
	}
}

func (c *vpcAssociationPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctx = gwlog.StartReconcileTrace(ctx, c.log, "vpcassociationpolicy", req.Name, req.Namespace)
	defer func() {
		gwlog.EndReconcileTrace(ctx, c.log)
	}()

	recErr := c.reconcile(ctx, req)
	if recErr != nil {
		c.log.Infow(ctx, "reconcile error", "name", req.Name, "message", recErr.Error())
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

func (c *vpcAssociationPolicyReconciler) reconcile(ctx context.Context, req ctrl.Request) error {
	k8sPolicy := &anv1alpha1.VpcAssociationPolicy{}
	err := c.client.Get(ctx, req.NamespacedName, k8sPolicy)
	if err != nil {
		return client.IgnoreNotFound(err)
	}
	c.log.Infow(ctx, "reconcile", "req", req, "targetRef", k8sPolicy.Spec.TargetRef)

	isDelete := !k8sPolicy.DeletionTimestamp.IsZero()
	isAssociation := k8sPolicy.Spec.AssociateWithVpc == nil || *k8sPolicy.Spec.AssociateWithVpc

	switch {
	case isDelete:
		err = c.delete(ctx, k8sPolicy)
	case !isAssociation:
		err = c.disassociate(ctx, k8sPolicy)
	default:
		err = c.upsert(ctx, k8sPolicy)
	}
	if err != nil {
		return err
	}

	c.log.Infow(ctx, "reconciled vpc association policy",
		"req", req,
		"targetRef", k8sPolicy.Spec.TargetRef,
		"isDeleted", isDelete,
	)
	return nil
}

func (c *vpcAssociationPolicyReconciler) upsert(ctx context.Context, k8sPolicy *anv1alpha1.VpcAssociationPolicy) error {
	reason, err := c.ph.ValidateAndUpdateCondition(ctx, k8sPolicy)
	if err != nil {
		return err
	}
	if reason != policy.ReasonAccepted {
		return c.setNotAccepted(ctx, k8sPolicy, reason)
	}
	if err := c.finalizerManager.AddFinalizers(ctx, k8sPolicy, finalizer); err != nil {
		return err
	}

	snName := string(k8sPolicy.Spec.TargetRef.Name)
	req := deploy.VpcAssociationRequest{
		SecurityGroupIds: utils.SliceMap(k8sPolicy.Spec.SecurityGroupIds, func(sg anv1alpha1.SecurityGroupId) string { return string(sg) }),
		AdditionalTags:   k8s.GetAdditionalTagsFromAnnotations(ctx, k8sPolicy),
		Dns:              dnsConfigFromSpec(k8sPolicy.Spec),
	}
	res, err := c.manager.UpsertVpcAssociation(ctx, snName, req)
	if err != nil {
		return c.setProgrammedFromError(ctx, k8sPolicy, err, "Waiting for the VPC association to become active")
	}
	if err := c.updateLatticeAnnotation(ctx, k8sPolicy, res.Arn); err != nil {
		return err
	}

	if len(res.DnsMismatch) > 0 {
		// Private DNS settings cannot be changed in place and recreating the association would
		// interrupt traffic, so the mismatch is reported and the decision left to the user.
		steps := recreateSteps
		if res.RamShared {
			steps = recreateStepsRamShared
		}
		msg := fmt.Sprintf("VPC association %s has private DNS settings that differ from this policy: %s. %s",
			res.Arn, deploy.FormatDnsMismatch(res.DnsMismatch), steps)
		return c.setProgrammed(ctx, k8sPolicy, metav1.ConditionFalse, anv1alpha1.VpcAssociationPolicyReasonPrivateDnsMismatch, msg,
			k8s.VpcAssociationPolicyEventReasonPrivateDnsMismatch)
	}
	msg := fmt.Sprintf("VPC association %s is active and matches the policy", res.Arn)
	if res.RamShared {
		msg = fmt.Sprintf("VPC association %s is active; security groups and tags of associations on RAM-shared service networks are not managed by the controller", res.Arn)
	}
	return c.setProgrammed(ctx, k8sPolicy, metav1.ConditionTrue, anv1alpha1.VpcAssociationPolicyReasonProgrammed, msg, "")
}

// disassociate handles spec.associateWithVpc=false.
func (c *vpcAssociationPolicyReconciler) disassociate(ctx context.Context, k8sPolicy *anv1alpha1.VpcAssociationPolicy) error {
	reason, err := c.ph.ValidateAndUpdateCondition(ctx, k8sPolicy)
	if err != nil {
		return err
	}
	if reason != policy.ReasonAccepted {
		// a policy that lost conflict resolution must not delete an association another policy manages
		return c.setNotAccepted(ctx, k8sPolicy, reason)
	}
	snName := string(k8sPolicy.Spec.TargetRef.Name)
	if err := c.deleteVpcAssociation(ctx, snName); err != nil {
		return c.setProgrammedFromError(ctx, k8sPolicy, err, "Waiting for the VPC association to be deleted")
	}
	if err := c.finalizerManager.RemoveFinalizers(ctx, k8sPolicy, finalizer); err != nil {
		return err
	}
	return c.setProgrammed(ctx, k8sPolicy, metav1.ConditionTrue, anv1alpha1.VpcAssociationPolicyReasonProgrammed,
		fmt.Sprintf("No VPC association is managed because spec.associateWithVpc is false (service network %s)", snName), "")
}

func (c *vpcAssociationPolicyReconciler) delete(ctx context.Context, k8sPolicy *anv1alpha1.VpcAssociationPolicy) error {
	if err := c.deleteVpcAssociation(ctx, string(k8sPolicy.Spec.TargetRef.Name)); err != nil {
		return err
	}
	return c.finalizerManager.RemoveFinalizers(ctx, k8sPolicy, finalizer)
}

// deleteVpcAssociation deletes the association; one that is already gone is not an error.
func (c *vpcAssociationPolicyReconciler) deleteVpcAssociation(ctx context.Context, snName string) error {
	err := c.manager.DeleteVpcAssociation(ctx, snName)
	if services.IsNotFoundError(err) {
		return nil
	}
	return err
}

func (c *vpcAssociationPolicyReconciler) updateLatticeAnnotation(ctx context.Context, k8sPolicy *anv1alpha1.VpcAssociationPolicy, resArn string) error {
	if k8sPolicy.Annotations == nil {
		k8sPolicy.Annotations = make(map[string]string)
	}
	k8sPolicy.Annotations["application-networking.k8s.aws/resourceArn"] = resArn
	return c.client.Update(ctx, k8sPolicy)
}

func (c *vpcAssociationPolicyReconciler) setNotAccepted(ctx context.Context, k8sPolicy *anv1alpha1.VpcAssociationPolicy, reason policy.ConditionReason) error {
	return c.setProgrammed(ctx, k8sPolicy, metav1.ConditionUnknown, anv1alpha1.VpcAssociationPolicyReasonNotAccepted,
		fmt.Sprintf("The policy is not accepted (reason %s); see the Accepted condition", reason), "")
}

// setProgrammedFromError reflects a manager error in the Programmed condition and returns the error
// unchanged, so the requeue behaviour (10s while in progress, backoff otherwise) does not change.
func (c *vpcAssociationPolicyReconciler) setProgrammedFromError(ctx context.Context, k8sPolicy *anv1alpha1.VpcAssociationPolicy, err error, pendingMsg string) error {
	var condErr error
	var requeue *lattice_runtime.RequeueNeededAfter
	switch {
	case errors.As(err, &requeue):
		condErr = c.setProgrammed(ctx, k8sPolicy, metav1.ConditionUnknown, anv1alpha1.VpcAssociationPolicyReasonPending, pendingMsg, "")
	case isTransientError(err):
		// keep a healthy Programmed=True through throttling; only report Pending when there is nothing better to show
		if !meta.IsStatusConditionTrue(k8sPolicy.Status.Conditions, string(anv1alpha1.VpcAssociationPolicyConditionProgrammed)) {
			condErr = c.setProgrammed(ctx, k8sPolicy, metav1.ConditionUnknown, anv1alpha1.VpcAssociationPolicyReasonPending,
				pendingMsg+": "+deterministicErrorMessage(err), "")
		}
	default:
		condErr = c.setProgrammed(ctx, k8sPolicy, metav1.ConditionFalse, anv1alpha1.VpcAssociationPolicyReasonReconcileFailed,
			deterministicErrorMessage(err), k8s.FailedReconcileEvent)
	}
	if condErr != nil {
		c.log.Infow(ctx, "failed to update Programmed condition", "name", k8sPolicy.Name, "message", condErr.Error())
	}
	return err
}

// setProgrammed writes the Programmed condition only when it changed and emits a Warning event for the
// change when eventReason is set, so periodic resyncs write and emit nothing.
func (c *vpcAssociationPolicyReconciler) setProgrammed(ctx context.Context, k8sPolicy *anv1alpha1.VpcAssociationPolicy,
	status metav1.ConditionStatus, reason gwv1.PolicyConditionReason, msg string, eventReason string) error {
	changed := meta.SetStatusCondition(&k8sPolicy.Status.Conditions, metav1.Condition{
		Type:               string(anv1alpha1.VpcAssociationPolicyConditionProgrammed),
		Status:             status,
		ObservedGeneration: k8sPolicy.Generation,
		Reason:             string(reason),
		Message:            msg,
	})
	if !changed {
		return nil
	}
	if err := c.client.Status().Update(ctx, k8sPolicy); err != nil {
		return err
	}
	if eventReason != "" {
		c.eventRecorder.Event(k8sPolicy, corev1.EventTypeWarning, eventReason, msg)
	}
	return nil
}

// dnsConfigFromSpec maps the policy's private DNS fields to the manager's request.
func dnsConfigFromSpec(spec anv1alpha1.VpcAssociationPolicySpec) deploy.VpcAssociationDnsConfig {
	cfg := deploy.VpcAssociationDnsConfig{PrivateDnsEnabled: spec.PrivateDnsEnabled}
	if spec.DnsOptions == nil {
		return cfg
	}
	if spec.DnsOptions.PrivateDnsPreference != nil {
		cfg.PrivateDnsPreference = string(*spec.DnsOptions.PrivateDnsPreference)
	}
	if len(spec.DnsOptions.PrivateDnsSpecifiedDomains) > 0 {
		cfg.PrivateDnsSpecifiedDomains = utils.SliceMap(spec.DnsOptions.PrivateDnsSpecifiedDomains, func(d anv1alpha1.PrivateDnsDomain) string { return string(d) })
	}
	return cfg
}

// isTransientError reports errors that resolve on their own: the service network not existing yet,
// throttling, VPC Lattice internal errors and VPC Lattice conflicts from a concurrent state change.
func isTransientError(err error) bool {
	var throttling *vpclatticetypes.ThrottlingException
	var internal *vpclatticetypes.InternalServerException
	var conflict *vpclatticetypes.ConflictException
	return services.IsNotFoundError(err) || errors.As(err, &throttling) || errors.As(err, &internal) || errors.As(err, &conflict)
}

// deterministicErrorMessage drops per-attempt noise such as request IDs from AWS errors, so a repeated
// failure does not rewrite the condition on every retry.
func deterministicErrorMessage(err error) string {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		return apiErr.ErrorCode() + ": " + apiErr.ErrorMessage()
	}
	return err.Error()
}
