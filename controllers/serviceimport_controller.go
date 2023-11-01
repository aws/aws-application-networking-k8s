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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	mcs_api "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
)

type serviceImportReconciler struct {
	log              gwlog.Logger
	client           client.Client
	Scheme           *runtime.Scheme
	finalizerManager k8s.FinalizerManager
	eventRecorder    record.EventRecorder
}

const (
	serviceImportFinalizer = "serviceimport.k8s.aws/resource"
)

func RegisterServiceImportController(
	log gwlog.Logger,
	mgr ctrl.Manager,
	finalizerManager k8s.FinalizerManager,
) error {
	mgrClient := mgr.GetClient()
	scheme := mgr.GetScheme()
	eventRecorder := mgr.GetEventRecorderFor("ServiceImport")

	r := &serviceImportReconciler{
		log:              log,
		client:           mgrClient,
		Scheme:           scheme,
		finalizerManager: finalizerManager,
		eventRecorder:    eventRecorder,
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&mcs_api.ServiceImport{}).
		Complete(r)
}

//+kubebuilder:rbac:groups=multicluster.x-k8s.io,resources=serviceimports,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=multicluster.x-k8s.io,resources=serviceimports/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=multicluster.x-k8s.io,resources=serviceimports/finalizers,verbs=update

func (r *serviceImportReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.log.Infow("reconcile", "name", req.Name)

	serviceImport := &mcs_api.ServiceImport{}

	if err := r.client.Get(ctx, req.NamespacedName, serviceImport); err != nil {
		r.log.Info("Item Not Found")
		return ctrl.Result{}, nil
	}

	if !serviceImport.DeletionTimestamp.IsZero() {
		r.log.Info("Deleting")
		r.finalizerManager.RemoveFinalizers(ctx, serviceImport, serviceImportFinalizer)
		return ctrl.Result{}, nil
	} else {
		if err := r.finalizerManager.AddFinalizers(ctx, serviceImport, serviceImportFinalizer); err != nil {
			r.eventRecorder.Event(serviceImport, corev1.EventTypeWarning, k8s.ServiceImportEventReasonFailedAddFinalizer, fmt.Sprintf("Failed add finalizer due to %v", err))
			return ctrl.Result{}, nil
		}
		r.log.Info("Adding/Updating")

		return ctrl.Result{}, nil
	}
}
