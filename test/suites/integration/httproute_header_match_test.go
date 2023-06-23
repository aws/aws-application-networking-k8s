package integration

import (
	"fmt"
	"log"
	"regexp"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	"github.com/aws/aws-sdk-go/service/vpclattice"
)

var _ = Describe("HTTPRoute header matches", func() {
	It("Create a HttpRoute with a header match rule, http traffic should work if pass the correct headers", func() {
		gateway := testFramework.NewGateway("", "")

		deployment3, service3 := testFramework.NewHttpApp(test.HTTPAppOptions{Name: "test-v3"})
		headerMatchHttpRoute := testFramework.NewHeaderMatchHttpRoute(gateway, []*v1.Service{service3})

		testFramework.ExpectCreated(ctx,
			gateway,
			headerMatchHttpRoute,
			service3,
			deployment3)

		time.Sleep(3 * time.Minute)
		vpcLatticeService := testFramework.GetVpcLatticeService(ctx, headerMatchHttpRoute)
		Expect(*vpcLatticeService.DnsEntry).To(ContainSubstring(latticestore.LatticeServiceName(headerMatchHttpRoute.Name, headerMatchHttpRoute.Namespace)))
		Eventually(func(g Gomega) {
			log.Println("Verifying VPC lattice service listeners and rules")
			listListenerResp, err := testFramework.LatticeClient.ListListenersWithContext(ctx, &vpclattice.ListListenersInput{
				ServiceIdentifier: vpcLatticeService.Id,
			})
			g.Expect(err).To(BeNil())
			g.Expect(len(listListenerResp.Items)).To(BeEquivalentTo(1))
			listener := listListenerResp.Items[0]
			g.Expect(*listener.Port).To(BeEquivalentTo(gateway.Spec.Listeners[0].Port))
			listenerId := listener.Id
			listRulesResp, err := testFramework.LatticeClient.ListRulesWithContext(ctx, &vpclattice.ListRulesInput{
				ListenerIdentifier: listenerId,
				ServiceIdentifier:  vpcLatticeService.Id,
			})

			headerMatchRuleNameRegExp := regexp.MustCompile("^k8s-[0-9]+-rule-1$")
			Expect(listRulesResp.Items).To(HaveLen(2)) //1 default rules + 1 newly added header match rule
			filteredRules := lo.Filter(listRulesResp.Items, func(rule *vpclattice.RuleSummary, _ int) bool {
				return headerMatchRuleNameRegExp.MatchString(*rule.Name)
			})
			Expect(filteredRules).To(HaveLen(1))
			headerMatchRule, err := testFramework.LatticeClient.GetRuleWithContext(ctx, &vpclattice.GetRuleInput{
				ServiceIdentifier:  vpcLatticeService.Id,
				ListenerIdentifier: listenerId,
				RuleIdentifier:     filteredRules[0].Id,
			})
			Expect(err).To(BeNil())
			headerMatches := headerMatchRule.Match.HttpMatch.HeaderMatches
			Expect(headerMatches).To(HaveLen(2))
			Expect(*headerMatches[0].Name).To(Equal("my-header-name1"))
			Expect(*headerMatches[0].Match.Exact).To(Equal("my-header-value1"))
			Expect(*headerMatches[1].Name).To(Equal("my-header-name2"))
			Expect(*headerMatches[1].Match.Exact).To(Equal("my-header-value2"))
		}).WithOffset(1).Should(Succeed())

		log.Println("Verifying traffic")
		dnsName := testFramework.GetVpcLatticeServiceDns(headerMatchHttpRoute.Name, headerMatchHttpRoute.Namespace)
		testFramework.Get(ctx, types.NamespacedName{Name: deployment3.Name, Namespace: deployment3.Namespace}, deployment3)
		pods := testFramework.GetPodsByDeploymentName(deployment3.Name, deployment3.Namespace)
		Expect(len(pods)).To(BeEquivalentTo(1))
		log.Println("pods[0].Name:", pods[0].Name)

		cmd := fmt.Sprintf("curl %s -H \"my-header-name1: my-header-value1\" -H \"my-header-name2: my-header-value2\"", dnsName)
		stdout, _, err := testFramework.PodExec(pods[0].Namespace, pods[0].Name, cmd, true)
		Expect(err).To(BeNil())
		Expect(stdout).To(ContainSubstring("test-v3 handler pod"))

		invalidCmd := fmt.Sprintf("curl %s -H \"my-header-name1: my-header-value1\" -H \"my-header-name2: value2-invalid\"", dnsName)
		stdout2, _, err2 := testFramework.PodExec(pods[0].Namespace, pods[0].Name, invalidCmd, true)
		Expect(err2).To(BeNil())
		Expect(stdout2).To(ContainSubstring("Not Found"))

		testFramework.ExpectDeleted(ctx,
			gateway,
			headerMatchHttpRoute,
			deployment3,
			service3)
		testFramework.EventuallyExpectNotFound(ctx,
			gateway,
			headerMatchHttpRoute,
			deployment3,
			service3)
	})
})
