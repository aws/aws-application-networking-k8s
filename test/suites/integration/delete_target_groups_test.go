package integration

import (
	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-sdk-go/aws"
	"log"
	"time"

	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = Describe("Delete Unused Target Groups", Ordered, func() {
	var (
		deployments        = make([]*appsv1.Deployment, 2)
		services           = make([]*v1.Service, 2)
		targetGroups       = make([]*vpclattice.TargetGroupSummary, 2)
		pathMatchHttpRoute *gwv1.HTTPRoute
	)

	BeforeAll(func() {
		deployments[0], services[0] = testFramework.NewHttpApp(test.HTTPAppOptions{
			Name:      "tg-delete-test-1",
			Namespace: k8snamespace,
		})
		deployments[1], services[1] = testFramework.NewHttpApp(test.HTTPAppOptions{
			Name:      "tg-delete-test-2",
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
			targetGroups[i] = testFramework.GetHttpTargetGroup(ctx, service)
			Expect(*targetGroups[i].Port).To(BeEquivalentTo(80))
			Expect(*targetGroups[i]).To(Not(BeNil()))

			// Verify VPC Lattice Targets exist
			Eventually(func(g Gomega) {
				targets, err := testFramework.LatticeClient.ListTargetsAsList(ctx, &vpclattice.ListTargetsInput{TargetGroupIdentifier: targetGroups[i].Id})
				g.Expect(err).To(BeNil())
				g.Expect(len(targets)).To(BeEquivalentTo(*deployments[i].Spec.Replicas))
				for _, target := range targets {
					g.Expect(*target.Port).To(BeEquivalentTo(service.Spec.Ports[0].TargetPort.IntVal))
					g.Expect(*target.Status).To(Equal(vpclattice.TargetStatusHealthy))
				}
			}).WithTimeout(3 * time.Minute).WithPolling(3 * time.Second).Should(Succeed())

		}
	})

	It("Kubernetes Service deletion deletes target groups", func() {
		testFramework.ExpectDeleted(ctx, services[0])
		verifyTargetGroupDeleted(targetGroups[0])
	})

	It("Kubernetes Deployment deletion triggers targets de-registering", func() {
		testFramework.ExpectDeleted(ctx, deployments[1])
		verifyNoTargetsForTargetGroup(targetGroups[1])
	})

	AfterAll(func() {
		// Lattice targets draining time for test cases in this file is actually unavoidable,
		// because these test cases logic themselves test lattice targets de-registering before delete the httproute
		testFramework.ExpectDeletedThenNotFound(ctx,
			pathMatchHttpRoute,
			services[0],
			deployments[0],
			services[1],
			deployments[1],
		)
	})
})

func verifyTargetGroupDeleted(targetGroup *vpclattice.TargetGroupSummary) {
	Eventually(func(g Gomega) {
		log.Println("Verifying VPC lattice target group deleted ", *targetGroup.Id)
		tg, err := testFramework.LatticeClient.GetTargetGroupWithContext(ctx, &vpclattice.GetTargetGroupInput{
			TargetGroupIdentifier: targetGroup.Id,
		})
		if err != nil && services.IsNotFoundError(err) {
			return
		}

		// showing up as "deleting" is also fine
		if aws.StringValue(tg.Status) == vpclattice.TargetGroupStatusDeleteInProgress {
			return
		}

		g.Expect(true).To(BeFalse())
	}).Should(Succeed())
}

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
