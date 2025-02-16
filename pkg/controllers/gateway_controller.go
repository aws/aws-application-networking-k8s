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

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/controllers/eventhandlers"

	"github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	lattice_runtime "github.com/aws/aws-application-networking-k8s/pkg/runtime"
	"github.com/aws/aws-application-networking-k8s/pkg/utils"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	deploy "github.com/aws/aws-application-networking-k8s/pkg/deploy/lattice"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	pkg_builder "sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	gatewayFinalizer = "gateway.k8s.aws/resources"
	defaultNamespace = "default"
)

type gatewayReconciler struct {
	log              gwlog.Logger
	client           client.Client
	scheme           *runtime.Scheme
	finalizerManager k8s.FinalizerManager
	eventRecorder    record.EventRecorder
	cloud            aws.Cloud
}

func RegisterGatewayController(
	log gwlog.Logger,
	cloud aws.Cloud,
	finalizerManager k8s.FinalizerManager,
	mgr ctrl.Manager,
) error {
	mgrClient := mgr.GetClient()
	scheme := mgr.GetScheme()
	evtRec := mgr.GetEventRecorderFor("gateway")

	r := &gatewayReconciler{
		log:              log,
		client:           mgrClient,
		scheme:           scheme,
		finalizerManager: finalizerManager,
		eventRecorder:    evtRec,
		cloud:            cloud,
	}

	if config.DefaultServiceNetwork != "" {
		// Attempt creation of default service network, move gracefully even if it fails.
		snManager := deploy.NewDefaultServiceNetworkManager(log, cloud)
		_, err := snManager.CreateOrUpdate(context.Background(), &model.ServiceNetwork{
			Spec: model.ServiceNetworkSpec{
				Name: config.DefaultServiceNetwork,
			},
		})
		if err != nil {
			log.Infof(context.TODO(), "Could not setup default service network %s, proceeding without it - %s",
				config.DefaultServiceNetwork, err.Error())
		}
	}

	gwClassEventHandler := eventhandlers.NewEnqueueRequestsForGatewayClassEvent(log, mgrClient)
	vpcAssociationPolicyEventHandler := eventhandlers.NewVpcAssociationPolicyEventHandler(log, mgrClient)
	builder := ctrl.NewControllerManagedBy(mgr).
		For(&gwv1.Gateway{}, pkg_builder.WithPredicates(predicate.GenerationChangedPredicate{}))
	builder.Watches(&gwv1.GatewayClass{}, gwClassEventHandler)

	//Watch VpcAssociationPolicy CRD if it is installed
	ok, err := k8s.IsGVKSupported(mgr, anv1alpha1.GroupVersion.String(), anv1alpha1.VpcAssociationPolicyKind)
	if err != nil {
		return err
	}
	if ok {
		builder.Watches(&anv1alpha1.VpcAssociationPolicy{}, vpcAssociationPolicyEventHandler.MapToGateway())
	} else {
		log.Infof(context.TODO(), "VpcAssociationPolicy CRD is not installed, skipping watch")
	}
	return builder.Complete(r)
}

//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways/finalizers,verbs=update

func (r *gatewayReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctx = gwlog.StartReconcileTrace(ctx, r.log, "gateway", req.Name, req.Namespace)
	defer func() {
		gwlog.EndReconcileTrace(ctx, r.log)
	}()

	recErr := r.reconcile(ctx, req)
	if recErr != nil {
		r.log.Infow(ctx, "reconcile error", "name", req.Name, "message", recErr.Error())
	}
	res, retryErr := lattice_runtime.HandleReconcileError(recErr)
	if res.RequeueAfter != 0 {
		r.log.Infow(ctx, "requeue request", "name", req.Name, "requeueAfter", res.RequeueAfter)
	} else if res.Requeue {
		r.log.Infow(ctx, "requeue request", "name", req.Name)
	} else if retryErr == nil {
		r.log.Infow(ctx, "reconciled", "name", req.Name)
	}
	return res, retryErr
}

func (r *gatewayReconciler) reconcile(ctx context.Context, req ctrl.Request) error {

	gw := &gwv1.Gateway{}
	if err := r.client.Get(ctx, req.NamespacedName, gw); err != nil {
		return client.IgnoreNotFound(err)
	}

	if !k8s.IsControlledByLatticeGatewayController(ctx, r.client, gw) {
		r.log.Infow(ctx, "Gateway is not controlled by Lattice Gateway Controller", "name", req.Name)
		return nil
	}

	if !gw.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, gw)
	} else {
		return r.reconcileUpsert(ctx, gw)
	}
}

func (r *gatewayReconciler) reconcileDelete(ctx context.Context, gw *gwv1.Gateway) error {
	routes, err := core.ListAllRoutes(ctx, r.client)
	if err != nil {
		return err
	}

	for _, route := range routes {
		parents, err := k8s.FindControlledParents(ctx, r.client, route)
		if len(parents) > 0 {
			gw := parents[0]
			return fmt.Errorf("cannot delete gateway %s/%s - found referencing route %s/%s",
				gw.Namespace, gw.Name, route.Namespace(), route.Name())
		}
		if err != nil {
			continue
		}
	}

	err = r.finalizerManager.RemoveFinalizers(ctx, gw, gatewayFinalizer)
	if err != nil {
		return err
	}

	return nil
}

func (r *gatewayReconciler) reconcileUpsert(ctx context.Context, gw *gwv1.Gateway) error {
	if err := r.finalizerManager.AddFinalizers(ctx, gw, gatewayFinalizer); err != nil {
		r.eventRecorder.Event(gw, corev1.EventTypeWarning,
			k8s.GatewayEventReasonFailedAddFinalizer, fmt.Sprintf("failed add finalizer: %s", err))
		return err
	}

	err := UpdateGWListenerStatus(ctx, r.client, gw)
	if err != nil {
		err2 := r.updateGatewayAcceptStatus(ctx, gw, false)
		if err2 != nil {
			return errors.Wrap(err2, err.Error())
		}
	}

	err = r.updateGatewayAcceptStatus(ctx, gw, true)
	if err != nil {
		return err
	}

	snInfo, err := r.cloud.Lattice().FindServiceNetwork(ctx, gw.Name)
	if err != nil {
		if services.IsNotFoundError(err) {
			if err = r.updateGatewayProgrammedStatus(ctx, gw, gwv1.GatewayReasonPending, "VPC Lattice Service Network not found"); err != nil {
				return lattice_runtime.NewRetryError()
			}
			return nil
		}
		if errors.Is(err, services.ErrNameConflict) {
			if err = r.updateGatewayProgrammedStatus(ctx, gw, gwv1.GatewayReasonInvalid, "Found multiple VPC Lattice Service Networks matching Gateway name. Either ensure only one Service Network has a matching name, or use the Service Network's id as the Gateway name."); err != nil {
				return lattice_runtime.NewRetryError()
			}
			return nil
		}
		return err
	}

	err = r.updateGatewayProgrammedStatus(ctx, gw, gwv1.GatewayReasonProgrammed, fmt.Sprintf("aws-service-network-arn: %s", *snInfo.SvcNetwork.Arn))
	if err != nil {
		return err
	}

	return nil
}

func (r *gatewayReconciler) updateGatewayProgrammedStatus(
	ctx context.Context,
	gw *gwv1.Gateway,
	reason gwv1.GatewayConditionReason,
	message string,
) error {
	gwOld := gw.DeepCopy()

	if reason == gwv1.GatewayReasonProgrammed {
		gw.Status.Conditions = utils.GetNewConditions(gw.Status.Conditions, metav1.Condition{
			Type:               string(gwv1.GatewayConditionProgrammed),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: gw.Generation,
			Reason:             string(gwv1.GatewayReasonProgrammed),
			Message:            message,
		})
	} else {
		gw.Status.Conditions = utils.GetNewConditions(gw.Status.Conditions, metav1.Condition{
			Type:               string(gwv1.GatewayConditionProgrammed),
			Status:             metav1.ConditionFalse,
			ObservedGeneration: gw.Generation,
			Reason:             string(reason),
			Message:            message,
		})
	}

	if err := r.client.Status().Patch(ctx, gw, client.MergeFrom(gwOld)); err != nil {
		return fmt.Errorf("update gw status error, gw: %s, err: %w", gw.Name, err)
	}
	return nil
}

func (r *gatewayReconciler) updateGatewayAcceptStatus(ctx context.Context, gw *gwv1.Gateway, accepted bool) error {
	gwOld := gw.DeepCopy()

	var cond metav1.Condition
	if accepted {
		cond = metav1.Condition{
			Type:               string(gwv1.GatewayConditionAccepted),
			ObservedGeneration: gw.Generation,
			Message:            config.LatticeGatewayControllerName,
			Status:             metav1.ConditionTrue,
			Reason:             string(gwv1.GatewayReasonAccepted),
		}
	} else {
		cond = metav1.Condition{
			Type:               string(gwv1.GatewayConditionAccepted),
			ObservedGeneration: gw.Generation,
			Message:            config.LatticeGatewayControllerName,
			Status:             metav1.ConditionFalse,
			Reason:             string(gwv1.GatewayReasonInvalid),
		}
	}
	gw.Status.Conditions = utils.GetNewConditions(gw.Status.Conditions, cond)

	if err := r.client.Status().Patch(ctx, gw, client.MergeFrom(gwOld)); err != nil {
		return fmt.Errorf("update gateway status error, gw: %s, accepted: %t, err: %w", gw.Name, accepted, err)
	}

	return nil
}

func UpdateGWListenerStatus(ctx context.Context, k8sClient client.Client, gw *gwv1.Gateway) error {
	hasValidListener := false

	gwOld := gw.DeepCopy()

	routes, err := core.ListAllRoutes(ctx, k8sClient)
	if err != nil {
		return err
	}

	// Add one of lattice domains as GW address. This is supposed to be a single ingress endpoint (or a single pool of them)
	// but we have different endpoints for each service. This can represent incorrect value in some cases (e.g. cross-account)
	// Due to size limit, we cannot put all service addresses here.
	if len(routes) > 0 {
		gw.Status.Addresses = []gwv1.GatewayStatusAddress{}
		addressType := gwv1.HostnameAddressType
		for _, route := range routes {
			if route.DeletionTimestamp().IsZero() && len(route.K8sObject().GetAnnotations()) > 0 {
				if domain, exists := route.K8sObject().GetAnnotations()[LatticeAssignedDomainName]; exists {
					gw.Status.Addresses = append(gw.Status.Addresses, gwv1.GatewayStatusAddress{
						Type:  &addressType,
						Value: domain,
					})
					break
				}
			}
		}
	}

	if len(gw.Spec.Listeners) == 0 {
		return fmt.Errorf("failed to find gateway listener")
	}

	defaultListener := gw.Spec.Listeners[0]

	// go through each section of gw
	for _, listener := range gw.Spec.Listeners {
		listenerStatus := gwv1.ListenerStatus{
			Name: listener.Name,
		}

		// mark listenerStatus's condition
		listenerStatus.Conditions = make([]metav1.Condition, 0)

		//Check if RouteGroupKind in listener spec is supported
		validListener, supportedKinds := listenerRouteGroupKindSupported(listener)
		if !validListener {
			condition := metav1.Condition{
				Type:               string(gwv1.ListenerConditionResolvedRefs),
				Status:             metav1.ConditionFalse,
				Reason:             string(gwv1.ListenerReasonInvalidRouteKinds),
				ObservedGeneration: gw.Generation,
				LastTransitionTime: metav1.Now(),
			}
			listenerStatus.SupportedKinds = supportedKinds
			listenerStatus.Conditions = append(listenerStatus.Conditions, condition)
		} else {
			hasValidListener = true

			condition := metav1.Condition{
				Type:               string(gwv1.ListenerConditionAccepted),
				Status:             metav1.ConditionTrue,
				Reason:             string(gwv1.ListenerReasonAccepted),
				ObservedGeneration: gw.Generation,
				LastTransitionTime: metav1.Now(),
			}

			for _, route := range routes {
				if !route.DeletionTimestamp().IsZero() {
					// Ignore the deleted route
					continue
				}

				for _, parentRef := range route.Spec().ParentRefs() {
					if parentRef.Name != gwv1.ObjectName(gw.Name) {
						continue
					}

					if parentRef.Namespace != nil &&
						*parentRef.Namespace != gwv1.Namespace(gw.Namespace) {
						continue
					}

					var sectionName string
					if parentRef.SectionName == nil {
						sectionName = string(defaultListener.Name)
					} else {
						sectionName = string(*parentRef.SectionName)
					}

					if sectionName != string(listener.Name) {
						continue
					}

					if parentRef.Port != nil && *parentRef.Port != listener.Port {
						continue
					}

					listenerStatus.AttachedRoutes++
				}
			}

			if listener.Protocol == gwv1.HTTPSProtocolType {
				listenerStatus.SupportedKinds = append(listenerStatus.SupportedKinds, gwv1.RouteGroupKind{
					Kind: "GRPCRoute",
				})
			}

			listenerStatus.SupportedKinds = append(listenerStatus.SupportedKinds, gwv1.RouteGroupKind{
				Kind: "HTTPRoute",
			})
			listenerStatus.Conditions = append(listenerStatus.Conditions, condition)
		}

		found := false
		for i, oldStatus := range gw.Status.Listeners {
			if oldStatus.Name == listenerStatus.Name {
				gw.Status.Listeners[i].AttachedRoutes = listenerStatus.AttachedRoutes
				gw.Status.Listeners[i].SupportedKinds = listenerStatus.SupportedKinds
				// Only have one condition in the logic
				gw.Status.Listeners[i].Conditions = utils.GetNewConditions(gw.Status.Listeners[i].Conditions, listenerStatus.Conditions[0])
				found = true
			}
		}
		if !found {
			gw.Status.Listeners = append(gw.Status.Listeners, listenerStatus)
		}
	}

	if err := k8sClient.Status().Patch(ctx, gw, client.MergeFrom(gwOld)); err != nil {
		return errors.Wrapf(err, "listener update failed")
	}

	if hasValidListener {
		return nil
	} else {
		return fmt.Errorf("no valid listeners for %s", gw.Name)
	}
}

func listenerRouteGroupKindSupported(listener gwv1.Listener) (bool, []gwv1.RouteGroupKind) {
	validRoute := true
	supportedKinds := make([]gwv1.RouteGroupKind, 0)

	for _, routeGroupKind := range listener.AllowedRoutes.Kinds {
		if routeGroupKind.Kind == "HTTPRoute" {
			supportedKinds = append(supportedKinds, gwv1.RouteGroupKind{
				Kind: "HTTPRoute",
			})
		} else if routeGroupKind.Kind == "GRPCRoute" {
			if listener.Protocol == gwv1.HTTPSProtocolType {
				supportedKinds = append(supportedKinds, gwv1.RouteGroupKind{
					Kind: "GRPCRoute",
				})
			} else {
				validRoute = false
			}
		} else {
			validRoute = false
		}
	}

	if validRoute {
		return true, supportedKinds
	} else {
		return false, supportedKinds
	}
}
