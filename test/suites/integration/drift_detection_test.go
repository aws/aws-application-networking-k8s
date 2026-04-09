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
		if config.ReconcileDefaultResyncInterval <= 0 {
			Skip("RECONCILE_DEFAULT_RESYNC_SECONDS not set or 0, skipping drift detection tests")
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
		originalId := aws.ToString(svc.Id)

		// Delete the service out-of-band: disassociate then delete
		associations, err := testFramework.LatticeClient.ListServiceNetworkServiceAssociationsAsList(ctx,
			&vpclattice.ListServiceNetworkServiceAssociationsInput{
				ServiceIdentifier: svc.Id,
			})
		Expect(err).ToNot(HaveOccurred())

		for _, assoc := range associations {
			_, err := testFramework.LatticeClient.DeleteServiceNetworkServiceAssociation(ctx,
				&vpclattice.DeleteServiceNetworkServiceAssociationInput{
					ServiceNetworkServiceAssociationIdentifier: assoc.Id,
				})
			Expect(err).ToNot(HaveOccurred())
		}

		// Wait for all associations to be fully deleted
		Eventually(func(g Gomega) {
			resp, err := testFramework.LatticeClient.ListServiceNetworkServiceAssociationsAsList(ctx,
				&vpclattice.ListServiceNetworkServiceAssociationsInput{
					ServiceIdentifier: svc.Id,
				})
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(resp).To(BeEmpty())
		}).WithTimeout(2 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())

		_, err = testFramework.LatticeClient.DeleteService(ctx, &vpclattice.DeleteServiceInput{
			ServiceIdentifier: svc.Id,
		})
		Expect(err).ToNot(HaveOccurred())

		// Wait for drift detection to recreate it with a new ID
		Eventually(func(g Gomega) {
			recreated := testFramework.GetVpcLatticeService(ctx, route)
			g.Expect(recreated).ToNot(BeNil())
			g.Expect(recreated.Status).To(Equal(types.ServiceStatusActive))
			g.Expect(aws.ToString(recreated.Id)).ToNot(Equal(originalId))
		}).WithTimeout(5 * time.Minute).WithPolling(15 * time.Second).Should(Succeed())
	})

	It("recreates a listener deleted out-of-band", func() {
		svc := testFramework.GetVpcLatticeService(ctx, route)
		Expect(svc).ToNot(BeNil())

		// Wait for an HTTP listener on port 80 to exist
		var listener *types.ListenerSummary
		Eventually(func(g Gomega) {
			listenersResp, err := testFramework.LatticeClient.ListListeners(ctx,
				&vpclattice.ListListenersInput{
					ServiceIdentifier: svc.Id,
				})
			g.Expect(err).ToNot(HaveOccurred())
			for i, l := range listenersResp.Items {
				if aws.ToInt32(l.Port) == 80 {
					listener = &listenersResp.Items[i]
				}
			}
			g.Expect(listener).ToNot(BeNil())
		}).WithTimeout(5 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())

		// Delete the listener out-of-band
		_, err := testFramework.LatticeClient.DeleteListener(ctx,
			&vpclattice.DeleteListenerInput{
				ServiceIdentifier:  svc.Id,
				ListenerIdentifier: listener.Id,
			})
		Expect(err).ToNot(HaveOccurred())

		originalListenerId := aws.ToString(listener.Id)

		// Wait for drift detection to recreate a listener on port 80 with a new ID
		Eventually(func(g Gomega) {
			listenersResp, err := testFramework.LatticeClient.ListListeners(ctx,
				&vpclattice.ListListenersInput{
					ServiceIdentifier: svc.Id,
				})
			g.Expect(err).ToNot(HaveOccurred())
			var found *types.ListenerSummary
			for i, l := range listenersResp.Items {
				if aws.ToInt32(l.Port) == 80 && aws.ToString(l.Id) != originalListenerId {
					found = &listenersResp.Items[i]
				}
			}
			g.Expect(found).ToNot(BeNil())
		}).WithTimeout(5 * time.Minute).WithPolling(15 * time.Second).Should(Succeed())
	})

	It("recreates a rule deleted out-of-band", func() {
		svc := testFramework.GetVpcLatticeService(ctx, route)
		Expect(svc).ToNot(BeNil())

		// Wait for listener to exist
		var listener *types.ListenerSummary
		Eventually(func(g Gomega) {
			listenersResp, err := testFramework.LatticeClient.ListListeners(ctx,
				&vpclattice.ListListenersInput{
					ServiceIdentifier: svc.Id,
				})
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(listenersResp.Items).ToNot(BeEmpty())
			listener = &listenersResp.Items[0]
		}).WithTimeout(5 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())

		// Wait for non-default rules to exist
		var nonDefaultRules []types.RuleSummary
		Eventually(func(g Gomega) {
			rules, err := testFramework.LatticeClient.ListRulesAsList(ctx,
				&vpclattice.ListRulesInput{
					ServiceIdentifier:  svc.Id,
					ListenerIdentifier: listener.Id,
				})
			g.Expect(err).ToNot(HaveOccurred())
			nonDefaultRules = nil
			for _, rule := range rules {
				if !aws.ToBool(rule.IsDefault) {
					nonDefaultRules = append(nonDefaultRules, rule)
				}
			}
			g.Expect(nonDefaultRules).ToNot(BeEmpty())
		}).WithTimeout(5 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())

		originalRuleId := aws.ToString(nonDefaultRules[0].Id)

		// Delete the rule out-of-band
		_, err := testFramework.LatticeClient.DeleteRule(ctx,
			&vpclattice.DeleteRuleInput{
				ServiceIdentifier:  svc.Id,
				ListenerIdentifier: listener.Id,
				RuleIdentifier:     nonDefaultRules[0].Id,
			})
		Expect(err).ToNot(HaveOccurred())

		// Wait for drift detection to recreate the rule with a new ID
		Eventually(func(g Gomega) {
			rules, err := testFramework.LatticeClient.ListRulesAsList(ctx,
				&vpclattice.ListRulesInput{
					ServiceIdentifier:  svc.Id,
					ListenerIdentifier: listener.Id,
				})
			g.Expect(err).ToNot(HaveOccurred())
			var newNonDefault []types.RuleSummary
			for _, rule := range rules {
				if !aws.ToBool(rule.IsDefault) {
					newNonDefault = append(newNonDefault, rule)
				}
			}
			g.Expect(newNonDefault).To(HaveLen(len(nonDefaultRules)))
			// The recreated rule should have a different ID
			ids := lo.Map(newNonDefault, func(r types.RuleSummary, _ int) string {
				return aws.ToString(r.Id)
			})
			g.Expect(ids).ToNot(ContainElement(originalRuleId))
		}).WithTimeout(5 * time.Minute).WithPolling(15 * time.Second).Should(Succeed())
	})

	It("reverts a rule action modified out-of-band", func() {
		svc := testFramework.GetVpcLatticeService(ctx, route)
		Expect(svc).ToNot(BeNil())

		// Wait for listener to exist
		var listener *types.ListenerSummary
		Eventually(func(g Gomega) {
			listenersResp, err := testFramework.LatticeClient.ListListeners(ctx,
				&vpclattice.ListListenersInput{
					ServiceIdentifier: svc.Id,
				})
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(listenersResp.Items).ToNot(BeEmpty())
			listener = &listenersResp.Items[0]
		}).WithTimeout(5 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())

		// Wait for non-default rules to exist
		var targetRule *types.RuleSummary
		Eventually(func(g Gomega) {
			rules, err := testFramework.LatticeClient.ListRulesAsList(ctx,
				&vpclattice.ListRulesInput{
					ServiceIdentifier:  svc.Id,
					ListenerIdentifier: listener.Id,
				})
			g.Expect(err).ToNot(HaveOccurred())
			for i, rule := range rules {
				if !aws.ToBool(rule.IsDefault) {
					targetRule = &rules[i]
					break
				}
			}
			g.Expect(targetRule).ToNot(BeNil())
		}).WithTimeout(5 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())

		// Get the full rule to capture the original action
		originalRule, err := testFramework.LatticeClient.GetRule(ctx,
			&vpclattice.GetRuleInput{
				ServiceIdentifier:  svc.Id,
				ListenerIdentifier: listener.Id,
				RuleIdentifier:     targetRule.Id,
			})
		Expect(err).ToNot(HaveOccurred())
		Expect(originalRule.Action).ToNot(BeNil())
		forwardAction, ok := originalRule.Action.(*types.RuleActionMemberForward)
		Expect(ok).To(BeTrue())
		Expect(forwardAction.Value.TargetGroups).ToNot(BeEmpty())

		originalTargetGroupArn := aws.ToString(forwardAction.Value.TargetGroups[0].TargetGroupIdentifier)

		// Modify the rule out-of-band: replace the action with a fixed 404 response
		statusCode := int32(404)
		_, err = testFramework.LatticeClient.UpdateRule(ctx,
			&vpclattice.UpdateRuleInput{
				ServiceIdentifier:  svc.Id,
				ListenerIdentifier: listener.Id,
				RuleIdentifier:     targetRule.Id,
				Action: &types.RuleActionMemberFixedResponse{
					Value: types.FixedResponseAction{
						StatusCode: &statusCode,
					},
				},
			})
		Expect(err).ToNot(HaveOccurred())

		// Verify the rule was modified
		modifiedRule, err := testFramework.LatticeClient.GetRule(ctx,
			&vpclattice.GetRuleInput{
				ServiceIdentifier:  svc.Id,
				ListenerIdentifier: listener.Id,
				RuleIdentifier:     targetRule.Id,
			})
		Expect(err).ToNot(HaveOccurred())
		_, isFixed := modifiedRule.Action.(*types.RuleActionMemberFixedResponse)
		Expect(isFixed).To(BeTrue())

		// Wait for drift detection to revert the action back to the original forward
		Eventually(func(g Gomega) {
			revertedRule, err := testFramework.LatticeClient.GetRule(ctx,
				&vpclattice.GetRuleInput{
					ServiceIdentifier:  svc.Id,
					ListenerIdentifier: listener.Id,
					RuleIdentifier:     targetRule.Id,
				})
			g.Expect(err).ToNot(HaveOccurred())
			fwd, ok := revertedRule.Action.(*types.RuleActionMemberForward)
			g.Expect(ok).To(BeTrue())
			g.Expect(fwd.Value.TargetGroups).ToNot(BeEmpty())
			g.Expect(aws.ToString(fwd.Value.TargetGroups[0].TargetGroupIdentifier)).
				To(Equal(originalTargetGroupArn))
		}).WithTimeout(5 * time.Minute).WithPolling(15 * time.Second).Should(Succeed())
	})

	AfterAll(func() {
		if httpRoute != nil {
			testFramework.ExpectDeletedThenNotFound(ctx,
				httpRoute,
				deployment,
				service,
			)
		}
	})
})
