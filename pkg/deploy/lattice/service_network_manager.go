package lattice

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	lattice_runtime "github.com/aws/aws-application-networking-k8s/pkg/runtime"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/vpclattice"
	"github.com/aws/aws-sdk-go-v2/service/vpclattice/types"
	"github.com/aws/smithy-go"

	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/config"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
)

//go:generate mockgen -destination service_network_manager_mock.go -package lattice github.com/aws/aws-application-networking-k8s/pkg/deploy/lattice ServiceNetworkManager

type ServiceNetworkManager interface {
	// UpsertVpcAssociation creates or updates the association between the service network and the
	// cluster VPC. Private DNS differences are applied in place, except on associations the controller
	// did not create on a RAM-shared service network, where they are reported.
	UpsertVpcAssociation(ctx context.Context, snName string, req VpcAssociationRequest) (VpcAssociationResult, error)
	DeleteVpcAssociation(ctx context.Context, snName string) error

	CreateOrUpdate(ctx context.Context, serviceNetwork *model.ServiceNetwork) (model.ServiceNetworkStatus, error)

	// Upsert finds or creates a service network by name and ensures ownership.
	// Does not create a VPC association.
	Upsert(ctx context.Context, name string, additionalTags services.Tags) (model.ServiceNetworkStatus, error)

	// Delete deletes a service network by name if owned by this controller.
	Delete(ctx context.Context, snName string) error
}

// VpcAssociationDnsConfig is the private DNS configuration of a VPC association. A nil
// PrivateDnsEnabled means the policy does not manage private DNS: nothing is sent, nothing is compared.
type VpcAssociationDnsConfig struct {
	PrivateDnsEnabled          *bool
	PrivateDnsPreference       string // a types.PrivateDnsPreference value, "" for the VPC Lattice default
	PrivateDnsSpecifiedDomains []string
}

type VpcAssociationRequest struct {
	SecurityGroupIds []string
	AdditionalTags   services.Tags
	Dns              VpcAssociationDnsConfig
}

// VpcAssociationDnsMismatch is a private DNS setting whose live value differs from the policy.
type VpcAssociationDnsMismatch struct {
	Field  string // "privateDnsEnabled", "dnsOptions.privateDnsPreference" or "dnsOptions.privateDnsSpecifiedDomains"
	Policy string
	Actual string
}

type VpcAssociationResult struct {
	Arn       string
	RamShared bool // the service network belongs to another account
	// DnsMismatch is set when an association the controller did not create on a RAM-shared service
	// network has private DNS settings that differ from the request; it is left as is.
	DnsMismatch *VpcAssociationDnsMismatch
}

func NewDefaultServiceNetworkManager(log gwlog.Logger, cloud pkg_aws.Cloud) *defaultServiceNetworkManager {
	return &defaultServiceNetworkManager{
		log:   log,
		cloud: cloud,
	}
}

type defaultServiceNetworkManager struct {
	log   gwlog.Logger
	cloud pkg_aws.Cloud
}

func (m *defaultServiceNetworkManager) UpsertVpcAssociation(ctx context.Context, snName string, req VpcAssociationRequest) (VpcAssociationResult, error) {
	sn, err := m.cloud.Lattice().FindServiceNetwork(ctx, snName)
	if err != nil {
		return VpcAssociationResult{}, err
	}
	isLocal, err := m.isLocalServiceNetwork(sn.SvcNetwork.Arn)
	if err != nil {
		return VpcAssociationResult{}, err
	}
	snva, err := m.getActiveVpcAssociation(ctx, *sn.SvcNetwork.Id)
	if err != nil {
		return VpcAssociationResult{}, err
	}
	if snva == nil {
		return m.createVpcAssociation(ctx, sn.SvcNetwork.Id, req, !isLocal)
	}

	var mismatch *VpcAssociationDnsMismatch
	if isLocal {
		owned, err := m.cloud.TryOwn(ctx, *snva.Arn, false)
		if err != nil {
			return VpcAssociationResult{}, err
		}
		if !owned {
			return VpcAssociationResult{}, services.NewConflictError("snva", snName,
				fmt.Sprintf("Found existing vpc association not owned by controller: %s", *snva.Arn))
		}
		if err := m.updateServiceNetworkVpcAssociation(ctx, req, snva.Id); err != nil {
			return VpcAssociationResult{}, err
		}
	} else {
		// security groups and tags of associations on RAM-shared service networks are never modified
		m.log.Infof(ctx, "Using existing VPC association for RAM-shared service network %s: %s", snName, *snva.Arn)
		mismatch, err = m.reconcileRamSharedDns(ctx, req.Dns, snva)
		if err != nil {
			return VpcAssociationResult{}, err
		}
	}
	if mismatch != nil {
		m.log.Infof(ctx, "vpc association %s has private DNS settings that differ from the policy: %s", *snva.Arn, mismatch)
	}
	return VpcAssociationResult{Arn: *snva.Arn, RamShared: !isLocal, DnsMismatch: mismatch}, nil
}

func (m *defaultServiceNetworkManager) createVpcAssociation(ctx context.Context, snId *string, req VpcAssociationRequest, ramShared bool) (VpcAssociationResult, error) {
	resp, err := m.cloud.Lattice().CreateServiceNetworkVpcAssociation(ctx, &vpclattice.CreateServiceNetworkVpcAssociationInput{
		ServiceNetworkIdentifier: snId,
		VpcIdentifier:            &config.VpcID,
		SecurityGroupIds:         req.SecurityGroupIds,
		Tags:                     m.cloud.MergeTags(m.cloud.DefaultTags(), req.AdditionalTags),
		PrivateDnsEnabled:        req.Dns.PrivateDnsEnabled, // nil when the policy does not manage private DNS
		DnsOptions:               req.Dns.latticeDnsOptions(),
	})
	if err != nil {
		return VpcAssociationResult{}, err
	}
	res := VpcAssociationResult{Arn: *resp.Arn, RamShared: ramShared}
	if resp.Status != types.ServiceNetworkVpcAssociationStatusActive {
		return res, fmt.Errorf("%w, vpc association status in %s", lattice_runtime.NewRetryError(), resp.Status)
	}
	return res, nil
}

// reconcileRamSharedDns compares the association's private DNS settings with the policy, reading it
// (falling back to the List summary when the read fails, for example when another account created it).
// Differences are applied when the controller created the association and returned otherwise.
func (m *defaultServiceNetworkManager) reconcileRamSharedDns(ctx context.Context, desired VpcAssociationDnsConfig, snva *types.ServiceNetworkVpcAssociationSummary) (*VpcAssociationDnsMismatch, error) {
	if desired.PrivateDnsEnabled == nil {
		return nil, nil
	}
	actualEnabled, actualOptions := snva.PrivateDnsEnabled, snva.DnsOptions
	live, err := m.cloud.Lattice().GetServiceNetworkVpcAssociation(ctx, &vpclattice.GetServiceNetworkVpcAssociationInput{
		ServiceNetworkVpcAssociationIdentifier: snva.Id,
	})
	if err != nil {
		m.log.Debugf(ctx, "falling back to the association summary for private DNS comparison of %s: %s", aws.ToString(snva.Arn), err)
	} else {
		actualEnabled, actualOptions = live.PrivateDnsEnabled, live.DnsOptions
	}
	mismatch := dnsMismatch(desired, actualEnabled, actualOptions)
	if mismatch == nil {
		return nil, nil
	}
	managed, err := m.cloud.IsArnManaged(ctx, *snva.Arn)
	if err != nil && !isForeignAssociationError(err) {
		return nil, err
	}
	if !managed {
		return mismatch, nil
	}
	input := &vpclattice.UpdateServiceNetworkVpcAssociationInput{ServiceNetworkVpcAssociationIdentifier: snva.Id}
	desired.applyTo(input)
	return nil, m.sendVpcAssociationUpdate(ctx, input)
}

// isForeignAssociationError reports the errors returned when reading the tags of an association
// created by another account.
func isForeignAssociationError(err error) bool {
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	code := apiErr.ErrorCode()
	return code == "AccessDeniedException" || code == "NotFoundException" || code == "ResourceNotFoundException"
}

// isLocalServiceNetwork determines if a service network belongs to the current AWS account
func (m *defaultServiceNetworkManager) isLocalServiceNetwork(arnStr *string) (bool, error) {
	if arnStr == nil {
		return false, fmt.Errorf("service network ARN is nil")
	}

	parsedArn, err := arn.Parse(*arnStr)
	if err != nil {
		return false, fmt.Errorf("failed to parse service network ARN %s: %w", *arnStr, err)
	}

	// Compare with controller's account
	controllerAccount := m.cloud.Config().AccountId
	if controllerAccount == "" {
		// If controller account is not set, assume it's local (backward compatibility)
		m.log.Debugf(context.Background(), "Controller account ID not set, assuming service network %s is local", *arnStr)
		return true, nil
	}

	return parsedArn.AccountID == controllerAccount, nil
}

func (m *defaultServiceNetworkManager) DeleteVpcAssociation(ctx context.Context, snName string) error {
	sn, err := m.cloud.Lattice().FindServiceNetwork(ctx, snName)
	if err != nil {
		return err
	}

	snva, err := m.getActiveVpcAssociation(ctx, *sn.SvcNetwork.Id)
	if err != nil {
		return err
	}
	if snva != nil {
		// association is active
		m.log.Debugf(ctx, "Disassociating ServiceNetwork %s from VPC", snName)

		owned, err := m.cloud.IsArnManaged(ctx, *snva.Arn)
		if err != nil {
			// TODO check for types.AccessDeniedException or a new error type ErrorCodeNotFoundException
			// when the api no longer responds with a 404 NotFoundException instead of either of the above.
			// ErrorCodeNotFoundException currently not part of the golang sdk for the lattice api. This is a distinct
			// error from types.ResourceNotFoundException.

			// In a scenario that the vpc association is created by a foreign account,
			// the owner account's controller cannot read the tags of this ServiceNetworkVpcAssociation,
			// and AccessDeniedException is expected.
			m.log.Warnf(ctx, "skipping delete vpc association, association: %s, error: %s", *snva.Arn, err)

			return nil
		}
		if !owned {
			m.log.Infof(ctx, "Association %s for %s not owned by controller, skipping deletion", *snva.Arn, snName)
			return nil
		}

		deleteServiceNetworkVpcAssociationInput := vpclattice.DeleteServiceNetworkVpcAssociationInput{
			ServiceNetworkVpcAssociationIdentifier: snva.Id,
		}
		_, err = m.cloud.Lattice().DeleteServiceNetworkVpcAssociation(ctx, &deleteServiceNetworkVpcAssociationInput)
		if err != nil {
			// Surface the real cause instead of polling forever; a NotFound race is tolerated by the caller.
			return fmt.Errorf("failed to delete vpc association %s for service network %s: %w", *snva.Arn, snName, err)
		}
		// Deletion is asynchronous: poll until the association is gone.
		return lattice_runtime.NewRetryError()
	}
	return nil
}

func (m *defaultServiceNetworkManager) getActiveVpcAssociation(ctx context.Context, serviceNetworkId string) (*types.ServiceNetworkVpcAssociationSummary, error) {
	vpcLatticeSess := m.cloud.Lattice()
	associationStatusInput := vpclattice.ListServiceNetworkVpcAssociationsInput{
		ServiceNetworkIdentifier: &serviceNetworkId,
		VpcIdentifier:            &config.VpcID,
	}

	resp, err := vpcLatticeSess.ListServiceNetworkVpcAssociationsAsList(ctx, &associationStatusInput)
	if err != nil {
		return nil, err
	}
	if len(resp) == 0 {
		return nil, nil
	}

	// There can be at most one response for this
	snva := resp[0]
	if string(snva.Status) == string(types.ServiceNetworkVpcAssociationStatusActive) {
		return &snva, nil
	}
	m.log.Debugf(ctx, "snva %s status: %s",
		aws.ToString(snva.Arn), string(snva.Status))
	switch string(snva.Status) {
	case string(types.ServiceNetworkVpcAssociationStatusActive),
		string(types.ServiceNetworkVpcAssociationStatusDeleteFailed),
		string(types.ServiceNetworkVpcAssociationStatusUpdateFailed):
		// the resource exists
		return &snva, nil
	case string(types.ServiceNetworkVpcAssociationStatusCreateFailed):
		// consider it does not exist
		return nil, nil
	default:
		// a mutation is in progress, try later
		return nil, lattice_runtime.NewRetryError()
	}
}

// The controller does not manage service network anymore, just having to upsert SN and SNVA for default SN setup.
// This function does not care about the association status, the caller is not supposed to wait for it.
func (m *defaultServiceNetworkManager) CreateOrUpdate(ctx context.Context, serviceNetwork *model.ServiceNetwork) (model.ServiceNetworkStatus, error) {
	// check if exists
	foundSnSummary, err := m.cloud.Lattice().FindServiceNetwork(ctx, serviceNetwork.Spec.Name)
	if err != nil && !services.IsNotFoundError(err) {
		return model.ServiceNetworkStatus{ServiceNetworkARN: "", ServiceNetworkID: ""}, err
	}

	var serviceNetworkId string
	var serviceNetworkArn string
	vpcLatticeSess := m.cloud.Lattice()
	if foundSnSummary == nil {
		m.log.Debugf(ctx, "Creating ServiceNetwork %s and tagging it with vpcId %s",
			serviceNetwork.Spec.Name, config.VpcID)

		serviceNetworkInput := vpclattice.CreateServiceNetworkInput{
			Name: &serviceNetwork.Spec.Name,
			Tags: m.cloud.DefaultTags(),
		}
		resp, err := vpcLatticeSess.CreateServiceNetwork(ctx, &serviceNetworkInput)
		if err != nil {
			return model.ServiceNetworkStatus{}, err
		}

		serviceNetworkId = aws.ToString(resp.Id)
		serviceNetworkArn = aws.ToString(resp.Arn)
	} else {
		m.log.Debugf(ctx, "ServiceNetwork %s exists, checking its VPC association", serviceNetwork.Spec.Name)
		serviceNetworkId = aws.ToString(foundSnSummary.SvcNetwork.Id)
		serviceNetworkArn = aws.ToString(foundSnSummary.SvcNetwork.Arn)

		snva, err := m.getActiveVpcAssociation(ctx, serviceNetworkId)
		if err != nil {
			return model.ServiceNetworkStatus{}, err
		}
		if snva != nil {
			m.log.Debugf(ctx, "ServiceNetwork %s already has VPC association %s",
				serviceNetwork.Spec.Name, aws.ToString(snva.Arn))
			return model.ServiceNetworkStatus{ServiceNetworkARN: serviceNetworkArn, ServiceNetworkID: serviceNetworkId}, nil
		}
	}

	m.log.Debugf(ctx, "Creating association between ServiceNetwork %s and VPC %s", serviceNetworkId, config.VpcID)
	createServiceNetworkVpcAssociationInput := vpclattice.CreateServiceNetworkVpcAssociationInput{
		ServiceNetworkIdentifier: &serviceNetworkId,
		VpcIdentifier:            &config.VpcID,
		Tags:                     m.cloud.DefaultTags(),
	}
	_, err = vpcLatticeSess.CreateServiceNetworkVpcAssociation(ctx, &createServiceNetworkVpcAssociationInput)
	if err != nil {
		return model.ServiceNetworkStatus{}, err
	}
	return model.ServiceNetworkStatus{ServiceNetworkARN: serviceNetworkArn, ServiceNetworkID: serviceNetworkId}, nil
}

func (m *defaultServiceNetworkManager) Upsert(ctx context.Context, name string, additionalTags services.Tags) (model.ServiceNetworkStatus, error) {
	allTags := m.cloud.MergeTags(m.cloud.DefaultTags(), additionalTags)

	sn, err := m.cloud.Lattice().FindServiceNetwork(ctx, name)
	if err != nil && !services.IsNotFoundError(err) {
		return model.ServiceNetworkStatus{}, err
	}

	if sn == nil {
		// SN not found, create it
		resp, err := m.cloud.Lattice().CreateServiceNetwork(ctx, &vpclattice.CreateServiceNetworkInput{
			Name: &name,
			Tags: allTags,
		})
		if err != nil {
			return model.ServiceNetworkStatus{}, err
		}
		m.log.Infof(ctx, "Created ServiceNetwork %s (id: %s)", name, aws.ToString(resp.Id))
		return model.ServiceNetworkStatus{
			ServiceNetworkARN: aws.ToString(resp.Arn),
			ServiceNetworkID:  aws.ToString(resp.Id),
		}, nil
	}

	// SN exists, adopt it
	snArn := aws.ToString(sn.SvcNetwork.Arn)
	owned, err := m.cloud.TryOwn(ctx, snArn, false)
	if err != nil {
		return model.ServiceNetworkStatus{}, fmt.Errorf("failed to check ownership of ServiceNetwork %s: %w", name, err)
	}
	if !owned {
		return model.ServiceNetworkStatus{}, fmt.Errorf("ServiceNetwork %s is owned by another controller", name)
	}

	m.log.Infof(ctx, "Adopted existing ServiceNetwork %s (arn: %s)", name, snArn)

	if err := m.cloud.Tagging().UpdateTags(ctx, snArn, additionalTags, m.cloud.DefaultTags()); err != nil {
		return model.ServiceNetworkStatus{}, fmt.Errorf("failed to update tags for ServiceNetwork %s: %w", name, err)
	}

	return model.ServiceNetworkStatus{
		ServiceNetworkARN: snArn,
		ServiceNetworkID:  aws.ToString(sn.SvcNetwork.Id),
	}, nil
}

func (m *defaultServiceNetworkManager) Delete(ctx context.Context, snName string) error {
	sn, err := m.cloud.Lattice().FindServiceNetwork(ctx, snName)
	if err != nil {
		if services.IsNotFoundError(err) {
			return nil // already gone
		}
		return err
	}

	snArn := aws.ToString(sn.SvcNetwork.Arn)
	owned, err := m.cloud.IsArnManaged(ctx, snArn)
	if err != nil {
		return fmt.Errorf("failed to check ownership of ServiceNetwork %s: %w", snName, err)
	}
	if !owned {
		m.log.Infof(ctx, "ServiceNetwork %s not owned by controller, skipping deletion", snName)
		return nil
	}

	_, err = m.cloud.Lattice().DeleteServiceNetwork(ctx, &vpclattice.DeleteServiceNetworkInput{
		ServiceNetworkIdentifier: sn.SvcNetwork.Id,
	})
	if err != nil {
		return err
	}

	m.log.Infof(ctx, "Deleted ServiceNetwork %s", snName)
	return nil
}

// updateServiceNetworkVpcAssociation updates tags, security groups and private DNS settings in place.
func (m *defaultServiceNetworkManager) updateServiceNetworkVpcAssociation(ctx context.Context, req VpcAssociationRequest, existingSnvaId *string) error {
	snva, err := m.cloud.Lattice().GetServiceNetworkVpcAssociation(ctx, &vpclattice.GetServiceNetworkVpcAssociationInput{
		ServiceNetworkVpcAssociationIdentifier: existingSnvaId,
	})
	if err != nil {
		return err
	}

	err = m.cloud.Tagging().UpdateTags(ctx, aws.ToString(snva.Arn), req.AdditionalTags, nil)
	if err != nil {
		return fmt.Errorf("failed to update tags for service network vpc association %s: %w", aws.ToString(snva.Id), err)
	}

	sgChanged := !unorderedStringSlicesEqual(req.SecurityGroupIds, snva.SecurityGroupIds)
	dnsChanged := dnsMismatch(req.Dns, snva.PrivateDnsEnabled, snva.DnsOptions) != nil
	if !sgChanged && !dnsChanged {
		return nil
	}
	input := &vpclattice.UpdateServiceNetworkVpcAssociationInput{ServiceNetworkVpcAssociationIdentifier: existingSnvaId}
	if dnsChanged {
		req.Dns.applyTo(input)
	}
	// VPC Lattice rejects an empty list. It must not block a private DNS change: the rejection is
	// surfaced on the next reconcile, once private DNS matches.
	if sgChanged && (len(req.SecurityGroupIds) > 0 || !dnsChanged) {
		input.SecurityGroupIds = req.SecurityGroupIds
	}
	return m.sendVpcAssociationUpdate(ctx, input)
}

func (m *defaultServiceNetworkManager) sendVpcAssociationUpdate(ctx context.Context, input *vpclattice.UpdateServiceNetworkVpcAssociationInput) error {
	updateSnvaResp, err := m.cloud.Lattice().UpdateServiceNetworkVpcAssociation(ctx, input)
	if err != nil {
		return err
	}
	if string(updateSnvaResp.Status) == string(types.ServiceNetworkVpcAssociationStatusActive) {
		return nil
	}
	return fmt.Errorf("%w, update snva status: %s", lattice_runtime.NewRetryError(), string(updateSnvaResp.Status))
}

// applyTo sets the private DNS fields of an update, leaving security groups untouched.
func (d VpcAssociationDnsConfig) applyTo(input *vpclattice.UpdateServiceNetworkVpcAssociationInput) {
	input.PrivateDnsEnabled = d.PrivateDnsEnabled
	input.DnsOptions = d.latticeDnsOptions()
}

// latticeDnsOptions is nil unless private DNS is enabled and an option is set, so VPC Lattice applies its defaults.
func (d VpcAssociationDnsConfig) latticeDnsOptions() *types.DnsOptions {
	if !aws.ToBool(d.PrivateDnsEnabled) || (d.PrivateDnsPreference == "" && len(d.PrivateDnsSpecifiedDomains) == 0) {
		return nil
	}
	return &types.DnsOptions{
		PrivateDnsPreference:       types.PrivateDnsPreference(d.PrivateDnsPreference),
		PrivateDnsSpecifiedDomains: d.PrivateDnsSpecifiedDomains,
	}
}

func usesSpecifiedDomains(preference string) bool {
	return preference == string(types.PrivateDnsPreferenceVerifiedDomainsAndSpecifiedDomains) ||
		preference == string(types.PrivateDnsPreferenceSpecifiedDomainsOnly)
}

// dnsMismatch compares the policy's private DNS settings with an existing association. Only settings
// the policy sets are compared, VPC Lattice defaults are normalised (nil enabled == false, empty
// preference == VERIFIED_DOMAINS_ONLY) and only the most significant difference is returned, since
// an update sends all the settings anyway.
func dnsMismatch(desired VpcAssociationDnsConfig, actualEnabled *bool, actual *types.DnsOptions) *VpcAssociationDnsMismatch {
	if desired.PrivateDnsEnabled == nil {
		return nil
	}
	want, got := *desired.PrivateDnsEnabled, aws.ToBool(actualEnabled)
	if want != got {
		return &VpcAssociationDnsMismatch{Field: "privateDnsEnabled", Policy: strconv.FormatBool(want), Actual: strconv.FormatBool(got)}
	}
	if !want {
		return nil
	}
	actualPref, actualDomains := string(types.PrivateDnsPreferenceVerifiedDomainsOnly), []string(nil)
	if actual != nil {
		actualDomains = actual.PrivateDnsSpecifiedDomains
		if actual.PrivateDnsPreference != "" {
			actualPref = string(actual.PrivateDnsPreference)
		}
	}
	if desired.PrivateDnsPreference != "" && desired.PrivateDnsPreference != actualPref {
		return &VpcAssociationDnsMismatch{Field: "dnsOptions.privateDnsPreference", Policy: desired.PrivateDnsPreference, Actual: actualPref}
	}
	if usesSpecifiedDomains(desired.PrivateDnsPreference) && !slices.Equal(normalizedDomains(desired.PrivateDnsSpecifiedDomains), normalizedDomains(actualDomains)) {
		return &VpcAssociationDnsMismatch{Field: "dnsOptions.privateDnsSpecifiedDomains", Policy: formatDomains(desired.PrivateDnsSpecifiedDomains), Actual: formatDomains(actualDomains)}
	}
	return nil
}

// String renders the mismatch as "field (policy: X, association: Y)".
func (d *VpcAssociationDnsMismatch) String() string {
	return fmt.Sprintf("%s (policy: %s, association: %s)", d.Field, d.Policy, d.Actual)
}

// normalizedDomains lower-cases, trims trailing dots and sorts, so lists compare regardless of order or spelling.
func normalizedDomains(in []string) []string {
	out := make([]string, len(in))
	for i, d := range in {
		out[i] = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(d)), ".")
	}
	slices.Sort(out)
	return out
}

func formatDomains(d []string) string {
	return "[" + strings.Join(normalizedDomains(d), ",") + "]"
}

func unorderedStringSlicesEqual(a, b []string) bool {
	return slices.Equal(slices.Sorted(slices.Values(a)), slices.Sorted(slices.Values(b)))
}
