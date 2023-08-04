package integration

import (
	"fmt"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"log"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	"github.com/aws/aws-sdk-go/service/vpclattice"
)

const (
	k8snamespace = "non-default"
)

var _ = Describe("HTTPRoute path matches", func() {
	It("HTTPRoute should support multiple path matches", func() {
		gateway := testFramework.NewGateway("", k8snamespace)
		deployment1, service1 := testFramework.NewHttpApp(test.HTTPAppOptions{Name: "test-v1", Namespace: k8snamespace})
		deployment2, service2 := testFramework.NewHttpApp(test.HTTPAppOptions{Name: "test-v2", Namespace: k8snamespace})
		pathMatchHttpRoute := testFramework.NewPathMatchHttpRoute(gateway, []client.Object{service1, service2}, "http",
			"", k8snamespace)

		// Create Kubernetes API Objects
		testFramework.ExpectCreated(ctx,
			gateway,
			pathMatchHttpRoute,
			service1,
			deployment1,
			service2,
			deployment2,
		)
		deployments := []*appsv1.Deployment{deployment1, deployment2}

		Eventually(func(g Gomega) { // Put lattice resource verification logic in the Eventually() block, because the controller need some time to create all lattice resources
			log.Println("Waiting for controller create vpc lattice resources, and then, will verify latticeService")
			vpcLatticeService := testFramework.GetVpcLatticeService(ctx, pathMatchHttpRoute)
			g.Expect(vpcLatticeService).NotTo(BeNil())
			g.Expect(*vpcLatticeService.DnsEntry).To(ContainSubstring(latticestore.LatticeServiceName(pathMatchHttpRoute.Name, pathMatchHttpRoute.Namespace)))

			log.Println("Verifying VPCServiceNetworkAssociation")
			sn := testFramework.GetServiceNetwork(ctx, gateway)
			vpcServiceNetworkAssociation, err := testFramework.IsVpcAssociateWithServiceNetwork(ctx, test.CurrentClusterVpcId, sn)
			g.Expect(vpcServiceNetworkAssociation).To(BeTrue())
			g.Expect(err).To(BeNil())

			log.Println("Verifying Lattice targetGroups and all targets healthy")
			for i, k8sService := range []*v1.Service{service1, service2} {
				targetGroup := testFramework.GetTargetGroup(ctx, k8sService)
				g.Expect(*targetGroup.VpcIdentifier).To(Equal(test.CurrentClusterVpcId))
				g.Expect(*targetGroup.Protocol).To(Equal("HTTP"))
				testFramework.IsAllLatticeTargetsHealthy(ctx, targetGroup)
				targets := testFramework.GetTargets(ctx, targetGroup, deployments[i])
				g.Expect(*targetGroup.Port).To(BeEquivalentTo(80))
				for _, target := range targets {
					g.Expect(*target.Port).To(BeEquivalentTo(service1.Spec.Ports[0].TargetPort.IntVal))
				}
			}

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
		Eventually(func(g Gomega) { // Put traffic verification logic in the Eventually() block, because connectivity setup may need some time to fully propagate to vpc lattice dataplane
			log.Println("Verifying traffic")
			dnsName := testFramework.GetVpcLatticeServiceDns(pathMatchHttpRoute.Name, pathMatchHttpRoute.Namespace)

			testFramework.Get(ctx, types.NamespacedName{Name: deployment1.Name, Namespace: deployment1.Namespace}, deployment1)

			//get the pods of deployment1
			pods := testFramework.GetPodsByDeploymentName(deployment1.Name, deployment1.Namespace)
			g.Expect(len(pods)).To(BeEquivalentTo(1))
			log.Println("pods[0].Name:", pods[0].Name)

			cmd1 := fmt.Sprintf("curl %s/pathmatch0", dnsName)
			stdout, _, err := testFramework.PodExec(pods[0].Namespace, pods[0].Name, cmd1, true)
			g.Expect(err).To(BeNil())
			Expect(stdout).To(ContainSubstring("test-v1 handler pod"))

			cmd2 := fmt.Sprintf("curl %s/pathmatch1", dnsName)
			stdout, _, err = testFramework.PodExec(pods[0].Namespace, pods[0].Name, cmd2, true)
			g.Expect(err).To(BeNil())
			g.Expect(stdout).To(ContainSubstring("test-v2 handler pod"))
		}).WithOffset(1).Should(Succeed())

		testFramework.ExpectDeleted(ctx,
			gateway,
			pathMatchHttpRoute,
			service1,
			deployment1,
			service2,
			deployment2,
		)
		testFramework.EventuallyExpectNotFound(ctx,
			gateway,
			pathMatchHttpRoute,
			service1,
			deployment1,
			service2,
			deployment2)

	})
})
