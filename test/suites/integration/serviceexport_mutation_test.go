package integration

import (
	"time"

	"github.com/aws/aws-sdk-go/service/vpclattice"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
)

var _ = Describe("ServiceExport Mutation Test", func() {
	Context("Test ServiceExport Deletion", func() {
		var (
			deployment    *appsv1.Deployment
			service       *v1.Service
			serviceExport *v1alpha1.ServiceExport
			targetGroup   *vpclattice.TargetGroupSummary
		)

		BeforeEach(func() {
			deployment, service = testFramework.NewNginxApp(test.ElasticSearchOptions{
				Name:      "http",
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
