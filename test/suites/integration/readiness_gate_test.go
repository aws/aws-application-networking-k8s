package integration

import (
	"github.com/aws/aws-application-networking-k8s/pkg/deploy/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/pkg/utils"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = Describe("Pod Readiness Gate", Ordered, func() {
	var (
		deployment *appsv1.Deployment
		service    *v1.Service
		route      *gwv1.HTTPRoute
	)

	BeforeAll(func() {
		deployment, service = testFramework.NewNginxApp(test.ElasticSearchOptions{
			Name:          "pod-test",
			Namespace:     k8snamespace,
			ReadinessGate: true,
		})
		route = testFramework.NewHttpRoute(testGateway, service, service.Kind)
		testFramework.ExpectCreated(ctx, deployment, service, route)
	})

	It("updates condition when injected", func() {
		Eventually(func(g Gomega) {
			pods := testFramework.GetPodsByDeploymentName(deployment.Name, deployment.Namespace)
			for _, pod := range pods {
				cond := utils.FindPodStatusCondition(pod.Status.Conditions, lattice.LatticeReadinessGateConditionType)
				g.Expect(cond).To(Not(BeNil()))
				g.Expect(cond.Status).To(Equal(v1.ConditionTrue))
			}
		}).Should(Succeed())
	})

	It("updates condition when a new pod is added", func() {
		testFramework.Get(ctx, k8s.NamespacedName(deployment), deployment)
		deployment.Spec.Replicas = lo.ToPtr(int32(3))
		testFramework.ExpectUpdated(ctx, deployment)

		Eventually(func(g Gomega) {
			pods := testFramework.GetPodsByDeploymentName(deployment.Name, deployment.Namespace)
			for _, pod := range pods {
				cond := utils.FindPodStatusCondition(pod.Status.Conditions, lattice.LatticeReadinessGateConditionType)
				g.Expect(cond).To(Not(BeNil()))
				g.Expect(cond.Status).To(Equal(v1.ConditionTrue))
			}
		}).Should(Succeed())
	})

	AfterAll(func() {
		testFramework.ExpectDeletedThenNotFound(ctx,
			deployment,
			service,
			route,
		)
	})
})
