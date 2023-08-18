package integration

import (
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"log"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/gateway-api/apis/v1beta1"
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
			vpcLatticeService = testFramework.GetVpcLatticeService(ctx, pathMatchHttpRoute)
			g.Expect(vpcLatticeService).NotTo(BeNil())
			g.Expect(*vpcLatticeService.DnsEntry).To(ContainSubstring(
				latticestore.LatticeServiceName(pathMatchHttpRoute.Name, pathMatchHttpRoute.Namespace)))
		}).Should(Succeed())
		// Verify VPC Lattice Target Group exists
		targetGroup = testFramework.GetTargetGroup(ctx, service)
		Expect(*targetGroup.VpcIdentifier).To(Equal(test.CurrentClusterVpcId))
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
		// Lattice targets draining time for test cases in this file is actually unavoidable,
		//because these test cases logic themselves test lattice targets de-registering before delete the httproute
		testFramework.CleanTestEnvironment(ctx)
	})

	//It("Kubernetes Service deletion triggers targets de-registering", func() {
	//	Fail("Currently controller have a bug, service deletion do NOT triggers targets de-registering, need to further investigate the root cause")
	//	testFramework.ExpectDeleted(ctx, service)
	//	verifyNoTargetsForTargetGroup(targetGroup)
	//})

	It("Kubernetes Deployment deletion triggers targets de-registering", func() {
		testFramework.ExpectDeleted(ctx, deployment)
		verifyNoTargetsForTargetGroup(targetGroup)
	})
})

func verifyNoTargetsForTargetGroup(targetGroup *vpclattice.TargetGroupSummary) {
	Eventually(func(g Gomega) {
		log.Println("Verifying VPC lattice Targets deregistered")
		targets, err := testFramework.LatticeClient.ListTargetsAsList(ctx, &vpclattice.ListTargetsInput{
			TargetGroupIdentifier: targetGroup.Id,
		})
		g.Expect(err).To(BeNil())
		log.Println("targets:", targets)
		for _, target := range targets {
			g.Expect(*target.Status).To(Equal(vpclattice.TargetStatusDraining))
		}
	}).Should(Succeed())
}
