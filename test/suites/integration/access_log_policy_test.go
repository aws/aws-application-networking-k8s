package integration

import (
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go/service/firehose"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/test/pkg/test"
)

var _ = Describe("Access Log Policy", Ordered, func() {
	const (
		k8sResourceName          = "test-access-log-policy"
		k8sResource2Name         = "test-access-log-policy-secondary"
		bucketName               = "k8s-test-lattice-bucket"
		logGroupName             = "k8s-test-lattice-log-group"
		logGroup2Name            = "k8s-test-lattice-log-group-secondary"
		deliveryStreamName       = "k8s-test-lattice-delivery-stream"
		deliveryStreamRoleName   = "k8s-test-lattice-delivery-stream-role"
		deliveryStreamRolePolicy = `{
			"Version": "2012-10-17",
			"Statement": [
				{
					"Effect": "Allow",
					"Action": ["s3:PutObject", "s3:GetBucketLocation"],
					"Resource": ["arn:aws:s3:::k8s-test-lattice-bucket/*"]
				}
			]
		}`
		deliveryStreamAssumeRolePolicy = `{
			"Version": "2012-10-17",
			"Statement": [
				{
					"Effect": "Allow",
					"Principal": {
						"Service": "firehose.amazonaws.com"
					},
					"Action": "sts:AssumeRole"
				}
			]
		}`
	)

	var (
		s3Client          *s3.S3
		logsClient        *cloudwatchlogs.CloudWatchLogs
		firehoseClient    *firehose.Firehose
		iamClient         *iam.IAM
		httpDeployment    *appsv1.Deployment
		grpcDeployment    *appsv1.Deployment
		httpK8sService    *corev1.Service
		grpcK8sService    *corev1.Service
		httpRoute         *gwv1beta1.HTTPRoute
		grpcRoute         *gwv1alpha2.GRPCRoute
		bucketArn         string
		logGroupArn       string
		logGroup2Arn      string
		deliveryStreamArn string
		roleArn           string
	)

	BeforeAll(func() {
		// Create S3 Bucket
		s3Client = s3.New(session.Must(session.NewSession(&aws.Config{Region: aws.String(config.Region)})))
		_, err := s3Client.CreateBucketWithContext(ctx, &s3.CreateBucketInput{
			Bucket: aws.String(bucketName),
		})
		Expect(err).To(BeNil())
		bucketArn = "arn:aws:s3:::" + bucketName

		// Create CloudWatch Log Group
		logsClient = cloudwatchlogs.New(session.Must(session.NewSession(&aws.Config{Region: aws.String(config.Region)})))
		_, err = logsClient.CreateLogGroupWithContext(ctx, &cloudwatchlogs.CreateLogGroupInput{
			LogGroupName: aws.String(logGroupName),
		})
		Expect(err).To(BeNil())
		logGroupArn = fmt.Sprintf("arn:aws:logs:%s:%s:log-group:%s:*", config.Region, config.AccountID, logGroupName)

		// Create secondary CloudWatch Log Group
		_, err = logsClient.CreateLogGroupWithContext(ctx, &cloudwatchlogs.CreateLogGroupInput{
			LogGroupName: aws.String(logGroup2Name),
		})
		Expect(err).To(BeNil())
		logGroup2Arn = fmt.Sprintf("arn:aws:logs:%s:%s:log-group:%s:*", config.Region, config.AccountID, logGroup2Name)

		// Create IAM Role for Firehose Delivery Stream
		iamClient = iam.New(session.Must(session.NewSession(&aws.Config{Region: aws.String(config.Region)})))
		createRoleOutput, err := iamClient.CreateRoleWithContext(ctx, &iam.CreateRoleInput{
			RoleName:                 aws.String(deliveryStreamRoleName),
			AssumeRolePolicyDocument: aws.String(deliveryStreamAssumeRolePolicy),
		})
		Expect(err).To(BeNil())
		roleArn = *createRoleOutput.Role.Arn

		// Attach S3 permissions to IAM Role
		_, err = iamClient.PutRolePolicyWithContext(ctx, &iam.PutRolePolicyInput{
			RoleName:       aws.String(deliveryStreamRoleName),
			PolicyName:     aws.String("FirehoseS3Permissions"),
			PolicyDocument: aws.String(deliveryStreamRolePolicy),
		})
		Expect(err).To(BeNil())

		// Wait for permissions to propagate
		time.Sleep(30 * time.Second)

		// Create Firehose Delivery Stream
		firehoseClient = firehose.New(session.Must(session.NewSession(&aws.Config{Region: aws.String(config.Region)})))
		_, err = firehoseClient.CreateDeliveryStreamWithContext(ctx, &firehose.CreateDeliveryStreamInput{
			DeliveryStreamName: aws.String(deliveryStreamName),
			DeliveryStreamType: aws.String(firehose.DeliveryStreamTypeDirectPut),
			ExtendedS3DestinationConfiguration: &firehose.ExtendedS3DestinationConfiguration{
				BucketARN: aws.String(bucketArn),
				RoleARN:   aws.String(roleArn),
			},
		})
		Expect(err).To(BeNil())
		describeDeliveryStreamOutput, err := firehoseClient.DescribeDeliveryStreamWithContext(ctx, &firehose.DescribeDeliveryStreamInput{
			DeliveryStreamName: aws.String(deliveryStreamName),
		})
		deliveryStreamArn = *describeDeliveryStreamOutput.DeliveryStreamDescription.DeliveryStreamARN

		// Create HTTP Route, Service, and Deployment
		httpDeployment, httpK8sService = testFramework.NewNginxApp(test.ElasticSearchOptions{
			Name:      k8sResourceName,
			Namespace: k8snamespace,
		})
		httpRoute = testFramework.NewHttpRoute(testGateway, httpK8sService, "Service")
		testFramework.ExpectCreated(ctx, httpRoute, httpDeployment, httpK8sService)

		// Create GRPC Route, Service, and Deployment
		grpcAppOptions := test.GrpcAppOptions{AppName: k8sResourceName, Namespace: k8snamespace}
		grpcDeployment, grpcK8sService = testFramework.NewGrpcBin(grpcAppOptions)
		grpcRouteRules := []gwv1alpha2.GRPCRouteRule{
			{
				BackendRefs: []gwv1alpha2.GRPCBackendRef{
					{
						BackendRef: gwv1alpha2.BackendRef{
							BackendObjectReference: gwv1beta1.BackendObjectReference{
								Name:      gwv1alpha2.ObjectName(grpcK8sService.Name),
								Namespace: lo.ToPtr(gwv1beta1.Namespace(grpcK8sService.Namespace)),
								Kind:      (*gwv1beta1.Kind)(lo.ToPtr("Service")),
								Port:      lo.ToPtr(gwv1beta1.PortNumber(19000)),
							},
						},
					},
				},
			},
		}
		grpcRoute = testFramework.NewGRPCRoute(k8snamespace, testGateway, grpcRouteRules)
		testFramework.ExpectCreated(ctx, grpcRoute, grpcDeployment, grpcK8sService)
	})

	It("creation produces an Access Log Subscription for the corresponding Service Network when the targetRef's Kind is Gateway", func() {
		accessLogPolicy := &anv1alpha1.AccessLogPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      k8sResourceName,
				Namespace: k8snamespace,
			},
			Spec: anv1alpha1.AccessLogPolicySpec{
				DestinationArn: aws.String(bucketArn),
				TargetRef: &gwv1alpha2.PolicyTargetReference{
					Group:     gwv1beta1.GroupName,
					Kind:      "Gateway",
					Name:      gwv1alpha2.ObjectName(testGateway.Name),
					Namespace: (*gwv1alpha2.Namespace)(aws.String(k8snamespace)),
				},
			},
		}
		testFramework.ExpectCreated(ctx, accessLogPolicy)

		Eventually(func(g Gomega) {
			// Policy status should be Accepted
			alpNamespacedName := types.NamespacedName{
				Name:      accessLogPolicy.Name,
				Namespace: accessLogPolicy.Namespace,
			}
			alp := &anv1alpha1.AccessLogPolicy{}
			err := testFramework.Client.Get(ctx, alpNamespacedName, alp)
			g.Expect(err).To(BeNil())
			g.Expect(len(alp.Status.Conditions)).To(BeEquivalentTo(1))
			g.Expect(alp.Status.Conditions[0].Type).To(BeEquivalentTo(string(gwv1alpha2.PolicyConditionAccepted)))
			g.Expect(alp.Status.Conditions[0].Status).To(BeEquivalentTo(metav1.ConditionTrue))
			g.Expect(alp.Status.Conditions[0].ObservedGeneration).To(BeEquivalentTo(1))
			g.Expect(alp.Status.Conditions[0].Reason).To(BeEquivalentTo(string(gwv1alpha2.PolicyReasonAccepted)))

			// Service Network should have Access Log Subscription with S3 Bucket destination
			listALSInput := &vpclattice.ListAccessLogSubscriptionsInput{
				ResourceIdentifier: testServiceNetwork.Arn,
			}
			listALSOutput, err := testFramework.LatticeClient.ListAccessLogSubscriptionsWithContext(ctx, listALSInput)
			g.Expect(err).To(BeNil())
			g.Expect(len(listALSOutput.Items)).To(BeEquivalentTo(1))
			g.Expect(listALSOutput.Items[0].ResourceId).To(BeEquivalentTo(testServiceNetwork.Id))
			g.Expect(*listALSOutput.Items[0].DestinationArn).To(BeEquivalentTo(bucketArn))

			// Access Log Subscription ARN should be in the Access Log Policy's annotations
			g.Expect(alp.Annotations[anv1alpha1.AccessLogSubscriptionAnnotationKey]).To(BeEquivalentTo(*listALSOutput.Items[0].Arn))

			// Access Log Subscription should have default tags and Access Log Policy tag applied
			expectedTags := testFramework.Cloud.DefaultTagsMergedWith(services.Tags{
				lattice.AccessLogPolicyTagKey: aws.String(alpNamespacedName.String()),
			})
			listTagsInput := &vpclattice.ListTagsForResourceInput{
				ResourceArn: listALSOutput.Items[0].Arn,
			}
			listTagsOutput, err := testFramework.LatticeClient.ListTagsForResourceWithContext(ctx, listTagsInput)
			g.Expect(err).To(BeNil())
			g.Expect(listTagsOutput.Tags).To(BeEquivalentTo(expectedTags))
		}).Should(Succeed())
	})

	It("creation produces an Access Log Subscription for the corresponding VPC Lattice Service when the targetRef's Kind is HTTPRoute", func() {
		accessLogPolicy := &anv1alpha1.AccessLogPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      k8sResourceName,
				Namespace: k8snamespace,
			},
			Spec: anv1alpha1.AccessLogPolicySpec{
				DestinationArn: aws.String(bucketArn),
				TargetRef: &gwv1alpha2.PolicyTargetReference{
					Group:     gwv1beta1.GroupName,
					Kind:      "HTTPRoute",
					Name:      gwv1alpha2.ObjectName(httpRoute.Name),
					Namespace: (*gwv1alpha2.Namespace)(aws.String(k8snamespace)),
				},
			},
		}
		testFramework.ExpectCreated(ctx, accessLogPolicy)

		Eventually(func(g Gomega) {
			// Policy status should be Accepted
			alpNamespacedName := types.NamespacedName{
				Name:      accessLogPolicy.Name,
				Namespace: accessLogPolicy.Namespace,
			}
			alp := &anv1alpha1.AccessLogPolicy{}
			err := testFramework.Client.Get(ctx, alpNamespacedName, alp)
			g.Expect(err).To(BeNil())
			g.Expect(len(alp.Status.Conditions)).To(BeEquivalentTo(1))
			g.Expect(alp.Status.Conditions[0].Type).To(BeEquivalentTo(string(gwv1alpha2.PolicyConditionAccepted)))
			g.Expect(alp.Status.Conditions[0].Status).To(BeEquivalentTo(metav1.ConditionTrue))
			g.Expect(alp.Status.Conditions[0].ObservedGeneration).To(BeEquivalentTo(1))
			g.Expect(alp.Status.Conditions[0].Reason).To(BeEquivalentTo(string(gwv1alpha2.PolicyReasonAccepted)))

			// VPC Lattice Service should have Access Log Subscription with S3 Bucket destination
			latticeService := testFramework.GetVpcLatticeService(ctx, core.NewHTTPRoute(*httpRoute))
			listALSInput := &vpclattice.ListAccessLogSubscriptionsInput{
				ResourceIdentifier: latticeService.Arn,
			}
			listALSOutput, err := testFramework.LatticeClient.ListAccessLogSubscriptionsWithContext(ctx, listALSInput)
			g.Expect(err).To(BeNil())
			g.Expect(len(listALSOutput.Items)).To(BeEquivalentTo(1))
			g.Expect(listALSOutput.Items[0].ResourceId).To(BeEquivalentTo(latticeService.Id))
			g.Expect(*listALSOutput.Items[0].DestinationArn).To(BeEquivalentTo(bucketArn))

			// Access Log Subscription ARN should be in the Access Log Policy's annotations
			g.Expect(alp.Annotations[anv1alpha1.AccessLogSubscriptionAnnotationKey]).To(BeEquivalentTo(*listALSOutput.Items[0].Arn))

			// Access Log Subscription should have default tags and Access Log Policy tag applied
			expectedTags := testFramework.Cloud.DefaultTagsMergedWith(services.Tags{
				lattice.AccessLogPolicyTagKey: aws.String(alpNamespacedName.String()),
			})
			listTagsInput := &vpclattice.ListTagsForResourceInput{
				ResourceArn: listALSOutput.Items[0].Arn,
			}
			listTagsOutput, err := testFramework.LatticeClient.ListTagsForResourceWithContext(ctx, listTagsInput)
			g.Expect(err).To(BeNil())
			g.Expect(listTagsOutput.Tags).To(BeEquivalentTo(expectedTags))
		}).Should(Succeed())
	})

	It("creation produces an Access Log Subscription for the corresponding VPC Lattice Service when the targetRef's Kind is GRPCRoute", func() {
		accessLogPolicy := &anv1alpha1.AccessLogPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      k8sResourceName,
				Namespace: k8snamespace,
			},
			Spec: anv1alpha1.AccessLogPolicySpec{
				DestinationArn: aws.String(bucketArn),
				TargetRef: &gwv1alpha2.PolicyTargetReference{
					Group:     gwv1beta1.GroupName,
					Kind:      "GRPCRoute",
					Name:      gwv1alpha2.ObjectName(grpcRoute.Name),
					Namespace: (*gwv1alpha2.Namespace)(aws.String(k8snamespace)),
				},
			},
		}
		testFramework.ExpectCreated(ctx, accessLogPolicy)

		Eventually(func(g Gomega) {
			// Policy status should be Accepted
			alpNamespacedName := types.NamespacedName{
				Name:      accessLogPolicy.Name,
				Namespace: accessLogPolicy.Namespace,
			}
			alp := &anv1alpha1.AccessLogPolicy{}
			err := testFramework.Client.Get(ctx, alpNamespacedName, alp)
			g.Expect(err).To(BeNil())
			g.Expect(len(alp.Status.Conditions)).To(BeEquivalentTo(1))
			g.Expect(alp.Status.Conditions[0].Type).To(BeEquivalentTo(string(gwv1alpha2.PolicyConditionAccepted)))
			g.Expect(alp.Status.Conditions[0].Status).To(BeEquivalentTo(metav1.ConditionTrue))
			g.Expect(alp.Status.Conditions[0].ObservedGeneration).To(BeEquivalentTo(1))
			g.Expect(alp.Status.Conditions[0].Reason).To(BeEquivalentTo(string(gwv1alpha2.PolicyReasonAccepted)))

			// VPC Lattice Service should have Access Log Subscription with S3 Bucket destination
			latticeService := testFramework.GetVpcLatticeService(ctx, core.NewGRPCRoute(*grpcRoute))
			listALSInput := &vpclattice.ListAccessLogSubscriptionsInput{
				ResourceIdentifier: latticeService.Arn,
			}
			listALSOutput, err := testFramework.LatticeClient.ListAccessLogSubscriptionsWithContext(ctx, listALSInput)
			g.Expect(err).To(BeNil())
			g.Expect(len(listALSOutput.Items)).To(BeEquivalentTo(1))
			g.Expect(listALSOutput.Items[0].ResourceId).To(BeEquivalentTo(latticeService.Id))
			g.Expect(*listALSOutput.Items[0].DestinationArn).To(BeEquivalentTo(bucketArn))

			// Access Log Subscription ARN should be in the Access Log Policy's annotations
			g.Expect(alp.Annotations[anv1alpha1.AccessLogSubscriptionAnnotationKey]).To(BeEquivalentTo(*listALSOutput.Items[0].Arn))

			// Access Log Subscription should have default tags and Access Log Policy tag applied
			expectedTags := testFramework.Cloud.DefaultTagsMergedWith(services.Tags{
				lattice.AccessLogPolicyTagKey: aws.String(alpNamespacedName.String()),
			})
			listTagsInput := &vpclattice.ListTagsForResourceInput{
				ResourceArn: listALSOutput.Items[0].Arn,
			}
			listTagsOutput, err := testFramework.LatticeClient.ListTagsForResourceWithContext(ctx, listTagsInput)
			g.Expect(err).To(BeNil())
			g.Expect(listTagsOutput.Tags).To(BeEquivalentTo(expectedTags))
		}).Should(Succeed())
	})

	It("creation produces Access Log Subscriptions with Bucket, Log Group, and Delivery Stream destinations on the same targetRef", func() {
		// Create Access Log Policy for S3 Bucket
		s3AccessLogPolicy := &anv1alpha1.AccessLogPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      k8sResourceName + "-bucket",
				Namespace: k8snamespace,
			},
			Spec: anv1alpha1.AccessLogPolicySpec{
				DestinationArn: aws.String(bucketArn),
				TargetRef: &gwv1alpha2.PolicyTargetReference{
					Group:     gwv1beta1.GroupName,
					Kind:      "Gateway",
					Name:      gwv1alpha2.ObjectName(testGateway.Name),
					Namespace: (*gwv1alpha2.Namespace)(aws.String(k8snamespace)),
				},
			},
		}
		testFramework.ExpectCreated(ctx, s3AccessLogPolicy)

		// Create Access Log Policy for CloudWatch Log Group
		cwAccessLogPolicy := &anv1alpha1.AccessLogPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      k8sResourceName + "-log-group",
				Namespace: k8snamespace,
			},
			Spec: anv1alpha1.AccessLogPolicySpec{
				DestinationArn: aws.String(logGroupArn),
				TargetRef: &gwv1alpha2.PolicyTargetReference{
					Group:     gwv1beta1.GroupName,
					Kind:      "Gateway",
					Name:      gwv1alpha2.ObjectName(testGateway.Name),
					Namespace: (*gwv1alpha2.Namespace)(aws.String(k8snamespace)),
				},
			},
		}
		testFramework.ExpectCreated(ctx, cwAccessLogPolicy)

		// Create Access Log Policy for Firehose Delivery Stream
		fhAccessLogPolicy := &anv1alpha1.AccessLogPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      k8sResourceName + "-delivery-stream",
				Namespace: k8snamespace,
			},
			Spec: anv1alpha1.AccessLogPolicySpec{
				DestinationArn: aws.String(deliveryStreamArn),
				TargetRef: &gwv1alpha2.PolicyTargetReference{
					Group:     gwv1beta1.GroupName,
					Kind:      "Gateway",
					Name:      gwv1alpha2.ObjectName(testGateway.Name),
					Namespace: (*gwv1alpha2.Namespace)(aws.String(k8snamespace)),
				},
			},
		}
		testFramework.ExpectCreated(ctx, fhAccessLogPolicy)

		// Service Network should have Access Log Subscription for each destination type
		Eventually(func(g Gomega) {
			output, err := testFramework.LatticeClient.ListAccessLogSubscriptions(&vpclattice.ListAccessLogSubscriptionsInput{
				ResourceIdentifier: testServiceNetwork.Arn,
			})
			g.Expect(err).To(BeNil())
			g.Expect(len(output.Items)).To(BeEquivalentTo(3))

			getDestinationArn := func(s *vpclattice.AccessLogSubscriptionSummary) string {
				return *s.DestinationArn
			}

			g.Expect(output.Items).To(ContainElement(WithTransform(getDestinationArn, Equal(bucketArn))))
			g.Expect(output.Items).To(ContainElement(WithTransform(getDestinationArn, Equal(logGroupArn))))
			g.Expect(output.Items).To(ContainElement(WithTransform(getDestinationArn, Equal(deliveryStreamArn))))
		}).Should(Succeed())

		// Every Access Log Policy status should be Accepted
		for _, accessLogPolicy := range []*anv1alpha1.AccessLogPolicy{s3AccessLogPolicy, cwAccessLogPolicy} {
			alpNamespacedName := types.NamespacedName{
				Name:      accessLogPolicy.Name,
				Namespace: accessLogPolicy.Namespace,
			}
			alp := &anv1alpha1.AccessLogPolicy{}
			err := testFramework.Client.Get(ctx, alpNamespacedName, alp)
			Expect(err).To(BeNil())
			Expect(len(alp.Status.Conditions)).To(BeEquivalentTo(1))
			Expect(alp.Status.Conditions[0].Type).To(BeEquivalentTo(string(gwv1alpha2.PolicyConditionAccepted)))
			Expect(alp.Status.Conditions[0].Status).To(BeEquivalentTo(metav1.ConditionTrue))
			Expect(alp.Status.Conditions[0].ObservedGeneration).To(BeEquivalentTo(1))
			Expect(alp.Status.Conditions[0].Reason).To(BeEquivalentTo(string(gwv1alpha2.PolicyReasonAccepted)))
		}
	})

	It("creation sets Access Log Policy status to Conflicted when creating a new policy for the same targetRef and destination type", func() {
		accessLogPolicy1 := &anv1alpha1.AccessLogPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      k8sResourceName + "-1",
				Namespace: k8snamespace,
			},
			Spec: anv1alpha1.AccessLogPolicySpec{
				DestinationArn: aws.String(bucketArn),
				TargetRef: &gwv1alpha2.PolicyTargetReference{
					Group:     gwv1beta1.GroupName,
					Kind:      "Gateway",
					Name:      gwv1alpha2.ObjectName(testGateway.Name),
					Namespace: (*gwv1alpha2.Namespace)(aws.String(k8snamespace)),
				},
			},
		}
		testFramework.ExpectCreated(ctx, accessLogPolicy1)

		accessLogPolicy2 := &anv1alpha1.AccessLogPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      k8sResourceName + "-2",
				Namespace: k8snamespace,
			},
			Spec: anv1alpha1.AccessLogPolicySpec{
				DestinationArn: aws.String(bucketArn),
				TargetRef: &gwv1alpha2.PolicyTargetReference{
					Group:     gwv1beta1.GroupName,
					Kind:      "Gateway",
					Name:      gwv1alpha2.ObjectName(testGateway.Name),
					Namespace: (*gwv1alpha2.Namespace)(aws.String(k8snamespace)),
				},
			},
		}
		testFramework.ExpectCreated(ctx, accessLogPolicy2)

		// Policy status should be Conflicted
		Eventually(func(g Gomega) {
			alpNamespacedName := types.NamespacedName{
				Name:      accessLogPolicy2.Name,
				Namespace: accessLogPolicy2.Namespace,
			}
			alp := &anv1alpha1.AccessLogPolicy{}
			err := testFramework.Client.Get(ctx, alpNamespacedName, alp)
			g.Expect(err).To(BeNil())
			g.Expect(len(alp.Status.Conditions)).To(BeEquivalentTo(1))
			g.Expect(alp.Status.Conditions[0].Type).To(BeEquivalentTo(string(gwv1alpha2.PolicyConditionAccepted)))
			g.Expect(alp.Status.Conditions[0].Status).To(BeEquivalentTo(metav1.ConditionFalse))
			g.Expect(alp.Status.Conditions[0].ObservedGeneration).To(BeEquivalentTo(1))
			g.Expect(alp.Status.Conditions[0].Reason).To(BeEquivalentTo(string(gwv1alpha2.PolicyReasonConflicted)))
		}).Should(Succeed())
	})

	It("creation sets Access Log Policy status to Invalid when the destination does not exist", func() {
		accessLogPolicy := &anv1alpha1.AccessLogPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      k8sResourceName,
				Namespace: k8snamespace,
			},
			Spec: anv1alpha1.AccessLogPolicySpec{
				DestinationArn: aws.String(bucketArn + "foo"),
				TargetRef: &gwv1alpha2.PolicyTargetReference{
					Group:     gwv1beta1.GroupName,
					Kind:      "Gateway",
					Name:      gwv1alpha2.ObjectName(testGateway.Name),
					Namespace: (*gwv1alpha2.Namespace)(aws.String(k8snamespace)),
				},
			},
		}
		testFramework.ExpectCreated(ctx, accessLogPolicy)

		// Policy status should be Invalid
		Eventually(func(g Gomega) {
			alpNamespacedName := types.NamespacedName{
				Name:      accessLogPolicy.Name,
				Namespace: accessLogPolicy.Namespace,
			}
			alp := &anv1alpha1.AccessLogPolicy{}
			err := testFramework.Client.Get(ctx, alpNamespacedName, alp)
			g.Expect(err).To(BeNil())
			g.Expect(len(alp.Status.Conditions)).To(BeEquivalentTo(1))
			g.Expect(alp.Status.Conditions[0].Type).To(BeEquivalentTo(string(gwv1alpha2.PolicyConditionAccepted)))
			g.Expect(alp.Status.Conditions[0].Status).To(BeEquivalentTo(metav1.ConditionFalse))
			g.Expect(alp.Status.Conditions[0].ObservedGeneration).To(BeEquivalentTo(1))
			g.Expect(alp.Status.Conditions[0].Reason).To(BeEquivalentTo(string(gwv1alpha2.PolicyReasonInvalid)))
		}).Should(Succeed())
	})

	It("creation sets Access Log Policy status to Invalid when the targetRef's Group is not gateway.networking.k8s.io", func() {
		accessLogPolicy := &anv1alpha1.AccessLogPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      k8sResourceName,
				Namespace: k8snamespace,
			},
			Spec: anv1alpha1.AccessLogPolicySpec{
				DestinationArn: aws.String(bucketArn),
				TargetRef: &gwv1alpha2.PolicyTargetReference{
					Group:     "invalid",
					Kind:      "Gateway",
					Name:      gwv1alpha2.ObjectName(testGateway.Name),
					Namespace: (*gwv1alpha2.Namespace)(aws.String(k8snamespace)),
				},
			},
		}
		testFramework.ExpectCreated(ctx, accessLogPolicy)

		// Policy status should be Invalid
		Eventually(func(g Gomega) {
			alpNamespacedName := types.NamespacedName{
				Name:      accessLogPolicy.Name,
				Namespace: accessLogPolicy.Namespace,
			}
			alp := &anv1alpha1.AccessLogPolicy{}
			err := testFramework.Client.Get(ctx, alpNamespacedName, alp)
			g.Expect(err).To(BeNil())
			g.Expect(len(alp.Status.Conditions)).To(BeEquivalentTo(1))
			g.Expect(alp.Status.Conditions[0].Type).To(BeEquivalentTo(string(gwv1alpha2.PolicyConditionAccepted)))
			g.Expect(alp.Status.Conditions[0].Status).To(BeEquivalentTo(metav1.ConditionFalse))
			g.Expect(alp.Status.Conditions[0].ObservedGeneration).To(BeEquivalentTo(1))
			g.Expect(alp.Status.Conditions[0].Reason).To(BeEquivalentTo(string(gwv1alpha2.PolicyReasonInvalid)))
		}).Should(Succeed())
	})

	It("creation sets Access Log Policy status to Invalid when the targetRef's Kind is not Gateway, HTTPRoute, or GRPCRoute", func() {
		accessLogPolicy := &anv1alpha1.AccessLogPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      k8sResourceName,
				Namespace: k8snamespace,
			},
			Spec: anv1alpha1.AccessLogPolicySpec{
				DestinationArn: aws.String(bucketArn),
				TargetRef: &gwv1alpha2.PolicyTargetReference{
					Group:     gwv1beta1.GroupName,
					Kind:      "Service",
					Name:      gwv1alpha2.ObjectName(testGateway.Name),
					Namespace: (*gwv1alpha2.Namespace)(aws.String(k8snamespace)),
				},
			},
		}
		testFramework.ExpectCreated(ctx, accessLogPolicy)

		// Policy status should be Invalid
		Eventually(func(g Gomega) {
			alpNamespacedName := types.NamespacedName{
				Name:      accessLogPolicy.Name,
				Namespace: accessLogPolicy.Namespace,
			}
			alp := &anv1alpha1.AccessLogPolicy{}
			err := testFramework.Client.Get(ctx, alpNamespacedName, alp)
			g.Expect(err).To(BeNil())
			g.Expect(len(alp.Status.Conditions)).To(BeEquivalentTo(1))
			g.Expect(alp.Status.Conditions[0].Type).To(BeEquivalentTo(string(gwv1alpha2.PolicyConditionAccepted)))
			g.Expect(alp.Status.Conditions[0].Status).To(BeEquivalentTo(metav1.ConditionFalse))
			g.Expect(alp.Status.Conditions[0].ObservedGeneration).To(BeEquivalentTo(1))
			g.Expect(alp.Status.Conditions[0].Reason).To(BeEquivalentTo(string(gwv1alpha2.PolicyReasonInvalid)))
		}).Should(Succeed())
	})

	It("update properly changes or replaces Access Log Subscription and sets Access Log Policy status", func() {
		originalAlsArn := ""
		currentAlsArn := ""
		expectedGeneration := 1
		latticeService := testFramework.GetVpcLatticeService(ctx, core.NewHTTPRoute(*httpRoute))
		accessLogPolicy := &anv1alpha1.AccessLogPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      k8sResourceName,
				Namespace: k8snamespace,
			},
			Spec: anv1alpha1.AccessLogPolicySpec{
				DestinationArn: aws.String(logGroupArn),
				TargetRef: &gwv1alpha2.PolicyTargetReference{
					Group:     gwv1beta1.GroupName,
					Kind:      "Gateway",
					Name:      gwv1alpha2.ObjectName(testGateway.Name),
					Namespace: (*gwv1alpha2.Namespace)(aws.String(k8snamespace)),
				},
			},
		}
		alpNamespacedName := types.NamespacedName{
			Name:      accessLogPolicy.Name,
			Namespace: accessLogPolicy.Namespace,
		}
		testFramework.ExpectCreated(ctx, accessLogPolicy)

		Eventually(func(g Gomega) {
			// Policy status should be Accepted
			alp := &anv1alpha1.AccessLogPolicy{}
			err := testFramework.Client.Get(ctx, alpNamespacedName, alp)
			g.Expect(err).To(BeNil())

			// Service Network should have 1 Access Log Subscription with CloudWatch Log Group destination
			listALSInput := &vpclattice.ListAccessLogSubscriptionsInput{
				ResourceIdentifier: testServiceNetwork.Arn,
			}
			listALSOutput, err := testFramework.LatticeClient.ListAccessLogSubscriptionsWithContext(ctx, listALSInput)
			g.Expect(err).To(BeNil())
			g.Expect(len(listALSOutput.Items)).To(BeEquivalentTo(1))
			g.Expect(listALSOutput.Items[0].ResourceId).To(BeEquivalentTo(testServiceNetwork.Id))
			g.Expect(*listALSOutput.Items[0].DestinationArn).To(BeEquivalentTo(logGroupArn))

			// Access Log Subscription ARN should be in the Access Log Policy's annotations
			g.Expect(alp.Annotations[anv1alpha1.AccessLogSubscriptionAnnotationKey]).To(BeEquivalentTo(*listALSOutput.Items[0].Arn))

			currentAlsArn = alp.Annotations[anv1alpha1.AccessLogSubscriptionAnnotationKey]
			originalAlsArn = alp.Annotations[anv1alpha1.AccessLogSubscriptionAnnotationKey]
		}).Should(Succeed())

		// Update to different destination of same type
		alp := &anv1alpha1.AccessLogPolicy{}
		err := testFramework.Client.Get(ctx, alpNamespacedName, alp)
		Expect(err).To(BeNil())
		alp.Spec.DestinationArn = aws.String(logGroup2Arn)
		testFramework.ExpectUpdated(ctx, alp)
		expectedGeneration = expectedGeneration + 1

		Eventually(func(g Gomega) {
			// Policy status should be Accepted
			alpNamespacedName := types.NamespacedName{
				Name:      accessLogPolicy.Name,
				Namespace: accessLogPolicy.Namespace,
			}
			alp := &anv1alpha1.AccessLogPolicy{}
			err := testFramework.Client.Get(ctx, alpNamespacedName, alp)
			g.Expect(err).To(BeNil())
			g.Expect(len(alp.Status.Conditions)).To(BeEquivalentTo(1))
			g.Expect(alp.Status.Conditions[0].Type).To(BeEquivalentTo(string(gwv1alpha2.PolicyConditionAccepted)))
			g.Expect(alp.Status.Conditions[0].Status).To(BeEquivalentTo(metav1.ConditionTrue))
			g.Expect(alp.Status.Conditions[0].ObservedGeneration).To(BeEquivalentTo(expectedGeneration))
			g.Expect(alp.Status.Conditions[0].Reason).To(BeEquivalentTo(string(gwv1alpha2.PolicyReasonAccepted)))

			// Service Network should have 1 Access Log Subscription with updated CloudWatch Log Group destination
			listALSInput := &vpclattice.ListAccessLogSubscriptionsInput{
				ResourceIdentifier: testServiceNetwork.Arn,
			}
			listALSOutput, err := testFramework.LatticeClient.ListAccessLogSubscriptionsWithContext(ctx, listALSInput)
			g.Expect(err).To(BeNil())
			g.Expect(len(listALSOutput.Items)).To(BeEquivalentTo(1))
			g.Expect(listALSOutput.Items[0].ResourceId).To(BeEquivalentTo(testServiceNetwork.Id))
			g.Expect(*listALSOutput.Items[0].DestinationArn).To(BeEquivalentTo(logGroup2Arn))

			// Access Log Subscription ARN should be unchanged
			g.Expect(alp.Annotations[anv1alpha1.AccessLogSubscriptionAnnotationKey]).To(BeEquivalentTo(*listALSOutput.Items[0].Arn))
			g.Expect(alp.Annotations[anv1alpha1.AccessLogSubscriptionAnnotationKey]).To(BeEquivalentTo(originalAlsArn))

			// Access Log Subscription should have default tags and Access Log Policy tag applied
			expectedTags := testFramework.Cloud.DefaultTagsMergedWith(services.Tags{
				lattice.AccessLogPolicyTagKey: aws.String(alpNamespacedName.String()),
			})
			listTagsInput := &vpclattice.ListTagsForResourceInput{
				ResourceArn: listALSOutput.Items[0].Arn,
			}
			listTagsOutput, err := testFramework.LatticeClient.ListTagsForResourceWithContext(ctx, listTagsInput)
			g.Expect(err).To(BeNil())
			g.Expect(listTagsOutput.Tags).To(BeEquivalentTo(expectedTags))
		}).Should(Succeed())

		// Update to different destination of different type
		alp = &anv1alpha1.AccessLogPolicy{}
		err = testFramework.Client.Get(ctx, alpNamespacedName, alp)
		Expect(err).To(BeNil())
		alp.Spec.DestinationArn = aws.String(bucketArn)
		testFramework.ExpectUpdated(ctx, alp)
		expectedGeneration = expectedGeneration + 1

		Eventually(func(g Gomega) {
			// Policy status should be Accepted
			alp := &anv1alpha1.AccessLogPolicy{}
			err := testFramework.Client.Get(ctx, alpNamespacedName, alp)
			g.Expect(err).To(BeNil())
			g.Expect(len(alp.Status.Conditions)).To(BeEquivalentTo(1))
			g.Expect(alp.Status.Conditions[0].Type).To(BeEquivalentTo(string(gwv1alpha2.PolicyConditionAccepted)))
			g.Expect(alp.Status.Conditions[0].Status).To(BeEquivalentTo(metav1.ConditionTrue))
			g.Expect(alp.Status.Conditions[0].ObservedGeneration).To(BeEquivalentTo(expectedGeneration))
			g.Expect(alp.Status.Conditions[0].Reason).To(BeEquivalentTo(string(gwv1alpha2.PolicyReasonAccepted)))

			// Service Network should only have 1 Access Log Subscription, with S3 Bucket destination
			listALSInput := &vpclattice.ListAccessLogSubscriptionsInput{
				ResourceIdentifier: testServiceNetwork.Arn,
			}
			listALSOutput, err := testFramework.LatticeClient.ListAccessLogSubscriptionsWithContext(ctx, listALSInput)
			g.Expect(err).To(BeNil())
			g.Expect(len(listALSOutput.Items)).To(BeEquivalentTo(1))
			g.Expect(listALSOutput.Items[0].ResourceId).To(BeEquivalentTo(testServiceNetwork.Id))
			g.Expect(*listALSOutput.Items[0].DestinationArn).To(BeEquivalentTo(bucketArn))

			// New Access Log Subscription ARN should be in the Access Log Policy's annotations
			g.Expect(alp.Annotations[anv1alpha1.AccessLogSubscriptionAnnotationKey]).To(BeEquivalentTo(*listALSOutput.Items[0].Arn))
			g.Expect(alp.Annotations[anv1alpha1.AccessLogSubscriptionAnnotationKey]).ToNot(BeEquivalentTo(originalAlsArn))
			currentAlsArn = alp.Annotations[anv1alpha1.AccessLogSubscriptionAnnotationKey]

			// New Access Log Subscription should have default tags and Access Log Policy tag applied
			expectedTags := testFramework.Cloud.DefaultTagsMergedWith(services.Tags{
				lattice.AccessLogPolicyTagKey: aws.String(alpNamespacedName.String()),
			})
			listTagsInput := &vpclattice.ListTagsForResourceInput{
				ResourceArn: listALSOutput.Items[0].Arn,
			}
			listTagsOutput, err := testFramework.LatticeClient.ListTagsForResourceWithContext(ctx, listTagsInput)
			g.Expect(err).To(BeNil())
			g.Expect(listTagsOutput.Tags).To(BeEquivalentTo(expectedTags))
		}).Should(Succeed())

		// Update to different targetRef
		alp = &anv1alpha1.AccessLogPolicy{}
		err = testFramework.Client.Get(ctx, alpNamespacedName, alp)
		Expect(err).To(BeNil())
		alp.Spec.TargetRef = &gwv1alpha2.PolicyTargetReference{
			Group:     gwv1beta1.GroupName,
			Kind:      "HTTPRoute",
			Name:      gwv1alpha2.ObjectName(httpRoute.Name),
			Namespace: (*gwv1alpha2.Namespace)(aws.String(k8snamespace)),
		}
		testFramework.ExpectUpdated(ctx, alp)
		expectedGeneration = expectedGeneration + 1

		Eventually(func(g Gomega) {
			// Policy status should be Accepted
			alp := &anv1alpha1.AccessLogPolicy{}
			err := testFramework.Client.Get(ctx, alpNamespacedName, alp)
			g.Expect(err).To(BeNil())
			g.Expect(len(alp.Status.Conditions)).To(BeEquivalentTo(1))
			g.Expect(alp.Status.Conditions[0].Type).To(BeEquivalentTo(string(gwv1alpha2.PolicyConditionAccepted)))
			g.Expect(alp.Status.Conditions[0].Status).To(BeEquivalentTo(metav1.ConditionTrue))
			g.Expect(alp.Status.Conditions[0].ObservedGeneration).To(BeEquivalentTo(expectedGeneration))
			g.Expect(alp.Status.Conditions[0].Reason).To(BeEquivalentTo(string(gwv1alpha2.PolicyReasonAccepted)))

			// Service Network should have 0 Access Log Subscriptions
			listALSForSNInput := &vpclattice.ListAccessLogSubscriptionsInput{
				ResourceIdentifier: testServiceNetwork.Arn,
			}
			listALSForSNOutput, err := testFramework.LatticeClient.ListAccessLogSubscriptionsWithContext(ctx, listALSForSNInput)
			g.Expect(err).To(BeNil())
			g.Expect(len(listALSForSNOutput.Items)).To(BeEquivalentTo(0))

			// VPC Lattice Service should have 1 Access Log Subscription, with S3 Bucket destination
			listALSForSvcInput := &vpclattice.ListAccessLogSubscriptionsInput{
				ResourceIdentifier: latticeService.Arn,
			}
			listALSForSvcOutput, err := testFramework.LatticeClient.ListAccessLogSubscriptionsWithContext(ctx, listALSForSvcInput)
			g.Expect(err).To(BeNil())
			g.Expect(len(listALSForSvcOutput.Items)).To(BeEquivalentTo(1))
			g.Expect(*listALSForSvcOutput.Items[0].DestinationArn).To(BeEquivalentTo(bucketArn))
			g.Expect(listALSForSvcOutput.Items[0].ResourceId).To(BeEquivalentTo(latticeService.Id))

			// New Access Log Subscription ARN should be in the Access Log Policy's annotations
			g.Expect(alp.Annotations[anv1alpha1.AccessLogSubscriptionAnnotationKey]).To(BeEquivalentTo(*listALSForSvcOutput.Items[0].Arn))
			g.Expect(alp.Annotations[anv1alpha1.AccessLogSubscriptionAnnotationKey]).ToNot(BeEquivalentTo(originalAlsArn))
			currentAlsArn = alp.Annotations[anv1alpha1.AccessLogSubscriptionAnnotationKey]

			// New Access Log Subscription should have default tags and Access Log Policy tag applied
			expectedTags := testFramework.Cloud.DefaultTagsMergedWith(services.Tags{
				lattice.AccessLogPolicyTagKey: aws.String(alpNamespacedName.String()),
			})
			listTagsInput := &vpclattice.ListTagsForResourceInput{
				ResourceArn: listALSForSvcOutput.Items[0].Arn,
			}
			listTagsOutput, err := testFramework.LatticeClient.ListTagsForResourceWithContext(ctx, listTagsInput)
			g.Expect(err).To(BeNil())
			g.Expect(listTagsOutput.Tags).To(BeEquivalentTo(expectedTags))
		}).Should(Succeed())

		// Update to destination that does not exist
		alp = &anv1alpha1.AccessLogPolicy{}
		err = testFramework.Client.Get(ctx, alpNamespacedName, alp)
		Expect(err).To(BeNil())
		alp.Spec.DestinationArn = aws.String(bucketArn + "doesnotexist")
		testFramework.ExpectUpdated(ctx, alp)
		expectedGeneration = expectedGeneration + 1

		Eventually(func(g Gomega) {
			// Policy status should be Invalid
			alp := &anv1alpha1.AccessLogPolicy{}
			err := testFramework.Client.Get(ctx, alpNamespacedName, alp)
			g.Expect(err).To(BeNil())
			g.Expect(len(alp.Status.Conditions)).To(BeEquivalentTo(1))
			g.Expect(alp.Status.Conditions[0].Type).To(BeEquivalentTo(string(gwv1alpha2.PolicyConditionAccepted)))
			g.Expect(alp.Status.Conditions[0].Status).To(BeEquivalentTo(metav1.ConditionFalse))
			g.Expect(alp.Status.Conditions[0].ObservedGeneration).To(BeEquivalentTo(expectedGeneration))
			g.Expect(alp.Status.Conditions[0].Reason).To(BeEquivalentTo(string(gwv1alpha2.PolicyReasonInvalid)))

			// VPC Lattice Service should still have previous Access Log Subscription
			listALSInput := &vpclattice.ListAccessLogSubscriptionsInput{
				ResourceIdentifier: latticeService.Arn,
			}
			listALSOutput, err := testFramework.LatticeClient.ListAccessLogSubscriptionsWithContext(ctx, listALSInput)
			g.Expect(err).To(BeNil())
			g.Expect(len(listALSOutput.Items)).To(BeEquivalentTo(1))
			g.Expect(listALSOutput.Items[0].ResourceId).To(BeEquivalentTo(latticeService.Id))
			g.Expect(*listALSOutput.Items[0].DestinationArn).To(BeEquivalentTo(bucketArn))

			// Same Access Log Subscription ARN should be in the Access Log Policy's annotations
			g.Expect(alp.Annotations[anv1alpha1.AccessLogSubscriptionAnnotationKey]).To(BeEquivalentTo(*listALSOutput.Items[0].Arn))
			g.Expect(alp.Annotations[anv1alpha1.AccessLogSubscriptionAnnotationKey]).To(BeEquivalentTo(currentAlsArn))
		}).Should(Succeed())

		// Update to targetRef that does not exist
		alp = &anv1alpha1.AccessLogPolicy{}
		err = testFramework.Client.Get(ctx, alpNamespacedName, alp)
		Expect(err).To(BeNil())
		alp.Spec.DestinationArn = aws.String(bucketArn)
		alp.Spec.TargetRef = &gwv1alpha2.PolicyTargetReference{
			Group:     gwv1beta1.GroupName,
			Kind:      "Gateway",
			Name:      "doesnotexist",
			Namespace: (*gwv1alpha2.Namespace)(aws.String(k8snamespace)),
		}
		testFramework.ExpectUpdated(ctx, alp)
		expectedGeneration = expectedGeneration + 1

		Eventually(func(g Gomega) {
			// Policy status should be TargetNotFound
			alp := &anv1alpha1.AccessLogPolicy{}
			err := testFramework.Client.Get(ctx, alpNamespacedName, alp)
			g.Expect(err).To(BeNil())
			g.Expect(len(alp.Status.Conditions)).To(BeEquivalentTo(1))
			g.Expect(alp.Status.Conditions[0].Type).To(BeEquivalentTo(string(gwv1alpha2.PolicyConditionAccepted)))
			g.Expect(alp.Status.Conditions[0].Status).To(BeEquivalentTo(metav1.ConditionFalse))
			g.Expect(alp.Status.Conditions[0].ObservedGeneration).To(BeEquivalentTo(expectedGeneration))
			g.Expect(alp.Status.Conditions[0].Reason).To(BeEquivalentTo(string(gwv1alpha2.PolicyReasonTargetNotFound)))

			// VPC Lattice Service should still have previous Access Log Subscription
			listALSInput := &vpclattice.ListAccessLogSubscriptionsInput{
				ResourceIdentifier: latticeService.Arn,
			}
			listALSOutput, err := testFramework.LatticeClient.ListAccessLogSubscriptionsWithContext(ctx, listALSInput)
			g.Expect(err).To(BeNil())
			g.Expect(len(listALSOutput.Items)).To(BeEquivalentTo(1))
			g.Expect(listALSOutput.Items[0].ResourceId).To(BeEquivalentTo(latticeService.Id))
			g.Expect(*listALSOutput.Items[0].DestinationArn).To(BeEquivalentTo(bucketArn))

			// Same Access Log Subscription ARN should be in the Access Log Policy's annotations
			g.Expect(alp.Annotations[anv1alpha1.AccessLogSubscriptionAnnotationKey]).To(BeEquivalentTo(*listALSOutput.Items[0].Arn))
			g.Expect(alp.Annotations[anv1alpha1.AccessLogSubscriptionAnnotationKey]).To(BeEquivalentTo(currentAlsArn))
		}).Should(Succeed())

		// Update to targetRef with wrong namespace
		alp = &anv1alpha1.AccessLogPolicy{}
		err = testFramework.Client.Get(ctx, alpNamespacedName, alp)
		Expect(err).To(BeNil())
		alp.Spec.DestinationArn = aws.String(bucketArn)
		alp.Spec.TargetRef.Namespace = (*gwv1alpha2.Namespace)(aws.String("invalid"))
		testFramework.ExpectUpdated(ctx, alp)
		expectedGeneration = expectedGeneration + 1

		Eventually(func(g Gomega) {
			// Policy status should be Invalid
			alp := &anv1alpha1.AccessLogPolicy{}
			err := testFramework.Client.Get(ctx, alpNamespacedName, alp)
			g.Expect(err).To(BeNil())
			g.Expect(len(alp.Status.Conditions)).To(BeEquivalentTo(1))
			g.Expect(alp.Status.Conditions[0].Type).To(BeEquivalentTo(string(gwv1alpha2.PolicyConditionAccepted)))
			g.Expect(alp.Status.Conditions[0].Status).To(BeEquivalentTo(metav1.ConditionFalse))
			g.Expect(alp.Status.Conditions[0].ObservedGeneration).To(BeEquivalentTo(expectedGeneration))
			g.Expect(alp.Status.Conditions[0].Reason).To(BeEquivalentTo(string(gwv1alpha2.PolicyReasonInvalid)))

			// VPC Lattice Service should still have previous Access Log Subscription
			listALSInput := &vpclattice.ListAccessLogSubscriptionsInput{
				ResourceIdentifier: latticeService.Arn,
			}
			listALSOutput, err := testFramework.LatticeClient.ListAccessLogSubscriptionsWithContext(ctx, listALSInput)
			g.Expect(err).To(BeNil())
			g.Expect(len(listALSOutput.Items)).To(BeEquivalentTo(1))
			g.Expect(listALSOutput.Items[0].ResourceId).To(BeEquivalentTo(latticeService.Id))
			g.Expect(*listALSOutput.Items[0].DestinationArn).To(BeEquivalentTo(bucketArn))

			// Same Access Log Subscription ARN should be in the Access Log Policy's annotations
			g.Expect(alp.Annotations[anv1alpha1.AccessLogSubscriptionAnnotationKey]).To(BeEquivalentTo(*listALSOutput.Items[0].Arn))
			g.Expect(alp.Annotations[anv1alpha1.AccessLogSubscriptionAnnotationKey]).To(BeEquivalentTo(currentAlsArn))
		}).Should(Succeed())

		// Create second Access Log Policy for original destination
		accessLogPolicy2 := &anv1alpha1.AccessLogPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      k8sResource2Name,
				Namespace: k8snamespace,
			},
			Spec: anv1alpha1.AccessLogPolicySpec{
				DestinationArn: aws.String(logGroupArn),
				TargetRef: &gwv1alpha2.PolicyTargetReference{
					Group:     gwv1beta1.GroupName,
					Kind:      "HTTPRoute",
					Name:      gwv1alpha2.ObjectName(httpRoute.Name),
					Namespace: (*gwv1alpha2.Namespace)(aws.String(k8snamespace)),
				},
			},
		}
		testFramework.ExpectCreated(ctx, accessLogPolicy2)

		Eventually(func(g Gomega) {
			// Policy status should be Accepted
			alpNamespacedName := types.NamespacedName{
				Name:      accessLogPolicy2.Name,
				Namespace: accessLogPolicy2.Namespace,
			}
			alp := &anv1alpha1.AccessLogPolicy{}
			err := testFramework.Client.Get(ctx, alpNamespacedName, alp)
			g.Expect(err).To(BeNil())
			g.Expect(len(alp.Status.Conditions)).To(BeEquivalentTo(1))
			g.Expect(alp.Status.Conditions[0].Type).To(BeEquivalentTo(string(gwv1alpha2.PolicyConditionAccepted)))
			g.Expect(alp.Status.Conditions[0].Status).To(BeEquivalentTo(metav1.ConditionTrue))
			g.Expect(alp.Status.Conditions[0].ObservedGeneration).To(BeEquivalentTo(1))
			g.Expect(alp.Status.Conditions[0].Reason).To(BeEquivalentTo(string(gwv1alpha2.PolicyReasonAccepted)))
		}).Should(Succeed())

		// Attempt to update first Access Log Policy to use the original destination
		alp = &anv1alpha1.AccessLogPolicy{}
		err = testFramework.Client.Get(ctx, alpNamespacedName, alp)
		Expect(err).To(BeNil())
		alp.Spec = anv1alpha1.AccessLogPolicySpec{
			DestinationArn: aws.String(logGroupArn),
			TargetRef: &gwv1alpha2.PolicyTargetReference{
				Group:     gwv1beta1.GroupName,
				Kind:      "HTTPRoute",
				Name:      gwv1alpha2.ObjectName(httpRoute.Name),
				Namespace: (*gwv1alpha2.Namespace)(aws.String(k8snamespace)),
			},
		}
		testFramework.ExpectUpdated(ctx, alp)
		expectedGeneration = expectedGeneration + 1

		Eventually(func(g Gomega) {
			// Policy status should be Conflicted
			alp := &anv1alpha1.AccessLogPolicy{}
			err := testFramework.Client.Get(ctx, alpNamespacedName, alp)
			g.Expect(err).To(BeNil())
			g.Expect(len(alp.Status.Conditions)).To(BeEquivalentTo(1))
			g.Expect(alp.Status.Conditions[0].Type).To(BeEquivalentTo(string(gwv1alpha2.PolicyConditionAccepted)))
			g.Expect(alp.Status.Conditions[0].Status).To(BeEquivalentTo(metav1.ConditionFalse))
			g.Expect(alp.Status.Conditions[0].ObservedGeneration).To(BeEquivalentTo(expectedGeneration))
			g.Expect(alp.Status.Conditions[0].Reason).To(BeEquivalentTo(string(gwv1alpha2.PolicyReasonConflicted)))

			// VPC Lattice Service should now have the old and new Access Log Subscriptions
			listALSInput := &vpclattice.ListAccessLogSubscriptionsInput{
				ResourceIdentifier: latticeService.Arn,
			}
			listALSOutput, err := testFramework.LatticeClient.ListAccessLogSubscriptionsWithContext(ctx, listALSInput)
			g.Expect(err).To(BeNil())
			g.Expect(len(listALSOutput.Items)).To(BeEquivalentTo(2))

			// Same Access Log Subscription ARN should be in the first Access Log Policy's annotations
			g.Expect(alp.Annotations[anv1alpha1.AccessLogSubscriptionAnnotationKey]).To(BeEquivalentTo(currentAlsArn))
		}).Should(Succeed())
	})

	It("deletion removes the Access Log Subscription for the corresponding Service Network when the targetRef's Kind is Gateway", func() {
		accessLogPolicy := &anv1alpha1.AccessLogPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      k8sResourceName,
				Namespace: k8snamespace,
			},
			Spec: anv1alpha1.AccessLogPolicySpec{
				DestinationArn: aws.String(bucketArn),
				TargetRef: &gwv1alpha2.PolicyTargetReference{
					Group:     gwv1beta1.GroupName,
					Kind:      "Gateway",
					Name:      gwv1alpha2.ObjectName(testGateway.Name),
					Namespace: (*gwv1alpha2.Namespace)(aws.String(k8snamespace)),
				},
			},
		}
		testFramework.ExpectCreated(ctx, accessLogPolicy)

		Eventually(func(g Gomega) {
			// Service Network should have an Access Log Subscription
			output, err := testFramework.LatticeClient.ListAccessLogSubscriptions(&vpclattice.ListAccessLogSubscriptionsInput{
				ResourceIdentifier: testServiceNetwork.Arn,
			})
			g.Expect(err).To(BeNil())
			g.Expect(len(output.Items)).To(BeEquivalentTo(1))
		}).Should(Succeed())

		testFramework.ExpectDeleted(ctx, accessLogPolicy)

		// Wait a moment for eventual consistency
		time.Sleep(1 * time.Second)

		Eventually(func(g Gomega) {
			// Service Network should no longer have an Access Log Subscription
			output, err := testFramework.LatticeClient.ListAccessLogSubscriptions(&vpclattice.ListAccessLogSubscriptionsInput{
				ResourceIdentifier: testServiceNetwork.Arn,
			})
			g.Expect(err).To(BeNil())
			g.Expect(len(output.Items)).To(BeEquivalentTo(0))
		}).Should(Succeed())
	})

	It("deletion removes the Access Log Subscription for the corresponding VPC Lattice Service when the targetRef's Kind is HTTPRoute", func() {
		accessLogPolicy := &anv1alpha1.AccessLogPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      k8sResourceName,
				Namespace: k8snamespace,
			},
			Spec: anv1alpha1.AccessLogPolicySpec{
				DestinationArn: aws.String(bucketArn),
				TargetRef: &gwv1alpha2.PolicyTargetReference{
					Group:     gwv1beta1.GroupName,
					Kind:      "HTTPRoute",
					Name:      gwv1alpha2.ObjectName(httpRoute.Name),
					Namespace: (*gwv1alpha2.Namespace)(aws.String(k8snamespace)),
				},
			},
		}
		testFramework.ExpectCreated(ctx, accessLogPolicy)

		latticeService := testFramework.GetVpcLatticeService(ctx, core.NewHTTPRoute(*httpRoute))

		Eventually(func(g Gomega) {
			// VPC Lattice Service should have an Access Log Subscription
			output, err := testFramework.LatticeClient.ListAccessLogSubscriptions(&vpclattice.ListAccessLogSubscriptionsInput{
				ResourceIdentifier: latticeService.Arn,
			})
			g.Expect(err).To(BeNil())
			g.Expect(len(output.Items)).To(BeEquivalentTo(1))
		}).Should(Succeed())

		testFramework.ExpectDeleted(ctx, accessLogPolicy)

		// Wait a moment for eventual consistency
		time.Sleep(1 * time.Second)

		Eventually(func(g Gomega) {
			// VPC Lattice Service should no longer have an Access Log Subscription
			output, err := testFramework.LatticeClient.ListAccessLogSubscriptions(&vpclattice.ListAccessLogSubscriptionsInput{
				ResourceIdentifier: latticeService.Arn,
			})
			g.Expect(err).To(BeNil())
			g.Expect(len(output.Items)).To(BeEquivalentTo(0))
		}).Should(Succeed())
	})

	It("deletion removes the Access Log Subscription for the corresponding VPC Lattice Service when the targetRef's Kind is GRPCRoute", func() {
		accessLogPolicy := &anv1alpha1.AccessLogPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      k8sResourceName,
				Namespace: k8snamespace,
			},
			Spec: anv1alpha1.AccessLogPolicySpec{
				DestinationArn: aws.String(bucketArn),
				TargetRef: &gwv1alpha2.PolicyTargetReference{
					Group:     gwv1beta1.GroupName,
					Kind:      "GRPCRoute",
					Name:      gwv1alpha2.ObjectName(grpcRoute.Name),
					Namespace: (*gwv1alpha2.Namespace)(aws.String(k8snamespace)),
				},
			},
		}
		testFramework.ExpectCreated(ctx, accessLogPolicy)

		latticeService := testFramework.GetVpcLatticeService(ctx, core.NewGRPCRoute(*grpcRoute))

		Eventually(func(g Gomega) {
			// VPC Lattice Service should have an Access Log Subscription
			output, err := testFramework.LatticeClient.ListAccessLogSubscriptions(&vpclattice.ListAccessLogSubscriptionsInput{
				ResourceIdentifier: latticeService.Arn,
			})
			g.Expect(err).To(BeNil())
			g.Expect(len(output.Items)).To(BeEquivalentTo(1))
		}).Should(Succeed())

		testFramework.ExpectDeleted(ctx, accessLogPolicy)

		// Wait a moment for eventual consistency
		time.Sleep(1 * time.Second)

		Eventually(func(g Gomega) {
			// VPC Lattice Service should no longer have an Access Log Subscription
			output, err := testFramework.LatticeClient.ListAccessLogSubscriptions(&vpclattice.ListAccessLogSubscriptionsInput{
				ResourceIdentifier: latticeService.Arn,
			})
			g.Expect(err).To(BeNil())
			g.Expect(len(output.Items)).To(BeEquivalentTo(0))
		}).Should(Succeed())
	})

	AfterEach(func() {
		// Delete Access Log Policies in test namespace
		alps := &anv1alpha1.AccessLogPolicyList{}
		err := testFramework.Client.List(ctx, alps, client.InNamespace(k8snamespace))
		Expect(err).To(BeNil())
		for _, alp := range alps.Items {
			testFramework.ExpectDeletedThenNotFound(ctx, &alp)
		}
	})

	AfterAll(func() {
		// Delete Kubernetes Routes, Services, and Deployments
		testFramework.ExpectDeleted(ctx, httpRoute, grpcRoute)
		testFramework.SleepForRouteDeletion()
		testFramework.ExpectDeletedThenNotFound(ctx,
			httpRoute,
			grpcRoute,
			httpK8sService,
			grpcK8sService,
			httpDeployment,
			grpcDeployment,
		)

		// Delete S3 Bucket contents
		output, err := s3Client.ListObjectsV2WithContext(ctx, &s3.ListObjectsV2Input{
			Bucket: aws.String(bucketName),
		})
		Expect(err).To(BeNil())
		for _, object := range output.Contents {
			_, err := s3Client.DeleteObjectWithContext(ctx, &s3.DeleteObjectInput{
				Bucket: aws.String(bucketName),
				Key:    object.Key,
			})
			Expect(err).To(BeNil())
		}

		// Delete S3 Bucket
		_, err = s3Client.DeleteBucketWithContext(ctx, &s3.DeleteBucketInput{
			Bucket: aws.String(bucketName),
		})
		Expect(err).To(BeNil())

		// Delete CloudWatch Log Groups
		_, err = logsClient.DeleteLogGroupWithContext(ctx, &cloudwatchlogs.DeleteLogGroupInput{
			LogGroupName: aws.String(logGroupName),
		})
		Expect(err).To(BeNil())
		_, err = logsClient.DeleteLogGroupWithContext(ctx, &cloudwatchlogs.DeleteLogGroupInput{
			LogGroupName: aws.String(logGroup2Name),
		})
		Expect(err).To(BeNil())

		// Delete Firehose Delivery Stream
		Eventually(func() (string, error) {
			describeInput := &firehose.DescribeDeliveryStreamInput{
				DeliveryStreamName: aws.String(deliveryStreamName),
			}
			describeOutput, err := firehoseClient.DescribeDeliveryStreamWithContext(ctx, describeInput)
			if err != nil {
				return "", err
			}

			status := *describeOutput.DeliveryStreamDescription.DeliveryStreamStatus
			if status != firehose.DeliveryStreamStatusActive {
				return status, fmt.Errorf("expected status to be ACTIVE, got %s", status)
			}

			_, err = firehoseClient.DeleteDeliveryStreamWithContext(ctx, &firehose.DeleteDeliveryStreamInput{
				DeliveryStreamName: aws.String(deliveryStreamName),
			})
			return status, err
		}, 5*time.Minute, time.Minute).Should(Equal(firehose.DeliveryStreamStatusActive))

		// Detach managed policies from IAM Role
		policies, err := iamClient.ListAttachedRolePoliciesWithContext(ctx, &iam.ListAttachedRolePoliciesInput{
			RoleName: aws.String(deliveryStreamRoleName),
		})
		Expect(err).To(BeNil())
		for _, policy := range policies.AttachedPolicies {
			_, err := iamClient.DetachRolePolicyWithContext(ctx, &iam.DetachRolePolicyInput{
				RoleName:  aws.String(deliveryStreamRoleName),
				PolicyArn: policy.PolicyArn,
			})
			Expect(err).To(BeNil())
		}

		// Delete inline policies from IAM Role
		inlinePolicies, err := iamClient.ListRolePoliciesWithContext(ctx, &iam.ListRolePoliciesInput{
			RoleName: aws.String(deliveryStreamRoleName),
		})
		Expect(err).To(BeNil())
		for _, policyName := range inlinePolicies.PolicyNames {
			_, err := iamClient.DeleteRolePolicyWithContext(ctx, &iam.DeleteRolePolicyInput{
				RoleName:   aws.String(deliveryStreamRoleName),
				PolicyName: policyName,
			})
			Expect(err).To(BeNil())
		}

		// Delete IAM Role
		_, err = iamClient.DeleteRoleWithContext(ctx, &iam.DeleteRoleInput{
			RoleName: aws.String(deliveryStreamRoleName),
		})
		Expect(err).To(BeNil())
	})
})
