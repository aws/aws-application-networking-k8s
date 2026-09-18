package lattice

import (
	"context"
	"errors"
	"testing"

	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	mocks "github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/vpclattice"
	"github.com/aws/aws-sdk-go-v2/service/vpclattice/types"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// helper to build an HTTPS service with a fixed cert for the update path.
func httpsServiceForCertTest(certArn string) *Service {
	return &Service{
		Spec: model.ServiceSpec{
			ServiceTagFields: model.ServiceTagFields{
				RouteName:      "https-svc",
				RouteNamespace: "ns",
				RouteType:      core.HttpRouteType,
			},
			ServiceNetworkNames: []string{"sn-keep"},
			CustomerCertARN:     certArn,
		},
	}
}

// wires up the common mock expectations for the Upsert update path (existing service,
// tags owned & unchanged, associations unchanged). GetService is wired separately per test.
func setupUpdatePathMocks(t *testing.T, mockLattice *mocks.MockLattice, mockTagging *mocks.MockTagging, cl pkg_aws.Cloud, svc *Service, times int) {
	t.Helper()

	mockLattice.EXPECT().
		FindService(gomock.Any(), gomock.Any()).
		Return(&types.ServiceSummary{
			Arn:  aws.String("svc-arn"),
			Id:   aws.String("svc-id"),
			Name: aws.String(svc.LatticeServiceName()),
		}, nil).
		Times(times)

	mockLattice.EXPECT().ListTagsForResource(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *vpclattice.ListTagsForResourceInput, _ ...interface{}) (*vpclattice.ListTagsForResourceOutput, error) {
			return &vpclattice.ListTagsForResourceOutput{
				Tags: cl.DefaultTagsMergedWith(svc.Spec.ToTags()),
			}, nil
		}).
		Times(times)

	// UpdateTags is called for the service and (because the association is in the
	// toUpdate set) for the association as well. Accept any ARN.
	mockTagging.EXPECT().UpdateTags(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	mockLattice.EXPECT().
		ListServiceNetworkServiceAssociationsAsList(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *ListSnSvcAssocsReq) ([]SnSvcAssocSummary, error) {
			return []SnSvcAssocSummary{
				{
					Arn:                aws.String("assoc-arn"),
					Id:                 aws.String("assoc-id"),
					ServiceNetworkName: aws.String("sn-keep"),
					Status:             types.ServiceNetworkServiceAssociationStatusActive,
				},
			}, nil
		}).
		Times(times)
}

// Proves the fix: when the certificate already matches what is configured on the
// Lattice service, repeated reconciles do NOT issue redundant UpdateService calls.
func Test_ServiceManager_UpdateService_SkippedWhenCertUnchanged(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()

	mockLattice := mocks.NewMockLattice(c)
	mockTagging := mocks.NewMockTagging(c)
	cfg := pkg_aws.CloudConfig{VpcId: "vpc-id", AccountId: "account-id"}
	cl := pkg_aws.NewDefaultCloudWithTagging(mockLattice, mockTagging, cfg)
	ctx := context.Background()
	m := NewServiceManager(gwlog.FallbackLogger, cl)

	const reconciles = 2
	const certArn = "arn:aws:acm:us-east-1:111122223333:certificate/abc"

	svc := httpsServiceForCertTest(certArn)
	setupUpdatePathMocks(t, mockLattice, mockTagging, cl, svc, reconciles)

	// GetService reports the SAME cert already configured => no update needed.
	mockLattice.EXPECT().
		GetService(gomock.Any(), gomock.Any()).
		Return(&vpclattice.GetServiceOutput{
			Id:             aws.String("svc-id"),
			CertificateArn: aws.String(certArn),
		}, nil).
		Times(reconciles)

	// THE KEY ASSERTION: UpdateService must never be called when the cert is unchanged.
	updateServiceCalls := 0
	mockLattice.EXPECT().
		UpdateService(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *vpclattice.UpdateServiceInput, _ ...interface{}) (*vpclattice.UpdateServiceOutput, error) {
			updateServiceCalls++
			return &vpclattice.UpdateServiceOutput{}, nil
		}).
		AnyTimes()

	for i := 0; i < reconciles; i++ {
		_, err := m.Upsert(ctx, svc)
		assert.Nil(t, err)
	}

	assert.Equal(t, 0, updateServiceCalls,
		"expected UpdateService to be skipped when the certificate is unchanged")
}

// Proves UpdateService IS still issued when the desired cert differs from the current
// one (cert first set / rotated). Covers both the empty-current and changed-current cases.
func Test_ServiceManager_UpdateService_CalledWhenCertChanged(t *testing.T) {
	tests := []struct {
		name       string
		currentArn *string // cert currently configured on the Lattice service (nil = none)
	}{
		{name: "first set (no current cert)", currentArn: nil},
		{name: "cert rotated (different current cert)", currentArn: aws.String("arn:aws:acm:us-east-1:111122223333:certificate/old")},
	}

	const desiredArn = "arn:aws:acm:us-east-1:111122223333:certificate/new"

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := gomock.NewController(t)
			defer c.Finish()

			mockLattice := mocks.NewMockLattice(c)
			mockTagging := mocks.NewMockTagging(c)
			cfg := pkg_aws.CloudConfig{VpcId: "vpc-id", AccountId: "account-id"}
			cl := pkg_aws.NewDefaultCloudWithTagging(mockLattice, mockTagging, cfg)
			ctx := context.Background()
			m := NewServiceManager(gwlog.FallbackLogger, cl)

			svc := httpsServiceForCertTest(desiredArn)
			setupUpdatePathMocks(t, mockLattice, mockTagging, cl, svc, 1)

			mockLattice.EXPECT().
				GetService(gomock.Any(), gomock.Any()).
				Return(&vpclattice.GetServiceOutput{
					Id:             aws.String("svc-id"),
					CertificateArn: tt.currentArn,
				}, nil).
				Times(1)

			updateServiceCalls := 0
			mockLattice.EXPECT().
				UpdateService(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, req *vpclattice.UpdateServiceInput, _ ...interface{}) (*vpclattice.UpdateServiceOutput, error) {
					updateServiceCalls++
					assert.Equal(t, desiredArn, aws.ToString(req.CertificateArn))
					assert.Equal(t, "svc-id", aws.ToString(req.ServiceIdentifier))
					return &vpclattice.UpdateServiceOutput{}, nil
				}).
				Times(1)

			_, err := m.Upsert(ctx, svc)
			assert.Nil(t, err)
			assert.Equal(t, 1, updateServiceCalls,
				"expected UpdateService to be called exactly once when the certificate differs")
		})
	}
}

// Proves that a failure reading the current certificate (GetService) surfaces as an
// error and does NOT issue an UpdateService, so the reconcile is retried rather than
// silently skipping a potentially-needed certificate update.
func Test_ServiceManager_UpdateService_GetServiceErrorPropagates(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()

	mockLattice := mocks.NewMockLattice(c)
	mockTagging := mocks.NewMockTagging(c)
	cfg := pkg_aws.CloudConfig{VpcId: "vpc-id", AccountId: "account-id"}
	cl := pkg_aws.NewDefaultCloudWithTagging(mockLattice, mockTagging, cfg)
	ctx := context.Background()
	m := NewServiceManager(gwlog.FallbackLogger, cl)

	const certArn = "arn:aws:acm:us-east-1:111122223333:certificate/abc"
	svc := httpsServiceForCertTest(certArn)

	// The update path runs checkAndUpdateTags (ListTagsForResource) and the service
	// UpdateTags before the cert block. GetService then fails, so association handling
	// is never reached.
	mockLattice.EXPECT().
		FindService(gomock.Any(), gomock.Any()).
		Return(&types.ServiceSummary{
			Arn:  aws.String("svc-arn"),
			Id:   aws.String("svc-id"),
			Name: aws.String(svc.LatticeServiceName()),
		}, nil).
		Times(1)
	mockLattice.EXPECT().ListTagsForResource(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *vpclattice.ListTagsForResourceInput, _ ...interface{}) (*vpclattice.ListTagsForResourceOutput, error) {
			return &vpclattice.ListTagsForResourceOutput{Tags: cl.DefaultTagsMergedWith(svc.Spec.ToTags())}, nil
		}).
		Times(1)
	mockTagging.EXPECT().UpdateTags(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	mockLattice.EXPECT().
		GetService(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("GetService_failed")).
		Times(1)

	// UpdateService must NOT be called when we cannot read the current cert.
	mockLattice.EXPECT().UpdateService(gomock.Any(), gomock.Any()).Times(0)

	_, err := m.Upsert(ctx, svc)
	assert.NotNil(t, err)
}
