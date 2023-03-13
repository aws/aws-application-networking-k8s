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
	"github.com/pkg/errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	gateway_api "sigs.k8s.io/gateway-api/apis/v1beta1"
)

// GatewayClassReconciler reconciles a GatewayClass object
type GatewayClassReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

var latticeControllerEnabled = false

func NewGatewayGlassReconciler(client client.Client, scheme *runtime.Scheme) *GatewayClassReconciler {
	return &GatewayClassReconciler{
		Client: client,
		Scheme: scheme,
	}

}

//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gatewayclasses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gatewayclasses/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gatewayclasses/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the GatewayClass object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *GatewayClassReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	gwClassLog := log.FromContext(ctx)

	gwClass := &gateway_api.GatewayClass{}

	if err := r.Client.Get(ctx, req.NamespacedName, gwClass); err != nil {
		gwClassLog.Info("NotFound")
		return ctrl.Result{}, nil
	}
	gwClassLog.Info("ReconcileLoop")
	if gwClass.Spec.ControllerName == config.LatticeGatewayControllerName {
		if !gwClass.DeletionTimestamp.IsZero() {
			gwClassLog.Info("Deleting amazon-vpc-lattice GatewayClass\n")
			latticeControllerEnabled = false
			return ctrl.Result{}, nil
		}
		gwClassLog.Info("Creating LatticeGatewayClass ")
		latticeControllerEnabled = true

		// Update Status
		gwClassOld := gwClass.DeepCopy()

		gwClass.Status.Conditions[0].LastTransitionTime = metav1.NewTime(time.Now())

		gwClass.Status.Conditions[0].Status = "True"
		gwClass.Status.Conditions[0].Message = string(gateway_api.GatewayClassReasonAccepted)
		gwClass.Status.Conditions[0].Reason = string(gateway_api.GatewayClassReasonAccepted)

		if err := r.Client.Status().Patch(ctx, gwClass, client.MergeFrom(gwClassOld)); err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed to update gatewayclass status")
		}

	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GatewayClassReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		// Uncomment the following line adding a pointer to an instance of the controlled resource as an argument
		For(&gateway_api.GatewayClass{}).
		Complete(r)
}
