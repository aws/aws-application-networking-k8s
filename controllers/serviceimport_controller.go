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
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"

	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	mcs_api "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

// ServiceImportReconciler reconciles a ServiceImport object
type ServiceImportReconciler struct {
	log              gwlog.Logger
	client           client.Client
	Scheme           *runtime.Scheme
	finalizerManager k8s.FinalizerManager
	eventRecorder    record.EventRecorder
	latticeDataStore *latticestore.LatticeDataStore
}

const (
	serviceImportFinalizer = "serviceimport.k8s.aws/resource"
)

func RegisterServiceImportController(
	log gwlog.Logger,
	mgr ctrl.Manager,
	dataStore *latticestore.LatticeDataStore,
	finalizerManager k8s.FinalizerManager,
) error {
	mgrClient := mgr.GetClient()
	scheme := mgr.GetScheme()
	eventRecorder := mgr.GetEventRecorderFor("ServiceImport")

	r := &ServiceImportReconciler{
		log:              log,
		client:           mgrClient,
		Scheme:           scheme,
		finalizerManager: finalizerManager,
		eventRecorder:    eventRecorder,
		latticeDataStore: dataStore,
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&mcs_api.ServiceImport{}).
		Complete(r)
}

//+kubebuilder:rbac:groups=multicluster.x-k8s.io,resources=serviceimports,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=multicluster.x-k8s.io,resources=serviceimports/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=multicluster.x-k8s.io,resources=serviceimports/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ServiceImport object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *ServiceImportReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	reconcileLog := log.FromContext(ctx)

	// TODO(user): your logic here
	reconcileLog.Info("ServiceImportReconciler")

	serviceImport := &mcs_api.ServiceImport{}

	if err := r.client.Get(ctx, req.NamespacedName, serviceImport); err != nil {
		reconcileLog.Info("Item Not Found")
		return ctrl.Result{}, nil
	}

	if !serviceImport.DeletionTimestamp.IsZero() {
		reconcileLog.Info("Deleting")
		r.finalizerManager.RemoveFinalizers(ctx, serviceImport, serviceImportFinalizer)
		return ctrl.Result{}, nil
	}

	// Handle add
	if err := r.finalizerManager.AddFinalizers(ctx, serviceImport, serviceImportFinalizer); err != nil {
		r.eventRecorder.Event(serviceImport, corev1.EventTypeWarning, k8s.ServiceImportEventReasonFailedAddFinalizer, fmt.Sprintf("Failed add finalizer due to %v", err))
		return ctrl.Result{}, nil
	}

	reconcileLog.Info("Adding/Updating")

	return ctrl.Result{}, nil
}
