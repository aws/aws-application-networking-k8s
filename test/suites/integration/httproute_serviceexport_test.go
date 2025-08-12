package integration

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
)

var _ = Describe("HTTPRoute Service Export/Import Test", Ordered, func() {
	var (
		httpDeployment *appsv1.Deployment
		httpSvc        *v1.Service
		httpRoute      *gwv1.HTTPRoute
		serviceExport  *anv1alpha1.ServiceExport
		serviceImport  *anv1alpha1.ServiceImport
	)

	It("Create k8s resource", func() {
		// Create an HTTP service and deployment
		httpDeployment, httpSvc = testFramework.NewHttpApp(test.HTTPAppOptions{Name: "my-http-exportedports", Namespace: k8snamespace})
		testFramework.ExpectCreated(ctx, httpDeployment, httpSvc)

		// Create ServiceImport
		serviceImport = testFramework.CreateServiceImport(httpSvc)
		testFramework.ExpectCreated(ctx, serviceImport)

		// Create ServiceExport with exportedPorts field
		serviceExport = &anv1alpha1.ServiceExport{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "application-networking.k8s.aws/v1alpha1",
				Kind:       "ServiceExport",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      httpSvc.Name,
				Namespace: httpSvc.Namespace,
				Annotations: map[string]string{
					"application-networking.k8s.aws/federation": "amazon-vpc-lattice",
				},
			},
			Spec: anv1alpha1.ServiceExportSpec{
				ExportedPorts: []anv1alpha1.ExportedPort{
					{
						Port:      httpSvc.Spec.Ports[0].Port,
						RouteType: "HTTP",
					},
				},
			},
		}
		testFramework.ExpectCreated(ctx, serviceExport)

		httpRoute = testFramework.NewHttpRoute(testGateway, httpSvc, "ServiceImport")
		testFramework.ExpectCreated(ctx, httpRoute)
	})

	It("Verify lattice resource & traffic", func() {
		route := core.NewHTTPRoute(*httpRoute)
		vpcLatticeService := testFramework.GetVpcLatticeService(ctx, route)
		fmt.Printf("vpcLatticeService: %v \n", vpcLatticeService)

		// Get the target group and verify it's configured for HTTP
		tgSummary := testFramework.GetTargetGroupWithProtocol(ctx, httpSvc, "http", "http1")
		tg, err := testFramework.LatticeClient.GetTargetGroup(&vpclattice.GetTargetGroupInput{
			TargetGroupIdentifier: aws.String(*tgSummary.Id),
		})
		Expect(tg).To(Not(BeNil()))
		Expect(err).To(BeNil())
		Expect(*tgSummary.VpcIdentifier).To(Equal(os.Getenv("CLUSTER_VPC_ID")))

		// Verify the target group is configured for HTTP
		Expect(*tgSummary.Protocol).To(Equal("HTTP"))
		Expect(*tg.Config.ProtocolVersion).To(Equal("HTTP1"))

		// Verify targets are registered
		Eventually(func(g Gomega) {
			targets := testFramework.GetTargets(ctx, tgSummary, httpDeployment)
			for _, target := range targets {
				g.Expect(*target.Port).To(BeEquivalentTo(httpSvc.Spec.Ports[0].TargetPort.IntVal))
			}
		}).WithTimeout(3 * time.Minute).WithOffset(1).Should(Succeed())

		log.Println("Verifying traffic")
		dnsName := testFramework.GetVpcLatticeServiceDns(httpRoute.Name, httpRoute.Namespace)
		pods := testFramework.GetPodsByDeploymentName(httpDeployment.Name, httpDeployment.Namespace)
		Expect(len(pods)).To(BeEquivalentTo(1))
		pod := pods[0]

		Eventually(func(g Gomega) {
			cmd := fmt.Sprintf("curl %s", dnsName)
			stdout, _, err := testFramework.PodExec(pod, cmd)
			g.Expect(err).To(BeNil())
			g.Expect(stdout).To(ContainSubstring("handler pod"))
		}).Should(Succeed())
	})

	AfterAll(func() {
		testFramework.ExpectDeletedThenNotFound(ctx,
			httpRoute,
			httpDeployment,
			httpSvc,
			serviceImport,
			serviceExport,
		)
	})
})
