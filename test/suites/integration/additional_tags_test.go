package integration

import (
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = Describe("Additional Tags test", Ordered, func() {
	var (
		httpDeployment1 *appsv1.Deployment
		httpSvc1        *v1.Service
		httpRoute       *gwv1.HTTPRoute
	)

	It("Set up HTTPRoute with additional tags", func() {
		httpDeployment1, httpSvc1 = testFramework.NewHttpApp(test.HTTPAppOptions{Name: "additional-tags-http", Namespace: k8snamespace})
		httpRoute = testFramework.NewHttpRoute(testGateway, httpSvc1, "Service")
		httpRoute.Annotations = map[string]string{
			"application-networking.k8s.aws/tags": "Environment=Dev,Project=MyApp,Team=Platform,CostCenter=12345",
		}

		testFramework.ExpectCreated(ctx,
			httpRoute,
			httpSvc1,
			httpDeployment1,
		)
	})

	It("Verify additional tags on HTTPRoute VPC Lattice resources", func() {
		Eventually(func(g Gomega) {
			route, _ := core.NewRoute(httpRoute)
			vpcLatticeService := testFramework.GetVpcLatticeService(ctx, route)

			serviceTags, err := testFramework.Cloud.Tagging().GetTagsForArns(ctx, []string{*vpcLatticeService.Arn})
			g.Expect(err).To(BeNil())
			serviceTagsMap := serviceTags[*vpcLatticeService.Arn]

			g.Expect(serviceTagsMap).To(HaveKeyWithValue("Environment", aws.String("Dev")))
			g.Expect(serviceTagsMap).To(HaveKeyWithValue("Project", aws.String("MyApp")))
			g.Expect(serviceTagsMap).To(HaveKeyWithValue("Team", aws.String("Platform")))
			g.Expect(serviceTagsMap).To(HaveKeyWithValue("CostCenter", aws.String("12345")))

			tgSummary := testFramework.GetTargetGroup(ctx, httpSvc1)
			tg, err := testFramework.LatticeClient.GetTargetGroup(&vpclattice.GetTargetGroupInput{
				TargetGroupIdentifier: aws.String(*tgSummary.Id),
			})
			g.Expect(err).To(BeNil())

			tgTags, err := testFramework.Cloud.Tagging().GetTagsForArns(ctx, []string{*tg.Arn})
			g.Expect(err).To(BeNil())
			tgTagsMap := tgTags[*tg.Arn]

			g.Expect(tgTagsMap).To(HaveKeyWithValue("Environment", aws.String("Dev")))
			g.Expect(tgTagsMap).To(HaveKeyWithValue("Project", aws.String("MyApp")))
			g.Expect(tgTagsMap).To(HaveKeyWithValue("Team", aws.String("Platform")))
			g.Expect(tgTagsMap).To(HaveKeyWithValue("CostCenter", aws.String("12345")))

			listeners, err := testFramework.LatticeClient.ListListeners(&vpclattice.ListListenersInput{
				ServiceIdentifier: vpcLatticeService.Id,
			})
			g.Expect(err).To(BeNil())
			g.Expect(len(listeners.Items)).To(BeNumerically(">", 0))

			for _, listener := range listeners.Items {
				listenerTags, err := testFramework.Cloud.Tagging().GetTagsForArns(ctx, []string{*listener.Arn})
				g.Expect(err).To(BeNil())
				listenerTagsMap := listenerTags[*listener.Arn]

				g.Expect(listenerTagsMap).To(HaveKeyWithValue("Environment", aws.String("Dev")))
				g.Expect(listenerTagsMap).To(HaveKeyWithValue("Project", aws.String("MyApp")))
				g.Expect(listenerTagsMap).To(HaveKeyWithValue("Team", aws.String("Platform")))
				g.Expect(listenerTagsMap).To(HaveKeyWithValue("CostCenter", aws.String("12345")))
			}

			for _, listener := range listeners.Items {
				rules, err := testFramework.LatticeClient.ListRules(&vpclattice.ListRulesInput{
					ServiceIdentifier:  vpcLatticeService.Id,
					ListenerIdentifier: listener.Id,
				})
				g.Expect(err).To(BeNil())

				for _, rule := range rules.Items {
					if rule.IsDefault != nil && *rule.IsDefault {
						continue
					}

					ruleTags, err := testFramework.Cloud.Tagging().GetTagsForArns(ctx, []string{*rule.Arn})
					g.Expect(err).To(BeNil())
					ruleTagsMap := ruleTags[*rule.Arn]

					g.Expect(ruleTagsMap).To(HaveKeyWithValue("Environment", aws.String("Dev")))
					g.Expect(ruleTagsMap).To(HaveKeyWithValue("Project", aws.String("MyApp")))
					g.Expect(ruleTagsMap).To(HaveKeyWithValue("Team", aws.String("Platform")))
					g.Expect(ruleTagsMap).To(HaveKeyWithValue("CostCenter", aws.String("12345")))
				}
			}

			associations, err := testFramework.LatticeClient.ListServiceNetworkServiceAssociations(&vpclattice.ListServiceNetworkServiceAssociationsInput{
				ServiceIdentifier: vpcLatticeService.Id,
			})
			g.Expect(err).To(BeNil())
			g.Expect(len(associations.Items)).To(BeNumerically(">", 0))

			for _, association := range associations.Items {
				associationTags, err := testFramework.Cloud.Tagging().GetTagsForArns(ctx, []string{*association.Arn})
				g.Expect(err).To(BeNil())
				associationTagsMap := associationTags[*association.Arn]

				g.Expect(associationTagsMap).To(HaveKeyWithValue("Environment", aws.String("Dev")))
				g.Expect(associationTagsMap).To(HaveKeyWithValue("Project", aws.String("MyApp")))
				g.Expect(associationTagsMap).To(HaveKeyWithValue("Team", aws.String("Platform")))
				g.Expect(associationTagsMap).To(HaveKeyWithValue("CostCenter", aws.String("12345")))
			}
		}).Within(1 * time.Minute).Should(Succeed())
	})

	It("Update HTTPRoute additional tags and verify AWS managed tags cannot be overridden", func() {
		err := testFramework.Get(ctx, client.ObjectKeyFromObject(httpRoute), httpRoute)
		Expect(err).To(BeNil())

		httpRoute.Annotations = map[string]string{
			"application-networking.k8s.aws/tags": "Environment=Prod,Project=MyApp,Team=Platform,application-networking.k8s.aws/ManagedBy=test-override",
		}
		testFramework.ExpectUpdated(ctx, httpRoute)

		Eventually(func(g Gomega) {
			route, _ := core.NewRoute(httpRoute)
			vpcLatticeService := testFramework.GetVpcLatticeService(ctx, route)

			serviceTags, err := testFramework.Cloud.Tagging().GetTagsForArns(ctx, []string{*vpcLatticeService.Arn})
			g.Expect(err).To(BeNil())
			serviceTagsMap := serviceTags[*vpcLatticeService.Arn]

			g.Expect(serviceTagsMap).To(HaveKeyWithValue("Environment", aws.String("Prod")))
			g.Expect(serviceTagsMap).To(HaveKeyWithValue("Project", aws.String("MyApp")))
			g.Expect(serviceTagsMap).To(HaveKeyWithValue("Team", aws.String("Platform")))
			g.Expect(serviceTagsMap).ToNot(HaveKey("CostCenter"))

			g.Expect(serviceTagsMap).ToNot(HaveKeyWithValue(pkg_aws.TagManagedBy, aws.String("test-override")))
			g.Expect(serviceTagsMap).To(HaveKeyWithValue(pkg_aws.TagManagedBy, aws.String(fmt.Sprintf("%s/%s/%s", testFramework.Cloud.Config().AccountId, testFramework.Cloud.Config().ClusterName, testFramework.Cloud.Config().VpcId))))

			tgSummary := testFramework.GetTargetGroup(ctx, httpSvc1)
			tg, err := testFramework.LatticeClient.GetTargetGroup(&vpclattice.GetTargetGroupInput{
				TargetGroupIdentifier: aws.String(*tgSummary.Id),
			})
			g.Expect(err).To(BeNil())

			tgTags, err := testFramework.Cloud.Tagging().GetTagsForArns(ctx, []string{*tg.Arn})
			g.Expect(err).To(BeNil())
			tgTagsMap := tgTags[*tg.Arn]

			g.Expect(tgTagsMap).To(HaveKeyWithValue("Environment", aws.String("Prod")))
			g.Expect(tgTagsMap).To(HaveKeyWithValue("Project", aws.String("MyApp")))
			g.Expect(tgTagsMap).To(HaveKeyWithValue("Team", aws.String("Platform")))
			g.Expect(tgTagsMap).ToNot(HaveKey("CostCenter"))

			g.Expect(tgTagsMap).ToNot(HaveKeyWithValue(pkg_aws.TagManagedBy, aws.String("test-override")))
			g.Expect(tgTagsMap).To(HaveKeyWithValue(pkg_aws.TagManagedBy, aws.String(fmt.Sprintf("%s/%s/%s", testFramework.Cloud.Config().AccountId, testFramework.Cloud.Config().ClusterName, testFramework.Cloud.Config().VpcId))))

			listeners, err := testFramework.LatticeClient.ListListeners(&vpclattice.ListListenersInput{
				ServiceIdentifier: vpcLatticeService.Id,
			})
			g.Expect(err).To(BeNil())
			g.Expect(len(listeners.Items)).To(BeNumerically(">", 0))

			for _, listener := range listeners.Items {
				listenerTags, err := testFramework.Cloud.Tagging().GetTagsForArns(ctx, []string{*listener.Arn})
				g.Expect(err).To(BeNil())
				listenerTagsMap := listenerTags[*listener.Arn]

				g.Expect(listenerTagsMap).To(HaveKeyWithValue("Environment", aws.String("Prod")))
				g.Expect(listenerTagsMap).To(HaveKeyWithValue("Project", aws.String("MyApp")))
				g.Expect(listenerTagsMap).To(HaveKeyWithValue("Team", aws.String("Platform")))
				g.Expect(listenerTagsMap).ToNot(HaveKey("CostCenter"))

				g.Expect(listenerTagsMap).ToNot(HaveKeyWithValue(pkg_aws.TagManagedBy, aws.String("test-override")))
				g.Expect(listenerTagsMap).To(HaveKeyWithValue(pkg_aws.TagManagedBy, aws.String(fmt.Sprintf("%s/%s/%s", testFramework.Cloud.Config().AccountId, testFramework.Cloud.Config().ClusterName, testFramework.Cloud.Config().VpcId))))
			}

			for _, listener := range listeners.Items {
				rules, err := testFramework.LatticeClient.ListRules(&vpclattice.ListRulesInput{
					ServiceIdentifier:  vpcLatticeService.Id,
					ListenerIdentifier: listener.Id,
				})
				g.Expect(err).To(BeNil())

				for _, rule := range rules.Items {
					if rule.IsDefault != nil && *rule.IsDefault {
						continue
					}

					ruleTags, err := testFramework.Cloud.Tagging().GetTagsForArns(ctx, []string{*rule.Arn})
					g.Expect(err).To(BeNil())
					ruleTagsMap := ruleTags[*rule.Arn]

					g.Expect(ruleTagsMap).To(HaveKeyWithValue("Environment", aws.String("Prod")))
					g.Expect(ruleTagsMap).To(HaveKeyWithValue("Project", aws.String("MyApp")))
					g.Expect(ruleTagsMap).To(HaveKeyWithValue("Team", aws.String("Platform")))
					g.Expect(ruleTagsMap).ToNot(HaveKey("CostCenter"))

					g.Expect(ruleTagsMap).ToNot(HaveKeyWithValue(pkg_aws.TagManagedBy, aws.String("test-override")))
					g.Expect(ruleTagsMap).To(HaveKeyWithValue(pkg_aws.TagManagedBy, aws.String(fmt.Sprintf("%s/%s/%s", testFramework.Cloud.Config().AccountId, testFramework.Cloud.Config().ClusterName, testFramework.Cloud.Config().VpcId))))
				}
			}

			associations, err := testFramework.LatticeClient.ListServiceNetworkServiceAssociations(&vpclattice.ListServiceNetworkServiceAssociationsInput{
				ServiceIdentifier: vpcLatticeService.Id,
			})
			g.Expect(err).To(BeNil())
			g.Expect(len(associations.Items)).To(BeNumerically(">", 0))

			for _, association := range associations.Items {
				associationTags, err := testFramework.Cloud.Tagging().GetTagsForArns(ctx, []string{*association.Arn})
				g.Expect(err).To(BeNil())
				associationTagsMap := associationTags[*association.Arn]

				g.Expect(associationTagsMap).To(HaveKeyWithValue("Environment", aws.String("Prod")))
				g.Expect(associationTagsMap).To(HaveKeyWithValue("Project", aws.String("MyApp")))
				g.Expect(associationTagsMap).To(HaveKeyWithValue("Team", aws.String("Platform")))
				g.Expect(associationTagsMap).ToNot(HaveKey("CostCenter"))

				g.Expect(associationTagsMap).ToNot(HaveKeyWithValue(pkg_aws.TagManagedBy, aws.String("test-override")))
				g.Expect(associationTagsMap).To(HaveKeyWithValue(pkg_aws.TagManagedBy, aws.String(fmt.Sprintf("%s/%s/%s", testFramework.Cloud.Config().AccountId, testFramework.Cloud.Config().ClusterName, testFramework.Cloud.Config().VpcId))))
			}
		}).Within(1 * time.Minute).Should(Succeed())
	})

	It("Cleanup HTTPRoute resources", func() {
		testFramework.ExpectDeletedThenNotFound(ctx,
			httpRoute,
			httpDeployment1,
			httpSvc1,
		)
	})

	var (
		serviceExportDeployment1 *appsv1.Deployment
		serviceExportSvc1        *v1.Service
		serviceExport1           *anv1alpha1.ServiceExport
	)

	It("Set up ServiceExport with additional tags", func() {
		serviceExportDeployment1, serviceExportSvc1 = testFramework.NewHttpApp(test.HTTPAppOptions{Name: "additional-tags-serviceexport", Namespace: k8snamespace})

		serviceExport1 = &anv1alpha1.ServiceExport{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "application-networking.k8s.aws/v1alpha1",
				Kind:       "ServiceExport",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceExportSvc1.Name,
				Namespace: serviceExportSvc1.Namespace,
				Annotations: map[string]string{
					"application-networking.k8s.aws/federation": "amazon-vpc-lattice",
					"application-networking.k8s.aws/tags":       "Environment=Dev,Project=MyApp,Team=Platform,CostCenter=12345",
				},
			},
			Spec: anv1alpha1.ServiceExportSpec{
				ExportedPorts: []anv1alpha1.ExportedPort{
					{
						Port:      serviceExportSvc1.Spec.Ports[0].Port,
						RouteType: "HTTP",
					},
				},
			},
		}

		testFramework.ExpectCreated(ctx,
			serviceExport1,
			serviceExportSvc1,
			serviceExportDeployment1,
		)
	})

	It("Verify additional tags on ServiceExport VPC Lattice resources", func() {
		Eventually(func(g Gomega) {
			tgSummary := testFramework.GetTargetGroup(ctx, serviceExportSvc1)
			g.Expect(tgSummary).ToNot(BeNil())

			tg, err := testFramework.LatticeClient.GetTargetGroup(&vpclattice.GetTargetGroupInput{
				TargetGroupIdentifier: aws.String(*tgSummary.Id),
			})
			g.Expect(err).To(BeNil())

			tgTags, err := testFramework.Cloud.Tagging().GetTagsForArns(ctx, []string{*tg.Arn})
			g.Expect(err).To(BeNil())
			tgTagsMap := tgTags[*tg.Arn]

			g.Expect(tgTagsMap).To(HaveKeyWithValue("Environment", aws.String("Dev")))
			g.Expect(tgTagsMap).To(HaveKeyWithValue("Project", aws.String("MyApp")))
			g.Expect(tgTagsMap).To(HaveKeyWithValue("Team", aws.String("Platform")))
			g.Expect(tgTagsMap).To(HaveKeyWithValue("CostCenter", aws.String("12345")))

			g.Expect(tgTagsMap).To(HaveKeyWithValue(pkg_aws.TagManagedBy, aws.String(fmt.Sprintf("%s/%s/%s", testFramework.Cloud.Config().AccountId, testFramework.Cloud.Config().ClusterName, testFramework.Cloud.Config().VpcId))))
		}).Within(1 * time.Minute).Should(Succeed())
	})

	It("Update ServiceExport additional tags and verify AWS managed tags cannot be overridden", func() {
		err := testFramework.Get(ctx, client.ObjectKeyFromObject(serviceExport1), serviceExport1)
		Expect(err).To(BeNil())

		serviceExport1.Annotations = map[string]string{
			"application-networking.k8s.aws/federation": "amazon-vpc-lattice",
			"application-networking.k8s.aws/tags":       "Environment=Prod,Project=MyApp,Team=Platform,application-networking.k8s.aws/ManagedBy=test-override",
		}
		testFramework.ExpectUpdated(ctx, serviceExport1)

		Eventually(func(g Gomega) {
			tgSummary := testFramework.GetTargetGroup(ctx, serviceExportSvc1)
			tg, err := testFramework.LatticeClient.GetTargetGroup(&vpclattice.GetTargetGroupInput{
				TargetGroupIdentifier: aws.String(*tgSummary.Id),
			})
			g.Expect(err).To(BeNil())

			tgTags, err := testFramework.Cloud.Tagging().GetTagsForArns(ctx, []string{*tg.Arn})
			g.Expect(err).To(BeNil())
			tgTagsMap := tgTags[*tg.Arn]

			g.Expect(tgTagsMap).To(HaveKeyWithValue("Environment", aws.String("Prod")))
			g.Expect(tgTagsMap).To(HaveKeyWithValue("Project", aws.String("MyApp")))
			g.Expect(tgTagsMap).To(HaveKeyWithValue("Team", aws.String("Platform")))
			g.Expect(tgTagsMap).ToNot(HaveKey("CostCenter"))

			g.Expect(tgTagsMap).ToNot(HaveKeyWithValue(pkg_aws.TagManagedBy, aws.String("test-override")))
			g.Expect(tgTagsMap).To(HaveKeyWithValue(pkg_aws.TagManagedBy, aws.String(fmt.Sprintf("%s/%s/%s", testFramework.Cloud.Config().AccountId, testFramework.Cloud.Config().ClusterName, testFramework.Cloud.Config().VpcId))))
		}).Within(1 * time.Minute).Should(Succeed())
	})

	It("Cleanup ServiceExport resources", func() {
		testFramework.ExpectDeletedThenNotFound(ctx,
			serviceExport1,
			serviceExportDeployment1,
			serviceExportSvc1,
		)
	})
})
