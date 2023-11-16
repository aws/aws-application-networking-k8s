package integration

import (
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

var _ = Describe("Test vpc association policy", Serial, Ordered, func() {
	var (
		vpcAssociationPolicy *v1alpha1.VpcAssociationPolicy
		sgId                 v1alpha1.SecurityGroupId
	)

	BeforeAll(func() {
		// Create security group
		describeVpcOutput, err := testFramework.Ec2Client.DescribeVpcs(&ec2.DescribeVpcsInput{
			VpcIds: []*string{aws.String(test.CurrentClusterVpcId)},
		})
		Expect(err).To(BeNil())
		sourceVPC := describeVpcOutput.Vpcs[0]
		createSgOutput, err := testFramework.Ec2Client.CreateSecurityGroupWithContext(ctx, &ec2.CreateSecurityGroupInput{
			Description: aws.String("k8s-test-lattice-snva-sg"),
			GroupName:   aws.String("k8s-test-lattice-snva-sg"),
			VpcId:       aws.String(test.CurrentClusterVpcId),
		})
		Expect(err).To(BeNil())
		sgId = v1alpha1.SecurityGroupId(*createSgOutput.GroupId)

		// Create security group inbound rules
		_, err = testFramework.Ec2Client.AuthorizeSecurityGroupIngress(&ec2.AuthorizeSecurityGroupIngressInput{
			GroupId: createSgOutput.GroupId,
			IpPermissions: []*ec2.IpPermission{
				{ // SG Rule to allow HTTP
					IpProtocol: aws.String("tcp"),
					IpRanges: []*ec2.IpRange{
						{
							CidrIp: sourceVPC.CidrBlock,
						},
					},
					FromPort: aws.Int64(80),
					ToPort:   aws.Int64(80),
				},
				{ // SG Rule to allow HTTPS
					IpProtocol: aws.String("tcp"),
					IpRanges: []*ec2.IpRange{
						{
							CidrIp: sourceVPC.CidrBlock,
						},
					},
					FromPort: aws.Int64(443),
					ToPort:   aws.Int64(443),
				},
			},
		})
		Expect(err).To(BeNil())
	})

	It("Create the VpcAssociationPolicy with a SecurityGroupId, expecting the ServiceNetworkVpcAssociation with a security group", func() {
		vpcAssociationPolicy = &v1alpha1.VpcAssociationPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-vpc-association-policy",
				Namespace: k8snamespace,
			},
			Spec: v1alpha1.VpcAssociationPolicySpec{
				TargetRef: &gwv1alpha2.PolicyTargetReference{
					Group:     gwv1.GroupName,
					Kind:      "Gateway",
					Name:      gwv1alpha2.ObjectName(testGateway.Name),
					Namespace: lo.ToPtr(gwv1alpha2.Namespace(k8snamespace)),
				},
				SecurityGroupIds: []v1alpha1.SecurityGroupId{sgId},
			},
		}
		testFramework.ExpectCreated(ctx, vpcAssociationPolicy)

		Eventually(func(g Gomega) {
			associated, snva, err := testFramework.IsVpcAssociatedWithServiceNetwork(ctx, test.CurrentClusterVpcId, testServiceNetwork)
			g.Expect(err).To(BeNil())
			g.Expect(associated).To(BeTrue())
			output, err := testFramework.LatticeClient.GetServiceNetworkVpcAssociationWithContext(ctx, &vpclattice.GetServiceNetworkVpcAssociationInput{
				ServiceNetworkVpcAssociationIdentifier: snva.Id,
			})
			g.Expect(err).To(BeNil())
			g.Expect(output.SecurityGroupIds).To(HaveLen(1))
			g.Expect(*output.SecurityGroupIds[0]).To(Equal(string(sgId)))
		}).WithTimeout(5 * time.Minute).Should(Succeed())
	})

	It("Update a VpcAssociationPolicy with associateWithVpc to false, expecting deleted ServiceNetworkVpcAssociation", func() {
		testFramework.Get(ctx, types.NamespacedName{
			Namespace: vpcAssociationPolicy.Namespace,
			Name:      vpcAssociationPolicy.Name,
		}, vpcAssociationPolicy)
		vpcAssociationPolicy.Spec.AssociateWithVpc = lo.ToPtr(false)
		testFramework.ExpectUpdated(ctx, vpcAssociationPolicy)

		Eventually(func(g Gomega) {
			//Expect Full SNVA cleanup
			associated, snva, err := testFramework.IsVpcAssociatedWithServiceNetwork(ctx, test.CurrentClusterVpcId, testServiceNetwork)
			g.Expect(err).To(Not(BeNil()))
			g.Expect(associated).To(BeFalse())
			g.Expect(snva).To(BeNil())
		}).WithTimeout(5 * time.Minute).Should(Succeed())
	})

	AfterAll(func() {
		testFramework.ExpectDeleted(ctx, vpcAssociationPolicy)

		// Clean up the security group
		_, err := testFramework.Ec2Client.DeleteSecurityGroup(&ec2.DeleteSecurityGroupInput{
			GroupId: aws.String(string(sgId)),
		})
		Expect(err).To(BeNil())

		// Re-create SNVA manually to recover network state without policy.
		_, err = testFramework.Cloud.Lattice().CreateServiceNetworkVpcAssociationWithContext(ctx, &vpclattice.CreateServiceNetworkVpcAssociationInput{
			ServiceNetworkIdentifier: testServiceNetwork.Id,
			VpcIdentifier:            &config.VpcID,
			Tags:                     testFramework.Cloud.DefaultTags(),
		})
		Expect(err).To(BeNil())

		Eventually(func(g Gomega) {
			associated, _, err := testFramework.IsVpcAssociatedWithServiceNetwork(ctx, test.CurrentClusterVpcId, testServiceNetwork)
			g.Expect(err).To(BeNil())
			g.Expect(associated).To(BeTrue())
		}).WithTimeout(5 * time.Minute).Should(Succeed())
	})
})
