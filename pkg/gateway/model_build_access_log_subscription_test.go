package gateway

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	testclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/gateway-api/apis/v1alpha2"

	anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
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
				Spec: anv1alpha1.AccessLogPolicySpec{
					DestinationArn: aws.String(s3DestinationArn),
					TargetRef: &v1alpha2.PolicyTargetReference{
						Kind: gatewayKind,
						Name: name,
					},
				},
			},
			expectedOutput: &lattice.AccessLogSubscription{
				Spec: lattice.AccessLogSubscriptionSpec{
					SourceType:     lattice.ServiceNetworkSourceType,
					SourceName:     name,
					DestinationArn: s3DestinationArn,
					IsDeleted:      false,
				},
			},
			onlyCompareSpecs: true,
			expectedError:    nil,
		},
		{
			description: "Policy on Gateway with namespace maps to ALS on Service Network with Gateway name",
			input: &anv1alpha1.AccessLogPolicy{
				Spec: anv1alpha1.AccessLogPolicySpec{
					DestinationArn: aws.String(s3DestinationArn),
					TargetRef: &v1alpha2.PolicyTargetReference{
						Kind:      gatewayKind,
						Name:      name,
						Namespace: (*v1alpha2.Namespace)(aws.String(namespace)),
					},
				},
			},
			expectedOutput: &lattice.AccessLogSubscription{
				Spec: lattice.AccessLogSubscriptionSpec{
					SourceType:     lattice.ServiceNetworkSourceType,
					SourceName:     name,
					DestinationArn: s3DestinationArn,
					IsDeleted:      false,
				},
			},
			onlyCompareSpecs: true,
			expectedError:    nil,
		},
		{
			description: "Policy on HTTPRoute without namespace maps to ALS on Service with HTTPRoute name + default namespace",
			input: &anv1alpha1.AccessLogPolicy{
				Spec: anv1alpha1.AccessLogPolicySpec{
					DestinationArn: aws.String(s3DestinationArn),
					TargetRef: &v1alpha2.PolicyTargetReference{
						Kind: httpRouteKind,
						Name: name,
					},
				},
			},
			expectedOutput: &lattice.AccessLogSubscription{
				Spec: lattice.AccessLogSubscriptionSpec{
					SourceType:     lattice.ServiceSourceType,
					SourceName:     fmt.Sprintf("%s-default", name),
					DestinationArn: s3DestinationArn,
					IsDeleted:      false,
				},
			},
			onlyCompareSpecs: true,
			expectedError:    nil,
		},
		{
			description: "Policy on HTTPRoute with namespace maps to ALS on Service Network with HTTPRoute name + namespace",
			input: &anv1alpha1.AccessLogPolicy{
				Spec: anv1alpha1.AccessLogPolicySpec{
					DestinationArn: aws.String(s3DestinationArn),
					TargetRef: &v1alpha2.PolicyTargetReference{
						Kind:      httpRouteKind,
						Name:      name,
						Namespace: (*v1alpha2.Namespace)(aws.String(namespace)),
					},
				},
			},
			expectedOutput: &lattice.AccessLogSubscription{
				Spec: lattice.AccessLogSubscriptionSpec{
					SourceType:     lattice.ServiceSourceType,
					SourceName:     fmt.Sprintf("%s-%s", name, namespace),
					DestinationArn: s3DestinationArn,
					IsDeleted:      false,
				},
			},
			onlyCompareSpecs: true,
			expectedError:    nil,
		},
		{
			description: "Policy on GRPCRoute without namespace maps to ALS on Service with GRPCRoute name + default namespace",
			input: &anv1alpha1.AccessLogPolicy{
				Spec: anv1alpha1.AccessLogPolicySpec{
					DestinationArn: aws.String(s3DestinationArn),
					TargetRef: &v1alpha2.PolicyTargetReference{
						Kind: grpcRouteKind,
						Name: name,
					},
				},
			},
			expectedOutput: &lattice.AccessLogSubscription{
				Spec: lattice.AccessLogSubscriptionSpec{
					SourceType:     lattice.ServiceSourceType,
					SourceName:     fmt.Sprintf("%s-default", name),
					DestinationArn: s3DestinationArn,
					IsDeleted:      false,
				},
			},
			onlyCompareSpecs: true,
			expectedError:    nil,
		},
		{
			description: "Policy on GRPCRoute with namespace maps to ALS on Service Network with GRPCRoute name + namespace",
			input: &anv1alpha1.AccessLogPolicy{
				Spec: anv1alpha1.AccessLogPolicySpec{
					DestinationArn: aws.String(s3DestinationArn),
					TargetRef: &v1alpha2.PolicyTargetReference{
						Kind:      grpcRouteKind,
						Name:      name,
						Namespace: (*v1alpha2.Namespace)(aws.String(namespace)),
					},
				},
			},
			expectedOutput: &lattice.AccessLogSubscription{
				Spec: lattice.AccessLogSubscriptionSpec{
					SourceType:     lattice.ServiceSourceType,
					SourceName:     fmt.Sprintf("%s-%s", name, namespace),
					DestinationArn: s3DestinationArn,
					IsDeleted:      false,
				},
			},
			onlyCompareSpecs: true,
			expectedError:    nil,
		},
		{
			description: "Policy on Gateway with deletion timestamp is marked as deleted",
			input: &anv1alpha1.AccessLogPolicy{
				ObjectMeta: v1.ObjectMeta{
					DeletionTimestamp: &v1.Time{},
				},
				Spec: anv1alpha1.AccessLogPolicySpec{
					DestinationArn: aws.String(s3DestinationArn),
					TargetRef: &v1alpha2.PolicyTargetReference{
						Kind: gatewayKind,
						Name: name,
					},
				},
			},
			expectedOutput: &lattice.AccessLogSubscription{
				Spec: lattice.AccessLogSubscriptionSpec{
					SourceType:     lattice.ServiceNetworkSourceType,
					SourceName:     name,
					DestinationArn: s3DestinationArn,
					IsDeleted:      true,
				},
			},
			onlyCompareSpecs: true,
			expectedError:    nil,
		},
		{
			description: "Policy missing destinationArn results in error",
			input: &anv1alpha1.AccessLogPolicy{
				Spec: anv1alpha1.AccessLogPolicySpec{
					TargetRef: &v1alpha2.PolicyTargetReference{
						Kind:      grpcRouteKind,
						Name:      name,
						Namespace: (*v1alpha2.Namespace)(aws.String(namespace)),
					},
				},
			},
			expectedOutput:   nil,
			onlyCompareSpecs: false,
			expectedError:    fmt.Errorf("access log policy's destinationArn cannot be nil"),
		},
	}

	for _, tt := range tests {
		_, als, err := modelBuilder.Build(ctx, tt.input)
		if tt.onlyCompareSpecs {
			assert.Equal(t, tt.expectedOutput.Spec, als.Spec, tt.description)
		} else {
			assert.Equal(t, tt.expectedOutput, als, tt.description)
		}
		assert.Equal(t, tt.expectedError, err, tt.description)
	}
}
