package webhook

import (
	"fmt"
	"github.com/aws/aws-application-networking-k8s/pkg/webhook"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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
				"aws-application-networking-k8s/pod-readiness-gate-inject": "enabled",
			},
		},
	}

	BeforeAll(func() {
		err1 := testFramework.Client.Create(ctx, untaggedNS)
		err2 := testFramework.Client.Create(ctx, taggedNS)
		if err1 != nil || err2 != nil {
			Fail("unable to create test namespaces")
		}
	})

	It("create deployment in untagged namespace, no readiness gate", func() {
		deployment, _ := testFramework.NewHttpApp(test.HTTPAppOptions{Name: "untagged-test-pod", Namespace: untaggedNS.Name})
		testFramework.ExpectCreated(ctx, deployment)
		testFramework.Get(ctx, types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, deployment)

		pods := testFramework.GetPodsByDeploymentName(deployment.Name, deployment.Namespace)
		Expect(len(pods)).To(BeEquivalentTo(1))

		pod := pods[0]
		pct := corev1.PodConditionType(webhook.PodReadinessGateConditionType)

		for _, rg := range pod.Spec.ReadinessGates {
			if rg.ConditionType == pct {
				Fail("Pod readiness gate was injected to unlabeled namespace")
			}
		}
	})

	It("create deployment in tagged namespace, has readiness gate", func() {
		deployment, _ := testFramework.NewHttpApp(test.HTTPAppOptions{Name: "tagged-test-pod", Namespace: taggedNS.Name})
		testFramework.ExpectCreated(ctx, deployment)
		testFramework.Get(ctx, types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, deployment)

		pods := testFramework.GetPodsByDeploymentName(deployment.Name, deployment.Namespace)
		Expect(len(pods)).To(BeEquivalentTo(1))

		pod := pods[0]
		pct := corev1.PodConditionType(webhook.PodReadinessGateConditionType)

		foundCount := 0
		for _, rg := range pod.Spec.ReadinessGates {
			if rg.ConditionType == pct {
				foundCount++
			}
		}

		if foundCount != 1 {
			Fail(fmt.Sprintf("Pod readiness gate was expected on labeled namespace. Found %d times", foundCount))
		}
	})

	AfterAll(func() {
		err1 := testFramework.Client.Delete(ctx, untaggedNS)
		err2 := testFramework.Client.Delete(ctx, taggedNS)
		if err1 != nil || err2 != nil {
			testFramework.Log.Warn("unable to delete test namespaces")
		}
	})
})
