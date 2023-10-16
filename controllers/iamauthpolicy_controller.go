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

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	pkg_builder "sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwvv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/aws/aws-application-networking-k8s/controllers/eventhandlers"
	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/deploy"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	lattice_runtime "github.com/aws/aws-application-networking-k8s/pkg/runtime"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

const (
	authPolicyFinalizer = "iamauthpolicy.k8s.aws/resources"
)

type authPolicyReconciler struct {
	log              gwlog.Logger
	client           client.Client
	scheme           *runtime.Scheme
	finalizerManager k8s.FinalizerManager
	eventRecorder    record.EventRecorder
	cloud            aws.Cloud
	dataStore        *latticestore.LatticeDataStore
	stackMarshaller  deploy.StackMarshaller
}

func RegisterIAMAuthPolicyController(
	log gwlog.Logger,
	cloud aws.Cloud,
	dataStore *latticestore.LatticeDataStore,
	finalizerManager k8s.FinalizerManager,
	mgr ctrl.Manager,
) error {
	k8sClient := mgr.GetClient()
	scheme := mgr.GetScheme()
	evtRec := mgr.GetEventRecorderFor("iamauthpolicy")

	stackMarshaller := deploy.NewDefaultStackMarshaller()

	r := &authPolicyReconciler{
		log:              log,
		client:           k8sClient,
		scheme:           scheme,
		finalizerManager: finalizerManager,
		eventRecorder:    evtRec,
		cloud:            cloud,
		stackMarshaller:  stackMarshaller,
		dataStore:        dataStore,
	}

	gatewayEventHandler := eventhandlers.NewGatewayEventHandler(log, k8sClient)
	httpRouteEventHandler := eventhandlers.NewHTTPRouteEventHandler(log, k8sClient)
	grpcRouteEventHandler := eventhandlers.NewGRPCRouteEventHandler(log, k8sClient)

	builder := ctrl.NewControllerManagedBy(mgr).
		For(&anv1alpha1.IAMAuthPolicy{}, pkg_builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(&source.Kind{Type: &gwvv1beta1.Gateway{}}, gatewayEventHandler.MapToIAMAuthPolicies(), pkg_builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(&source.Kind{Type: &gwvv1beta1.HTTPRoute{}}, httpRouteEventHandler.MapToIAMAuthPolicies(), pkg_builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(&source.Kind{Type: &gwv1alpha2.GRPCRoute{}}, grpcRouteEventHandler.MapToIAMAuthPolicies(), pkg_builder.WithPredicates(predicate.GenerationChangedPredicate{}))
	return builder.Complete(r)
}

func (r *authPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
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

func (r *authPolicyReconciler) reconcile(ctx context.Context, req ctrl.Request) error {
	//TODO: implement reconcile
	return nil
}
