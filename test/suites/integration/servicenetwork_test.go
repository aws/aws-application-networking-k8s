package integration

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = Describe("ServiceNetwork CRD", Serial, Ordered, func() {
	var (
		serviceNetwork *anv1alpha1.ServiceNetwork
	)

	const snName = "test-sn-e2e"

	AfterAll(func() {
		if serviceNetwork != nil {
			testFramework.ExpectDeletedThenNotFound(ctx, serviceNetwork)
		}
		// Verify Lattice SN is also gone
		Eventually(func(g Gomega) {
			sn, err := testFramework.LatticeClient.FindServiceNetwork(ctx, snName)
			if err != nil {
				return // not found error is fine
			}
			g.Expect(sn).To(BeNil())
		}).Should(Succeed())
	})

	It("Create ServiceNetwork CR and verify Lattice service network is created", func() {
		serviceNetwork = &anv1alpha1.ServiceNetwork{
			ObjectMeta: metav1.ObjectMeta{
				Name:   snName,
				Labels: map[string]string{test.DiscoveryLabel: "true"},
				Annotations: map[string]string{
					"application-networking.k8s.aws/tags": "Environment=Dev,Team=Platform",
				},
			},
		}
		testFramework.ExpectCreated(ctx, serviceNetwork)

		// Verify Lattice SN exists with correct tags and CR status is updated
		Eventually(func(g Gomega) {
			snInfo, err := testFramework.LatticeClient.FindServiceNetwork(ctx, snName)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(snInfo).ToNot(BeNil())
			g.Expect(aws.StringValue(snInfo.SvcNetwork.Name)).To(Equal(snName))

			snArn := aws.StringValue(snInfo.SvcNetwork.Arn)
			tags, err := testFramework.Cloud.Tagging().GetTagsForArns(ctx, []string{snArn})
			g.Expect(err).ToNot(HaveOccurred())
			snTags := tags[snArn]
			g.Expect(snTags).To(HaveKeyWithValue("Environment", aws.String("Dev")))
			g.Expect(snTags).To(HaveKeyWithValue("Team", aws.String("Platform")))

			updated := &anv1alpha1.ServiceNetwork{}
			err = testFramework.Get(ctx, client.ObjectKeyFromObject(serviceNetwork), updated)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(updated.Status.ServiceNetworkARN).ToNot(BeEmpty())
			g.Expect(updated.Status.ServiceNetworkID).ToNot(BeEmpty())

			programmed := false
			for _, cond := range updated.Status.Conditions {
				if cond.Type == "Programmed" && cond.Status == metav1.ConditionTrue {
					programmed = true
				}
			}
			g.Expect(programmed).To(BeTrue())
		}).Should(Succeed())
	})
})

var _ = Describe("ServiceNetwork CRD adopt existing", Serial, Ordered, func() {
	var (
		serviceNetwork *anv1alpha1.ServiceNetwork
		preCreatedSnId *string
	)

	const snName = "test-sn-adopt-e2e"

	BeforeAll(func() {
		// Pre-create a Lattice SN via SDK (simulating external creation)
		resp, err := testFramework.LatticeClient.CreateServiceNetworkWithContext(ctx, &vpclattice.CreateServiceNetworkInput{
			Name: aws.String(snName),
		})
		Expect(err).ToNot(HaveOccurred())
		preCreatedSnId = resp.Id
	})

	AfterAll(func() {
		if serviceNetwork != nil {
			testFramework.ExpectDeletedThenNotFound(ctx, serviceNetwork)
		}
		Eventually(func(g Gomega) {
			sn, err := testFramework.LatticeClient.FindServiceNetwork(ctx, snName)
			if err != nil {
				return
			}
			g.Expect(sn).To(BeNil())
		}).Should(Succeed())
	})

	It("Create ServiceNetwork CR for pre-existing Lattice SN and verify adoption", func() {
		serviceNetwork = &anv1alpha1.ServiceNetwork{
			ObjectMeta: metav1.ObjectMeta{
				Name:   snName,
				Labels: map[string]string{test.DiscoveryLabel: "true"},
			},
		}
		testFramework.ExpectCreated(ctx, serviceNetwork)

		// Verify CR status shows the pre-existing SN's ID and is Programmed
		Eventually(func(g Gomega) {
			updated := &anv1alpha1.ServiceNetwork{}
			err := testFramework.Get(ctx, client.ObjectKeyFromObject(serviceNetwork), updated)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(updated.Status.ServiceNetworkID).To(Equal(aws.StringValue(preCreatedSnId)))
			g.Expect(updated.Status.ServiceNetworkARN).ToNot(BeEmpty())

			programmed := false
			for _, cond := range updated.Status.Conditions {
				if cond.Type == "Programmed" && cond.Status == metav1.ConditionTrue {
					programmed = true
				}
			}
			g.Expect(programmed).To(BeTrue())

			// Verify ManagedBy tag was added
			tags, err := testFramework.Cloud.Tagging().GetTagsForArns(ctx, []string{updated.Status.ServiceNetworkARN})
			g.Expect(err).ToNot(HaveOccurred())
			snTags := tags[updated.Status.ServiceNetworkARN]
			g.Expect(snTags).To(HaveKey("application-networking.k8s.aws/ManagedBy"))
		}).Should(Succeed())
	})
})

var _ = Describe("ServiceNetwork CRD delete blocked by gateway", Serial, Ordered, func() {
	var (
		serviceNetwork *anv1alpha1.ServiceNetwork
		gateway        *gwv1.Gateway
	)

	const snName = "test-sn-del-gw-e2e"

	AfterAll(func() {
		// Delete gateway first to unblock SN deletion
		if gateway != nil {
			testFramework.ExpectDeletedThenNotFound(ctx, gateway)
		}
		if serviceNetwork != nil {
			testFramework.ExpectDeletedThenNotFound(ctx, serviceNetwork)
		}
		Eventually(func(g Gomega) {
			sn, err := testFramework.LatticeClient.FindServiceNetwork(ctx, snName)
			if err != nil {
				return
			}
			g.Expect(sn).To(BeNil())
		}).Should(Succeed())
	})

	It("Create ServiceNetwork and Gateway, then verify delete is blocked", func() {
		// Create the ServiceNetwork CR
		serviceNetwork = &anv1alpha1.ServiceNetwork{
			ObjectMeta: metav1.ObjectMeta{
				Name:   snName,
				Labels: map[string]string{test.DiscoveryLabel: "true"},
			},
		}
		testFramework.ExpectCreated(ctx, serviceNetwork)

		// Wait for it to be programmed
		Eventually(func(g Gomega) {
			updated := &anv1alpha1.ServiceNetwork{}
			err := testFramework.Get(ctx, client.ObjectKeyFromObject(serviceNetwork), updated)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(updated.Status.ServiceNetworkARN).ToNot(BeEmpty())
		}).Should(Succeed())

		// Create a Gateway with the same name
		gateway = testFramework.NewGateway(snName, k8snamespace)
		testFramework.ExpectCreated(ctx, gateway)

		// Delete the ServiceNetwork CR
		err := testFramework.Delete(ctx, serviceNetwork)
		Expect(err).ToNot(HaveOccurred())

		// Verify deletion is blocked — CR should still exist with DeleteBlocked status
		Eventually(func(g Gomega) {
			updated := &anv1alpha1.ServiceNetwork{}
			err := testFramework.Get(ctx, client.ObjectKeyFromObject(serviceNetwork), updated)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(updated.DeletionTimestamp).ToNot(BeNil())

			blocked := false
			for _, cond := range updated.Status.Conditions {
				if cond.Type == "Programmed" && cond.Status == metav1.ConditionFalse && cond.Reason == "DeleteBlocked" {
					blocked = true
				}
			}
			g.Expect(blocked).To(BeTrue())
		}).Should(Succeed())
	})
})

var _ = Describe("ServiceNetwork CRD delete blocked by association", Serial, Ordered, func() {
	var (
		serviceNetwork *anv1alpha1.ServiceNetwork
		serviceId      *string
		snssaId        *string
	)

	const snName = "test-sn-del-assoc-e2e"
	const svcName = "test-svc-block-e2e"

	AfterAll(func() {
		// Clean up association first to unblock SN deletion
		if snssaId != nil {
			testFramework.LatticeClient.DeleteServiceNetworkServiceAssociationWithContext(ctx, &vpclattice.DeleteServiceNetworkServiceAssociationInput{
				ServiceNetworkServiceAssociationIdentifier: snssaId,
			})
			Eventually(func(g Gomega) {
				_, err := testFramework.LatticeClient.GetServiceNetworkServiceAssociationWithContext(ctx, &vpclattice.GetServiceNetworkServiceAssociationInput{
					ServiceNetworkServiceAssociationIdentifier: snssaId,
				})
				g.Expect(err).To(HaveOccurred())
			}).Should(Succeed())
		}
		if serviceId != nil {
			Eventually(func() error {
				_, err := testFramework.LatticeClient.DeleteServiceWithContext(ctx, &vpclattice.DeleteServiceInput{
					ServiceIdentifier: serviceId,
				})
				return err
			}).Should(Succeed())
		}
		if serviceNetwork != nil {
			testFramework.ExpectDeletedThenNotFound(ctx, serviceNetwork)
		}
		Eventually(func(g Gomega) {
			sn, err := testFramework.LatticeClient.FindServiceNetwork(ctx, snName)
			if err != nil {
				return
			}
			g.Expect(sn).To(BeNil())
		}).Should(Succeed())
	})

	// This test verifies that Lattice returns a ConflictException when deleting a
	// service network that still has associations (VPC, service, resource, or endpoint).
	// We use a service association here as it's the easiest to set up without
	// depending on extra VPCs or other infrastructure. The controller code path
	// is the same for all association types — DeleteServiceNetwork returns
	// ConflictException and the controller surfaces the message in CR status.
	It("Create ServiceNetwork with service association, then verify delete is blocked", func() {
		// Create the ServiceNetwork CR
		serviceNetwork = &anv1alpha1.ServiceNetwork{
			ObjectMeta: metav1.ObjectMeta{
				Name:   snName,
				Labels: map[string]string{test.DiscoveryLabel: "true"},
			},
		}
		testFramework.ExpectCreated(ctx, serviceNetwork)

		// Wait for it to be programmed
		var snId string
		Eventually(func(g Gomega) {
			updated := &anv1alpha1.ServiceNetwork{}
			err := testFramework.Get(ctx, client.ObjectKeyFromObject(serviceNetwork), updated)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(updated.Status.ServiceNetworkID).ToNot(BeEmpty())
			snId = updated.Status.ServiceNetworkID
		}).Should(Succeed())

		// Create a Lattice service via SDK and wait for it to be active
		svcResp, err := testFramework.LatticeClient.CreateServiceWithContext(ctx, &vpclattice.CreateServiceInput{
			Name: aws.String(svcName),
		})
		Expect(err).ToNot(HaveOccurred())
		serviceId = svcResp.Id

		Eventually(func(g Gomega) {
			svc, err := testFramework.LatticeClient.FindService(ctx, svcName)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(svc).ToNot(BeNil())
			g.Expect(aws.StringValue(svc.Status)).To(Equal(vpclattice.ServiceStatusActive))
		}).Should(Succeed())

		// Associate the service to the SN
		assocResp, err := testFramework.LatticeClient.CreateServiceNetworkServiceAssociationWithContext(ctx, &vpclattice.CreateServiceNetworkServiceAssociationInput{
			ServiceNetworkIdentifier: aws.String(snId),
			ServiceIdentifier:        serviceId,
		})
		Expect(err).ToNot(HaveOccurred())
		snssaId = assocResp.Id

		// Wait for association to be active
		Eventually(func(g Gomega) {
			resp, err := testFramework.LatticeClient.GetServiceNetworkServiceAssociationWithContext(ctx, &vpclattice.GetServiceNetworkServiceAssociationInput{
				ServiceNetworkServiceAssociationIdentifier: snssaId,
			})
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(aws.StringValue(resp.Status)).To(Equal(vpclattice.ServiceNetworkServiceAssociationStatusActive))
		}).Should(Succeed())

		// Delete the ServiceNetwork CR
		err = testFramework.Delete(ctx, serviceNetwork)
		Expect(err).ToNot(HaveOccurred())

		// Verify deletion is blocked with ConflictException
		Eventually(func(g Gomega) {
			updated := &anv1alpha1.ServiceNetwork{}
			err := testFramework.Get(ctx, client.ObjectKeyFromObject(serviceNetwork), updated)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(updated.DeletionTimestamp).ToNot(BeNil())

			deleteError := false
			for _, cond := range updated.Status.Conditions {
				if cond.Type == "Programmed" && cond.Status == metav1.ConditionFalse && cond.Reason == "DeleteError" {
					deleteError = true
				}
			}
			g.Expect(deleteError).To(BeTrue())
		}).Should(Succeed())
	})
})
