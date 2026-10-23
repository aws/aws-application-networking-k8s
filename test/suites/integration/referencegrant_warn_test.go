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
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
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

		// Warn phase must not block: the VPC Lattice service for the route is still created
		// even though the cross-namespace backendRef has no ReferenceGrant.
		Eventually(func(g Gomega) {
			g.Expect(testFramework.GetVpcLatticeService(ctx, core.NewHTTPRoute(*httpRoute))).ToNot(BeNil())
		}).Should(Succeed())
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

		// Remove the grant: the reference becomes unpermitted again, which clears the
		// dedup entry the honored reconcile stored and re-emits the warning. The grant
		// deletion alone must drive this (no forced spec change), which exercises the
		// ReferenceGrant watch, its CRD gate, and the referencegrants RBAC rule. It also
		// fails if the grant was never actually honored (the dedup entry would still hold
		// the old message and suppress the event).
		testFramework.ExpectDeletedThenNotFound(ctx, grant)
		grant = nil

		Eventually(func(g Gomega) {
			g.Expect(warnEventCount(g)).To(BeNumerically(">", baseline))
		}).Should(Succeed())
	})

	It("warns but does not block a cross-namespace ServiceImport backendRef with no ReferenceGrant", func() {
		// ServiceImport resolves on a different code path than Service and is the
		// higher-severity cross-cluster vector, so exercise the warn contract for it too.
		// The backendRef uses kind ServiceImport with the group omitted (the common form),
		// which the controller normalizes to the application networking group.
		siImport := testFramework.CreateServiceImport(service) // named service.Name in backendNS
		siRoute := testFramework.NewHttpRoute(testGateway, service, "ServiceImport")
		siRoute.Namespace = k8snamespace
		siRoute.Name = "referencegrant-warn-si-route"
		var siGrant *gwv1.ReferenceGrant

		testFramework.ExpectCreated(ctx, siImport, siRoute)
		DeferCleanup(func() {
			objs := []client.Object{siRoute, siImport}
			if siGrant != nil {
				objs = append(objs, siGrant)
			}
			testFramework.ExpectDeletedThenNotFound(ctx, objs...)
		})

		siWarnCount := func(g Gomega) int {
			events := &corev1.EventList{}
			g.Expect(testFramework.List(ctx, events, client.InNamespace(k8snamespace))).To(Succeed())
			count := 0
			for _, event := range events.Items {
				if event.InvolvedObject.UID == siRoute.UID &&
					event.Reason == k8s.RouteEventReasonCrossNamespaceRefNotPermitted {
					count += int(event.Count)
				}
			}
			return count
		}

		// The ServiceImport cross-namespace ref with no grant is warned about.
		Eventually(func(g Gomega) {
			g.Expect(siWarnCount(g)).To(BeNumerically(">", 0))
		}).Should(Succeed())

		// Warn phase must not block: route status is left untouched. We do not assert that
		// a VPC Lattice service is created here — a bare ServiceImport with no backing
		// target group does not resolve to one, which is orthogonal to the warn behavior.
		// We do require the ResolvedRefs condition to be present so the "not False" check
		// below is not vacuously satisfied by an absent condition; we do not require True,
		// because an unbacked ServiceImport legitimately may not resolve.
		Consistently(func(g Gomega) {
			route := &gwv1.HTTPRoute{}
			g.Expect(testFramework.Get(ctx, client.ObjectKeyFromObject(siRoute), route)).To(Succeed())
			g.Expect(route.Status.Parents).ToNot(BeEmpty())
			found := false
			for _, parent := range route.Status.Parents {
				for _, c := range parent.Conditions {
					if c.Type == string(gwv1.RouteConditionResolvedRefs) {
						found = true
						g.Expect(c.Status).ToNot(Equal(metav1.ConditionFalse),
							"warn phase must not block: ResolvedRefs must not be set False")
					}
				}
			}
			g.Expect(found).To(BeTrue(), "ResolvedRefs condition must be present")
		}).WithTimeout(30 * time.Second).WithPolling(5 * time.Second).Should(Succeed())

		// A ReferenceGrant naming kind ServiceImport in the application networking group
		// permits the reference; the warning must stop.
		siGrant = &gwv1.ReferenceGrant{
			ObjectMeta: metav1.ObjectMeta{Name: "allow-si-route", Namespace: backendNS},
			Spec: gwv1.ReferenceGrantSpec{
				From: []gwv1.ReferenceGrantFrom{{
					Group:     gwv1.Group(gwv1.GroupName),
					Kind:      gwv1.Kind("HTTPRoute"),
					Namespace: gwv1.Namespace(k8snamespace),
				}},
				To: []gwv1.ReferenceGrantTo{{
					Group: gwv1.Group(anv1alpha1.GroupName),
					Kind:  gwv1.Kind("ServiceImport"),
				}},
			},
		}
		testFramework.ExpectCreated(ctx, siGrant)

		// Force a reconcile that observes the grant, then wait for the controller to
		// process that generation.
		var wantGen int64
		Eventually(func(g Gomega) {
			route := &gwv1.HTTPRoute{}
			g.Expect(testFramework.Get(ctx, client.ObjectKeyFromObject(siRoute), route)).To(Succeed())
			prevGen := route.Generation
			route.Spec.Rules[0].BackendRefs[0].Weight = lo.ToPtr(int32(2))
			g.Expect(testFramework.Update(ctx, route)).To(Succeed())
			g.Expect(route.Generation).To(BeNumerically(">", prevGen))
			wantGen = route.Generation
		}).Should(Succeed())

		Eventually(func(g Gomega) {
			route := &gwv1.HTTPRoute{}
			g.Expect(testFramework.Get(ctx, client.ObjectKeyFromObject(siRoute), route)).To(Succeed())
			g.Expect(route.Status.Parents).ToNot(BeEmpty())
			g.Expect(route.Status.Parents[0].Conditions).ToNot(BeEmpty())
			g.Expect(route.Status.Parents[0].Conditions[0].ObservedGeneration).To(BeNumerically(">=", wantGen))
		}).Should(Succeed())

		// After the grant is observed, the warning count stops growing.
		baseline, prev := -1, -2
		Eventually(func(g Gomega) {
			cur := siWarnCount(g)
			stable := cur == prev
			prev = cur
			baseline = cur
			g.Expect(stable).To(BeTrue())
		}).WithPolling(10 * time.Second).WithTimeout(2 * time.Minute).Should(Succeed())

		Consistently(func(g Gomega) {
			g.Expect(siWarnCount(g)).To(Equal(baseline))
		}).WithTimeout(30 * time.Second).WithPolling(5 * time.Second).Should(Succeed())

		// Remove the grant: the ServiceImport reference becomes unpermitted again, which
		// clears the dedup entry the honored reconcile stored and re-emits the warning.
		// The grant deletion alone drives this (no forced spec change), exercising the
		// ReferenceGrant watch, its CRD gate, and the referencegrants RBAC rule on the
		// ServiceImport path. It also fails if the grant was never actually honored (the
		// dedup entry would still hold the old message and suppress the event).
		testFramework.ExpectDeletedThenNotFound(ctx, siGrant)
		siGrant = nil

		Eventually(func(g Gomega) {
			g.Expect(siWarnCount(g)).To(BeNumerically(">", baseline))
		}).Should(Succeed())
	})
})
