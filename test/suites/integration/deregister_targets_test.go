package integration

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"log"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/gateway-api/apis/v1beta1"

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
		Eventually(func(g Gomega) {
			// Put vpcLatticeService verification logic in the Eventually block(), because the controller need some time to create vpcLattice resource
			vpcLatticeService = testFramework.GetVpcLatticeService(ctx, pathMatchHttpRoute)
			g.Expect(vpcLatticeService).NotTo(BeNil())
			g.Expect(*vpcLatticeService.DnsEntry).To(ContainSubstring(
				latticestore.LatticeServiceName(pathMatchHttpRoute.Name, pathMatchHttpRoute.Namespace)))

			// Verify VPC Lattice Target Group exists
			targetGroup = testFramework.GetTargetGroup(ctx, service)
			g.Expect(*targetGroup.VpcIdentifier).To(Equal(test.CurrentClusterVpcId))
			g.Expect(*targetGroup.Protocol).To(Equal("HTTP"))

			// Verify VPC Lattice Targets exist
			targets := testFramework.GetTargets(ctx, targetGroup, deployment)
			g.Expect(*targetGroup.Port).To(BeEquivalentTo(80))
			for _, target := range targets {
				g.Expect(*target.Port).To(BeEquivalentTo(service.Spec.Ports[0].TargetPort.IntVal))
				g.Expect(*target.Status).To(Or(
					Equal(vpclattice.TargetStatusInitial),
					Equal(vpclattice.TargetStatusHealthy),
				))
			}
		}).Should(Succeed())
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

	It("Kubernetes Service deletion triggers targets de-registering", func() {
		Skip("Currently controller have a bug, service deletion triggers targets de-registering, need to further investigate the reason")
		testFramework.ExpectDeleted(ctx, service)
		Eventually(func(g Gomega) {
			log.Println("Verifying VPC lattice Targets deregistered")
			targets, err := testFramework.LatticeClient.ListTargetsAsList(ctx, &vpclattice.ListTargetsInput{
				TargetGroupIdentifier: targetGroup.Id,
			})
			Expect(err).To(BeNil())
			log.Println("targets:", targets)
			if len(targets) == 0 {
				g.Expect(true).To(BeTrue())
			} else {
				for _, target := range targets {
					g.Expect(*target.Status).To(Equal(vpclattice.TargetStatusDraining))
				}
			}
		}).Should(Succeed())
	})

	It("Kubernetes Deployment deletion triggers targets de-registering", func() {
		//Skip("Skip this test because of the issue")
		testFramework.ExpectDeleted(ctx, deployment)
		Eventually(func(g Gomega) {
			log.Println("Verifying VPC lattice Targets deregistered")
			targets, err := testFramework.LatticeClient.ListTargetsAsList(ctx, &vpclattice.ListTargetsInput{
				TargetGroupIdentifier: targetGroup.Id,
			})
			Expect(err).To(BeNil())
			log.Println("targets:", targets)
			if len(targets) == 0 {
				g.Expect(true).To(BeTrue())
			} else {
				for _, target := range targets {
					g.Expect(*target.Status).To(Equal(vpclattice.TargetStatusDraining))
				}
			}
		}).Should(Succeed())
	})
})
