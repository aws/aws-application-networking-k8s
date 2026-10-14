package lattice

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/vpclattice"
	"github.com/aws/aws-sdk-go-v2/service/vpclattice/types"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	mocks "github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	lattice_runtime "github.com/aws/aws-application-networking-k8s/pkg/runtime"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

const (
	dnsTestSnId        = "sn-0dns0000000000001"
	dnsTestSnName      = "dns-test"
	dnsTestLocalSnArn  = "arn:aws:vpc-lattice:region:account-id:servicenetwork/" + dnsTestSnId
	dnsTestSharedSnArn = "arn:aws:vpc-lattice:region:248189924968:servicenetwork/" + dnsTestSnId
	dnsTestSnvaId      = "snva-0dns000000000001"
	dnsTestSnvaArn     = "arn:aws:vpc-lattice:region:account-id:servicenetworkvpcassociation/" + dnsTestSnvaId
)

type dnsTestEnv struct {
	ctx         context.Context
	mockLattice *mocks.MockLattice
	mockTagging *mocks.MockTagging
	cloud       pkg_aws.Cloud
	snMgr       *defaultServiceNetworkManager
}

// newDnsTestEnv wires the mocks and stubs FindServiceNetwork for a service network with the given ARN.
func newDnsTestEnv(t *testing.T, snArn string) *dnsTestEnv {
	c := gomock.NewController(t)
	t.Cleanup(c.Finish)
	ctx := context.TODO()
	mockLattice := mocks.NewMockLattice(c)
	mockTagging := mocks.NewMockTagging(c)
	cloud := pkg_aws.NewDefaultCloudWithTagging(mockLattice, mockTagging, TestCloudConfig)

	mockLattice.EXPECT().FindServiceNetwork(ctx, dnsTestSnName).Return(&mocks.ServiceNetworkInfo{
		SvcNetwork: types.ServiceNetworkSummary{
			Arn:  aws.String(snArn),
			Id:   aws.String(dnsTestSnId),
			Name: aws.String(dnsTestSnName),
		},
	}, nil)

	return &dnsTestEnv{
		ctx:         ctx,
		mockLattice: mockLattice,
		mockTagging: mockTagging,
		cloud:       cloud,
		snMgr:       NewDefaultServiceNetworkManager(gwlog.FallbackLogger, cloud),
	}
}

func (e *dnsTestEnv) expectNoAssociation() {
	e.mockLattice.EXPECT().ListServiceNetworkVpcAssociationsAsList(e.ctx, gomock.Any()).
		Return([]types.ServiceNetworkVpcAssociationSummary{}, nil)
}

func (e *dnsTestEnv) expectActiveAssociation(snArn string, enabled *bool, opts *types.DnsOptions) {
	e.mockLattice.EXPECT().ListServiceNetworkVpcAssociationsAsList(e.ctx, gomock.Any()).
		Return([]types.ServiceNetworkVpcAssociationSummary{{
			Arn:               aws.String(dnsTestSnvaArn),
			Id:                aws.String(dnsTestSnvaId),
			ServiceNetworkArn: aws.String(snArn),
			ServiceNetworkId:  aws.String(dnsTestSnId),
			Status:            types.ServiceNetworkVpcAssociationStatusActive,
			VpcId:             &config.VpcID,
			PrivateDnsEnabled: enabled,
			DnsOptions:        opts,
		}}, nil)
}

// expectOwnedLocalAssociation stubs the calls the local, controller-owned path makes before comparing
// private DNS settings: ownership via tags, the authoritative Get, and the tag update.
func (e *dnsTestEnv) expectOwnedLocalAssociation(sgIds []string, enabled *bool, opts *types.DnsOptions) {
	e.mockLattice.EXPECT().ListTagsForResource(e.ctx, gomock.Any()).Return(&vpclattice.ListTagsForResourceOutput{
		Tags: e.cloud.DefaultTags(),
	}, nil)
	e.mockLattice.EXPECT().GetServiceNetworkVpcAssociation(e.ctx, gomock.Any()).Return(&vpclattice.GetServiceNetworkVpcAssociationOutput{
		Arn:               aws.String(dnsTestSnvaArn),
		Id:                aws.String(dnsTestSnvaId),
		Status:            types.ServiceNetworkVpcAssociationStatusActive,
		SecurityGroupIds:  sgIds,
		PrivateDnsEnabled: enabled,
		DnsOptions:        opts,
	}, nil)
	e.mockTagging.EXPECT().UpdateTags(e.ctx, dnsTestSnvaArn, gomock.Any(), nil).Return(nil)
}

func (e *dnsTestEnv) expectCreate(assertInput func(input *vpclattice.CreateServiceNetworkVpcAssociationInput)) {
	e.mockLattice.EXPECT().CreateServiceNetworkVpcAssociation(e.ctx, gomock.Any()).DoAndReturn(
		func(ctx context.Context, input *vpclattice.CreateServiceNetworkVpcAssociationInput, opts ...func(*vpclattice.Options)) (*vpclattice.CreateServiceNetworkVpcAssociationOutput, error) {
			assertInput(input)
			return &vpclattice.CreateServiceNetworkVpcAssociationOutput{
				Arn:    aws.String(dnsTestSnvaArn),
				Id:     aws.String(dnsTestSnvaId),
				Status: types.ServiceNetworkVpcAssociationStatusActive,
			}, nil
		})
}

func Test_UpsertVpcAssociation_Create_PrivateDnsUnmanaged_SendsNoDnsFields(t *testing.T) {
	e := newDnsTestEnv(t, dnsTestLocalSnArn)
	e.expectNoAssociation()
	sgIds := []string{"sg-123456789"}
	e.expectCreate(func(input *vpclattice.CreateServiceNetworkVpcAssociationInput) {
		assert.Nil(t, input.PrivateDnsEnabled, "unmanaged private DNS must not be sent")
		assert.Nil(t, input.DnsOptions, "unmanaged private DNS must not send DnsOptions")
		assert.Equal(t, sgIds, input.SecurityGroupIds)
		assert.Equal(t, e.cloud.DefaultTags(), input.Tags)
	})

	resp, err := e.snMgr.UpsertVpcAssociation(e.ctx, dnsTestSnName, VpcAssociationRequest{SecurityGroupIds: sgIds})

	assert.Nil(t, err)
	assert.Equal(t, dnsTestSnvaArn, resp.Arn)
	assert.Empty(t, resp.DnsMismatch)
	assert.False(t, resp.RamShared)
}

func Test_UpsertVpcAssociation_Create_PrivateDnsEnabled_WithDnsOptions(t *testing.T) {
	e := newDnsTestEnv(t, dnsTestLocalSnArn)
	e.expectNoAssociation()
	domains := []string{"*.internal.example.com", "db.example.com"}
	additionalTags := mocks.Tags{"Environment": "Prod"}
	e.expectCreate(func(input *vpclattice.CreateServiceNetworkVpcAssociationInput) {
		assert.True(t, aws.ToBool(input.PrivateDnsEnabled))
		if assert.NotNil(t, input.DnsOptions) {
			assert.Equal(t, types.PrivateDnsPreferenceVerifiedDomainsAndSpecifiedDomains, input.DnsOptions.PrivateDnsPreference)
			assert.Equal(t, domains, input.DnsOptions.PrivateDnsSpecifiedDomains)
		}
		assert.Equal(t, []string{"sg-123456789"}, input.SecurityGroupIds)
		assert.Equal(t, e.cloud.MergeTags(e.cloud.DefaultTags(), additionalTags), input.Tags)
	})

	resp, err := e.snMgr.UpsertVpcAssociation(e.ctx, dnsTestSnName, VpcAssociationRequest{
		SecurityGroupIds: []string{"sg-123456789"},
		AdditionalTags:   additionalTags,
		Dns: VpcAssociationDnsConfig{
			PrivateDnsEnabled:          aws.Bool(true),
			PrivateDnsPreference:       string(types.PrivateDnsPreferenceVerifiedDomainsAndSpecifiedDomains),
			PrivateDnsSpecifiedDomains: domains,
		},
	})

	assert.Nil(t, err)
	assert.Equal(t, dnsTestSnvaArn, resp.Arn)
	assert.Empty(t, resp.DnsMismatch)
}

func Test_UpsertVpcAssociation_Create_PrivateDnsEnabled_WithoutDnsOptions(t *testing.T) {
	e := newDnsTestEnv(t, dnsTestLocalSnArn)
	e.expectNoAssociation()
	e.expectCreate(func(input *vpclattice.CreateServiceNetworkVpcAssociationInput) {
		assert.True(t, aws.ToBool(input.PrivateDnsEnabled))
		assert.Nil(t, input.DnsOptions, "VPC Lattice applies its default preference when no options are set")
	})

	_, err := e.snMgr.UpsertVpcAssociation(e.ctx, dnsTestSnName, VpcAssociationRequest{
		Dns: VpcAssociationDnsConfig{PrivateDnsEnabled: aws.Bool(true)},
	})

	assert.Nil(t, err)
}

func Test_UpsertVpcAssociation_Create_PrivateDnsExplicitlyDisabled(t *testing.T) {
	e := newDnsTestEnv(t, dnsTestLocalSnArn)
	e.expectNoAssociation()
	e.expectCreate(func(input *vpclattice.CreateServiceNetworkVpcAssociationInput) {
		if assert.NotNil(t, input.PrivateDnsEnabled) {
			assert.False(t, *input.PrivateDnsEnabled)
		}
		assert.Nil(t, input.DnsOptions)
	})

	_, err := e.snMgr.UpsertVpcAssociation(e.ctx, dnsTestSnName, VpcAssociationRequest{
		Dns: VpcAssociationDnsConfig{PrivateDnsEnabled: aws.Bool(false)},
	})

	assert.Nil(t, err)
}

func Test_UpsertVpcAssociation_LocalExisting_PrivateDnsMismatch_ReportedWithoutUpdate(t *testing.T) {
	e := newDnsTestEnv(t, dnsTestLocalSnArn)
	sgIds := []string{"sg-123456789"}
	e.expectActiveAssociation(dnsTestLocalSnArn, nil, nil)
	e.expectOwnedLocalAssociation(sgIds, aws.Bool(false), &types.DnsOptions{})
	e.mockLattice.EXPECT().UpdateServiceNetworkVpcAssociation(e.ctx, gomock.Any()).Times(0)
	e.mockLattice.EXPECT().CreateServiceNetworkVpcAssociation(e.ctx, gomock.Any()).Times(0)
	e.mockLattice.EXPECT().DeleteServiceNetworkVpcAssociation(e.ctx, gomock.Any()).Times(0)

	resp, err := e.snMgr.UpsertVpcAssociation(e.ctx, dnsTestSnName, VpcAssociationRequest{
		SecurityGroupIds: sgIds,
		Dns:              VpcAssociationDnsConfig{PrivateDnsEnabled: aws.Bool(true)},
	})

	assert.Nil(t, err, "a private DNS mismatch is reported, not returned as an error")
	assert.Equal(t, dnsTestSnvaArn, resp.Arn)
	assert.False(t, resp.RamShared)
	assert.Equal(t, []VpcAssociationDnsMismatch{{Field: "privateDnsEnabled", Policy: "true", Actual: "false"}}, resp.DnsMismatch)
}

func Test_UpsertVpcAssociation_LocalExisting_PrivateDnsMismatch_DoesNotBlockSecurityGroupUpdate(t *testing.T) {
	e := newDnsTestEnv(t, dnsTestLocalSnArn)
	newSgIds := []string{"sg-new"}
	e.expectActiveAssociation(dnsTestLocalSnArn, nil, nil)
	e.expectOwnedLocalAssociation([]string{"sg-old"}, aws.Bool(false), nil)
	e.mockLattice.EXPECT().UpdateServiceNetworkVpcAssociation(e.ctx, gomock.Any()).DoAndReturn(
		func(ctx context.Context, input *vpclattice.UpdateServiceNetworkVpcAssociationInput, opts ...func(*vpclattice.Options)) (*vpclattice.UpdateServiceNetworkVpcAssociationOutput, error) {
			assert.Equal(t, newSgIds, input.SecurityGroupIds)
			return &vpclattice.UpdateServiceNetworkVpcAssociationOutput{
				Arn:              aws.String(dnsTestSnvaArn),
				SecurityGroupIds: newSgIds,
				Status:           types.ServiceNetworkVpcAssociationStatusActive,
			}, nil
		}).Times(1)

	resp, err := e.snMgr.UpsertVpcAssociation(e.ctx, dnsTestSnName, VpcAssociationRequest{
		SecurityGroupIds: newSgIds,
		Dns:              VpcAssociationDnsConfig{PrivateDnsEnabled: aws.Bool(true)},
	})

	assert.Nil(t, err)
	assert.Equal(t, dnsTestSnvaArn, resp.Arn)
	assert.Len(t, resp.DnsMismatch, 1)
}

func Test_UpsertVpcAssociation_LocalExisting_PrivateDnsUnmanaged_NoMismatch(t *testing.T) {
	e := newDnsTestEnv(t, dnsTestLocalSnArn)
	e.expectActiveAssociation(dnsTestLocalSnArn, aws.Bool(true), &types.DnsOptions{PrivateDnsPreference: types.PrivateDnsPreferenceAllDomains})
	e.expectOwnedLocalAssociation(nil, aws.Bool(true), &types.DnsOptions{PrivateDnsPreference: types.PrivateDnsPreferenceAllDomains})
	e.mockLattice.EXPECT().UpdateServiceNetworkVpcAssociation(e.ctx, gomock.Any()).Times(0)

	resp, err := e.snMgr.UpsertVpcAssociation(e.ctx, dnsTestSnName, VpcAssociationRequest{})

	assert.Nil(t, err)
	assert.Empty(t, resp.DnsMismatch, "a policy without private DNS fields must not report associations enabled by other means")
}

func Test_UpsertVpcAssociation_LocalExisting_PrivateDnsMatchesServerDefault_NoMismatch(t *testing.T) {
	e := newDnsTestEnv(t, dnsTestLocalSnArn)
	e.expectActiveAssociation(dnsTestLocalSnArn, aws.Bool(true), nil)
	// VPC Lattice reports the association as enabled without an explicit preference.
	e.expectOwnedLocalAssociation(nil, aws.Bool(true), nil)
	e.mockLattice.EXPECT().UpdateServiceNetworkVpcAssociation(e.ctx, gomock.Any()).Times(0)

	resp, err := e.snMgr.UpsertVpcAssociation(e.ctx, dnsTestSnName, VpcAssociationRequest{
		Dns: VpcAssociationDnsConfig{
			PrivateDnsEnabled:    aws.Bool(true),
			PrivateDnsPreference: string(types.PrivateDnsPreferenceVerifiedDomainsOnly),
		},
	})

	assert.Nil(t, err)
	assert.Empty(t, resp.DnsMismatch)
}

func Test_UpsertVpcAssociation_RAMSharedExisting_PrivateDnsMismatch_UsesGetOutput(t *testing.T) {
	e := newDnsTestEnv(t, dnsTestSharedSnArn)
	e.expectActiveAssociation(dnsTestSharedSnArn, aws.Bool(true), nil) // stale or partial summary
	e.mockLattice.EXPECT().GetServiceNetworkVpcAssociation(e.ctx, gomock.Any()).Return(&vpclattice.GetServiceNetworkVpcAssociationOutput{
		Arn:               aws.String(dnsTestSnvaArn),
		Id:                aws.String(dnsTestSnvaId),
		Status:            types.ServiceNetworkVpcAssociationStatusActive,
		PrivateDnsEnabled: aws.Bool(false),
	}, nil)
	// RAM-shared associations are never owned, tagged or updated by the controller.
	e.mockLattice.EXPECT().ListTagsForResource(e.ctx, gomock.Any()).Times(0)
	e.mockLattice.EXPECT().UpdateServiceNetworkVpcAssociation(e.ctx, gomock.Any()).Times(0)
	e.mockLattice.EXPECT().CreateServiceNetworkVpcAssociation(e.ctx, gomock.Any()).Times(0)

	resp, err := e.snMgr.UpsertVpcAssociation(e.ctx, dnsTestSnName, VpcAssociationRequest{
		SecurityGroupIds: []string{"sg-ignored-on-ram-shared"},
		Dns:              VpcAssociationDnsConfig{PrivateDnsEnabled: aws.Bool(true)},
	})

	assert.Nil(t, err)
	assert.Equal(t, dnsTestSnvaArn, resp.Arn)
	assert.True(t, resp.RamShared)
	assert.Equal(t, []VpcAssociationDnsMismatch{{Field: "privateDnsEnabled", Policy: "true", Actual: "false"}}, resp.DnsMismatch)
}

func Test_UpsertVpcAssociation_RAMSharedExisting_PrivateDnsUnmanaged_NoGet(t *testing.T) {
	e := newDnsTestEnv(t, dnsTestSharedSnArn)
	e.expectActiveAssociation(dnsTestSharedSnArn, aws.Bool(true), nil)
	e.mockLattice.EXPECT().GetServiceNetworkVpcAssociation(e.ctx, gomock.Any()).Times(0)
	e.mockLattice.EXPECT().UpdateServiceNetworkVpcAssociation(e.ctx, gomock.Any()).Times(0)

	resp, err := e.snMgr.UpsertVpcAssociation(e.ctx, dnsTestSnName, VpcAssociationRequest{})

	assert.Nil(t, err)
	assert.True(t, resp.RamShared)
	assert.Empty(t, resp.DnsMismatch)
}

func Test_UpsertVpcAssociation_RAMSharedExisting_GetFails_FallsBackToSummary(t *testing.T) {
	e := newDnsTestEnv(t, dnsTestSharedSnArn)
	e.expectActiveAssociation(dnsTestSharedSnArn, aws.Bool(true), &types.DnsOptions{PrivateDnsPreference: types.PrivateDnsPreferenceAllDomains})
	e.mockLattice.EXPECT().GetServiceNetworkVpcAssociation(e.ctx, gomock.Any()).Return(nil, errors.New("AccessDeniedException"))
	e.mockLattice.EXPECT().UpdateServiceNetworkVpcAssociation(e.ctx, gomock.Any()).Times(0)

	resp, err := e.snMgr.UpsertVpcAssociation(e.ctx, dnsTestSnName, VpcAssociationRequest{
		Dns: VpcAssociationDnsConfig{
			PrivateDnsEnabled:    aws.Bool(true),
			PrivateDnsPreference: string(types.PrivateDnsPreferenceVerifiedDomainsOnly),
		},
	})

	assert.Nil(t, err)
	assert.Equal(t, []VpcAssociationDnsMismatch{{
		Field:  "dnsOptions.privateDnsPreference",
		Policy: "VERIFIED_DOMAINS_ONLY",
		Actual: "ALL_DOMAINS",
	}}, resp.DnsMismatch)
}

func Test_dnsMismatch(t *testing.T) {
	enabled := func(pref string, domains ...string) VpcAssociationDnsConfig {
		return VpcAssociationDnsConfig{PrivateDnsEnabled: aws.Bool(true), PrivateDnsPreference: pref, PrivateDnsSpecifiedDomains: domains}
	}
	specified := string(types.PrivateDnsPreferenceSpecifiedDomainsOnly)
	verified := string(types.PrivateDnsPreferenceVerifiedDomainsOnly)
	all := string(types.PrivateDnsPreferenceAllDomains)

	tests := []struct {
		name          string
		desired       VpcAssociationDnsConfig
		actualEnabled *bool
		actual        *types.DnsOptions
		want          []VpcAssociationDnsMismatch
	}{
		{name: "unmanaged ignores an enabled association", desired: VpcAssociationDnsConfig{}, actualEnabled: aws.Bool(true), actual: &types.DnsOptions{PrivateDnsPreference: types.PrivateDnsPreferenceAllDomains}},
		{name: "disabled on both sides ignores leftover options", desired: VpcAssociationDnsConfig{PrivateDnsEnabled: aws.Bool(false)}, actualEnabled: nil, actual: &types.DnsOptions{PrivateDnsPreference: types.PrivateDnsPreferenceAllDomains}},
		{name: "desired enabled, actual disabled", desired: enabled(""), actualEnabled: aws.Bool(false), want: []VpcAssociationDnsMismatch{{Field: "privateDnsEnabled", Policy: "true", Actual: "false"}}},
		{name: "desired enabled, actual nil counts as disabled", desired: enabled(""), actualEnabled: nil, want: []VpcAssociationDnsMismatch{{Field: "privateDnsEnabled", Policy: "true", Actual: "false"}}},
		{name: "desired disabled, actual enabled", desired: VpcAssociationDnsConfig{PrivateDnsEnabled: aws.Bool(false)}, actualEnabled: aws.Bool(true), want: []VpcAssociationDnsMismatch{{Field: "privateDnsEnabled", Policy: "false", Actual: "true"}}},
		{name: "enabled without preference does not compare the preference", desired: enabled(""), actualEnabled: aws.Bool(true), actual: &types.DnsOptions{PrivateDnsPreference: types.PrivateDnsPreferenceAllDomains}},
		{name: "explicit default matches nil options", desired: enabled(verified), actualEnabled: aws.Bool(true), actual: nil},
		{name: "explicit default matches empty options", desired: enabled(verified), actualEnabled: aws.Bool(true), actual: &types.DnsOptions{}},
		{name: "preference differs", desired: enabled(all), actualEnabled: aws.Bool(true), actual: nil, want: []VpcAssociationDnsMismatch{{Field: "dnsOptions.privateDnsPreference", Policy: all, Actual: verified}}},
		{name: "domains equal ignoring order, case and trailing dot", desired: enabled(specified, "B.Example.com", "a.example.com."), actualEnabled: aws.Bool(true), actual: &types.DnsOptions{PrivateDnsPreference: types.PrivateDnsPreferenceSpecifiedDomainsOnly, PrivateDnsSpecifiedDomains: []string{"a.example.com", "b.example.com"}}},
		{name: "domains differ", desired: enabled(specified, "a.example.com", "b.example.com"), actualEnabled: aws.Bool(true), actual: &types.DnsOptions{PrivateDnsPreference: types.PrivateDnsPreferenceSpecifiedDomainsOnly, PrivateDnsSpecifiedDomains: []string{"a.example.com"}}, want: []VpcAssociationDnsMismatch{{Field: "dnsOptions.privateDnsSpecifiedDomains", Policy: "[a.example.com,b.example.com]", Actual: "[a.example.com]"}}},
		{name: "domains ignored for preferences that do not use them", desired: enabled(all), actualEnabled: aws.Bool(true), actual: &types.DnsOptions{PrivateDnsPreference: types.PrivateDnsPreferenceAllDomains, PrivateDnsSpecifiedDomains: []string{"leftover.example.com"}}},
		{name: "enabled mismatch wins over preference mismatch", desired: enabled(all), actualEnabled: aws.Bool(false), actual: &types.DnsOptions{PrivateDnsPreference: types.PrivateDnsPreferenceVerifiedDomainsOnly}, want: []VpcAssociationDnsMismatch{{Field: "privateDnsEnabled", Policy: "true", Actual: "false"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, dnsMismatch(tt.desired, tt.actualEnabled, tt.actual))
		})
	}
}

func Test_latticeDnsOptions(t *testing.T) {
	tests := []struct {
		name string
		cfg  VpcAssociationDnsConfig
		want *types.DnsOptions
	}{
		{name: "unmanaged", cfg: VpcAssociationDnsConfig{}},
		{name: "disabled with options", cfg: VpcAssociationDnsConfig{PrivateDnsEnabled: aws.Bool(false), PrivateDnsPreference: "ALL_DOMAINS"}},
		{name: "enabled without options", cfg: VpcAssociationDnsConfig{PrivateDnsEnabled: aws.Bool(true)}},
		{name: "enabled with preference", cfg: VpcAssociationDnsConfig{PrivateDnsEnabled: aws.Bool(true), PrivateDnsPreference: "ALL_DOMAINS"},
			want: &types.DnsOptions{PrivateDnsPreference: types.PrivateDnsPreferenceAllDomains}},
		{name: "enabled with preference and domains", cfg: VpcAssociationDnsConfig{PrivateDnsEnabled: aws.Bool(true), PrivateDnsPreference: "SPECIFIED_DOMAINS_ONLY", PrivateDnsSpecifiedDomains: []string{"*.example.com"}},
			want: &types.DnsOptions{PrivateDnsPreference: types.PrivateDnsPreferenceSpecifiedDomainsOnly, PrivateDnsSpecifiedDomains: []string{"*.example.com"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.cfg.latticeDnsOptions())
		})
	}
}

func Test_FormatDnsMismatch(t *testing.T) {
	assert.Equal(t, "", FormatDnsMismatch(nil))
	assert.Equal(t,
		"privateDnsEnabled (policy: true, association: false); dnsOptions.privateDnsPreference (policy: ALL_DOMAINS, association: VERIFIED_DOMAINS_ONLY)",
		FormatDnsMismatch([]VpcAssociationDnsMismatch{
			{Field: "privateDnsEnabled", Policy: "true", Actual: "false"},
			{Field: "dnsOptions.privateDnsPreference", Policy: "ALL_DOMAINS", Actual: "VERIFIED_DOMAINS_ONLY"},
		}))
}

func Test_unorderedStringSlicesEqual(t *testing.T) {
	assert.True(t, unorderedStringSlicesEqual(nil, []string{}))
	assert.True(t, unorderedStringSlicesEqual([]string{"b", "a"}, []string{"a", "b"}))
	assert.False(t, unorderedStringSlicesEqual([]string{"a"}, []string{"a", "b"}))
	assert.False(t, unorderedStringSlicesEqual([]string{"a"}, []string{"b"}))
}

func Test_UpsertVpcAssociation_Create_RAMShared_SetsRamShared(t *testing.T) {
	e := newDnsTestEnv(t, dnsTestSharedSnArn)
	e.expectNoAssociation()
	e.expectCreate(func(input *vpclattice.CreateServiceNetworkVpcAssociationInput) {
		assert.True(t, aws.ToBool(input.PrivateDnsEnabled))
	})

	resp, err := e.snMgr.UpsertVpcAssociation(e.ctx, dnsTestSnName, VpcAssociationRequest{
		Dns: VpcAssociationDnsConfig{PrivateDnsEnabled: aws.Bool(true)},
	})

	assert.Nil(t, err)
	assert.True(t, resp.RamShared)
}

func Test_UpsertVpcAssociation_LocalExisting_MismatchWithSecurityGroupUpdateInProgress_ReturnsRetryError(t *testing.T) {
	e := newDnsTestEnv(t, dnsTestLocalSnArn)
	e.expectActiveAssociation(dnsTestLocalSnArn, nil, nil)
	e.expectOwnedLocalAssociation([]string{"sg-old"}, aws.Bool(false), nil)
	e.mockLattice.EXPECT().UpdateServiceNetworkVpcAssociation(e.ctx, gomock.Any()).Return(&vpclattice.UpdateServiceNetworkVpcAssociationOutput{
		Arn:    aws.String(dnsTestSnvaArn),
		Status: types.ServiceNetworkVpcAssociationStatusUpdateInProgress,
	}, nil)

	resp, err := e.snMgr.UpsertVpcAssociation(e.ctx, dnsTestSnName, VpcAssociationRequest{
		SecurityGroupIds: []string{"sg-new"},
		Dns:              VpcAssociationDnsConfig{PrivateDnsEnabled: aws.Bool(true)},
	})

	var requeue *lattice_runtime.RequeueNeededAfter
	assert.True(t, errors.As(err, &requeue), "an in-progress update is retried; the mismatch is re-evaluated on the next pass")
	assert.Empty(t, resp.DnsMismatch)
}

func Test_DeleteVpcAssociation_DeleteFails_ReturnsCauseNotRetryError(t *testing.T) {
	e := newDnsTestEnv(t, dnsTestLocalSnArn)
	e.expectActiveAssociation(dnsTestLocalSnArn, nil, nil)
	e.mockLattice.EXPECT().ListTagsForResource(e.ctx, gomock.Any()).Return(&vpclattice.ListTagsForResourceOutput{
		Tags: e.cloud.DefaultTags(),
	}, nil)
	e.mockLattice.EXPECT().DeleteServiceNetworkVpcAssociation(e.ctx, gomock.Any()).
		Return(nil, &types.AccessDeniedException{Message: aws.String("not allowed")})

	err := e.snMgr.DeleteVpcAssociation(e.ctx, dnsTestSnName)

	var requeue *lattice_runtime.RequeueNeededAfter
	assert.False(t, errors.As(err, &requeue), "a failed delete must not look like an in-progress delete")
	var denied *types.AccessDeniedException
	assert.True(t, errors.As(err, &denied))
	assert.Contains(t, err.Error(), dnsTestSnvaArn)
}

func Test_DeleteVpcAssociation_DeleteAccepted_ReturnsRetryErrorUntilGone(t *testing.T) {
	e := newDnsTestEnv(t, dnsTestLocalSnArn)
	e.expectActiveAssociation(dnsTestLocalSnArn, nil, nil)
	e.mockLattice.EXPECT().ListTagsForResource(e.ctx, gomock.Any()).Return(&vpclattice.ListTagsForResourceOutput{
		Tags: e.cloud.DefaultTags(),
	}, nil)
	e.mockLattice.EXPECT().DeleteServiceNetworkVpcAssociation(e.ctx, gomock.Any()).
		Return(&vpclattice.DeleteServiceNetworkVpcAssociationOutput{Status: types.ServiceNetworkVpcAssociationStatusDeleteInProgress}, nil)

	err := e.snMgr.DeleteVpcAssociation(e.ctx, dnsTestSnName)

	var requeue *lattice_runtime.RequeueNeededAfter
	assert.True(t, errors.As(err, &requeue))
}
