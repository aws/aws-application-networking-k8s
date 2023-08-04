package integration

import (
	"fmt"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"log"
	"regexp"
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

		Eventually(func(g Gomega) { // Put lattice resource verification logic in the Eventually() block, because the controller need some time to create all lattice resources
			log.Println("Waiting for controller create vpc lattice resources, and then, will verify latticeService")
			vpcLatticeService := testFramework.GetVpcLatticeService(ctx, headerMatchHttpRoute)
			g.Expect(vpcLatticeService).NotTo(BeNil())
			g.Expect(*vpcLatticeService.DnsEntry).To(ContainSubstring(latticestore.LatticeServiceName(headerMatchHttpRoute.Name, headerMatchHttpRoute.Namespace)))

			log.Println("Verifying VPC Service Network Association")
			serviceNetwork := testFramework.GetServiceNetwork(ctx, gateway)
			isCurrentVpcAssociateWithServiceNetwork, err := testFramework.IsVpcAssociateWithServiceNetwork(ctx, test.CurrentClusterVpcId, serviceNetwork)
			g.Expect(isCurrentVpcAssociateWithServiceNetwork).To(BeTrue())
			g.Expect(err).To(BeNil())

			log.Println("Verifying all lattice targets are healthy")
			retrievedTg := testFramework.GetTargetGroup(ctx, service3)
			IsAllLatticeTargetsHealthy, err := testFramework.IsAllLatticeTargetsHealthy(ctx, retrievedTg)
			g.Expect(IsAllLatticeTargetsHealthy).To(BeTrue())
			g.Expect(err).To(BeNil())

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
			g.Expect(listRulesResp.Items).To(HaveLen(2)) //1 default rules + 1 newly added header match rule
			filteredRules := lo.Filter(listRulesResp.Items, func(rule *vpclattice.RuleSummary, _ int) bool {
				return headerMatchRuleNameRegExp.MatchString(*rule.Name)
			})
			g.Expect(filteredRules).To(HaveLen(1))
			headerMatchRule, err := testFramework.LatticeClient.GetRuleWithContext(ctx, &vpclattice.GetRuleInput{
				ServiceIdentifier:  vpcLatticeService.Id,
				ListenerIdentifier: listenerId,
				RuleIdentifier:     filteredRules[0].Id,
			})
			g.Expect(err).To(BeNil())
			headerMatches := headerMatchRule.Match.HttpMatch.HeaderMatches
			g.Expect(headerMatches).To(HaveLen(2))
			g.Expect(*headerMatches[0].Name).To(Equal("my-header-name1"))
			g.Expect(*headerMatches[0].Match.Exact).To(Equal("my-header-value1"))
			g.Expect(*headerMatches[1].Name).To(Equal("my-header-name2"))
			g.Expect(*headerMatches[1].Match.Exact).To(Equal("my-header-value2"))
		}).WithOffset(1).Should(Succeed())

		Eventually(func(g Gomega) { // Put traffic verification logic in the Eventually() block, because connectivity setup may need some time to fully propagate to vpc lattice dataplane
			log.Println("Verifying traffic")
			dnsName := testFramework.GetVpcLatticeServiceDns(headerMatchHttpRoute.Name, headerMatchHttpRoute.Namespace)
			testFramework.Get(ctx, types.NamespacedName{Name: deployment3.Name, Namespace: deployment3.Namespace}, deployment3)
			pods := testFramework.GetPodsByDeploymentName(deployment3.Name, deployment3.Namespace)
			g.Expect(len(pods)).To(BeEquivalentTo(1))
			log.Println("pods[0].Name:", pods[0].Name)

			cmd := fmt.Sprintf("curl %s -H \"my-header-name1: my-header-value1\" -H \"my-header-name2: my-header-value2\"", dnsName)
			stdout, _, err := testFramework.PodExec(pods[0].Namespace, pods[0].Name, cmd, true)
			g.Expect(err).To(BeNil())
			g.Expect(stdout).To(ContainSubstring("test-v3 handler pod"))

			invalidCmd := fmt.Sprintf("curl %s -H \"my-header-name1: my-header-value1\" -H \"my-header-name2: value2-invalid\"", dnsName)
			stdout2, _, err2 := testFramework.PodExec(pods[0].Namespace, pods[0].Name, invalidCmd, true)
			g.Expect(err2).To(BeNil())
			g.Expect(stdout2).To(ContainSubstring("Not Found"))
		}).WithOffset(1).Should(Succeed())

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
