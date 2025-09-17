package integration

import (
	"time"

	"github.com/aws/aws-sdk-go/service/vpclattice"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
)

const (
	StandaloneAnnotation = "application-networking.k8s.aws/standalone"
	LatticeServiceArn    = "application-networking.k8s.aws/lattice-service-arn"
)

var _ = Describe("Standalone Service Creation", Ordered, func() {

	var (
		deployment *appsv1.Deployment
		service    *corev1.Service
		httpRoute  *gwv1.HTTPRoute
	)

	BeforeEach(func() {
		deployment, service = testFramework.NewNginxApp(test.ElasticSearchOptions{
			Name:      "standalone-test",
			Namespace: k8snamespace,
		})
	})

	Context("HTTPRoute with standalone annotation", func() {
		It("creates VPC Lattice service without service network association", func() {
			// Create HTTPRoute with standalone annotation
			httpRoute = testFramework.NewHttpRoute(testGateway, service, "Service")
			
			// Add standalone annotation to the route
			if httpRoute.Annotations == nil {
				httpRoute.Annotations = make(map[string]string)
			}
			httpRoute.Annotations[StandaloneAnnotation] = "true"

			// Create the resources
			testFramework.ExpectCreated(ctx, httpRoute, deployment, service)

			// Verify VPC Lattice service is created
			route, _ := core.NewRoute(httpRoute)
			vpcLatticeService := testFramework.GetVpcLatticeService(ctx, route)
			Expect(vpcLatticeService).ToNot(BeNil())
			Expect(vpcLatticeService.Arn).ToNot(BeNil())

			// Verify no service network associations exist for this service
			Eventually(func(g Gomega) {
				associations, err := testFramework.LatticeClient.ListServiceNetworkServiceAssociationsAsList(ctx, &vpclattice.ListServiceNetworkServiceAssociationsInput{
					ServiceIdentifier: vpcLatticeService.Id,
				})
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(associations).To(BeEmpty(), "Standalone service should not have service network associations")
			}).WithTimeout(2 * time.Minute).Should(Succeed())

			// Verify service ARN is surfaced in route annotations
			Eventually(func(g Gomega) {
				updatedRoute := &gwv1.HTTPRoute{}
				err := testFramework.Get(ctx, types.NamespacedName{
					Name:      httpRoute.Name,
					Namespace: httpRoute.Namespace,
				}, updatedRoute)
				g.Expect(err).ToNot(HaveOccurred())
				
				annotations := updatedRoute.GetAnnotations()
				g.Expect(annotations).ToNot(BeNil())
				g.Expect(annotations).To(HaveKey(LatticeServiceArn))
				g.Expect(annotations[LatticeServiceArn]).To(Equal(lo.FromPtr(vpcLatticeService.Arn)))
			}).WithTimeout(2 * time.Minute).Should(Succeed())

			// Verify standalone service is accessible and functional
			// Get target group and verify it's created properly
			targetGroup := testFramework.GetTargetGroup(ctx, service)
			Expect(targetGroup).ToNot(BeNil())
			
			// Verify targets are registered
			testFramework.GetTargets(ctx, targetGroup, deployment)
		})
	})

	Context("Gateway with standalone annotation", func() {
		It("creates standalone services for routes referencing the gateway", func() {
			// Create separate resources for this test to avoid cleanup conflicts
			gatewayDeployment, gatewayService := testFramework.NewNginxApp(test.ElasticSearchOptions{
				Name:      "gateway-standalone-test",
				Namespace: k8snamespace,
			})

			// Create a gateway with standalone annotation
			standaloneGateway := testFramework.NewGateway("standalone-gateway", k8snamespace)
			if standaloneGateway.Annotations == nil {
				standaloneGateway.Annotations = make(map[string]string)
			}
			standaloneGateway.Annotations[StandaloneAnnotation] = "true"
			testFramework.ExpectCreated(ctx, standaloneGateway)

			// Create HTTPRoute referencing the standalone gateway
			gatewayHttpRoute := testFramework.NewHttpRoute(standaloneGateway, gatewayService, "Service")

			// Create the resources
			testFramework.ExpectCreated(ctx, gatewayHttpRoute, gatewayDeployment, gatewayService)

			// Verify VPC Lattice service is created
			route, _ := core.NewRoute(gatewayHttpRoute)
			vpcLatticeService := testFramework.GetVpcLatticeService(ctx, route)
			Expect(vpcLatticeService).ToNot(BeNil())

			// Verify no service network associations exist for this service
			Eventually(func(g Gomega) {
				associations, err := testFramework.LatticeClient.ListServiceNetworkServiceAssociationsAsList(ctx, &vpclattice.ListServiceNetworkServiceAssociationsInput{
					ServiceIdentifier: vpcLatticeService.Id,
				})
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(associations).To(BeEmpty(), "Standalone service should not have service network associations")
			}).WithTimeout(2 * time.Minute).Should(Succeed())

			// Verify service ARN is surfaced in route annotations
			Eventually(func(g Gomega) {
				updatedRoute := &gwv1.HTTPRoute{}
				err := testFramework.Get(ctx, types.NamespacedName{
					Name:      gatewayHttpRoute.Name,
					Namespace: gatewayHttpRoute.Namespace,
				}, updatedRoute)
				g.Expect(err).ToNot(HaveOccurred())
				
				annotations := updatedRoute.GetAnnotations()
				g.Expect(annotations).ToNot(BeNil())
				g.Expect(annotations).To(HaveKey(LatticeServiceArn))
				g.Expect(annotations[LatticeServiceArn]).To(Equal(lo.FromPtr(vpcLatticeService.Arn)))
			}).WithTimeout(2 * time.Minute).Should(Succeed())

			// Clean up resources in correct order: route first, then gateway
			testFramework.ExpectDeletedThenNotFound(ctx, gatewayHttpRoute, gatewayDeployment, gatewayService)
			testFramework.ExpectDeletedThenNotFound(ctx, standaloneGateway)
		})
	})

	Context("Route-level annotation precedence", func() {
		It("route-level standalone=true overrides gateway-level standalone=false", func() {
			// Create separate resources for this test to avoid cleanup conflicts
			precedenceDeployment, precedenceService := testFramework.NewNginxApp(test.ElasticSearchOptions{
				Name:      "precedence-test",
				Namespace: k8snamespace,
			})

			// Create a gateway with standalone=false annotation
			precedenceGateway := testFramework.NewGateway("precedence-gateway", k8snamespace)
			if precedenceGateway.Annotations == nil {
				precedenceGateway.Annotations = make(map[string]string)
			}
			precedenceGateway.Annotations[StandaloneAnnotation] = "false"
			testFramework.ExpectCreated(ctx, precedenceGateway)

			// Create HTTPRoute with standalone=true annotation (overriding gateway)
			precedenceHttpRoute := testFramework.NewHttpRoute(precedenceGateway, precedenceService, "Service")
			if precedenceHttpRoute.Annotations == nil {
				precedenceHttpRoute.Annotations = make(map[string]string)
			}
			precedenceHttpRoute.Annotations[StandaloneAnnotation] = "true"

			// Create the resources
			testFramework.ExpectCreated(ctx, precedenceHttpRoute, precedenceDeployment, precedenceService)

			// Verify VPC Lattice service is created as standalone (route annotation takes precedence)
			route, _ := core.NewRoute(precedenceHttpRoute)
			vpcLatticeService := testFramework.GetVpcLatticeService(ctx, route)
			Expect(vpcLatticeService).ToNot(BeNil())

			// Verify no service network associations exist (standalone behavior)
			Eventually(func(g Gomega) {
				associations, err := testFramework.LatticeClient.ListServiceNetworkServiceAssociationsAsList(ctx, &vpclattice.ListServiceNetworkServiceAssociationsInput{
					ServiceIdentifier: vpcLatticeService.Id,
				})
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(associations).To(BeEmpty(), "Route-level standalone=true should override gateway-level standalone=false")
			}).WithTimeout(2 * time.Minute).Should(Succeed())

			// Clean up resources in correct order: route first, then gateway
			testFramework.ExpectDeletedThenNotFound(ctx, precedenceHttpRoute, precedenceDeployment, precedenceService)
			testFramework.ExpectDeletedThenNotFound(ctx, precedenceGateway)
		})
	})

	Context("Backward compatibility", func() {
		It("creates service network associations when no standalone annotations are present", func() {
			// Create HTTPRoute without any standalone annotations
			httpRoute = testFramework.NewHttpRoute(testGateway, service, "Service")

			// Create the resources
			testFramework.ExpectCreated(ctx, httpRoute, deployment, service)

			// Verify VPC Lattice service is created
			route, _ := core.NewRoute(httpRoute)
			vpcLatticeService := testFramework.GetVpcLatticeService(ctx, route)
			Expect(vpcLatticeService).ToNot(BeNil())

			// Verify service network associations exist (default behavior)
			Eventually(func(g Gomega) {
				associations, err := testFramework.LatticeClient.ListServiceNetworkServiceAssociationsAsList(ctx, &vpclattice.ListServiceNetworkServiceAssociationsInput{
					ServiceIdentifier: vpcLatticeService.Id,
				})
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(associations).ToNot(BeEmpty(), "Default behavior should create service network associations")
			}).WithTimeout(2 * time.Minute).Should(Succeed())
		})
	})

	AfterEach(func() {
		// Only clean up if resources weren't already cleaned up in the test
		if httpRoute != nil {
			testFramework.ExpectDeletedThenNotFound(ctx,
				httpRoute,
				deployment,
				service,
			)
		}
	})
})