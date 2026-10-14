package integration

import (
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	ec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	vpclattice "github.com/aws/aws-sdk-go-v2/service/vpclattice"
	vpclatticetypes "github.com/aws/aws-sdk-go-v2/service/vpclattice/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	pkg_k8s "github.com/aws/aws-application-networking-k8s/pkg/k8s"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

var _ = Describe("Test vpc association policy", Serial, Ordered, func() {
	var (
		vpcAssociationPolicy *v1alpha1.VpcAssociationPolicy
		sgId                 v1alpha1.SecurityGroupId
		privateDnsSnvaId     string
	)

	policyKey := func() types.NamespacedName {
		return types.NamespacedName{Namespace: vpcAssociationPolicy.Namespace, Name: vpcAssociationPolicy.Name}
	}

	updatePolicy := func(mutate func(spec *v1alpha1.VpcAssociationPolicySpec)) {
		err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			if err := testFramework.Get(ctx, policyKey(), vpcAssociationPolicy); err != nil {
				return err
			}
			mutate(&vpcAssociationPolicy.Spec)
			return testFramework.Client.Update(ctx, vpcAssociationPolicy)
		})
		Expect(err).ToNot(HaveOccurred())
	}

	// expectNoAssociation requires an authoritative empty list; a transient List error must not pass.
	expectNoAssociation := func(g Gomega) {
		list, err := testFramework.LatticeClient.ListServiceNetworkVpcAssociationsAsList(ctx, &vpclattice.ListServiceNetworkVpcAssociationsInput{
			ServiceNetworkIdentifier: testServiceNetwork.Id,
			VpcIdentifier:            aws.String(test.CurrentClusterVpcId),
		})
		g.Expect(err).To(BeNil())
		g.Expect(list).To(BeEmpty())
	}

	programmedCondition := func(g Gomega) *metav1.Condition {
		g.Expect(testFramework.Get(ctx, policyKey(), vpcAssociationPolicy)).To(Succeed())
		cnd := meta.FindStatusCondition(vpcAssociationPolicy.Status.Conditions, string(v1alpha1.VpcAssociationPolicyConditionProgrammed))
		g.Expect(cnd).ToNot(BeNil(), "expected a Programmed condition")
		return cnd
	}

	BeforeAll(func() {
		// Look up existing SG first (idempotent — handles leaked SG from prior failed run)
		describeSgOutput, err := testFramework.Ec2Client.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{
			Filters: []ec2types.Filter{
				{Name: aws.String("group-name"), Values: []string{"k8s-test-lattice-snva-sg"}},
				{Name: aws.String("vpc-id"), Values: []string{test.CurrentClusterVpcId}},
			},
		})
		Expect(err).To(BeNil())

		if len(describeSgOutput.SecurityGroups) > 0 {
			sgId = v1alpha1.SecurityGroupId(*describeSgOutput.SecurityGroups[0].GroupId)
		} else {
			describeVpcOutput, err := testFramework.Ec2Client.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{
				VpcIds: []string{test.CurrentClusterVpcId},
			})
			Expect(err).To(BeNil())
			sourceVPC := describeVpcOutput.Vpcs[0]

			createSgOutput, err := testFramework.Ec2Client.CreateSecurityGroup(ctx, &ec2.CreateSecurityGroupInput{
				Description: aws.String("k8s-test-lattice-snva-sg"),
				GroupName:   aws.String("k8s-test-lattice-snva-sg"),
				VpcId:       aws.String(test.CurrentClusterVpcId),
			})
			Expect(err).To(BeNil())
			sgId = v1alpha1.SecurityGroupId(*createSgOutput.GroupId)

			// Create security group inbound rules only for newly created SG
			_, err = testFramework.Ec2Client.AuthorizeSecurityGroupIngress(ctx, &ec2.AuthorizeSecurityGroupIngressInput{
				GroupId: createSgOutput.GroupId,
				IpPermissions: []ec2types.IpPermission{
					{ // SG Rule to allow HTTP
						IpProtocol: aws.String("tcp"),
						IpRanges: []ec2types.IpRange{
							{
								CidrIp: sourceVPC.CidrBlock,
							},
						},
						FromPort: aws.Int32(80),
						ToPort:   aws.Int32(80),
					},
					{ // SG Rule to allow HTTPS
						IpProtocol: aws.String("tcp"),
						IpRanges: []ec2types.IpRange{
							{
								CidrIp: sourceVPC.CidrBlock,
							},
						},
						FromPort: aws.Int32(443),
						ToPort:   aws.Int32(443),
					},
				},
			})
			Expect(err).To(BeNil())
		}
	})

	It("Create the VpcAssociationPolicy with a SecurityGroupId and additional tags, expecting the ServiceNetworkVpcAssociation with a security group and additional tags", func() {
		vpcAssociationPolicy = &v1alpha1.VpcAssociationPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-vpc-association-policy",
				Namespace: k8snamespace,
				Annotations: map[string]string{
					"application-networking.k8s.aws/tags": "Environment=Dev,Project=MyApp,Team=Platform,CostCenter=12345",
				},
			},
			Spec: v1alpha1.VpcAssociationPolicySpec{
				TargetRef: &gwv1alpha2.NamespacedPolicyTargetReference{
					Group:     gwv1.GroupName,
					Kind:      "Gateway",
					Name:      gwv1.ObjectName(testGateway.Name),
					Namespace: lo.ToPtr(gwv1.Namespace(k8snamespace)),
				},
				SecurityGroupIds: []v1alpha1.SecurityGroupId{sgId},
				AssociateWithVpc: lo.ToPtr(true),
			},
		}
		testFramework.ExpectCreated(ctx, vpcAssociationPolicy)

		Eventually(func(g Gomega) {
			associated, snva, err := testFramework.IsVpcAssociatedWithServiceNetwork(ctx, test.CurrentClusterVpcId, testServiceNetwork)
			g.Expect(err).To(BeNil())
			g.Expect(associated).To(BeTrue())

			output, err := testFramework.LatticeClient.GetServiceNetworkVpcAssociation(ctx, &vpclattice.GetServiceNetworkVpcAssociationInput{
				ServiceNetworkVpcAssociationIdentifier: snva.Id,
			})
			g.Expect(err).To(BeNil())
			g.Expect(output.SecurityGroupIds).To(HaveLen(1))
			g.Expect(output.SecurityGroupIds[0]).To(Equal(string(sgId)))
			// Without privateDnsEnabled in the policy the association keeps the VPC Lattice default.
			g.Expect(aws.ToBool(output.PrivateDnsEnabled)).To(BeFalse())

			snvaArn := aws.ToString(snva.Arn)

			tagsMap, err := testFramework.Cloud.Tagging().GetTagsForArns(ctx, []string{snvaArn})
			g.Expect(err).To(BeNil())

			tags := tagsMap[snvaArn]
			g.Expect(tags).To(HaveKeyWithValue("Environment", "Dev"))
			g.Expect(tags).To(HaveKeyWithValue("Project", "MyApp"))
			g.Expect(tags).To(HaveKeyWithValue("Team", "Platform"))
			g.Expect(tags).To(HaveKeyWithValue("CostCenter", "12345"))
			g.Expect(tags).To(HaveKeyWithValue(pkg_aws.TagManagedBy, fmt.Sprintf("%s/%s/%s", testFramework.Cloud.Config().AccountId, testFramework.Cloud.Config().ClusterName, testFramework.Cloud.Config().VpcId)))

			cnd := programmedCondition(g)
			g.Expect(cnd.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(cnd.Reason).To(Equal(string(v1alpha1.VpcAssociationPolicyReasonProgrammed)))
		}).WithTimeout(5 * time.Minute).Should(Succeed())
	})

	It("Update VpcAssociationPolicy with additional tags changed and attempting to override aws managed tags", func() {
		err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			if err := testFramework.Get(ctx, policyKey(), vpcAssociationPolicy); err != nil {
				return err
			}
			vpcAssociationPolicy.ObjectMeta.Annotations["application-networking.k8s.aws/tags"] = "Environment=Prod,Project=MyApp-v2,Team=DevOps,application-networking.k8s.aws/ManagedBy=test-override"
			return testFramework.Client.Update(ctx, vpcAssociationPolicy)
		})
		Expect(err).ToNot(HaveOccurred())

		Eventually(func(g Gomega) {
			associated, snva, err := testFramework.IsVpcAssociatedWithServiceNetwork(ctx, test.CurrentClusterVpcId, testServiceNetwork)
			g.Expect(err).To(BeNil())
			g.Expect(associated).To(BeTrue())

			snvaArn := aws.ToString(snva.Arn)
			testFramework.Log.Infof(ctx, "ServiceNetworkVpcAssociation ARN: %s", snvaArn)
			tagsMap, err := testFramework.Cloud.Tagging().GetTagsForArns(ctx, []string{snvaArn})
			g.Expect(err).To(BeNil())
			tags := tagsMap[snvaArn]
			g.Expect(tags).To(HaveKeyWithValue("Environment", "Prod"))
			g.Expect(tags).To(HaveKeyWithValue("Project", "MyApp-v2"))
			g.Expect(tags).To(HaveKeyWithValue("Team", "DevOps"))
			g.Expect(tags).ToNot(HaveKey("CostCenter"))

			g.Expect(tags).ToNot(HaveKeyWithValue(pkg_aws.TagManagedBy, "test-override"))
			g.Expect(tags).To(HaveKeyWithValue(pkg_aws.TagManagedBy, fmt.Sprintf("%s/%s/%s", testFramework.Cloud.Config().AccountId, testFramework.Cloud.Config().ClusterName, testFramework.Cloud.Config().VpcId)))
		}).WithTimeout(5 * time.Minute).Should(Succeed())
	})

	It("Update a VpcAssociationPolicy with associateWithVpc to false, expecting deleted ServiceNetworkVpcAssociation", func() {
		updatePolicy(func(spec *v1alpha1.VpcAssociationPolicySpec) {
			spec.AssociateWithVpc = lo.ToPtr(false)
		})

		Eventually(func(g Gomega) {
			//Expect Full SNVA cleanup
			expectNoAssociation(g)

			cnd := programmedCondition(g)
			g.Expect(cnd.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(cnd.Message).To(ContainSubstring("No VPC association is managed"))
		}).WithTimeout(5 * time.Minute).Should(Succeed())
	})

	It("Re-enable the association with privateDnsEnabled and dnsOptions, expecting a new ServiceNetworkVpcAssociation created with private DNS", func() {
		updatePolicy(func(spec *v1alpha1.VpcAssociationPolicySpec) {
			spec.AssociateWithVpc = lo.ToPtr(true)
			spec.PrivateDnsEnabled = lo.ToPtr(true)
			spec.DnsOptions = &v1alpha1.VpcAssociationDnsOptions{
				PrivateDnsPreference: lo.ToPtr(v1alpha1.PrivateDnsPreferenceAllDomains),
			}
		})

		Eventually(func(g Gomega) {
			associated, snva, err := testFramework.IsVpcAssociatedWithServiceNetwork(ctx, test.CurrentClusterVpcId, testServiceNetwork)
			g.Expect(err).To(BeNil())
			g.Expect(associated).To(BeTrue())

			output, err := testFramework.LatticeClient.GetServiceNetworkVpcAssociation(ctx, &vpclattice.GetServiceNetworkVpcAssociationInput{
				ServiceNetworkVpcAssociationIdentifier: snva.Id,
			})
			g.Expect(err).To(BeNil())
			g.Expect(aws.ToBool(output.PrivateDnsEnabled)).To(BeTrue())
			g.Expect(output.DnsOptions).ToNot(BeNil())
			g.Expect(output.DnsOptions.PrivateDnsPreference).To(Equal(vpclatticetypes.PrivateDnsPreferenceAllDomains))
			g.Expect(output.SecurityGroupIds).To(ConsistOf(string(sgId)))
			privateDnsSnvaId = aws.ToString(snva.Id)

			cnd := programmedCondition(g)
			g.Expect(cnd.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(cnd.Reason).To(Equal(string(v1alpha1.VpcAssociationPolicyReasonProgrammed)))
			g.Expect(vpcAssociationPolicy.Annotations).To(HaveKeyWithValue("application-networking.k8s.aws/resourceArn", aws.ToString(snva.Arn)))
		}).WithTimeout(5 * time.Minute).Should(Succeed())
	})

	It("Disable private DNS on the existing association, expecting Programmed=False with reason PrivateDnsMismatch and the association left unchanged", func() {
		updatePolicy(func(spec *v1alpha1.VpcAssociationPolicySpec) {
			spec.PrivateDnsEnabled = lo.ToPtr(false)
			spec.DnsOptions = nil
		})

		Eventually(func(g Gomega) {
			cnd := programmedCondition(g)
			g.Expect(cnd.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(cnd.Reason).To(Equal(string(v1alpha1.VpcAssociationPolicyReasonPrivateDnsMismatch)))
			g.Expect(cnd.Message).To(ContainSubstring("privateDnsEnabled (policy: false, association: true)"))
			g.Expect(cnd.Message).To(ContainSubstring("set spec.associateWithVpc to false"))

			// The association is never recreated automatically.
			associated, snva, err := testFramework.IsVpcAssociatedWithServiceNetwork(ctx, test.CurrentClusterVpcId, testServiceNetwork)
			g.Expect(err).To(BeNil())
			g.Expect(associated).To(BeTrue())
			g.Expect(aws.ToString(snva.Id)).To(Equal(privateDnsSnvaId))
			output, err := testFramework.LatticeClient.GetServiceNetworkVpcAssociation(ctx, &vpclattice.GetServiceNetworkVpcAssociationInput{
				ServiceNetworkVpcAssociationIdentifier: snva.Id,
			})
			g.Expect(err).To(BeNil())
			g.Expect(aws.ToBool(output.PrivateDnsEnabled)).To(BeTrue())

			events := &corev1.EventList{}
			g.Expect(testFramework.List(ctx, events, client.InNamespace(k8snamespace))).To(Succeed())
			found := false
			for _, event := range events.Items {
				if event.InvolvedObject.UID == vpcAssociationPolicy.UID &&
					event.Reason == pkg_k8s.VpcAssociationPolicyEventReasonPrivateDnsMismatch &&
					event.Type == corev1.EventTypeWarning {
					found = true
					break
				}
			}
			g.Expect(found).To(BeTrue(), "expected a PrivateDnsMismatch warning event on the policy")
		}).WithTimeout(3 * time.Minute).Should(Succeed())
	})

	It("Recreate the association through associateWithVpc false then true, expecting the new private DNS settings", func() {
		updatePolicy(func(spec *v1alpha1.VpcAssociationPolicySpec) {
			spec.AssociateWithVpc = lo.ToPtr(false)
		})
		Eventually(func(g Gomega) {
			expectNoAssociation(g)
		}).WithTimeout(5 * time.Minute).Should(Succeed())

		updatePolicy(func(spec *v1alpha1.VpcAssociationPolicySpec) {
			spec.AssociateWithVpc = lo.ToPtr(true)
			spec.PrivateDnsEnabled = lo.ToPtr(true)
			spec.DnsOptions = &v1alpha1.VpcAssociationDnsOptions{
				PrivateDnsPreference: lo.ToPtr(v1alpha1.PrivateDnsPreferenceVerifiedDomainsOnly),
			}
		})
		Eventually(func(g Gomega) {
			associated, snva, err := testFramework.IsVpcAssociatedWithServiceNetwork(ctx, test.CurrentClusterVpcId, testServiceNetwork)
			g.Expect(err).To(BeNil())
			g.Expect(associated).To(BeTrue())
			g.Expect(aws.ToString(snva.Id)).ToNot(Equal(privateDnsSnvaId))

			output, err := testFramework.LatticeClient.GetServiceNetworkVpcAssociation(ctx, &vpclattice.GetServiceNetworkVpcAssociationInput{
				ServiceNetworkVpcAssociationIdentifier: snva.Id,
			})
			g.Expect(err).To(BeNil())
			g.Expect(aws.ToBool(output.PrivateDnsEnabled)).To(BeTrue())
			g.Expect(output.DnsOptions).ToNot(BeNil())
			g.Expect(output.DnsOptions.PrivateDnsPreference).To(Equal(vpclatticetypes.PrivateDnsPreferenceVerifiedDomainsOnly))

			cnd := programmedCondition(g)
			g.Expect(cnd.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(cnd.Reason).To(Equal(string(v1alpha1.VpcAssociationPolicyReasonProgrammed)))
		}).WithTimeout(5 * time.Minute).Should(Succeed())
	})

	DescribeTable("rejects invalid private DNS configurations at admission",
		func(mutate func(spec *v1alpha1.VpcAssociationPolicySpec), expectedError string) {
			invalid := &v1alpha1.VpcAssociationPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-vpc-association-policy",
					Namespace: k8snamespace,
				},
				Spec: v1alpha1.VpcAssociationPolicySpec{
					// A Gateway that does not exist: admission validation still runs, and should a stale CRD
					// ever admit the object, the controller finds nothing to act on.
					TargetRef: &gwv1alpha2.NamespacedPolicyTargetReference{
						Group:     gwv1.GroupName,
						Kind:      "Gateway",
						Name:      "admission-test-nonexistent-gateway",
						Namespace: lo.ToPtr(gwv1.Namespace(k8snamespace)),
					},
					AssociateWithVpc: lo.ToPtr(false),
				},
			}
			mutate(&invalid.Spec)
			err := testFramework.Create(ctx, invalid)
			if err == nil {
				// Never leave a second policy for the gateway behind (a stale CRD without the CEL rules would accept it).
				_ = testFramework.Delete(ctx, invalid)
			}
			Expect(err).To(HaveOccurred(), "the API server must reject the policy")
			Expect(apierrors.IsInvalid(err)).To(BeTrue(), err.Error())
			Expect(err.Error()).To(ContainSubstring(expectedError))
		},
		Entry("dnsOptions without privateDnsEnabled", func(spec *v1alpha1.VpcAssociationPolicySpec) {
			spec.DnsOptions = &v1alpha1.VpcAssociationDnsOptions{PrivateDnsPreference: lo.ToPtr(v1alpha1.PrivateDnsPreferenceAllDomains)}
		}, "dnsOptions can only be set when privateDnsEnabled is true"),
		Entry("dnsOptions with privateDnsEnabled false", func(spec *v1alpha1.VpcAssociationPolicySpec) {
			spec.PrivateDnsEnabled = lo.ToPtr(false)
			spec.DnsOptions = &v1alpha1.VpcAssociationDnsOptions{PrivateDnsPreference: lo.ToPtr(v1alpha1.PrivateDnsPreferenceAllDomains)}
		}, "dnsOptions can only be set when privateDnsEnabled is true"),
		Entry("SPECIFIED_DOMAINS_ONLY without domains", func(spec *v1alpha1.VpcAssociationPolicySpec) {
			spec.PrivateDnsEnabled = lo.ToPtr(true)
			spec.DnsOptions = &v1alpha1.VpcAssociationDnsOptions{PrivateDnsPreference: lo.ToPtr(v1alpha1.PrivateDnsPreferenceSpecifiedDomainsOnly)}
		}, "privateDnsSpecifiedDomains is required when privateDnsPreference is"),
		Entry("domains with ALL_DOMAINS", func(spec *v1alpha1.VpcAssociationPolicySpec) {
			spec.PrivateDnsEnabled = lo.ToPtr(true)
			spec.DnsOptions = &v1alpha1.VpcAssociationDnsOptions{
				PrivateDnsPreference:       lo.ToPtr(v1alpha1.PrivateDnsPreferenceAllDomains),
				PrivateDnsSpecifiedDomains: []v1alpha1.PrivateDnsDomain{"*.example.com"},
			}
		}, "privateDnsSpecifiedDomains can only be set when privateDnsPreference is"),
		Entry("domains without a preference", func(spec *v1alpha1.VpcAssociationPolicySpec) {
			spec.PrivateDnsEnabled = lo.ToPtr(true)
			spec.DnsOptions = &v1alpha1.VpcAssociationDnsOptions{PrivateDnsSpecifiedDomains: []v1alpha1.PrivateDnsDomain{"*.example.com"}}
		}, "privateDnsSpecifiedDomains can only be set when privateDnsPreference is"),
		Entry("unknown preference SPECIFIED_DOMAINS", func(spec *v1alpha1.VpcAssociationPolicySpec) {
			spec.PrivateDnsEnabled = lo.ToPtr(true)
			spec.DnsOptions = &v1alpha1.VpcAssociationDnsOptions{PrivateDnsPreference: lo.ToPtr(v1alpha1.PrivateDnsPreference("SPECIFIED_DOMAINS"))}
		}, "Unsupported value"),
		Entry("more than ten domains", func(spec *v1alpha1.VpcAssociationPolicySpec) {
			spec.PrivateDnsEnabled = lo.ToPtr(true)
			domains := make([]v1alpha1.PrivateDnsDomain, 0, 11)
			for i := 0; i < 11; i++ {
				domains = append(domains, v1alpha1.PrivateDnsDomain(fmt.Sprintf("d%d.example.com", i)))
			}
			spec.DnsOptions = &v1alpha1.VpcAssociationDnsOptions{
				PrivateDnsPreference:       lo.ToPtr(v1alpha1.PrivateDnsPreferenceSpecifiedDomainsOnly),
				PrivateDnsSpecifiedDomains: domains,
			}
		}, "must have at most 10 items"),
		Entry("empty domain", func(spec *v1alpha1.VpcAssociationPolicySpec) {
			spec.PrivateDnsEnabled = lo.ToPtr(true)
			spec.DnsOptions = &v1alpha1.VpcAssociationDnsOptions{
				PrivateDnsPreference:       lo.ToPtr(v1alpha1.PrivateDnsPreferenceSpecifiedDomainsOnly),
				PrivateDnsSpecifiedDomains: []v1alpha1.PrivateDnsDomain{""},
			}
		}, "should be at least 1 chars long"),
	)

	AfterAll(func() {
		// Delete the policy first and wait until it is gone: the controller removes the finalizer only
		// after the association it created has been deleted, which makes the security group deletable.
		if vpcAssociationPolicy != nil {
			testFramework.ExpectDeletedThenNotFound(ctx, vpcAssociationPolicy)
		}

		if sgId != "" {
			_, err := testFramework.Ec2Client.DeleteSecurityGroup(ctx, &ec2.DeleteSecurityGroupInput{
				GroupId: aws.String(string(sgId)),
			})
			Expect(err).To(BeNil())
		}

		// Re-create SNVA manually to recover network state without policy. A ConflictException means
		// an association already exists, which is the desired end state.
		_, err := testFramework.Cloud.Lattice().CreateServiceNetworkVpcAssociation(ctx, &vpclattice.CreateServiceNetworkVpcAssociationInput{
			ServiceNetworkIdentifier: testServiceNetwork.Id,
			VpcIdentifier:            &config.VpcID,
			Tags:                     testFramework.Cloud.DefaultTags(),
		})
		var conflict *vpclatticetypes.ConflictException
		if err != nil && !errors.As(err, &conflict) {
			Expect(err).ToNot(HaveOccurred())
		}

		Eventually(func(g Gomega) {
			associated, _, err := testFramework.IsVpcAssociatedWithServiceNetwork(ctx, test.CurrentClusterVpcId, testServiceNetwork)
			g.Expect(err).To(BeNil())
			g.Expect(associated).To(BeTrue())
		}).WithTimeout(5 * time.Minute).Should(Succeed())
	})
})
