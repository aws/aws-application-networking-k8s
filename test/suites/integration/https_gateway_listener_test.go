package integration

import (
	"fmt"
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"log"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

var _ = Describe("Test Https gateway listener", func() {
	It("Create a Gateway with https listener, create a HttpRoute that reference it, the traffic should work", func() {
		gateway := testFramework.NewHttpsGateway()

		clientDeployment, clientService := testFramework.NewHttpApp(test.HTTPAppOptions{Name: "client"})
		serverDeployment, serverService := testFramework.NewHttpApp(test.HTTPAppOptions{Name: "https-gw-server"})
		httproute := testFramework.NewPathMatchHttpRoute(gateway, []client.Object{serverService}, "https")

		testFramework.ExpectCreated(ctx,
			gateway,
			httproute,
			clientDeployment,
			clientService,
			serverDeployment,
			serverService)

		time.Sleep(3 * time.Minute)
		vpcLatticeService := testFramework.GetVpcLatticeService(ctx, httproute)
		Expect(*vpcLatticeService.DnsEntry).To(ContainSubstring(latticestore.AWSServiceName(httproute.Name, httproute.Namespace)))
		Eventually(func(g Gomega) {
			log.Println("Verifying VPC lattice service listeners and rules")
			listListenerResp, err := testFramework.LatticeClient.ListListenersWithContext(ctx, &vpclattice.ListListenersInput{
				ServiceIdentifier: vpcLatticeService.Id,
			})
			g.Expect(err).To(BeNil())
			g.Expect(len(listListenerResp.Items)).To(BeEquivalentTo(1))
			retrievedListener := listListenerResp.Items[0]
			g.Expect(*retrievedListener.Port).To(BeEquivalentTo(gateway.Spec.Listeners[0].Port))
			g.Expect(*retrievedListener.Protocol).To(BeEquivalentTo(gateway.Spec.Listeners[0].Protocol))

			listenerId := retrievedListener.Id
			listRulesResp, err := testFramework.LatticeClient.ListRulesWithContext(ctx, &vpclattice.ListRulesInput{
				ListenerIdentifier: listenerId,
				ServiceIdentifier:  vpcLatticeService.Id,
			})
			Expect(listRulesResp.Items).To(HaveLen(2)) //1 default rules + 1 newly added path match route match rule

		}).WithOffset(1).Should(Succeed())

		log.Println("Verifying traffic")
		dnsName := testFramework.GetVpcLatticeServiceDns(httproute.Name, httproute.Namespace)
		pods := testFramework.GetPodsByDeploymentName(clientDeployment.Name, clientDeployment.Namespace)
		Expect(len(pods)).To(BeEquivalentTo(1))
		log.Println("pods[0].Name:", pods[0].Name)

		cmd := fmt.Sprintf("curl -k https://%s/pathmatch0", dnsName)
		stdout, _, err := testFramework.PodExec(pods[0].Namespace, pods[0].Name, cmd)
		Expect(err).To(BeNil())
		Expect(stdout).To(ContainSubstring("https-gw-server handler pod"))
		log.Println("stdout:", stdout)

		testFramework.ExpectDeleted(ctx,
			gateway,
			httproute,
			clientDeployment,
			clientService,
			serverDeployment,
			serverService)
		testFramework.EventuallyExpectNotFound(ctx,
			gateway,
			httproute,
			clientDeployment,
			clientService,
			serverDeployment,
			serverService)
	})
})
