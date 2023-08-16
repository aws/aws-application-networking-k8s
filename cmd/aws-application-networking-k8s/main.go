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

	"github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	"github.com/go-logr/zapr"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	"github.com/aws/aws-application-networking-k8s/controllers"
	//+kubebuilder:scaffold:imports
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/external-dns/endpoint"
	gateway_api "sigs.k8s.io/gateway-api/apis/v1beta1"
	mcs_api "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	//+kubebuilder:scaffold:scheme
	utilruntime.Must(gateway_api.AddToScheme(scheme))
	utilruntime.Must(mcs_api.AddToScheme(scheme))
	addEndpointToScheme(scheme)
}

func addEndpointToScheme(scheme *runtime.Scheme) {
	dnsEndpointGV := schema.GroupVersion{
		Group:   "externaldns.k8s.io",
		Version: "v1alpha1",
	}
	scheme.AddKnownTypes(dnsEndpointGV, &endpoint.DNSEndpoint{}, &endpoint.DNSEndpointList{})
	metav1.AddToGroupVersion(scheme, dnsEndpointGV)
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var debug bool

	// setup glog level
	flag.Lookup("logtostderr").Value.Set("true")

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
		"AccoundId", config.AccountID,
		"DefaultServiceNetwork", config.DefaultServiceNetwork,
		"UseLongTgName", config.UseLongTGName,
	)

	cloud, err := aws.NewCloud(log.Named("cloud"))
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
	latticeDataStore := latticestore.NewLatticeDataStoreWithLog(log.Named("datastore"))

	// parent logging scope for all controllers
	ctrlLog := log.Named("controller")

	err = controllers.RegisterPodController(ctrlLog.Named("pod"), mgr)
	if err != nil {
		setupLog.Fatalf("pod controller setup failed: %s", err)
	}

	err = controllers.RegisterServiceController(ctrlLog.Named("service"), cloud, latticeDataStore, finalizerManager, mgr)
	if err != nil {
		setupLog.Fatalf("service controller setup failed: %s", err)
	}

	err = controllers.RegisterGatewayClassController(ctrlLog.Named("gateway-class"), mgr)
	if err != nil {
		setupLog.Fatalf("gateway-class controller setup failed: %s", err)
	}

	err = controllers.RegisterGatewayController(ctrlLog.Named("gateway"), cloud, latticeDataStore, finalizerManager, mgr)
	if err != nil {
		setupLog.Fatalf("gateway controller setup failed: %s", err)
	}

	httpRouteReconciler := controllers.NewHttpRouteReconciler(cloud, mgr.GetClient(), mgr.GetScheme(), mgr.GetEventRecorderFor("httproute"), finalizerManager, latticeDataStore)

	if err = httpRouteReconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "HTTPRoute")
		os.Exit(1)
	}

	serviceImportReconciler := controllers.NewServceImportReconciler(mgr.GetClient(), mgr.GetScheme(),
		mgr.GetEventRecorderFor("ServiceImport"), finalizerManager, latticeDataStore)

	if err = serviceImportReconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ServiceImport")
		os.Exit(1)
	}

	serviceExportReconciler := controllers.NewServiceExportReconciler(cloud, mgr.GetClient(),
		mgr.GetScheme(), mgr.GetEventRecorderFor("serviceExport"), finalizerManager, latticeDataStore)

	if err = serviceExportReconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "serviceExport")
		os.Exit(1)
	}

	go latticestore.GetDefaultLatticeDataStore().ServeIntrospection()

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
