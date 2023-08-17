package integration

import (
	"context"
	"os"
	"testing"

	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var testFramework *test.Framework
var ctx context.Context

var _ = BeforeSuite(func() {
	vpcid := os.Getenv("CLUSTER_VPC_ID")
	if vpcid == "" {
		Fail("CLUSTER_VPC_ID environment variable must be set to run integration tests")
	}
})

func TestIntegration(t *testing.T) {
	ctx = test.NewContext(t)
	testFramework = test.NewFramework(ctx)
	RegisterFailHandler(Fail)
	RunSpecs(t, "Integration")
}
