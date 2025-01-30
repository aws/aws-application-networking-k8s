package webhook

import (
	"context"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"os"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"testing"
)

const (
	k8snamespace = "webhook-" + test.K8sNamespace
)

var testFramework *test.Framework
var ctx context.Context
var testGateway *gwv1.Gateway
var testServiceNetwork *vpclattice.ServiceNetworkSummary

var _ = SynchronizedBeforeSuite(func() {
	vpcId := os.Getenv("CLUSTER_VPC_ID")
	if vpcId == "" {
		Fail("CLUSTER_VPC_ID environment variable must be set to run integration tests")
	}

	// provision gateway, wait for service network association
	testGateway = testFramework.NewGateway("test-gateway", k8snamespace)
	testFramework.ExpectCreated(ctx, testGateway)

	testServiceNetwork = testFramework.GetServiceNetwork(ctx, testGateway)

	testFramework.Log.Infof(ctx, "Expecting VPC %s and service network %s association", vpcId, *testServiceNetwork.Id)
	Eventually(func(g Gomega) {
		associated, _, _ := testFramework.IsVpcAssociatedWithServiceNetwork(ctx, vpcId, testServiceNetwork)
		g.Expect(associated).To(BeTrue())
	}).Should(Succeed())
}, func() {
	testGateway = testFramework.NewGateway("test-gateway", k8snamespace)
	testServiceNetwork = testFramework.GetServiceNetwork(ctx, testGateway)
})

func TestIntegration(t *testing.T) {
	ctx = test.NewContext(t)
	logger := gwlog.NewLogger(zap.DebugLevel)
	testFramework = test.NewFramework(ctx, logger, k8snamespace)
	RegisterFailHandler(Fail)
	RunSpecs(t, "WebhookIntegration")
}

var _ = SynchronizedAfterSuite(func() {}, func() {
	testFramework.ExpectDeletedThenNotFound(ctx, testGateway)
})
