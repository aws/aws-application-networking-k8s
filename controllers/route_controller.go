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
)

type RouteType string

const (
	HTTP RouteType = "http"
	GRPC RouteType = "grpc"
)

var routeTypeToFinalizer = map[RouteType]string{
	HTTP: "httproute.k8s.aws/resources",
	GRPC: "grpcroute.k8s.aws/resources",
}

// RouteReconciler reconciles a HTTPRoute and GRPCRoute objects
type RouteReconciler struct {
	routeType        RouteType
	log              gwlog.Logger
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
	LatticeAssignedDomainName = "application-networking.k8s.aws/lattice-assigned-domain-name"
)

func RegisterAllRouteControllers(
	log gwlog.Logger,
	cloud aws.Cloud,
	datastore *latticestore.LatticeDataStore,
	finalizerManager k8s.FinalizerManager,
	mgr ctrl.Manager,
) error {
	mgrClient := mgr.GetClient()
	gwEventHandler := eventhandlers.NewEnqueueRequestGatewayEvent(mgrClient)
	svcEventHandler := eventhandlers.NewEqueueHTTPRequestServiceEvent(mgrClient)
	svcImportEventHandler := eventhandlers.NewEqueueRequestServiceImportEvent(mgrClient)

	routeInfos := []struct {
		routeType      RouteType
		gatewayApiType client.Object
	}{
		{HTTP, &gateway_api_v1beta1.HTTPRoute{}},
		{GRPC, &gateway_api_v1alpha2.GRPCRoute{}},
	}

	for _, routeInfo := range routeInfos {
		reconciler := RouteReconciler{
			routeType:        routeInfo.routeType,
			log:              log,
			client:           mgrClient,
			scheme:           mgr.GetScheme(),
			finalizerManager: finalizerManager,
			eventRecorder:    mgr.GetEventRecorderFor(string(routeInfo.routeType) + "route"),
			latticeDataStore: datastore,
			modelBuilder:     gateway.NewLatticeServiceBuilder(mgrClient, datastore, cloud),
			stackDeployer:    deploy.NewLatticeServiceStackDeploy(cloud, mgrClient, datastore),
			stackMarshaller:  deploy.NewDefaultStackMarshaller(),
		}

		err := ctrl.NewControllerManagedBy(mgr).
			For(routeInfo.gatewayApiType).
			Watches(&source.Kind{Type: &gateway_api_v1beta1.Gateway{}}, gwEventHandler).
			Watches(&source.Kind{Type: &corev1.Service{}}, svcEventHandler).
			Watches(&source.Kind{Type: &mcs_api.ServiceImport{}}, svcImportEventHandler).
			Complete(&reconciler)

		if err != nil {
			return err
		}
	}

	return nil
}

//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=grpcroutes;httproutes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=grpcroutes/status;httproutes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=grpcroutes/finalizers;httproutes/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Route object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *RouteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return lattice_runtime.HandleReconcileError(r.reconcile(ctx, req))
}

func (r *RouteReconciler) reconcile(ctx context.Context, req ctrl.Request) error {
	route, err := r.getRoute(ctx, req)
	if err != nil {
		return client.IgnoreNotFound(err)
	}

	if err := r.client.Get(ctx, req.NamespacedName, route.K8sObject()); err != nil {
		return client.IgnoreNotFound(err)
	}

	if !r.isRouteRelevant(ctx, route) {
		// not relevant
		return nil
	}

	if !route.DeletionTimestamp().IsZero() {
		r.log.Info("Deleting")
		r.eventRecorder.Event(route.K8sObject(), corev1.EventTypeNormal,
			k8s.RouteEventReasonReconcile, "Deleting Reconcile")
		if err := r.cleanupGRPCRouteResources(ctx, route); err != nil {
			glog.V(6).Infof("Failed to cleanup GRPCRoute %v err %v\n", route, err)
			return err
		}
		updateRouteListenerStatus(ctx, r.client, route)
		r.finalizerManager.RemoveFinalizers(ctx, route.K8sObject(), routeTypeToFinalizer[r.routeType])

		// TODO delete metrics
		return nil
	} else {
		r.log.Info("Adding/Updating")
		r.eventRecorder.Event(route.K8sObject(), corev1.EventTypeNormal,
			k8s.RouteEventReasonReconcile, "Adding/Updating Reconcile")
		err := r.reconcileRouteResource(ctx, route)
		// TODO add/update metrics
		return err
	}
}

func (r *RouteReconciler) getRoute(ctx context.Context, req ctrl.Request) (core.Route, error) {
	switch r.routeType {
	case HTTP:
		httpRoute := &core.HTTPRoute{}
		err := r.client.Get(ctx, req.NamespacedName, httpRoute.K8sObject())
		return httpRoute, err
	case GRPC:
		grpcRoute := &core.GRPCRoute{}
		err := r.client.Get(ctx, req.NamespacedName, grpcRoute.K8sObject())
		return grpcRoute, err
	default:
		return nil, fmt.Errorf("unknown route type for type %s", string(r.routeType))
	}
}

func updateRouteListenerStatus(ctx context.Context, k8sClient client.Client, route core.Route) error {
	gw := &gateway_api_v1beta1.Gateway{}

	gwNamespace := route.Namespace()
	if route.Spec().ParentRefs()[0].Namespace != nil {
		gwNamespace = string(*route.Spec().ParentRefs()[0].Namespace)
	}
	gwName := types.NamespacedName{
		Namespace: gwNamespace,
		// TODO assume one parent for now and point to service network
		Name: string(route.Spec().ParentRefs()[0].Name),
	}

	if err := k8sClient.Get(ctx, gwName, gw); err != nil {
		return errors.Wrapf(err, "update route listener: gw not found, gw: %s", gwName)
	}

	return UpdateGWListenerStatus(ctx, k8sClient, gw)
}

func (r *RouteReconciler) cleanupGRPCRouteResources(ctx context.Context, grpcRoute core.Route) error {

	_, _, err := r.buildAndDeployModel(ctx, grpcRoute)

	return err
}

func (r *RouteReconciler) isRouteRelevant(ctx context.Context, route core.Route) bool {
	if len(route.Spec().ParentRefs()) == 0 {
		glog.V(2).Infof("Ignore GRPCRoute which has no ParentRefs gateway %v \n ", route.Spec())
		return false
	}

	gw := &gateway_api_v1beta1.Gateway{}

	gwNamespace := route.Namespace()
	if route.Spec().ParentRefs()[0].Namespace != nil {
		gwNamespace = string(*route.Spec().ParentRefs()[0].Namespace)
	}
	gwName := types.NamespacedName{
		Namespace: gwNamespace,
		Name:      string(route.Spec().ParentRefs()[0].Name),
	}

	if err := r.client.Get(ctx, gwName, gw); err != nil {
		glog.V(6).Infof("Could not find gateway %s: %s\n", gwName.String(), err.Error())
		glog.V(6).Infof("Ignore GRPCRoute whose ParentRef gateway object has NOT defined yet for %v\n", route.Spec())
		return false
	}

	// make sure gateway is an aws-vpc-lattice
	gwClass := &gateway_api_v1beta1.GatewayClass{}
	gwClassName := types.NamespacedName{
		Namespace: defaultNameSpace,
		Name:      string(gw.Spec.GatewayClassName),
	}

	if err := r.client.Get(ctx, gwClassName, gwClass); err != nil {
		glog.V(6).Infof("Ignore GRPCRoute that NOT controlled by any GatewayClass for %v\n", route.Spec())
		return false
	}

	if gwClass.Spec.ControllerName == config.LatticeGatewayControllerName {
		glog.V(6).Infof("Found aws-vpc-lattice for GRPCRoute for %v\n", route.Spec())

		return true
	} else {
		glog.V(6).Infof("Ignore non aws-vpc-lattice GRPCRoute !!! %v\n", route.Spec())
		return false
	}
}

func (r *RouteReconciler) buildAndDeployModel(ctx context.Context, route core.Route) (core.Stack, *latticemodel.Service, error) {
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
		glog.V(6).Infof("RouteReconciler: Failed deploy %s due to err %v \n", route.Name(), err)

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

func (r *RouteReconciler) reconcileRouteResource(ctx context.Context, route core.Route) error {
	glog.V(6).Infof("Beginning -- reconcileRouteResource, [%v]\n", route)

	if err := r.finalizerManager.AddFinalizers(ctx, route.K8sObject(), routeTypeToFinalizer[r.routeType]); err != nil {
		r.eventRecorder.Event(route.K8sObject(), corev1.EventTypeWarning, k8s.RouteEventReasonFailedAddFinalizer, fmt.Sprintf("Failed add finalizer due to %v", err))
	}

	_, _, err := r.buildAndDeployModel(ctx, route)

	//TODO add metric

	if err == nil {
		r.eventRecorder.Event(route.K8sObject(), corev1.EventTypeNormal,
			k8s.RouteEventReasonDeploySucceed, "Adding/Updating reconcile Done!")

		serviceStatus, err1 := r.latticeDataStore.GetLatticeService(route.Name(), route.Namespace())

		if err1 == nil {
			switch coreRoute := route.(type) {
			case *core.HTTPRoute:
				r.updateHTTPRouteStatus(ctx, serviceStatus.DNS, coreRoute)
			case *core.GRPCRoute:
				r.updateGRPCRouteStatus(ctx, serviceStatus.DNS, coreRoute)
			default:
				return fmt.Errorf("unsupported route type for route %+v, routeType %s", coreRoute, r.routeType)
			}
		}
	}

	return err
}

func (r *RouteReconciler) updateHTTPRouteStatus(ctx context.Context, dns string, coreRoute *core.HTTPRoute) error {
	glog.V(6).Infof("updateHTTPRouteStatus: httpRoute %v, dns %v\n", coreRoute, dns)
	httpRoute := coreRoute.Inner()
	httprouteOld := httpRoute.DeepCopy()

	if len(httpRoute.ObjectMeta.Annotations) == 0 {
		httpRoute.ObjectMeta.Annotations = make(map[string]string)
	}

	httpRoute.ObjectMeta.Annotations[LatticeAssignedDomainName] = dns
	if err := r.client.Patch(ctx, httpRoute, client.MergeFrom(httprouteOld)); err != nil {
		glog.V(2).Infof("updateHTTPRouteStatus: Patch() received err %v \n", err)
		return errors.Wrapf(err, "failed to update httpRoute status")
	}
	httprouteOld = httpRoute.DeepCopy()

	if len(httpRoute.Status.RouteStatus.Parents) == 0 {
		httpRoute.Status.RouteStatus.Parents = make([]gateway_api_v1beta1.RouteParentStatus, 1)
	}
	httpRoute.Status.RouteStatus.Parents[0].ParentRef = httpRoute.Spec.ParentRefs[0]
	httpRoute.Status.RouteStatus.Parents[0].ControllerName = config.LatticeGatewayControllerName

	// Update listener Status
	if err := updateRouteListenerStatus(ctx, r.client, coreRoute); err != nil {
		updateRouteCondition(coreRoute, metav1.Condition{
			Type:               string(gateway_api_v1beta1.RouteConditionAccepted),
			Status:             metav1.ConditionFalse,
			ObservedGeneration: httpRoute.Generation,
			Reason:             string(gateway_api_v1beta1.RouteReasonNoMatchingParent),
			Message:            fmt.Sprintf("Could not match gateway %s: %s", httpRoute.Spec.ParentRefs[0].Name, err.Error()),
		})
	} else {
		updateRouteCondition(coreRoute, metav1.Condition{
			Type:               string(gateway_api_v1beta1.RouteConditionAccepted),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: httpRoute.Generation,
			Reason:             string(gateway_api_v1beta1.RouteReasonAccepted),
			Message:            fmt.Sprintf("DNS Name: %s", dns),
		})
		updateRouteCondition(coreRoute, metav1.Condition{
			Type:               string(gateway_api_v1beta1.RouteConditionResolvedRefs),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: httpRoute.Generation,
			Reason:             string(gateway_api_v1beta1.RouteReasonResolvedRefs),
			Message:            fmt.Sprintf("DNS Name: %s", dns),
		})
	}

	if err := r.client.Status().Patch(ctx, httpRoute, client.MergeFrom(httprouteOld)); err != nil {
		glog.V(2).Infof("updateHTTPRouteStatus: Patch() received err %v \n", err)
		return errors.Wrapf(err, "failed to update httpRoute status")
	}
	glog.V(6).Infof("updateHTTPRouteStatus patched dns %v \n", dns)

	return nil
}

func (r *RouteReconciler) updateGRPCRouteStatus(ctx context.Context, dns string, coreRoute *core.GRPCRoute) error {
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
	if err := updateRouteListenerStatus(ctx, r.client, coreRoute); err != nil {
		updateRouteCondition(coreRoute, metav1.Condition{
			Type:               string(gateway_api_v1beta1.RouteConditionAccepted),
			Status:             metav1.ConditionFalse,
			ObservedGeneration: grpcRoute.Generation,
			Reason:             string(gateway_api_v1beta1.RouteReasonNoMatchingParent),
			Message:            fmt.Sprintf("Could not match gateway %s: %s", grpcRoute.Spec.ParentRefs[0].Name, err.Error()),
		})
	} else {
		updateRouteCondition(coreRoute, metav1.Condition{
			Type:               string(gateway_api_v1beta1.RouteConditionAccepted),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: grpcRoute.Generation,
			Reason:             string(gateway_api_v1beta1.RouteReasonAccepted),
			Message:            fmt.Sprintf("DNS Name: %s", dns),
		})
		updateRouteCondition(coreRoute, metav1.Condition{
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

func updateRouteCondition(route core.Route, updated metav1.Condition) {
	route.Status().Parents()[0].Conditions = updateCondition(route.Status().Parents()[0].Conditions, updated)
}
