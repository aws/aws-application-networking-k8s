package webhook

import (
	"context"
	"os"

	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"testing"
)

const (
	k8snamespace = "webhook-" + test.K8sNamespace
)

var testFramework *test.Framework
var ctx context.Context

var _ = SynchronizedBeforeSuite(func() {
	vpcId := os.Getenv("CLUSTER_VPC_ID")
	if vpcId == "" {
		Fail("CLUSTER_VPC_ID environment variable must be set to run integration tests")
	}
}, func() {
})

func TestIntegration(t *testing.T) {
	ctx = test.NewContext(t)
	logger := gwlog.NewLogger(zap.DebugLevel)
	testFramework = test.NewFramework(ctx, logger, k8snamespace)
	RegisterFailHandler(Fail)
	RunSpecs(t, "WebhookIntegration")
}

var _ = SynchronizedAfterSuite(func() {}, func() {
})
