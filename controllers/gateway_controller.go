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
	"github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/deploy"
	"github.com/aws/aws-application-networking-k8s/pkg/gateway"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	lattice_runtime "github.com/aws/aws-application-networking-k8s/pkg/runtime"
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
	gateway_api "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/aws/aws-application-networking-k8s/controllers/eventhandlers"
)

const (
	gatewayFinalizer = "gateway.k8s.aws/resources"
	defaultNameSpace = "default"
)

// GatewayReconciler reconciles a Gateway object
type GatewayReconciler struct {
	log              gwlog.Logger
	client           client.Client
	scheme           *runtime.Scheme
	finalizerManager k8s.FinalizerManager
	eventRecorder    record.EventRecorder
	modelBuilder     gateway.ServiceNetworkModelBuilder
	stackDeployer    deploy.StackDeployer
	cloud            aws.Cloud
	datastore        *latticestore.LatticeDataStore
	stackMarshaller  deploy.StackMarshaller
}

func RegisterGatewayController(
	log gwlog.Logger,
	cloud aws.Cloud,
	datastore *latticestore.LatticeDataStore,
	finalizerManager k8s.FinalizerManager,
	mgr ctrl.Manager,
) error {
	mgrClient := mgr.GetClient()
	scheme := mgr.GetScheme()
	evtRec := mgr.GetEventRecorderFor("gateway")

	modelBuilder := gateway.NewServiceNetworkModelBuilder()
	stackDeployer := deploy.NewServiceNetworkStackDeployer(cloud, mgrClient, datastore)
	stackMarshaller := deploy.NewDefaultStackMarshaller()

	r := &GatewayReconciler{
		log:              log,
		client:           mgrClient,
		scheme:           scheme,
		finalizerManager: finalizerManager,
		eventRecorder:    evtRec,
		modelBuilder:     modelBuilder,
		stackDeployer:    stackDeployer,
		cloud:            cloud,
		datastore:        datastore,
		stackMarshaller:  stackMarshaller,
	}

	gwClassEventHandler := eventhandlers.NewEnqueueRequestsForGatewayClassEvent(mgrClient)
	return ctrl.NewControllerManagedBy(mgr).
		For(&gateway_api.Gateway{}).
		Watches(
			&source.Kind{Type: &gateway_api.GatewayClass{}},
			gwClassEventHandler).
		Complete(r)
}

//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Gateway object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *GatewayReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
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

func (r *GatewayReconciler) isDefaultNameSpace(n string) bool {
	return n == defaultNameSpace
}

func (r *GatewayReconciler) reconcile(ctx context.Context, req ctrl.Request) error {

	gw := &gateway_api.Gateway{}
	if err := r.client.Get(ctx, req.NamespacedName, gw); err != nil {
		return client.IgnoreNotFound(err)
	}

	gwClass := &gateway_api.GatewayClass{}
	gwClassName := types.NamespacedName{
		Namespace: defaultNameSpace,
		Name:      string(gw.Spec.GatewayClassName),
	}

	if err := r.client.Get(ctx, gwClassName, gwClass); err != nil {
		r.log.Infow("ignore, not linked to any gateway-class", "name", req.Name, "gwclass", gwClassName)
		return client.IgnoreNotFound(err)
	}

	if gwClass.Spec.ControllerName == config.LatticeGatewayControllerName {
		if !gw.DeletionTimestamp.IsZero() {
			routes, err := core.ListAllRoutes(context.TODO(), r.client)
			if err != nil {
				return err
			}

			for _, route := range routes {
				if len(route.Spec().ParentRefs()) <= 0 {
					continue
				}
				gwNamespace := route.Namespace()
				if route.Spec().ParentRefs()[0].Namespace != nil {
					gwNamespace = string(*route.Spec().ParentRefs()[0].Namespace)
				}
				gwName := types.NamespacedName{
					Namespace: gwNamespace,
					Name:      string(route.Spec().ParentRefs()[0].Name),
				}

				httpGw := &gateway_api.Gateway{}
				if err := r.client.Get(context.TODO(), gwName, httpGw); err != nil {
					continue
				}

				if httpGw.Name == gw.Name && httpGw.Namespace == gw.Namespace {
					return fmt.Errorf("cannot delete gw, there is reference to httpGw, gw: %s, httpGw: %s", gw.Name, httpGw.Name)
				}
			}

			if err := r.cleanupGatewayResources(ctx, gw); err != nil {
				return errors.Wrapf(err, "failed to cleanup gw: %s", gw.Name)
			}

			err = r.finalizerManager.RemoveFinalizers(ctx, gw, gatewayFinalizer)
			if err != nil {
				return err
			}

			return nil
		}
		return r.reconcileGatewayResources(ctx, gw)
	} else {
		r.log.Infow("ignore non aws gateways", "name", req.Name, "gwClass controller name", gwClass.Spec.ControllerName)
	}
	return nil
}

func (r *GatewayReconciler) buildAndDeployModel(ctx context.Context, gw *gateway_api.Gateway) (core.Stack, *latticemodel.ServiceNetwork, error) {
	stack, serviceNetwork, err := r.modelBuilder.Build(ctx, gw)
	if err != nil {
		r.eventRecorder.Event(gw, corev1.EventTypeWarning,
			k8s.GatewayEventReasonFailedBuildModel,
			fmt.Sprintf("failed build model: %s", err))
		return nil, nil, err
	}
	jsonStack, err := r.stackMarshaller.Marshal(stack)
	if err != nil {
		return nil, nil, err
	}
	r.log.Debugw("successfully built model", "stack", jsonStack)

	if err := r.stackDeployer.Deploy(ctx, stack); err != nil {
		/*
			r.eventRecorder.Event(gw, corev1.EventTypeWarning,
				k8s.GatewayEventReasonFailedDeployModel, fmt.Sprintf("Failed deploy model due to %v", err))
		*/
		return nil, nil, err
	}
	r.log.Debugw("successfully deployed model",
		"stack", stack.StackID().Name+":"+stack.StackID().Namespace,
	)

	return stack, serviceNetwork, err
}
func (r *GatewayReconciler) reconcileGatewayResources(ctx context.Context, gw *gateway_api.Gateway) error {
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

	_, _, err = r.buildAndDeployModel(ctx, gw)
	if err != nil {
		return err
	}

	snInfo, err := r.cloud.Lattice().FindServiceNetwork(ctx, gw.Name, config.AccountID)
	if err != nil {
		return err
	}
	if snInfo == nil {
		return fmt.Errorf("Service network %s for account %s not found", gw.Name, config.AccountID)
	}

	if err = r.updateGatewayStatus(ctx, *snInfo.SvcNetwork.Arn, gw); err != nil {
		return err
	}

	return nil
}

func (r *GatewayReconciler) cleanupGatewayResources(ctx context.Context, gw *gateway_api.Gateway) error {
	_, _, err := r.buildAndDeployModel(ctx, gw)
	return err
}

func (r *GatewayReconciler) updateGatewayStatus(
	ctx context.Context,
	snArn string,
	gw *gateway_api.Gateway,
) error {
	gwOld := gw.DeepCopy()

	gw.Status.Conditions = utils.GetNewConditions(gw.Status.Conditions, metav1.Condition{
		Type:               string(gateway_api.GatewayConditionProgrammed),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: gw.Generation,
		Reason:             string(gateway_api.GatewayReasonProgrammed),
		Message:            fmt.Sprintf("aws-gateway-arn: %s", snArn),
	})

	if err := r.client.Status().Patch(ctx, gw, client.MergeFrom(gwOld)); err != nil {
		return fmt.Errorf("update gw status error, gw: %s, err: %w", gw.Name, err)
	}
	return nil
}

func (r *GatewayReconciler) updateGatewayAcceptStatus(ctx context.Context, gw *gateway_api.Gateway, accepted bool) error {
	gwOld := gw.DeepCopy()

	var cond metav1.Condition
	if accepted {
		cond = metav1.Condition{
			Type:               string(gateway_api.GatewayConditionAccepted),
			ObservedGeneration: gw.Generation,
			Message:            config.LatticeGatewayControllerName,
			Status:             metav1.ConditionTrue,
			Reason:             string(gateway_api.GatewayReasonAccepted),
		}
	} else {
		cond = metav1.Condition{
			Type:               string(gateway_api.GatewayConditionAccepted),
			ObservedGeneration: gw.Generation,
			Message:            config.LatticeGatewayControllerName,
			Status:             metav1.ConditionFalse,
			Reason:             string(gateway_api.GatewayReasonInvalid),
		}
	}
	gw.Status.Conditions = utils.GetNewConditions(gw.Status.Conditions, cond)

	if err := r.client.Status().Patch(ctx, gw, client.MergeFrom(gwOld)); err != nil {
		return fmt.Errorf("update gateway status error, gw: %s, accepted: %t, err: %w", gw.Name, accepted, err)
	}

	return nil
}

func UpdateGWListenerStatus(ctx context.Context, k8sClient client.Client, gw *gateway_api.Gateway) error {
	hasValidListener := false

	gwOld := gw.DeepCopy()

	routes, err := core.ListAllRoutes(ctx, k8sClient)
	if err != nil {
		return err
	}

	// Add one of lattice domains as GW address. This can represent incorrect value in some cases (e.g. cross-account)
	// TODO: support multiple endpoint addresses across services.
	if len(routes) > 0 {
		gw.Status.Addresses = []gateway_api.GatewayAddress{}
		addressType := gateway_api.HostnameAddressType
		for _, route := range routes {
			if route.DeletionTimestamp().IsZero() && len(route.K8sObject().GetAnnotations()) > 0 {
				if domain, exists := route.K8sObject().GetAnnotations()[LatticeAssignedDomainName]; exists {
					gw.Status.Addresses = append(gw.Status.Addresses, gateway_api.GatewayAddress{
						Type:  &addressType,
						Value: domain,
					})
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
		listenerStatus := gateway_api.ListenerStatus{
			Name: listener.Name,
		}

		// mark listenerStatus's condition
		listenerStatus.Conditions = make([]metav1.Condition, 0)

		//Check if RouteGroupKind in listener spec is supported
		validListener, supportedKinds := listenerRouteGroupKindSupported(listener)
		if !validListener {
			condition := metav1.Condition{
				Type:               string(gateway_api.ListenerConditionResolvedRefs),
				Status:             metav1.ConditionFalse,
				Reason:             string(gateway_api.ListenerReasonInvalidRouteKinds),
				ObservedGeneration: gw.Generation,
				LastTransitionTime: metav1.Now(),
			}
			listenerStatus.SupportedKinds = supportedKinds
			listenerStatus.Conditions = append(listenerStatus.Conditions, condition)
		} else {
			hasValidListener = true

			condition := metav1.Condition{
				Type:               string(gateway_api.ListenerConditionAccepted),
				Status:             metav1.ConditionTrue,
				Reason:             string(gateway_api.ListenerReasonAccepted),
				ObservedGeneration: gw.Generation,
				LastTransitionTime: metav1.Now(),
			}

			for _, route := range routes {
				if !route.DeletionTimestamp().IsZero() {
					// Ignore the deleted route
					continue
				}

				for _, parentRef := range route.Spec().ParentRefs() {
					if parentRef.Name != gateway_api.ObjectName(gw.Name) {
						continue
					}

					if parentRef.Namespace != nil &&
						*parentRef.Namespace != gateway_api.Namespace(gw.Namespace) {
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

			if listener.Protocol == gateway_api.HTTPSProtocolType {
				listenerStatus.SupportedKinds = append(listenerStatus.SupportedKinds, gateway_api.RouteGroupKind{
					Kind: "GRPCRoute",
				})
			}

			listenerStatus.SupportedKinds = append(listenerStatus.SupportedKinds, gateway_api.RouteGroupKind{
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

func listenerRouteGroupKindSupported(listener gateway_api.Listener) (bool, []gateway_api.RouteGroupKind) {
	validRoute := true
	supportedKinds := make([]gateway_api.RouteGroupKind, 0)

	for _, routeGroupKind := range listener.AllowedRoutes.Kinds {
		if routeGroupKind.Kind == "HTTPRoute" {
			supportedKinds = append(supportedKinds, gateway_api.RouteGroupKind{
				Kind: "HTTPRoute",
			})
		} else if routeGroupKind.Kind == "GRPCRoute" {
			if listener.Protocol == gateway_api.HTTPSProtocolType {
				supportedKinds = append(supportedKinds, gateway_api.RouteGroupKind{
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
