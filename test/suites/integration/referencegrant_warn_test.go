package integration

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
)

var _ = Describe("ReferenceGrant Warn Test", Serial, Ordered, func() {
	var (
		backendNS  = "referencegrant-backend"
		namespace  *corev1.Namespace
		deployment *appsv1.Deployment
		service    *corev1.Service
		httpRoute  *gwv1.HTTPRoute
		grant      *gwv1.ReferenceGrant
	)

	warnEventCount := func(g Gomega) int {
		events := &corev1.EventList{}
		g.Expect(testFramework.List(ctx, events, client.InNamespace(k8snamespace))).To(Succeed())
		count := 0
		for _, event := range events.Items {
			if event.InvolvedObject.UID == httpRoute.UID &&
				event.Reason == k8s.RouteEventReasonCrossNamespaceRefNotPermitted {
				count += int(event.Count)
			}
		}
		return count
	}

	resolvedRefsStaysTrue := func(g Gomega) {
		route := &gwv1.HTTPRoute{}
		g.Expect(testFramework.Get(ctx, client.ObjectKeyFromObject(httpRoute), route)).To(Succeed())
		g.Expect(route.Status.Parents).ToNot(BeEmpty())
		found := false
		for _, parent := range route.Status.Parents {
			for _, c := range parent.Conditions {
				if c.Type == string(gwv1.RouteConditionResolvedRefs) {
					found = true
					g.Expect(c.Status).To(Equal(metav1.ConditionTrue),
						"warn phase must not block: ResolvedRefs must stay True")
				}
			}
		}
		g.Expect(found).To(BeTrue(), "ResolvedRefs condition must be present")
	}

	BeforeAll(func() {
		namespace = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: backendNS}}
		testFramework.ExpectCreated(ctx, namespace)

		deployment, service = testFramework.NewNginxApp(test.ElasticSearchOptions{
			Name:      "referencegrant-backend",
			Namespace: backendNS,
		})
		testFramework.ExpectCreated(ctx, deployment, service)

		httpRoute = testFramework.NewHttpRoute(testGateway, service, "Service")
		httpRoute.Namespace = k8snamespace
		httpRoute.Name = "referencegrant-warn-route"
		testFramework.ExpectCreated(ctx, httpRoute)
	})

	AfterAll(func() {
		objs := []client.Object{httpRoute, deployment, service}
		if grant != nil {
			objs = append(objs, grant)
		}
		objs = append(objs, namespace)
		testFramework.ExpectDeletedThenNotFound(ctx, objs...)
	})

	It("warns but still resolves a cross-namespace backendRef with no ReferenceGrant", func() {
		Eventually(func(g Gomega) {
			g.Expect(warnEventCount(g)).To(BeNumerically(">", 0))
		}).Should(Succeed())

		Consistently(resolvedRefsStaysTrue).WithTimeout(30 * time.Second).WithPolling(5 * time.Second).Should(Succeed())
	})

	It("stops warning once a ReferenceGrant permits the reference", func() {
		Eventually(func(g Gomega) {
			g.Expect(warnEventCount(g)).To(BeNumerically(">", 0))
		}).Should(Succeed())

		grant = &gwv1.ReferenceGrant{
			ObjectMeta: metav1.ObjectMeta{Name: "allow-route", Namespace: backendNS},
			Spec: gwv1.ReferenceGrantSpec{
				From: []gwv1.ReferenceGrantFrom{{
					Group:     gwv1.Group(gwv1.GroupName),
					Kind:      gwv1.Kind("HTTPRoute"),
					Namespace: gwv1.Namespace(k8snamespace),
				}},
				To: []gwv1.ReferenceGrantTo{{
					Group: gwv1.Group(""),
					Kind:  gwv1.Kind("Service"),
					Name:  lo.ToPtr(gwv1.ObjectName(service.Name)),
				}},
			},
		}
		testFramework.ExpectCreated(ctx, grant)

		// Force a reconcile that observes the grant via a real spec change (a non-default
		// backendRef weight bumps metadata.generation, which the route predicate accepts),
		// then wait until the controller has processed that generation.
		var wantGen int64
		Eventually(func(g Gomega) {
			route := &gwv1.HTTPRoute{}
			g.Expect(testFramework.Get(ctx, client.ObjectKeyFromObject(httpRoute), route)).To(Succeed())
			prevGen := route.Generation
			route.Spec.Rules[0].BackendRefs[0].Weight = lo.ToPtr(int32(2))
			g.Expect(testFramework.Update(ctx, route)).To(Succeed())
			g.Expect(route.Generation).To(BeNumerically(">", prevGen))
			wantGen = route.Generation
		}).Should(Succeed())

		Eventually(func(g Gomega) {
			route := &gwv1.HTTPRoute{}
			g.Expect(testFramework.Get(ctx, client.ObjectKeyFromObject(httpRoute), route)).To(Succeed())
			g.Expect(route.Status.Parents).ToNot(BeEmpty())
			g.Expect(route.Status.Parents[0].Conditions).ToNot(BeEmpty())
			g.Expect(route.Status.Parents[0].Conditions[0].ObservedGeneration).To(BeNumerically(">=", wantGen))
		}).Should(Succeed())

		// Wait until the warning count is stable across two reads before snapshotting it:
		// validateRoute writes status before the warning is emitted and events are
		// delivered asynchronously, so observedGeneration alone does not prove the last
		// warning has been counted.
		baseline, prev := -1, -2
		Eventually(func(g Gomega) {
			cur := warnEventCount(g)
			stable := cur == prev
			prev = cur
			baseline = cur
			g.Expect(stable).To(BeTrue())
		}).WithPolling(10 * time.Second).WithTimeout(2 * time.Minute).Should(Succeed())

		// Force one more reconcile that must observe the now-established grant, so the
		// assertion fails if the grant is not honored (rather than passing because nothing
		// reconciled).
		var afterGen int64
		Eventually(func(g Gomega) {
			route := &gwv1.HTTPRoute{}
			g.Expect(testFramework.Get(ctx, client.ObjectKeyFromObject(httpRoute), route)).To(Succeed())
			prevGen := route.Generation
			route.Spec.Rules[0].BackendRefs[0].Weight = lo.ToPtr(int32(3))
			g.Expect(testFramework.Update(ctx, route)).To(Succeed())
			g.Expect(route.Generation).To(BeNumerically(">", prevGen))
			afterGen = route.Generation
		}).Should(Succeed())

		Eventually(func(g Gomega) {
			route := &gwv1.HTTPRoute{}
			g.Expect(testFramework.Get(ctx, client.ObjectKeyFromObject(httpRoute), route)).To(Succeed())
			g.Expect(route.Status.Parents).ToNot(BeEmpty())
			g.Expect(route.Status.Parents[0].Conditions).ToNot(BeEmpty())
			g.Expect(route.Status.Parents[0].Conditions[0].ObservedGeneration).To(BeNumerically(">=", afterGen))
		}).Should(Succeed())

		Consistently(func(g Gomega) {
			g.Expect(warnEventCount(g)).To(Equal(baseline))
		}).WithTimeout(30 * time.Second).WithPolling(5 * time.Second).Should(Succeed())
	})
})
