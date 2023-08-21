package integration

import (
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"os"
	"sigs.k8s.io/gateway-api/apis/v1beta1"
	"sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

var _ = Describe("HTTPRoute Creation", func() {

	var (
		deployment    *appsv1.Deployment
		service       *v1.Service
		serviceExport *v1alpha1.ServiceExport
		serviceImport *v1alpha1.ServiceImport
		httpRoute     *v1beta1.HTTPRoute
	)

	BeforeEach(func() {
		deployment, service = testFramework.NewNginxApp(test.ElasticSearchOptions{
			Name:      "port-test",
			Namespace: k8snamespace,
		})
	})

	Context("Order #1: serviceImport, httpRoute, serviceExport, & service", func() {
		It("creates successfully", func() {
			serviceImport = testFramework.CreateServiceImport(service)
			testFramework.ExpectCreated(ctx, serviceImport)

			httpRoute = testFramework.NewHttpRoute(testGateway, service, "ServiceImport")
			testFramework.ExpectCreated(ctx, httpRoute)

			serviceExport = testFramework.CreateServiceExport(service)
			testFramework.ExpectCreated(ctx, serviceExport)

			testFramework.ExpectCreated(ctx, service, deployment)

			verifyResourceCreation(httpRoute, service)
		})
	})

	Context("Order #2: httpRoute, serviceImport, service, & serviceExport", func() {
		It("creates successfully", func() {
			httpRoute = testFramework.NewHttpRoute(testGateway, service, "ServiceImport")
			testFramework.ExpectCreated(ctx, httpRoute)

			serviceImport = testFramework.CreateServiceImport(service)
			testFramework.ExpectCreated(ctx, serviceImport)

			testFramework.ExpectCreated(ctx, service, deployment)

			serviceExport = testFramework.CreateServiceExport(service)
			testFramework.ExpectCreated(ctx, serviceExport)

			verifyResourceCreation(httpRoute, service)
		})
	})

	Context("Order #3: serviceExport, httpRoute, serviceImport, & service", func() {
		It("creates successfully", func() {
			serviceExport = testFramework.CreateServiceExport(service)
			testFramework.ExpectCreated(ctx, serviceExport)

			httpRoute = testFramework.NewHttpRoute(testGateway, service, "ServiceImport")
			testFramework.ExpectCreated(ctx, httpRoute)

			serviceImport = testFramework.CreateServiceImport(service)
			testFramework.ExpectCreated(ctx, serviceImport)

			testFramework.ExpectCreated(ctx, service, deployment)

			verifyResourceCreation(httpRoute, service)
		})
	})

	AfterEach(func() {
		testFramework.ExpectDeleted(ctx, httpRoute)
		testFramework.SleepForRouteDeletion()
		testFramework.ExpectDeletedThenNotFound(ctx,
			deployment,
			service,
			serviceImport,
			serviceExport,
			httpRoute,
		)
	})
})

func verifyResourceCreation(
	httpRoute *v1beta1.HTTPRoute,
	service *v1.Service,
) {
	_ = testFramework.GetVpcLatticeService(ctx, httpRoute)

	targetGroup := testFramework.GetTargetGroup(ctx, service)
	Expect(*targetGroup.VpcIdentifier).To(Equal(os.Getenv("CLUSTER_VPC_ID")))
	Expect(*targetGroup.Protocol).To(Equal("HTTP"))
}
