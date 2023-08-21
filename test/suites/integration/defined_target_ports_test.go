package integration

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"os"
	"sigs.k8s.io/gateway-api/apis/v1beta1"
	"sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
	"time"

	"github.com/aws/aws-application-networking-k8s/pkg/latticestore"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	"github.com/aws/aws-sdk-go/service/vpclattice"
)

var _ = Describe("Defined Target Ports", func() {
	var (
		gateway           *v1beta1.Gateway
		deployment        *appsv1.Deployment
		service           *v1.Service
		serviceExport     *v1alpha1.ServiceExport
		serviceImport     *v1alpha1.ServiceImport
		httpRoute         *v1beta1.HTTPRoute
		vpcLatticeService *vpclattice.ServiceSummary
		targetGroup       *vpclattice.TargetGroupSummary
		definedPorts      []int64
	)

	const definedPort = 82

	BeforeEach(func() {
		gateway = testFramework.NewGateway("test-gateway", k8snamespace)
		deployment, service = testFramework.NewNginxApp(test.ElasticSearchOptions{
			Name:      "e2e-ports-test",
			Namespace: k8snamespace,
		})
	})

	AfterEach(func() {
		testFramework.CleanTestEnvironment(ctx)
	})

	It("Port Annotation on Service Export", func() {

		serviceExport = testFramework.CreateServiceExport(service)
		serviceImport = testFramework.CreateServiceImport(service)
		httpRoute = testFramework.NewHttpRoute(gateway, service, "ServiceImport")
		testFramework.ExpectCreated(
			ctx,
			gateway,
			serviceExport,
			serviceImport,
			service,
			deployment,
			httpRoute,
		)
		definedPorts = []int64{int64(service.Spec.Ports[0].Port), int64(service.Spec.Ports[1].Port)}
		// Verify VPC Lattice Service exists
		vpcLatticeService = testFramework.GetVpcLatticeService(ctx, httpRoute)
		Expect(*vpcLatticeService.DnsEntry).To(ContainSubstring(latticestore.LatticeServiceName(httpRoute.Name, httpRoute.Namespace)))

		performVerification(targetGroup, service, deployment, definedPorts)
	})

	It("Modify service port", func() {

		serviceExport = testFramework.CreateServiceExport(service)
		serviceImport = testFramework.CreateServiceImport(service)
		httpRoute = testFramework.NewHttpRoute(gateway, service, "Service")
		testFramework.ExpectCreated(
			ctx,
			gateway,
			service,
			deployment,
			httpRoute,
		)
		definedPorts = []int64{int64(service.Spec.Ports[0].Port), int64(service.Spec.Ports[1].Port)}
		// Verify VPC Lattice Service exists
		vpcLatticeService = testFramework.GetVpcLatticeService(ctx, httpRoute)
		Expect(*vpcLatticeService.DnsEntry).To(ContainSubstring(latticestore.LatticeServiceName(httpRoute.Name, httpRoute.Namespace)))

		performVerification(targetGroup, service, deployment, definedPorts)

		definedPorts = []int64{int64(service.Spec.Ports[0].Port), int64(definedPort)}
		testFramework.Get(ctx, types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, service)
		service.Spec.Ports[1].Port = definedPort
		service.Spec.Ports[1].TargetPort = intstr.FromInt(definedPort)
		err := testFramework.Update(ctx, service)
		Expect(err).To(BeNil())
		testFramework.Get(ctx, types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, service)

		targetGroup = testFramework.GetTargetGroup(ctx, service)
		Expect(*targetGroup.VpcIdentifier).To(Equal(os.Getenv("CLUSTER_VPC_ID")))
		Expect(*targetGroup.Protocol).To(Equal("HTTP"))

		var verifiedIps []string
		podIps, targets := testFramework.GetAllTargets(ctx, targetGroup, deployment)
		Eventually(func(g Gomega) {
			podIps, targets = testFramework.GetAllTargets(ctx, targetGroup, deployment)
			for _, target := range targets {
				if lo.Contains(verifiedIps, *target.Id) {
					continue
				} else if targetUsesDefinedPort(definedPorts, target) {
					verifiedIps = append(verifiedIps, *target.Id)

					// Health checks are completed on port 80 currently, once this is configurable we can check for healthy
					g.Expect(*target.Status).To(Or(
						Equal(vpclattice.TargetStatusInitial),
						Equal(vpclattice.TargetStatusDraining),
						Equal(vpclattice.TargetStatusUnhealthy),
					))
				}
			}
			g.Expect(podIps).Should(HaveLen(len(verifiedIps)))
		}).WithPolling(10 * time.Second).WithTimeout(5 * time.Minute).Should(Succeed())
	})

	It("BackendRef Ports registered only", func() {

		serviceExport = testFramework.CreateServiceExport(service)
		serviceImport = testFramework.CreateServiceImport(service)
		httpRoute = testFramework.NewHttpRoute(gateway, service, "Service")
		testFramework.ExpectCreated(
			ctx,
			gateway,
			service,
			deployment,
			httpRoute,
		)
		//time.Sleep(resourceCreationWaitTime)
		definedPorts = []int64{int64(service.Spec.Ports[0].Port), int64(service.Spec.Ports[1].Port)}
		// Verify VPC Lattice Service exists
		vpcLatticeService = testFramework.GetVpcLatticeService(ctx, httpRoute)
		Expect(*vpcLatticeService.DnsEntry).To(ContainSubstring(latticestore.LatticeServiceName(httpRoute.Name, httpRoute.Namespace)))

		performVerification(targetGroup, service, deployment, definedPorts)
	})
})

func performVerification(targetGroup *vpclattice.TargetGroupSummary, service *v1.Service, deployment *appsv1.Deployment, definedPorts []int64) {
	// Verify VPC Lattice Target Group exists
	targetGroup = testFramework.GetTargetGroup(ctx, service)
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
