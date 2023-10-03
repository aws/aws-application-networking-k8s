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
	"time"

	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	"github.com/pkg/errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

type gatewayClassReconciler struct {
	log                      gwlog.Logger
	client                   client.Client
	scheme                   *runtime.Scheme
	latticeControllerEnabled bool
}

func RegisterGatewayClassController(log gwlog.Logger, mgr ctrl.Manager) error {
	r := &gatewayClassReconciler{
		log:                      log,
		client:                   mgr.GetClient(),
		scheme:                   mgr.GetScheme(),
		latticeControllerEnabled: false,
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&gwv1beta1.GatewayClass{}).
		Complete(r)
}

//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gatewayclasses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gatewayclasses/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gatewayclasses/finalizers,verbs=update

func (r *gatewayClassReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.log.Infow("reconcile", "name", req.Name)

	gwClass := &gwv1beta1.GatewayClass{}
	if err := r.client.Get(ctx, req.NamespacedName, gwClass); err != nil {
		r.log.Debugw("gateway not found", "name", req.Name)
		return ctrl.Result{}, nil
	}

	if gwClass.Spec.ControllerName != config.LatticeGatewayControllerName {
		return ctrl.Result{}, nil
	}
	if !gwClass.DeletionTimestamp.IsZero() {
		r.latticeControllerEnabled = false
		r.log.Infow("deleted", "name", gwClass.Name)
		return ctrl.Result{}, nil
	}
	r.latticeControllerEnabled = true

	// Update Status
	gwClassOld := gwClass.DeepCopy()
	gwClass.Status.Conditions[0].LastTransitionTime = metav1.NewTime(time.Now())
	gwClass.Status.Conditions[0].ObservedGeneration = gwClass.Generation
	gwClass.Status.Conditions[0].Status = "True"
	gwClass.Status.Conditions[0].Message = string(gwv1beta1.GatewayClassReasonAccepted)
	gwClass.Status.Conditions[0].Reason = string(gwv1beta1.GatewayClassReasonAccepted)

	if err := r.client.Status().Patch(ctx, gwClass, client.MergeFrom(gwClassOld)); err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to update gatewayclass status")
	}

	r.log.Infow("reconciled", "name", gwClass.Name, "status", gwClass.Status)
	return ctrl.Result{}, nil
}
