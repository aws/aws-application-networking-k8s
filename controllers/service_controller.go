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

	"github.com/aws/aws-application-networking-k8s/controllers/eventhandlers"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/source"
	gateway_api "sigs.k8s.io/gateway-api/apis/v1beta1"
	mcs_api "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/deploy"
	"github.com/aws/aws-application-networking-k8s/pkg/gateway"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

const (
	// Typo
	serviceFinalizer = "service.ki8s.aws/resources"
)

// ServiceReconciler reconciles a Service object
type ServiceReconciler struct {
	log              gwlog.Logger
	Client           client.Client
	Scheme           *runtime.Scheme
	finalizerManager k8s.FinalizerManager
	eventRecorder    record.EventRecorder
	modelBuilder     gateway.LatticeTargetsBuilder
	stackDeployer    deploy.StackDeployer

	ds             *latticestore.LatticeDataStore
	stackMashaller deploy.StackMarshaller
}

func NewServiceReconciler(
	log gwlog.Logger,
	client client.Client,
	scheme *runtime.Scheme,
	eventRecorder record.EventRecorder,
	finalizerManager k8s.FinalizerManager,
	ds *latticestore.LatticeDataStore,
	cloud aws.Cloud) *ServiceReconciler {
	modelBuild := gateway.NewTargetsBuilder(client, cloud, ds)
	stackDeploy := deploy.NewTargetsStackDeploy(cloud, client, ds)
	stackMarshaller := deploy.NewDefaultStackMarshaller()

	return &ServiceReconciler{
		log:              log,
		Client:           client,
		Scheme:           scheme,
		finalizerManager: finalizerManager,
		modelBuilder:     modelBuild,
		stackDeployer:    stackDeploy,
		eventRecorder:    eventRecorder,
		ds:               ds,
		stackMashaller:   stackMarshaller,
	}

}

//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=core,resources=services/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=endpoints,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=endpoints/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=core,resources=configmaps, verbs=create;delete;patch;update;get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Service object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *ServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	svc := &corev1.Service{}
	if err := r.Client.Get(ctx, req.NamespacedName, svc); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	r.log.Infow("reconcile", "req", req)

	if !svc.DeletionTimestamp.IsZero() {
		tgNameD := latticestore.TargetGroupName(svc.Name, svc.Namespace)
		TGDeleted := r.ds.GetTargetGroupsByTG(tgNameD)
		for _, tg := range TGDeleted {
			r.log.Debugf("service deletion trigger target IP list registration %v and tg %v", tgNameD, tg)
			r.reconcileTargetsResource(ctx, svc, tg.TargetGroupKey.RouteName)

		}
		r.finalizerManager.RemoveFinalizers(ctx, svc, serviceFinalizer)

		return ctrl.Result{}, nil
	}

	// TODO also need to check serviceexport object to trigger building TargetGroup
	tgName := latticestore.TargetGroupName(svc.Name, svc.Namespace)
	TGs := r.ds.GetTargetGroupsByTG(tgName) // isServiceImport = false
	for _, tg := range TGs {
		r.log.Debugw("endpoints change trigger target IP list registration", "tg", tg)
		r.reconcileTargetsResource(ctx, svc, tg.TargetGroupKey.RouteName)
	}

	return ctrl.Result{}, nil
}

func (r *ServiceReconciler) reconcileTargetsResource(ctx context.Context, svc *corev1.Service, routename string) {
	if err := r.finalizerManager.AddFinalizers(ctx, svc, serviceFinalizer); err != nil {
		r.eventRecorder.Event(svc, corev1.EventTypeWarning, k8s.ServiceEventReasonFailedAddFinalizer, fmt.Sprintf("Failed and finalizer due %v", err))
	}

	r.buildAndDeployModel(ctx, svc, routename)
}

func (r *ServiceReconciler) buildAndDeployModel(ctx context.Context, svc *corev1.Service, routename string) (core.Stack, *latticemodel.Targets, error) {
	stack, latticeTargets, err := r.modelBuilder.Build(ctx, svc, routename)

	if err != nil {
		r.eventRecorder.Event(svc, corev1.EventTypeWarning,
			k8s.ServiceEventReasonFailedBuildModel, fmt.Sprintf("Failed build model due to %v", err))
		return nil, nil, err
	}

	jsonStack, _ := r.stackMashaller.Marshal(stack)
	r.log.Infow("successfully built model", "stack", jsonStack)

	if err := r.stackDeployer.Deploy(ctx, stack); err != nil {
		r.eventRecorder.Event(svc, corev1.EventTypeWarning,
			k8s.ServiceEventReasonFailedDeployModel, fmt.Sprintf("Failed deploy mode due to %v", err))
		return nil, nil, err
	}

	r.log.Infow("successfully deployed model", "service", svc.Name)
	return stack, latticeTargets, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	//TODO handle endslices
	epsEventsHandler := eventhandlers.NewEnqueueRequestEndpointEvent(r.Client)
	httpRouteEventHandler := eventhandlers.NewEnqueueRequestHTTPRouteEvent(r.Client)
	serviceExportHandler := eventhandlers.NewEqueueRequestServiceExportEvent(r.Client)

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Service{}).
		Watches(&source.Kind{Type: &corev1.Endpoints{}}, epsEventsHandler).
		Watches(&source.Kind{Type: &gateway_api.HTTPRoute{}}, httpRouteEventHandler).
		Watches(&source.Kind{Type: &mcs_api.ServiceExport{}}, serviceExportHandler).
		Complete(r)
}
