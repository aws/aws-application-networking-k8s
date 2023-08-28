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

	"github.com/pkg/errors"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/source"
	gateway_api_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gateway_api "sigs.k8s.io/gateway-api/apis/v1beta1"
	gateway_api_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	mcs_api "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"sigs.k8s.io/external-dns/endpoint"

	"github.com/aws/aws-application-networking-k8s/controllers/eventhandlers"
	"github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
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

var routeTypeToFinalizer = map[core.RouteType]string{
	core.HttpRouteType: "httproute.k8s.aws/resources",
	core.GrpcRouteType: "grpcroute.k8s.aws/resources",
}

// RouteReconciler reconciles a HTTPRoute and GRPCRoute objects
type RouteReconciler struct {
	routeType        core.RouteType
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
	gwEventHandler := eventhandlers.NewEnqueueRequestGatewayEvent(log, mgrClient)
	svcEventHandler := eventhandlers.NewServiceEventHandler(log, mgrClient)

	routeInfos := []struct {
		routeType      core.RouteType
		gatewayApiType client.Object
	}{
		{core.HttpRouteType, &gateway_api_v1beta1.HTTPRoute{}},
		{core.GrpcRouteType, &gateway_api_v1alpha2.GRPCRoute{}},
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
			modelBuilder:     gateway.NewLatticeServiceBuilder(log, mgrClient, datastore, cloud),
			stackDeployer:    deploy.NewLatticeServiceStackDeploy(log, cloud, mgrClient, datastore),
			stackMarshaller:  deploy.NewDefaultStackMarshaller(),
		}

		svcImportEventHandler := eventhandlers.NewServiceImportEventHandler(log, mgrClient)

		builder := ctrl.NewControllerManagedBy(mgr).
			For(routeInfo.gatewayApiType).
			Watches(&source.Kind{Type: &gateway_api_v1beta1.Gateway{}}, gwEventHandler).
			Watches(&source.Kind{Type: &corev1.Service{}}, svcEventHandler.MapToRoute(routeInfo.routeType)).
			Watches(&source.Kind{Type: &mcs_api.ServiceImport{}}, svcImportEventHandler.MapToRoute(routeInfo.routeType)).
			Watches(&source.Kind{Type: &corev1.Endpoints{}}, svcEventHandler.MapToRoute(routeInfo.routeType))

		if ok, err := k8s.IsGVKSupported(mgr, v1alpha1.GroupVersion.String(), v1alpha1.TargetGroupPolicyKind); ok {
			builder.Watches(&source.Kind{Type: &v1alpha1.TargetGroupPolicy{}}, svcEventHandler.MapToRoute(routeInfo.routeType))
		} else {
			if err != nil {
				return err
			}
			log.Infof("TargetGroupPolicy CRD is not installed, skipping watch")
		}

		if ok, err := k8s.IsGVKSupported(mgr, "externaldns.k8s.io/v1alpha1", "DNSEndpoint"); ok {
			builder.Owns(&endpoint.DNSEndpoint{})
		} else {
			if err != nil {
				return err
			}
			log.Infof("DNSEndpoint CRD is not installed, skipping watch")
		}

		err := builder.Complete(&reconciler)
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

	if err = r.client.Get(ctx, req.NamespacedName, route.K8sObject()); err != nil {
		return client.IgnoreNotFound(err)
	}

	if !r.isRouteRelevant(ctx, route) {
		return nil
	}

	if !route.DeletionTimestamp().IsZero() {
		r.log.Infow("reconcile, deleting", "name", req.Name)
		r.eventRecorder.Event(route.K8sObject(), corev1.EventTypeNormal,
			k8s.RouteEventReasonReconcile, "Deleting Reconcile")
		if err := r.cleanupRouteResources(ctx, route); err != nil {
			return fmt.Errorf("failed to cleanup GRPCRoute %v, %v: %w", route.Name(), route.Namespace(), err)
		}
		err = updateRouteListenerStatus(ctx, r.client, route)
		if err != nil {
			return err
		}
		err = r.finalizerManager.RemoveFinalizers(ctx, route.K8sObject(), routeTypeToFinalizer[r.routeType])
		if err != nil {
			return err
		}

		// TODO delete metrics
		return nil
	} else {
		r.log.Infow("reconcile, adding or updating", "name", req.Name)
		r.eventRecorder.Event(route.K8sObject(), corev1.EventTypeNormal,
			k8s.RouteEventReasonReconcile, "Adding/Updating Reconcile")
		err := r.reconcileRouteResource(ctx, route)
		// TODO add/update metrics
		return err
	}
}

func (r *RouteReconciler) getRoute(ctx context.Context, req ctrl.Request) (core.Route, error) {
	switch r.routeType {
	case core.HttpRouteType:
		return core.GetHTTPRoute(ctx, r.client, req.NamespacedName)
	case core.GrpcRouteType:
		return core.GetGRPCRoute(ctx, r.client, req.NamespacedName)
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
		return fmt.Errorf("update route listener: gw not found, gw: %s, err: %w", gwName, err)
	}

	return UpdateGWListenerStatus(ctx, k8sClient, gw)
}

func (r *RouteReconciler) cleanupRouteResources(ctx context.Context, route core.Route) error {
	_, _, err := r.buildAndDeployModel(ctx, route)
	return err
}

func (r *RouteReconciler) isRouteRelevant(ctx context.Context, route core.Route) bool {
	if len(route.Spec().ParentRefs()) == 0 {
		r.log.Infof("Ignore Route which has no ParentRefs gateway %v ", route.Name())
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
		r.log.Infof("Could not find gateway %s with err %s. Ignoring route %+v whose ParentRef gateway object"+
			" is not defined.", gwName.String(), err, route.Spec())
		return false
	}

	// make sure gateway is an aws-vpc-lattice
	gwClass := &gateway_api_v1beta1.GatewayClass{}
	gwClassName := types.NamespacedName{
		Namespace: defaultNameSpace,
		Name:      string(gw.Spec.GatewayClassName),
	}

	if err := r.client.Get(ctx, gwClassName, gwClass); err != nil {
		r.log.Infof("Ignore Route not controlled by any GatewayClass %v, %v", route.Name(), route.Namespace())
		return false
	}

	if gwClass.Spec.ControllerName == config.LatticeGatewayControllerName {
		r.log.Infof("Found aws-vpc-lattice for Route for %v, %v", route.Name(), route.Namespace())
		return true
	}

	r.log.Infof("Ignore non aws-vpc-lattice Route %v, %v", route.Name(), route.Namespace())
	return false
}

func (r *RouteReconciler) buildAndDeployModel(
	ctx context.Context,
	route core.Route,
) (core.Stack, *latticemodel.Service, error) {
	stack, latticeService, err := r.modelBuilder.Build(ctx, route)

	if err != nil {
		r.eventRecorder.Event(route.K8sObject(), corev1.EventTypeWarning,
			k8s.RouteEventReasonFailedBuildModel, fmt.Sprintf("Failed build model due to %s", err))
		r.log.Infof("buildAndDeployModel, Failed build model for %s due to %s", route.Name(), err)

		// Build failed
		// TODO continue deploy to trigger reconcile of stale Route and policy
		return nil, nil, err
	}

	stackJSON, err := r.stackMarshaller.Marshal(stack)
	if err != nil {
		//TODO
		r.log.Infof("error on r.stackMarshaller.Marshal error %s", err)
	}

	r.log.Info("Successfully built model:", stackJSON, "")

	if err := r.stackDeployer.Deploy(ctx, stack); err != nil {
		r.log.Infof("RouteReconciler: Failed deploy %s due to err %s", route.Name(), err)

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

	r.log.Info("Successfully deployed model")

	return stack, latticeService, err
}

func (r *RouteReconciler) reconcileRouteResource(ctx context.Context, route core.Route) error {
	r.log.Infof("Beginning -- reconcileRouteResource, [%v]", route)

	if err := r.finalizerManager.AddFinalizers(ctx, route.K8sObject(), routeTypeToFinalizer[r.routeType]); err != nil {
		r.eventRecorder.Event(route.K8sObject(), corev1.EventTypeWarning, k8s.RouteEventReasonFailedAddFinalizer, fmt.Sprintf("Failed add finalizer due to %v", err))
	}

	backendRefIPFamiliesErr := r.validateBackendRefsIpFamilies(ctx, route)

	if backendRefIPFamiliesErr != nil {
		httpRouteOld := route.DeepCopy()

		route.Status().UpdateParentRefs(route.Spec().ParentRefs()[0], config.LatticeGatewayControllerName)

		route.Status().UpdateRouteCondition(metav1.Condition{
			Type:               string(gateway_api.RouteConditionAccepted),
			Status:             metav1.ConditionFalse,
			ObservedGeneration: route.K8sObject().GetGeneration(),
			Reason:             string(gateway_api.RouteReasonUnsupportedValue),
			Message:            fmt.Sprintf("Dual stack Service is not supported"),
		})

		if err := r.client.Status().Patch(ctx, route.K8sObject(), client.MergeFrom(httpRouteOld.K8sObject())); err != nil {
			return errors.Wrapf(err, "failed to update httproute status")
		}

		return backendRefIPFamiliesErr
	}

	_, _, err := r.buildAndDeployModel(ctx, route)

	//TODO add metric

	if err == nil {
		r.eventRecorder.Event(route.K8sObject(), corev1.EventTypeNormal,
			k8s.RouteEventReasonDeploySucceed, "Adding/Updating reconcile Done!")

		serviceStatus, err1 := r.latticeDataStore.GetLatticeService(route.Name(), route.Namespace())

		if err1 == nil {
			r.updateRouteStatus(ctx, serviceStatus.DNS, route)
		}
	}

	return err
}

func (r *RouteReconciler) updateRouteStatus(ctx context.Context, dns string, route core.Route) error {
	r.log.Infof("updateRouteStatus: route name %s, namespace %s, dns %s", route.Name(), route.Namespace(), dns)
	routeOld := route.DeepCopy()

	if len(route.K8sObject().GetAnnotations()) == 0 {
		route.K8sObject().SetAnnotations(make(map[string]string))
	}

	route.K8sObject().GetAnnotations()[LatticeAssignedDomainName] = dns
	if err := r.client.Patch(ctx, route.K8sObject(), client.MergeFrom(routeOld.K8sObject())); err != nil {
		return fmt.Errorf("failed to update route status due to err %w", err)
	}
	routeOld = route.DeepCopy()

	route.Status().UpdateParentRefs(route.Spec().ParentRefs()[0], config.LatticeGatewayControllerName)

	// Update listener Status
	if err := updateRouteListenerStatus(ctx, r.client, route); err != nil {
		route.Status().UpdateRouteCondition(metav1.Condition{
			Type:               string(gateway_api_v1beta1.RouteConditionAccepted),
			Status:             metav1.ConditionFalse,
			ObservedGeneration: route.K8sObject().GetGeneration(),
			Reason:             string(gateway_api_v1beta1.RouteReasonNoMatchingParent),
			Message:            fmt.Sprintf("Could not match gateway %s: %s", route.Spec().ParentRefs()[0].Name, err),
		})
	} else {
		route.Status().UpdateRouteCondition(metav1.Condition{
			Type:               string(gateway_api_v1beta1.RouteConditionAccepted),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: route.K8sObject().GetGeneration(),
			Reason:             string(gateway_api_v1beta1.RouteReasonAccepted),
			Message:            fmt.Sprintf("DNS Name: %s", dns),
		})
		route.Status().UpdateRouteCondition(metav1.Condition{
			Type:               string(gateway_api_v1beta1.RouteConditionResolvedRefs),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: route.K8sObject().GetGeneration(),
			Reason:             string(gateway_api_v1beta1.RouteReasonResolvedRefs),
			Message:            fmt.Sprintf("DNS Name: %s", dns),
		})
	}

	if err := r.client.Status().Patch(ctx, route.K8sObject(), client.MergeFrom(routeOld.K8sObject())); err != nil {
		return fmt.Errorf("failed to update route status due to err %w", err)
	}

	r.log.Infof("updateRouteStatus patched dns %v", dns)
	return nil
}

func (r *RouteReconciler) validateBackendRefsIpFamilies(ctx context.Context, route core.Route) error {
	rules := route.Spec().Rules()

	for _, rule := range rules {
		backendRefs := rule.BackendRefs()

		for _, backendRef := range backendRefs {
			// For now we skip checking service import
			if *backendRef.Kind() == "ServiceImport" {
				continue
			}

			svc := &corev1.Service{}

			key := types.NamespacedName{
				Name: string(backendRef.Name()),
			}

			if backendRef.Namespace() != nil {
				key.Namespace = string(*backendRef.Namespace())
			} else {
				key.Namespace = route.Namespace()
			}

			if err := r.client.Get(ctx, key, svc); err != nil {
				// Ignore error since Service might not be created yet
				continue
			}

			if len(svc.Spec.IPFamilies) > 1 {
				return errors.New("Invalid IpFamilies, Lattice Target Group doesn't support dual stack ip addresses")
			}
		}
	}

	return nil
}
