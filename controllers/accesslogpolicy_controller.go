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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/deploy"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	lattice_runtime "github.com/aws/aws-application-networking-k8s/pkg/runtime"
	"github.com/aws/aws-application-networking-k8s/pkg/utils"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

const (
	accesLogPolicyFinalizer = "accesslogpolicy.k8s.aws/resources"
)

type accessLogPolicyReconciler struct {
	log              gwlog.Logger
	client           client.Client
	scheme           *runtime.Scheme
	finalizerManager k8s.FinalizerManager
	eventRecorder    record.EventRecorder
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

	stackDeployer := deploy.NewServiceNetworkStackDeployer(log, cloud, mgrClient)
	stackMarshaller := deploy.NewDefaultStackMarshaller()

	r := &accessLogPolicyReconciler{
		log:              log,
		client:           mgrClient,
		scheme:           scheme,
		finalizerManager: finalizerManager,
		eventRecorder:    evtRec,
		stackDeployer:    stackDeployer,
		cloud:            cloud,
		stackMarshaller:  stackMarshaller,
	}

	builder := ctrl.NewControllerManagedBy(mgr).
		For(&anv1alpha1.AccessLogPolicy{})

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

	if !alp.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, alp)
	} else {
		return r.reconcileUpsert(ctx, alp)
	}
}

func (r *accessLogPolicyReconciler) reconcileDelete(ctx context.Context, alp *anv1alpha1.AccessLogPolicy) error {
	r.log.Infof("RECONCILING DELETE FOR ALP %+v", alp) // TODO: Remove me

	err := r.finalizerManager.RemoveFinalizers(ctx, alp, accesLogPolicyFinalizer)
	if err != nil {
		return err
	}

	return nil
}

func (r *accessLogPolicyReconciler) reconcileUpsert(ctx context.Context, alp *anv1alpha1.AccessLogPolicy) error {
	r.log.Infof("RECONCILING UPSERT FOR ALP %+v", alp) // TODO: Remove me

	if err := r.finalizerManager.AddFinalizers(ctx, alp, accesLogPolicyFinalizer); err != nil {
		r.eventRecorder.Event(alp, corev1.EventTypeWarning,
			k8s.AccessLogPolicyEventReasonFailedAddFinalizer, fmt.Sprintf("Failed to add finalizer due to %s", err))
		return err
	}

	if !r.targetRefExists(ctx, alp) {
		r.log.Infof("Could not find Acces Log Policy targetRef %s %s",
			alp.Spec.TargetRef.Kind, alp.Spec.TargetRef.Name)

		// TODO: Set status

		return nil
	}

	// TODO: Build AccessLogSubscription model

	// TODO: Set status

	return nil
}

func (r *accessLogPolicyReconciler) targetRefExists(ctx context.Context, alp *anv1alpha1.AccessLogPolicy) bool {
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
		gateway := &gwv1beta1.Gateway{}
		err = r.client.Get(ctx, targetRefNamespacedName, gateway)
	case "HTTPRoute":
		httpRoute := &gwv1beta1.HTTPRoute{}
		err = r.client.Get(ctx, targetRefNamespacedName, httpRoute)
	case "GRPCRoute":
		grpcRoute := &gwv1alpha2.GRPCRoute{}
		err = r.client.Get(ctx, targetRefNamespacedName, grpcRoute)
	default:
		r.log.Infof("Access Log Policy targetRef is for an unsupported Kind: %s", alp.Spec.TargetRef.Kind)
		return false
	}

	return err == nil
}

func (r *accessLogPolicyReconciler) updateAccessLogPolicyStatus(
	ctx context.Context,
	alp *anv1alpha1.AccessLogPolicy,
	reason gwv1alpha2.PolicyConditionReason,
) error {
	alpOld := alp.DeepCopy()

	status := metav1.ConditionTrue
	if reason != gwv1alpha2.PolicyReasonAccepted {
		status = metav1.ConditionFalse
	}

	condition := metav1.Condition{
		Type:               string(gwv1alpha2.PolicyConditionAccepted),
		ObservedGeneration: alp.Generation,
		Message:            config.LatticeGatewayControllerName,
		Status:             status,
		Reason:             string(reason),
	}

	alp.Status.Conditions = utils.GetNewConditions(alp.Status.Conditions, cond)

	if err := r.client.Status().Patch(ctx, alp, client.MergeFrom(alpOld)); err != nil {
		return fmt.Errorf("failed to set Access Log Policy Accepted status to %s and reason to %s, %w",
			status, reason, err)
	}

	return nil
}
