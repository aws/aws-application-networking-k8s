package sharing

import (
	"context"
	anaws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ram"
	"os"

	"github.com/aws/aws-sdk-go/service/vpclattice"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"

	"testing"

	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const (
	k8snamespace = "e2e-test"
)

var k8sTestTags = map[string]*string{
	anaws.TagBase + "TestSuite": aws.String("sharing"),
}
var k8sRamTestTags []*ram.Tag
var testFramework *test.Framework
var ctx context.Context
var testGateway *gwv1.Gateway
var testServiceNetwork *vpclattice.ServiceNetworkSummary
var _ = SynchronizedBeforeSuite(func() {
	for key, value := range k8sTestTags {
		k8sRamTestTags = append(k8sRamTestTags, &ram.Tag{
			Key: aws.String(key),
			Value: value,
		})
	}

	vpcId := os.Getenv("CLUSTER_VPC_ID")
	if vpcId == "" {
		Fail("CLUSTER_VPC_ID environment variable must be set to run integration tests")
	}

	testFramework.ExpectToBeClean(ctx)
	grpcurlRunnerPod := test.NewGrpcurlRunnerPod("grpc-runner", k8snamespace)
	if err := testFramework.Get(ctx, client.ObjectKeyFromObject(grpcurlRunnerPod), testFramework.GrpcurlRunner); err != nil {
		if apierrors.IsNotFound(err) {
			testFramework.ExpectCreated(ctx, grpcurlRunnerPod)
			testFramework.GrpcurlRunner = grpcurlRunnerPod
		}
	}

	// provision gateway, wait for service network association
	testGateway = testFramework.NewGateway("test-gateway", k8snamespace)
	testFramework.ExpectCreated(ctx, testGateway)

	testServiceNetwork = testFramework.GetServiceNetwork(ctx, testGateway)

	testFramework.Log.Infof("Expecting VPC %s and service network %s association", vpcId, *testServiceNetwork.Id)
	Eventually(func(g Gomega) {
		associated, snva, _ := testFramework.IsVpcAssociatedWithServiceNetwork(ctx, vpcId, testServiceNetwork)
		g.Expect(associated).To(BeTrue())
		managed, _ := testFramework.Cloud.IsArnManaged(ctx, *snva.Arn)
		g.Expect(managed).To(BeTrue())
	}).Should(Succeed())

}, func() {
	testGateway = testFramework.NewGateway("test-gateway", k8snamespace)
	testServiceNetwork = testFramework.GetServiceNetwork(ctx, testGateway)
	testFramework.GrpcurlRunner = test.NewGrpcurlRunnerPod("grpc-runner", k8snamespace)
})

func TestIntegration(t *testing.T) {
	ctx = test.NewContext(t)
	logger := gwlog.NewLogger(zap.DebugLevel)
	testFramework = test.NewFramework(ctx, logger, k8snamespace)
	RegisterFailHandler(Fail)
	RunSpecs(t, "Sharing")
}

var _ = SynchronizedAfterSuite(func() {}, func() {
	testFramework.ExpectDeletedThenNotFound(ctx, testGateway, testFramework.GrpcurlRunner)
})
