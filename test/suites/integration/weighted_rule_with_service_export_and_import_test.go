package integration

import (
	"fmt"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	"log"
	"os"
	"regexp"
	"strings"
	"time"
)

var _ = Describe("Test weighted httproute rules and service export import", func() {
	It("Create a k8s service, do service export and service import for it, In the weighted httproute, refer the service and serviceImport", func() {
		gateway := testFramework.NewGateway()
		deployment0, service0 := testFramework.NewHttpApp(test.HTTPAppOptions{Name: "service-import-export-test0"})
		deployment1, service1 := testFramework.NewHttpApp(test.HTTPAppOptions{Name: "service-import-export-test1"})
		serviceExport1, serviceImport1 := testFramework.GetServiceExportAndServiceImportByService(service1)
		deployments := []*appsv1.Deployment{deployment0, deployment1}
		httpRoute := testFramework.NewWeightedRoutingHttpRoute(gateway,
			[]*test.ObjectAndWeight{
				{
					Object: service0,
					Weight: 10,
				},
				{
					Object: serviceImport1,
					Weight: 90,
				},
			}, "http")
		log.Println("httpRoute0: ", *httpRoute.Spec.Rules[0].BackendRefs[0].Weight)
		log.Println("httpRoute1: ", *httpRoute.Spec.Rules[0].BackendRefs[1].Weight)

		testFramework.ExpectCreated(ctx,
			gateway,
			httpRoute,
			deployment0,
			service0,
			deployment1,
			service1,
			serviceExport1,
			serviceImport1,
		)

		time.Sleep(3 * time.Minute)
		vpcLatticeService := testFramework.GetVpcLatticeService(ctx, httpRoute)
		Expect(*vpcLatticeService.DnsEntry).To(ContainSubstring(latticestore.AWSServiceName(httpRoute.Name, httpRoute.Namespace)))
		retrievedTg0 := testFramework.GetTargetGroup(ctx, service0)
		retrievedTg1 := testFramework.GetTargetGroup(ctx, service1)
		for i, retrievedTargetGroupSummary := range []*vpclattice.TargetGroupSummary{retrievedTg0, retrievedTg1} {
			Expect(retrievedTargetGroupSummary).NotTo(BeNil())
			Expect(*retrievedTargetGroupSummary.VpcIdentifier).To(Equal(os.Getenv("CLUSTER_VPC_ID")))
			Expect(*retrievedTargetGroupSummary.Protocol).To(Equal("HTTP"))
			targets := testFramework.GetTargets(ctx, retrievedTargetGroupSummary, deployments[i])
			Expect(len(targets)).To(BeEquivalentTo(1))
			Expect(*retrievedTargetGroupSummary.Port).To(BeEquivalentTo(service1.Spec.Ports[0].TargetPort.IntVal))
			for _, target := range targets {
				Expect(*target.Port).To(BeEquivalentTo(service1.Spec.Ports[0].TargetPort.IntVal))
				Expect(*target.Status).To(Or(
					Equal(vpclattice.TargetStatusInitial),
					Equal(vpclattice.TargetStatusHealthy),
				))
			}
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
			g.Expect(listRulesResp.Items).To(HaveLen(2)) // one default rule, one Weighted rule
			weightedRuleNameRegExp := regexp.MustCompile("^k8s-[0-9]+-rule-1$")
			filteredRules := lo.Filter(listRulesResp.Items, func(rule *vpclattice.RuleSummary, _ int) bool {
				return weightedRuleNameRegExp.MatchString(*rule.Name)
			})
			Expect(filteredRules).To(HaveLen(1))
			retrievedWeightedTGRule, err := testFramework.LatticeClient.GetRuleWithContext(ctx, &vpclattice.GetRuleInput{
				ServiceIdentifier:  vpcLatticeService.Id,
				ListenerIdentifier: listenerId,
				RuleIdentifier:     filteredRules[0].Id,
			})
			retrievedWeightedTargetGroup0InRule := retrievedWeightedTGRule.Action.Forward.TargetGroups[0]
			retrievedWeightedTargetGroup1InRule := retrievedWeightedTGRule.Action.Forward.TargetGroups[1]
			Expect(*retrievedWeightedTargetGroup0InRule.TargetGroupIdentifier).To(Equal(*retrievedTg0.Id))
			Expect(*retrievedWeightedTargetGroup0InRule.Weight).To(BeEquivalentTo(10))
			Expect(*retrievedWeightedTargetGroup1InRule.TargetGroupIdentifier).To(Equal(*retrievedTg1.Id))
			Expect(*retrievedWeightedTargetGroup1InRule.Weight).To(BeEquivalentTo(90))
		}).Should(Succeed())
		log.Println("Verifying Weighted rule traffic")
		dnsName := testFramework.GetVpcLatticeServiceDns(httpRoute.Name, httpRoute.Namespace)

		pods := testFramework.GetPodsByDeploymentName(deployment0.Name, deployment0.Namespace)
		Expect(len(pods)).To(BeEquivalentTo(1))
		log.Println("pods[0].Name:", pods[0].Name)
		cmd := fmt.Sprintf("curl %s", dnsName)
		hitTg0 := 0
		hitTg1 := 0
		for i := 0; i < 20; i++ {
			stdout, _, err := testFramework.PodExec(pods[0].Namespace, pods[0].Name, cmd)
			Expect(err).To(BeNil())
			if strings.Contains(stdout, "service-import-export-test0 handler pod") {
				hitTg0++
			} else if strings.Contains(stdout, "service-import-export-test1 handler pod") {
				hitTg1++
			} else {
				Fail("Unexpected response")
			}
		}
		log.Printf("Expect 10 %% of traffic for tg0 hitTg0 :%d \n", hitTg0)
		log.Printf("Expect 90 %% of traffic for tg1 hitTg1: %d  \n", hitTg1)
		Expect(hitTg0).To(BeNumerically("<", hitTg1))

		testFramework.ExpectDeleted(ctx,
			gateway,
			httpRoute,
			deployment0,
			service0,
			deployment1,
			service1,
			serviceExport1,
			serviceImport1,
		)
		testFramework.EventuallyExpectNotFound(ctx,
			gateway,
			httpRoute,
			deployment0,
			service0,
			deployment1,
			service1,
			serviceExport1,
			serviceImport1,
		)
	})
})
