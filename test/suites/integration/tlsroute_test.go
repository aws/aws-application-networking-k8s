package integration

import (
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"sigs.k8s.io/gateway-api/apis/v1alpha2"
	"sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
)

var _ = Describe("TLSRoute test", func() {
	var (
		deployment1 *appsv1.Deployment
		service1    *v1.Service
		tlsRoute    *v1alpha2.TLSRoute
	)

	It("create a tlsRoute", func() {
		deployment1, service1 = testFramework.NewHttpsApp(test.HTTPsAppOptions{Name: "my-https-1", Namespace: k8snamespace})
		tlsRoute = testFramework.NewTLSRoute(k8snamespace, testGateway, []v1alpha2.TLSRouteRule{
			{
				BackendRefs: []gwv1.BackendRef{
					{
						BackendObjectReference: v1beta1.BackendObjectReference{
							Name:      v1alpha2.ObjectName(service1.Name),
							Namespace: lo.ToPtr(v1beta1.Namespace(service1.Namespace)),
							Kind:      lo.ToPtr(v1beta1.Kind("Service")),
							Port:      lo.ToPtr(v1beta1.PortNumber(443)),
						},
					},
				},
			},
		})

		// Create Kubernetes API Objects
		testFramework.ExpectCreated(ctx,
			tlsRoute,
			service1,
			deployment1,
		)
		route, _ := core.NewRoute(tlsRoute)
		vpcLatticeService := testFramework.GetVpcLatticeService(ctx, route)
		fmt.Printf("vpcLatticeService: %v \n", vpcLatticeService)

		tgSummary := testFramework.GetTCPTargetGroup(ctx, service1)
		tg, err := testFramework.LatticeClient.GetTargetGroup(&vpclattice.GetTargetGroupInput{
			TargetGroupIdentifier: aws.String(*tgSummary.Id),
		})
		Expect(err).To(BeNil())
		Expect(tg).NotTo(BeNil())
		Expect(*tgSummary.VpcIdentifier).To(Equal(os.Getenv("CLUSTER_VPC_ID")))
		Expect(*tgSummary.Protocol).To(Equal("TCP"))
		if tg.Config.HealthCheck != nil {
			Expect(*tg.Config.HealthCheck.Enabled).To(BeFalse())
		}
		targetsV1 := testFramework.GetTargets(ctx, tgSummary, deployment1)
		Expect(*tgSummary.Port).To(BeEquivalentTo(80))
		for _, target := range targetsV1 {
			Expect(*target.Port).To(BeEquivalentTo(service1.Spec.Ports[0].TargetPort.IntVal))
			Expect(*target.Status).To(Equal(vpclattice.TargetStatusUnavailable))
		}
		log.Println("Verifying traffic")
		dnsName := testFramework.GetVpcLatticeServiceTLSDns(tlsRoute.Name, tlsRoute.Namespace)

		dnsIP, _ := net.LookupIP(dnsName)

		testFramework.Get(ctx, types.NamespacedName{Name: deployment1.Name, Namespace: deployment1.Namespace}, deployment1)

		//get the pods of deployment1
		pods := testFramework.GetPodsByDeploymentName(deployment1.Name, deployment1.Namespace)
		Expect(len(pods)).To(BeEquivalentTo(1))
		pod := pods[0]

		Eventually(func(g Gomega) {
			cmd := fmt.Sprintf("curl -k https://tls.test.com:444 --resolve tls.test.com:444:%s", dnsIP[0])
			stdout, _, err := testFramework.PodExec(pod, cmd)
			g.Expect(err).To(BeNil())
			g.Expect(stdout).To(ContainSubstring("my-https-1 handler pod"))
		}).WithTimeout(30 * time.Second).WithOffset(1).Should(Succeed())

	})

	AfterEach(func() {
		testFramework.ExpectDeletedThenNotFound(ctx,
			tlsRoute,
			deployment1,
			service1,
		)
	})
})
