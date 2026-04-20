package integration

import (
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"

	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = Describe("Drift detection", Ordered, func() {
	var (
		deployment *appsv1.Deployment
		service    *v1.Service
		httpRoute  *gwv1.HTTPRoute
		route      core.Route
	)

	BeforeAll(func() {
		if config.ReconcileDefaultResyncSeconds <= 0 {
			Fail("RECONCILE_DEFAULT_RESYNC_SECONDS must be set to a positive value for drift detection tests. " +
				"Ensure both the controller and test runner are started with e.g. RECONCILE_DEFAULT_RESYNC_SECONDS=60")
		}

		deployment, service = testFramework.NewNginxApp(test.ElasticSearchOptions{
			Name:      "drift-test",
			Namespace: k8snamespace,
		})
		httpRoute = testFramework.NewHttpRoute(testGateway, service, "Service")
		testFramework.ExpectCreated(ctx, deployment, service, httpRoute)

		route = core.NewHTTPRoute(gwv1.HTTPRoute(*httpRoute))
		// Wait for Lattice service to be fully active
		testFramework.GetVpcLatticeService(ctx, route)
	})

	It("recreates a Lattice service deleted out-of-band", func() {
		svc := testFramework.GetVpcLatticeService(ctx, route)
		Expect(svc).ToNot(BeNil())
		originalId := aws.StringValue(svc.Id)

		// Delete the service out-of-band: disassociate then delete
		assocResp, err := testFramework.LatticeClient.ListServiceNetworkServiceAssociations(
			&vpclattice.ListServiceNetworkServiceAssociationsInput{
				ServiceIdentifier: svc.Id,
			})
		Expect(err).ToNot(HaveOccurred())

		for _, assoc := range assocResp.Items {
			_, err := testFramework.LatticeClient.DeleteServiceNetworkServiceAssociation(
				&vpclattice.DeleteServiceNetworkServiceAssociationInput{
					ServiceNetworkServiceAssociationIdentifier: assoc.Id,
				})
			Expect(err).ToNot(HaveOccurred())
		}
		if len(assocResp.Items) > 0 {
			time.Sleep(30 * time.Second)
		}

		_, err = testFramework.LatticeClient.DeleteService(&vpclattice.DeleteServiceInput{
			ServiceIdentifier: svc.Id,
		})
		Expect(err).ToNot(HaveOccurred())

		// Wait for drift detection to recreate it with a new ID
		Eventually(func(g Gomega) {
			recreated := testFramework.GetVpcLatticeService(ctx, route)
			g.Expect(recreated).ToNot(BeNil())
			g.Expect(*recreated.Status).To(Equal(vpclattice.ServiceStatusActive))
			g.Expect(aws.StringValue(recreated.Id)).ToNot(Equal(originalId))
		}).WithTimeout(5 * time.Minute).WithPolling(15 * time.Second).Should(Succeed())
	})

	It("recreates a listener deleted out-of-band", func() {
		svc := testFramework.GetVpcLatticeService(ctx, route)
		Expect(svc).ToNot(BeNil())

		// Wait for an HTTP listener on port 80 to exist
		var listener *vpclattice.ListenerSummary
		Eventually(func(g Gomega) {
			listenersResp, err := testFramework.LatticeClient.ListListenersWithContext(ctx,
				&vpclattice.ListListenersInput{
					ServiceIdentifier: svc.Id,
				})
			g.Expect(err).ToNot(HaveOccurred())
			for _, l := range listenersResp.Items {
				if aws.Int64Value(l.Port) == 80 {
					listener = l
				}
			}
			g.Expect(listener).ToNot(BeNil())
		}).WithTimeout(5 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())

		// Delete the listener out-of-band
		_, err := testFramework.LatticeClient.DeleteListenerWithContext(ctx,
			&vpclattice.DeleteListenerInput{
				ServiceIdentifier:  svc.Id,
				ListenerIdentifier: listener.Id,
			})
		Expect(err).ToNot(HaveOccurred())

		originalListenerId := aws.StringValue(listener.Id)

		// Wait for drift detection to recreate a listener on port 80 with a new ID
		Eventually(func(g Gomega) {
			listenersResp, err := testFramework.LatticeClient.ListListenersWithContext(ctx,
				&vpclattice.ListListenersInput{
					ServiceIdentifier: svc.Id,
				})
			g.Expect(err).ToNot(HaveOccurred())
			var found *vpclattice.ListenerSummary
			for _, l := range listenersResp.Items {
				if aws.Int64Value(l.Port) == 80 && aws.StringValue(l.Id) != originalListenerId {
					found = l
				}
			}
			g.Expect(found).ToNot(BeNil())
		}).WithTimeout(5 * time.Minute).WithPolling(15 * time.Second).Should(Succeed())
	})

	It("recreates a rule deleted out-of-band", func() {
		svc := testFramework.GetVpcLatticeService(ctx, route)
		Expect(svc).ToNot(BeNil())

		// Wait for listener to exist
		var listener *vpclattice.ListenerSummary
		Eventually(func(g Gomega) {
			listenersResp, err := testFramework.LatticeClient.ListListenersWithContext(ctx,
				&vpclattice.ListListenersInput{
					ServiceIdentifier: svc.Id,
				})
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(listenersResp.Items).ToNot(BeEmpty())
			listener = listenersResp.Items[0]
		}).WithTimeout(5 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())

		// Wait for non-default rules to exist
		var nonDefaultRules []*vpclattice.RuleSummary
		Eventually(func(g Gomega) {
			rulesResp, err := testFramework.LatticeClient.ListRulesWithContext(ctx,
				&vpclattice.ListRulesInput{
					ServiceIdentifier:  svc.Id,
					ListenerIdentifier: listener.Id,
				})
			g.Expect(err).ToNot(HaveOccurred())
			nonDefaultRules = nil
			for _, rule := range rulesResp.Items {
				if !aws.BoolValue(rule.IsDefault) {
					nonDefaultRules = append(nonDefaultRules, rule)
				}
			}
			g.Expect(nonDefaultRules).ToNot(BeEmpty())
		}).WithTimeout(5 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())

		originalRuleId := aws.StringValue(nonDefaultRules[0].Id)

		// Delete the rule out-of-band
		_, err := testFramework.LatticeClient.DeleteRuleWithContext(ctx,
			&vpclattice.DeleteRuleInput{
				ServiceIdentifier:  svc.Id,
				ListenerIdentifier: listener.Id,
				RuleIdentifier:     nonDefaultRules[0].Id,
			})
		Expect(err).ToNot(HaveOccurred())

		// Wait for drift detection to recreate the rule with a new ID
		Eventually(func(g Gomega) {
			resp, err := testFramework.LatticeClient.ListRulesWithContext(ctx,
				&vpclattice.ListRulesInput{
					ServiceIdentifier:  svc.Id,
					ListenerIdentifier: listener.Id,
				})
			g.Expect(err).ToNot(HaveOccurred())
			var newNonDefault []*vpclattice.RuleSummary
			for _, rule := range resp.Items {
				if !aws.BoolValue(rule.IsDefault) {
					newNonDefault = append(newNonDefault, rule)
				}
			}
			g.Expect(newNonDefault).To(HaveLen(len(nonDefaultRules)))
			// The recreated rule should have a different ID
			for _, rule := range newNonDefault {
				if aws.StringValue(rule.Id) != originalRuleId {
					return // found a new rule, success
				}
			}
			g.Expect(false).To(BeTrue(), "expected a rule with a different ID than the deleted one")
		}).WithTimeout(5 * time.Minute).WithPolling(15 * time.Second).Should(Succeed())
	})

	It("reverts a rule action modified out-of-band", func() {
		svc := testFramework.GetVpcLatticeService(ctx, route)
		Expect(svc).ToNot(BeNil())

		// Wait for listener to exist
		var listener *vpclattice.ListenerSummary
		Eventually(func(g Gomega) {
			listenersResp, err := testFramework.LatticeClient.ListListenersWithContext(ctx,
				&vpclattice.ListListenersInput{
					ServiceIdentifier: svc.Id,
				})
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(listenersResp.Items).ToNot(BeEmpty())
			listener = listenersResp.Items[0]
		}).WithTimeout(5 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())

		// Wait for non-default rules to exist
		var targetRule *vpclattice.RuleSummary
		Eventually(func(g Gomega) {
			rulesResp, err := testFramework.LatticeClient.ListRulesWithContext(ctx,
				&vpclattice.ListRulesInput{
					ServiceIdentifier:  svc.Id,
					ListenerIdentifier: listener.Id,
				})
			g.Expect(err).ToNot(HaveOccurred())
			for _, rule := range rulesResp.Items {
				if !aws.BoolValue(rule.IsDefault) {
					targetRule = rule
					break
				}
			}
			g.Expect(targetRule).ToNot(BeNil())
		}).WithTimeout(5 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())

		// Get the full rule to capture the original action
		originalRule, err := testFramework.LatticeClient.GetRuleWithContext(ctx,
			&vpclattice.GetRuleInput{
				ServiceIdentifier:  svc.Id,
				ListenerIdentifier: listener.Id,
				RuleIdentifier:     targetRule.Id,
			})
		Expect(err).ToNot(HaveOccurred())
		Expect(originalRule.Action).ToNot(BeNil())
		Expect(originalRule.Action.Forward).ToNot(BeNil())
		Expect(originalRule.Action.Forward.TargetGroups).ToNot(BeEmpty())

		originalTargetGroupArn := aws.StringValue(originalRule.Action.Forward.TargetGroups[0].TargetGroupIdentifier)

		// Modify the rule out-of-band: replace the action with a fixed 404 response
		_, err = testFramework.LatticeClient.UpdateRuleWithContext(ctx,
			&vpclattice.UpdateRuleInput{
				ServiceIdentifier:  svc.Id,
				ListenerIdentifier: listener.Id,
				RuleIdentifier:     targetRule.Id,
				Action: &vpclattice.RuleAction{
					FixedResponse: &vpclattice.FixedResponseAction{
						StatusCode: aws.Int64(404),
					},
				},
			})
		Expect(err).ToNot(HaveOccurred())

		// Verify the rule was modified
		modifiedRule, err := testFramework.LatticeClient.GetRuleWithContext(ctx,
			&vpclattice.GetRuleInput{
				ServiceIdentifier:  svc.Id,
				ListenerIdentifier: listener.Id,
				RuleIdentifier:     targetRule.Id,
			})
		Expect(err).ToNot(HaveOccurred())
		Expect(modifiedRule.Action.FixedResponse).ToNot(BeNil())

		// Wait for drift detection to revert the action back to the original forward
		Eventually(func(g Gomega) {
			revertedRule, err := testFramework.LatticeClient.GetRuleWithContext(ctx,
				&vpclattice.GetRuleInput{
					ServiceIdentifier:  svc.Id,
					ListenerIdentifier: listener.Id,
					RuleIdentifier:     targetRule.Id,
				})
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(revertedRule.Action.Forward).ToNot(BeNil())
			g.Expect(revertedRule.Action.Forward.TargetGroups).ToNot(BeEmpty())
			g.Expect(aws.StringValue(revertedRule.Action.Forward.TargetGroups[0].TargetGroupIdentifier)).
				To(Equal(originalTargetGroupArn))
		}).WithTimeout(5 * time.Minute).WithPolling(15 * time.Second).Should(Succeed())
	})

	AfterAll(func() {
		testFramework.ExpectDeletedThenNotFound(ctx,
			httpRoute,
			deployment,
			service,
		)
	})
})
