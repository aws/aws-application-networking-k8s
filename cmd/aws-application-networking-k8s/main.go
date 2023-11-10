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

package main

import (
	"flag"
	"os"

	"github.com/go-logr/zapr"

	"github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/external-dns/endpoint"
	gateway_api_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gateway_api_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/aws/aws-application-networking-k8s/controllers"

	//+kubebuilder:scaffold:imports
	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	//+kubebuilder:scaffold:scheme
	utilruntime.Must(gateway_api_v1alpha2.AddToScheme(scheme))
	utilruntime.Must(gateway_api_v1beta1.AddToScheme(scheme))
	utilruntime.Must(anv1alpha1.AddToScheme(scheme))
	addOptionalCRDs(scheme)
}

func addOptionalCRDs(scheme *runtime.Scheme) {
	dnsEndpoint := schema.GroupVersion{
		Group:   "externaldns.k8s.io",
		Version: "v1alpha1",
	}
	scheme.AddKnownTypes(dnsEndpoint, &endpoint.DNSEndpoint{}, &endpoint.DNSEndpointList{})
	metav1.AddToGroupVersion(scheme, dnsEndpoint)

	groupVersion := schema.GroupVersion{
		Group:   anv1alpha1.GroupName,
		Version: "v1alpha1",
	}

	scheme.AddKnownTypes(groupVersion,
		&anv1alpha1.TargetGroupPolicy{}, &anv1alpha1.TargetGroupPolicyList{},
		&anv1alpha1.AccessLogPolicy{}, &anv1alpha1.AccessLogPolicyList{},
		&anv1alpha1.VpcAssociationPolicy{}, &anv1alpha1.VpcAssociationPolicyList{},
		&anv1alpha1.IAMAuthPolicy{}, &anv1alpha1.IAMAuthPolicyList{})

	metav1.AddToGroupVersion(scheme, groupVersion)
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var debug bool

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&debug, "debug", false, "enable debug mode")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.Parse()

	log := gwlog.NewLogger(debug)
	ctrl.SetLogger(zapr.NewLogger(log.Desugar()).WithName("runtime"))

	setupLog := log.Named("setup")
	err := config.ConfigInit()
	if err != nil {
		setupLog.Fatalf("init config failed: %s", err)
	}
	setupLog.Infow("init config",
		"VpcId", config.VpcID,
		"Region", config.Region,
		"AccountId", config.AccountID,
		"DefaultServiceNetwork", config.DefaultServiceNetwork,
		"ClusterName", config.ClusterName,
	)

	cloud, err := aws.NewCloud(log.Named("cloud"), aws.CloudConfig{
		VpcId:       config.VpcID,
		AccountId:   config.AccountID,
		Region:      config.Region,
		ClusterName: config.ClusterName,
	})
	if err != nil {
		setupLog.Fatal("cloud client setup failed: %s", err)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "amazon-vpc-lattice.io",
	})
	if err != nil {
		setupLog.Fatal("manager setup failed:", err)
	}

	finalizerManager := k8s.NewDefaultFinalizerManager(mgr.GetClient())

	// parent logging scope for all controllers
	ctrlLog := log.Named("controller")

	err = controllers.RegisterPodController(ctrlLog.Named("pod"), mgr)
	if err != nil {
		setupLog.Fatalf("pod controller setup failed: %s", err)
	}

	err = controllers.RegisterServiceController(ctrlLog.Named("service"), cloud, finalizerManager, mgr)
	if err != nil {
		setupLog.Fatalf("service controller setup failed: %s", err)
	}

	err = controllers.RegisterGatewayClassController(ctrlLog.Named("gateway-class"), mgr)
	if err != nil {
		setupLog.Fatalf("gateway-class controller setup failed: %s", err)
	}

	err = controllers.RegisterGatewayController(ctrlLog.Named("gateway"), cloud, finalizerManager, mgr)
	if err != nil {
		setupLog.Fatalf("gateway controller setup failed: %s", err)
	}

	err = controllers.RegisterAllRouteControllers(ctrlLog.Named("route"), cloud, finalizerManager, mgr)
	if err != nil {
		setupLog.Fatalf("route controller setup failed: %s", err)
	}

	err = controllers.RegisterServiceImportController(ctrlLog.Named("service-import"), mgr, finalizerManager)
	if err != nil {
		setupLog.Fatalf("serviceimport controller setup failed: %s", err)
	}

	err = controllers.RegisterServiceExportController(ctrlLog.Named("service-export"), cloud, finalizerManager, mgr)
	if err != nil {
		setupLog.Fatalf("serviceexport controller setup failed: %s", err)
	}

	err = controllers.RegisterAccessLogPolicyController(ctrlLog.Named("access-log-policy"), cloud, finalizerManager, mgr)
	if err != nil {
		setupLog.Fatalf("accesslogpolicy controller setup failed: %s", err)
	}

	err = controllers.RegisterIAMAuthPolicyController(ctrlLog.Named("iam-auth-policy"), mgr, cloud)
	if err != nil {
		setupLog.Fatalf("iam auth policy controller setup failed: %s", err)
	}

	err = controllers.RegisterVpcAssociationPolicyController(ctrlLog.Named("vpc-association-policy"), mgr, cloud)
	if err != nil {
		setupLog.Fatalf("vpc association policy controller setup failed: %s", err)
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}

}
