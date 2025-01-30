package webhook

import (
	"fmt"
	"github.com/aws/aws-application-networking-k8s/pkg/utils"
	"github.com/aws/aws-application-networking-k8s/pkg/webhook"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"time"
)

var _ = Describe("Readiness Gate Inject", Ordered, func() {
	untaggedNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "webhook-e2e-test-no-tag",
		},
	}
	taggedNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "webhook-e2e-test-tagged",
			Labels: map[string]string{
				"application-networking.k8s.aws/pod-readiness-gate-inject": "enabled",
			},
		},
	}

	BeforeAll(func() {
		Eventually(func(g Gomega) {
			_ = testFramework.Delete(ctx, untaggedNS)
			_ = testFramework.Delete(ctx, taggedNS)
			testFramework.EventuallyExpectNotFound(ctx, untaggedNS, taggedNS)
		}).WithTimeout(30 * time.Second).WithOffset(1).Should(Succeed())

		Eventually(func(g Gomega) {
			testFramework.ExpectCreated(ctx, untaggedNS, taggedNS)
		}).WithTimeout(30 * time.Second).WithOffset(1).Should(Succeed())
	})

	It("create deployment in untagged namespace, no readiness gate", func() {
		deployment, service := testFramework.NewHttpApp(test.HTTPAppOptions{Name: "untagged-test-pod", Namespace: untaggedNS.Name})
		Eventually(func(g Gomega) {
			testFramework.Create(ctx, service)
			testFramework.Create(ctx, deployment)
			testFramework.Get(ctx, types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, deployment)

			pods := testFramework.GetPodsByDeploymentName(deployment.Name, deployment.Namespace)
			g.Expect(len(pods)).To(BeEquivalentTo(1))

			pod := pods[0]
			pct := corev1.PodConditionType(webhook.PodReadinessGateConditionType)

			for _, rg := range pod.Spec.ReadinessGates {
				if rg.ConditionType == pct {
					g.Expect(true).To(BeFalse(), "Pod readiness gate was injected to unlabeled namespace")
				}
			}
		}).WithTimeout(30 * time.Second).WithOffset(1).Should(Succeed())
	})

	It("create deployment in tagged namespace, but no gateway/route reference, no readiness gate", func() {
		deployment, service := testFramework.NewHttpApp(test.HTTPAppOptions{Name: "tagged-nope-pod", Namespace: taggedNS.Name})
		Eventually(func(g Gomega) {
			testFramework.Create(ctx, service)
			testFramework.Create(ctx, deployment)
			testFramework.Get(ctx, types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, deployment)

			pods := testFramework.GetPodsByDeploymentName(deployment.Name, deployment.Namespace)
			g.Expect(len(pods)).To(BeEquivalentTo(1))

			pod := pods[0]
			pct := corev1.PodConditionType(webhook.PodReadinessGateConditionType)

			for _, rg := range pod.Spec.ReadinessGates {
				if rg.ConditionType == pct {
					g.Expect(true).To(BeFalse(), "Pod readiness gate was injected without gateway/route reference")
				}
			}
		}).WithTimeout(30 * time.Second).WithOffset(1).Should(Succeed())
	})

	It("create deployment in tagged namespace, gate injected and transitions to healthy", func() {
		deployment, service := testFramework.NewHttpApp(test.HTTPAppOptions{Name: "tagged-yes-pod", Namespace: taggedNS.Name})
		httpRoute := testFramework.NewHttpRoute(testGateway, service, "Service")

		// first create the http route so we can be sure the readiness gate gets flagged
		err := testFramework.Create(ctx, httpRoute)
		Expect(err).ToNot(HaveOccurred())
		err = testFramework.Create(ctx, service)
		Expect(err).ToNot(HaveOccurred())
		// creating the deployment will trigger the readiness gate injection
		err = testFramework.Create(ctx, deployment)
		Expect(err).ToNot(HaveOccurred())

		Eventually(func(g Gomega) {
			testFramework.Get(ctx, types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, deployment)

			pods := testFramework.GetPodsByDeploymentName(deployment.Name, deployment.Namespace)
			g.Expect(len(pods)).To(BeEquivalentTo(1))

			pod := pods[0]
			pct := corev1.PodConditionType(webhook.PodReadinessGateConditionType)

			foundCount := 0
			for _, rg := range pod.Spec.ReadinessGates {
				if rg.ConditionType == pct {
					foundCount++
				}
			}

			g.Expect(foundCount).To(Equal(1),
				fmt.Sprintf("One Pod readiness gate is expected. Found %d times", foundCount))

			status := utils.FindPodStatusCondition(pod.Status.Conditions, pct)
			g.Expect(status).ToNot(BeNil(), "Pod status should not be nil")
			g.Expect(status.Status).To(Equal(corev1.ConditionTrue), "Pod status should be true")
		}).WithTimeout(180 * time.Second).WithOffset(1).Should(Succeed())
	})

	AfterAll(func() {
		testFramework.ExpectDeletedThenNotFound(ctx, untaggedNS, taggedNS)
	})
})
