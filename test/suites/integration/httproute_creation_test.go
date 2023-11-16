package integration

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"

	"github.com/aws/aws-sdk-go/aws"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

var _ = Describe("HTTPRoute Creation", func() {

	var (
		deployment    *appsv1.Deployment
		service       *v1.Service
		serviceExport *anv1alpha1.ServiceExport
		serviceImport *anv1alpha1.ServiceImport
		httpRoute     *gwv1.HTTPRoute
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

	Context("Order #4: Service, HttpRoute", func() {
		It("Create lattice resources successfully when no Targets available", func() {
			httpRoute = testFramework.NewHttpRoute(testGateway, service, "Service")

			// Override port match to make the BackendRef not be able to find
			// any port causing no available targets to register
			httpRoute.Spec.Rules[0].BackendRefs[0].BackendRef.Port = (*gwv1.PortNumber)(aws.Int32(100))

			testFramework.ExpectCreated(
				ctx,
				httpRoute,
				deployment,
				service,
			)

			testFramework.GetTargetGroup(ctx, service)
			testFramework.GetVpcLatticeService(ctx, core.NewHTTPRoute(gwv1beta1.HTTPRoute(*httpRoute)))
		})
	})

	AfterEach(func() {
		testFramework.ExpectDeletedThenNotFound(ctx,
			httpRoute,
			deployment,
			service,
			serviceImport,
			serviceExport,
		)
	})
})

func verifyResourceCreation(
	httpRoute *gwv1.HTTPRoute,
	service *v1.Service,
) {
	route, _ := core.NewRoute(httpRoute)
	_ = testFramework.GetVpcLatticeService(ctx, route)

	targetGroup := testFramework.GetTargetGroup(ctx, service)
	Expect(*targetGroup.VpcIdentifier).To(Equal(os.Getenv("CLUSTER_VPC_ID")))
	Expect(*targetGroup.Protocol).To(Equal("HTTP"))
}
