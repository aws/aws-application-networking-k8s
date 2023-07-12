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

	"github.com/golang/glog"

	"github.com/aws/aws-application-networking-k8s/controllers/eventhandlers"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
	mcs_api "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/deploy"
	"github.com/aws/aws-application-networking-k8s/pkg/gateway"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	lattice_runtime "github.com/aws/aws-application-networking-k8s/pkg/runtime"
)

// ServiceExportReconciler reconciles a ServiceExport object
type ServiceExportReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	finalizerManager k8s.FinalizerManager
	eventRecorder    record.EventRecorder
	modelBuilder     gateway.TargetGroupModelBuilder
	stackDeployer    deploy.StackDeployer
	latticeDataStore *latticestore.LatticeDataStore
	stackMashaller   deploy.StackMarshaller
}

const (
	serviceExportFinalizer = "serviceexport.k8s.aws/resources"
)

func NewServiceExportReconciler(cloud aws.Cloud, client client.Client, scheme *runtime.Scheme, eventRecorder record.EventRecorder,
	finalizerManager k8s.FinalizerManager, latticeDataStore *latticestore.LatticeDataStore) *ServiceExportReconciler {
	modelBuilder := gateway.NewTargetGroupBuilder(client, latticeDataStore, cloud)
	stackDeploy := deploy.NewTargetGroupStackDeploy(cloud, client, latticeDataStore)
	stackMarshaller := deploy.NewDefaultStackMarshaller()

	return &ServiceExportReconciler{
		Client:           client,
		Scheme:           scheme,
		finalizerManager: finalizerManager,
		modelBuilder:     modelBuilder,
		stackDeployer:    stackDeploy,
		eventRecorder:    eventRecorder,
		latticeDataStore: latticeDataStore,
		stackMashaller:   stackMarshaller,
	}
}

//+kubebuilder:rbac:groups=multicluster.x-k8s.io,resources=serviceexports,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=multicluster.x-k8s.io,resources=serviceexports/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=multicluster.x-k8s.io,resources=serviceexports/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ServiceExport object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *ServiceExportReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return lattice_runtime.HandleReconcileError(r.reconcile(ctx, req))
}

func (r *ServiceExportReconciler) reconcile(ctx context.Context, req ctrl.Request) error {
	srvLog := log.FromContext(ctx)

	// TODO(user): your logic here
	srvLog.Info("ServiceExportReconciler")
	srvExport := &mcs_api.ServiceExport{}

	if err := r.Client.Get(ctx, req.NamespacedName, srvExport); err != nil {
		return client.IgnoreNotFound(err)
	}

	if srvExport.ObjectMeta.Annotations["multicluster.x-k8s.io/federation"] == "amazon-vpc-lattice" {
		glog.V(6).Infof("ServiceExportReconciler --- found matching service export --- %s\n", srvExport.Name)

		if !srvExport.DeletionTimestamp.IsZero() {
			srvLog.Info("Deleting")
			if err := r.cleanupServiceExportResources(ctx, srvExport); err != nil {
				glog.V(6).Infof("Failed to clean up service export %v, err :%v \n", srvExport, err)
				return err
			}

			srvLog.Info("Successfully delete")

			r.finalizerManager.RemoveFinalizers(ctx, srvExport, serviceExportFinalizer)
			return nil
		}

		return r.reconcileServiceExportResources(ctx, srvExport)
	}

	return nil
}

func (r *ServiceExportReconciler) cleanupServiceExportResources(ctx context.Context, srvExport *mcs_api.ServiceExport) error {
	_, _, err := r.buildAndDeployModel(ctx, srvExport)
	return err
}

func (r *ServiceExportReconciler) reconcileServiceExportResources(ctx context.Context, srvExport *mcs_api.ServiceExport) error {
	if err := r.finalizerManager.AddFinalizers(ctx, srvExport, serviceExportFinalizer); err != nil {
		r.eventRecorder.Event(srvExport, corev1.EventTypeWarning, k8s.GatewayEventReasonFailedAddFinalizer, fmt.Sprintf("Failed add finalizer due to %v", err))
		return errors.New("TODO")
	}

	_, _, err := r.buildAndDeployModel(ctx, srvExport)
	return err
}

func (r *ServiceExportReconciler) buildAndDeployModel(ctx context.Context, srvExport *mcs_api.ServiceExport) (core.Stack, *latticemodel.TargetGroup, error) {
	gwLog := log.FromContext(ctx)

	stack, targetGroup, err := r.modelBuilder.Build(ctx, srvExport)

	if err != nil {
		glog.V(6).Infof("Failed to buildAndDeployModel for service export %v\n", srvExport)

		r.eventRecorder.Event(srvExport, corev1.EventTypeWarning,
			k8s.GatewayEventReasonFailedBuildModel,
			fmt.Sprintf("Failed BuildModel due to %v", err))

		// Build failed means the K8S serviceexport, service are NOT ready to be deployed to lattice
		// return nil  to complete controller loop for current change.
		// TODO continue deploy to trigger reconcile of stale SDK objects
		//return stack, targetGroup, nil
	}
	glog.V(6).Infof("buildAndDeployModel: stack=%v, targetgroup=%v, err = %v\n", stack, targetGroup, err)

	stackJSON, err := r.stackMashaller.Marshal(stack)

	if err != nil {
		glog.V(6).Infof("Error on marshalling serviceExport model for name: %v namespace: %v\n", srvExport.Name, srvExport.Namespace)
	}

	gwLog.Info("Successfully built model", stackJSON, "")

	if err := r.stackDeployer.Deploy(ctx, stack); err != nil {
		r.eventRecorder.Event(srvExport, corev1.EventTypeWarning,
			k8s.ServiceExportEventReasonFailedDeployModel, fmt.Sprintf("Failed deploy model due to %v", err))
		return nil, nil, err
	}
	gwLog.Info("Successfully deployed model")

	return stack, targetGroup, err
}

// SetupWithManager sets up the controller with the Manager.
// TODO need to watch service event too
func (r *ServiceExportReconciler) SetupWithManager(mgr ctrl.Manager) error {
	svcEventsHandler := eventhandlers.NewEqueueRequestServiceEvent(r.Client)
	return ctrl.NewControllerManagedBy(mgr).
		// Uncomment the following line adding a pointer to an instance of the controlled resource as an argument
		For(&mcs_api.ServiceExport{}).
		Watches(&source.Kind{Type: &corev1.Service{}}, svcEventsHandler).
		Complete(r)
}
