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
		expectedOutput   *lattice.AccessLogSubscription
		onlyCompareSpecs bool
		expectedError    error
	}{
		{
			description: "Policy on Gateway without namespace maps to ALS on Service Network with Gateway name",
			input: &anv1alpha1.AccessLogPolicy{
				ObjectMeta: apimachineryv1.ObjectMeta{
					Namespace: namespace,
					Name:      name,
				},
				Spec: anv1alpha1.AccessLogPolicySpec{
					DestinationArn: aws.String(s3DestinationArn),
					TargetRef: &gwv1alpha2.PolicyTargetReference{
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
			description: "Policy on Gateway with namespace maps to ALS on Service Network with Gateway name",
			input: &anv1alpha1.AccessLogPolicy{
				ObjectMeta: apimachineryv1.ObjectMeta{
					Namespace: namespace,
					Name:      name,
				},
				Spec: anv1alpha1.AccessLogPolicySpec{
					DestinationArn: aws.String(s3DestinationArn),
					TargetRef: &gwv1alpha2.PolicyTargetReference{
						Kind:      gatewayKind,
						Name:      name,
						Namespace: (*gwv1alpha2.Namespace)(aws.String(namespace)),
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
			description: "Policy on HTTPRoute without namespace maps to ALS on Service with HTTPRoute name + Policy's namespace",
			input: &anv1alpha1.AccessLogPolicy{
				ObjectMeta: apimachineryv1.ObjectMeta{
					Namespace: namespace,
					Name:      name,
				},
				Spec: anv1alpha1.AccessLogPolicySpec{
					DestinationArn: aws.String(s3DestinationArn),
					TargetRef: &gwv1alpha2.PolicyTargetReference{
						Kind: httpRouteKind,
						Name: name,
					},
				},
			},
			expectedOutput: &lattice.AccessLogSubscription{
				Spec: lattice.AccessLogSubscriptionSpec{
					SourceType:        lattice.ServiceSourceType,
					SourceName:        fmt.Sprintf("%s-%s", name, namespace),
					DestinationArn:    s3DestinationArn,
					ALPNamespacedName: expectedNamespacedName,
					EventType:         core.CreateEvent,
				},
			},
			onlyCompareSpecs: true,
			expectedError:    nil,
		},
		{
			description: "Policy on HTTPRoute with namespace maps to ALS on Service Network with HTTPRoute name + namespace",
			input: &anv1alpha1.AccessLogPolicy{
				ObjectMeta: apimachineryv1.ObjectMeta{
					Namespace: namespace,
					Name:      name,
				},
				Spec: anv1alpha1.AccessLogPolicySpec{
					DestinationArn: aws.String(s3DestinationArn),
					TargetRef: &gwv1alpha2.PolicyTargetReference{
						Kind:      httpRouteKind,
						Name:      name,
						Namespace: (*gwv1alpha2.Namespace)(aws.String(namespace)),
					},
				},
			},
			expectedOutput: &lattice.AccessLogSubscription{
				Spec: lattice.AccessLogSubscriptionSpec{
					SourceType:        lattice.ServiceSourceType,
					SourceName:        fmt.Sprintf("%s-%s", name, namespace),
					DestinationArn:    s3DestinationArn,
					ALPNamespacedName: expectedNamespacedName,
					EventType:         core.CreateEvent,
				},
			},
			onlyCompareSpecs: true,
			expectedError:    nil,
		},
		{
			description: "Policy on GRPCRoute without namespace maps to ALS on Service with GRPCRoute name + Policy's namespace",
			input: &anv1alpha1.AccessLogPolicy{
				ObjectMeta: apimachineryv1.ObjectMeta{
					Namespace: namespace,
					Name:      name,
				},
				Spec: anv1alpha1.AccessLogPolicySpec{
					DestinationArn: aws.String(s3DestinationArn),
					TargetRef: &gwv1alpha2.PolicyTargetReference{
						Kind: grpcRouteKind,
						Name: name,
					},
				},
			},
			expectedOutput: &lattice.AccessLogSubscription{
				Spec: lattice.AccessLogSubscriptionSpec{
					SourceType:        lattice.ServiceSourceType,
					SourceName:        fmt.Sprintf("%s-%s", name, namespace),
					DestinationArn:    s3DestinationArn,
					ALPNamespacedName: expectedNamespacedName,
					EventType:         core.CreateEvent,
				},
			},
			onlyCompareSpecs: true,
			expectedError:    nil,
		},
		{
			description: "Policy on GRPCRoute with namespace maps to ALS on Service Network with GRPCRoute name + namespace",
			input: &anv1alpha1.AccessLogPolicy{
				ObjectMeta: apimachineryv1.ObjectMeta{
					Namespace: namespace,
					Name:      name,
				},
				Spec: anv1alpha1.AccessLogPolicySpec{
					DestinationArn: aws.String(s3DestinationArn),
					TargetRef: &gwv1alpha2.PolicyTargetReference{
						Kind:      grpcRouteKind,
						Name:      name,
						Namespace: (*gwv1alpha2.Namespace)(aws.String(namespace)),
					},
				},
			},
			expectedOutput: &lattice.AccessLogSubscription{
				Spec: lattice.AccessLogSubscriptionSpec{
					SourceType:        lattice.ServiceSourceType,
					SourceName:        fmt.Sprintf("%s-%s", name, namespace),
					DestinationArn:    s3DestinationArn,
					ALPNamespacedName: expectedNamespacedName,
					EventType:         core.CreateEvent,
				},
			},
			onlyCompareSpecs: true,
			expectedError:    nil,
		},
		{
			description: "Policy on Gateway with deletion timestamp is marked as deleted",
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
					TargetRef: &gwv1alpha2.PolicyTargetReference{
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
			description: "Policy on Gateway with Access Log Subscription annotation present is marked as updated",
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
					TargetRef: &gwv1alpha2.PolicyTargetReference{
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
	}

	for _, tt := range tests {
		fmt.Printf("Testing: %s\n", tt.description)
		_, als, err := modelBuilder.Build(ctx, tt.input)
		if tt.onlyCompareSpecs {
			assert.Equal(t, tt.expectedOutput.Spec, als.Spec, tt.description)
		} else {
			assert.Equal(t, tt.expectedOutput, als, tt.description)
		}
		assert.Equal(t, tt.expectedError, err, tt.description)
	}
}
