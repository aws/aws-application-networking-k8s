package integration

import (
	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"os"
	"sigs.k8s.io/gateway-api/apis/v1beta1"
	"sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

var _ = Describe("Defined Target Ports", func() {
	var (
		deployment        *appsv1.Deployment
		service           *v1.Service
		serviceExport     *v1alpha1.ServiceExport
		httpRoute         *v1beta1.HTTPRoute
		vpcLatticeService *vpclattice.ServiceSummary
		definedPorts      []int64
	)

	BeforeEach(func() {
		deployment, service = testFramework.NewNginxApp(test.ElasticSearchOptions{
			Name:      "http",
			Port:      8080,
			Port2:     8081,
			Namespace: k8snamespace,
		})
		serviceExport = testFramework.CreateServiceExport(service)
		httpRoute = testFramework.NewHttpRoute(testGateway, service, "Service")
	})

	AfterEach(func() {
		testFramework.ExpectDeleted(ctx, httpRoute)
		testFramework.SleepForRouteDeletion()
		testFramework.ExpectDeletedThenNotFound(ctx,
			serviceExport,
			deployment,
			service,
			httpRoute,
		)
	})

	It("take effect when on port annotation of ServiceExport", func() {
		testFramework.ExpectCreated(
			ctx,
			service,
			deployment,
			serviceExport,
		)
		definedPorts = []int64{int64(service.Spec.Ports[0].TargetPort.IntVal)}
		performVerification(service, deployment, definedPorts)
	})

	It("take effect when on HttpRoute BackendRef", func() {
		testFramework.ExpectCreated(
			ctx,
			service,
			deployment,
			httpRoute,
		)
		definedPorts = []int64{int64(service.Spec.Ports[0].TargetPort.IntVal)}
		// Verify VPC Lattice Service exists
		vpcLatticeService = testFramework.GetVpcLatticeService(ctx, httpRoute)
		Expect(*vpcLatticeService.DnsEntry).To(ContainSubstring(latticestore.LatticeServiceName(httpRoute.Name, httpRoute.Namespace)))

		performVerification(service, deployment, definedPorts)
	})
})

func performVerification(service *v1.Service, deployment *appsv1.Deployment, definedPorts []int64) {
	// Verify VPC Lattice Target Group exists
	targetGroup := testFramework.GetTargetGroup(ctx, service)
	Expect(*targetGroup.VpcIdentifier).To(Equal(os.Getenv("CLUSTER_VPC_ID")))
	Expect(*targetGroup.Protocol).To(Equal("HTTP"))

	targets := testFramework.GetTargets(ctx, targetGroup, deployment)
	Expect(*targetGroup.Port).To(BeEquivalentTo(80))
	for _, target := range targets {
		Expect(targetUsesDefinedPort(definedPorts, target)).To(BeTrue())

		Expect(*target.Status).To(Or(
			Equal(vpclattice.TargetStatusInitial),
			Equal(vpclattice.TargetStatusHealthy),
		))
	}
}

func targetUsesDefinedPort(definedPorts []int64, target *vpclattice.TargetSummary) bool {
	for _, definedPort := range definedPorts {
		if *target.Port == definedPort {
			return true
		}
	}
	return false
}
