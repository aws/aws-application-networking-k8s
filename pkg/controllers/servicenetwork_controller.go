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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/controllers/predicates"
	deploy "github.com/aws/aws-application-networking-k8s/pkg/deploy/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	lattice_runtime "github.com/aws/aws-application-networking-k8s/pkg/runtime"
	"github.com/aws/aws-application-networking-k8s/pkg/utils"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

const (
	serviceNetworkFinalizer = "servicenetwork.k8s.aws/resources"
)

type serviceNetworkReconciler struct {
	log              gwlog.Logger
	client           client.Client
	cloud            pkg_aws.Cloud
	finalizerManager k8s.FinalizerManager
	snManager        deploy.ServiceNetworkManager
	eventRecorder    record.EventRecorder
}

//+kubebuilder:rbac:groups=application-networking.k8s.aws,resources=servicenetworks,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups=application-networking.k8s.aws,resources=servicenetworks/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=application-networking.k8s.aws,resources=servicenetworks/finalizers,verbs=update

func RegisterServiceNetworkController(
	log gwlog.Logger,
	cloud pkg_aws.Cloud,
	finalizerManager k8s.FinalizerManager,
	mgr ctrl.Manager,
) error {
	r := &serviceNetworkReconciler{
		log:              log,
		client:           mgr.GetClient(),
		cloud:            cloud,
		finalizerManager: finalizerManager,
		snManager:        deploy.NewDefaultServiceNetworkManager(log, cloud),
		eventRecorder:    mgr.GetEventRecorderFor("service-network-controller"),
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&anv1alpha1.ServiceNetwork{}, builder.WithPredicates(predicate.Or(predicate.GenerationChangedPredicate{}, predicates.AdditionalTagsAnnotationChangedPredicate))).
		Complete(r)
}

func (r *serviceNetworkReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctx = gwlog.StartReconcileTrace(ctx, r.log, "servicenetwork", req.Name, req.Namespace)
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

func (r *serviceNetworkReconciler) reconcile(ctx context.Context, req ctrl.Request) error {
	sn := &anv1alpha1.ServiceNetwork{}
	if err := r.client.Get(ctx, req.NamespacedName, sn); err != nil {
		return client.IgnoreNotFound(err)
	}

	r.eventRecorder.Event(sn, corev1.EventTypeNormal, k8s.ReconcilingEvent, "Started reconciling")

	var err error
	if !sn.DeletionTimestamp.IsZero() {
		err = r.reconcileDelete(ctx, sn)
	} else {
		err = r.reconcileUpsert(ctx, sn)
	}

	if err != nil {
		r.eventRecorder.Event(sn, corev1.EventTypeWarning, k8s.FailedReconcileEvent, fmt.Sprintf("Reconcile failed: %s", err))
		return err
	}

	r.eventRecorder.Event(sn, corev1.EventTypeNormal, k8s.ReconciledEvent, "Successfully reconciled")
	return nil
}

func (r *serviceNetworkReconciler) reconcileUpsert(ctx context.Context, sn *anv1alpha1.ServiceNetwork) error {
	if err := r.finalizerManager.AddFinalizers(ctx, sn, serviceNetworkFinalizer); err != nil {
		return err
	}

	additionalTags := k8s.GetAdditionalTagsFromAnnotations(ctx, sn)

	status, err := r.snManager.Upsert(ctx, sn.Name, additionalTags)
	if err != nil {
		r.updateStatus(ctx, sn, metav1.ConditionFalse, "ReconcileError", err.Error(), "", "")
		return err
	}

	r.updateStatus(ctx, sn, metav1.ConditionTrue, "Programmed", "ServiceNetwork is programmed", status.ServiceNetworkARN, status.ServiceNetworkID)
	return nil
}

func (r *serviceNetworkReconciler) reconcileDelete(ctx context.Context, sn *anv1alpha1.ServiceNetwork) error {
	// Check for active Gateways referencing this SN
	gwList := &gwv1.GatewayList{}
	if err := r.client.List(ctx, gwList); err != nil {
		return fmt.Errorf("failed to list gateways: %w", err)
	}
	for i := range gwList.Items {
		gw := &gwList.Items[i]
		if gw.Name == sn.Name && gw.DeletionTimestamp.IsZero() &&
			k8s.IsControlledByLatticeGatewayController(ctx, r.client, gw) {
			msg := fmt.Sprintf("Cannot delete: Gateway %s/%s still references this service network", gw.Namespace, gw.Name)
			r.updateStatus(ctx, sn, metav1.ConditionFalse, "DeleteBlocked", msg, sn.Status.ServiceNetworkARN, sn.Status.ServiceNetworkID)
			return errors.New(msg)
		}
	}

	if err := r.snManager.Delete(ctx, sn.Name); err != nil {
		r.updateStatus(ctx, sn, metav1.ConditionFalse, "DeleteError", err.Error(), sn.Status.ServiceNetworkARN, sn.Status.ServiceNetworkID)
		return err
	}

	return r.finalizerManager.RemoveFinalizers(ctx, sn, serviceNetworkFinalizer)
}

func (r *serviceNetworkReconciler) updateStatus(ctx context.Context, sn *anv1alpha1.ServiceNetwork, programmedStatus metav1.ConditionStatus, reason, message, arn, id string) {
	snOld := sn.DeepCopy()

	sn.Status.Conditions = utils.GetNewConditions(sn.Status.Conditions, metav1.Condition{
		Type:               "Accepted",
		Status:             metav1.ConditionTrue,
		ObservedGeneration: sn.Generation,
		Reason:             "Accepted",
		Message:            "ServiceNetwork is accepted",
	})
	sn.Status.Conditions = utils.GetNewConditions(sn.Status.Conditions, metav1.Condition{
		Type:               "Programmed",
		Status:             programmedStatus,
		ObservedGeneration: sn.Generation,
		Reason:             reason,
		Message:            message,
	})
	sn.Status.ServiceNetworkARN = arn
	sn.Status.ServiceNetworkID = id

	if err := r.client.Status().Patch(ctx, sn, client.MergeFrom(snOld)); err != nil {
		r.log.Errorf(ctx, "Failed to update ServiceNetwork status: %s", err)
	}
}
