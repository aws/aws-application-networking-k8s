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
	StandaloneTransitionAnnotation = "application-networking.k8s.aws/standalone"
	LatticeServiceArnAnnotation    = "application-networking.k8s.aws/lattice-service-arn"
)

var _ = Describe("Standalone Service Transition Scenarios", Ordered, func() {

	Context("Transition from standalone to service network mode", func() {
		var (
			deployment *appsv1.Deployment
			service    *corev1.Service
			httpRoute  *gwv1.HTTPRoute
		)

		BeforeEach(func() {
			deployment, service = testFramework.NewNginxApp(test.ElasticSearchOptions{
				Name:      "standalone-to-network-test",
				Namespace: k8snamespace,
			})
		})

		It("should transition from standalone to service network mode by removing annotation", func() {
			By("Creating HTTPRoute with standalone annotation")
			httpRoute = testFramework.NewHttpRoute(testGateway, service, "Service")

			// Add standalone annotation to the route
			if httpRoute.Annotations == nil {
				httpRoute.Annotations = make(map[string]string)
			}
			httpRoute.Annotations[StandaloneTransitionAnnotation] = "true"

			// Create the resources
			testFramework.ExpectCreated(ctx, httpRoute, deployment, service)

			// Verify VPC Lattice service is created as standalone
			route, _ := core.NewRoute(httpRoute)
			vpcLatticeService := testFramework.GetVpcLatticeService(ctx, route)
			Expect(vpcLatticeService).ToNot(BeNil())
			Expect(vpcLatticeService.Arn).ToNot(BeNil())

			// Verify no service network associations exist initially (standalone mode)
			Eventually(func(g Gomega) {
				associations, err := testFramework.LatticeClient.ListServiceNetworkServiceAssociationsAsList(ctx, &vpclattice.ListServiceNetworkServiceAssociationsInput{
					ServiceIdentifier: vpcLatticeService.Id,
				})
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(associations).To(BeEmpty(), "Service should initially be standalone with no service network associations")
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
				g.Expect(annotations).To(HaveKey(LatticeServiceArnAnnotation))
				g.Expect(annotations[LatticeServiceArnAnnotation]).To(Equal(lo.FromPtr(vpcLatticeService.Arn)))
			}).WithTimeout(2 * time.Minute).Should(Succeed())

			By("Removing standalone annotation to transition to service network mode")
			Eventually(func(g Gomega) {
				latestRoute := &gwv1.HTTPRoute{}
				err := testFramework.Get(ctx, types.NamespacedName{
					Name:      httpRoute.Name,
					Namespace: httpRoute.Namespace,
				}, latestRoute)
				g.Expect(err).ToNot(HaveOccurred())

				// Remove the standalone annotation
				if latestRoute.Annotations != nil {
					delete(latestRoute.Annotations, StandaloneTransitionAnnotation)
					err = testFramework.Update(ctx, latestRoute)
					g.Expect(err).ToNot(HaveOccurred())
				}
			}).WithTimeout(30 * time.Second).Should(Succeed())

			// Allow time for controller to process the annotation change
			time.Sleep(15 * time.Second)

			By("Verifying service network associations are created after annotation removal")
			Eventually(func(g Gomega) {
				associations, err := testFramework.LatticeClient.ListServiceNetworkServiceAssociationsAsList(ctx, &vpclattice.ListServiceNetworkServiceAssociationsInput{
					ServiceIdentifier: vpcLatticeService.Id,
				})
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(associations).ToNot(BeEmpty(), "Service should have service network associations after removing standalone annotation")

				// Verify at least one association is active
				activeAssociations := lo.Filter(associations, func(assoc *vpclattice.ServiceNetworkServiceAssociationSummary, _ int) bool {
					return lo.FromPtr(assoc.Status) == vpclattice.ServiceNetworkServiceAssociationStatusActive
				})
				g.Expect(activeAssociations).ToNot(BeEmpty(), "At least one service network association should be active")
			}).WithTimeout(3 * time.Minute).Should(Succeed())

			By("Verifying service remains functional during transition")
			// Get target group and verify it's still created properly
			targetGroup := testFramework.GetTargetGroup(ctx, service)
			Expect(targetGroup).ToNot(BeNil())

			// Verify targets are still registered
			testFramework.GetTargets(ctx, targetGroup, deployment)

			By("Verifying service ARN annotation is still present after transition")
			Eventually(func(g Gomega) {
				updatedRoute := &gwv1.HTTPRoute{}
				err := testFramework.Get(ctx, types.NamespacedName{
					Name:      httpRoute.Name,
					Namespace: httpRoute.Namespace,
				}, updatedRoute)
				g.Expect(err).ToNot(HaveOccurred())

				annotations := updatedRoute.GetAnnotations()
				g.Expect(annotations).ToNot(BeNil())
				g.Expect(annotations).To(HaveKey(LatticeServiceArnAnnotation))
				g.Expect(annotations[LatticeServiceArnAnnotation]).To(Equal(lo.FromPtr(vpcLatticeService.Arn)))
			}).WithTimeout(2 * time.Minute).Should(Succeed())
		})

		AfterEach(func() {
			testFramework.ExpectDeletedThenNotFound(ctx, httpRoute, deployment, service)
		})
	})

	Context("Transition from service network to standalone mode", func() {
		var (
			deployment *appsv1.Deployment
			service    *corev1.Service
			httpRoute  *gwv1.HTTPRoute
		)

		BeforeEach(func() {
			deployment, service = testFramework.NewNginxApp(test.ElasticSearchOptions{
				Name:      "network-to-standalone-test",
				Namespace: k8snamespace,
			})
		})

		It("should transition from service network to standalone mode by adding annotation", func() {
			By("Creating HTTPRoute without standalone annotation (default service network mode)")
			httpRoute = testFramework.NewHttpRoute(testGateway, service, "Service")

			// Create the resources
			testFramework.ExpectCreated(ctx, httpRoute, deployment, service)

			// Verify VPC Lattice service is created
			route, _ := core.NewRoute(httpRoute)
			vpcLatticeService := testFramework.GetVpcLatticeService(ctx, route)
			Expect(vpcLatticeService).ToNot(BeNil())
			Expect(vpcLatticeService.Arn).ToNot(BeNil())

			// Verify service network associations exist initially (default behavior)
			Eventually(func(g Gomega) {
				associations, err := testFramework.LatticeClient.ListServiceNetworkServiceAssociationsAsList(ctx, &vpclattice.ListServiceNetworkServiceAssociationsInput{
					ServiceIdentifier: vpcLatticeService.Id,
				})
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(associations).ToNot(BeEmpty(), "Service should initially have service network associations")

				// Verify at least one association is active
				activeAssociations := lo.Filter(associations, func(assoc *vpclattice.ServiceNetworkServiceAssociationSummary, _ int) bool {
					return lo.FromPtr(assoc.Status) == vpclattice.ServiceNetworkServiceAssociationStatusActive
				})
				g.Expect(activeAssociations).ToNot(BeEmpty(), "At least one service network association should be active initially")
			}).WithTimeout(2 * time.Minute).Should(Succeed())

			By("Adding standalone annotation to transition to standalone mode")
			Eventually(func(g Gomega) {
				latestRoute := &gwv1.HTTPRoute{}
				err := testFramework.Get(ctx, types.NamespacedName{
					Name:      httpRoute.Name,
					Namespace: httpRoute.Namespace,
				}, latestRoute)
				g.Expect(err).ToNot(HaveOccurred())

				// Add the standalone annotation
				if latestRoute.Annotations == nil {
					latestRoute.Annotations = make(map[string]string)
				}
				latestRoute.Annotations[StandaloneTransitionAnnotation] = "true"

				err = testFramework.Update(ctx, latestRoute)
				g.Expect(err).ToNot(HaveOccurred())
			}).WithTimeout(30 * time.Second).Should(Succeed())

			// Allow time for controller to process the annotation change
			time.Sleep(15 * time.Second)

			By("Verifying service network associations are removed after adding standalone annotation")
			Eventually(func(g Gomega) {
				associations, err := testFramework.LatticeClient.ListServiceNetworkServiceAssociationsAsList(ctx, &vpclattice.ListServiceNetworkServiceAssociationsInput{
					ServiceIdentifier: vpcLatticeService.Id,
				})
				g.Expect(err).ToNot(HaveOccurred())

				// Check if associations are either empty or all are being deleted/deleted
				if len(associations) > 0 {
					// If associations exist, they should all be in a deletion state
					deletingOrDeletedAssociations := lo.Filter(associations, func(assoc *vpclattice.ServiceNetworkServiceAssociationSummary, _ int) bool {
						status := lo.FromPtr(assoc.Status)
						return status == vpclattice.ServiceNetworkServiceAssociationStatusDeleteInProgress ||
							status == vpclattice.ServiceNetworkServiceAssociationStatusDeleteFailed
					})
					g.Expect(len(deletingOrDeletedAssociations)).To(Equal(len(associations)), "All associations should be in deletion state when transitioning to standalone")
				}
			}).WithTimeout(3 * time.Minute).Should(Succeed())

			By("Verifying service remains functional during transition")
			// Get target group and verify it's still created properly
			targetGroup := testFramework.GetTargetGroup(ctx, service)
			Expect(targetGroup).ToNot(BeNil())

			// Verify targets are still registered
			testFramework.GetTargets(ctx, targetGroup, deployment)

			By("Verifying service ARN annotation is present after transition")
			Eventually(func(g Gomega) {
				updatedRoute := &gwv1.HTTPRoute{}
				err := testFramework.Get(ctx, types.NamespacedName{
					Name:      httpRoute.Name,
					Namespace: httpRoute.Namespace,
				}, updatedRoute)
				g.Expect(err).ToNot(HaveOccurred())

				annotations := updatedRoute.GetAnnotations()
				g.Expect(annotations).ToNot(BeNil())
				g.Expect(annotations).To(HaveKey(LatticeServiceArnAnnotation))
				g.Expect(annotations[LatticeServiceArnAnnotation]).To(Equal(lo.FromPtr(vpcLatticeService.Arn)))
			}).WithTimeout(2 * time.Minute).Should(Succeed())

			By("Verifying final state has no active service network associations")
			Eventually(func(g Gomega) {
				associations, err := testFramework.LatticeClient.ListServiceNetworkServiceAssociationsAsList(ctx, &vpclattice.ListServiceNetworkServiceAssociationsInput{
					ServiceIdentifier: vpcLatticeService.Id,
				})
				g.Expect(err).ToNot(HaveOccurred())

				// Filter for active associations - there should be none
				activeAssociations := lo.Filter(associations, func(assoc *vpclattice.ServiceNetworkServiceAssociationSummary, _ int) bool {
					return lo.FromPtr(assoc.Status) == vpclattice.ServiceNetworkServiceAssociationStatusActive
				})
				g.Expect(activeAssociations).To(BeEmpty(), "No active service network associations should remain in standalone mode")
			}).WithTimeout(4 * time.Minute).Should(Succeed())
		})

		AfterEach(func() {
			testFramework.ExpectDeletedThenNotFound(ctx, httpRoute, deployment, service)
		})
	})

	Context("Route-level annotation transitions with service network association verification", func() {
		var (
			deployment *appsv1.Deployment
			service    *corev1.Service
			httpRoute  *gwv1.HTTPRoute
		)

		BeforeEach(func() {
			deployment, service = testFramework.NewNginxApp(test.ElasticSearchOptions{
				Name:      "route-transition-test",
				Namespace: k8snamespace,
			})
		})

		It("should properly manage service network associations during route-level transitions", func() {
			By("Creating HTTPRoute in service network mode (no annotation)")
			httpRoute = testFramework.NewHttpRoute(testGateway, service, "Service")

			// Create the resources
			testFramework.ExpectCreated(ctx, httpRoute, deployment, service)

			// Verify VPC Lattice service is created
			route, _ := core.NewRoute(httpRoute)
			vpcLatticeService := testFramework.GetVpcLatticeService(ctx, route)
			Expect(vpcLatticeService).ToNot(BeNil())
			Expect(vpcLatticeService.Arn).ToNot(BeNil())

			By("Verifying initial service network associations exist")
			Eventually(func(g Gomega) {
				associations, err := testFramework.LatticeClient.ListServiceNetworkServiceAssociationsAsList(ctx, &vpclattice.ListServiceNetworkServiceAssociationsInput{
					ServiceIdentifier: vpcLatticeService.Id,
				})
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(associations).ToNot(BeEmpty(), "Initial service should have service network associations")

				activeAssociations := lo.Filter(associations, func(assoc *vpclattice.ServiceNetworkServiceAssociationSummary, _ int) bool {
					return lo.FromPtr(assoc.Status) == vpclattice.ServiceNetworkServiceAssociationStatusActive
				})
				g.Expect(activeAssociations).ToNot(BeEmpty(), "At least one association should be active initially")
			}).WithTimeout(2 * time.Minute).Should(Succeed())

			By("Transitioning to standalone mode by adding annotation")
			Eventually(func(g Gomega) {
				latestRoute := &gwv1.HTTPRoute{}
				err := testFramework.Get(ctx, types.NamespacedName{
					Name:      httpRoute.Name,
					Namespace: httpRoute.Namespace,
				}, latestRoute)
				g.Expect(err).ToNot(HaveOccurred())

				if latestRoute.Annotations == nil {
					latestRoute.Annotations = make(map[string]string)
				}
				latestRoute.Annotations[StandaloneTransitionAnnotation] = "true"

				err = testFramework.Update(ctx, latestRoute)
				g.Expect(err).ToNot(HaveOccurred())
			}).WithTimeout(30 * time.Second).Should(Succeed())

			By("Verifying service network associations are removed in standalone mode")
			Eventually(func(g Gomega) {
				associations, err := testFramework.LatticeClient.ListServiceNetworkServiceAssociationsAsList(ctx, &vpclattice.ListServiceNetworkServiceAssociationsInput{
					ServiceIdentifier: vpcLatticeService.Id,
				})
				g.Expect(err).ToNot(HaveOccurred())

				activeAssociations := lo.Filter(associations, func(assoc *vpclattice.ServiceNetworkServiceAssociationSummary, _ int) bool {
					return lo.FromPtr(assoc.Status) == vpclattice.ServiceNetworkServiceAssociationStatusActive
				})
				g.Expect(activeAssociations).To(BeEmpty(), "No active associations should exist in standalone mode")
			}).WithTimeout(4 * time.Minute).Should(Succeed())

			By("Transitioning back to service network mode by removing annotation")
			Eventually(func(g Gomega) {
				latestRoute := &gwv1.HTTPRoute{}
				err := testFramework.Get(ctx, types.NamespacedName{
					Name:      httpRoute.Name,
					Namespace: httpRoute.Namespace,
				}, latestRoute)
				g.Expect(err).ToNot(HaveOccurred())

				// Remove standalone annotation
				if latestRoute.Annotations != nil {
					delete(latestRoute.Annotations, StandaloneTransitionAnnotation)
					err = testFramework.Update(ctx, latestRoute)
					g.Expect(err).ToNot(HaveOccurred())
				}
			}).WithTimeout(30 * time.Second).Should(Succeed())

			By("Verifying service network associations are recreated")
			Eventually(func(g Gomega) {
				associations, err := testFramework.LatticeClient.ListServiceNetworkServiceAssociationsAsList(ctx, &vpclattice.ListServiceNetworkServiceAssociationsInput{
					ServiceIdentifier: vpcLatticeService.Id,
				})
				g.Expect(err).ToNot(HaveOccurred())

				// Log current associations for debugging
				testFramework.Log.Infof(ctx, "Current associations count: %d", len(associations))
				for i, assoc := range associations {
					testFramework.Log.Infof(ctx, "Association %d: ID=%s, Status=%s, ServiceNetwork=%s",
						i, lo.FromPtr(assoc.Id), lo.FromPtr(assoc.Status), lo.FromPtr(assoc.ServiceNetworkName))
				}

				activeAssociations := lo.Filter(associations, func(assoc *vpclattice.ServiceNetworkServiceAssociationSummary, _ int) bool {
					return lo.FromPtr(assoc.Status) == vpclattice.ServiceNetworkServiceAssociationStatusActive
				})

				// Check for associations in creation state as well
				creatingAssociations := lo.Filter(associations, func(assoc *vpclattice.ServiceNetworkServiceAssociationSummary, _ int) bool {
					status := lo.FromPtr(assoc.Status)
					return status == vpclattice.ServiceNetworkServiceAssociationStatusCreateInProgress ||
						status == vpclattice.ServiceNetworkServiceAssociationStatusActive
				})

				testFramework.Log.Infof(ctx, "Active associations: %d, Creating/Active associations: %d",
					len(activeAssociations), len(creatingAssociations))

				g.Expect(creatingAssociations).ToNot(BeEmpty(), "Associations should be recreated (active or creating) when returning to service network mode")
			}).WithTimeout(5 * time.Minute).Should(Succeed())

			By("Verifying service functionality is maintained throughout transitions")
			// Get target group and verify it's still created properly
			targetGroup := testFramework.GetTargetGroup(ctx, service)
			Expect(targetGroup).ToNot(BeNil())

			// Verify targets are still registered
			testFramework.GetTargets(ctx, targetGroup, deployment)

			By("Verifying service ARN annotation persists through transitions")
			Eventually(func(g Gomega) {
				updatedRoute := &gwv1.HTTPRoute{}
				err := testFramework.Get(ctx, types.NamespacedName{
					Name:      httpRoute.Name,
					Namespace: httpRoute.Namespace,
				}, updatedRoute)
				g.Expect(err).ToNot(HaveOccurred())

				annotations := updatedRoute.GetAnnotations()
				g.Expect(annotations).ToNot(BeNil())
				g.Expect(annotations).To(HaveKey(LatticeServiceArnAnnotation))
				g.Expect(annotations[LatticeServiceArnAnnotation]).To(Equal(lo.FromPtr(vpcLatticeService.Arn)))
			}).WithTimeout(2 * time.Minute).Should(Succeed())
		})

		AfterEach(func() {
			testFramework.ExpectDeletedThenNotFound(ctx, httpRoute, deployment, service)
		})
	})

	Context("Gateway-level annotation transitions", func() {
		var (
			deployment *appsv1.Deployment
			service    *corev1.Service
			httpRoute  *gwv1.HTTPRoute
		)

		BeforeEach(func() {
			deployment, service = testFramework.NewNginxApp(test.ElasticSearchOptions{
				Name:      "gateway-transition-test",
				Namespace: k8snamespace,
			})
		})

		It("should handle transitions when gateway-level annotation changes with route recreation", func() {
			By("Adding standalone annotation to existing test gateway")
			Eventually(func(g Gomega) {
				latestGateway := &gwv1.Gateway{}
				err := testFramework.Get(ctx, types.NamespacedName{
					Name:      testGateway.Name,
					Namespace: testGateway.Namespace,
				}, latestGateway)
				g.Expect(err).ToNot(HaveOccurred())

				if latestGateway.Annotations == nil {
					latestGateway.Annotations = make(map[string]string)
				}
				latestGateway.Annotations[StandaloneTransitionAnnotation] = "true"
				err = testFramework.Update(ctx, latestGateway)
				g.Expect(err).ToNot(HaveOccurred())
			}).WithTimeout(30 * time.Second).Should(Succeed())

			By("Creating HTTPRoute referencing standalone gateway")
			httpRoute = testFramework.NewHttpRoute(testGateway, service, "Service")

			// Create the resources
			testFramework.ExpectCreated(ctx, httpRoute, deployment, service)

			// Verify VPC Lattice service is created
			route, _ := core.NewRoute(httpRoute)
			vpcLatticeService := testFramework.GetVpcLatticeService(ctx, route)
			Expect(vpcLatticeService).ToNot(BeNil())

			By("Verifying route inherits standalone behavior from gateway")
			Eventually(func(g Gomega) {
				associations, err := testFramework.LatticeClient.ListServiceNetworkServiceAssociationsAsList(ctx, &vpclattice.ListServiceNetworkServiceAssociationsInput{
					ServiceIdentifier: vpcLatticeService.Id,
				})
				g.Expect(err).ToNot(HaveOccurred())

				activeAssociations := lo.Filter(associations, func(assoc *vpclattice.ServiceNetworkServiceAssociationSummary, _ int) bool {
					return lo.FromPtr(assoc.Status) == vpclattice.ServiceNetworkServiceAssociationStatusActive
				})
				g.Expect(activeAssociations).To(BeEmpty(), "Route should inherit standalone behavior from gateway")
			}).WithTimeout(3 * time.Minute).Should(Succeed())

			By("Deleting route to prepare for transition test")
			testFramework.ExpectDeletedThenNotFound(ctx, httpRoute)

			By("Removing standalone annotation from gateway")
			Eventually(func(g Gomega) {
				latestGateway := &gwv1.Gateway{}
				err := testFramework.Get(ctx, types.NamespacedName{
					Name:      testGateway.Name,
					Namespace: testGateway.Namespace,
				}, latestGateway)
				g.Expect(err).ToNot(HaveOccurred())

				if latestGateway.Annotations != nil {
					delete(latestGateway.Annotations, StandaloneTransitionAnnotation)
					err = testFramework.Update(ctx, latestGateway)
					g.Expect(err).ToNot(HaveOccurred())
				}
			}).WithTimeout(30 * time.Second).Should(Succeed())

			By("Verifying gateway annotation has been removed")
			Eventually(func(g Gomega) {
				latestGateway := &gwv1.Gateway{}
				err := testFramework.Get(ctx, types.NamespacedName{
					Name:      testGateway.Name,
					Namespace: testGateway.Namespace,
				}, latestGateway)
				g.Expect(err).ToNot(HaveOccurred())

				// Verify the annotation is actually removed
				if latestGateway.Annotations != nil {
					_, exists := latestGateway.Annotations[StandaloneTransitionAnnotation]
					g.Expect(exists).To(BeFalse(), "Standalone annotation should be removed from gateway")
				}
			}).WithTimeout(30 * time.Second).Should(Succeed())

			By("Recreating HTTPRoute to test transition to service network mode")
			httpRoute = testFramework.NewHttpRoute(testGateway, service, "Service")
			testFramework.ExpectCreated(ctx, httpRoute)

			// Get the new VPC Lattice service
			route, _ = core.NewRoute(httpRoute)
			vpcLatticeService = testFramework.GetVpcLatticeService(ctx, route)
			Expect(vpcLatticeService).ToNot(BeNil())

			By("Verifying route now uses service network mode")
			Eventually(func(g Gomega) {
				associations, err := testFramework.LatticeClient.ListServiceNetworkServiceAssociationsAsList(ctx, &vpclattice.ListServiceNetworkServiceAssociationsInput{
					ServiceIdentifier: vpcLatticeService.Id,
				})
				g.Expect(err).ToNot(HaveOccurred())

				activeAssociations := lo.Filter(associations, func(assoc *vpclattice.ServiceNetworkServiceAssociationSummary, _ int) bool {
					return lo.FromPtr(assoc.Status) == vpclattice.ServiceNetworkServiceAssociationStatusActive
				})
				g.Expect(activeAssociations).ToNot(BeEmpty(), "Route should use service network mode when gateway annotation is removed")
			}).WithTimeout(3 * time.Minute).Should(Succeed())

			By("Verifying service remains functional during gateway-level transitions")
			// Get target group and verify it's still created properly
			targetGroup := testFramework.GetTargetGroup(ctx, service)
			Expect(targetGroup).ToNot(BeNil())

			// Verify targets are still registered
			testFramework.GetTargets(ctx, targetGroup, deployment)
		})

		AfterEach(func() {
			if httpRoute != nil {
				testFramework.ExpectDeletedThenNotFound(ctx, httpRoute)
			}
			testFramework.ExpectDeletedThenNotFound(ctx, deployment, service)

			// Clean up the standalone annotation from the test gateway
			Eventually(func(g Gomega) {
				latestGateway := &gwv1.Gateway{}
				err := testFramework.Get(ctx, types.NamespacedName{
					Name:      testGateway.Name,
					Namespace: testGateway.Namespace,
				}, latestGateway)
				if err != nil {
					// Gateway might already be deleted, which is fine
					return
				}

				if latestGateway.Annotations != nil {
					delete(latestGateway.Annotations, StandaloneTransitionAnnotation)
					err = testFramework.Update(ctx, latestGateway)
					g.Expect(err).ToNot(HaveOccurred())
				}
			}).WithTimeout(30 * time.Second).Should(Succeed())
		})
	})

	Context("Error handling during transitions", func() {
		var (
			deployment *appsv1.Deployment
			service    *corev1.Service
			httpRoute  *gwv1.HTTPRoute
		)

		BeforeEach(func() {
			deployment, service = testFramework.NewNginxApp(test.ElasticSearchOptions{
				Name:      "error-handling-test",
				Namespace: k8snamespace,
			})
		})

		It("should handle invalid annotation values gracefully during transitions", func() {
			By("Creating HTTPRoute with invalid standalone annotation value")
			httpRoute = testFramework.NewHttpRoute(testGateway, service, "Service")

			// Add invalid standalone annotation value
			if httpRoute.Annotations == nil {
				httpRoute.Annotations = make(map[string]string)
			}
			httpRoute.Annotations[StandaloneTransitionAnnotation] = "invalid-value"

			// Create the resources
			testFramework.ExpectCreated(ctx, httpRoute, deployment, service)

			// Verify VPC Lattice service is created
			route, _ := core.NewRoute(httpRoute)
			vpcLatticeService := testFramework.GetVpcLatticeService(ctx, route)
			Expect(vpcLatticeService).ToNot(BeNil())

			By("Verifying invalid annotation value is treated as false (default behavior)")
			Eventually(func(g Gomega) {
				associations, err := testFramework.LatticeClient.ListServiceNetworkServiceAssociationsAsList(ctx, &vpclattice.ListServiceNetworkServiceAssociationsInput{
					ServiceIdentifier: vpcLatticeService.Id,
				})
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(associations).ToNot(BeEmpty(), "Invalid annotation value should be treated as false, creating service network associations")
			}).WithTimeout(2 * time.Minute).Should(Succeed())

			By("Correcting annotation value to valid 'true'")
			Eventually(func(g Gomega) {
				latestRoute := &gwv1.HTTPRoute{}
				err := testFramework.Get(ctx, types.NamespacedName{
					Name:      httpRoute.Name,
					Namespace: httpRoute.Namespace,
				}, latestRoute)
				g.Expect(err).ToNot(HaveOccurred())

				latestRoute.Annotations[StandaloneTransitionAnnotation] = "true"

				err = testFramework.Update(ctx, latestRoute)
				g.Expect(err).ToNot(HaveOccurred())
			}).WithTimeout(30 * time.Second).Should(Succeed())

			// Allow time for controller to process the corrected annotation
			time.Sleep(15 * time.Second)

			By("Verifying corrected annotation works properly")
			Eventually(func(g Gomega) {
				associations, err := testFramework.LatticeClient.ListServiceNetworkServiceAssociationsAsList(ctx, &vpclattice.ListServiceNetworkServiceAssociationsInput{
					ServiceIdentifier: vpcLatticeService.Id,
				})
				g.Expect(err).ToNot(HaveOccurred())

				activeAssociations := lo.Filter(associations, func(assoc *vpclattice.ServiceNetworkServiceAssociationSummary, _ int) bool {
					return lo.FromPtr(assoc.Status) == vpclattice.ServiceNetworkServiceAssociationStatusActive
				})
				g.Expect(activeAssociations).To(BeEmpty(), "Corrected annotation should result in standalone mode")
			}).WithTimeout(3 * time.Minute).Should(Succeed())

			By("Verifying service remains functional despite annotation errors")
			// Get target group and verify it's still created properly
			targetGroup := testFramework.GetTargetGroup(ctx, service)
			Expect(targetGroup).ToNot(BeNil())

			// Verify targets are still registered
			testFramework.GetTargets(ctx, targetGroup, deployment)
		})

		AfterEach(func() {
			testFramework.ExpectDeletedThenNotFound(ctx, httpRoute, deployment, service)
		})
	})
})
