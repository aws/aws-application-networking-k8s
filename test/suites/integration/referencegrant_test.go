package integration

import (
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/vpclattice"
	"github.com/aws/aws-sdk-go-v2/service/vpclattice/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
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

	// resolvedRefs returns the route's ResolvedRefs condition and checks that the route is
	// still Accepted, since a blocked backendRef must not reject the route.
	resolvedRefs := func(g Gomega, route client.Object) metav1.Condition {
		g.Expect(testFramework.Get(ctx, client.ObjectKeyFromObject(route), route)).To(Succeed())
		var parents []gwv1.RouteParentStatus
		switch r := route.(type) {
		case *gwv1.HTTPRoute:
			parents = r.Status.Parents
		case *gwv1.TLSRoute:
			parents = r.Status.Parents
		}
		g.Expect(parents).ToNot(BeEmpty())
		var found *metav1.Condition
		for i, c := range parents[0].Conditions {
			if c.Type == string(gwv1.RouteConditionAccepted) {
				g.Expect(c.Status).To(Equal(metav1.ConditionTrue), "a blocked backendRef must not reject the route")
			}
			if c.Type == string(gwv1.RouteConditionResolvedRefs) {
				found = &parents[0].Conditions[i]
			}
		}
		g.Expect(found).ToNot(BeNil(), "ResolvedRefs condition must be present")
		return *found
	}

	expectRefNotPermitted := func(g Gomega, route client.Object) {
		c := resolvedRefs(g, route)
		g.Expect(c.Status).To(Equal(metav1.ConditionFalse))
		g.Expect(c.Reason).To(Equal(string(gwv1.RouteReasonRefNotPermitted)))
	}

	// ruleActions returns the action of every non-default rule on the route's VPC Lattice
	// service.
	ruleActions := func(g Gomega, route core.Route) []types.RuleAction {
		svc := testFramework.GetVpcLatticeService(ctx, route)
		listeners, err := testFramework.LatticeClient.ListListenersAsList(ctx,
			&vpclattice.ListListenersInput{ServiceIdentifier: svc.Id})
		g.Expect(err).ToNot(HaveOccurred())
		var actions []types.RuleAction
		for _, l := range listeners {
			rules, err := testFramework.LatticeClient.GetRulesAsList(ctx,
				&vpclattice.ListRulesInput{ServiceIdentifier: svc.Id, ListenerIdentifier: l.Id})
			g.Expect(err).ToNot(HaveOccurred())
			for _, r := range rules {
				// GetRule omits isDefault on non-default rules.
				if !aws.ToBool(r.IsDefault) {
					actions = append(actions, r.Action)
				}
			}
		}
		return actions
	}

	// expectRulesReturn500 checks that the route's rules send no traffic to any backend.
	expectRulesReturn500 := func(g Gomega, route core.Route) {
		actions := ruleActions(g, route)
		g.Expect(actions).ToNot(BeEmpty())
		for _, a := range actions {
			fixed, ok := a.(*types.RuleActionMemberFixedResponse)
			g.Expect(ok).To(BeTrue(), "a blocked rule must not forward")
			g.Expect(*fixed.Value.StatusCode).To(BeEquivalentTo(500))
		}
	}

	// expectRulesForwardTo checks that a rule forwards to the given target group. GetRule may
	// return either its id or its ARN, so both are accepted.
	expectRulesForwardTo := func(g Gomega, route core.Route, tgId, tgArn *string) {
		var forwardedTo []string
		for _, a := range ruleActions(g, route) {
			if fwd, ok := a.(*types.RuleActionMemberForward); ok {
				for _, wtg := range fwd.Value.TargetGroups {
					forwardedTo = append(forwardedTo, aws.ToString(wtg.TargetGroupIdentifier))
				}
			}
		}
		g.Expect(forwardedTo).To(ContainElement(BeElementOf(aws.ToString(tgId), aws.ToString(tgArn))),
			"a permitted rule must forward to the backend's target group")
	}

	tlsPassthroughListeners := func(g Gomega, route core.Route) int {
		svc := testFramework.GetVpcLatticeService(ctx, route)
		listeners, err := testFramework.LatticeClient.ListListenersAsList(ctx,
			&vpclattice.ListListenersInput{ServiceIdentifier: svc.Id})
		g.Expect(err).ToNot(HaveOccurred())
		return len(lo.Filter(listeners, func(l types.ListenerSummary, _ int) bool {
			return l.Protocol == types.ListenerProtocolTlsPassthrough
		}))
	}

	newGrant := func(name, fromKind string, to gwv1.ReferenceGrantTo) *gwv1.ReferenceGrant {
		return &gwv1.ReferenceGrant{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: backendNS},
			Spec: gwv1.ReferenceGrantSpec{
				From: []gwv1.ReferenceGrantFrom{{
					Group:     gwv1.Group(gwv1.GroupName),
					Kind:      gwv1.Kind(fromKind),
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
		coreRoute := core.NewHTTPRoute(*route)

		// The route stays Accepted and gets a VPC Lattice service, but its rule returns 500
		// and no target group is created for the blocked backend.
		Eventually(func(g Gomega) {
			expectRefNotPermitted(g, route)
			expectRulesReturn500(g, coreRoute)
		}).WithTimeout(3 * time.Minute).Should(Succeed())
		tgSpec := model.TargetGroupSpec{
			TargetGroupTagFields: model.TargetGroupTagFields{
				K8SServiceName:      service.Name,
				K8SServiceNamespace: service.Namespace,
				K8SRouteName:        route.Name,
				K8SRouteNamespace:   route.Namespace,
			},
			Protocol:        string(types.TargetGroupProtocolHttp),
			ProtocolVersion: string(types.TargetGroupProtocolVersionHttp1),
		}
		tg, err := testFramework.FindTargetGroupFromSpec(ctx, tgSpec)
		Expect(err).ToNot(HaveOccurred())
		Expect(tg).To(BeNil(), "no target group may be created for a blocked backend")

		// Creating and deleting the grant takes effect through the ReferenceGrant watch
		// alone, with no change to the route.
		grant = newGrant("allow-route", "HTTPRoute", gwv1.ReferenceGrantTo{
			Group: gwv1.Group(""),
			Kind:  gwv1.Kind("Service"),
			Name:  lo.ToPtr(gwv1.ObjectName(service.Name)),
		})
		testFramework.ExpectCreated(ctx, grant)
		Eventually(func(g Gomega) {
			c := resolvedRefs(g, route)
			g.Expect(c.Status).To(Equal(metav1.ConditionTrue))
			tg, err := testFramework.FindTargetGroupFromSpec(ctx, tgSpec)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(tg).ToNot(BeNil())
			expectRulesForwardTo(g, coreRoute, tg.Id, tg.Arn)
		}).WithTimeout(3 * time.Minute).Should(Succeed())

		testFramework.ExpectDeletedThenNotFound(ctx, grant)
		grant = nil
		Eventually(func(g Gomega) {
			expectRefNotPermitted(g, route)
			expectRulesReturn500(g, coreRoute)
		}).WithTimeout(3 * time.Minute).Should(Succeed())
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
		coreRoute := core.NewHTTPRoute(*route)

		Eventually(func(g Gomega) {
			expectRefNotPermitted(g, route)
			expectRulesReturn500(g, coreRoute)
		}).WithTimeout(3 * time.Minute).Should(Succeed())

		// The ServiceImport exists, so ResolvedRefs becomes True once the grant permits it.
		// We do not check forwarding: a bare ServiceImport has no exported target group, so
		// the deploy fails before the rule is updated, which is unrelated to the grant.
		grant = newGrant("allow-si-route", "HTTPRoute", gwv1.ReferenceGrantTo{
			Group: gwv1.Group(anv1alpha1.GroupName),
			Kind:  gwv1.Kind("ServiceImport"),
		})
		testFramework.ExpectCreated(ctx, grant)
		Eventually(func(g Gomega) {
			g.Expect(resolvedRefs(g, route).Status).To(Equal(metav1.ConditionTrue))
		}).WithTimeout(3 * time.Minute).Should(Succeed())

		// For the same reason, the rule still returns 500 from the blocked phase, so only the
		// status shows that revoking the grant took effect.
		testFramework.ExpectDeletedThenNotFound(ctx, grant)
		grant = nil
		Eventually(func(g Gomega) {
			expectRefNotPermitted(g, route)
		}).WithTimeout(3 * time.Minute).Should(Succeed())
	})

	It("removes the TLS passthrough listener when the ReferenceGrant is deleted", func() {
		httpsDeployment, httpsSvc := testFramework.NewHttpsApp(test.HTTPsAppOptions{
			Name:      "referencegrant-tls-backend",
			Namespace: backendNS,
		})
		grant := newGrant("allow-tls-route", "TLSRoute", gwv1.ReferenceGrantTo{
			Group: gwv1.Group(""),
			Kind:  gwv1.Kind("Service"),
		})
		route := testFramework.NewTLSRoute(k8snamespace, testGateway, []gwv1.TLSRouteRule{{
			BackendRefs: []gwv1.BackendRef{{
				BackendObjectReference: gwv1.BackendObjectReference{
					Name:      gwv1.ObjectName(httpsSvc.Name),
					Namespace: lo.ToPtr(gwv1.Namespace(backendNS)),
					Kind:      lo.ToPtr(gwv1.Kind("Service")),
					Port:      lo.ToPtr(gwv1.PortNumber(443)),
				},
			}},
		}})
		route.Name = "referencegrant-tls-route"
		route.Spec.Hostnames = []gwv1.Hostname{"referencegrant-tls-test.com"}
		testFramework.ExpectCreated(ctx, httpsDeployment, httpsSvc, grant, route)
		deleteGrant := true
		DeferCleanup(func() {
			objs := []client.Object{route, httpsDeployment, httpsSvc}
			if deleteGrant {
				objs = append(objs, grant)
			}
			testFramework.ExpectDeletedThenNotFound(ctx, objs...)
		})
		coreRoute, _ := core.NewRoute(route)

		Eventually(func(g Gomega) {
			g.Expect(resolvedRefs(g, route).Status).To(Equal(metav1.ConditionTrue))
			g.Expect(tlsPassthroughListeners(g, coreRoute)).To(Equal(1))
		}).WithTimeout(3 * time.Minute).Should(Succeed())

		// A TLS passthrough listener can only forward, so revoking the grant must remove it.
		testFramework.ExpectDeletedThenNotFound(ctx, grant)
		deleteGrant = false
		Eventually(func(g Gomega) {
			expectRefNotPermitted(g, route)
			g.Expect(tlsPassthroughListeners(g, coreRoute)).To(Equal(0))
		}).WithTimeout(3 * time.Minute).Should(Succeed())
	})
})
