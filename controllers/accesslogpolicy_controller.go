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
	"sigs.k8s.io/controller-runtime/pkg/predicate"
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
	evtRec := mgr.GetEventRecorderFor("accesslogpolicy")

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
		For(&anv1alpha1.AccessLogPolicy{}, pkg_builder.WithPredicates(predicate.GenerationChangedPredicate{}))

	return builder.Complete(r)
}

func (r *accessLogPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.log.Infow("reconcile", "name", req.Name)
	recErr := r.reconcile(ctx, req)
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

	if alp.Spec.TargetRef.Group != gwv1beta1.GroupName {
		message := "The targetRef's Group must be " + gwv1beta1.GroupName
		err := r.updateAccessLogPolicyStatus(ctx, alp, gwv1alpha2.PolicyReasonInvalid, message)
		if err != nil {
			return err
		}
		return nil
	}

	if !slices.Contains([]string{"Gateway", "HTTPRoute", "GRPCRoute"}, string(alp.Spec.TargetRef.Kind)) {
		message := "The targetRef's Kind must be Gateway, HTTPRoute, or GRPCRoute"
		err := r.updateAccessLogPolicyStatus(ctx, alp, gwv1alpha2.PolicyReasonInvalid, message)
		if err != nil {
			return err
		}
		return nil
	}

	if !alp.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, alp)
	} else {
		return r.reconcileUpsert(ctx, alp)
	}
}

func (r *accessLogPolicyReconciler) reconcileDelete(ctx context.Context, alp *anv1alpha1.AccessLogPolicy) error {
	_, err := r.buildAndDeployModel(ctx, alp)
	if err != nil {
		return err
	}

	err = r.finalizerManager.RemoveFinalizers(ctx, alp, accessLogPolicyFinalizer)
	if err != nil {
		return err
	}

	return nil
}

func (r *accessLogPolicyReconciler) reconcileUpsert(ctx context.Context, alp *anv1alpha1.AccessLogPolicy) error {
	if err := r.finalizerManager.AddFinalizers(ctx, alp, accessLogPolicyFinalizer); err != nil {
		r.eventRecorder.Event(alp, corev1.EventTypeWarning,
			k8s.AccessLogPolicyEventReasonFailedAddFinalizer, fmt.Sprintf("Failed to add finalizer due to %s", err))
		return err
	}

	targetRefExists, err := r.targetRefExists(ctx, alp)
	if err != nil {
		return err
	}
	if !targetRefExists {
		message := "The targetRef could not be found"
		err := r.updateAccessLogPolicyStatus(ctx, alp, gwv1alpha2.PolicyReasonTargetNotFound, message)
		if err != nil {
			return err
		}
		return nil
	}

	stack, err := r.buildAndDeployModel(ctx, alp)
	if err != nil {
		if services.IsConflictError(err) {
			message := "An Access Log Policy with a Destination Arn for the same destination type already exists for this targetRef"
			return r.updateAccessLogPolicyStatus(ctx, alp, gwv1alpha2.PolicyReasonConflicted, message)
		} else if services.IsInvalidError(err) {
			message := "The AWS resource with the provided Destination Arn could not be found"
			return r.updateAccessLogPolicyStatus(ctx, alp, gwv1alpha2.PolicyReasonInvalid, message)
		}
		return err
	}

	err = r.updateAccessLogPolicyAnnotations(ctx, alp, stack)
	if err != nil {
		return err
	}

	err = r.updateAccessLogPolicyStatus(ctx, alp, gwv1alpha2.PolicyReasonAccepted, config.LatticeGatewayControllerName)
	if err != nil {
		return err
	}

	return nil
}

func (r *accessLogPolicyReconciler) targetRefExists(ctx context.Context, alp *anv1alpha1.AccessLogPolicy) (bool, error) {
	targetRefNamespace := alp.Namespace
	if alp.Spec.TargetRef.Namespace != nil {
		targetRefNamespace = string(*alp.Spec.TargetRef.Namespace)
	}

	targetRefNamespacedName := types.NamespacedName{
		Name:      string(alp.Spec.TargetRef.Name),
		Namespace: targetRefNamespace,
	}

	var err error

	switch alp.Spec.TargetRef.Kind {
	case "Gateway":
		gw := &gwv1beta1.Gateway{}
		err = r.client.Get(ctx, targetRefNamespacedName, gw)
	case "HTTPRoute":
		httpRoute := &gwv1beta1.HTTPRoute{}
		err = r.client.Get(ctx, targetRefNamespacedName, httpRoute)
	case "GRPCRoute":
		grpcRoute := &gwv1alpha2.GRPCRoute{}
		err = r.client.Get(ctx, targetRefNamespacedName, grpcRoute)
	default:
		return false, fmt.Errorf("access Log Policy targetRef is for an unsupported Kind: %s",
			alp.Spec.TargetRef.Kind)
	}

	if err != nil && !errors.IsNotFound(err) {
		return false, err
	}

	return err == nil, nil
}

func (r *accessLogPolicyReconciler) buildAndDeployModel(
	ctx context.Context,
	alp *anv1alpha1.AccessLogPolicy,
) (core.Stack, error) {
	stack, _, err := r.modelBuilder.Build(ctx, alp)
	if err != nil {
		r.eventRecorder.Event(alp, corev1.EventTypeWarning, k8s.AccessLogPolicyEventReasonFailedBuildModel,
			fmt.Sprintf("Failed to build model due to %s", err))
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
		return fmt.Errorf("failed to set Access Log Policy Accepted status to %s and reason to %s, %w",
			status, reason, err)
	}

	return nil
}
