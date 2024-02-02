package integration

import (
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = Describe("HTTPRoute method matches", func() {

	var (
		methodMatchHttpRoute *gwv1.HTTPRoute
		deployment1          *appsv1.Deployment
		getService           *corev1.Service
		deployment2          *appsv1.Deployment
		postService          *corev1.Service
	)

	BeforeEach(func() {
		deployment1, getService = testFramework.NewHttpApp(test.HTTPAppOptions{Name: "test-get", Namespace: k8snamespace})
		deployment2, postService = testFramework.NewHttpApp(test.HTTPAppOptions{Name: "test-post", Namespace: k8snamespace})
		methodMatchHttpRoute = testFramework.NewMethodMatchHttpRoute(testGateway, getService, postService, "route-by-method", k8snamespace)

		// Create Kubernetes API Objects
		testFramework.ExpectCreated(ctx,
			methodMatchHttpRoute,
			getService,
			deployment1,
			postService,
			deployment2,
		)
	})

	It("HTTPRoute should route by HTTP method", func() {
		route, _ := core.NewRoute(methodMatchHttpRoute)
		vpcLatticeService := testFramework.GetVpcLatticeService(ctx, route)

		log.Println("Verifying VPC Lattice service listeners and rules")
		Eventually(func(g Gomega) {
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
		pod := pods[0]

		Eventually(func(g Gomega) {
			cmd := fmt.Sprintf("curl -X GET %s", dnsName)
			stdout, _, err := testFramework.PodExec(pod, cmd)
			g.Expect(err).To(BeNil())
			g.Expect(stdout).To(ContainSubstring("test-get handler pod"))
		}).WithTimeout(60 * time.Second).WithOffset(1).Should(Succeed())

		Eventually(func(g Gomega) {
			cmd := fmt.Sprintf("curl -X POST %s", dnsName)
			stdout, _, err := testFramework.PodExec(pod, cmd)
			g.Expect(err).To(BeNil())
			g.Expect(stdout).To(ContainSubstring("test-post handler pod"))
		}).WithTimeout(30 * time.Second).WithOffset(1).Should(Succeed())

		Eventually(func(g Gomega) {
			invalidCmd := fmt.Sprintf("curl -X DELETE %s", dnsName)
			stdout, _, err := testFramework.PodExec(pod, invalidCmd)
			g.Expect(err).To(BeNil())
			g.Expect(stdout).To(ContainSubstring("Not Found"))
		}).WithTimeout(30 * time.Second).WithOffset(1).Should(Succeed())
	})

	AfterEach(func() {
		testFramework.ExpectDeletedThenNotFound(ctx,
			methodMatchHttpRoute,
			deployment1,
			deployment2,
			getService,
			postService,
		)
	})
})
