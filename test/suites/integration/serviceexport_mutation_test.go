package integration

import (
	"time"

	"github.com/aws/aws-sdk-go/service/vpclattice"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"

	"github.com/aws/aws-application-networking-k8s/pkg/utils"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
)

var _ = Describe("ServiceExport Mutation Test", Ordered, func() {
	Context("Test ServiceExport Deletion", func() {
		var (
			deployment    *appsv1.Deployment
			service       *v1.Service
			serviceExport *anv1alpha1.ServiceExport
			targetGroup   *vpclattice.TargetGroupSummary
		)

		BeforeEach(func() {
			deployment, service = testFramework.NewNginxApp(test.ElasticSearchOptions{
				Name:      "serviceexport-mutation",
				Port:      8080,
				Port2:     8081,
				Namespace: k8snamespace,
			})
			serviceExport = testFramework.CreateServiceExport(service)
			numOfServiceExportAnnotationsDefinedPorts := 1
			testFramework.ExpectCreated(ctx, serviceExport, service, deployment)
			targetGroup = testFramework.GetTargetGroup(ctx, service)
			Expect(targetGroup).To(Not(BeNil()))
			Expect(*targetGroup.VpcIdentifier).To(Equal(test.CurrentClusterVpcId))
			Expect(*targetGroup.Protocol).To(Equal("HTTP"))
			Eventually(func(g Gomega) {
				_, retrievedTargets := testFramework.GetAllTargets(ctx, targetGroup, deployment)
				g.Expect(len(retrievedTargets)).To(Equal(numOfServiceExportAnnotationsDefinedPorts * int(*deployment.Spec.Replicas)))
			}).Should(Succeed())
		})

		AfterEach(func() {
			testFramework.ExpectDeletedThenNotFound(ctx,
				serviceExport,
				deployment,
				service,
			)
		})

		When("Update number of pods for a k8s service", func() {
			It("Expect corresponding serviceExport created target group's targets change as well", func() {

				//Get lattice targets before change deployment Replicas number
				retrievedTargets, err := testFramework.LatticeClient.ListTargetsAsList(ctx, &vpclattice.ListTargetsInput{TargetGroupIdentifier: targetGroup.Id})
				Expect(err).To(BeNil())
				enpoints := v1.Endpoints{}
				testFramework.Get(ctx, client.ObjectKeyFromObject(service), &enpoints)

				Expect(len(retrievedTargets)).To(Equal(2))
				Expect(len(enpoints.Subsets[0].Addresses)).To(Equal(2))
				ipsFromK8sEndpoints := utils.SliceMap(enpoints.Subsets[0].Addresses, func(addr v1.EndpointAddress) string { return addr.IP })
				ipsFromLatticeTargets := utils.SliceMap(retrievedTargets, func(target *vpclattice.TargetSummary) string { return *target.Id })
				Expect(ipsFromK8sEndpoints).To(ConsistOf(ipsFromLatticeTargets))

				//Update Deployment Replicas number to 3
				testFramework.Get(ctx, client.ObjectKeyFromObject(deployment), deployment)
				deployment.Spec.Replicas = lo.ToPtr(int32(3))
				testFramework.ExpectUpdated(ctx, deployment)

				Eventually(func(g Gomega) {
					//Get lattice targets again
					retrievedTargets, err := testFramework.LatticeClient.ListTargetsAsList(ctx, &vpclattice.ListTargetsInput{TargetGroupIdentifier: targetGroup.Id})
					Expect(err).To(BeNil())
					testFramework.Get(ctx, client.ObjectKeyFromObject(service), &enpoints)

					g.Expect(len(retrievedTargets)).To(Equal(3))
					g.Expect(len(enpoints.Subsets[0].Addresses)).To(Equal(3))
					ipsFromK8sEndpoints = utils.SliceMap(enpoints.Subsets[0].Addresses, func(addr v1.EndpointAddress) string { return addr.IP })
					ipsFromLatticeTargets = utils.SliceMap(retrievedTargets, func(target *vpclattice.TargetSummary) string { return *target.Id })
					g.Expect(ipsFromK8sEndpoints).To(ConsistOf(ipsFromLatticeTargets))
				}).Should(Succeed())

			})

			When("Delete ServiceExport, while corresponding K8sService exists", func() {
				It("Expect targetGroup not found", func() {
					testFramework.ExpectDeletedThenNotFound(ctx, serviceExport)
					testFramework.VerifyTargetGroupNotFound(targetGroup)
				})
			})

			When("Delete ServiceExport, while corresponding K8sService do NOT exists", func() {
				It("Expect targetGroup not found", func() {
					testFramework.ExpectDeletedThenNotFound(ctx, service)
					time.Sleep(5 * time.Second)
					testFramework.ExpectDeletedThenNotFound(ctx, serviceExport)
					testFramework.VerifyTargetGroupNotFound(targetGroup)
				})
			})
		})
	})
})
