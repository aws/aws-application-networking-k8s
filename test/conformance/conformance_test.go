package conformance_test

import (
	"io/fs"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	clientset "k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/yaml"

	"k8s.io/apimachinery/pkg/util/sets"

	conformance "sigs.k8s.io/gateway-api/conformance"
	conformanceconfig "sigs.k8s.io/gateway-api/conformance/utils/config"
	"sigs.k8s.io/gateway-api/conformance/utils/suite"
	"sigs.k8s.io/gateway-api/pkg/features"

	v1 "sigs.k8s.io/gateway-api/apis/v1"
	"sigs.k8s.io/gateway-api/apis/v1alpha2"
	"sigs.k8s.io/gateway-api/apis/v1alpha3"
	"sigs.k8s.io/gateway-api/apis/v1beta1"
)

type skipEntry struct {
	Name     string `json:"name"`
	Category string `json:"category"`
	Reason   string `json:"reason"`
}

func loadSkipList(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile("skip-tests.yaml")
	require.NoError(t, err, "failed to read skip-tests.yaml")

	var entries []skipEntry
	require.NoError(t, yaml.Unmarshal(data, &entries), "failed to parse skip-tests.yaml")

	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name
	}
	return names
}

func scaledTimeouts() conformanceconfig.TimeoutConfig {
	d := conformanceconfig.DefaultTimeoutConfig()
	m := time.Duration(2)
	return conformanceconfig.TimeoutConfig{
		CreateTimeout:                      m * d.CreateTimeout,
		DeleteTimeout:                      m * d.DeleteTimeout,
		GetTimeout:                         m * d.GetTimeout,
		GatewayMustHaveAddress:             m * d.GatewayMustHaveAddress,
		GatewayMustHaveCondition:           m * d.GatewayMustHaveCondition,
		GatewayStatusMustHaveListeners:     m * d.GatewayStatusMustHaveListeners,
		GatewayListenersMustHaveConditions: m * d.GatewayListenersMustHaveConditions,
		GWCMustBeAccepted:                  m * d.GWCMustBeAccepted,
		HTTPRouteMustNotHaveParents:        m * d.HTTPRouteMustNotHaveParents,
		HTTPRouteMustHaveCondition:         m * d.HTTPRouteMustHaveCondition,
		TLSRouteMustHaveCondition:          m * d.TLSRouteMustHaveCondition,
		RouteMustHaveParents:               m * d.RouteMustHaveParents,
		ManifestFetchTimeout:               m * d.ManifestFetchTimeout,
		MaxTimeToConsistency:               m * d.MaxTimeToConsistency,
		NamespacesMustBeReady:              m * d.NamespacesMustBeReady,
		RequestTimeout:                     m * d.RequestTimeout,
		LatestObservedGenerationSet:        m * d.LatestObservedGenerationSet,
		DefaultTestTimeout:                 m * d.DefaultTestTimeout,
		RequiredConsecutiveSuccesses:        d.RequiredConsecutiveSuccesses,
	}
}

func TestConformance(t *testing.T) {
	log.SetLogger(zap.New(zap.WriteTo(os.Stdout), zap.UseDevMode(true)))

	cfg, err := config.GetConfig()
	require.NoError(t, err, "error loading Kubernetes config")

	c, err := client.New(cfg, client.Options{})
	require.NoError(t, err, "error creating client")

	require.NoError(t, v1alpha3.Install(c.Scheme()))
	require.NoError(t, v1alpha2.Install(c.Scheme()))
	require.NoError(t, v1beta1.Install(c.Scheme()))
	require.NoError(t, v1.Install(c.Scheme()))
	require.NoError(t, apiextensionsv1.AddToScheme(c.Scheme()))

	cs, err := clientset.NewForConfig(cfg)
	require.NoError(t, err, "error creating clientset")

	version := os.Getenv("CONTROLLER_VERSION")
	if version == "" {
		version = "unknown"
	}

	reportOutput := os.Getenv("REPORT_OUTPUT")
	if reportOutput == "" {
		reportOutput = "/report.yaml"
	}

	opts := suite.ConformanceOptions{
		Client:               c,
		ClientOptions:        client.Options{},
		Clientset:            cs,
		RestConfig:           cfg,
		GatewayClassName:     "amazon-vpc-lattice",
		TimeoutConfig:        scaledTimeouts(),
		SkipTests:            loadSkipList(t),
		CleanupBaseResources: true,
		SupportedFeatures:   sets.New(features.SupportGateway, features.SupportHTTPRoute),
		ManifestFS:          []fs.FS{&conformance.Manifests},
		ConformanceProfiles:  sets.New(suite.GatewayHTTPConformanceProfileName),
		Implementation: suite.ParseImplementation(
			"aws",
			"aws-application-networking-k8s",
			"https://github.com/aws/aws-application-networking-k8s",
			version,
			"https://github.com/aws/aws-application-networking-k8s/issues",
		),
		ReportOutputPath: reportOutput,
	}

	conformance.RunConformanceWithOptions(t, opts)
}
