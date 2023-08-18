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

	// would be good to simply wait here until service network has been provisioned and associated
	testGateway = testFramework.NewGateway("test-gateway", k8snamespace)
	testFramework.ExpectCreated(ctx, testGateway)
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
