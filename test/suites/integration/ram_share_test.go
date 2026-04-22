package integration

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	arn2 "github.com/aws/aws-sdk-go-v2/aws/arn"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	ram "github.com/aws/aws-sdk-go-v2/service/ram"
	ramtypes "github.com/aws/aws-sdk-go-v2/service/ram/types"
	resourcegroupstaggingapi "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	rgtatypes "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
	sts "github.com/aws/aws-sdk-go-v2/service/sts"
	vpclattice "github.com/aws/aws-sdk-go-v2/service/vpclattice"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/utils"
)

type testClients struct {
	ramClient1     *ram.Client
	ramClient2     *ram.Client
	latticeClient1 *vpclattice.Client
	latticeClient2 *vpclattice.Client
	rgClient1      *resourcegroupstaggingapi.Client
	rgClient2      *resourcegroupstaggingapi.Client
}

var _ = Describe("RAM Share", Ordered, func() {

	const (
		k8sResourceNamePrefix = "k8s-test-ram-"
		testSuite             = "ram-share"
	)

	var (
		clients              *testClients
		secondaryTestRoleArn string
	)

	BeforeAll(func() {
		secondaryTestRoleArn = os.Getenv("SECONDARY_ACCOUNT_TEST_ROLE_ARN")

		primaryCfg, err := awsconfig.LoadDefaultConfig(context.TODO(),
			awsconfig.WithRegion(config.Region),
		)
		Expect(err).NotTo(HaveOccurred())

		stsClient := sts.NewFromConfig(primaryCfg)
		assumeRoleInput := &sts.AssumeRoleInput{
			RoleArn:         aws.String(secondaryTestRoleArn),
			RoleSessionName: aws.String("aws-application-networking-k8s-ram-e2e-test"),
		}

		assumeRoleResult, err := stsClient.AssumeRole(ctx, assumeRoleInput)
		Expect(err).NotTo(HaveOccurred())

		creds := assumeRoleResult.Credentials

		secondaryCfg, err := awsconfig.LoadDefaultConfig(context.TODO(),
			awsconfig.WithRegion(config.Region),
			awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
				*creds.AccessKeyId,
				*creds.SecretAccessKey,
				*creds.SessionToken,
			)),
		)
		Expect(err).NotTo(HaveOccurred())

		clients = &testClients{
			ramClient1:     ram.NewFromConfig(primaryCfg),
			ramClient2:     ram.NewFromConfig(secondaryCfg),
			latticeClient1: vpclattice.NewFromConfig(primaryCfg, func(o *vpclattice.Options) {
				o.RetryMaxAttempts = 10
			}),
			latticeClient2: vpclattice.NewFromConfig(secondaryCfg, func(o *vpclattice.Options) {
				o.RetryMaxAttempts = 10
			}),
			rgClient1:      resourcegroupstaggingapi.NewFromConfig(primaryCfg),
			rgClient2:      resourcegroupstaggingapi.NewFromConfig(secondaryCfg),
		}
	})

	It("makes service network from secondary account usable for gateway with matching name", func() {
		randomName := k8sResourceNamePrefix + utils.RandomAlphaString(10)
		createSharedServiceNetwork(randomName, clients)

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
			programmedCondition := meta.FindStatusCondition(gw.Status.Conditions, string(gwv1.GatewayConditionProgrammed))
			g.Expect(programmedCondition).ToNot(BeNil())
			g.Expect(programmedCondition.Status).To(BeEquivalentTo(metav1.ConditionTrue))
			g.Expect(programmedCondition.Reason).To(BeEquivalentTo(string(gwv1.GatewayReasonProgrammed)))
		}).Should(Succeed())
	})

	It("makes service network from secondary account usable for gateway with matching id", func() {
		randomName := k8sResourceNamePrefix + utils.RandomAlphaString(10)

		serviceNetwork := createSharedServiceNetwork(randomName, clients)

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
			programmedCondition := meta.FindStatusCondition(gw.Status.Conditions, string(gwv1.GatewayConditionProgrammed))
			g.Expect(programmedCondition).ToNot(BeNil())
			g.Expect(programmedCondition.Status).To(BeEquivalentTo(metav1.ConditionTrue))
			g.Expect(programmedCondition.Reason).To(BeEquivalentTo(string(gwv1.GatewayReasonProgrammed)))
		}).Should(Succeed())
	})

	It("results in reconciliation failure when name conflicts with another resource", func() {
		randomName := k8sResourceNamePrefix + utils.RandomAlphaString(10)

		// Create primary account's service network using randomName
		createSNInput := &vpclattice.CreateServiceNetworkInput{
			Name: aws.String(randomName),
			Tags: testFramework.NewTestTags(testSuite),
		}
		_, err := clients.latticeClient1.CreateServiceNetwork(ctx, createSNInput)
		Expect(err).NotTo(HaveOccurred())

		createSharedServiceNetwork(randomName, clients)

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
			programmedCondition := meta.FindStatusCondition(gw.Status.Conditions, string(gwv1.GatewayConditionProgrammed))
			g.Expect(programmedCondition).ToNot(BeNil())
			g.Expect(programmedCondition.Status).To(BeEquivalentTo(metav1.ConditionFalse))
			g.Expect(programmedCondition.Reason).To(BeEquivalentTo(string(gwv1.GatewayReasonInvalid)))
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
		var tagFilters []rgtatypes.TagFilter
		for key, value := range testFramework.NewTestTags(testSuite) {
			tagFilters = append(tagFilters, rgtatypes.TagFilter{
				Key:    aws.String(key),
				Values: []string{value},
			})
		}
		getResourcesInput := &resourcegroupstaggingapi.GetResourcesInput{
			TagFilters: tagFilters,
		}

		deleteTaggedResources := func(
			getResourcesResult *resourcegroupstaggingapi.GetResourcesOutput,
			vpcLatticeClient *vpclattice.Client,
			ramClient *ram.Client,
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
					_, err := vpcLatticeClient.DeleteServiceNetwork(ctx, deleteSnInput)
					Expect(err).ToNot(HaveOccurred())
				case "resource-share":
					deleteShareInput := &ram.DeleteResourceShareInput{
						ResourceShareArn: aws.String(arn.String()),
					}
					_, err := ramClient.DeleteResourceShare(ctx, deleteShareInput)
					Expect(err).ToNot(HaveOccurred())
				default:
					Fail(fmt.Sprintf("Unknown resource type %s", resourceType))
				}
			}
		}

		// Delete secondary account AWS resources
		secondaryGetResourcesResult, err := clients.rgClient2.GetResources(ctx, getResourcesInput)
		Expect(err).ToNot(HaveOccurred())
		deleteTaggedResources(secondaryGetResourcesResult, clients.latticeClient2, clients.ramClient2)

		// Delete primary account AWS resources
		primaryGetResourcesResult, err := clients.rgClient1.GetResources(ctx, getResourcesInput)
		Expect(err).ToNot(HaveOccurred())
		deleteTaggedResources(primaryGetResourcesResult, clients.latticeClient1, clients.ramClient1)
	})
})

func createSharedServiceNetwork(serviceNetworkName string, clients *testClients) *vpclattice.GetServiceNetworkOutput {
	tags := testFramework.NewTestTags("ram-share")

	// Create secondary account's service network
	createSNInput := &vpclattice.CreateServiceNetworkInput{
		Name: aws.String(serviceNetworkName),
		Tags: tags,
	}
	createSNResult, err := clients.latticeClient2.CreateServiceNetwork(ctx, createSNInput)
	Expect(err).NotTo(HaveOccurred())

	var k8sRamTestTags []ramtypes.Tag
	for key, value := range tags {
		k8sRamTestTags = append(k8sRamTestTags, ramtypes.Tag{
			Key:   aws.String(key),
			Value: aws.String(value),
		})
	}

	// Share service network to primary account using randomName
	createShareInput := &ram.CreateResourceShareInput{
		Name:                    aws.String(serviceNetworkName),
		ResourceArns:            []string{*createSNResult.Arn},
		Principals:              []string{config.AccountID},
		AllowExternalPrincipals: aws.Bool(true),
		Tags:                    k8sRamTestTags,
	}
	createShareResult, err := clients.ramClient2.CreateResourceShare(ctx, createShareInput)
	Expect(err).To(BeNil())

	// Wait for resource share invitation to appear in primary account
	var invitation *ramtypes.ResourceShareInvitation = nil
	Eventually(func(g Gomega) {
		listInvitationsInput := &ram.GetResourceShareInvitationsInput{
			ResourceShareArns: []string{*createShareResult.ResourceShare.ResourceShareArn},
		}
		listInvitationsResult, err := clients.ramClient1.GetResourceShareInvitations(ctx, listInvitationsInput)
		Expect(err).NotTo(HaveOccurred())

		if len(listInvitationsResult.ResourceShareInvitations) > 0 {
			invitation = &listInvitationsResult.ResourceShareInvitations[0]
		}
		g.Expect(invitation).ToNot(BeNil())
	}).WithTimeout(5 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())

	// Accept resource share invitation
	acceptInput := &ram.AcceptResourceShareInvitationInput{
		ResourceShareInvitationArn: invitation.ResourceShareInvitationArn,
	}
	_, err = clients.ramClient1.AcceptResourceShareInvitation(ctx, acceptInput)
	Expect(err).ToNot(HaveOccurred())

	// Wait for service network to appear in primary account
	var sn *vpclattice.GetServiceNetworkOutput
	Eventually(func(g Gomega) {
		getSNInput := &vpclattice.GetServiceNetworkInput{
			ServiceNetworkIdentifier: createSNResult.Id,
		}
		sn, err = clients.latticeClient1.GetServiceNetwork(ctx, getSNInput)
		g.Expect(err).ToNot(HaveOccurred())
	}).WithTimeout(5 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())

	return sn
}
