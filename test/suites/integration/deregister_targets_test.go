package integration

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"log"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/gateway-api/apis/v1beta1"
	"time"

	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	"github.com/aws/aws-sdk-go/service/vpclattice"
)

var _ = Describe("Deregister Targets", func() {
	var (
		gateway            *v1beta1.Gateway
		deployment         *appsv1.Deployment
		service            *v1.Service
		pathMatchHttpRoute *v1beta1.HTTPRoute
		vpcLatticeService  *vpclattice.ServiceSummary
		targetGroup        *vpclattice.TargetGroupSummary
	)

	BeforeEach(func() {
		gateway = testFramework.NewGateway("", k8snamespace)
		deployment, service = testFramework.NewHttpApp(test.HTTPAppOptions{
			Name:      "target-deregistration-test",
			Namespace: k8snamespace,
		})
		pathMatchHttpRoute = testFramework.NewPathMatchHttpRoute(
			gateway, []client.Object{service}, "http", "", k8snamespace)
		testFramework.ExpectCreated(
			ctx,
			gateway,
			pathMatchHttpRoute,
			service,
			deployment,
		)
		time.Sleep(3 * time.Minute) // Wait for creation of VPCLattice resources

		// Verify VPC Lattice Service exists
		vpcLatticeService = testFramework.GetVpcLatticeService(ctx, pathMatchHttpRoute)
		Expect(*vpcLatticeService.DnsEntry).To(ContainSubstring(
			latticestore.AWSServiceName(pathMatchHttpRoute.Name, pathMatchHttpRoute.Namespace)))

		// Verify VPC Lattice Target Group exists
		targetGroup = testFramework.GetTargetGroup(ctx, service)
		Expect(*targetGroup.VpcIdentifier).To(Equal(os.Getenv("CLUSTER_VPC_ID")))
		Expect(*targetGroup.Protocol).To(Equal("HTTP"))

		// Verify VPC Lattice Targets exist
		targets := testFramework.GetTargets(ctx, targetGroup, deployment)
		Expect(*targetGroup.Port).To(BeEquivalentTo(80))
		for _, target := range targets {
			Expect(*target.Port).To(BeEquivalentTo(service.Spec.Ports[0].TargetPort.IntVal))
			Expect(*target.Status).To(Or(
				Equal(vpclattice.TargetStatusInitial),
				Equal(vpclattice.TargetStatusHealthy),
			))
		}
	})

	AfterEach(func() {
		testFramework.CleanTestEnvironment(ctx)
		testFramework.EventuallyExpectNotFound(
			ctx,
			gateway,
			pathMatchHttpRoute,
			service,
			deployment,
		)
	})

	It("Kubernetes Service deletion deregisters targets", func() {
		testFramework.ExpectDeleted(ctx, service)
		Eventually(func(g Gomega) {
			log.Println("Verifying VPC lattice Targets deregistered")
			targets := testFramework.GetTargets(ctx, targetGroup, deployment)
			Expect(len(targets) == 0)
		}).WithTimeout(5*time.Minute + 10*time.Second)
	})

	It("Kubernetes Deployment deletion deregisters targets", func() {
		testFramework.ExpectDeleted(ctx, deployment)
		Eventually(func(g Gomega) {
			log.Println("Verifying VPC lattice Targets deregistered")
			targets := testFramework.GetTargets(ctx, targetGroup, deployment)
			Expect(len(targets) == 0)
		}).WithTimeout(5*time.Minute + 10*time.Second)
	})
})
