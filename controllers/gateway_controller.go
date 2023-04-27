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
	"time"

	"github.com/golang/glog"
	"github.com/pkg/errors"

	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
	gateway_api "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/aws/aws-application-networking-k8s/controllers/eventhandlers"
	"github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/deploy"
	"github.com/aws/aws-application-networking-k8s/pkg/gateway"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	latticemodel "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	lattice_runtime "github.com/aws/aws-application-networking-k8s/pkg/runtime"
)

const (
	gatewayFinalizer = "gateway.k8s.aws/resources"
	defaultNameSpace = "default"
)

// GatewayReconciler reconciles a Gateway object
type GatewayReconciler struct {
	client.Client
	Scheme              *runtime.Scheme
	gwClassReconciler   *GatewayClassReconciler
	httpRouteReconciler *HTTPRouteReconciler
	finalizerManager    k8s.FinalizerManager
	eventRecorder       record.EventRecorder
	modelBuilder        gateway.ServiceNetworkModelBuilder
	stackDeployer       deploy.StackDeployer
	cloud               aws.Cloud
	latticeDataStore    *latticestore.LatticeDataStore
	stackMarshaller     deploy.StackMarshaller
}

func NewGatewayReconciler(client client.Client, scheme *runtime.Scheme, eventRecorder record.EventRecorder,
	gwClassReconciler *GatewayClassReconciler, finalizerManager k8s.FinalizerManager,
	ds *latticestore.LatticeDataStore, cloud aws.Cloud) *GatewayReconciler {

	modelBuilder := gateway.NewServiceNetworkModelBuilder()
	stackDeployer := deploy.NewServiceNetworkStackDeployer(cloud, client, ds)
	stackMarshaller := deploy.NewDefaultStackMarshaller()
	return &GatewayReconciler{
		Client:            client,
		Scheme:            scheme,
		gwClassReconciler: gwClassReconciler,
		finalizerManager:  finalizerManager,
		eventRecorder:     eventRecorder,
		modelBuilder:      modelBuilder,
		stackDeployer:     stackDeployer,
		cloud:             cloud,
		latticeDataStore:  ds,
		stackMarshaller:   stackMarshaller,
	}
}

func (r *GatewayReconciler) UpdateGatewayReconciler(httpRoute *HTTPRouteReconciler) {
	r.httpRouteReconciler = httpRoute
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
	return lattice_runtime.HandleReconcileError(r.reconcile(ctx, req))
}

func (r *GatewayReconciler) isDefaultNameSpace(n string) bool {
	return n == defaultNameSpace
}

func (r *GatewayReconciler) reconcile(ctx context.Context, req ctrl.Request) error {
	gwLog := log.FromContext(ctx)

	gwLog.Info("GatewayReconciler")
	gw := &gateway_api.Gateway{}

	if err := r.Client.Get(ctx, req.NamespacedName, gw); err != nil {
		return client.IgnoreNotFound(err)
	}

	gwClass := &gateway_api.GatewayClass{}
	gwClassName := types.NamespacedName{
		Namespace: defaultNameSpace,
		Name:      string(gw.Spec.GatewayClassName),
	}

	if err := r.gwClassReconciler.Client.Get(ctx, gwClassName, gwClass); err != nil {
		gwLog.Info("Ignore it since not link to any gatewayclass")
		return client.IgnoreNotFound(err)
	}

	if gwClass.Spec.ControllerName == config.LatticeGatewayControllerName {

		if !gw.DeletionTimestamp.IsZero() {

			glog.V(6).Info(fmt.Sprintf("Checking if gateway can be deleted %v\n", gw.Name))

			httpRouteList := &gateway_api.HTTPRouteList{}

			r.Client.List(context.TODO(), httpRouteList)
			for _, httpRoute := range httpRouteList.Items {

				if len(httpRoute.Spec.ParentRefs) <= 0 {
					continue
				}
				gwName := types.NamespacedName{
					Namespace: httpRoute.Namespace,
					Name:      string(httpRoute.Spec.ParentRefs[0].Name),
				}

				httpGW := &gateway_api.Gateway{}

				if err := r.Client.Get(context.TODO(), gwName, httpGW); err != nil {
					continue
				}

				if httpGW.Name == gw.Name && httpGW.Namespace == gw.Namespace {

					gwLog.Info("Can not delete because it is referenced by some HTTPRoutes")
					return errors.New("retry later, since it is referenced by some HTTPRoutes")
				}

			}

			if err := r.cleanupGatewayResources(ctx, gw); err != nil {
				glog.V(2).Info(fmt.Sprintf("Failed to cleanup gw %v, err %v \n", gw, err))
				return err

			}
			gwLog.Info("Successfully removed finalizer")
			r.finalizerManager.RemoveFinalizers(ctx, gw, gatewayFinalizer)
			return nil
		}

		return r.reconcileGatewayResources(ctx, gw)
	} else {
		gwLog.Info("Ignore non aws gateways!!!")
	}
	return nil
}

func (r *GatewayReconciler) buildAndDeployModel(ctx context.Context, gw *gateway_api.Gateway) (core.Stack, *latticemodel.ServiceNetwork, error) {
	gwLog := log.FromContext(ctx)

	stack, serviceNetwork, err := r.modelBuilder.Build(ctx, gw)

	if err != nil {
		r.eventRecorder.Event(gw, corev1.EventTypeWarning,
			k8s.GatewayEventReasonFailedBuildModel,
			fmt.Sprintf("Failed BuildModel due to %v", err))
	}

	stackJSON, err := r.stackMarshaller.Marshal(stack)
	gwLog.Info("Successfully built model", stackJSON, "")

	if err := r.stackDeployer.Deploy(ctx, stack); err != nil {
		/*
			r.eventRecorder.Event(gw, corev1.EventTypeWarning,
				k8s.GatewayEventReasonFailedDeployModel, fmt.Sprintf("Failed deploy model due to %v", err))
		*/
		return nil, nil, err
	}
	gwLog.Info("Successfully deployed model")

	return stack, serviceNetwork, err
}
func (r *GatewayReconciler) reconcileGatewayResources(ctx context.Context, gw *gateway_api.Gateway) error {
	gwLog := log.FromContext(ctx)

	gwLog.Info("reconcile gateway resource")

	if err := r.finalizerManager.AddFinalizers(ctx, gw, gatewayFinalizer); err != nil {
		r.eventRecorder.Event(gw, corev1.EventTypeWarning, k8s.GatewayEventReasonFailedAddFinalizer, fmt.Sprintf("Failed add finalizer due to %v", err))
		return errors.New("TODO ")
	}
	if UpdateGWListenerStatus(ctx, r.Client, gw) == nil {
		r.updateGatewayAcceptStatus(ctx, gw, true)
	} else {
		r.updateGatewayAcceptStatus(ctx, gw, false)
		return errors.New("failed to update gateway listener status")
	}

	_, _, err := r.buildAndDeployModel(ctx, gw)

	if err != nil {
		glog.V(6).Infof("Failed on buildAndDeployModel %v\n", err)
		return err
	}

	var serviceNetworkStatus latticestore.ServiceNetwork
	serviceNetworkStatus, err = r.latticeDataStore.GetServiceNetworkStatus(gw.Name, config.AccountID)

	glog.V(6).Infof("serviceNetworkStatus : %v for %s  error %v \n", serviceNetworkStatus, gw.Name, err)

	if err = r.updateGatewayStatus(ctx, &serviceNetworkStatus, gw); err != nil {
		glog.V(2).Infof("Failed to updateGatewayStatus %v err %v\n", gw, err)
		return errors.New("failed to update gateway status")
	}
	return nil

}

func (r *GatewayReconciler) cleanupGatewayResources(ctx context.Context, gw *gateway_api.Gateway) error {
	_, _, err := r.buildAndDeployModel(ctx, gw)
	return err

}

func (r *GatewayReconciler) updateGatewayStatus(ctx context.Context, serviceNetworkStatus *latticestore.ServiceNetwork, gw *gateway_api.Gateway) error {

	glog.V(6).Infof("updateGatewayStatus: updating last transition time \n")
	gwOld := gw.DeepCopy()

	if len(gw.Status.Conditions) <= 1 {
		condition := metav1.Condition{}
		gw.Status.Conditions = append(gw.Status.Conditions, condition)
	}

	gw.Status.Conditions[1].LastTransitionTime = metav1.NewTime(time.Now())
	gw.Status.Conditions[1].Status = "True"
	gw.Status.Conditions[1].Message = fmt.Sprintf("aws-gateway-arn: %s", serviceNetworkStatus.ARN)
	gw.Status.Conditions[1].Reason = "Reconciled"
	gw.Status.Conditions[1].ObservedGeneration = gw.Generation
	gw.Status.Conditions[0].ObservedGeneration = gw.Generation // update the accept
	gw.Status.Conditions[1].Type = string(gateway_api.GatewayConditionProgrammed)

	// TODO following is causing crash on some platform, see https://t.corp.amazon.com/b7c9ea6c-5168-4616-b718-c1bdf78dbdf1/communication
	//gw.Annotations["gateway.networking.k8s.io/aws-gateway-id"] = serviceNetworkStatus.ID

	if err := r.Client.Status().Patch(ctx, gw, client.MergeFrom(gwOld)); err != nil {
		return errors.Wrapf(err, "failed to update gateway status")
	}

	return nil
}

func (r *GatewayReconciler) updateGatewayAcceptStatus(ctx context.Context, gw *gateway_api.Gateway, accepted bool) error {

	gwOld := gw.DeepCopy()

	glog.V(6).Infof("updateGatewayStatus: updating last transition time \n")
	if gw.Status.Conditions[0].LastTransitionTime == eventhandlers.ZeroTransitionTime {
		gw.Status.Conditions[0].LastTransitionTime = metav1.NewTime(time.Now())
	}
	if accepted {
		gw.Status.Conditions[0].Status = "True"
		gw.Status.Conditions[0].Reason = "Accepted"
	} else {
		gw.Status.Conditions[0].Status = "False"
		gw.Status.Conditions[0].Reason = "Invalid"
	}
	gw.Status.Conditions[0].Message = config.LatticeGatewayControllerName
	gw.Status.Conditions[0].ObservedGeneration = gw.Generation
	gw.Status.Conditions[0].Type = string(gateway_api.GatewayConditionAccepted)

	if err := r.Client.Status().Patch(ctx, gw, client.MergeFrom(gwOld)); err != nil {
		return errors.Wrapf(err, "failed to update gateway status")
	}

	return nil
}

func (r *GatewayReconciler) updateBadStatus(ctx context.Context, message string, gw *gateway_api.Gateway) error {

	gwOld := gw.DeepCopy()

	glog.V(6).Infof("updateGatewayStatus: updating last transition time \n")
	if gw.Status.Conditions[0].LastTransitionTime == eventhandlers.ZeroTransitionTime {
		gw.Status.Conditions[0].LastTransitionTime = metav1.NewTime(time.Now())
	}

	gw.Status.Conditions[0].Status = "False"
	gw.Status.Conditions[0].Message = message
	gw.Status.Conditions[0].Reason = "NoReconcile"
	gw.Status.Conditions[0].Type = "NotAccepted"

	if err := r.Client.Status().Patch(ctx, gw, client.MergeFrom(gwOld)); err != nil {
		return errors.Wrapf(err, "failed to update gateway status")
	}

	return nil
}

func UpdateHTTPRouteListenerStatus(ctx context.Context, k8sclient client.Client, httproute *gateway_api.HTTPRoute) error {
	gw := &gateway_api.Gateway{}

	gwNamespace := "default"
	if httproute.Spec.ParentRefs[0].Namespace != nil {
		gwNamespace = string(*httproute.Spec.ParentRefs[0].Namespace)
	}
	gwName := types.NamespacedName{
		Namespace: gwNamespace,
		// TODO assume one parent for now and point to service network
		Name: string(httproute.Spec.ParentRefs[0].Name),
	}

	if err := k8sclient.Get(ctx, gwName, gw); err != nil {
		glog.V(2).Infof("Failed to update gateway listener status due to gatewag not found for %v\n", httproute.Spec)
		return errors.New("gateway not found")
	}

	return UpdateGWListenerStatus(ctx, k8sclient, gw)
}

func listenerRouteGroupKindSupported(listener gateway_api.Listener) (bool, []gateway_api.RouteGroupKind) {
	defaultSupportedKind := []gateway_api.RouteGroupKind{
		gateway_api.RouteGroupKind{
			Kind: "HTTPRoute",
		},
	}

	validRoute := true
	supportedKind := make([]gateway_api.RouteGroupKind, 0)

	for _, routeGroupKind := range listener.AllowedRoutes.Kinds {
		// today, controller only support HTTPRoute
		if routeGroupKind.Kind != "HTTPRoute" {
			validRoute = false
		} else {
			supportedKind = append(supportedKind, gateway_api.RouteGroupKind{
				Kind: "HTTPRoute",
			})
		}

	}

	if validRoute {
		return true, defaultSupportedKind
	} else {
		return false, supportedKind
	}

}

func UpdateGWListenerStatus(ctx context.Context, k8sclient client.Client, gw *gateway_api.Gateway) error {
	hasValidListener := false

	gwOld := gw.DeepCopy()

	glog.V(6).Infof("Before update, the snapshot of listeners  %v \n", gw.Status.Listeners)

	gw.Status.Listeners = make([]gateway_api.ListenerStatus, 0)

	httpRouteList := &gateway_api.HTTPRouteList{}

	k8sclient.List(context.TODO(), httpRouteList)

	if len(gw.Spec.Listeners) == 0 {
		glog.V(2).Infof("Failed to find gateway listener for gw %v ", gw)
		return errors.New("no gateway listner found")
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
		validListener, supportedkind := listenerRouteGroupKindSupported(listener)
		if !validListener {
			condition := metav1.Condition{
				Type:               string(gateway_api.ListenerConditionResolvedRefs),
				Status:             metav1.ConditionFalse,
				Reason:             string(gateway_api.ListenerReasonInvalidRouteKinds),
				ObservedGeneration: gw.Generation,
				LastTransitionTime: metav1.NewTime(time.Now()),
			}
			listenerStatus.SupportedKinds = supportedkind
			listenerStatus.Conditions = append(listenerStatus.Conditions, condition)

		} else {

			hasValidListener = true
			attachedRoutes := 0

			condition := metav1.Condition{
				Type:               "Accepted",
				Status:             "True",
				Reason:             "Accepted",
				ObservedGeneration: gw.Generation,
				// TODO need to use previous one
				LastTransitionTime: metav1.NewTime(time.Now()),
			}

			for _, httpRoute := range httpRouteList.Items {
				if !httpRoute.DeletionTimestamp.IsZero() {
					// Ignore the delete httproute
					continue
				}
				for _, parentRef := range httpRoute.Spec.ParentRefs {
					if parentRef.Name != gateway_api.ObjectName(gw.Name) {
						continue
					}

					if parentRef.Namespace != nil &&
						*parentRef.Namespace != gateway_api.Namespace(gw.Namespace) {
						continue
					}
					attachedRoutes++

					var httpSectionName string
					if parentRef.SectionName == nil {
						httpSectionName = string(defaultListener.Name)

					} else {
						httpSectionName = string(*parentRef.SectionName)
					}

					if httpSectionName != string(listener.Name) {
						continue
					}
					listenerStatus.AttachedRoutes++
					attachedRoutes++

				}
			}
			httpKind := gateway_api.RouteGroupKind{
				Kind: "HTTPRoute",
			}
			listenerStatus.SupportedKinds = append(listenerStatus.SupportedKinds, httpKind)
			listenerStatus.Conditions = append(listenerStatus.Conditions, condition)

		}
		gw.Status.Listeners = append(gw.Status.Listeners, listenerStatus)

	}

	glog.V(6).Infof("After update, the snapshot of listener status %v", gw.Status.Listeners)

	if err := k8sclient.Status().Patch(ctx, gw, client.MergeFrom(gwOld)); err != nil {
		glog.V(2).Infof("liwwu1-failed to update gateway listener status %v, status %v", err, gw.Status.Listeners)
		return errors.Wrapf(err, "failed to update gateway status")
	}

	if hasValidListener {
		return nil
	} else {
		glog.V(2).Infof("no valid listeners for %v\n", gw.Name)
		return errors.New("invalid listeners")
	}

}

// SetupWithManager sets up the controller with the Manager.
func (r *GatewayReconciler) SetupWithManager(mgr ctrl.Manager) error {
	gwClassEventHandler := eventhandlers.NewEnqueueRequestsForGatewayClassEvent(r.Client)
	return ctrl.NewControllerManagedBy(mgr).
		// Uncomment the following line adding a pointer to an instance of the controlled resource as an argument
		For(&gateway_api.Gateway{}).
		Watches(
			&source.Kind{Type: &gateway_api.GatewayClass{}},
			gwClassEventHandler).
		Complete(r)
}
