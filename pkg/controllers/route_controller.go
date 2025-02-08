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

	"sigs.k8s.io/controller-runtime/pkg/controller"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/external-dns/endpoint"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	discoveryv1 "k8s.io/api/discovery/v1"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/controllers/eventhandlers"
	"github.com/aws/aws-application-networking-k8s/pkg/deploy"
	"github.com/aws/aws-application-networking-k8s/pkg/deploy/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/gateway"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	lattice_runtime "github.com/aws/aws-application-networking-k8s/pkg/runtime"
	k8sutils "github.com/aws/aws-application-networking-k8s/pkg/utils"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

var routeTypeToFinalizer = map[core.RouteType]string{
	core.HttpRouteType: "httproute.k8s.aws/resources",
	core.GrpcRouteType: "grpcroute.k8s.aws/resources",
	core.TlsRouteType:  "tlsroute.k8s.aws/resources",
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
		{core.HttpRouteType, &gwv1.HTTPRoute{
			TypeMeta: metav1.TypeMeta{
				APIVersion: gwv1.GroupVersion.String(),
				Kind:       "HTTPRoute",
			},
		}},
		{core.GrpcRouteType, &gwv1.GRPCRoute{
			TypeMeta: metav1.TypeMeta{
				APIVersion: gwv1.GroupVersion.String(),
				Kind:       "GRPCRoute",
			},
		}},
		{core.TlsRouteType, &gwv1alpha2.TLSRoute{
			TypeMeta: metav1.TypeMeta{
				APIVersion: gwv1.GroupVersion.String(),
				Kind:       "TLSRoute",
			},
		}},
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
			Watches(&gwv1.Gateway{}, gwEventHandler).
			Watches(&corev1.Service{}, svcEventHandler.MapToRoute(routeInfo.routeType)).
			Watches(&anv1alpha1.ServiceImport{}, svcImportEventHandler.MapToRoute(routeInfo.routeType)).
			Watches(&discoveryv1.EndpointSlice{}, svcEventHandler.MapToRoute(routeInfo.routeType)).
			WithOptions(controller.Options{
				MaxConcurrentReconciles: config.RouteMaxConcurrentReconciles,
			})

		if ok, err := k8s.IsGVKSupported(mgr, anv1alpha1.GroupVersion.String(), anv1alpha1.TargetGroupPolicyKind); ok {
			builder.Watches(&anv1alpha1.TargetGroupPolicy{}, svcEventHandler.MapToRoute(routeInfo.routeType))
		} else {
			if err != nil {
				return err
			}
			log.Infof(context.TODO(), "TargetGroupPolicy CRD is not installed, skipping watch")
		}

		if ok, err := k8s.IsGVKSupported(mgr, "externaldns.k8s.io/v1alpha1", "DNSEndpoint"); ok {
			builder.Owns(&endpoint.DNSEndpoint{})
		} else {
			if err != nil {
				return err
			}
			log.Infof(context.TODO(), "DNSEndpoint CRD is not installed, skipping watch")
		}

		err := builder.Complete(&reconciler)
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *routeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctx = gwlog.StartReconcileTrace(ctx, r.log, "route", req.Name, req.Namespace)
	defer func() {
		gwlog.EndReconcileTrace(ctx, r.log)
	}()

	recErr := r.reconcile(ctx, req)
	if recErr != nil {
		r.log.Infow(ctx, "reconcile error", "name", req.Name, "message", recErr.Error())
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
	r.log.Infow(ctx, "reconcile, deleting", "name", req.Name)
	r.eventRecorder.Event(route.K8sObject(), corev1.EventTypeNormal,
		k8s.RouteEventReasonReconcile, "Deleting Reconcile")

	if _, err := r.buildAndDeployModel(ctx, route); err != nil {
		return fmt.Errorf("failed to cleanup route %s, %s: %w", route.Name(), route.Namespace(), err)
	}

	if err := updateRouteListenerStatus(ctx, r.client, route); err != nil {
		return err
	}

	r.log.Infow(ctx, "reconciled", "name", req.Name)
	return r.finalizerManager.RemoveFinalizers(ctx, route.K8sObject(), routeTypeToFinalizer[r.routeType])
}

func (r *routeReconciler) getRoute(ctx context.Context, req ctrl.Request) (core.Route, error) {
	switch r.routeType {
	case core.HttpRouteType:
		return core.GetHTTPRoute(ctx, r.client, req.NamespacedName)
	case core.GrpcRouteType:
		return core.GetGRPCRoute(ctx, r.client, req.NamespacedName)
	case core.TlsRouteType:
		return core.GetTLSRoute(ctx, r.client, req.NamespacedName)
	default:
		return nil, fmt.Errorf("unknown route type for type %s", string(r.routeType))
	}
}

func updateRouteListenerStatus(ctx context.Context, k8sClient client.Client, route core.Route) error {
	gw := &gwv1.Gateway{}

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
		r.log.Infof(ctx, "Ignore Route which has no ParentRefs gateway %s ", route.Name())
		return false
	}

	gw := &gwv1.Gateway{}

	gwNamespace := route.Namespace()
	if route.Spec().ParentRefs()[0].Namespace != nil {
		gwNamespace = string(*route.Spec().ParentRefs()[0].Namespace)
	}
	gwName := types.NamespacedName{
		Namespace: gwNamespace,
		Name:      string(route.Spec().ParentRefs()[0].Name),
	}

	if err := r.client.Get(ctx, gwName, gw); err != nil {
		r.log.Infof(ctx, "Could not find gateway %s with err %s. Ignoring route %+v whose ParentRef gateway object"+
			" is not defined.", gwName.String(), err, route.Spec())
		return false
	}

	// make sure gateway is an aws-vpc-lattice
	gwClass := &gwv1.GatewayClass{}
	gwClassName := types.NamespacedName{
		Name: string(gw.Spec.GatewayClassName),
	}

	if err := r.client.Get(ctx, gwClassName, gwClass); err != nil {
		r.log.Infof(ctx, "Ignore Route not controlled by any GatewayClass %s, %s", route.Name(), route.Namespace())
		return false
	}

	if gwClass.Spec.ControllerName == config.LatticeGatewayControllerName {
		r.log.Infof(ctx, "Found aws-vpc-lattice for Route for %s, %s", route.Name(), route.Namespace())
		return true
	}

	r.log.Infof(ctx, "Ignore non aws-vpc-lattice Route %s, %s", route.Name(), route.Namespace())
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
		r.log.Infof(ctx, "buildAndDeployModel, Failed build model for %s due to %s", route.Name(), err)

		// Build failed
		// TODO continue deploy to trigger reconcile of stale Route and policy
		return nil, err
	}

	json, err := r.stackMarshaller.Marshal(stack)
	if err != nil {
		r.log.Errorf(ctx, "error on r.stackMarshaller.Marshal error %s", err)
	}

	r.log.Debugf(ctx, "stack: %s", json)

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
	r.log.Infow(ctx, "reconcile, adding or updating", "name", req.Name)
	r.eventRecorder.Event(route.K8sObject(), corev1.EventTypeNormal,
		k8s.RouteEventReasonReconcile, "Adding/Updating Reconcile")

	if err := r.finalizerManager.AddFinalizers(ctx, route.K8sObject(), routeTypeToFinalizer[r.routeType]); err != nil {
		r.eventRecorder.Event(route.K8sObject(), corev1.EventTypeWarning, k8s.RouteEventReasonFailedAddFinalizer, fmt.Sprintf("Failed add finalizer due to %s", err))
	}

	if err := r.validateRoute(ctx, route); err != nil {
		// TODO: we suppose to stop reconciliation here, but that will create problem when
		// we delete Service and we suppose to delete TargetGroup, this validation will
		// throw error if Service is not found.  For now just update route status and log
		// error.
		r.log.Infof(ctx, "route: %s: %s", route.Name(), err)
	}

	backendRefIPFamiliesErr := r.validateBackendRefsIpFamilies(ctx, route)

	if backendRefIPFamiliesErr != nil {
		httpRouteOld := route.DeepCopy()

		route.Status().UpdateParentRefs(route.Spec().ParentRefs()[0], config.LatticeGatewayControllerName)

		route.Status().UpdateRouteCondition(metav1.Condition{
			Type:               string(gwv1.RouteConditionAccepted),
			Status:             metav1.ConditionFalse,
			ObservedGeneration: route.K8sObject().GetGeneration(),
			Reason:             string(gwv1.RouteReasonUnsupportedValue),
			Message:            "Dual stack Service is not supported",
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
				Type:               string(gwv1.RouteConditionAccepted),
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

	svcName := k8sutils.LatticeServiceName(route.Name(), route.Namespace())
	svc, err := r.cloud.Lattice().FindService(ctx, svcName)
	if err != nil && !services.IsNotFoundError(err) {
		return err
	}

	if svc == nil || svc.DnsEntry == nil || svc.DnsEntry.DomainName == nil {
		r.log.Infof(ctx, "Either service, dns entry, or domain name is not available. Will Retry")
		return errors.New(lattice.LATTICE_RETRY)
	}

	if err := r.updateRouteAnnotation(ctx, *svc.DnsEntry.DomainName, route); err != nil {
		return err
	}

	r.log.Infow(ctx, "reconciled", "name", req.Name)
	return nil
}

func (r *routeReconciler) updateRouteAnnotation(ctx context.Context, dns string, route core.Route) error {
	r.log.Debugf(ctx, "Updating route %s-%s with DNS %s", route.Name(), route.Namespace(), dns)
	routeOld := route.DeepCopy()

	if len(route.K8sObject().GetAnnotations()) == 0 {
		route.K8sObject().SetAnnotations(make(map[string]string))
	}

	route.K8sObject().GetAnnotations()[LatticeAssignedDomainName] = dns
	if err := r.client.Patch(ctx, route.K8sObject(), client.MergeFrom(routeOld.K8sObject())); err != nil {
		return fmt.Errorf("failed to update route status due to err %w", err)
	}

	r.log.Debugf(ctx, "Successfully updated route %s-%s with DNS %s", route.Name(), route.Namespace(), dns)
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

var (
	ErrValidation          = errors.New("validation")
	ErrParentRefsNotFound  = errors.New("parentRefs are not found")
	ErrRouteGKNotSupported = errors.New("route GroupKind is not supported")
)

// Validation for route spec. Will validate and update route status. Returns error if not valid.
// Validation rules are suppose to be compliant to Gateway API Spec.
//
//	https://gateway-api.sigs.k8s.io/reference/spec/#gateway.networking.k8s.io%2fv1.RouteConditionType
//
// There are 3 condition types: Accepted, PartiallyInvalid, ResolvedRefs.
// We dont support PartiallyInvalid for now and reject entire route if there is at least one invalid field.
// Accepted type is related to parentRefs, and ResolvedRefs to backendRefs. These 2 are validated independently.
func (r *routeReconciler) validateRoute(ctx context.Context, route core.Route) error {
	parentRefsAccepted, err := r.validateRouteParentRefs(ctx, route)
	if err != nil {
		return err
	}

	resolvedRefsCnd, err := r.validateBackedRefs(ctx, route)
	if err != nil {
		return err
	}

	// we need to update each parentRef with backendRef status
	parentRefsAcceptedResolvedRefs := make([]gwv1.RouteParentStatus, len(parentRefsAccepted))
	for i, rps := range parentRefsAccepted {
		meta.SetStatusCondition(&rps.Conditions, resolvedRefsCnd)
		parentRefsAcceptedResolvedRefs[i] = rps
	}

	route.Status().SetParents(parentRefsAcceptedResolvedRefs)

	err = r.client.Status().Update(ctx, route.K8sObject())
	if err != nil {
		return fmt.Errorf("validate route: %w", err)
	}

	if r.hasNotAcceptedCondition(route) {
		return fmt.Errorf("%w: route has validation errors, see status", ErrValidation)
	}

	return nil
}

// checks if route has at least single condition with status = false
func (r *routeReconciler) hasNotAcceptedCondition(route core.Route) bool {
	rps := route.Status().Parents()
	for _, ps := range rps {
		for _, cnd := range ps.Conditions {
			if cnd.Status != metav1.ConditionTrue {
				return true
			}
		}
	}
	return false
}

// find Gateway by Route and parentRef, returns nil if not found
func (r *routeReconciler) findRouteParentGw(ctx context.Context, route core.Route, parentRef gwv1.ParentReference) (*gwv1.Gateway, error) {
	ns := route.Namespace()
	if parentRef.Namespace != nil && *parentRef.Namespace != "" {
		ns = string(*parentRef.Namespace)
	}
	gwName := types.NamespacedName{
		Namespace: ns,
		Name:      string(parentRef.Name),
	}
	gw := &gwv1.Gateway{}
	err := r.client.Get(ctx, gwName, gw)
	if err != nil {
		return nil, client.IgnoreNotFound(err)
	}
	return gw, nil
}

// Validation rules for route parentRefs
//
// Will ignore status update when:
// - parentRef does not exists, includes when parentRef Kind is not Gateway
//
// If parent GW exists will check:
// - NoMatchingParent: parentRef sectionName and port matches Listener name and port
// - TODO: NoMatchingListenerHostname: listener hostname matches one of route hostnames
// - TODO: NotAllowedByListeners: listener allowedRoutes contains route GroupKind
func (r *routeReconciler) validateRouteParentRefs(ctx context.Context, route core.Route) ([]gwv1.RouteParentStatus, error) {
	if len(route.Spec().ParentRefs()) == 0 {
		return nil, ErrParentRefsNotFound
	}

	parentStatuses := []gwv1.RouteParentStatus{}
	for _, parentRef := range route.Spec().ParentRefs() {
		gw, err := r.findRouteParentGw(ctx, route, parentRef)
		if err != nil {
			return nil, err
		}
		if gw == nil {
			continue // ignore status update if gw not found
		}

		noMatchingParent := true
		for _, listener := range gw.Spec.Listeners {
			if parentRef.Port != nil && *parentRef.Port != listener.Port {
				continue
			}
			if parentRef.SectionName != nil && *parentRef.SectionName != listener.Name {
				continue
			}
			noMatchingParent = false
		}

		parentStatus := gwv1.RouteParentStatus{
			ParentRef:      parentRef,
			ControllerName: "application-networking.k8s.aws/gateway-api-controller",
			Conditions:     []metav1.Condition{},
		}

		var cnd metav1.Condition
		switch {
		case noMatchingParent:
			cnd = r.newCondition(route, gwv1.RouteConditionAccepted, gwv1.RouteReasonNoMatchingParent, "")
		default:
			cnd = r.newCondition(route, gwv1.RouteConditionAccepted, gwv1.RouteReasonAccepted, "")
		}
		meta.SetStatusCondition(&parentStatus.Conditions, cnd)
		parentStatuses = append(parentStatuses, parentStatus)
	}

	return parentStatuses, nil
}

// set of valid Kinds for Route Backend References
var validBackendKinds = k8sutils.NewSet("Service", "ServiceImport")

// validate route's backed references, will return non-accepted
// condition if at least one backendRef not in a valid state
func (r *routeReconciler) validateBackedRefs(ctx context.Context, route core.Route) (metav1.Condition, error) {
	var empty metav1.Condition
	for _, rule := range route.Spec().Rules() {
		for _, ref := range rule.BackendRefs() {
			kind := "Service"
			if ref.Kind() != nil {
				kind = string(*ref.Kind())
			}
			if !validBackendKinds.Contains(kind) {
				return r.newCondition(route, gwv1.RouteConditionResolvedRefs, gwv1.RouteReasonInvalidKind, kind), nil
			}

			namespace := route.Namespace()
			if ref.Namespace() != nil {
				namespace = string(*ref.Namespace())
			}
			objKey := types.NamespacedName{
				Namespace: namespace,
				Name:      string(ref.Name()),
			}
			var obj client.Object

			switch kind {
			case "Service":
				obj = &corev1.Service{}
			case "ServiceImport":
				obj = &anv1alpha1.ServiceImport{}
			default:
				return empty, fmt.Errorf("invalid backed end ref kind, must be validated before, kind=%s", kind)
			}
			err := r.client.Get(ctx, objKey, obj)
			if err != nil {
				if apierrors.IsNotFound(err) {
					msg := fmt.Sprintf("backendRef name: %s", ref.Name())
					return r.newCondition(route, gwv1.RouteConditionResolvedRefs, gwv1.RouteReasonBackendNotFound, msg), nil
				}
			}
		}
	}
	return r.newCondition(route, gwv1.RouteConditionResolvedRefs, gwv1.RouteReasonResolvedRefs, ""), nil
}

func (r *routeReconciler) newCondition(route core.Route, t gwv1.RouteConditionType, reason gwv1.RouteConditionReason, msg string) metav1.Condition {
	status := metav1.ConditionTrue
	if reason != gwv1.RouteReasonAccepted && reason != gwv1.RouteReasonResolvedRefs {
		status = metav1.ConditionFalse
	}
	return metav1.Condition{
		Type:               string(t),
		Status:             status,
		ObservedGeneration: route.K8sObject().GetGeneration(),
		Reason:             string(reason),
		Message:            msg,
	}
}
