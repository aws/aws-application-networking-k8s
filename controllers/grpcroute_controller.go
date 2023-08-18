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

	"github.com/golang/glog"
	"github.com/pkg/errors"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
	gateway_api_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gateway_api_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	mcs_api "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/aws/aws-application-networking-k8s/controllers/eventhandlers"
	"github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/deploy"
	"github.com/aws/aws-application-networking-k8s/pkg/deploy/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/gateway"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	lattice_runtime "github.com/aws/aws-application-networking-k8s/pkg/runtime"
	"sigs.k8s.io/external-dns/endpoint"
)

// GRPCRouteReconciler reconciles a GRPCRoute object
type GRPCRouteReconciler struct {
	client           client.Client
	scheme           *runtime.Scheme
	finalizerManager k8s.FinalizerManager
	eventRecorder    record.EventRecorder
	modelBuilder     gateway.LatticeServiceBuilder
	stackDeployer    deploy.StackDeployer
	latticeDataStore *latticestore.LatticeDataStore
	stackMarshaller  deploy.StackMarshaller
}

const (
	grpcRouteFinalizer = "grpcroute.k8s.aws/resources"
)

func NewGrpcRouteReconciler(
	cloud aws.Cloud,
	client client.Client,
	scheme *runtime.Scheme,
	eventRecorder record.EventRecorder,
	finalizerManager k8s.FinalizerManager,
	latticeDataStore *latticestore.LatticeDataStore,
) *GRPCRouteReconciler {
	modelBuilder := gateway.NewLatticeServiceBuilder(client, latticeDataStore, cloud)
	stackDeployer := deploy.NewLatticeServiceStackDeploy(cloud, client, latticeDataStore)
	stackMarshaller := deploy.NewDefaultStackMarshaller()

	return &GRPCRouteReconciler{
		client:           client,
		scheme:           scheme,
		finalizerManager: finalizerManager,
		modelBuilder:     modelBuilder,
		stackDeployer:    stackDeployer,
		eventRecorder:    eventRecorder,
		latticeDataStore: latticeDataStore,
		stackMarshaller:  stackMarshaller,
	}
}

//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=grpcroutes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=grpcroutes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=grpcroutes/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the GRPCRoute object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *GRPCRouteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return lattice_runtime.HandleReconcileError(r.reconcile(ctx, req))
}

func (r *GRPCRouteReconciler) reconcile(ctx context.Context, req ctrl.Request) error {
	httpLog := log.FromContext(ctx)

	httpLog.Info("GRPCRouteReconciler")

	grpcRoute := &core.GRPCRoute{}

	if err := r.client.Get(ctx, req.NamespacedName, grpcRoute.K8sObject()); err != nil {
		return client.IgnoreNotFound(err)
	}

	if !r.isGRPCRouteRelevant(ctx, grpcRoute) {
		// not relevant
		return nil
	}

	if !grpcRoute.DeletionTimestamp().IsZero() {
		httpLog.Info("Deleting")
		r.eventRecorder.Event(grpcRoute.K8sObject(), corev1.EventTypeNormal,
			k8s.RouteEventReasonReconcile, "Deleting Reconcile")
		if err := r.cleanupGRPCRouteResources(ctx, grpcRoute); err != nil {
			glog.V(6).Infof("Failed to cleanup GRPCRoute %v err %v\n", grpcRoute, err)
			return err
		}
		UpdateGRPCRouteListenerStatus(ctx, r.client, grpcRoute)
		r.finalizerManager.RemoveFinalizers(ctx, grpcRoute.K8sObject(), grpcRouteFinalizer)

		// TODO delete metrics
		return nil
	} else {
		httpLog.Info("Adding/Updating")
		r.eventRecorder.Event(grpcRoute.K8sObject(), corev1.EventTypeNormal,
			k8s.RouteEventReasonReconcile, "Adding/Updating Reconcile")
		err := r.reconcileGRPCRouteResource(ctx, grpcRoute)
		// TODO add/update metrics
		return err
	}

}

func UpdateGRPCRouteListenerStatus(ctx context.Context, k8sClient client.Client, grpcRoute *core.GRPCRoute) error {
	gw := &gateway_api_v1beta1.Gateway{}

	gwNamespace := grpcRoute.Namespace()
	if grpcRoute.Spec().ParentRefs()[0].Namespace != nil {
		gwNamespace = string(*grpcRoute.Spec().ParentRefs()[0].Namespace)
	}
	gwName := types.NamespacedName{
		Namespace: gwNamespace,
		// TODO assume one parent for now and point to service network
		Name: string(grpcRoute.Spec().ParentRefs()[0].Name),
	}

	if err := k8sClient.Get(ctx, gwName, gw); err != nil {
		return errors.Wrapf(err, "update route listener: gw not found, gw: %s", gwName)
	}

	return UpdateGWListenerStatus(ctx, k8sClient, gw)
}

func (r *GRPCRouteReconciler) cleanupGRPCRouteResources(ctx context.Context, grpcRoute core.Route) error {

	_, _, err := r.buildAndDeployModel(ctx, grpcRoute)

	return err
}

func (r *GRPCRouteReconciler) isGRPCRouteRelevant(ctx context.Context, grpcRoute *core.GRPCRoute) bool {
	if len(grpcRoute.Spec().ParentRefs()) == 0 {
		glog.V(2).Infof("Ignore GRPCRoute which has no ParentRefs gateway %v \n ", grpcRoute.Spec())
		return false
	}

	gw := &gateway_api_v1beta1.Gateway{}

	gwNamespace := grpcRoute.Namespace()
	if grpcRoute.Spec().ParentRefs()[0].Namespace != nil {
		gwNamespace = string(*grpcRoute.Spec().ParentRefs()[0].Namespace)
	}
	gwName := types.NamespacedName{
		Namespace: gwNamespace,
		Name:      string(grpcRoute.Spec().ParentRefs()[0].Name),
	}

	if err := r.client.Get(ctx, gwName, gw); err != nil {
		glog.V(6).Infof("Could not find gateway %s: %s\n", gwName.String(), err.Error())
		glog.V(6).Infof("Ignore GRPCRoute whose ParentRef gateway object has NOT defined yet for %v\n", grpcRoute.Spec())
		return false
	}

	// make sure gateway is a aws-vpc-lattice
	gwClass := &gateway_api_v1beta1.GatewayClass{}
	gwClassName := types.NamespacedName{
		Namespace: defaultNameSpace,
		Name:      string(gw.Spec.GatewayClassName),
	}

	if err := r.client.Get(ctx, gwClassName, gwClass); err != nil {
		glog.V(6).Infof("Ignore GRPCRoute that NOT controlled by any GatewayClass for %v\n", grpcRoute.Spec())
		return false
	}

	if gwClass.Spec.ControllerName == config.LatticeGatewayControllerName {
		glog.V(6).Infof("Found aws-vpc-lattice for GRPCRoute for %v\n", grpcRoute.Spec())

		return true
	} else {
		glog.V(6).Infof("Ignore non aws-vpc-lattice GRPCRoute !!! %v\n", grpcRoute.Spec())
		return false
	}
}

func (r *GRPCRouteReconciler) buildAndDeployModel(ctx context.Context, route core.Route) (core.Stack, *latticemodel.Service, error) {
	httpLog := log.FromContext(ctx)

	stack, latticeService, err := r.modelBuilder.Build(ctx, route)

	if err != nil {

		r.eventRecorder.Event(route.K8sObject(), corev1.EventTypeWarning,
			k8s.RouteEventReasonFailedBuildModel, fmt.Sprintf("Failed build model due to %+v", err))
		glog.V(6).Infof("buildAndDeployModel, Failed build model for %v due to %+v\n", route.Name(), err)

		// Build failed
		// TODO continue deploy to trigger reconcile of stale GRPCRoute and policy
		return nil, nil, err
	}

	stackJSON, err := r.stackMarshaller.Marshal(stack)
	if err != nil {
		//TODO
		glog.V(6).Infof("error on r.stackMarshaller.Marshal error %v \n", err)
	}

	httpLog.Info("Successfully built model:", stackJSON, "")

	if err := r.stackDeployer.Deploy(ctx, stack); err != nil {
		glog.V(6).Infof("GRPCRouteReconciler: Failed deploy %s due to err %v \n", route.Name(), err)

		var retryErr = errors.New(lattice.LATTICE_RETRY)

		if errors.As(err, &retryErr) {
			r.eventRecorder.Event(route.K8sObject(), corev1.EventTypeNormal,
				k8s.RouteEventReasonRetryReconcile, "retry reconcile...")

		} else {
			r.eventRecorder.Event(route.K8sObject(), corev1.EventTypeWarning,
				k8s.RouteEventReasonFailedDeployModel, fmt.Sprintf("Failed deploy model due to %v", err))
		}
		return nil, nil, err
	}

	httpLog.Info("Successfully deployed model")

	return stack, latticeService, err
}

func (r *GRPCRouteReconciler) reconcileGRPCRouteResource(ctx context.Context, grpcRoute *core.GRPCRoute) error {
	glog.V(6).Infof("Beginning -- reconcileGRPCRouteResource, [%v]\n", grpcRoute)

	if err := r.finalizerManager.AddFinalizers(ctx, grpcRoute.K8sObject(), grpcRouteFinalizer); err != nil {
		r.eventRecorder.Event(grpcRoute.K8sObject(), corev1.EventTypeWarning, k8s.RouteEventReasonFailedAddFinalizer, fmt.Sprintf("Failed add finalizer due to %v", err))
	}

	_, _, err := r.buildAndDeployModel(ctx, grpcRoute)

	//TODO add metric

	if err == nil {
		r.eventRecorder.Event(grpcRoute.K8sObject(), corev1.EventTypeNormal,
			k8s.RouteEventReasonDeploySucceed, "Adding/Updating reconcile Done!")

		serviceStatus, err1 := r.latticeDataStore.GetLatticeService(grpcRoute.Name(), grpcRoute.Namespace())

		if err1 == nil {
			r.updateGRPCRouteStatus(ctx, serviceStatus.DNS, grpcRoute)
		}
	}

	return err

}

func (r *GRPCRouteReconciler) updateGRPCRouteStatus(ctx context.Context, dns string, coreRoute *core.GRPCRoute) error {
	glog.V(6).Infof("updateGRPCRouteStatus: grpcRoute %v, dns %v\n", coreRoute, dns)
	grpcRoute := coreRoute.Inner()
	grpcrouteOld := grpcRoute.DeepCopy()

	if len(grpcRoute.ObjectMeta.Annotations) == 0 {
		grpcRoute.ObjectMeta.Annotations = make(map[string]string)
	}

	grpcRoute.ObjectMeta.Annotations[LatticeAssignedDomainName] = dns
	if err := r.client.Patch(ctx, grpcRoute, client.MergeFrom(grpcrouteOld)); err != nil {
		glog.V(2).Infof("updateGRPCRouteStatus: Patch() received err %v \n", err)
		return errors.Wrapf(err, "failed to update grpcRoute status")
	}
	grpcrouteOld = grpcRoute.DeepCopy()

	if len(grpcRoute.Status.RouteStatus.Parents) == 0 {
		grpcRoute.Status.RouteStatus.Parents = make([]gateway_api_v1beta1.RouteParentStatus, 1)
	}
	grpcRoute.Status.RouteStatus.Parents[0].ParentRef = grpcRoute.Spec.ParentRefs[0]
	grpcRoute.Status.RouteStatus.Parents[0].ControllerName = config.LatticeGatewayControllerName

	// Update listener Status
	if err := UpdateGRPCRouteListenerStatus(ctx, r.client, coreRoute); err != nil {
		updateGRPCRouteCondition(coreRoute, metav1.Condition{
			Type:               string(gateway_api_v1beta1.RouteConditionAccepted),
			Status:             metav1.ConditionFalse,
			ObservedGeneration: grpcRoute.Generation,
			Reason:             string(gateway_api_v1beta1.RouteReasonNoMatchingParent),
			Message:            fmt.Sprintf("Could not match gateway %s: %s", grpcRoute.Spec.ParentRefs[0].Name, err.Error()),
		})
	} else {
		updateGRPCRouteCondition(coreRoute, metav1.Condition{
			Type:               string(gateway_api_v1beta1.RouteConditionAccepted),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: grpcRoute.Generation,
			Reason:             string(gateway_api_v1beta1.RouteReasonAccepted),
			Message:            fmt.Sprintf("DNS Name: %s", dns),
		})
		updateGRPCRouteCondition(coreRoute, metav1.Condition{
			Type:               string(gateway_api_v1beta1.RouteConditionResolvedRefs),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: grpcRoute.Generation,
			Reason:             string(gateway_api_v1beta1.RouteReasonResolvedRefs),
			Message:            fmt.Sprintf("DNS Name: %s", dns),
		})
	}

	if err := r.client.Status().Patch(ctx, grpcRoute, client.MergeFrom(grpcrouteOld)); err != nil {
		glog.V(2).Infof("updateGRPCRouteStatus: Patch() received err %v \n", err)
		return errors.Wrapf(err, "failed to update grpcRoute status")
	}
	glog.V(6).Infof("updateGRPCRouteStatus patched dns %v \n", dns)

	return nil
}

func updateGRPCRouteCondition(grpcRoute *core.GRPCRoute, updated metav1.Condition) {
	grpcRoute.Status().Parents()[0].Conditions = updateCondition(grpcRoute.Status().Parents()[0].Conditions, updated)
}

// SetupWithManager sets up the controller with the Manager.
func (r *GRPCRouteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	gwEventHandler := eventhandlers.NewEnqueueRequestGatewayEvent(r.client)
	svcEventHandler := eventhandlers.NewEqueueHTTPRequestServiceEvent(r.client)
	svcImportEventHandler := eventhandlers.NewEqueueRequestServiceImportEvent(r.client)

	builder := ctrl.NewControllerManagedBy(mgr).
		For(&gateway_api_v1alpha2.GRPCRoute{}).
		Watches(&source.Kind{Type: &gateway_api_v1beta1.Gateway{}}, gwEventHandler).
		Watches(&source.Kind{Type: &corev1.Service{}}, svcEventHandler).
		Watches(&source.Kind{Type: &mcs_api.ServiceImport{}}, svcImportEventHandler)

	if ok, err := isExternalDNSSupported(mgr); ok {
		builder.Owns(&endpoint.DNSEndpoint{})
	} else {
		// This means DNSEndpoint CRD does not exist which is fine, but getting API error is not.
		if err != nil {
			glog.V(2).Infof("Unknown error while discovering CRD: %v", err)
			return err
		}
	}
	return builder.Complete(r)
}
