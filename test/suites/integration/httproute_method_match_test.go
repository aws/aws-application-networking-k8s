package integration

import (
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("HTTPRoute method matches", func() {
	It("HTTPRoute should route by HTTP method", func() {
		gateway := testFramework.NewGateway("", k8snamespace)
		deployment1, getService := testFramework.NewHttpApp(test.HTTPAppOptions{Name: "test-get", Namespace: k8snamespace})
		deployment2, postService := testFramework.NewHttpApp(test.HTTPAppOptions{Name: "test-post", Namespace: k8snamespace})
		methodMatchHttpRoute := testFramework.NewMethodMatchHttpRoute(gateway, getService, postService, "route-by-method", k8snamespace)

		// Create Kubernetes API Objects
		testFramework.ExpectCreated(ctx,
			gateway,
			methodMatchHttpRoute,
			getService,
			deployment1,
			postService,
			deployment2,
		)

		time.Sleep(3 * time.Minute)

		// Verify VPC Lattice Resource
		vpcLatticeService := testFramework.GetVpcLatticeService(ctx, methodMatchHttpRoute)

		Eventually(func(g Gomega) {
			log.Println("Verifying VPC Lattice service listeners and rules")
			listListenerResp, err := testFramework.LatticeClient.ListListenersWithContext(ctx, &vpclattice.ListListenersInput{
				ServiceIdentifier: vpcLatticeService.Id,
			})
			g.Expect(err).To(BeNil())
			g.Expect(len(listListenerResp.Items)).To(BeEquivalentTo(1))
			listener := listListenerResp.Items[0]
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
			g.Expect(err).To(BeNil())

			httprouteRules := methodMatchHttpRoute.Spec.Rules
			retrievedRules := []string{
				*rule0.Match.HttpMatch.Method,
				*rule1.Match.HttpMatch.Method}
			expectedRules := []string{string(*httprouteRules[0].Matches[0].Method),
				string(*httprouteRules[1].Matches[0].Method)}
			log.Println("retrievedRules", retrievedRules)
			log.Println("expectedRules", expectedRules)

			g.Expect(retrievedRules).To(
				ContainElements(expectedRules))
		}).WithOffset(1).Should(Succeed())

		log.Println("Verifying traffic")
		dnsName := testFramework.GetVpcLatticeServiceDns(methodMatchHttpRoute.Name, methodMatchHttpRoute.Namespace)

		testFramework.Get(ctx, types.NamespacedName{Name: deployment1.Name, Namespace: deployment1.Namespace}, deployment1)

		//get the pods of deployment1
		pods := testFramework.GetPodsByDeploymentName(deployment1.Name, deployment1.Namespace)
		cmd1 := fmt.Sprintf("curl -X GET %s", dnsName)
		stdout, _, err := testFramework.PodExec(pods[0].Namespace, pods[0].Name, cmd1, true)
		Expect(err).To(BeNil())
		Expect(stdout).To(ContainSubstring("test-get handler pod"))

		cmd2 := fmt.Sprintf("curl -X POST %s", dnsName)
		stdout, _, err = testFramework.PodExec(pods[0].Namespace, pods[0].Name, cmd2, true)
		Expect(err).To(BeNil())
		Expect(stdout).To(ContainSubstring("test-post handler pod"))

		invalidCmd := fmt.Sprintf("curl -X DELETE %s", dnsName)
		stdout, _, err = testFramework.PodExec(pods[0].Namespace, pods[0].Name, invalidCmd, true)
		Expect(err).To(BeNil())
		Expect(stdout).To(ContainSubstring("Not Found"))

		testFramework.ExpectDeleted(ctx,
			gateway,
			methodMatchHttpRoute,
		)
		time.Sleep(30 * time.Second) // Use a trick to delete httpRoute first and then delete the service and deployment to avoid draining lattice targets
		testFramework.ExpectDeleted(ctx,
			getService,
			deployment1,
			postService,
			deployment2,
		)

		testFramework.EventuallyExpectNotFound(ctx,
			gateway,
			methodMatchHttpRoute,
			getService,
			deployment1,
			postService,
			deployment2)

	})
})
