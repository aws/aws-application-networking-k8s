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

var _ = Describe("Deregister Targets", Ordered, func() {
	var (
		deployments        = make([]*appsv1.Deployment, 2)
		services           = make([]*v1.Service, 2)
		targetGroups       = make([]*vpclattice.TargetGroupSummary, 2)
		pathMatchHttpRoute *v1beta1.HTTPRoute
	)

	BeforeAll(func() {
		deployments[0], services[0] = testFramework.NewHttpApp(test.HTTPAppOptions{
			Name:      "target-deregistration-test-1",
			Namespace: k8snamespace,
		})
		deployments[1], services[1] = testFramework.NewHttpApp(test.HTTPAppOptions{
			Name:      "target-deregistration-test-2",
			Namespace: k8snamespace,
		})
		pathMatchHttpRoute = testFramework.NewPathMatchHttpRoute(
			testGateway, []client.Object{services[0], services[1]}, "http", "", k8snamespace)
		testFramework.ExpectCreated(
			ctx,
			pathMatchHttpRoute,
			services[0],
			deployments[0],
			services[1],
			deployments[1],
		)
		route, _ := core.NewRoute(pathMatchHttpRoute)
		_ = testFramework.GetVpcLatticeService(ctx, route)

		for i, service := range services {
			// Verify VPC Lattice Target Group exists
			targetGroups[i] = testFramework.GetTargetGroup(ctx, service)
			Expect(*targetGroups[i]).To(Not(BeNil()))

			// Verify VPC Lattice Targets exist
			targets := testFramework.GetTargets(ctx, targetGroups[i], deployments[i])
			Expect(*targetGroups[i].Port).To(BeEquivalentTo(80))
			for _, target := range targets {
				Expect(*target.Port).To(BeEquivalentTo(service.Spec.Ports[0].TargetPort.IntVal))
				Expect(*target.Status).To(Or(
					Equal(vpclattice.TargetStatusInitial),
					Equal(vpclattice.TargetStatusHealthy),
				))
			}
		}
	})

	It("Kubernetes Service deletion deregisters targets", func() {
		testFramework.ExpectDeleted(ctx, services[0])
		verifyNoTargetsForTargetGroup(targetGroups[0])
	})

	It("Kubernetes Deployment deletion triggers targets de-registering", func() {
		testFramework.ExpectDeleted(ctx, services[1])
		verifyNoTargetsForTargetGroup(targetGroups[1])
	})

	AfterAll(func() {
		// Lattice targets draining time for test cases in this file is actually unavoidable,
		//because these test cases logic themselves test lattice targets de-registering before delete the httproute
		testFramework.ExpectDeletedThenNotFound(ctx,
			pathMatchHttpRoute,
			services[0],
			deployments[0],
			services[1],
			deployments[1],
		)
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
