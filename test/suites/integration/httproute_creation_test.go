package integration

import (
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

var _ = Describe("HTTPRoute Creation", func() {

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
		testFramework.ExpectCreated(ctx, gateway)

		deployment, service = testFramework.NewNginxApp(test.ElasticSearchOptions{
			Name:      "port-test",
			Namespace: k8snamespace,
		})
	})

	Context("Order #1: serviceImport, httpRoute, serviceExport, & service", func() {
		It("creates successfully", func() {
			serviceImport = testFramework.CreateServiceImport(service)
			testFramework.ExpectCreated(ctx, serviceImport)

			httpRoute = testFramework.NewHttpRoute(gateway, service, "ServiceImport")
			testFramework.ExpectCreated(ctx, httpRoute)

			serviceExport = testFramework.CreateServiceExport(service)
			testFramework.ExpectCreated(ctx, serviceExport)

			testFramework.ExpectCreated(ctx, service, deployment)

			verifyResourceCreation(vpcLatticeService, httpRoute, targetGroup, service)
		})
	})

	Context("Order #2: httpRoute, serviceImport, service, & serviceExport", func() {
		It("creates successfully", func() {
			httpRoute = testFramework.NewHttpRoute(gateway, service, "ServiceImport")
			testFramework.ExpectCreated(ctx, httpRoute)

			serviceImport = testFramework.CreateServiceImport(service)
			testFramework.ExpectCreated(ctx, serviceImport)

			testFramework.ExpectCreated(ctx, service, deployment)

			serviceExport = testFramework.CreateServiceExport(service)
			testFramework.ExpectCreated(ctx, serviceExport)

			verifyResourceCreation(vpcLatticeService, httpRoute, targetGroup, service)
		})
	})

	Context("Order #3: serviceExport, httpRoute, serviceImport, & service", func() {
		It("creates successfully", func() {
			serviceExport = testFramework.CreateServiceExport(service)
			testFramework.ExpectCreated(ctx, serviceExport)

			httpRoute = testFramework.NewHttpRoute(gateway, service, "ServiceImport")
			testFramework.ExpectCreated(ctx, httpRoute)

			serviceImport = testFramework.CreateServiceImport(service)
			testFramework.ExpectCreated(ctx, serviceImport)

			testFramework.ExpectCreated(ctx, service, deployment)

			verifyResourceCreation(vpcLatticeService, httpRoute, targetGroup, service)
		})
	})

	AfterEach(func() {
		testFramework.CleanTestEnvironment(ctx)
	})
})

func verifyResourceCreation(
	vpcLatticeService *vpclattice.ServiceSummary,
	httpRoute *v1beta1.HTTPRoute,
	targetGroup *vpclattice.TargetGroupSummary,
	service *v1.Service,
) {
	time.Sleep(3 * time.Minute) // Allow time for Lattice resources to be created

	vpcLatticeService = testFramework.GetVpcLatticeService(ctx, httpRoute)
	Expect(*vpcLatticeService.DnsEntry).To(ContainSubstring(latticestore.LatticeServiceName(httpRoute.Name, httpRoute.Namespace)))

	targetGroup = testFramework.GetTargetGroup(ctx, service)
	Expect(*targetGroup.VpcIdentifier).To(Equal(os.Getenv("CLUSTER_VPC_ID")))
	Expect(*targetGroup.Protocol).To(Equal("HTTP"))
}
