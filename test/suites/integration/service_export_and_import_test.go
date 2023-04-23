package integration

import (
	"fmt"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"log"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

var _ = Describe("Test service export and Service import", func() {
	It("Create a k8s service, do service export and service import for it, refer this serviceImport in the httproute", func() {
		gateway := testFramework.NewHttpGateway()
		deployment, service := testFramework.NewHttpApp(test.HTTPAppOptions{Name: "service-import-export-test"})
		serviceExport, serviceImport := testFramework.GetServiceExportAndServiceImportByService(service)

		httpRoute := testFramework.NewPathMatchHttpRoute(gateway, []client.Object{service, serviceImport}, "http")
		testFramework.ExpectCreated(ctx,
			gateway,
			httpRoute,
			service,
			serviceExport,
			serviceImport,
			deployment)

		time.Sleep(3 * time.Minute)
		vpcLatticeService := testFramework.GetVpcLatticeService(ctx, httpRoute)
		Expect(*vpcLatticeService.DnsEntry).To(ContainSubstring(latticestore.AWSServiceName(httpRoute.Name, httpRoute.Namespace)))

		tg := testFramework.GetTargetGroup(ctx, service)
		Expect(tg).NotTo(BeNil())
		Expect(*tg.VpcIdentifier).To(Equal(os.Getenv("CLUSTER_VPC_ID")))
		Expect(*tg.Protocol).To(Equal("HTTP"))
		targets := testFramework.GetTargets(ctx, tg, deployment)
		Expect(len(targets)).To(BeEquivalentTo(1))
		Expect(*tg.Port).To(BeEquivalentTo(service.Spec.Ports[0].TargetPort.IntVal))
		for _, target := range targets {
			Expect(*target.Port).To(BeEquivalentTo(service.Spec.Ports[0].TargetPort.IntVal))
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
			g.Expect(listRulesResp.Items).To(HaveLen(3)) // one default rule, one /pathmatch0 rule and one /pathmatch1 rule

		}).Should(Succeed())
		log.Println("Verifying traffic")
		dnsName := testFramework.GetVpcLatticeServiceDns(httpRoute.Name, httpRoute.Namespace)
		pods := testFramework.GetPodsByDeploymentName(deployment.Name, deployment.Namespace)
		Expect(len(pods)).To(BeEquivalentTo(1))
		log.Println("pods[0].Name:", pods[0].Name)

		// /pathmatch0 refer to a `Service` and /pathmatch1 refer to a `ServiceImport`. Both `Service` and `ServiceImport` refer the same targetGroup
		cmd := fmt.Sprintf("curl %s/pathmatch1", dnsName)
		stdout, _, err := testFramework.PodExec(pods[0].Namespace, pods[0].Name, cmd)
		Expect(err).To(BeNil())
		Expect(stdout).To(ContainSubstring("service-import-export-test handler pod"))

		testFramework.ExpectDeleted(ctx,
			gateway,
			httpRoute,
			deployment,
			service,
			serviceExport,
			serviceImport)
		testFramework.EventuallyExpectNotFound(ctx,
			gateway,
			httpRoute,
			deployment,
			service,
			serviceExport,
			serviceImport)
	})
})
