package integration

import (
	"os"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/utils"

	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = Describe("Defined Target Ports", Ordered, func() {
	var (
		deployment        *appsv1.Deployment
		service           *v1.Service
		serviceExport     *anv1alpha1.ServiceExport
		httpRoute         *gwv1.HTTPRoute
		vpcLatticeService *vpclattice.ServiceSummary
		definedPorts      []int64
	)

	BeforeAll(func() {
		deployment, service = testFramework.NewNginxApp(test.ElasticSearchOptions{
			Name:      "target-port-definition",
			Port:      8080,
			Port2:     8081,
			Namespace: k8snamespace,
		})
		serviceExport = testFramework.CreateServiceExport(service)
		httpRoute = testFramework.NewHttpRoute(testGateway, service, "Service")

		definedPorts = []int64{int64(service.Spec.Ports[0].TargetPort.IntVal)}
		testFramework.ExpectCreated(
			ctx,
			service,
			deployment,
			httpRoute,
			serviceExport,
		)
	})

	AfterAll(func() {
		testFramework.ExpectDeletedThenNotFound(ctx,
			httpRoute,
			serviceExport,
			deployment,
			service,
		)
	})

	It("take effect when on port annotation of ServiceExport", func() {
		performVerification(service, deployment, definedPorts)
	})

	It("take effect when on HttpRoute BackendRef", func() {
		// Verify VPC Lattice Service exists
		route, _ := core.NewRoute(httpRoute)
		vpcLatticeService = testFramework.GetVpcLatticeService(ctx, route)
		lsn := utils.LatticeServiceName(route.Name(), route.Namespace())
		Expect(*vpcLatticeService.DnsEntry).To(ContainSubstring(lsn))

		performVerification(service, deployment, definedPorts)
	})
})

func performVerification(service *v1.Service, deployment *appsv1.Deployment, definedPorts []int64) {
	// Verify VPC Lattice Target Group exists
	targetGroup := testFramework.GetHttpTargetGroup(ctx, service)
	Expect(*targetGroup.VpcIdentifier).To(Equal(os.Getenv("CLUSTER_VPC_ID")))
	Expect(*targetGroup.Protocol).To(Equal("HTTP"))
	Expect(*targetGroup.Port).To(BeEquivalentTo(80))
	targets, err := testFramework.LatticeClient.ListTargetsAsList(ctx, &vpclattice.ListTargetsInput{TargetGroupIdentifier: targetGroup.Id})
	Expect(err).To(BeNil())
	for _, target := range targets {
		Expect(targetUsesDefinedPort(definedPorts, target)).To(BeTrue())
		Expect(*target.Status).To(Or(
			Equal(vpclattice.TargetStatusInitial),
			Equal(vpclattice.TargetStatusHealthy),
			Equal(vpclattice.TargetStatusUnused),
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
