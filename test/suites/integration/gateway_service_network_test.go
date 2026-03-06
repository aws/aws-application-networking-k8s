package integration

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = Describe("Gateway ServiceNetwork Lifecycle", Ordered, func() {

	Context("Sibling Gateway deletion safety", Ordered, func() {
		var (
			ns2    *corev1.Namespace
			gw1    *gwv1.Gateway
			gw2    *gwv1.Gateway
			gwName string
		)

		BeforeAll(func() {
			gwName = "sibling-gw-test"

			gw1 = testFramework.NewGateway(gwName, k8snamespace)
			testFramework.ExpectCreated(ctx, gw1)

			ns2 = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "e2e-sibling-ns"}}
			testFramework.ExpectCreated(ctx, ns2)
			gw2 = testFramework.NewGateway(gwName, ns2.Name)
			testFramework.ExpectCreated(ctx, gw2)

			// Wait for SN to exist
			_ = testFramework.GetServiceNetwork(ctx, gw1)
		})

		It("deleting gw1 does NOT delete the ServiceNetwork while gw2 exists", func() {
			testFramework.ExpectDeletedThenNotFound(ctx, gw1)

			// SN must still exist because gw2 is alive
			Consistently(func(g Gomega) {
				sn := testFramework.GetServiceNetwork(ctx, gw2)
				g.Expect(sn).ToNot(BeNil())
			}, "30s", "5s").Should(Succeed())
		})

		It("deleting gw2 (last Gateway) deletes the ServiceNetwork", func() {
			testFramework.ExpectDeletedThenNotFound(ctx, gw2)

			Eventually(func(g Gomega) {
				list, err := testFramework.LatticeClient.ListServiceNetworksWithContext(ctx, &vpclattice.ListServiceNetworksInput{})
				g.Expect(err).ToNot(HaveOccurred())
				for _, sn := range list.Items {
					g.Expect(aws.StringValue(sn.Name)).ToNot(Equal(gwName))
				}
			}).Should(Succeed())

			// Cleanup namespace
			testFramework.ExpectDeletedThenNotFound(ctx, ns2)
		})
	})

	Context("Service association delete guard", Ordered, func() {
		var (
			gw        *gwv1.Gateway
			gwName    string
			dummySvcId *string
		)

		BeforeAll(func() {
			gwName = "assoc-guard-test"
			gw = testFramework.NewGateway(gwName, k8snamespace)
			testFramework.ExpectCreated(ctx, gw)

			sn := testFramework.GetServiceNetwork(ctx, gw)

			// Create a dummy Lattice service and wait for it to become ACTIVE
			svcResp, err := testFramework.LatticeClient.CreateServiceWithContext(ctx, &vpclattice.CreateServiceInput{
				Name: aws.String(gwName + "-dummy-svc"),
			})
			Expect(err).ToNot(HaveOccurred())
			dummySvcId = svcResp.Id

			Eventually(func(g Gomega) {
				out, err := testFramework.LatticeClient.GetServiceWithContext(ctx, &vpclattice.GetServiceInput{
					ServiceIdentifier: dummySvcId,
				})
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(aws.StringValue(out.Status)).To(Equal(vpclattice.ServiceStatusActive))
			}).Should(Succeed())

			_, err = testFramework.LatticeClient.CreateServiceNetworkServiceAssociationWithContext(ctx, &vpclattice.CreateServiceNetworkServiceAssociationInput{
				ServiceNetworkIdentifier: sn.Id,
				ServiceIdentifier:        dummySvcId,
			})
			Expect(err).ToNot(HaveOccurred())

			// Wait for association to become active
			Eventually(func(g Gomega) {
				assocs, err := testFramework.LatticeClient.ListServiceNetworkServiceAssociationsAsList(ctx,
					&vpclattice.ListServiceNetworkServiceAssociationsInput{ServiceNetworkIdentifier: sn.Id})
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(assocs).ToNot(BeEmpty())
			}).Should(Succeed())
		})

		It("Gateway deletion is blocked while SN has active service associations", func() {
			Expect(testFramework.Delete(ctx, gw)).To(Succeed())

			// Gateway should still exist — finalizer can't be removed
			Consistently(func(g Gomega) {
				got := &gwv1.Gateway{}
				err := testFramework.Get(ctx, types.NamespacedName{Name: gw.Name, Namespace: gw.Namespace}, got)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(got.DeletionTimestamp.IsZero()).To(BeFalse())
			}, "30s", "5s").Should(Succeed())
		})

		It("Gateway deletes after removing the service association", func() {
			sn := testFramework.GetServiceNetwork(ctx, gw)

			// Remove associations
			assocs, err := testFramework.LatticeClient.ListServiceNetworkServiceAssociationsAsList(ctx,
				&vpclattice.ListServiceNetworkServiceAssociationsInput{ServiceNetworkIdentifier: sn.Id})
			Expect(err).ToNot(HaveOccurred())
			for _, a := range assocs {
				_, err := testFramework.LatticeClient.DeleteServiceNetworkServiceAssociationWithContext(ctx,
					&vpclattice.DeleteServiceNetworkServiceAssociationInput{
						ServiceNetworkServiceAssociationIdentifier: a.Id,
					})
				Expect(err).ToNot(HaveOccurred())
			}

			// Wait for associations to be fully deleted, then delete dummy service
			Eventually(func(g Gomega) {
				_, err := testFramework.LatticeClient.DeleteServiceWithContext(ctx, &vpclattice.DeleteServiceInput{
					ServiceIdentifier: dummySvcId,
				})
				g.Expect(err).ToNot(HaveOccurred())
			}).Should(Succeed())

			// Gateway should now fully delete
			testFramework.EventuallyExpectNotFound(ctx, gw)

			// SN should be gone
			Eventually(func(g Gomega) {
				list, err := testFramework.LatticeClient.ListServiceNetworksWithContext(ctx, &vpclattice.ListServiceNetworksInput{})
				g.Expect(err).ToNot(HaveOccurred())
				for _, s := range list.Items {
					g.Expect(aws.StringValue(s.Name)).ToNot(Equal(gwName))
				}
			}).Should(Succeed())
		})
	})
})
