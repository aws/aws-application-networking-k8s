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

	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/utils"
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
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"sigs.k8s.io/external-dns/endpoint"

	"github.com/aws/aws-application-networking-k8s/controllers/eventhandlers"
	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/deploy"
	"github.com/aws/aws-application-networking-k8s/pkg/deploy/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/gateway"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	lattice_runtime "github.com/aws/aws-application-networking-k8s/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

var routeTypeToFinalizer = map[core.RouteType]string{
	core.HttpRouteType: "httproute.k8s.aws/resources",
	core.GrpcRouteType: "grpcroute.k8s.aws/resources",
}

type routeReconciler struct {
	routeType        core.RouteType
	log              gwlog.Logger
	client           client.Client
	scheme           *runtime.Scheme
	finalizerManager k8s.FinalizerManager
	eventRecorder    record.EventRecorder
	modelBuilder     gateway.LatticeServiceBuilder
	stackDeployer    deploy.StackDeployer
	stackMarshaller  deploy.StackMarshaller
	cloud            aws.Cloud
}

const (
	LatticeAssignedDomainName = "application-networking.k8s.aws/lattice-assigned-domain-name"
)

func RegisterAllRouteControllers(
	log gwlog.Logger,
	cloud aws.Cloud,
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
		{core.HttpRouteType, &gwv1beta1.HTTPRoute{}},
		{core.GrpcRouteType, &gwv1alpha2.GRPCRoute{}},
	}

	for _, routeInfo := range routeInfos {
		brTgBuilder := gateway.NewBackendRefTargetGroupBuilder(log, mgrClient)
		reconciler := routeReconciler{
			routeType:        routeInfo.routeType,
			log:              log,
			client:           mgrClient,
			scheme:           mgr.GetScheme(),
			finalizerManager: finalizerManager,
			eventRecorder:    mgr.GetEventRecorderFor(string(routeInfo.routeType) + "route"),
			modelBuilder:     gateway.NewLatticeServiceBuilder(log, mgrClient, brTgBuilder),
			stackDeployer:    deploy.NewLatticeServiceStackDeploy(log, cloud, mgrClient),
			stackMarshaller:  deploy.NewDefaultStackMarshaller(),
			cloud:            cloud,
		}

		svcImportEventHandler := eventhandlers.NewServiceImportEventHandler(log, mgrClient)

		builder := ctrl.NewControllerManagedBy(mgr).
			For(routeInfo.gatewayApiType, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
			Watches(&source.Kind{Type: &gwv1beta1.Gateway{}}, gwEventHandler).
			Watches(&source.Kind{Type: &corev1.Service{}}, svcEventHandler.MapToRoute(routeInfo.routeType)).
			Watches(&source.Kind{Type: &anv1alpha1.ServiceImport{}}, svcImportEventHandler.MapToRoute(routeInfo.routeType)).
			Watches(&source.Kind{Type: &corev1.Endpoints{}}, svcEventHandler.MapToRoute(routeInfo.routeType))

		if ok, err := k8s.IsGVKSupported(mgr, anv1alpha1.GroupVersion.String(), anv1alpha1.TargetGroupPolicyKind); ok {
			builder.Watches(&source.Kind{Type: &anv1alpha1.TargetGroupPolicy{}}, svcEventHandler.MapToRoute(routeInfo.routeType))
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

func (r *routeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.log.Infow("reconcile", "name", req.Name)
	recErr := r.reconcile(ctx, req)
	if recErr != nil {
		r.log.Infow("reconcile error", "name", req.Name, "message", recErr.Error())
	}
	return lattice_runtime.HandleReconcileError(recErr)
}

func (r *routeReconciler) reconcile(ctx context.Context, req ctrl.Request) error {
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
		return r.reconcileDelete(ctx, req, route)
	} else {
		return r.reconcileUpsert(ctx, req, route)
	}
}

func (r *routeReconciler) reconcileDelete(ctx context.Context, req ctrl.Request, route core.Route) error {
	r.log.Infow("reconcile, deleting", "name", req.Name)
	r.eventRecorder.Event(route.K8sObject(), corev1.EventTypeNormal,
		k8s.RouteEventReasonReconcile, "Deleting Reconcile")

	if _, err := r.buildAndDeployModel(ctx, route); err != nil {
		return fmt.Errorf("failed to cleanup route %s, %s: %w", route.Name(), route.Namespace(), err)
	}

	if err := updateRouteListenerStatus(ctx, r.client, route); err != nil {
		return err
	}

	r.log.Infow("reconciled", "name", req.Name)
	return r.finalizerManager.RemoveFinalizers(ctx, route.K8sObject(), routeTypeToFinalizer[r.routeType])
}

func (r *routeReconciler) getRoute(ctx context.Context, req ctrl.Request) (core.Route, error) {
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
	gw := &gwv1beta1.Gateway{}

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

func (r *routeReconciler) isRouteRelevant(ctx context.Context, route core.Route) bool {
	if len(route.Spec().ParentRefs()) == 0 {
		r.log.Infof("Ignore Route which has no ParentRefs gateway %s ", route.Name())
		return false
	}

	gw := &gwv1beta1.Gateway{}

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
	gwClass := &gwv1beta1.GatewayClass{}
	gwClassName := types.NamespacedName{
		Namespace: defaultNamespace,
		Name:      string(gw.Spec.GatewayClassName),
	}

	if err := r.client.Get(ctx, gwClassName, gwClass); err != nil {
		r.log.Infof("Ignore Route not controlled by any GatewayClass %s, %s", route.Name(), route.Namespace())
		return false
	}

	if gwClass.Spec.ControllerName == config.LatticeGatewayControllerName {
		r.log.Infof("Found aws-vpc-lattice for Route for %s, %s", route.Name(), route.Namespace())
		return true
	}

	r.log.Infof("Ignore non aws-vpc-lattice Route %s, %s", route.Name(), route.Namespace())
	return false
}

func (r *routeReconciler) buildAndDeployModel(
	ctx context.Context,
	route core.Route,
) (core.Stack, error) {
	stack, err := r.modelBuilder.Build(ctx, route)

	if err != nil {
		r.eventRecorder.Event(route.K8sObject(), corev1.EventTypeWarning,
			k8s.RouteEventReasonFailedBuildModel, fmt.Sprintf("Failed build model due to %s", err))
		r.log.Infof("buildAndDeployModel, Failed build model for %s due to %s", route.Name(), err)

		// Build failed
		// TODO continue deploy to trigger reconcile of stale Route and policy
		return nil, err
	}

	json, err := r.stackMarshaller.Marshal(stack)
	if err != nil {
		r.log.Errorf("error on r.stackMarshaller.Marshal error %s", err)
	}

	r.log.Debugf("stack: %s", json)

	if err := r.stackDeployer.Deploy(ctx, stack); err != nil {
		if errors.As(err, &lattice.RetryErr) {
			r.eventRecorder.Event(route.K8sObject(), corev1.EventTypeNormal,
				k8s.RouteEventReasonRetryReconcile, "retry reconcile...")
		} else {
			r.eventRecorder.Event(route.K8sObject(), corev1.EventTypeWarning,
				k8s.RouteEventReasonFailedDeployModel, fmt.Sprintf("Failed deploy model due to %s", err))
		}
		return nil, err
	}

	return stack, err
}

func (r *routeReconciler) reconcileUpsert(ctx context.Context, req ctrl.Request, route core.Route) error {
	r.log.Infow("reconcile, adding or updating", "name", req.Name)
	r.eventRecorder.Event(route.K8sObject(), corev1.EventTypeNormal,
		k8s.RouteEventReasonReconcile, "Adding/Updating Reconcile")

	if err := r.finalizerManager.AddFinalizers(ctx, route.K8sObject(), routeTypeToFinalizer[r.routeType]); err != nil {
		r.eventRecorder.Event(route.K8sObject(), corev1.EventTypeWarning, k8s.RouteEventReasonFailedAddFinalizer, fmt.Sprintf("Failed add finalizer due to %s", err))
	}

	backendRefIPFamiliesErr := r.validateBackendRefsIpFamilies(ctx, route)

	if backendRefIPFamiliesErr != nil {
		httpRouteOld := route.DeepCopy()

		route.Status().UpdateParentRefs(route.Spec().ParentRefs()[0], config.LatticeGatewayControllerName)

		route.Status().UpdateRouteCondition(metav1.Condition{
			Type:               string(gwv1beta1.RouteConditionAccepted),
			Status:             metav1.ConditionFalse,
			ObservedGeneration: route.K8sObject().GetGeneration(),
			Reason:             string(gwv1beta1.RouteReasonUnsupportedValue),
			Message:            fmt.Sprintf("Dual stack Service is not supported"),
		})

		if err := r.client.Status().Patch(ctx, route.K8sObject(), client.MergeFrom(httpRouteOld.K8sObject())); err != nil {
			return errors.Wrapf(err, "failed to update httproute status")
		}

		return backendRefIPFamiliesErr
	}

	if _, err := r.buildAndDeployModel(ctx, route); err != nil {
		if services.IsConflictError(err) {
			// Stop reconciliation of this route if the route cannot be owned / has conflict
			route.Status().UpdateParentRefs(route.Spec().ParentRefs()[0], config.LatticeGatewayControllerName)
			route.Status().UpdateRouteCondition(metav1.Condition{
				Type:               string(gwv1beta1.RouteConditionAccepted),
				Status:             metav1.ConditionFalse,
				ObservedGeneration: route.K8sObject().GetGeneration(),
				Reason:             "Conflicted",
				Message:            err.Error(),
			})
			if err = r.client.Status().Update(ctx, route.K8sObject()); err != nil {
				return fmt.Errorf("failed to update route status for conflict due to err %w", err)
			}
			return nil
		}
		return err
	}

	r.eventRecorder.Event(route.K8sObject(), corev1.EventTypeNormal,
		k8s.RouteEventReasonDeploySucceed, "Adding/Updating reconcile Done!")

	svcName := utils.LatticeServiceName(route.Name(), route.Namespace())
	svc, err := r.cloud.Lattice().FindService(ctx, svcName)
	if err != nil && !services.IsNotFoundError(err) {
		return err
	}

	if svc == nil || svc.DnsEntry == nil || svc.DnsEntry.DomainName == nil {
		r.log.Infof("Either service, dns entry, or domain name is not available. Will Retry")
		return errors.New(lattice.LATTICE_RETRY)
	}

	if err := r.updateRouteStatus(ctx, *svc.DnsEntry.DomainName, route); err != nil {
		return err
	}

	r.log.Infow("reconciled", "name", req.Name)
	return nil
}

func (r *routeReconciler) updateRouteStatus(ctx context.Context, dns string, route core.Route) error {
	r.log.Debugf("Updating route %s-%s with DNS %s", route.Name(), route.Namespace(), dns)
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
			Type:               string(gwv1beta1.RouteConditionAccepted),
			Status:             metav1.ConditionFalse,
			ObservedGeneration: route.K8sObject().GetGeneration(),
			Reason:             string(gwv1beta1.RouteReasonNoMatchingParent),
			Message:            fmt.Sprintf("Could not match gateway %s: %s", route.Spec().ParentRefs()[0].Name, err),
		})
	} else {
		route.Status().UpdateRouteCondition(metav1.Condition{
			Type:               string(gwv1beta1.RouteConditionAccepted),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: route.K8sObject().GetGeneration(),
			Reason:             string(gwv1beta1.RouteReasonAccepted),
			Message:            fmt.Sprintf("DNS Name: %s", dns),
		})
		route.Status().UpdateRouteCondition(metav1.Condition{
			Type:               string(gwv1beta1.RouteConditionResolvedRefs),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: route.K8sObject().GetGeneration(),
			Reason:             string(gwv1beta1.RouteReasonResolvedRefs),
			Message:            fmt.Sprintf("DNS Name: %s", dns),
		})
	}

	if err := r.client.Status().Patch(ctx, route.K8sObject(), client.MergeFrom(routeOld.K8sObject())); err != nil {
		return fmt.Errorf("failed to update route status due to err %w", err)
	}

	r.log.Debugf("Successfully updated route %s-%s with DNS %s", route.Name(), route.Namespace(), dns)
	return nil
}

func (r *routeReconciler) validateBackendRefsIpFamilies(ctx context.Context, route core.Route) error {
	rules := route.Spec().Rules()

	for _, rule := range rules {
		backendRefs := rule.BackendRefs()

		for _, backendRef := range backendRefs {
			// For now we skip checking service import
			if *backendRef.Kind() == "ServiceImport" {
				continue
			}

			svc, err := gateway.GetServiceForBackendRef(ctx, r.client, route, backendRef)
			if err != nil {
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
