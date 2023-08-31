package integration

import (
	"log"

	"github.com/aws/aws-sdk-go/service/vpclattice"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
)

var _ = Describe("Deregister Targets", func() {
	var (
		deployment         *appsv1.Deployment
		service            *v1.Service
		pathMatchHttpRoute *v1beta1.HTTPRoute
		targetGroup        *vpclattice.TargetGroupSummary
	)

	BeforeEach(func() {
		deployment, service = testFramework.NewHttpApp(test.HTTPAppOptions{
			Name:      "target-deregistration-test",
			Namespace: k8snamespace,
		})
		pathMatchHttpRoute = testFramework.NewPathMatchHttpRoute(
			testGateway, []client.Object{service}, "http", "", k8snamespace)
		testFramework.ExpectCreated(
			ctx,
			pathMatchHttpRoute,
			service,
			deployment,
		)
		route, _ := core.NewRoute(pathMatchHttpRoute)
		_ = testFramework.GetVpcLatticeService(ctx, route)

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
		testFramework.ExpectDeletedThenNotFound(ctx,
			pathMatchHttpRoute,
			service,
			deployment,
		)
	})

	//It("Kubernetes Service deletion deregisters targets", func() {
	//	Fail("Currently controller have a bug, service deletion does NOT trigger targets de-registering, need to further investigate the root cause")
	//	testFramework.ExpectDeleted(ctx, service)
	//  verifyNoTargetsForTargetGroup(targetGroup)
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
