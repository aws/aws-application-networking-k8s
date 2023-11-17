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
	"fmt"
	"reflect"
	"strings"

	"golang.org/x/exp/slices"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
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
	evtRec := mgr.GetEventRecorderFor("access-log-policy")

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
		For(&anv1alpha1.AccessLogPolicy{}, pkg_builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(&gwv1beta1.Gateway{}, handler.EnqueueRequestsFromMapFunc(r.findImpactedAccessLogPolicies), pkg_builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(&gwv1beta1.HTTPRoute{}, handler.EnqueueRequestsFromMapFunc(r.findImpactedAccessLogPolicies), pkg_builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(&gwv1alpha2.GRPCRoute{}, handler.EnqueueRequestsFromMapFunc(r.findImpactedAccessLogPolicies), pkg_builder.WithPredicates(predicate.GenerationChangedPredicate{}))

	return builder.Complete(r)
}

func (r *accessLogPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.log.Infow("reconcile", "name", req.Name)
	recErr := r.reconcile(ctx, req)
	if recErr != nil {
		r.log.Infow("reconcile error", "name", req.Name, "message", recErr.Error())
	}
	res, retryErr := lattice_runtime.HandleReconcileError(recErr)
	if res.RequeueAfter != 0 {
		r.log.Infow("requeue request", "name", req.Name, "requeueAfter", res.RequeueAfter)
	} else if res.Requeue {
		r.log.Infow("requeue request", "name", req.Name)
	} else if retryErr == nil {
		r.log.Infow("reconciled", "name", req.Name)
	}
	return res, retryErr
}

func (r *accessLogPolicyReconciler) reconcile(ctx context.Context, req ctrl.Request) error {
	alp := &anv1alpha1.AccessLogPolicy{}
	if err := r.client.Get(ctx, req.NamespacedName, alp); err != nil {
		return client.IgnoreNotFound(err)
	}

	r.eventRecorder.Event(alp, corev1.EventTypeNormal, k8s.ReconcilingEvent, "Started reconciling")

	if !alp.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, alp)
	} else {
		return r.reconcileUpsert(ctx, alp)
	}
}

func (r *accessLogPolicyReconciler) reconcileDelete(ctx context.Context, alp *anv1alpha1.AccessLogPolicy) error {
	_, err := r.buildAndDeployModel(ctx, alp)
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

	// Validate Group
	if alp.Spec.TargetRef.Group != gwv1beta1.GroupName {
		message := fmt.Sprintf("The targetRef's Group must be \"%s\" but was \"%s\"",
			gwv1beta1.GroupName, alp.Spec.TargetRef.Group)
		r.eventRecorder.Event(alp, corev1.EventTypeWarning, k8s.FailedReconcileEvent, message)
		return r.updateAccessLogPolicyStatus(ctx, alp, gwv1alpha2.PolicyReasonInvalid, message)
	}

	// Validate targetRef Kind
	validKinds := []string{"Gateway", "HTTPRoute", "GRPCRoute"}
	if !slices.Contains(validKinds, string(alp.Spec.TargetRef.Kind)) {
		message := fmt.Sprintf("The targetRef's Kind must be \"Gateway\", \"HTTPRoute\", or \"GRPCRoute\""+
			" but was \"%s\"", alp.Spec.TargetRef.Kind)
		r.eventRecorder.Event(alp, corev1.EventTypeWarning, k8s.FailedReconcileEvent, message)
		return r.updateAccessLogPolicyStatus(ctx, alp, gwv1alpha2.PolicyReasonInvalid, message)
	}

	// Validate targetRef Namespace
	targetRefNamespace := k8s.NamespaceOrDefault(alp.Spec.TargetRef.Namespace)
	if targetRefNamespace != alp.Namespace {
		message := fmt.Sprintf("The targetRef's namespace, \"%s\", does not match the Access Log Policy's"+
			" namespace, \"%s\"", string(*alp.Spec.TargetRef.Namespace), alp.Namespace)
		r.eventRecorder.Event(alp, corev1.EventTypeWarning, k8s.FailedReconcileEvent, message)
		return r.updateAccessLogPolicyStatus(ctx, alp, gwv1alpha2.PolicyReasonInvalid, message)
	}

	// Check if targetRef exists
	targetRef, err := r.findTargetRef(ctx, alp)
	if err != nil {
		return err
	}
	if targetRef == nil {
		message := fmt.Sprintf("%s target \"%s/%s\" could not be found", alp.Spec.TargetRef.Kind, targetRefNamespace, alp.Spec.TargetRef.Name)
		r.eventRecorder.Event(alp, corev1.EventTypeWarning, k8s.FailedReconcileEvent, message)
		return r.updateAccessLogPolicyStatus(ctx, alp, gwv1alpha2.PolicyReasonTargetNotFound, message)
	}

	// Check for conflicting Access Log Policies
	alpsOnSameTarget := utils.SliceFilter(r.findAccessLogPoliciesOnObj(ctx, targetRef), func(p anv1alpha1.AccessLogPolicy) bool {
		return alp.Namespace != p.Namespace || alp.Name != p.Name
	})
	alpDestinationParts := strings.Split(*alp.Spec.DestinationArn, ":")
	for _, p := range alpsOnSameTarget {
		pDestinationParts := strings.Split(*p.Spec.DestinationArn, ":")
		if len(alpDestinationParts) > 3 && len(pDestinationParts) > 3 && alpDestinationParts[2] == pDestinationParts[2] {
			message := "An Access Log Policy with a Destination Arn for the same destination type already exists for this targetRef"
			r.eventRecorder.Event(alp, corev1.EventTypeWarning, k8s.FailedReconcileEvent, message)
			return r.updateAccessLogPolicyStatus(ctx, alp, gwv1alpha2.PolicyReasonConflicted, config.LatticeGatewayControllerName)
		}
	}

	err = r.updateAccessLogPolicyStatus(ctx, alp, gwv1alpha2.PolicyReasonAccepted, config.LatticeGatewayControllerName)
	if err != nil {
		return err
	}

	stack, err := r.buildAndDeployModel(ctx, alp)
	if err != nil {
		if services.IsConflictError(err) {
			message := "A VPC Lattice Access Log Subscription with a Destination Arn for the same destination type already exists for this targetRef"
			r.eventRecorder.Event(alp, corev1.EventTypeWarning, k8s.FailedReconcileEvent, message)
			r.log.Infow(message)
			return nil
		} else if services.IsInvalidError(err) {
			message := fmt.Sprintf("The AWS resource with Destination Arn \"%s\" could not be found", *alp.Spec.DestinationArn)
			r.eventRecorder.Event(alp, corev1.EventTypeWarning, k8s.FailedReconcileEvent, message)
			r.log.Infow(message)
			return nil
		}
		r.eventRecorder.Event(alp, corev1.EventTypeWarning, k8s.FailedReconcileEvent,
			"Failed to create or update due to "+err.Error())
		return err
	}

	err = r.updateAccessLogPolicyAnnotations(ctx, alp, stack)
	if err != nil {
		return err
	}

	r.eventRecorder.Event(alp, corev1.EventTypeNormal, k8s.ReconciledEvent, "Successfully reconciled")

	return nil
}

func (r *accessLogPolicyReconciler) findTargetRef(ctx context.Context, alp *anv1alpha1.AccessLogPolicy) (client.Object, error) {
	targetRefNamespacedName := types.NamespacedName{
		Name:      string(alp.Spec.TargetRef.Name),
		Namespace: k8s.NamespaceOrDefault(alp.Spec.TargetRef.Namespace),
	}

	var err error

	switch alp.Spec.TargetRef.Kind {
	case "Gateway":
		gw := &gwv1beta1.Gateway{}
		if err = r.client.Get(ctx, targetRefNamespacedName, gw); err == nil {
			return gw, nil
		}
	case "HTTPRoute":
		httpRoute := &gwv1beta1.HTTPRoute{}
		if err = r.client.Get(ctx, targetRefNamespacedName, httpRoute); err == nil {
			return httpRoute, nil
		}
	case "GRPCRoute":
		grpcRoute := &gwv1alpha2.GRPCRoute{}
		if err = r.client.Get(ctx, targetRefNamespacedName, grpcRoute); err == nil {
			return grpcRoute, nil
		}
	default:
		return nil, fmt.Errorf("Access Log Policy targetRef is for unsupported Kind: %s", alp.Spec.TargetRef.Kind)
	}

	if errors.IsNotFound(err) {
		return nil, nil
	}

	return nil, err
}

func (r *accessLogPolicyReconciler) buildAndDeployModel(
	ctx context.Context,
	alp *anv1alpha1.AccessLogPolicy,
) (core.Stack, error) {
	stack, _, err := r.modelBuilder.Build(ctx, alp)
	if err != nil {
		return nil, err
	}

	jsonStack, err := r.stackMarshaller.Marshal(stack)
	if err != nil {
		return nil, err
	}
	r.log.Debugw("Successfully built model", "stack", jsonStack)

	if err := r.stackDeployer.Deploy(ctx, stack); err != nil {
		return nil, err
	}
	r.log.Debugf("successfully deployed model for stack %s:%s", stack.StackID().Name, stack.StackID().Namespace)

	return stack, nil
}

func (r *accessLogPolicyReconciler) updateAccessLogPolicyAnnotations(
	ctx context.Context,
	alp *anv1alpha1.AccessLogPolicy,
	stack core.Stack,
) error {
	var accessLogSubscriptions []*model.AccessLogSubscription
	err := stack.ListResources(&accessLogSubscriptions)
	if err != nil {
		return err
	}

	for _, als := range accessLogSubscriptions {
		if als.Spec.EventType != core.DeleteEvent {
			oldAlp := alp.DeepCopy()
			if alp.ObjectMeta.Annotations == nil {
				alp.ObjectMeta.Annotations = make(map[string]string)
			}
			alp.ObjectMeta.Annotations[anv1alpha1.AccessLogSubscriptionAnnotationKey] = als.Status.Arn
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
	alps := r.findAccessLogPoliciesOnObj(ctx, eventObj)
	return utils.SliceMap(alps, func(alp anv1alpha1.AccessLogPolicy) reconcile.Request {
		return reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: alp.Namespace,
				Name:      alp.Name,
			},
		}
	})
}

func (r *accessLogPolicyReconciler) findAccessLogPoliciesOnObj(
	ctx context.Context,
	obj client.Object,
) []anv1alpha1.AccessLogPolicy {
	listOptions := &client.ListOptions{
		Namespace: obj.GetNamespace(),
	}

	alps := &anv1alpha1.AccessLogPolicyList{}
	err := r.client.List(ctx, alps, listOptions)
	if err != nil {
		r.log.Errorf("Failed to list all Access Log Policies, %s", err)
		return []anv1alpha1.AccessLogPolicy{}
	}

	alpsOnObj := make([]anv1alpha1.AccessLogPolicy, 0, len(alps.Items))
	for _, alp := range alps.Items {
		if string(alp.Spec.TargetRef.Name) != obj.GetName() {
			continue
		}

		targetRefKind := string(alp.Spec.TargetRef.Kind)
		eventObjKind := reflect.TypeOf(obj).Elem().Name()
		if targetRefKind != eventObjKind {
			continue
		}

		alpsOnObj = append(alpsOnObj, alp)
	}

	return alpsOnObj
}
