package integration

import (
	"fmt"
	awsClient "github.com/aws/aws-sdk-go/aws/client"
	"os"
	"strings"
	"time"

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
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/utils"
)

type testClients struct {
	ramClient1     *ram.RAM
	ramClient2     *ram.RAM
	latticeClient1 *vpclattice.VPCLattice
	latticeClient2 *vpclattice.VPCLattice
	rgClient1      *resourcegroupstaggingapi.ResourceGroupsTaggingAPI
	rgClient2      *resourcegroupstaggingapi.ResourceGroupsTaggingAPI
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

		retryer := awsClient.DefaultRetryer{
			MinThrottleDelay: 500 * time.Millisecond,
			MinRetryDelay:    500 * time.Millisecond,
			MaxThrottleDelay: 5 * time.Second,
			MaxRetryDelay:    5 * time.Second,
			NumMaxRetries:    5,
		}

		primarySess := session.Must(session.NewSession(&aws.Config{
			Region:  aws.String(config.Region),
			Retryer: retryer,
		}))

		stsClient := sts.New(primarySess)
		assumeRoleInput := &sts.AssumeRoleInput{
			RoleArn:         aws.String(secondaryTestRoleArn),
			RoleSessionName: aws.String("aws-application-networking-k8s-ram-e2e-test"),
		}

		assumeRoleResult, err := stsClient.AssumeRoleWithContext(ctx, assumeRoleInput)
		Expect(err).NotTo(HaveOccurred())

		creds := assumeRoleResult.Credentials

		secondarySess := session.Must(session.NewSession(&aws.Config{
			Region:  aws.String(config.Region),
			Retryer: retryer,
			Credentials: credentials.NewStaticCredentials(
				*creds.AccessKeyId,
				*creds.SecretAccessKey,
				*creds.SessionToken,
			),
		}))

		clients = &testClients{
			ramClient1:     ram.New(primarySess),
			ramClient2:     ram.New(secondarySess),
			latticeClient1: vpclattice.New(primarySess),
			latticeClient2: vpclattice.New(secondarySess),
			rgClient1:      resourcegroupstaggingapi.New(primarySess),
			rgClient2:      resourcegroupstaggingapi.New(secondarySess),
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
		_, err := clients.latticeClient1.CreateServiceNetworkWithContext(ctx, createSNInput)
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
		var tagFilters []*resourcegroupstaggingapi.TagFilter
		for key, value := range testFramework.NewTestTags(testSuite) {
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
		secondaryGetResourcesResult, err := clients.rgClient2.GetResources(getResourcesInput)
		Expect(err).ToNot(HaveOccurred())
		deleteTaggedResources(secondaryGetResourcesResult, clients.latticeClient2, clients.ramClient2)

		// Delete primary account AWS resources
		primaryGetResourcesResult, err := clients.rgClient1.GetResources(getResourcesInput)
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
	createSNResult, err := clients.latticeClient2.CreateServiceNetworkWithContext(ctx, createSNInput)
	Expect(err).NotTo(HaveOccurred())

	var k8sRamTestTags []*ram.Tag
	for key, value := range tags {
		k8sRamTestTags = append(k8sRamTestTags, &ram.Tag{
			Key:   aws.String(key),
			Value: value,
		})
	}

	// Share service network to primary account using randomName
	createShareInput := &ram.CreateResourceShareInput{
		Name:                    aws.String(serviceNetworkName),
		ResourceArns:            []*string{createSNResult.Arn},
		Principals:              []*string{aws.String(config.AccountID)},
		AllowExternalPrincipals: aws.Bool(true),
		Tags:                    k8sRamTestTags,
	}
	createShareResult, err := clients.ramClient2.CreateResourceShareWithContext(ctx, createShareInput)
	Expect(err).To(BeNil())

	// Wait for resource share invitation to appear in primary account
	var invitation *ram.ResourceShareInvitation = nil
	Eventually(func(g Gomega) {
		listInvitationsInput := &ram.GetResourceShareInvitationsInput{
			ResourceShareArns: []*string{createShareResult.ResourceShare.ResourceShareArn},
		}
		listInvitationsResult, err := clients.ramClient1.GetResourceShareInvitations(listInvitationsInput)
		Expect(err).NotTo(HaveOccurred())

		if len(listInvitationsResult.ResourceShareInvitations) > 0 {
			invitation = listInvitationsResult.ResourceShareInvitations[0]
		}
		g.Expect(invitation).ToNot(BeNil())
	}).WithTimeout(5 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())

	// Accept resource share invitation
	acceptInput := &ram.AcceptResourceShareInvitationInput{
		ResourceShareInvitationArn: invitation.ResourceShareInvitationArn,
	}
	_, err = clients.ramClient1.AcceptResourceShareInvitation(acceptInput)
	Expect(err).ToNot(HaveOccurred())

	// Wait for service network to appear in primary account
	var sn *vpclattice.GetServiceNetworkOutput
	Eventually(func(g Gomega) {
		getSNInput := &vpclattice.GetServiceNetworkInput{
			ServiceNetworkIdentifier: createSNResult.Id,
		}
		sn, err = clients.latticeClient1.GetServiceNetworkWithContext(ctx, getSNInput)
		g.Expect(err).ToNot(HaveOccurred())
	}).WithTimeout(5 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())

	return sn
}
