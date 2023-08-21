package integration

import (
	"context"
	"os"
	"sigs.k8s.io/gateway-api/apis/v1beta1"
	"testing"

	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	k8snamespace = "non-default"
)

var testFramework *test.Framework
var ctx context.Context
var testGateway *v1beta1.Gateway

var _ = BeforeSuite(func() {
	vpcid := os.Getenv("CLUSTER_VPC_ID")
	if vpcid == "" {
		Fail("CLUSTER_VPC_ID environment variable must be set to run integration tests")
	}

	testFramework.ExpectToBeClean(ctx)

	// provision gateway, wait for service network association
	testGateway = testFramework.NewGateway("test-gateway", k8snamespace)
	testFramework.ExpectCreated(ctx, testGateway)

	sn := testFramework.GetServiceNetwork(ctx, testGateway)

	test.Logger(ctx).Infof("Expecting VPC %s and service network %s association", vpcid, *sn.Id)
	Eventually(func(g Gomega) {
		g.Expect(testFramework.IsVpcAssociatedWithServiceNetwork(ctx, vpcid, sn)).To(BeTrue())
	})
})

func TestIntegration(t *testing.T) {
	ctx = test.NewContext(t)
	testFramework = test.NewFramework(ctx, k8snamespace)
	RegisterFailHandler(Fail)
	RunSpecs(t, "Integration")
}

var _ = AfterSuite(func() {
	testFramework.ExpectDeletedThenNotFound(ctx, testGateway)
})
