package integration

import (
	"fmt"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"log"
	"os"
	"strings"

	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	"github.com/aws/aws-sdk-go/service/vpclattice"
)

var _ = Describe("Test 2 listeners gateway with weighted httproute rules and service export import", func() {
	// Clean up resources in case an assertion failed before cleaning up
	// at the end
	AfterEach(func() {
		testFramework.CleanTestEnvironment(ctx)
	})

	It("Create a gateway with 2 listeners(http and https), create a weightedRoutingHttpRoute that parentRef to both http and https listeners,"+
		" and this httpRoute BackendRef to one service and one serviceImport, weighted traffic should work for both http and https listeners",
		func() {
			gateway := testFramework.NewGateway("", "")
			deployment0, service0 := testFramework.NewHttpApp(test.HTTPAppOptions{Name: "service-import-export-test0"})
			deployment1, service1 := testFramework.NewHttpApp(test.HTTPAppOptions{Name: "service-import-export-test1"})
			serviceExport1, serviceImport1 := testFramework.CreateServiceExportAndServiceImportByService(service1)
			deployments := []*appsv1.Deployment{deployment0, deployment1}
			httpRoute := testFramework.NewWeightedRoutingHttpRoute(gateway,
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
				gateway,
				httpRoute,
				deployment0,
				service0,
				deployment1,
				service1,
				serviceExport1,
				serviceImport1,
			)

			Eventually(func(g Gomega) { // Put lattice resource verification logic in the Eventually() block, because the controller need some time to create all lattice resources

				log.Println("Waiting for controller create vpc lattice resources, and then, will verify latticeService")
				vpcLatticeService := testFramework.GetVpcLatticeService(ctx, httpRoute)
				g.Expect(vpcLatticeService).NotTo(BeNil())
				g.Expect(*vpcLatticeService.DnsEntry).To(ContainSubstring(latticestore.LatticeServiceName(httpRoute.Name, httpRoute.Namespace)))

				log.Println("Verifying VpcServiceNetworkAssociation")
				sn := testFramework.GetServiceNetwork(ctx, gateway)
				associated, err := testFramework.IsVpcAssociateWithServiceNetwork(ctx, test.CurrentClusterVpcId, sn)
				g.Expect(err).To(BeNil())
				g.Expect(associated).To(BeTrue())

				var retrievedTgs [2]*vpclattice.TargetGroupSummary
				log.Println("Verifying Target Groups and All targets healthy")
				for i, service := range []*corev1.Service{service0, service1} {
					retrievedTargetGroupSummary := testFramework.GetTargetGroup(ctx, service)
					retrievedTgs[i] = retrievedTargetGroupSummary
					g.Expect(retrievedTargetGroupSummary).NotTo(BeNil())
					g.Expect(*retrievedTargetGroupSummary.VpcIdentifier).To(Equal(os.Getenv("CLUSTER_VPC_ID")))
					g.Expect(*retrievedTargetGroupSummary.Protocol).To(Equal("HTTP"))
					isAllLatticeTargetsHealthy, err := testFramework.IsAllLatticeTargetsHealthy(ctx, retrievedTargetGroupSummary)
					g.Expect(err).To(BeNil())
					g.Expect(isAllLatticeTargetsHealthy).To(BeTrue())
					targets := testFramework.GetTargets(ctx, retrievedTargetGroupSummary, deployments[i])
					g.Expect(err).To(BeNil())
					g.Expect(len(targets)).To(BeEquivalentTo(1))
					g.Expect(*retrievedTargetGroupSummary.Port).To(BeEquivalentTo(80))
					for _, target := range targets {
						g.Expect(*target.Port).To(BeEquivalentTo(service1.Spec.Ports[0].TargetPort.IntVal))
					}
				}

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
					g.Expect(nonDefaultRule).To(HaveLen(1))
					retrievedWeightedTGRule, _ := testFramework.LatticeClient.GetRuleWithContext(ctx, &vpclattice.GetRuleInput{
						ServiceIdentifier:  vpcLatticeService.Id,
						ListenerIdentifier: listenerId,
						RuleIdentifier:     nonDefaultRule[0].Id,
					})
					retrievedWeightedTargetGroup0InRule := retrievedWeightedTGRule.Action.Forward.TargetGroups[0]
					retrievedWeightedTargetGroup1InRule := retrievedWeightedTGRule.Action.Forward.TargetGroups[1]
					g.Expect(*retrievedWeightedTargetGroup0InRule.TargetGroupIdentifier).To(Equal(*retrievedTgs[0].Id))
					g.Expect(*retrievedWeightedTargetGroup0InRule.Weight).To(BeEquivalentTo(20))
					g.Expect(*retrievedWeightedTargetGroup1InRule.TargetGroupIdentifier).To(Equal(*retrievedTgs[1].Id))
					g.Expect(*retrievedWeightedTargetGroup1InRule.Weight).To(BeEquivalentTo(80))
				}
			}).Should(Succeed())
			Eventually(func(g Gomega) { // Put traffic verification logic in the Eventually() block, because connectivity setup may need some time to fully propagate to vpc lattice dataplane
				log.Println("Verifying Weighted rule traffic")
				dnsName := testFramework.GetVpcLatticeServiceDns(httpRoute.Name, httpRoute.Namespace)
				pods := testFramework.GetPodsByDeploymentName(deployment0.Name, deployment0.Namespace)
				g.Expect(len(pods)).To(BeEquivalentTo(1))
				log.Println("client pod name:", pods[0].Name)
				protocols := []string{"http", "https"}
				for _, protocol := range protocols {

					var cmd string
					if protocol == "http" {
						cmd = fmt.Sprintf("curl %s", dnsName)
					} else if protocol == "https" {
						cmd = fmt.Sprintf("curl -k https://%s", dnsName)
					} else {
						Fail("Unexpected listener protocol")
					}
					hitTg0 := 0
					hitTg1 := 0
					for i := 0; i < 20; i++ {
						stdout, _, err := testFramework.PodExec(pods[0].Namespace, pods[0].Name, cmd, false)
						g.Expect(err).To(BeNil())
						if strings.Contains(stdout, "service-import-export-test0 handler pod") {
							hitTg0++
						} else if strings.Contains(stdout, "service-import-export-test1 handler pod") {
							hitTg1++
						} else {
							Fail(fmt.Sprintf("Unexpected response: %s", stdout))
						}
					}
					log.Printf("Send traffic to %s listener: \n", protocol)
					log.Printf("Expect 20 %% of traffic hit tg0, hitTg0: %d \n", hitTg0)
					log.Printf("Expect 80 %% of traffic hit tg1, hitTg1: %d  \n", hitTg1)
					g.Expect(hitTg0).To(BeNumerically("<", hitTg1))
				}
			}).WithOffset(1).Should(Succeed())

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
