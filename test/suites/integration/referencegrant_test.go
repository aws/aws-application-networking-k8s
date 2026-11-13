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

	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
)

var _ = Describe("ReferenceGrant Test", Serial, Ordered, func() {
	var (
		backendNS  = "referencegrant-backend"
		namespace  *corev1.Namespace
		deployment *appsv1.Deployment
		service    *corev1.Service
	)

	resolvedRefs := func(g Gomega, route *gwv1.HTTPRoute) metav1.Condition {
		got := &gwv1.HTTPRoute{}
		g.Expect(testFramework.Get(ctx, client.ObjectKeyFromObject(route), got)).To(Succeed())
		g.Expect(got.Status.Parents).ToNot(BeEmpty())
		for _, c := range got.Status.Parents[0].Conditions {
			if c.Type == string(gwv1.RouteConditionAccepted) {
				g.Expect(c.Status).To(Equal(metav1.ConditionTrue), "a blocked backendRef must not reject the route")
			}
		}
		for _, c := range got.Status.Parents[0].Conditions {
			if c.Type == string(gwv1.RouteConditionResolvedRefs) {
				return c
			}
		}
		g.Expect(false).To(BeTrue(), "ResolvedRefs condition must be present")
		return metav1.Condition{}
	}

	expectBlocked := func(route *gwv1.HTTPRoute) {
		Eventually(func(g Gomega) {
			c := resolvedRefs(g, route)
			g.Expect(c.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(c.Reason).To(Equal(string(gwv1.RouteReasonRefNotPermitted)))
		}).WithTimeout(2 * time.Minute).Should(Succeed())
	}

	expectNotBlocked := func(route *gwv1.HTTPRoute) {
		Eventually(func(g Gomega) {
			g.Expect(resolvedRefs(g, route).Reason).ToNot(Equal(string(gwv1.RouteReasonRefNotPermitted)))
		}).WithTimeout(2 * time.Minute).Should(Succeed())
	}

	newGrant := func(name string, to gwv1.ReferenceGrantTo) *gwv1.ReferenceGrant {
		return &gwv1.ReferenceGrant{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: backendNS},
			Spec: gwv1.ReferenceGrantSpec{
				From: []gwv1.ReferenceGrantFrom{{
					Group:     gwv1.Group(gwv1.GroupName),
					Kind:      gwv1.Kind("HTTPRoute"),
					Namespace: gwv1.Namespace(k8snamespace),
				}},
				To: []gwv1.ReferenceGrantTo{to},
			},
		}
	}

	BeforeAll(func() {
		namespace = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: backendNS}}
		testFramework.ExpectCreated(ctx, namespace)

		deployment, service = testFramework.NewNginxApp(test.ElasticSearchOptions{
			Name:      "referencegrant-backend",
			Namespace: backendNS,
		})
		testFramework.ExpectCreated(ctx, deployment, service)
	})

	AfterAll(func() {
		testFramework.ExpectDeletedThenNotFound(ctx, deployment, service, namespace)
	})

	It("blocks a cross-namespace Service backendRef until a ReferenceGrant permits it", func() {
		route := testFramework.NewHttpRoute(testGateway, service, "Service")
		route.Namespace = k8snamespace
		route.Name = "referencegrant-route"
		testFramework.ExpectCreated(ctx, route)
		var grant *gwv1.ReferenceGrant
		DeferCleanup(func() {
			objs := []client.Object{route}
			if grant != nil {
				objs = append(objs, grant)
			}
			testFramework.ExpectDeletedThenNotFound(ctx, objs...)
		})

		expectBlocked(route)

		// The route is still accepted, so its VPC Lattice service exists; the blocked
		// backend gets no target group.
		Eventually(func(g Gomega) {
			g.Expect(testFramework.GetVpcLatticeService(ctx, core.NewHTTPRoute(*route))).ToNot(BeNil())
		}).Should(Succeed())

		// Creating and deleting the grant drives the change through the ReferenceGrant
		// watch alone, with no change to the route.
		grant = newGrant("allow-route", gwv1.ReferenceGrantTo{
			Group: gwv1.Group(""),
			Kind:  gwv1.Kind("Service"),
			Name:  lo.ToPtr(gwv1.ObjectName(service.Name)),
		})
		testFramework.ExpectCreated(ctx, grant)
		expectNotBlocked(route)

		testFramework.ExpectDeletedThenNotFound(ctx, grant)
		grant = nil
		expectBlocked(route)
	})

	It("blocks a cross-namespace ServiceImport backendRef until a ReferenceGrant permits it", func() {
		// The backendRef leaves the group out, the common form. A grant must still name
		// the application networking group.
		svcImport := testFramework.CreateServiceImport(service)
		route := testFramework.NewHttpRoute(testGateway, service, "ServiceImport")
		route.Namespace = k8snamespace
		route.Name = "referencegrant-si-route"
		testFramework.ExpectCreated(ctx, svcImport, route)
		var grant *gwv1.ReferenceGrant
		DeferCleanup(func() {
			objs := []client.Object{route, svcImport}
			if grant != nil {
				objs = append(objs, grant)
			}
			testFramework.ExpectDeletedThenNotFound(ctx, objs...)
		})

		expectBlocked(route)

		// We do not assert ResolvedRefs True here: a bare ServiceImport with no backing
		// target group may not resolve, which is unrelated to the grant.
		grant = newGrant("allow-si-route", gwv1.ReferenceGrantTo{
			Group: gwv1.Group(anv1alpha1.GroupName),
			Kind:  gwv1.Kind("ServiceImport"),
		})
		testFramework.ExpectCreated(ctx, grant)
		expectNotBlocked(route)

		testFramework.ExpectDeletedThenNotFound(ctx, grant)
		grant = nil
		expectBlocked(route)
	})
})
