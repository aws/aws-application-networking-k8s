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
	"os"
	"time"
)

var _ = Describe("HTTPRoute", func() {
	Context("HTTPRules", Ordered, func() {
		It("httprules should support multiple path matches", func() {
			gateway := testFramework.NewGateway()
			testFramework.ExpectCreated(ctx, gateway)

			deployment1, service1 := testFramework.NewHttpApp(test.HTTPAppOptions{Name: "test-v1"})
			deployment2, service2 := testFramework.NewHttpApp(test.HTTPAppOptions{Name: "test-v2"})
			pathMatchHttpRoute := testFramework.NewPathMatchHttpRoute(gateway, []*v1.Service{service1, service2})

			// Create Kubernetes API Objects
			testFramework.ExpectCreated(ctx,
				pathMatchHttpRoute,
				service1,
				deployment1,
				service2,
				deployment2,
			)
			time.Sleep(1 * time.Minute) //Need some time to wait for VPCLattice resources to be created

			// Verify VPC Lattice Resource
			vpcLatticeService := testFramework.GetVpcLatticeService(ctx, pathMatchHttpRoute)
			Expect(*vpcLatticeService.DnsEntry).To(ContainSubstring(latticestore.AWSServiceName(pathMatchHttpRoute.Name, pathMatchHttpRoute.Namespace)))

			targetGroupV1 := testFramework.GetTargetGroup(ctx, service1)
			Expect(*targetGroupV1.VpcIdentifier).To(Equal(os.Getenv("CLUSTER_VPC_ID")))
			Expect(*targetGroupV1.Protocol).To(Equal("HTTP"))
			targetsV1 := testFramework.GetTargets(ctx, targetGroupV1, deployment1)
			Expect(*targetGroupV1.Port).To(BeEquivalentTo(service1.Spec.Ports[0].TargetPort.IntVal))
			for _, target := range targetsV1 {
				Expect(*target.Port).To(BeEquivalentTo(service1.Spec.Ports[0].TargetPort.IntVal))
				Expect(*target.Status).To(Or(
					Equal(vpclattice.TargetStatusInitial),
					Equal(vpclattice.TargetStatusHealthy),
				))
			}

			targetGroupV2 := testFramework.GetTargetGroup(ctx, service2)
			Expect(*targetGroupV2.VpcIdentifier).To(Equal(os.Getenv("CLUSTER_VPC_ID")))
			Expect(*targetGroupV2.Protocol).To(Equal("HTTP"))
			targetsV2 := testFramework.GetTargets(ctx, targetGroupV2, deployment2)

			Expect(*targetGroupV2.Port).To(BeEquivalentTo(service2.Spec.Ports[0].TargetPort.IntVal))
			for _, target := range targetsV2 {
				Expect(*target.Port).To(BeEquivalentTo(service2.Spec.Ports[0].TargetPort.IntVal))
				Expect(*target.Status).To(Or(
					Equal(vpclattice.TargetStatusInitial),
					Equal(vpclattice.TargetStatusHealthy),
				))
			}

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
			time.Sleep(30 * time.Second) //Need to wait for config propagate to VPC lattice dataplane

			log.Println("Verifying traffic")
			dnsName := testFramework.GetVpcLatticeServiceDns(pathMatchHttpRoute.Name, pathMatchHttpRoute.Namespace)

			testFramework.Get(ctx, types.NamespacedName{Name: deployment1.Name, Namespace: deployment1.Namespace}, deployment1)

			//get the pods of deployment1
			pods := testFramework.GetPodsByDeploymentName(deployment1.Name, deployment1.Namespace)
			Expect(len(pods)).To(BeEquivalentTo(1))
			log.Println("pods[0].Name:", pods[0].Name)

			cmd1 := fmt.Sprintf("curl %s/pathmatch0", dnsName)
			stdout, _, err := testFramework.PodExec(pods[0].Namespace, pods[0].Name, cmd1)
			Expect(err).To(BeNil())
			Expect(stdout).To(ContainSubstring("test-v1 handler pod"))

			cmd2 := fmt.Sprintf("curl %s/pathmatch1", dnsName)
			stdout, _, err = testFramework.PodExec(pods[0].Namespace, pods[0].Name, cmd2)
			Expect(err).To(BeNil())
			Expect(stdout).To(ContainSubstring("test-v2 handler pod"))

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
})
