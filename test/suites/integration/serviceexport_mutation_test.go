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
			deployment      *appsv1.Deployment
			service         *v1.Service
			serviceExport   *anv1alpha1.ServiceExport
			httpTargetGroup *vpclattice.TargetGroupSummary
			grpcTargetGroup *vpclattice.TargetGroupSummary
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

			httpTargetGroup = testFramework.GetHttpTargetGroup(ctx, service)
			Expect(httpTargetGroup).To(Not(BeNil()))
			Expect(*httpTargetGroup.VpcIdentifier).To(Equal(test.CurrentClusterVpcId))
			Expect(*httpTargetGroup.Protocol).To(Equal("HTTP"))

			grpcTargetGroup = testFramework.GetGrpcTargetGroup(ctx, service)
			Expect(grpcTargetGroup).To(Not(BeNil()))
			Expect(*grpcTargetGroup.VpcIdentifier).To(Equal(test.CurrentClusterVpcId))
			Expect(*grpcTargetGroup.Protocol).To(Equal("HTTPS"))

			Eventually(func(g Gomega) {
				_, retrievedTargets := testFramework.GetAllTargets(ctx, httpTargetGroup, deployment)
				g.Expect(len(retrievedTargets)).To(Equal(numOfServiceExportAnnotationsDefinedPorts * int(*deployment.Spec.Replicas)))

				_, retrievedTargets = testFramework.GetAllTargets(ctx, grpcTargetGroup, deployment)
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
				httpTargets, err := testFramework.LatticeClient.ListTargetsAsList(ctx, &vpclattice.ListTargetsInput{
					TargetGroupIdentifier: httpTargetGroup.Id,
				})
				Expect(err).To(BeNil())

				grpcTargets, err := testFramework.LatticeClient.ListTargetsAsList(ctx, &vpclattice.ListTargetsInput{
					TargetGroupIdentifier: grpcTargetGroup.Id,
				})
				Expect(err).To(BeNil())

				endpoints := v1.Endpoints{}
				err = testFramework.Get(ctx, client.ObjectKeyFromObject(service), &endpoints)
				Expect(err).To(BeNil())

				Expect(len(httpTargets)).To(Equal(2))
				Expect(len(grpcTargets)).To(Equal(2))
				Expect(len(endpoints.Subsets[0].Addresses)).To(Equal(2))
				ipsFromK8sEndpoints := utils.SliceMap(endpoints.Subsets[0].Addresses, func(addr v1.EndpointAddress) string { return addr.IP })
				httpIpsFromLatticeTargets := utils.SliceMap(httpTargets, func(target *vpclattice.TargetSummary) string { return *target.Id })
				grpcIpsFromLatticeTargets := utils.SliceMap(grpcTargets, func(target *vpclattice.TargetSummary) string { return *target.Id })
				Expect(ipsFromK8sEndpoints).To(ConsistOf(httpIpsFromLatticeTargets))
				Expect(ipsFromK8sEndpoints).To(ConsistOf(grpcIpsFromLatticeTargets))

				//Update Deployment Replicas number to 3
				err = testFramework.Get(ctx, client.ObjectKeyFromObject(deployment), deployment)
				Expect(err).To(BeNil())
				deployment.Spec.Replicas = lo.ToPtr(int32(3))
				testFramework.ExpectUpdated(ctx, deployment)

				Eventually(func(g Gomega) {
					//Get lattice targets again
					httpTargets, err := testFramework.LatticeClient.ListTargetsAsList(ctx, &vpclattice.ListTargetsInput{
						TargetGroupIdentifier: httpTargetGroup.Id,
					})
					Expect(err).To(BeNil())

					grpcTargets, err := testFramework.LatticeClient.ListTargetsAsList(ctx, &vpclattice.ListTargetsInput{
						TargetGroupIdentifier: grpcTargetGroup.Id,
					})
					Expect(err).To(BeNil())
					testFramework.Get(ctx, client.ObjectKeyFromObject(service), &endpoints)

					g.Expect(len(httpTargets)).To(Equal(3))
					g.Expect(len(grpcTargets)).To(Equal(3))
					g.Expect(len(endpoints.Subsets[0].Addresses)).To(Equal(3))
					ipsFromK8sEndpoints = utils.SliceMap(endpoints.Subsets[0].Addresses, func(addr v1.EndpointAddress) string { return addr.IP })
					httpIpsFromLatticeTargets = utils.SliceMap(httpTargets, func(target *vpclattice.TargetSummary) string { return *target.Id })
					grpcIpsFromLatticeTargets = utils.SliceMap(grpcTargets, func(target *vpclattice.TargetSummary) string { return *target.Id })
					g.Expect(ipsFromK8sEndpoints).To(ConsistOf(httpIpsFromLatticeTargets))
					g.Expect(ipsFromK8sEndpoints).To(ConsistOf(grpcIpsFromLatticeTargets))
				}).Should(Succeed())
			})

			When("Delete ServiceExport, while corresponding K8sService exists", func() {
				It("Expect targetGroup not found", func() {
					testFramework.ExpectDeletedThenNotFound(ctx, serviceExport)
					testFramework.VerifyTargetGroupNotFound(httpTargetGroup)
					testFramework.VerifyTargetGroupNotFound(grpcTargetGroup)
				})
			})

			When("Delete ServiceExport, while corresponding K8sService do NOT exists", func() {
				It("Expect targetGroup not found", func() {
					testFramework.ExpectDeletedThenNotFound(ctx, service)
					time.Sleep(5 * time.Second)
					testFramework.ExpectDeletedThenNotFound(ctx, serviceExport)
					testFramework.VerifyTargetGroupNotFound(httpTargetGroup)
					testFramework.VerifyTargetGroupNotFound(grpcTargetGroup)
				})
			})
		})
	})
})
