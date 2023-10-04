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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/deploy"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	lattice_runtime "github.com/aws/aws-application-networking-k8s/pkg/runtime"
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

	// Does targetRef exist?
	// If yes, proceed

	return nil
}

func (r *accessLogPolicyReconciler) targetRefExists(ctx context.Context, alp *anv1alpha1.AccessLogPolicy) bool {
	if alp.Spec.TargetRef.Kind == "Gateway" {

	} else if alp.Spec.TargetRef.Kind == "HTTPRoute" {

	} else if alp.Spec.TargetRef.Kind == "GRPCRoute" {

	} else {

	}
}
