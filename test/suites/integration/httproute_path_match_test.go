package integration

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/aws/aws-sdk-go/service/vpclattice"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = Describe("HTTPRoute path matches", func() {
	var (
		deployment1        *appsv1.Deployment
		deployment2        *appsv1.Deployment
		service1           *v1.Service
		service2           *v1.Service
		pathMatchHttpRoute *gwv1.HTTPRoute
	)

	It("HTTPRoute should support multiple path matches", func() {
		deployment1, service1 = testFramework.NewHttpApp(test.HTTPAppOptions{Name: "path-match-test-v1", Namespace: k8snamespace})
		deployment2, service2 = testFramework.NewHttpApp(test.HTTPAppOptions{Name: "path-match-test-v2", Namespace: k8snamespace})
		pathMatchHttpRoute = testFramework.NewPathMatchHttpRoute(testGateway, []client.Object{service1, service2}, "http",
			"", k8snamespace)

		// Create Kubernetes API Objects
		testFramework.ExpectCreated(ctx,
			pathMatchHttpRoute,
			service1,
			deployment1,
			service2,
			deployment2,
		)
		route, _ := core.NewRoute(pathMatchHttpRoute)
		vpcLatticeService := testFramework.GetVpcLatticeService(ctx, route)

		targetGroupV1 := testFramework.GetHttpTargetGroup(ctx, service1)
		Expect(*targetGroupV1.VpcIdentifier).To(Equal(os.Getenv("CLUSTER_VPC_ID")))
		Expect(*targetGroupV1.Protocol).To(Equal("HTTP"))
		targetsV1 := testFramework.GetTargets(ctx, targetGroupV1, deployment1)
		Expect(*targetGroupV1.Port).To(BeEquivalentTo(80))
		for _, target := range targetsV1 {
			Expect(*target.Port).To(BeEquivalentTo(service1.Spec.Ports[0].TargetPort.IntVal))
			Expect(*target.Status).To(Or(
				Equal(vpclattice.TargetStatusInitial),
				Equal(vpclattice.TargetStatusHealthy),
			))
		}

		targetGroupV2 := testFramework.GetHttpTargetGroup(ctx, service2)
		Expect(*targetGroupV2.VpcIdentifier).To(Equal(os.Getenv("CLUSTER_VPC_ID")))
		Expect(*targetGroupV2.Protocol).To(Equal("HTTP"))
		targetsV2 := testFramework.GetTargets(ctx, targetGroupV2, deployment2)
		Expect(*targetGroupV2.Port).To(BeEquivalentTo(80))
		for _, target := range targetsV2 {
			Expect(*target.Port).To(BeEquivalentTo(service2.Spec.Ports[0].TargetPort.IntVal))
			Expect(*target.Status).To(Or(
				Equal(vpclattice.TargetStatusInitial),
				Equal(vpclattice.TargetStatusHealthy),
			))
		}

		log.Println("Verifying VPC lattice service listeners and rules")
		Eventually(func(g Gomega) {
			listListenerResp, err := testFramework.LatticeClient.ListListenersWithContext(ctx, &vpclattice.ListListenersInput{
				ServiceIdentifier: vpcLatticeService.Id,
			})
			g.Expect(err).To(BeNil())
			g.Expect(len(listListenerResp.Items)).To(BeEquivalentTo(1))
			listener := listListenerResp.Items[0]
			g.Expect(*listener.Port).To(BeEquivalentTo(testGateway.Spec.Listeners[0].Port))
			listenerId := listener.Id
			listRulesResp, err := testFramework.LatticeClient.ListRulesWithContext(ctx, &vpclattice.ListRulesInput{
				ListenerIdentifier: listenerId,
				ServiceIdentifier:  vpcLatticeService.Id,
			})
			nonDefaultRules := lo.Filter(listRulesResp.Items, func(rule *vpclattice.RuleSummary, _ int) bool {
				return rule.IsDefault == nil || *rule.IsDefault == false
			})
			ruleIds := lo.Map(nonDefaultRules, func(rule *vpclattice.RuleSummary, _ int) *string {
				return rule.Id
			})

			g.Expect(len(ruleIds)).To(Equal(2))

			rule0, err := testFramework.LatticeClient.GetRuleWithContext(ctx, &vpclattice.GetRuleInput{
				ServiceIdentifier:  vpcLatticeService.Id,
				ListenerIdentifier: listenerId,
				RuleIdentifier:     ruleIds[0],
			})
			g.Expect(err).To(BeNil())

			rule1, err := testFramework.LatticeClient.GetRuleWithContext(ctx, &vpclattice.GetRuleInput{
				ServiceIdentifier:  vpcLatticeService.Id,
				ListenerIdentifier: listenerId,
				RuleIdentifier:     ruleIds[1],
			})
			httprouteRules := pathMatchHttpRoute.Spec.Rules

			g.Expect(err).To(BeNil())
			retrievedRules := []string{
				*rule0.Match.HttpMatch.PathMatch.Match.Prefix,
				*rule1.Match.HttpMatch.PathMatch.Match.Prefix}
			expectedRules := []string{*httprouteRules[0].Matches[0].Path.Value,
				*httprouteRules[1].Matches[0].Path.Value}
			log.Println("retrievedRules", retrievedRules)
			log.Println("expectedRules", expectedRules)

			g.Expect(retrievedRules).To(
				ContainElements(expectedRules))
		}).WithOffset(1).Should(Succeed())

		log.Println("Verifying traffic")
		dnsName := testFramework.GetVpcLatticeServiceDns(pathMatchHttpRoute.Name, pathMatchHttpRoute.Namespace)

		testFramework.Get(ctx, types.NamespacedName{Name: deployment1.Name, Namespace: deployment1.Namespace}, deployment1)

		//get the pods of deployment1
		pods := testFramework.GetPodsByDeploymentName(deployment1.Name, deployment1.Namespace)
		Expect(len(pods)).To(BeEquivalentTo(1))
		pod := pods[0]

		Eventually(func(g Gomega) {
			cmd := fmt.Sprintf("curl %s/pathmatch0", dnsName)
			stdout, _, err := testFramework.PodExec(pod, cmd)
			g.Expect(err).To(BeNil())
			g.Expect(stdout).To(ContainSubstring("test-v1 handler pod"))
		}).WithTimeout(30 * time.Second).WithOffset(1).Should(Succeed())

		Eventually(func(g Gomega) {
			cmd := fmt.Sprintf("curl %s/pathmatch1", dnsName)
			stdout, _, err := testFramework.PodExec(pod, cmd)
			g.Expect(err).To(BeNil())
			g.Expect(stdout).To(ContainSubstring("test-v2 handler pod"))
		}).WithTimeout(30 * time.Second).WithOffset(1).Should(Succeed())
	})

	AfterEach(func() {
		testFramework.ExpectDeletedThenNotFound(ctx,
			pathMatchHttpRoute,
			deployment1,
			deployment2,
			service1,
			service2,
		)
	})
})
