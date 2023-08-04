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
	gateway_api "sigs.k8s.io/gateway-api/apis/v1beta1"
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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/discovery"
	"sigs.k8s.io/external-dns/endpoint"
)

// HTTPRouteReconciler reconciles a HTTPRoute object
type HTTPRouteReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	gwReconciler      *GatewayReconciler
	gwClassReconciler *GatewayClassReconciler
	finalizerManager  k8s.FinalizerManager
	eventRecorder     record.EventRecorder
	modelBuilder      gateway.LatticeServiceBuilder
	stackDeployer     deploy.StackDeployer
	latticeDataStore  *latticestore.LatticeDataStore
	stackMashaller    deploy.StackMarshaller
}

const (
	httpRouteFinalizer        = "httproute.k8s.aws/resources"
	LatticeAssignedDomainName = "application-networking.k8s.aws/lattice-assigned-domain-name"
)

func NewHttpRouteReconciler(cloud aws.Cloud, client client.Client, scheme *runtime.Scheme, eventRecorder record.EventRecorder,
	gwReconciler *GatewayReconciler, gwClassReconciler *GatewayClassReconciler, finalizerManager k8s.FinalizerManager,
	latticeDataStore *latticestore.LatticeDataStore) *HTTPRouteReconciler {
	modelBuilder := gateway.NewLatticeServiceBuilder(client, latticeDataStore, cloud)
	stackDeployer := deploy.NewLatticeServiceStackDeploy(cloud, client, latticeDataStore)
	stackMarshaller := deploy.NewDefaultStackMarshaller()

	return &HTTPRouteReconciler{
		Client:            client,
		Scheme:            scheme,
		gwReconciler:      gwReconciler,
		gwClassReconciler: gwClassReconciler,
		finalizerManager:  finalizerManager,
		modelBuilder:      modelBuilder,
		stackDeployer:     stackDeployer,
		eventRecorder:     eventRecorder,
		latticeDataStore:  latticeDataStore,
		stackMashaller:    stackMarshaller,
	}
}

//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the HTTPRoute object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *HTTPRouteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return lattice_runtime.HandleReconcileError(r.reconcile(ctx, req))
}

func (r *HTTPRouteReconciler) reconcile(ctx context.Context, req ctrl.Request) error {
	httpLog := log.FromContext(ctx)

	// TODO(user): your logic here
	httpLog.Info("HTTPRouteReconciler")

	httpRoute := core.HTTPRoute{}

	if err := r.Client.Get(ctx, req.NamespacedName, &httpRoute.HTTPRoute); err != nil {
		return client.IgnoreNotFound(err)
	}

	if !r.isHTTPRouteRelevant(ctx, httpRoute) {
		// not relevalnt
		return nil
	}

	if !httpRoute.DeletionTimestamp.IsZero() {
		httpLog.Info("Deleting")
		r.eventRecorder.Event(httpRoute.GetRuntimeObject(), corev1.EventTypeNormal,
			k8s.HTTPRouteeventReasonReconcile, "Deleting Reconcile")
		if err := r.cleanupHTTPRouteResources(ctx, httpRoute); err != nil {
			glog.V(6).Infof("Failed to cleanup HTTPRoute %v err %v\n", httpRoute, err)
			return err
		}
		UpdateHTTPRouteListenerStatus(ctx, r.Client, httpRoute)
		r.finalizerManager.RemoveFinalizers(ctx, &httpRoute, httpRouteFinalizer)

		// TODO delete metrics
		return nil
	} else {
		httpLog.Info("Adding/Updating")
		r.eventRecorder.Event(httpRoute.GetRuntimeObject(), corev1.EventTypeNormal,
			k8s.HTTPRouteeventReasonReconcile, "Adding/Updating Reconcile")
		err := r.reconcileHTTPRouteResource(ctx, httpRoute)
		// TODO add/update metrics
		return err
	}

}

func (r *HTTPRouteReconciler) cleanupHTTPRouteResources(ctx context.Context, httpRoute core.Route) error {

	_, _, err := r.buildAndDeployModel(ctx, httpRoute)

	return err
}

func (r *HTTPRouteReconciler) isHTTPRouteRelevant(ctx context.Context, httpRoute core.HTTPRoute) bool {

	if len(httpRoute.Spec.ParentRefs) == 0 {
		glog.V(2).Infof("Ignore HTTPRoute which has no ParentRefs gateway %v \n ", httpRoute.Spec)
		return false
	}

	gw := &gateway_api.Gateway{}

	gwNamespace := httpRoute.Namespace
	if httpRoute.Spec.ParentRefs[0].Namespace != nil {
		gwNamespace = string(*httpRoute.Spec.ParentRefs[0].Namespace)
	}
	gwName := types.NamespacedName{
		Namespace: gwNamespace,
		Name:      string(httpRoute.Spec.ParentRefs[0].Name),
	}

	if err := r.gwReconciler.Client.Get(ctx, gwName, gw); err != nil {
		glog.V(6).Infof("Could not find gateway %s: %s\n", gwName.String(), err.Error())
		glog.V(6).Infof("Ignore HTTPRoute whose ParentRef gatway object has NOT defined yet for %v\n", httpRoute.Spec)
		return false
	}

	// make sure gateway is a aws-vpc-lattice
	gwClass := &gateway_api.GatewayClass{}
	gwClassName := types.NamespacedName{
		Namespace: "default",
		Name:      string(gw.Spec.GatewayClassName),
	}

	if err := r.gwClassReconciler.Client.Get(ctx, gwClassName, gwClass); err != nil {
		glog.V(6).Infof("Ignore HTTPRoute that NOT controlled by any GatewayClass for %v\n", httpRoute.Spec)
		return false
	}

	if gwClass.Spec.ControllerName == config.LatticeGatewayControllerName {
		glog.V(6).Infof("Found aws-vpc-lattice for HTTPRoute for %v\n", httpRoute.Spec)

		return true
	} else {
		glog.V(6).Infof("Ignore non aws-vpc-lattice HTTPRoute !!! %v\n", httpRoute.Spec)
		return false
	}
}

func (r *HTTPRouteReconciler) buildAndDeployModel(ctx context.Context, route core.Route) (core.Stack, *latticemodel.Service, error) {
	httpLog := log.FromContext(ctx)

	stack, latticeService, err := r.modelBuilder.Build(ctx, route)

	if err != nil {

		r.eventRecorder.Event(route.GetRuntimeObject(), corev1.EventTypeWarning,
			k8s.HTTPRouteEventReasonFailedBuildModel, fmt.Sprintf("Failed build model due to %v", err))
		glog.V(6).Infof("buildAndDeployModel, Failed build model for %v due to %v\n", route.GetName(), err)

		// Build failed
		// TODO continue deploy to trigger reconsile of stale HTTProute and policy
		return nil, nil, err
	}

	stackJSON, err := r.stackMashaller.Marshal(stack)
	if err != nil {
		//TODO
		glog.V(6).Infof("error on r.stackMashaller.Marshal error %v \n", err)
	}

	httpLog.Info("Successfully built model:", stackJSON, "")

	if err := r.stackDeployer.Deploy(ctx, stack); err != nil {
		glog.V(6).Infof("HTTPRouteReconciler: Failed deploy %s due to err %v \n", route.GetName(), err)

		var retryErr = errors.New(lattice.LATTICE_RETRY)

		if errors.As(err, &retryErr) {
			r.eventRecorder.Event(route.GetRuntimeObject(), corev1.EventTypeNormal,
				k8s.HTTPRouteEventReasonRetryReconcile, "retry reconcile...")

		} else {
			r.eventRecorder.Event(route.GetRuntimeObject(), corev1.EventTypeWarning,
				k8s.HTTPRouteEventReasonFailedDeployModel, fmt.Sprintf("Failed deploy model due to %v", err))
		}
		return nil, nil, err
	}

	httpLog.Info("Successfully deployed model")

	return stack, latticeService, err
}

func (r *HTTPRouteReconciler) reconcileHTTPRouteResource(ctx context.Context, httpRoute core.HTTPRoute) error {
	glog.V(6).Infof("Beginning -- reconcileHTTPRouteResource, [%v]\n", httpRoute)

	if err := r.finalizerManager.AddFinalizers(ctx, &httpRoute.HTTPRoute, httpRouteFinalizer); err != nil {
		r.eventRecorder.Event(httpRoute.GetRuntimeObject(), corev1.EventTypeWarning, k8s.HTTPRouteventReasonFailedAddFinalizer, fmt.Sprintf("Failed add finalizer due to %v", err))
	}

	_, _, err := r.buildAndDeployModel(ctx, httpRoute)

	//TODO add metric

	if err == nil {
		r.eventRecorder.Event(httpRoute.GetRuntimeObject(), corev1.EventTypeNormal,
			k8s.HTTPRouteeventReasonDeploySucceed, "Adding/Updating reconcile Done!")

		serviceStatus, err1 := r.latticeDataStore.GetLatticeService(httpRoute.Name, httpRoute.Namespace)

		if err1 == nil {
			r.updateHTTPRouteStatus(ctx, serviceStatus.DNS, httpRoute)
		}
	}

	return err

}

func (r *HTTPRouteReconciler) updateHTTPRouteStatus(ctx context.Context, dns string, httpRoute core.HTTPRoute) error {
	glog.V(6).Infof("updateHTTPRouteStatus: httpRoute %v, dns %v\n", httpRoute, dns)
	httprouteOld := httpRoute.HTTPRoute.DeepCopy()

	if len(httpRoute.ObjectMeta.Annotations) == 0 {
		httpRoute.ObjectMeta.Annotations = make(map[string]string)
	}

	httpRoute.ObjectMeta.Annotations[LatticeAssignedDomainName] = dns
	if err := r.Client.Patch(ctx, &httpRoute.HTTPRoute, client.MergeFrom(httprouteOld)); err != nil {
		glog.V(2).Infof("updateHTTPRouteStatus: Patch() received err %v \n", err)
		return errors.Wrapf(err, "failed to update httpRoute status")
	}
	httprouteOld = httpRoute.HTTPRoute.DeepCopy()

	if len(httpRoute.Status.RouteStatus.Parents) == 0 {
		httpRoute.Status.RouteStatus.Parents = make([]gateway_api.RouteParentStatus, 1)
	}
	httpRoute.Status.RouteStatus.Parents[0].ParentRef = httpRoute.Spec.ParentRefs[0]
	httpRoute.Status.RouteStatus.Parents[0].ControllerName = config.LatticeGatewayControllerName

	// Update listener Status
	if err := UpdateHTTPRouteListenerStatus(ctx, r.Client, httpRoute); err != nil {
		updateRouteCondition(httpRoute, metav1.Condition{
			Type:               string(gateway_api.RouteConditionAccepted),
			Status:             metav1.ConditionFalse,
			ObservedGeneration: httpRoute.Generation,
			Reason:             string(gateway_api.RouteReasonNoMatchingParent),
			Message:            fmt.Sprintf("Could not match gateway %s: %s", httpRoute.Spec.ParentRefs[0].Name, err.Error()),
		})
	} else {
		updateRouteCondition(httpRoute, metav1.Condition{
			Type:               string(gateway_api.RouteConditionAccepted),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: httpRoute.Generation,
			Reason:             string(gateway_api.RouteReasonAccepted),
			Message:            fmt.Sprintf("DNS Name: %s", dns),
		})
		updateRouteCondition(httpRoute, metav1.Condition{
			Type:               string(gateway_api.RouteConditionResolvedRefs),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: httpRoute.Generation,
			Reason:             string(gateway_api.RouteReasonResolvedRefs),
			Message:            fmt.Sprintf("DNS Name: %s", dns),
		})
	}

	if err := r.Client.Status().Patch(ctx, &httpRoute.HTTPRoute, client.MergeFrom(httprouteOld)); err != nil {
		glog.V(2).Infof("updateHTTPRouteStatus: Patch() received err %v \n", err)
		return errors.Wrapf(err, "failed to update httpRoute status")
	}
	glog.V(6).Infof("updateHTTPRouteStatus patched dns %v \n", dns)

	return nil
}

func updateRouteCondition(httproute core.HTTPRoute, updated metav1.Condition) {
	httproute.Status.RouteStatus.Parents[0].Conditions = updateCondition(httproute.Status.RouteStatus.Parents[0].Conditions, updated)
}

// SetupWithManager sets up the controller with the Manager.
func (r *HTTPRouteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	gwEventHandler := eventhandlers.NewEnqueueRequestGatewayEvent(r.Client)
	svcEventHandler := eventhandlers.NewEqueueHTTPRequestServiceEvent(r.Client)
	svcImportEventHandler := eventhandlers.NewEqueueRequestServiceImportEvent(r.Client)

	builder := ctrl.NewControllerManagedBy(mgr).
		For(&gateway_api.HTTPRoute{}).
		Watches(&source.Kind{Type: &gateway_api.Gateway{}}, gwEventHandler).
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

func isExternalDNSSupported(mgr ctrl.Manager) (bool, error) {
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(mgr.GetConfig())
	if err != nil {
		return false, err
	}
	// Query for known OpenShift API resource to verify it is available
	apiResources, err := discoveryClient.ServerResourcesForGroupVersion("externaldns.k8s.io/v1alpha1")
	if err != nil {
		if apierrors.IsNotFound(err) {
			glog.V(2).Infof("DNSEndpoint CRD is not supported")
			return false, nil
		}
		return false, err
	}
	for i := range apiResources.APIResources {
		if apiResources.APIResources[i].Kind == "DNSEndpoint" {
			return true, nil
		}
	}
	return false, nil
}
