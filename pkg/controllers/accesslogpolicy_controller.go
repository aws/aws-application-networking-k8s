/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"golang.org/x/exp/slices"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	pkg_builder "sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/controllers/predicates"
	"github.com/aws/aws-application-networking-k8s/pkg/deploy"
	"github.com/aws/aws-application-networking-k8s/pkg/gateway"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	lattice_runtime "github.com/aws/aws-application-networking-k8s/pkg/runtime"
	"github.com/aws/aws-application-networking-k8s/pkg/utils"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

const (
	accessLogPolicyFinalizer = "accesslogpolicy.k8s.aws/resources"
)

type accessLogPolicyReconciler struct {
	log              gwlog.Logger
	client           client.Client
	scheme           *runtime.Scheme
	finalizerManager k8s.FinalizerManager
	eventRecorder    record.EventRecorder
	modelBuilder     gateway.AccessLogSubscriptionModelBuilder
	stackDeployer    deploy.StackDeployer
	cloud            aws.Cloud
	stackMarshaller  deploy.StackMarshaller
}

func RegisterAccessLogPolicyController(
	log gwlog.Logger,
	cloud aws.Cloud,
	finalizerManager k8s.FinalizerManager,
	mgr ctrl.Manager,
) error {
	mgrClient := mgr.GetClient()
	scheme := mgr.GetScheme()
	evtRec := mgr.GetEventRecorderFor("access-log-policy-controller")

	modelBuilder := gateway.NewAccessLogSubscriptionModelBuilder(log, mgrClient)
	stackDeployer := deploy.NewAccessLogSubscriptionStackDeployer(log, cloud, mgrClient)
	stackMarshaller := deploy.NewDefaultStackMarshaller()

	r := &accessLogPolicyReconciler{
		log:              log,
		client:           mgrClient,
		scheme:           scheme,
		finalizerManager: finalizerManager,
		eventRecorder:    evtRec,
		modelBuilder:     modelBuilder,
		stackDeployer:    stackDeployer,
		cloud:            cloud,
		stackMarshaller:  stackMarshaller,
	}

	builder := ctrl.NewControllerManagedBy(mgr).
		For(&anv1alpha1.AccessLogPolicy{}, pkg_builder.WithPredicates(predicate.Or(predicate.GenerationChangedPredicate{}, predicates.AdditionalTagsAnnotationChangedPredicate))).
		Watches(&gwv1.Gateway{}, handler.EnqueueRequestsFromMapFunc(r.findImpactedAccessLogPolicies), pkg_builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(&gwv1.HTTPRoute{}, handler.EnqueueRequestsFromMapFunc(r.findImpactedAccessLogPolicies), pkg_builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(&gwv1.GRPCRoute{}, handler.EnqueueRequestsFromMapFunc(r.findImpactedAccessLogPolicies), pkg_builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(&gwv1alpha2.TLSRoute{}, handler.EnqueueRequestsFromMapFunc(r.findImpactedAccessLogPolicies), pkg_builder.WithPredicates(predicate.GenerationChangedPredicate{}))

	return builder.Complete(r)
}

func (r *accessLogPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctx = gwlog.StartReconcileTrace(ctx, r.log, "accesslogpolicy", req.Name, req.Namespace)
	defer func() {
		gwlog.EndReconcileTrace(ctx, r.log)
	}()

	recErr := r.reconcile(ctx, req)
	if recErr != nil {
		r.log.Infow(ctx, "reconcile error", "name", req.Name, "message", recErr.Error())
	}
	res, retryErr := lattice_runtime.HandleReconcileError(recErr)
	if res.RequeueAfter != 0 {
		r.log.Infow(ctx, "requeue request", "name", req.Name, "requeueAfter", res.RequeueAfter)
	} else if retryErr != nil && !errors.Is(retryErr, reconcile.TerminalError(nil)) {
		r.log.Infow(ctx, "requeue request using exponential backoff", "name", req.Name)
	} else if retryErr == nil {
		r.log.Infow(ctx, "reconciled", "name", req.Name)
	}
	return res, retryErr
}

func (r *accessLogPolicyReconciler) reconcile(ctx context.Context, req ctrl.Request) error {
	alp := &anv1alpha1.AccessLogPolicy{}
	if err := r.client.Get(ctx, req.NamespacedName, alp); err != nil {
		return client.IgnoreNotFound(err)
	}

	r.eventRecorder.Event(alp, corev1.EventTypeNormal, k8s.ReconcilingEvent, "Started reconciling")

	var err error
	if !alp.DeletionTimestamp.IsZero() {
		err = r.reconcileDelete(ctx, alp)
	} else {
		err = r.reconcileUpsert(ctx, alp)
	}

	if err != nil {
		r.eventRecorder.Event(alp, corev1.EventTypeWarning, k8s.FailedReconcileEvent, fmt.Sprintf("Reconcile failed: %s", err))
		return err
	}

	r.eventRecorder.Event(alp, corev1.EventTypeNormal, k8s.ReconciledEvent, "Successfully reconciled")
	return nil
}

func (r *accessLogPolicyReconciler) reconcileDelete(ctx context.Context, alp *anv1alpha1.AccessLogPolicy) error {
	targetRefName := alp.Annotations[anv1alpha1.AccessLogSubscriptionResourceNameAnnotationKey]
	_, err := r.buildAndDeployModel(ctx, alp, targetRefName)
	if err != nil {
		r.eventRecorder.Event(alp, corev1.EventTypeWarning,
			k8s.FailedReconcileEvent, fmt.Sprintf("Failed to delete due to %s", err))
		return err
	}

	err = r.finalizerManager.RemoveFinalizers(ctx, alp, accessLogPolicyFinalizer)
	if err != nil {
		r.eventRecorder.Event(alp, corev1.EventTypeWarning,
			k8s.FailedReconcileEvent, fmt.Sprintf("Failed to remove finalizer due to %s", err))
		return err
	}

	return nil
}

func (r *accessLogPolicyReconciler) reconcileUpsert(ctx context.Context, alp *anv1alpha1.AccessLogPolicy) error {
	if err := r.finalizerManager.AddFinalizers(ctx, alp, accessLogPolicyFinalizer); err != nil {
		r.eventRecorder.Event(alp, corev1.EventTypeWarning,
			k8s.FailedReconcileEvent, fmt.Sprintf("Failed to add finalizer due to %s", err))
		return err
	}

	if alp.Spec.TargetRef.Group != gwv1.GroupName {
		message := fmt.Sprintf("The targetRef's Group must be \"%s\" but was \"%s\"",
			gwv1.GroupName, alp.Spec.TargetRef.Group)
		r.eventRecorder.Event(alp, corev1.EventTypeWarning, k8s.FailedReconcileEvent, message)
		return r.updateAccessLogPolicyStatus(ctx, alp, gwv1alpha2.PolicyReasonInvalid, message)
	}

	validKinds := []string{"Gateway", "HTTPRoute", "GRPCRoute"}
	if !slices.Contains(validKinds, string(alp.Spec.TargetRef.Kind)) {
		message := fmt.Sprintf("The targetRef's Kind must be \"Gateway\", \"HTTPRoute\", or \"GRPCRoute\""+
			" but was \"%s\"", alp.Spec.TargetRef.Kind)
		r.eventRecorder.Event(alp, corev1.EventTypeWarning, k8s.FailedReconcileEvent, message)
		return r.updateAccessLogPolicyStatus(ctx, alp, gwv1alpha2.PolicyReasonInvalid, message)
	}

	targetRefNamespace := k8s.NamespaceOrDefault(alp.Spec.TargetRef.Namespace)
	if targetRefNamespace != alp.Namespace {
		message := fmt.Sprintf("The targetRef's namespace, \"%s\", does not match the Access Log Policy's"+
			" namespace, \"%s\"", targetRefNamespace, alp.Namespace)
		r.eventRecorder.Event(alp, corev1.EventTypeWarning, k8s.FailedReconcileEvent, message)
		return r.updateAccessLogPolicyStatus(ctx, alp, gwv1alpha2.PolicyReasonInvalid, message)
	}

	targetRefExists, targetRef, err := r.targetRefExists(ctx, alp)
	if err != nil {
		return err
	}
	if !targetRefExists {
		message := fmt.Sprintf("%s target \"%s/%s\" could not be found", alp.Spec.TargetRef.Kind, targetRefNamespace, alp.Spec.TargetRef.Name)
		r.eventRecorder.Event(alp, corev1.EventTypeWarning, k8s.FailedReconcileEvent, message)
		return r.updateAccessLogPolicyStatus(ctx, alp, gwv1alpha2.PolicyReasonTargetNotFound, message)
	}

	targetRefName, err := r.targetRefToResourceName(alp, targetRef)
	if err != nil {
		if k8s.IsInvalidServiceNameOverrideError(err) {
			return r.updateAccessLogPolicyStatus(ctx, alp, gwv1alpha2.PolicyReasonInvalid, err.Error())
		}
		return err
	}

	stack, err := r.buildAndDeployModel(ctx, alp, targetRefName)
	if err != nil {
		if services.IsConflictError(err) {
			message := "An Access Log Policy with a Destination Arn for the same destination type already exists for this targetRef"
			r.eventRecorder.Event(alp, corev1.EventTypeWarning, k8s.FailedReconcileEvent, message)
			return r.updateAccessLogPolicyStatus(ctx, alp, gwv1alpha2.PolicyReasonConflicted, message)
		} else if services.IsInvalidError(err) {
			message := fmt.Sprintf("The AWS resource with Destination Arn \"%s\" could not be found", *alp.Spec.DestinationArn)
			r.eventRecorder.Event(alp, corev1.EventTypeWarning, k8s.FailedReconcileEvent, message)
			return r.updateAccessLogPolicyStatus(ctx, alp, gwv1alpha2.PolicyReasonInvalid, message)
		}
		r.eventRecorder.Event(alp, corev1.EventTypeWarning, k8s.FailedReconcileEvent,
			"Failed to create or update due to "+err.Error())
		return err
	}

	err = r.updateAccessLogPolicyAnnotations(ctx, alp, stack, targetRefName)
	if err != nil {
		return err
	}

	err = r.updateAccessLogPolicyStatus(ctx, alp, gwv1alpha2.PolicyReasonAccepted, config.LatticeGatewayControllerName)
	if err != nil {
		return err
	}

	r.eventRecorder.Event(alp, corev1.EventTypeNormal, k8s.ReconciledEvent, "Successfully reconciled")

	return nil
}

func (r *accessLogPolicyReconciler) targetRefExists(ctx context.Context, alp *anv1alpha1.AccessLogPolicy) (bool, metav1.Object, error) {
	targetRefNamespacedName := types.NamespacedName{
		Name:      string(alp.Spec.TargetRef.Name),
		Namespace: k8s.NamespaceOrDefault(alp.Spec.TargetRef.Namespace),
	}

	var err error
	var targetObj metav1.Object

	switch alp.Spec.TargetRef.Kind {
	case "Gateway":
		gw := &gwv1.Gateway{}
		err = r.client.Get(ctx, targetRefNamespacedName, gw)
		targetObj = gw
	case "HTTPRoute":
		route := &gwv1.HTTPRoute{}
		err = r.client.Get(ctx, targetRefNamespacedName, route)
		targetObj = route
	case "GRPCRoute":
		route := &gwv1.GRPCRoute{}
		err = r.client.Get(ctx, targetRefNamespacedName, route)
		targetObj = route
	default:
		return false, nil, fmt.Errorf("access Log Policy targetRef is for unsupported Kind: %s", alp.Spec.TargetRef.Kind)
	}

	if err != nil && !apierrors.IsNotFound(err) {
		return false, nil, err
	}

	exists := err == nil
	return exists, targetObj, nil
}

func (r *accessLogPolicyReconciler) buildAndDeployModel(
	ctx context.Context,
	alp *anv1alpha1.AccessLogPolicy,
	targetRefName string,
) (core.Stack, error) {
	stack, _, err := r.modelBuilder.Build(ctx, alp, targetRefName)
	if err != nil {
		return nil, err
	}

	jsonStack, err := r.stackMarshaller.Marshal(stack)
	if err != nil {
		return nil, err
	}
	r.log.Debugw(ctx, "Successfully built model", "stack", jsonStack)

	if err := r.stackDeployer.Deploy(ctx, stack); err != nil {
		return nil, err
	}
	r.log.Debugf(ctx, "successfully deployed model for stack %s:%s", stack.StackID().Name, stack.StackID().Namespace)

	return stack, nil
}

func (r *accessLogPolicyReconciler) updateAccessLogPolicyAnnotations(
	ctx context.Context,
	alp *anv1alpha1.AccessLogPolicy,
	stack core.Stack,
	targetRefName string,
) error {
	var accessLogSubscriptions []*model.AccessLogSubscription
	err := stack.ListResources(&accessLogSubscriptions)
	if err != nil {
		return err
	}

	for _, als := range accessLogSubscriptions {
		if als.Spec.EventType != core.DeleteEvent {
			oldAlp := alp.DeepCopy()
			if alp.Annotations == nil {
				alp.Annotations = make(map[string]string)
			}
			alp.Annotations[anv1alpha1.AccessLogSubscriptionAnnotationKey] = als.Status.Arn
			alp.Annotations[anv1alpha1.AccessLogSubscriptionResourceNameAnnotationKey] = targetRefName
			if err := r.client.Patch(ctx, alp, client.MergeFrom(oldAlp)); err != nil {
				r.eventRecorder.Event(alp, corev1.EventTypeWarning, k8s.FailedReconcileEvent,
					"Failed to update annotation due to "+err.Error())
				return fmt.Errorf("failed to add annotation to Access Log Policy %s-%s, %w",
					alp.Name, alp.Namespace, err)
			}
		}
	}

	return nil
}

func (r *accessLogPolicyReconciler) updateAccessLogPolicyStatus(
	ctx context.Context,
	alp *anv1alpha1.AccessLogPolicy,
	reason gwv1alpha2.PolicyConditionReason,
	message string,
) error {
	status := metav1.ConditionTrue
	if reason != gwv1alpha2.PolicyReasonAccepted {
		status = metav1.ConditionFalse
	}

	alp.Status.Conditions = utils.GetNewConditions(alp.Status.Conditions, metav1.Condition{
		Type:               string(gwv1alpha2.PolicyConditionAccepted),
		ObservedGeneration: alp.Generation,
		Message:            message,
		Status:             status,
		Reason:             string(reason),
	})

	if err := r.client.Status().Update(ctx, alp); err != nil {
		r.eventRecorder.Event(alp, corev1.EventTypeWarning, k8s.FailedReconcileEvent,
			"Failed to update status due to "+err.Error())
		return fmt.Errorf("failed to set Accepted status to %s and reason to %s due to %s", status, reason, err)
	}

	return nil
}

func (r *accessLogPolicyReconciler) findImpactedAccessLogPolicies(ctx context.Context, eventObj client.Object) []reconcile.Request {
	listOptions := &client.ListOptions{
		Namespace: eventObj.GetNamespace(),
	}

	alps := &anv1alpha1.AccessLogPolicyList{}
	err := r.client.List(ctx, alps, listOptions)
	if err != nil {
		r.log.Errorf(ctx, "Failed to list all Access Log Policies, %s", err)
		return []reconcile.Request{}
	}

	requests := make([]reconcile.Request, 0, len(alps.Items))
	for _, alp := range alps.Items {
		if string(alp.Spec.TargetRef.Name) != eventObj.GetName() {
			continue
		}

		targetRefKind := string(alp.Spec.TargetRef.Kind)
		eventObjKind := reflect.TypeOf(eventObj).Elem().Name()
		if targetRefKind != eventObjKind {
			continue
		}

		r.log.Debugf(ctx, "Adding Access Log Policy %s/%s to queue due to %s event", alp.Namespace, alp.Name, targetRefKind)
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: alp.Namespace,
				Name:      alp.Name,
			},
		})
	}

	return requests
}

func (r *accessLogPolicyReconciler) targetRefToResourceName(
	alp *anv1alpha1.AccessLogPolicy,
	targetObj metav1.Object,
) (string, error) {
	targetRef := alp.Spec.TargetRef

	if targetRef.Kind == "Gateway" {
		return string(targetRef.Name), nil
	}

	if targetRef.Kind == "HTTPRoute" || targetRef.Kind == "GRPCRoute" {
		namespace := alp.Namespace
		if targetRef.Namespace != nil {
			namespace = string(*targetRef.Namespace)
		}

		serviceNameOverride, err := k8s.GetServiceNameOverrideWithValidation(targetObj)
		if err != nil {
			return "", fmt.Errorf("route '%s' has %w", targetRef.Name, err)
		}

		return utils.LatticeServiceName(string(targetRef.Name), namespace, serviceNameOverride), nil
	}

	return "", fmt.Errorf("unsupported targetRef kind: %s", targetRef.Kind)
}
