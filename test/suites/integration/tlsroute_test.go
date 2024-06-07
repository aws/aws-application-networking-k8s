package integration

import (
	"fmt"
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

var _ = Describe("TLSRoute test", Ordered, func() {
	var (
		httpsDeployment1 *appsv1.Deployment
		httpsSvc1        *v1.Service
		tlsRoute         *v1alpha2.TLSRoute
	)

	It("Set up k8s resource", func() {
		httpsDeployment1, httpsSvc1 = testFramework.NewHttpsApp(test.HTTPsAppOptions{Name: "my-https-1", Namespace: k8snamespace})
		tlsRoute = testFramework.NewTLSRoute(k8snamespace, testGateway, []v1alpha2.TLSRouteRule{
			{
				BackendRefs: []gwv1.BackendRef{
					{
						BackendObjectReference: v1beta1.BackendObjectReference{
							Name:      v1alpha2.ObjectName(httpsSvc1.Name),
							Namespace: lo.ToPtr(v1beta1.Namespace(httpsSvc1.Namespace)),
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
			httpsSvc1,
			httpsDeployment1,
		)
	})
	It("Verify Lattice resource", func() {
		route, _ := core.NewRoute(tlsRoute)
		vpcLatticeService := testFramework.GetVpcLatticeService(ctx, route)
		fmt.Printf("vpcLatticeService: %v \n", vpcLatticeService)
		tgSummary := testFramework.GetTCPTargetGroup(ctx, httpsSvc1)
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
		targets := testFramework.GetTargets(ctx, tgSummary, httpsDeployment1)
		for _, target := range targets {
			Expect(*target.Port).To(BeEquivalentTo(httpsSvc1.Spec.Ports[0].TargetPort.IntVal))
			Expect(*target.Status).To(Equal(vpclattice.TargetStatusUnavailable))
		}
	})

	It("Verify traffic", func() {
		latticeGeneratedDnsName := testFramework.GetVpcLatticeServiceTLSDns(tlsRoute.Name, tlsRoute.Namespace)
		dnsIP, err := net.DefaultResolver.LookupIP(ctx, "ip4", latticeGeneratedDnsName)
		Expect(err).To(BeNil())
		testFramework.Get(ctx, types.NamespacedName{Name: httpsDeployment1.Name, Namespace: httpsDeployment1.Namespace}, httpsDeployment1)
		//get the pods of httpsDeployment1
		pods := testFramework.GetPodsByDeploymentName(httpsDeployment1.Name, httpsDeployment1.Namespace)
		Expect(len(pods)).To(BeEquivalentTo(1))
		pod := pods[0]
		customDns := string(tlsRoute.Spec.Hostnames[0])
		Eventually(func(g Gomega) {
			cmd := fmt.Sprintf("curl -k https://%s:444 --resolve %s:444:%s", customDns, customDns, dnsIP[0])
			stdout, _, err := testFramework.PodExec(pod, cmd)
			g.Expect(err).To(BeNil())
			g.Expect(stdout).To(ContainSubstring("my-https-1 handler pod"))
		}).WithTimeout(3 * time.Minute).WithOffset(1).Should(Succeed())
	})

	AfterAll(func() {
		testFramework.ExpectDeletedThenNotFound(ctx,
			tlsRoute,
			httpsDeployment1,
			httpsSvc1,
		)
	})
})
