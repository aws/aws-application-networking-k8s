package integration

import (
	"fmt"
	"log"
	"regexp"
	"time"

	"github.com/aws/aws-sdk-go/service/vpclattice"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = Describe("HTTPRoute header matches", func() {

	var (
		deployment           *appsv1.Deployment
		service              *v1.Service
		headerMatchHttpRoute *gwv1.HTTPRoute
	)

	It("Create a HttpRoute with a header match rule, http traffic should work if pass the correct headers", func() {
		deployment, service = testFramework.NewHttpApp(test.HTTPAppOptions{Name: "header-match-test-v3", Namespace: k8snamespace})
		headerMatchHttpRoute = testFramework.NewHeaderMatchHttpRoute(testGateway, []*v1.Service{service})

		testFramework.ExpectCreated(ctx,
			headerMatchHttpRoute,
			service,
			deployment)
		route, _ := core.NewRoute(headerMatchHttpRoute)
		vpcLatticeService := testFramework.GetVpcLatticeService(ctx, route)

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

		dnsName := testFramework.GetVpcLatticeServiceDns(headerMatchHttpRoute.Name, headerMatchHttpRoute.Namespace)
		testFramework.Get(ctx, types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, deployment)
		pods := testFramework.GetPodsByDeploymentName(deployment.Name, deployment.Namespace)
		Expect(len(pods)).To(BeEquivalentTo(1))
		pod := pods[0]

		// after rules in place, it can take some time for listener rules to fully propagate
		log.Println("Verifying traffic")

		// check correct headers
		Eventually(func(g Gomega) {
			cmd := fmt.Sprintf("curl %s -H \"my-header-name1: my-header-value1\" -H \"my-header-name2: my-header-value2\"", dnsName)
			stdout, _, err := testFramework.PodExec(pod, cmd)
			g.Expect(err).To(BeNil())
			g.Expect(stdout).To(ContainSubstring("test-v3 handler pod"))
		}).WithTimeout(2 * time.Minute).WithOffset(1).Should(Succeed())

		// check incorrect headers
		Eventually(func(g Gomega) {
			invalidCmd := fmt.Sprintf("curl %s -H \"my-header-name1: my-header-value1\" -H \"my-header-name2: value2-invalid\"", dnsName)
			stdout, _, err := testFramework.PodExec(pod, invalidCmd)
			g.Expect(err).To(BeNil())
			g.Expect(stdout).To(ContainSubstring("Not Found"))
		}).WithTimeout(2 * time.Minute).WithOffset(1).Should(Succeed())
	})

	AfterEach(func() {
		testFramework.ExpectDeletedThenNotFound(ctx,
			headerMatchHttpRoute,
			service,
			deployment,
		)
	})
})
