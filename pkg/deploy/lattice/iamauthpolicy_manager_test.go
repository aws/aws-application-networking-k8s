package lattice

import (
	"context"
	"errors"
	"testing"

	mocks_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	mocks "github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

const testPolicyDoc = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":"*","Action":"vpc-lattice-svcs:Invoke","Resource":"*"}]}`

// Test_policyDocsEqual covers the JSON-aware policy comparator. The comparator
// is the load-bearing piece of the diff-before-write optimization: a false
// positive (returning true when docs really differ) would make the controller
// silently skip a Put that the user wanted to happen, leaving the policy not
// applied. So the test explicitly covers identity, harmless variations
// (whitespace, key order), genuine differences, and malformed inputs.
func Test_policyDocsEqual(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want bool
	}{
		{
			name: "identical strings",
			a:    testPolicyDoc,
			b:    testPolicyDoc,
			want: true,
		},
		{
			name: "same content with different whitespace and indentation",
			a:    testPolicyDoc,
			b: `{
				"Version": "2012-10-17",
				"Statement": [
					{
						"Effect": "Allow",
						"Principal": "*",
						"Action": "vpc-lattice-svcs:Invoke",
						"Resource": "*"
					}
				]
			}`,
			want: true,
		},
		{
			name: "same content with different top-level key order",
			a:    `{"Version":"2012-10-17","Statement":[]}`,
			b:    `{"Statement":[],"Version":"2012-10-17"}`,
			want: true,
		},
		{
			name: "same content with different statement key order",
			a:    `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"vpc-lattice-svcs:Invoke","Resource":"*","Principal":"*"}]}`,
			b:    testPolicyDoc,
			want: true,
		},
		{
			name: "different effect",
			a:    `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":"*","Action":"vpc-lattice-svcs:Invoke","Resource":"*"}]}`,
			b:    `{"Version":"2012-10-17","Statement":[{"Effect":"Deny","Principal":"*","Action":"vpc-lattice-svcs:Invoke","Resource":"*"}]}`,
			want: false,
		},
		{
			name: "different number of statements",
			a:    testPolicyDoc,
			b:    `{"Version":"2012-10-17","Statement":[]}`,
			want: false,
		},
		{
			// Lists are intentionally compared as ordered. If Lattice ever
			// returns Action lists in a different order than the user wrote,
			// we'll do one extra rewrite — that's harmless and preferable to
			// the false-positive risk of unconditional list normalization.
			name: "list ordering treated as significant",
			a:    `{"Action":["a","b"]}`,
			b:    `{"Action":["b","a"]}`,
			want: false,
		},
		{
			name: "first arg empty",
			a:    "",
			b:    testPolicyDoc,
			want: false,
		},
		{
			name: "second arg empty",
			a:    testPolicyDoc,
			b:    "",
			want: false,
		},
		{
			name: "first arg malformed JSON",
			a:    "{not json",
			b:    testPolicyDoc,
			want: false,
		},
		{
			name: "second arg malformed JSON",
			a:    testPolicyDoc,
			b:    "{not json",
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, policyDocsEqual(tt.a, tt.b))
		})
	}
}

// Test_IAMAuthPolicyManager_Put_ServiceNetwork_InSync verifies the
// diff-before-write optimization for a Gateway-targeted (ServiceNetwork)
// policy: when Lattice already reports State=Active and the same policy doc,
// neither PutAuthPolicy nor UpdateServiceNetwork is called.
//
// The test uses gomock's strict expectations to prove the calls are absent —
// any unexpected call to a write API would fail the test.
func Test_IAMAuthPolicyManager_Put_ServiceNetwork_InSync(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()

	mockCloud := mocks_aws.NewMockCloud(c)
	mockLattice := mocks.NewMockLattice(c)
	mockCloud.EXPECT().Lattice().Return(mockLattice).AnyTimes()

	mockLattice.EXPECT().FindServiceNetwork(ctx, "test-gateway").Return(
		&mocks.ServiceNetworkInfo{
			SvcNetwork: vpclattice.ServiceNetworkSummary{Id: aws.String("sn-1234")},
		}, nil)
	mockLattice.EXPECT().GetAuthPolicyWithContext(ctx, gomock.Any()).Return(
		&vpclattice.GetAuthPolicyOutput{
			Policy: aws.String(testPolicyDoc),
			State:  aws.String(vpclattice.AuthPolicyStateActive),
		}, nil)
	// PutAuthPolicy and UpdateServiceNetwork must not be called.

	mgr := NewIAMAuthPolicyManager(mockCloud)
	status, err := mgr.Put(ctx, model.IAMAuthPolicy{
		Type:   model.ServiceNetworkType,
		Name:   "test-gateway",
		Policy: testPolicyDoc,
	})
	assert.NoError(t, err)
	assert.Equal(t, "sn-1234", status.ResourceId)
}

// Test_IAMAuthPolicyManager_Put_Service_InSync mirrors the ServiceNetwork
// test for an HTTPRoute/GRPCRoute-targeted (Service) policy.
func Test_IAMAuthPolicyManager_Put_Service_InSync(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()

	mockCloud := mocks_aws.NewMockCloud(c)
	mockLattice := mocks.NewMockLattice(c)
	mockCloud.EXPECT().Lattice().Return(mockLattice).AnyTimes()

	mockLattice.EXPECT().FindService(ctx, "test-service").Return(
		&vpclattice.ServiceSummary{Id: aws.String("svc-5678")}, nil)
	mockLattice.EXPECT().GetAuthPolicyWithContext(ctx, gomock.Any()).Return(
		&vpclattice.GetAuthPolicyOutput{
			Policy: aws.String(testPolicyDoc),
			State:  aws.String(vpclattice.AuthPolicyStateActive),
		}, nil)
	// PutAuthPolicy and UpdateService must not be called.

	mgr := NewIAMAuthPolicyManager(mockCloud)
	status, err := mgr.Put(ctx, model.IAMAuthPolicy{
		Type:   model.ServiceType,
		Name:   "test-service",
		Policy: testPolicyDoc,
	})
	assert.NoError(t, err)
	assert.Equal(t, "svc-5678", status.ResourceId)
}

// Test_IAMAuthPolicyManager_Put_Drifted_PolicyDoc covers the case where the
// resource has AuthType=AWS_IAM (State=Active) but the attached policy
// document differs from the desired one. The rewrite path must still fire.
func Test_IAMAuthPolicyManager_Put_Drifted_PolicyDoc(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()

	mockCloud := mocks_aws.NewMockCloud(c)
	mockLattice := mocks.NewMockLattice(c)
	mockCloud.EXPECT().Lattice().Return(mockLattice).AnyTimes()

	mockLattice.EXPECT().FindServiceNetwork(ctx, "test-gateway").Return(
		&mocks.ServiceNetworkInfo{
			SvcNetwork: vpclattice.ServiceNetworkSummary{Id: aws.String("sn-1234")},
		}, nil)
	mockLattice.EXPECT().GetAuthPolicyWithContext(ctx, gomock.Any()).Return(
		&vpclattice.GetAuthPolicyOutput{
			Policy: aws.String(`{"Version":"2012-10-17","Statement":[]}`), // different from desired
			State:  aws.String(vpclattice.AuthPolicyStateActive),
		}, nil)
	mockLattice.EXPECT().PutAuthPolicyWithContext(ctx, gomock.Any()).Return(
		&vpclattice.PutAuthPolicyOutput{}, nil)
	mockLattice.EXPECT().UpdateServiceNetworkWithContext(ctx, gomock.Any()).Return(
		&vpclattice.UpdateServiceNetworkOutput{}, nil)

	mgr := NewIAMAuthPolicyManager(mockCloud)
	status, err := mgr.Put(ctx, model.IAMAuthPolicy{
		Type:   model.ServiceNetworkType,
		Name:   "test-gateway",
		Policy: testPolicyDoc,
	})
	assert.NoError(t, err)
	assert.Equal(t, "sn-1234", status.ResourceId)
}

// Test_IAMAuthPolicyManager_Put_Drifted_AuthType covers the case where the
// policy document matches but the resource's AuthType is not AWS_IAM
// (State=Inactive). Both calls must still fire — we don't bother
// distinguishing partial drift, see putSn comment.
func Test_IAMAuthPolicyManager_Put_Drifted_AuthType(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()

	mockCloud := mocks_aws.NewMockCloud(c)
	mockLattice := mocks.NewMockLattice(c)
	mockCloud.EXPECT().Lattice().Return(mockLattice).AnyTimes()

	mockLattice.EXPECT().FindServiceNetwork(ctx, "test-gateway").Return(
		&mocks.ServiceNetworkInfo{
			SvcNetwork: vpclattice.ServiceNetworkSummary{Id: aws.String("sn-1234")},
		}, nil)
	mockLattice.EXPECT().GetAuthPolicyWithContext(ctx, gomock.Any()).Return(
		&vpclattice.GetAuthPolicyOutput{
			Policy: aws.String(testPolicyDoc),
			State:  aws.String(vpclattice.AuthPolicyStateInactive),
		}, nil)
	mockLattice.EXPECT().PutAuthPolicyWithContext(ctx, gomock.Any()).Return(
		&vpclattice.PutAuthPolicyOutput{}, nil)
	mockLattice.EXPECT().UpdateServiceNetworkWithContext(ctx, gomock.Any()).Return(
		&vpclattice.UpdateServiceNetworkOutput{}, nil)

	mgr := NewIAMAuthPolicyManager(mockCloud)
	status, err := mgr.Put(ctx, model.IAMAuthPolicy{
		Type:   model.ServiceNetworkType,
		Name:   "test-gateway",
		Policy: testPolicyDoc,
	})
	assert.NoError(t, err)
	assert.Equal(t, "sn-1234", status.ResourceId)
}

// Test_IAMAuthPolicyManager_Put_GetAuthPolicy_NotFound covers the case where
// GetAuthPolicy returns ResourceNotFoundException (no policy ever attached).
// The manager treats this as "needs rewrite" rather than an error.
func Test_IAMAuthPolicyManager_Put_GetAuthPolicy_NotFound(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()

	mockCloud := mocks_aws.NewMockCloud(c)
	mockLattice := mocks.NewMockLattice(c)
	mockCloud.EXPECT().Lattice().Return(mockLattice).AnyTimes()

	mockLattice.EXPECT().FindServiceNetwork(ctx, "test-gateway").Return(
		&mocks.ServiceNetworkInfo{
			SvcNetwork: vpclattice.ServiceNetworkSummary{Id: aws.String("sn-1234")},
		}, nil)
	mockLattice.EXPECT().GetAuthPolicyWithContext(ctx, gomock.Any()).Return(
		nil, mocks.NewNotFoundError("auth policy", "sn-1234"))
	mockLattice.EXPECT().PutAuthPolicyWithContext(ctx, gomock.Any()).Return(
		&vpclattice.PutAuthPolicyOutput{}, nil)
	mockLattice.EXPECT().UpdateServiceNetworkWithContext(ctx, gomock.Any()).Return(
		&vpclattice.UpdateServiceNetworkOutput{}, nil)

	mgr := NewIAMAuthPolicyManager(mockCloud)
	status, err := mgr.Put(ctx, model.IAMAuthPolicy{
		Type:   model.ServiceNetworkType,
		Name:   "test-gateway",
		Policy: testPolicyDoc,
	})
	assert.NoError(t, err)
	assert.Equal(t, "sn-1234", status.ResourceId)
}

// Test_IAMAuthPolicyManager_Put_GetAuthPolicy_OtherError verifies that an
// unexpected error from GetAuthPolicy is bubbled up rather than swallowed.
// We must not skip the rewrite based on an inconclusive read.
func Test_IAMAuthPolicyManager_Put_GetAuthPolicy_OtherError(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	ctx := context.TODO()

	mockCloud := mocks_aws.NewMockCloud(c)
	mockLattice := mocks.NewMockLattice(c)
	mockCloud.EXPECT().Lattice().Return(mockLattice).AnyTimes()

	mockLattice.EXPECT().FindServiceNetwork(ctx, "test-gateway").Return(
		&mocks.ServiceNetworkInfo{
			SvcNetwork: vpclattice.ServiceNetworkSummary{Id: aws.String("sn-1234")},
		}, nil)
	getErr := errors.New("throttled")
	mockLattice.EXPECT().GetAuthPolicyWithContext(ctx, gomock.Any()).Return(nil, getErr)
	// PutAuthPolicy and UpdateServiceNetwork must not be called.

	mgr := NewIAMAuthPolicyManager(mockCloud)
	_, err := mgr.Put(ctx, model.IAMAuthPolicy{
		Type:   model.ServiceNetworkType,
		Name:   "test-gateway",
		Policy: testPolicyDoc,
	})
	assert.ErrorIs(t, err, getErr)
}
