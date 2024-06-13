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

	"github.com/aws/aws-application-networking-k8s/pkg/controllers/eventhandlers"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/deploy"
	"github.com/aws/aws-application-networking-k8s/pkg/gateway"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	lattice_runtime "github.com/aws/aws-application-networking-k8s/pkg/runtime"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	discoveryv1 "k8s.io/api/discovery/v1"
)

type serviceExportReconciler struct {
	log              gwlog.Logger
	client           client.Client
	Scheme           *runtime.Scheme
	finalizerManager k8s.FinalizerManager
	eventRecorder    record.EventRecorder
	modelBuilder     gateway.SvcExportTargetGroupModelBuilder
	stackDeployer    deploy.StackDeployer
	stackMarshaller  deploy.StackMarshaller
}

const (
	serviceExportFinalizer = "serviceexport.k8s.aws/resources"
)

func RegisterServiceExportController(
	log gwlog.Logger,
	cloud aws.Cloud,
	finalizerManager k8s.FinalizerManager,
	mgr ctrl.Manager,
) error {
	mgrClient := mgr.GetClient()
	scheme := mgr.GetScheme()
	eventRecorder := mgr.GetEventRecorderFor("serviceExport")

	modelBuilder := gateway.NewSvcExportTargetGroupBuilder(log, mgrClient)
	stackDeploy := deploy.NewTargetGroupStackDeploy(log, cloud, mgrClient)
	stackMarshaller := deploy.NewDefaultStackMarshaller()

	r := &serviceExportReconciler{
		log:              log,
		client:           mgrClient,
		Scheme:           scheme,
		finalizerManager: finalizerManager,
		modelBuilder:     modelBuilder,
		stackDeployer:    stackDeploy,
		eventRecorder:    eventRecorder,
		stackMarshaller:  stackMarshaller,
	}

	svcEventHandler := eventhandlers.NewServiceEventHandler(log, r.client)

	builder := ctrl.NewControllerManagedBy(mgr).
		For(&anv1alpha1.ServiceExport{}).
		Watches(&corev1.Service{}, svcEventHandler.MapToServiceExport()).
		Watches(&discoveryv1.EndpointSlice{}, svcEventHandler.MapToServiceExport())

	if ok, err := k8s.IsGVKSupported(mgr, anv1alpha1.GroupVersion.String(), anv1alpha1.TargetGroupPolicyKind); ok {
		builder.Watches(&anv1alpha1.TargetGroupPolicy{}, svcEventHandler.MapToServiceExport())
	} else {
		if err != nil {
			return err
		}
		log.Infof(context.TODO(), "TargetGroupPolicy CRD is not installed, skipping watch")
	}

	return builder.Complete(r)
}

//+kubebuilder:rbac:groups=application-networking.k8s.aws,resources=serviceexports,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=application-networking.k8s.aws,resources=serviceexports/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=application-networking.k8s.aws,resources=serviceexports/finalizers,verbs=update

func (r *serviceExportReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctx = gwlog.StartReconcileTrace(ctx, r.log, "serviceexport", req.Name, req.Namespace)
	defer func() {
		gwlog.EndReconcileTrace(ctx, r.log)
	}()

	recErr := r.reconcile(ctx, req)
	if recErr != nil {
		r.log.Infow(ctx, "reconcile error", "name", req.Name, "message", recErr.Error())
	}
	return lattice_runtime.HandleReconcileError(recErr)
}

func (r *serviceExportReconciler) reconcile(ctx context.Context, req ctrl.Request) error {
	srvExport := &anv1alpha1.ServiceExport{}

	if err := r.client.Get(ctx, req.NamespacedName, srvExport); err != nil {
		return client.IgnoreNotFound(err)
	}

	if srvExport.ObjectMeta.Annotations["application-networking.k8s.aws/federation"] != "amazon-vpc-lattice" {
		return nil
	}
	r.log.Debugf(ctx, "Found matching service export %s-%s", srvExport.Name, srvExport.Namespace)

	if !srvExport.DeletionTimestamp.IsZero() {
		if err := r.buildAndDeployModel(ctx, srvExport); err != nil {
			return err
		}
		err := r.finalizerManager.RemoveFinalizers(ctx, srvExport, serviceExportFinalizer)
		if err != nil {
			r.log.Errorf(ctx, "Failed to remove finalizers for service export %s-%s due to %s",
				srvExport.Name, srvExport.Namespace, err)
		}
		return nil
	} else {
		if err := r.finalizerManager.AddFinalizers(ctx, srvExport, serviceExportFinalizer); err != nil {
			r.eventRecorder.Event(srvExport, corev1.EventTypeWarning, k8s.GatewayEventReasonFailedAddFinalizer, fmt.Sprintf("Failed add finalizer due to %v", err))
			return errors.New("TODO")
		}

		err := r.buildAndDeployModel(ctx, srvExport)
		return err
	}
}

func (r *serviceExportReconciler) buildAndDeployModel(
	ctx context.Context,
	srvExport *anv1alpha1.ServiceExport,
) error {
	stack, err := r.modelBuilder.Build(ctx, srvExport)

	if err != nil {
		r.log.Debugf(ctx, "Failed to buildAndDeployModel for service export %s-%s due to %s",
			srvExport.Name, srvExport.Namespace, err)

		r.eventRecorder.Event(srvExport, corev1.EventTypeWarning,
			k8s.GatewayEventReasonFailedBuildModel,
			fmt.Sprintf("Failed BuildModel due to %s", err))

		return err
	}

	json, err := r.stackMarshaller.Marshal(stack)
	if err != nil {
		r.log.Errorf(ctx, "Error on marshalling model for service export %s-%s", srvExport.Name, srvExport.Namespace)
	}
	r.log.Debugf(ctx, "stack: %s", json)

	if err := r.stackDeployer.Deploy(ctx, stack); err != nil {
		r.eventRecorder.Event(srvExport, corev1.EventTypeWarning,
			k8s.ServiceExportEventReasonFailedDeployModel, fmt.Sprintf("Failed deploy model due to %s", err))
		return err
	}

	r.log.Debugf(ctx, "Successfully deployed model for service export %s-%s", srvExport.Name, srvExport.Namespace)
	return err
}
