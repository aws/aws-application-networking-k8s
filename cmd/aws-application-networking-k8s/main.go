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
	"fmt"
	"os"

	"github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-sdk-go/aws/session"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/aws/aws-application-networking-k8s/controllers"
	//+kubebuilder:scaffold:imports

	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/external-dns/endpoint"
	gateway_api "sigs.k8s.io/gateway-api/apis/v1beta1"
	mcs_api "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

type controllerConfig struct {
	region                string
	vpcID                 string
	accountID             string
	defaultServiceNetwork string
	logLevel              string
	useLongTGName         bool
}

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
	var lgcConfig controllerConfig

	errs := initConfig(&lgcConfig)
	if len(errs) > 0 {
		for _, err := range errs {
			setupLog.Error(err, "unable to initialize config")
		}
		os.Exit(1)
	}

	// setup glog level
	flag.Lookup("logtostderr").Value.Set("true")
	flag.Lookup("v").Value.Set(lgcConfig.logLevel)

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	cloud, err := aws.NewCloud(lgcConfig.region, lgcConfig.accountID, lgcConfig.defaultServiceNetwork, lgcConfig.vpcID, lgcConfig.useLongTGName)

	if err != nil {
		setupLog.Error(err, "unable to initialize AWS cloud")
		os.Exit(1)
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
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}
	finalizerManager := k8s.NewDefaultFinalizerManager(mgr.GetClient(), ctrl.Log)
	latticeDataStore := latticestore.NewLatticeDataStore()

	if err = (&controllers.PodReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Pod")
		os.Exit(1)
	}

	serviceReconciler := controllers.NewServiceReconciler(mgr.GetClient(), mgr.GetScheme(),
		mgr.GetEventRecorderFor("service"), finalizerManager, latticeDataStore, cloud)

	if err = serviceReconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Service")
		os.Exit(1)
	}
	gwClassReconciler := controllers.NewGatewayGlassReconciler(mgr.GetClient(),
		mgr.GetScheme())

	if err = gwClassReconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "GatewayClass")
		os.Exit(1)
	}

	gwReconciler := controllers.NewGatewayReconciler(mgr.GetClient(),
		mgr.GetScheme(), mgr.GetEventRecorderFor("gateway"), gwClassReconciler, finalizerManager,
		latticeDataStore, cloud, lgcConfig.accountID, lgcConfig.defaultServiceNetwork)

	if err = gwReconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Gateway")
		os.Exit(1)
	}

	httpRouteReconciler := controllers.NewHttpRouteReconciler(cloud, mgr.GetClient(),
		mgr.GetScheme(), mgr.GetEventRecorderFor("httproute"), gwReconciler, gwClassReconciler, finalizerManager,
		latticeDataStore, lgcConfig.defaultServiceNetwork)

	if err = httpRouteReconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "HTTPRoute")
		os.Exit(1)
	}

	gwReconciler.UpdateGatewayReconciler(httpRouteReconciler)

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

const (
	defaultLogLevel                 = "Info"
	UnknownInput                    = ""
	NO_DEFAULT_SERVICE_NETWORK      = "NO_DEFAULT_SERVICE_NETWORK"
	REGION                          = "REGION"
	CLUSTER_VPC_ID                  = "CLUSTER_VPC_ID"
	CLUSTER_LOCAL_GATEWAY           = "CLUSTER_LOCAL_GATEWAY"
	AWS_ACCOUNT_ID                  = "AWS_ACCOUNT_ID"
	TARGET_GROUP_NAME_LEN_MODE      = "TARGET_GROUP_NAME_LEN_MODE"
	GATEWAY_API_CONTROLLER_LOGLEVEL = "GATEWAY_API_CONTROLLER_LOGLEVEL"
)

func initConfig(lgc *controllerConfig) []error {
	sess, _ := session.NewSession()
	metadata := aws.NewEC2Metadata(sess)
	errs := make([]error, 0)
	var err error

	// CLUSTER_VPC_ID
	lgc.vpcID = os.Getenv(CLUSTER_VPC_ID)
	if lgc.vpcID == "" {
		lgc.vpcID, err = metadata.VpcID()
		if err != nil {
			errs = append(errs, fmt.Errorf("vpcId is not specified: %s", err))
		}
	}

	// REGION
	lgc.region = os.Getenv(REGION)
	if lgc.region == "" {
		lgc.region, err = metadata.Region()
		if err != nil {
			errs = append(errs, fmt.Errorf("region is not specified: %s", err))
		}
	}

	// AWS_ACCOUNT_ID
	lgc.accountID = os.Getenv(AWS_ACCOUNT_ID)
	if lgc.accountID == "" {
		lgc.accountID, err = metadata.AccountId()
		if err != nil {
			errs = append(errs, fmt.Errorf("account is not specified: %s", err))
		}
	}

	// CLUSTER_LOCAL_GATEWAY
	lgc.defaultServiceNetwork = os.Getenv(CLUSTER_LOCAL_GATEWAY)

	// TARGET_GROUP_NAME_LEN_MODE
	tgNameLengthMode := os.Getenv(TARGET_GROUP_NAME_LEN_MODE)

	if tgNameLengthMode == "long" {
		lgc.useLongTGName = true
	} else {
		lgc.useLongTGName = false
	}

	return errs
}
