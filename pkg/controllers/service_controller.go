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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	lattice_runtime "github.com/aws/aws-application-networking-k8s/pkg/runtime"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

const (
	serviceFinalizer = "service.ki8s.aws/resources"
)

type serviceReconciler struct {
	log              gwlog.Logger
	client           client.Client
	scheme           *runtime.Scheme
	finalizerManager k8s.FinalizerManager
	eventRecorder    record.EventRecorder
}

func RegisterServiceController(
	log gwlog.Logger,
	cloud aws.Cloud,
	finalizerManager k8s.FinalizerManager,
	mgr ctrl.Manager,
) error {
	client := mgr.GetClient()
	scheme := mgr.GetScheme()
	evtRec := mgr.GetEventRecorderFor("service")
	sr := &serviceReconciler{
		log:              log,
		client:           client,
		scheme:           scheme,
		finalizerManager: finalizerManager,
		eventRecorder:    evtRec,
	}
	err := ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Service{}).
		Complete(sr)
	return err
}

//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=core,resources=services/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=endpoints,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=endpoints/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=core,resources=configmaps, verbs=create;delete;patch;update;get;list;watch

func (r *serviceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.log.Infow("reconcile", "name", req.Name)
	recErr := r.reconcile(ctx, req)
	if recErr != nil {
		r.log.Infow("reconcile error", "name", req.Name, "message", recErr.Error())
	}
	return lattice_runtime.HandleReconcileError(recErr)
}

func (r *serviceReconciler) reconcile(ctx context.Context, req ctrl.Request) error {
	// today, target registration is handled through the route controller
	// if that proves not to be fast enough, we can look to do quicker
	// target registration with the service controller

	svc := &corev1.Service{}
	if err := r.client.Get(ctx, req.NamespacedName, svc); err != nil {
		return client.IgnoreNotFound(err)
	}
	if !svc.DeletionTimestamp.IsZero() {
		r.finalizerManager.RemoveFinalizers(ctx, svc, serviceFinalizer)
	}

	r.log.Infow("reconciled", "name", req.Name)
	return nil
}
