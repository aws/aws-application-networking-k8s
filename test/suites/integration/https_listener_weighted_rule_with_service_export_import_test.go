package integration

import (
	"fmt"
	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"log"
	"os"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = Describe("Test 2 listeners with weighted httproute rules and service export import", Ordered, func() {
	var (
		deployment0    *appsv1.Deployment
		deployment1    *appsv1.Deployment
		service0       *v1.Service
		service1       *v1.Service
		serviceExport1 *anv1alpha1.ServiceExport
		serviceImport1 *anv1alpha1.ServiceImport
		httpRoute      *gwv1.HTTPRoute
	)

	It("Create a weightedRoutingHttpRoute that parentRef to both http and https listeners,"+
		" and this httpRoute BackendRef to one service and one serviceImport, weighted traffic should work for both http and https listeners",
		func() {
			deployment0, service0 = testFramework.NewHttpApp(test.HTTPAppOptions{Name: "service-import-export-test0", Namespace: k8snamespace})
			deployment1, service1 = testFramework.NewHttpApp(test.HTTPAppOptions{Name: "service-import-export-test1", Namespace: k8snamespace})
			serviceExport1, serviceImport1 = testFramework.CreateServiceExportAndServiceImportByService(service1)
			deployments := []*appsv1.Deployment{deployment0, deployment1}
			httpRoute = testFramework.NewWeightedRoutingHttpRoute(testGateway,
				[]*test.ObjectAndWeight{
					{
						Object: service0,
						Weight: 20,
					},
					{
						Object: serviceImport1,
						Weight: 80,
					},
				}, []string{"http", "https"})
			log.Println("httpRoute0 Weight:", *httpRoute.Spec.Rules[0].BackendRefs[0].Weight)
			log.Println("httpRoute1 Weight:", *httpRoute.Spec.Rules[0].BackendRefs[1].Weight)

			testFramework.ExpectCreated(ctx,
				httpRoute,
				deployment0,
				service0,
				deployment1,
				service1,
				serviceExport1,
				serviceImport1,
			)
			route, _ := core.NewRoute(httpRoute)
			vpcLatticeService := testFramework.GetVpcLatticeService(ctx, route)

			log.Println("Verifying Target Groups")
			retrievedTg0 := testFramework.GetTargetGroup(ctx, service0)
			retrievedTg1 := testFramework.GetTargetGroup(ctx, service1)
			for i, retrievedTargetGroupSummary := range []*vpclattice.TargetGroupSummary{retrievedTg0, retrievedTg1} {
				Expect(retrievedTargetGroupSummary).NotTo(BeNil())
				Expect(*retrievedTargetGroupSummary.VpcIdentifier).To(Equal(os.Getenv("CLUSTER_VPC_ID")))
				Expect(*retrievedTargetGroupSummary.Protocol).To(Equal("HTTP"))
				targets := testFramework.GetTargets(ctx, retrievedTargetGroupSummary, deployments[i])
				Expect(len(targets)).To(BeEquivalentTo(1))
				Expect(*retrievedTargetGroupSummary.Port).To(BeEquivalentTo(80))
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
				g.Expect(len(listListenerResp.Items)).To(BeEquivalentTo(2))
				listeners := listListenerResp.Items
				for _, listener := range listeners {
					//listener protocol should be 443 or 80
					g.Expect(*listener.Port).To(Or(BeEquivalentTo(80), BeEquivalentTo(443)))
					listenerId := listener.Id
					listRulesResp, _ := testFramework.LatticeClient.ListRulesWithContext(ctx, &vpclattice.ListRulesInput{
						ListenerIdentifier: listenerId,
						ServiceIdentifier:  vpcLatticeService.Id,
					})
					g.Expect(listRulesResp.Items).To(HaveLen(2)) // one default rule, one Weighted rule
					nonDefaultRule := lo.Filter(listRulesResp.Items, func(rule *vpclattice.RuleSummary, _ int) bool {
						return *rule.IsDefault == false
					})
					Expect(nonDefaultRule).To(HaveLen(1))
					retrievedWeightedTGRule, _ := testFramework.LatticeClient.GetRuleWithContext(ctx, &vpclattice.GetRuleInput{
						ServiceIdentifier:  vpcLatticeService.Id,
						ListenerIdentifier: listenerId,
						RuleIdentifier:     nonDefaultRule[0].Id,
					})
					retrievedWeightedTargetGroup0InRule := retrievedWeightedTGRule.Action.Forward.TargetGroups[0]
					retrievedWeightedTargetGroup1InRule := retrievedWeightedTGRule.Action.Forward.TargetGroups[1]
					g.Expect(*retrievedWeightedTargetGroup0InRule.TargetGroupIdentifier).To(Equal(*retrievedTg0.Id))
					g.Expect(*retrievedWeightedTargetGroup0InRule.Weight).To(BeEquivalentTo(20))
					g.Expect(*retrievedWeightedTargetGroup1InRule.TargetGroupIdentifier).To(Equal(*retrievedTg1.Id))
					g.Expect(*retrievedWeightedTargetGroup1InRule.Weight).To(BeEquivalentTo(80))
				}
			}).Should(Succeed())
			log.Println("Verifying Weighted rule traffic")
			dnsName := testFramework.GetVpcLatticeServiceDns(httpRoute.Name, httpRoute.Namespace)

			pods := testFramework.GetPodsByDeploymentName(deployment0.Name, deployment0.Namespace)
			Expect(len(pods)).To(BeEquivalentTo(1))
			pod := pods[0]

			protocols := []string{"http", "https"}
			for _, protocol := range protocols {
				// just make sure we can reach via both protocols
				var cmd string
				if protocol == "http" {
					cmd = fmt.Sprintf("curl %s", dnsName)
				} else if protocol == "https" {
					cmd = fmt.Sprintf("curl -k https://%s", dnsName)
				} else {
					Fail("Unexpected listener protocol")
				}

				Eventually(func(g Gomega) {
					stdout, _, err := testFramework.PodExec(pod, cmd)
					g.Expect(err).To(BeNil())
					g.Expect(stdout).To(ContainSubstring("handler pod"))
				}).Should(Succeed())
			}
		})

	AfterEach(func() {
		testFramework.ExpectDeletedThenNotFound(ctx,
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
