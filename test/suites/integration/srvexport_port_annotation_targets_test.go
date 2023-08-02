package integration

import (
	"log"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/gateway-api/apis/v1beta1"
	"sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	"github.com/aws/aws-sdk-go/service/vpclattice"
)

var _ = Describe("Port Annotations Targets", func() {
	var (
		gateway           *v1beta1.Gateway
		deployment        *appsv1.Deployment
		service           *v1.Service
		serviceExport     *v1alpha1.ServiceExport
		serviceImport     *v1alpha1.ServiceImport
		httpRoute         *v1beta1.HTTPRoute
		vpcLatticeService *vpclattice.ServiceSummary
		targetGroup       *vpclattice.TargetGroupSummary
	)

	BeforeEach(func() {
		gateway = testFramework.NewGateway("test-gateway", k8snamespace)
		deployment, service = testFramework.NewElasticeApp(test.ElasticSearchOptions{
			Name:      "port-test",
			Namespace: k8snamespace,
		})
		serviceExport = testFramework.CreateServiceExport(service)
		serviceImport = testFramework.CreateServiceImport(service)
		httpRoute = testFramework.NewHttpRoute(gateway, service)
		testFramework.ExpectCreated(
			ctx,
			gateway,
			serviceExport,
			serviceImport,
			service,
			deployment,
			httpRoute,
		)
		Eventually(func(g Gomega) {
			// Put vpcLatticeService verification logic in the Eventually block(), because the controller need some time to create vpcLattice resource
			vpcLatticeService = testFramework.GetVpcLatticeService(ctx, httpRoute)
			g.Expect(vpcLatticeService).NotTo(BeNil())
			g.Expect(*vpcLatticeService.DnsEntry).To(ContainSubstring(latticestore.LatticeServiceName(httpRoute.Name, httpRoute.Namespace)))
		}).Should(Succeed())

		// Verify VPC Lattice Target Group exists
		targetGroup = testFramework.GetTargetGroup(ctx, service)
		Expect(*targetGroup.VpcIdentifier).To(Equal(os.Getenv("CLUSTER_VPC_ID")))
		Expect(*targetGroup.Protocol).To(Equal("HTTP"))
	})

	AfterEach(func() {
		testFramework.DeleteAllTestCasesCreatedK8sResourceAndVpcLatticeResource(ctx)
		testFramework.EventuallyExpectNotFound(
			ctx,
			gateway,
			serviceExport,
			serviceImport,
			service,
			deployment,
			httpRoute,
		)
	})

	It("Port Annotaion on Service Export", func() {

		targets := testFramework.GetTargets(ctx, targetGroup, deployment)
		Expect(*targetGroup.Port).To(BeEquivalentTo(80))
		log.Println("Verifying Targets are only craeted for the port defined in Port Annotaion in ServiceExport")
		for _, target := range targets {
			Expect(*target.Port).To(BeEquivalentTo(service.Spec.Ports[0].Port))
			Expect(*target.Status).To(Or(
				Equal(vpclattice.TargetStatusInitial),
				Equal(vpclattice.TargetStatusHealthy),
			))
			log.Println("Target:", target)
		}

		testFramework.ExpectDeleted(ctx, service)
		Eventually(func(g Gomega) {
			log.Println("Verifying Targets are only craeted for the port defined in Port Annotaion in ServiceExport")
			targets := testFramework.GetTargets(ctx, targetGroup, deployment)
			g.Expect(targets).To(HaveLen(0))
		}).WithTimeout(5*time.Minute + 30*time.Second).Should(Succeed())
	})
})
