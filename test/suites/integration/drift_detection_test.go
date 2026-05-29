package integration

import (
	"errors"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/vpclattice"
	"github.com/aws/aws-sdk-go-v2/service/vpclattice/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/controllers"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
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

		// Delete the service out-of-band: delete associations then the service.
		// The controller's resync may recreate associations, so we keep deleting
		// until the service itself is successfully deleted.
		Eventually(func(g Gomega) {
			// Delete any associations first (required before service deletion)
			resp, err := testFramework.LatticeClient.ListServiceNetworkServiceAssociationsAsList(ctx,
				&vpclattice.ListServiceNetworkServiceAssociationsInput{
					ServiceIdentifier: svc.Id,
				})
			g.Expect(err).ToNot(HaveOccurred())
			for _, a := range resp {
				testFramework.LatticeClient.DeleteServiceNetworkServiceAssociation(ctx,
					&vpclattice.DeleteServiceNetworkServiceAssociationInput{
						ServiceNetworkServiceAssociationIdentifier: a.Id,
					})
			}
			// Attempt to delete the service; fails if associations still exist
			_, err = testFramework.LatticeClient.DeleteService(ctx, &vpclattice.DeleteServiceInput{
				ServiceIdentifier: svc.Id,
			})
			g.Expect(err).ToNot(HaveOccurred())
		}).WithTimeout(4 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())

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

var _ = Describe("IAMAuthPolicy Drift detection", Ordered, func() {
	const (
		driftPolicyName = "drift-iam-auth-policy"
		driftAppName    = "drift-iam-auth"
		// Same All-Allow document used in iamauthpolicy_test.go.
		allowAllInvoke = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":"*","Action":"vpc-lattice-svcs:Invoke","Resource":"*"}]}`
	)

	var (
		deployment   *appsv1.Deployment
		k8sService   *v1.Service
		httpRoute    *gwv1.HTTPRoute
		policy       *anv1alpha1.IAMAuthPolicy
		latticeSvcId string
		resourceId   string
	)

	BeforeAll(func() {
		if config.ReconcileDefaultResyncInterval <= 0 {
			Skip("RECONCILE_DEFAULT_RESYNC_SECONDS not set or 0, skipping drift detection tests")
		}

		deployment, k8sService = testFramework.NewHttpApp(test.HTTPAppOptions{
			Name:      driftAppName,
			Namespace: k8snamespace,
		})
		httpRoute = testFramework.NewHttpRoute(testGateway, k8sService, "Service")
		testFramework.ExpectCreated(ctx, deployment, k8sService, httpRoute)

		// Wait for the Lattice service backing the route to be active.
		latticeSvc := testFramework.GetVpcLatticeService(ctx, core.NewHTTPRoute(*httpRoute))
		latticeSvcId = aws.ToString(latticeSvc.Id)

		// Create the IAMAuthPolicy targeting the route. The controller will
		// flip the service's auth type to AWS_IAM and put the policy doc.
		policy = &anv1alpha1.IAMAuthPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      driftPolicyName,
				Namespace: k8snamespace,
			},
			Spec: anv1alpha1.IAMAuthPolicySpec{
				Policy: allowAllInvoke,
				TargetRef: &gwv1alpha2.NamespacedPolicyTargetReference{
					Group: gwv1.GroupName,
					Kind:  "HTTPRoute",
					Name:  gwv1.ObjectName(httpRoute.Name),
				},
			},
		}
		testFramework.ExpectCreated(ctx, policy)

		// Wait for the policy to be Accepted and the resource-id annotation to
		// be populated. After this point, the lattice auth policy doc is set
		// and the service auth type is AWS_IAM.
		Eventually(func(g Gomega) {
			p := &anv1alpha1.IAMAuthPolicy{}
			g.Expect(testFramework.Client.Get(ctx, client.ObjectKeyFromObject(policy), p)).To(Succeed())
			g.Expect(GetPolicyStatusReason(p)).To(Equal(gwv1.PolicyReasonAccepted))
			g.Expect(p.Annotations[controllers.IAMAuthPolicyAnnotationType]).To(Equal(model.ServiceType))
			g.Expect(p.Annotations[controllers.IAMAuthPolicyAnnotationResId]).ToNot(BeEmpty())
			resourceId = p.Annotations[controllers.IAMAuthPolicyAnnotationResId]
		}).WithTimeout(2 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())

		// Sanity check: the annotation resource id is the lattice service id.
		Expect(resourceId).To(Equal(latticeSvcId))
	})

	It("recreates an auth policy deleted out-of-band", func() {
		Expect(resourceId).ToNot(BeEmpty())

		// Sanity check: auth policy doc is set on the lattice service.
		existing, err := testFramework.LatticeClient.GetAuthPolicy(ctx,
			&vpclattice.GetAuthPolicyInput{ResourceIdentifier: &resourceId})
		Expect(err).ToNot(HaveOccurred())
		Expect(aws.ToString(existing.Policy)).To(Equal(allowAllInvoke))

		// Delete the auth policy out-of-band.
		_, err = testFramework.LatticeClient.DeleteAuthPolicy(ctx,
			&vpclattice.DeleteAuthPolicyInput{ResourceIdentifier: &resourceId})
		Expect(err).ToNot(HaveOccurred())

		// Verify deletion took effect. The Lattice GetAuthPolicy API may
		// return either a not-found error or an output with an empty Policy
		// when no policy is attached, so accept both.
		deleted, _ := testFramework.LatticeClient.GetAuthPolicy(ctx,
			&vpclattice.GetAuthPolicyInput{ResourceIdentifier: &resourceId})
		if deleted != nil {
			Expect(aws.ToString(deleted.Policy)).To(BeEmpty())
		}

		// Wait for drift detection to restore the auth policy.
		timeout := 2*config.ReconcileDefaultResyncInterval + 60*time.Second
		Eventually(func(g Gomega) {
			out, err := testFramework.LatticeClient.GetAuthPolicy(ctx,
				&vpclattice.GetAuthPolicyInput{ResourceIdentifier: &resourceId})
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(aws.ToString(out.Policy)).To(Equal(allowAllInvoke))
		}).WithTimeout(timeout).WithPolling(15 * time.Second).Should(Succeed())
	})

	It("re-enables AWS_IAM auth type when flipped to NONE", func() {
		Expect(latticeSvcId).ToNot(BeEmpty())

		// Sanity check: service auth type is AWS_IAM.
		svc, err := testFramework.LatticeClient.GetService(ctx,
			&vpclattice.GetServiceInput{ServiceIdentifier: &latticeSvcId})
		Expect(err).ToNot(HaveOccurred())
		Expect(svc.AuthType).To(Equal(types.AuthTypeAwsIam))

		// Flip the service auth type to NONE out-of-band.
		_, err = testFramework.LatticeClient.UpdateService(ctx, &vpclattice.UpdateServiceInput{
			ServiceIdentifier: &latticeSvcId,
			AuthType:          types.AuthTypeNone,
		})
		Expect(err).ToNot(HaveOccurred())

		// Verify the change took effect.
		flipped, err := testFramework.LatticeClient.GetService(ctx,
			&vpclattice.GetServiceInput{ServiceIdentifier: &latticeSvcId})
		Expect(err).ToNot(HaveOccurred())
		Expect(flipped.AuthType).To(Equal(types.AuthTypeNone))

		// Wait for drift detection to restore AWS_IAM.
		timeout := 2*config.ReconcileDefaultResyncInterval + 60*time.Second
		Eventually(func(g Gomega) {
			s, err := testFramework.LatticeClient.GetService(ctx,
				&vpclattice.GetServiceInput{ServiceIdentifier: &latticeSvcId})
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(s.AuthType).To(Equal(types.AuthTypeAwsIam))
		}).WithTimeout(timeout).WithPolling(15 * time.Second).Should(Succeed())
	})

	AfterAll(func() {
		// Order matters: delete the policy first so the controller can reset
		// the lattice service's auth type before the route deletion tears the
		// service down.
		if policy != nil {
			testFramework.ExpectDeletedThenNotFound(ctx, policy)
		}
		if httpRoute != nil {
			testFramework.ExpectDeletedThenNotFound(ctx, httpRoute, deployment, k8sService)
		}
	})
})

var _ = Describe("VpcAssociationPolicy Drift detection", Serial, Ordered, func() {
	const (
		driftVapName      = "drift-vpc-association-policy"
		driftVapSGName    = "k8s-test-drift-vap-sg"
		driftVapSGAltName = "k8s-test-drift-vap-sg-alt"
	)

	var (
		vap     *anv1alpha1.VpcAssociationPolicy
		sgId    anv1alpha1.SecurityGroupId
		altSgId anv1alpha1.SecurityGroupId
	)

	BeforeAll(func() {
		if config.ReconcileDefaultResyncInterval <= 0 {
			Skip("RECONCILE_DEFAULT_RESYNC_SECONDS not set or 0, skipping drift detection tests")
		}

		// Create two test security groups in the cluster VPC: one used by the
		// VAP and one used to exercise SG drift out-of-band.
		createSg, err := testFramework.Ec2Client.CreateSecurityGroup(ctx, &ec2.CreateSecurityGroupInput{
			Description: aws.String(driftVapSGName),
			GroupName:   aws.String(driftVapSGName),
			VpcId:       aws.String(test.CurrentClusterVpcId),
		})
		Expect(err).To(BeNil())
		sgId = anv1alpha1.SecurityGroupId(aws.ToString(createSg.GroupId))

		createAltSg, err := testFramework.Ec2Client.CreateSecurityGroup(ctx, &ec2.CreateSecurityGroupInput{
			Description: aws.String(driftVapSGAltName),
			GroupName:   aws.String(driftVapSGAltName),
			VpcId:       aws.String(test.CurrentClusterVpcId),
		})
		Expect(err).To(BeNil())
		altSgId = anv1alpha1.SecurityGroupId(aws.ToString(createAltSg.GroupId))

		// Create the VpcAssociationPolicy targeting the test gateway. The
		// controller drives the SNVA to be active with our SG.
		vap = &anv1alpha1.VpcAssociationPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      driftVapName,
				Namespace: k8snamespace,
			},
			Spec: anv1alpha1.VpcAssociationPolicySpec{
				TargetRef: &gwv1alpha2.NamespacedPolicyTargetReference{
					Group:     gwv1.GroupName,
					Kind:      "Gateway",
					Name:      gwv1.ObjectName(testGateway.Name),
					Namespace: lo.ToPtr(gwv1.Namespace(k8snamespace)),
				},
				SecurityGroupIds: []anv1alpha1.SecurityGroupId{sgId},
				AssociateWithVpc: lo.ToPtr(true),
			},
		}
		testFramework.ExpectCreated(ctx, vap)

		// Wait for the SNVA to be active and to reflect the configured SG.
		Eventually(func(g Gomega) {
			associated, snva, err := testFramework.IsVpcAssociatedWithServiceNetwork(ctx, test.CurrentClusterVpcId, testServiceNetwork)
			g.Expect(err).To(BeNil())
			g.Expect(associated).To(BeTrue())
			out, err := testFramework.LatticeClient.GetServiceNetworkVpcAssociation(ctx, &vpclattice.GetServiceNetworkVpcAssociationInput{
				ServiceNetworkVpcAssociationIdentifier: snva.Id,
			})
			g.Expect(err).To(BeNil())
			g.Expect(out.SecurityGroupIds).To(HaveLen(1))
			g.Expect(out.SecurityGroupIds[0]).To(Equal(string(sgId)))
		}).WithTimeout(5 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())
	})

	It("recreates the SNVA deleted out-of-band", func() {
		// Capture the currently-active SNVA id.
		associated, snva, err := testFramework.IsVpcAssociatedWithServiceNetwork(ctx, test.CurrentClusterVpcId, testServiceNetwork)
		Expect(err).To(BeNil())
		Expect(associated).To(BeTrue())
		originalId := aws.ToString(snva.Id)
		Expect(originalId).ToNot(BeEmpty())

		// Delete the SNVA out-of-band.
		_, err = testFramework.LatticeClient.DeleteServiceNetworkVpcAssociation(ctx, &vpclattice.DeleteServiceNetworkVpcAssociationInput{
			ServiceNetworkVpcAssociationIdentifier: snva.Id,
		})
		Expect(err).To(BeNil())

		// Wait for the original SNVA to be fully gone. Lattice does not
		// expose a "DELETED" status: deletion completes when the SNVA no
		// longer appears in the list of associations for this VPC.
		Eventually(func(g Gomega) {
			list, err := testFramework.LatticeClient.ListServiceNetworkVpcAssociationsAsList(ctx, &vpclattice.ListServiceNetworkVpcAssociationsInput{
				ServiceNetworkIdentifier: testServiceNetwork.Id,
				VpcIdentifier:            aws.String(test.CurrentClusterVpcId),
			})
			g.Expect(err).To(BeNil())
			for _, a := range list {
				g.Expect(aws.ToString(a.Id)).ToNot(Equal(originalId))
			}
		}).WithTimeout(2 * time.Minute).WithPolling(10 * time.Second).Should(Succeed())

		// Wait for drift detection to recreate an active SNVA with a new id.
		timeout := 2*config.ReconcileDefaultResyncInterval + 60*time.Second
		Eventually(func(g Gomega) {
			associated, snva, err := testFramework.IsVpcAssociatedWithServiceNetwork(ctx, test.CurrentClusterVpcId, testServiceNetwork)
			g.Expect(err).To(BeNil())
			g.Expect(associated).To(BeTrue())
			g.Expect(aws.ToString(snva.Id)).ToNot(Equal(originalId))
		}).WithTimeout(timeout).WithPolling(15 * time.Second).Should(Succeed())
	})

	It("restores security groups drifted out-of-band", func() {
		// Make sure the SNVA currently reflects the policy's SG.
		var snvaId *string
		Eventually(func(g Gomega) {
			associated, snva, err := testFramework.IsVpcAssociatedWithServiceNetwork(ctx, test.CurrentClusterVpcId, testServiceNetwork)
			g.Expect(err).To(BeNil())
			g.Expect(associated).To(BeTrue())
			out, err := testFramework.LatticeClient.GetServiceNetworkVpcAssociation(ctx, &vpclattice.GetServiceNetworkVpcAssociationInput{
				ServiceNetworkVpcAssociationIdentifier: snva.Id,
			})
			g.Expect(err).To(BeNil())
			g.Expect(out.SecurityGroupIds).To(ConsistOf(string(sgId)))
			snvaId = snva.Id
		}).WithTimeout(2 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())
		Expect(snvaId).ToNot(BeNil())

		// Drift the SG list out-of-band.
		_, err := testFramework.LatticeClient.UpdateServiceNetworkVpcAssociation(ctx, &vpclattice.UpdateServiceNetworkVpcAssociationInput{
			ServiceNetworkVpcAssociationIdentifier: snvaId,
			SecurityGroupIds:                       []string{string(altSgId)},
		})
		Expect(err).To(BeNil())

		// Verify the drift took effect (the SNVA now reflects the alt SG).
		Eventually(func(g Gomega) {
			out, err := testFramework.LatticeClient.GetServiceNetworkVpcAssociation(ctx, &vpclattice.GetServiceNetworkVpcAssociationInput{
				ServiceNetworkVpcAssociationIdentifier: snvaId,
			})
			g.Expect(err).To(BeNil())
			g.Expect(out.SecurityGroupIds).To(ConsistOf(string(altSgId)))
		}).WithTimeout(2 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())

		// Wait for drift detection to restore the policy's SG list.
		timeout := 2*config.ReconcileDefaultResyncInterval + 60*time.Second
		Eventually(func(g Gomega) {
			out, err := testFramework.LatticeClient.GetServiceNetworkVpcAssociation(ctx, &vpclattice.GetServiceNetworkVpcAssociationInput{
				ServiceNetworkVpcAssociationIdentifier: snvaId,
			})
			g.Expect(err).To(BeNil())
			g.Expect(out.SecurityGroupIds).To(ConsistOf(string(sgId)))
		}).WithTimeout(timeout).WithPolling(15 * time.Second).Should(Succeed())
	})

	AfterAll(func() {
		// Delete the VAP first. The controller will tear down the SNVA and
		// only remove the finalizer once the SNVA is fully gone, so the test
		// SGs become safe to delete after the VAP is NotFound.
		if vap != nil {
			testFramework.ExpectDeletedThenNotFound(ctx, vap)
		}
		if sgId != "" {
			_, err := testFramework.Ec2Client.DeleteSecurityGroup(ctx, &ec2.DeleteSecurityGroupInput{
				GroupId: aws.String(string(sgId)),
			})
			Expect(err).To(BeNil())
		}
		if altSgId != "" {
			_, err := testFramework.Ec2Client.DeleteSecurityGroup(ctx, &ec2.DeleteSecurityGroupInput{
				GroupId: aws.String(string(altSgId)),
			})
			Expect(err).To(BeNil())
		}

		// Restore the cluster's baseline network state: test-gateway must end
		// up associated with the cluster VPC (other suites rely on this).
		// With drift detection enabled, the controller may have already
		// recreated the association by the time this runs, and VPC Lattice
		// permits only one association per VPC/service-network pair. So make
		// this idempotent: only create the association if one does not already
		// exist, and tolerate a ConflictException from a check-then-create
		// race.
		if testServiceNetwork != nil {
			existing, err := testFramework.LatticeClient.ListServiceNetworkVpcAssociationsAsList(ctx, &vpclattice.ListServiceNetworkVpcAssociationsInput{
				ServiceNetworkIdentifier: testServiceNetwork.Id,
				VpcIdentifier:            &config.VpcID,
			})
			Expect(err).To(BeNil())

			if len(existing) == 0 {
				_, err := testFramework.Cloud.Lattice().CreateServiceNetworkVpcAssociation(ctx, &vpclattice.CreateServiceNetworkVpcAssociationInput{
					ServiceNetworkIdentifier: testServiceNetwork.Id,
					VpcIdentifier:            &config.VpcID,
					Tags:                     testFramework.Cloud.DefaultTags(),
				})
				// A concurrent reconcile may have created the association
				// between our list check and this create; that is the desired
				// end state, so tolerate the conflict.
				var ce *types.ConflictException
				if err != nil && !errors.As(err, &ce) {
					Expect(err).To(BeNil())
				}
			}

			Eventually(func(g Gomega) {
				associated, _, err := testFramework.IsVpcAssociatedWithServiceNetwork(ctx, test.CurrentClusterVpcId, testServiceNetwork)
				g.Expect(err).To(BeNil())
				g.Expect(associated).To(BeTrue())
			}).WithTimeout(5 * time.Minute).Should(Succeed())
		}
	})
})
