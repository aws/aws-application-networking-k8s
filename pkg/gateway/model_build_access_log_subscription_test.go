package gateway

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	apimachineryv1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

const (
	s3DestinationArn = "arn:aws:s3:::test"
	gatewayKind      = "Gateway"
	httpRouteKind    = "HTTPRoute"
	grpcRouteKind    = "GRPCRoute"
	name             = "TestName"
	namespace        = "TestNamespace"
)

func Test_BuildAccessLogSubscription(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()
	scheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(scheme)
	client := testclient.NewClientBuilder().WithScheme(scheme).Build()
	modelBuilder := NewAccessLogSubscriptionModelBuilder(gwlog.FallbackLogger, client)
	expectedNamespacedName := types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}

	tests := []struct {
		description      string
		input            *anv1alpha1.AccessLogPolicy
		targetRefName    string
		expectedOutput   *lattice.AccessLogSubscription
		onlyCompareSpecs bool
		expectedError    error
	}{
		{
			description:   "Policy on Gateway uses passed targetRefName as ServiceNetwork SourceName",
			targetRefName: name,
			input: &anv1alpha1.AccessLogPolicy{
				ObjectMeta: apimachineryv1.ObjectMeta{
					Namespace: namespace,
					Name:      name,
				},
				Spec: anv1alpha1.AccessLogPolicySpec{
					DestinationArn: aws.String(s3DestinationArn),
					TargetRef: &gwv1alpha2.NamespacedPolicyTargetReference{
						Kind: gatewayKind,
						Name: name,
					},
				},
			},
			expectedOutput: &lattice.AccessLogSubscription{
				Spec: lattice.AccessLogSubscriptionSpec{
					SourceType:        lattice.ServiceNetworkSourceType,
					SourceName:        name,
					DestinationArn:    s3DestinationArn,
					ALPNamespacedName: expectedNamespacedName,
					EventType:         core.CreateEvent,
				},
			},
			onlyCompareSpecs: true,
			expectedError:    nil,
		},
		{
			description:   "Policy on HTTPRoute uses passed targetRefName as Service SourceName",
			targetRefName: name,
			input: &anv1alpha1.AccessLogPolicy{
				ObjectMeta: apimachineryv1.ObjectMeta{
					Namespace: namespace,
					Name:      name,
				},
				Spec: anv1alpha1.AccessLogPolicySpec{
					DestinationArn: aws.String(s3DestinationArn),
					TargetRef: &gwv1alpha2.NamespacedPolicyTargetReference{
						Kind: httpRouteKind,
						Name: name,
					},
				},
			},
			expectedOutput: &lattice.AccessLogSubscription{
				Spec: lattice.AccessLogSubscriptionSpec{
					SourceType:        lattice.ServiceSourceType,
					SourceName:        name,
					DestinationArn:    s3DestinationArn,
					ALPNamespacedName: expectedNamespacedName,
					EventType:         core.CreateEvent,
				},
			},
			onlyCompareSpecs: true,
			expectedError:    nil,
		},
		{
			description:   "Policy on GRPCRoute uses passed targetRefName as Service SourceName",
			targetRefName: name,
			input: &anv1alpha1.AccessLogPolicy{
				ObjectMeta: apimachineryv1.ObjectMeta{
					Namespace: namespace,
					Name:      name,
				},
				Spec: anv1alpha1.AccessLogPolicySpec{
					DestinationArn: aws.String(s3DestinationArn),
					TargetRef: &gwv1alpha2.NamespacedPolicyTargetReference{
						Kind: grpcRouteKind,
						Name: name,
					},
				},
			},
			expectedOutput: &lattice.AccessLogSubscription{
				Spec: lattice.AccessLogSubscriptionSpec{
					SourceType:        lattice.ServiceSourceType,
					SourceName:        name,
					DestinationArn:    s3DestinationArn,
					ALPNamespacedName: expectedNamespacedName,
					EventType:         core.CreateEvent,
				},
			},
			onlyCompareSpecs: true,
			expectedError:    nil,
		},
		{
			description:   "Policy on Gateway with deletion timestamp is marked as deleted",
			targetRefName: name,
			input: &anv1alpha1.AccessLogPolicy{
				ObjectMeta: apimachineryv1.ObjectMeta{
					Namespace:         namespace,
					Name:              name,
					DeletionTimestamp: &apimachineryv1.Time{},
					Annotations: map[string]string{
						anv1alpha1.AccessLogSubscriptionAnnotationKey: "arn:aws:vpc-lattice:us-west-2:123456789012:accesslogsubscription/als-12345678901234567",
					},
				},
				Spec: anv1alpha1.AccessLogPolicySpec{
					DestinationArn: aws.String(s3DestinationArn),
					TargetRef: &gwv1alpha2.NamespacedPolicyTargetReference{
						Kind: gatewayKind,
						Name: name,
					},
				},
			},
			expectedOutput: &lattice.AccessLogSubscription{
				Spec: lattice.AccessLogSubscriptionSpec{
					SourceType:        lattice.ServiceNetworkSourceType,
					SourceName:        name,
					DestinationArn:    s3DestinationArn,
					ALPNamespacedName: expectedNamespacedName,
					EventType:         core.DeleteEvent,
				},
			},
			onlyCompareSpecs: true,
			expectedError:    nil,
		},
		{
			description:   "Policy on Gateway with Access Log Subscription annotation present is marked as updated",
			targetRefName: name,
			input: &anv1alpha1.AccessLogPolicy{
				ObjectMeta: apimachineryv1.ObjectMeta{
					Namespace: namespace,
					Name:      name,
					Annotations: map[string]string{
						anv1alpha1.AccessLogSubscriptionAnnotationKey: "arn:aws:vpc-lattice:us-west-2:123456789012:accesslogsubscription/als-12345678901234567",
					},
				},
				Spec: anv1alpha1.AccessLogPolicySpec{
					DestinationArn: aws.String(s3DestinationArn),
					TargetRef: &gwv1alpha2.NamespacedPolicyTargetReference{
						Kind: gatewayKind,
						Name: name,
					},
				},
			},
			expectedOutput: &lattice.AccessLogSubscription{
				Spec: lattice.AccessLogSubscriptionSpec{
					SourceType:        lattice.ServiceNetworkSourceType,
					SourceName:        name,
					DestinationArn:    s3DestinationArn,
					ALPNamespacedName: expectedNamespacedName,
					EventType:         core.UpdateEvent,
				},
			},
			onlyCompareSpecs: true,
			expectedError:    nil,
		},
		{
			description:   "Delete event skips destinationArn validation",
			targetRefName: name,
			input: &anv1alpha1.AccessLogPolicy{
				ObjectMeta: apimachineryv1.ObjectMeta{
					Namespace:         namespace,
					Name:              name,
					DeletionTimestamp: &apimachineryv1.Time{},
					Annotations: map[string]string{
						anv1alpha1.AccessLogSubscriptionAnnotationKey: "arn:aws:vpc-lattice:us-west-2:123456789012:accesslogsubscription/als-12345678901234567",
					},
				},
				Spec: anv1alpha1.AccessLogPolicySpec{
					TargetRef: &gwv1alpha2.NamespacedPolicyTargetReference{
						Kind: gatewayKind,
						Name: name,
					},
				},
			},
			expectedOutput: &lattice.AccessLogSubscription{
				Spec: lattice.AccessLogSubscriptionSpec{
					SourceType:        lattice.ServiceNetworkSourceType,
					SourceName:        name,
					DestinationArn:    "",
					ALPNamespacedName: expectedNamespacedName,
					EventType:         core.DeleteEvent,
				},
			},
			onlyCompareSpecs: true,
			expectedError:    nil,
		},
	}

	for _, tt := range tests {
		fmt.Printf("Testing: %s\n", tt.description)
		_, als, err := modelBuilder.Build(ctx, tt.input, tt.targetRefName)
		if tt.onlyCompareSpecs {
			assert.Equal(t, tt.expectedOutput.Spec, als.Spec, tt.description)
		} else {
			assert.Equal(t, tt.expectedOutput, als, tt.description)
		}
		assert.Equal(t, tt.expectedError, err, tt.description)
	}
}

func Test_BuildAccessLogSubscription_WithAndWithoutAdditionalTagsAnnotation(t *testing.T) {
	ctx := context.TODO()
	scheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(scheme)
	client := testclient.NewClientBuilder().WithScheme(scheme).Build()
	modelBuilder := NewAccessLogSubscriptionModelBuilder(gwlog.FallbackLogger, client)

	tests := []struct {
		name                   string
		input                  *anv1alpha1.AccessLogPolicy
		expectedAdditionalTags map[string]*string
		description            string
	}{
		{
			name: "AccessLogPolicy with additional tags annotation",
			input: &anv1alpha1.AccessLogPolicy{
				ObjectMeta: apimachineryv1.ObjectMeta{
					Namespace: namespace,
					Name:      name,
					Annotations: map[string]string{
						"application-networking.k8s.aws/tags": "Environment=Prod,Project=AccessLogTest,Team=Platform",
					},
				},
				Spec: anv1alpha1.AccessLogPolicySpec{
					DestinationArn: aws.String(s3DestinationArn),
					TargetRef: &gwv1alpha2.NamespacedPolicyTargetReference{
						Kind: gatewayKind,
						Name: name,
					},
				},
			},
			expectedAdditionalTags: map[string]*string{
				"Environment": &[]string{"Prod"}[0],
				"Project":     &[]string{"AccessLogTest"}[0],
				"Team":        &[]string{"Platform"}[0],
			},
			description: "should set additional tags from AccessLogPolicy annotations in access log subscription spec",
		},
		{
			name: "AccessLogPolicy without additional tags annotation",
			input: &anv1alpha1.AccessLogPolicy{
				ObjectMeta: apimachineryv1.ObjectMeta{
					Namespace: namespace,
					Name:      name,
				},
				Spec: anv1alpha1.AccessLogPolicySpec{
					DestinationArn: aws.String(s3DestinationArn),
					TargetRef: &gwv1alpha2.NamespacedPolicyTargetReference{
						Kind: gatewayKind,
						Name: name,
					},
				},
			},
			expectedAdditionalTags: nil,
			description:            "should have nil additional tags when no annotation present in access log subscription spec",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, als, err := modelBuilder.Build(ctx, tt.input, name)
			assert.NoError(t, err, tt.description)

			assert.Equal(t, tt.expectedAdditionalTags, als.Spec.AdditionalTags, tt.description)
		})
	}
}
