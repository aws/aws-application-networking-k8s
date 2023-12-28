package integration

import (
	"fmt"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/utils"
	"github.com/aws/aws-sdk-go/aws"
	arn2 "github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ram"
	"github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"strings"
	"time"
)

var _ = Describe("RAM Share", Ordered, func() {

	const (
		k8sResourceNamePrefix = "k8s-test-ram-"
	)

	var (
		secondaryAccountId            string
		primaryRamClient              *ram.RAM
		secondaryRamClient            *ram.RAM
		primaryVpcLatticeClient       *vpclattice.VPCLattice
		secondaryVpcLatticeClient     *vpclattice.VPCLattice
		primaryResourceGroupsClient   *resourcegroupstaggingapi.ResourceGroupsTaggingAPI
		secondaryResourceGroupsClient *resourcegroupstaggingapi.ResourceGroupsTaggingAPI
	)

	BeforeAll(func() {
		parts := strings.Split(SecondaryAccountTestRoleArn, ":")
		Expect(len(parts)).To(BeEquivalentTo(6), "Invalid secondary account role arn")
		secondaryAccountId = parts[4]

		primarySess := session.Must(session.NewSession(&aws.Config{Region: aws.String(config.Region)}))
		stsClient := sts.New(primarySess)
		assumeRoleInput := &sts.AssumeRoleInput{
			RoleArn:         aws.String(SecondaryAccountTestRoleArn),
			RoleSessionName: aws.String("aws-application-networking-k8s-ram-e2e-test"),
		}

		assumeRoleResult, err := stsClient.AssumeRoleWithContext(ctx, assumeRoleInput)
		Expect(err).NotTo(HaveOccurred())

		creds := assumeRoleResult.Credentials

		secondarySess := session.Must(session.NewSession(&aws.Config{
			Region: aws.String(config.Region),
			Credentials: credentials.NewStaticCredentials(
				*creds.AccessKeyId,
				*creds.SecretAccessKey,
				*creds.SessionToken,
			),
		}))

		primaryRamClient = ram.New(primarySess)
		secondaryRamClient = ram.New(secondarySess)
		primaryVpcLatticeClient = vpclattice.New(primarySess)
		secondaryVpcLatticeClient = vpclattice.New(secondarySess)
		primaryResourceGroupsClient = resourcegroupstaggingapi.New(primarySess)
		secondaryResourceGroupsClient = resourcegroupstaggingapi.New(secondarySess)
	})

	It("makes service network from secondary account usable for gateway with matching name", func() {
		randomName := k8sResourceNamePrefix + utils.RandomAlphaString(10)
		createSharedServiceNetwork(
			randomName,
			secondaryAccountId,
			primaryVpcLatticeClient,
			secondaryVpcLatticeClient,
			primaryRamClient,
			secondaryRamClient,
		)

		// Create gateway with same name as service network
		gateway := &gwv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      randomName,
				Namespace: k8snamespace,
			},
			Spec: gwv1.GatewaySpec{
				GatewayClassName: "amazon-vpc-lattice",
				Listeners: []gwv1.Listener{
					{
						Name:     gwv1.SectionName(randomName),
						Port:     gwv1.PortNumber(80),
						Protocol: gwv1.HTTPProtocolType,
					},
				},
			},
		}
		err := testFramework.Create(ctx, gateway)
		Expect(err).ToNot(HaveOccurred())

		Eventually(func(g Gomega) {
			// Policy status should be Accepted
			gwNamespacedName := types.NamespacedName{
				Name:      gateway.Name,
				Namespace: gateway.Namespace,
			}
			gw := &gwv1.Gateway{}
			err := testFramework.Client.Get(ctx, gwNamespacedName, gw)
			g.Expect(err).To(BeNil())
			g.Expect(len(gw.Status.Conditions) >= 2).To(BeTrue())
			g.Expect(gw.Status.Conditions[1].Type).To(BeEquivalentTo(string(gwv1.GatewayConditionProgrammed)))
			g.Expect(gw.Status.Conditions[1].Status).To(BeEquivalentTo(metav1.ConditionTrue))
			g.Expect(gw.Status.Conditions[1].Reason).To(BeEquivalentTo(string(gwv1.GatewayReasonProgrammed)))
		}).Should(Succeed())
	})

	It("makes service network from secondary account usable for gateway with matching id", func() {
		randomName := k8sResourceNamePrefix + utils.RandomAlphaString(10)

		serviceNetwork := createSharedServiceNetwork(
			randomName,
			secondaryAccountId,
			primaryVpcLatticeClient,
			secondaryVpcLatticeClient,
			primaryRamClient,
			secondaryRamClient,
		)

		// Create gateway with service network id as name
		gateway := &gwv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      *serviceNetwork.Id,
				Namespace: k8snamespace,
			},
			Spec: gwv1.GatewaySpec{
				GatewayClassName: "amazon-vpc-lattice",
				Listeners: []gwv1.Listener{
					{
						Name:     gwv1.SectionName(randomName),
						Port:     gwv1.PortNumber(80),
						Protocol: gwv1.HTTPProtocolType,
					},
				},
			},
		}
		err := testFramework.Create(ctx, gateway)
		Expect(err).ToNot(HaveOccurred())

		Eventually(func(g Gomega) {
			// Policy status should be Accepted
			gwNamespacedName := types.NamespacedName{
				Name:      gateway.Name,
				Namespace: gateway.Namespace,
			}
			gw := &gwv1.Gateway{}
			err := testFramework.Client.Get(ctx, gwNamespacedName, gw)
			g.Expect(err).To(BeNil())
			g.Expect(len(gw.Status.Conditions) >= 2).To(BeTrue())
			g.Expect(gw.Status.Conditions[1].Type).To(BeEquivalentTo(string(gwv1.GatewayConditionProgrammed)))
			g.Expect(gw.Status.Conditions[1].Status).To(BeEquivalentTo(metav1.ConditionTrue))
			g.Expect(gw.Status.Conditions[1].Reason).To(BeEquivalentTo(string(gwv1.GatewayReasonProgrammed)))
		}).Should(Succeed())
	})

	It("results in reconciliation failure when name conflicts with another resource", func() {
		randomName := k8sResourceNamePrefix + utils.RandomAlphaString(10)

		// Create primary account's service network using randomName
		createSNInput := &vpclattice.CreateServiceNetworkInput{
			Name: aws.String(randomName),
			Tags: k8sTestTags,
		}
		_, err := primaryVpcLatticeClient.CreateServiceNetworkWithContext(ctx, createSNInput)
		Expect(err).NotTo(HaveOccurred())

		createSharedServiceNetwork(
			randomName,
			secondaryAccountId,
			primaryVpcLatticeClient,
			secondaryVpcLatticeClient,
			primaryRamClient,
			secondaryRamClient,
		)

		// Create gateway with same name as service networks
		gateway := &gwv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      randomName,
				Namespace: k8snamespace,
			},
			Spec: gwv1.GatewaySpec{
				GatewayClassName: "amazon-vpc-lattice",
				Listeners: []gwv1.Listener{
					{
						Name:     gwv1.SectionName(randomName),
						Port:     gwv1.PortNumber(80),
						Protocol: gwv1.HTTPProtocolType,
					},
				},
			},
		}
		err = testFramework.Create(ctx, gateway)
		Expect(err).ToNot(HaveOccurred())

		Eventually(func(g Gomega) {
			// Policy status should be Accepted
			gwNamespacedName := types.NamespacedName{
				Name:      gateway.Name,
				Namespace: gateway.Namespace,
			}
			gw := &gwv1.Gateway{}
			err := testFramework.Client.Get(ctx, gwNamespacedName, gw)
			g.Expect(err).To(BeNil())
			g.Expect(len(gw.Status.Conditions) >= 2).To(BeTrue())
			g.Expect(gw.Status.Conditions[1].Type).To(BeEquivalentTo(string(gwv1.GatewayConditionProgrammed)))
			g.Expect(gw.Status.Conditions[1].Status).To(BeEquivalentTo(metav1.ConditionFalse))
			g.Expect(gw.Status.Conditions[1].Reason).To(BeEquivalentTo(gwv1.GatewayReasonInvalid))
		}).Should(Succeed())
	})

	AfterEach(func() {
		// Delete gateways in test namespace except the framework gateway
		gws := &gwv1.GatewayList{}
		err := testFramework.Client.List(ctx, gws, client.InNamespace(k8snamespace))
		Expect(err).To(BeNil())
		for _, gw := range gws.Items {
			if gw.Name != testGateway.Name {
				testFramework.ExpectDeletedThenNotFound(ctx, &gw)
			}
		}
	})

	AfterAll(func() {
		// Find all AWS resources created in tests
		var tagFilters []*resourcegroupstaggingapi.TagFilter
		for key, value := range k8sTestTags {
			tagFilters = append(tagFilters, &resourcegroupstaggingapi.TagFilter{
				Key:    aws.String(key),
				Values: []*string{value},
			})
		}
		getResourcesInput := &resourcegroupstaggingapi.GetResourcesInput{
			TagFilters: tagFilters,
		}

		deleteTaggedResources := func(
			getResourcesResult *resourcegroupstaggingapi.GetResourcesOutput,
			vpcLatticeClient *vpclattice.VPCLattice,
			ramClient *ram.RAM,
		) {
			for _, mapping := range getResourcesResult.ResourceTagMappingList {
				arn, err := arn2.Parse(*mapping.ResourceARN)
				Expect(err).ToNot(HaveOccurred())

				resourceType := strings.Split(arn.Resource, "/")[0]
				switch resourceType {
				case "servicenetwork":
					deleteSnInput := &vpclattice.DeleteServiceNetworkInput{
						ServiceNetworkIdentifier: aws.String(arn.String()),
					}
					_, err := vpcLatticeClient.DeleteServiceNetworkWithContext(ctx, deleteSnInput)
					Expect(err).ToNot(HaveOccurred())
				case "resource-share":
					deleteShareInput := &ram.DeleteResourceShareInput{
						ResourceShareArn: aws.String(arn.String()),
					}
					_, err := ramClient.DeleteResourceShareWithContext(ctx, deleteShareInput)
					Expect(err).ToNot(HaveOccurred())
				default:
					Fail(fmt.Sprintf("Unknown resource type %s", resourceType))
				}
			}
		}

		// Delete secondary account AWS resources
		secondaryGetResourcesResult, err := secondaryResourceGroupsClient.GetResources(getResourcesInput)
		Expect(err).ToNot(HaveOccurred())
		deleteTaggedResources(secondaryGetResourcesResult, secondaryVpcLatticeClient, secondaryRamClient)

		// Delete primary account AWS resources
		primaryGetResourcesResult, err := primaryResourceGroupsClient.GetResources(getResourcesInput)
		Expect(err).ToNot(HaveOccurred())
		deleteTaggedResources(primaryGetResourcesResult, primaryVpcLatticeClient, primaryRamClient)
	})
})

func createSharedServiceNetwork(
	serviceNetworkName string,
	secondaryAccountId string,
	primaryVpcLatticeClient *vpclattice.VPCLattice,
	secondaryVpcLatticeClient *vpclattice.VPCLattice,
	primaryRamClient *ram.RAM,
	secondaryRamClient *ram.RAM,
) *vpclattice.GetServiceNetworkOutput {
	// Create secondary account's service network using randomName
	createSNInput := &vpclattice.CreateServiceNetworkInput{
		Name: aws.String(serviceNetworkName),
		Tags: k8sTestTags,
	}
	createSNResult, err := secondaryVpcLatticeClient.CreateServiceNetworkWithContext(ctx, createSNInput)
	Expect(err).NotTo(HaveOccurred())

	// Share service network to primary account using randomName
	createShareInput := &ram.CreateResourceShareInput{
		Name:                    aws.String(serviceNetworkName),
		ResourceArns:            []*string{createSNResult.Arn},
		Principals:              []*string{aws.String(config.AccountID)},
		AllowExternalPrincipals: aws.Bool(true),
		Tags:                    k8sRamTestTags,
	}
	createShareResult, err := secondaryRamClient.CreateResourceShareWithContext(ctx, createShareInput)
	Expect(err).To(BeNil())

	// Wait for resource share invitation to appear in primary account
	var invitation *ram.ResourceShareInvitation = nil
	Eventually(func(g Gomega) {
		listInvitationsInput := &ram.GetResourceShareInvitationsInput{
			ResourceShareArns: []*string{createShareResult.ResourceShare.ResourceShareArn},
		}
		listInvitationsResult, err := primaryRamClient.GetResourceShareInvitations(listInvitationsInput)
		Expect(err).NotTo(HaveOccurred())

		if len(listInvitationsResult.ResourceShareInvitations) > 0 {
			invitation = listInvitationsResult.ResourceShareInvitations[0]
		}
		g.Expect(invitation).ToNot(BeNil())
	}).Should(Succeed())

	// Accept resource share invitation
	acceptInput := &ram.AcceptResourceShareInvitationInput{
		ResourceShareInvitationArn: invitation.ResourceShareInvitationArn,
	}
	_, err = primaryRamClient.AcceptResourceShareInvitation(acceptInput)
	Expect(err).ToNot(HaveOccurred())

	// Wait for service network to appear in primary account
	var sn *vpclattice.GetServiceNetworkOutput
	Eventually(func(g Gomega) {
		getSNInput := &vpclattice.GetServiceNetworkInput{
			ServiceNetworkIdentifier: createSNResult.Id,
		}
		sn, err = primaryVpcLatticeClient.GetServiceNetworkWithContext(ctx, getSNInput)
		g.Expect(err).ToNot(HaveOccurred())
	}).WithTimeout(5 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())

	return sn
}
