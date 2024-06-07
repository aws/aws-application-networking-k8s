package lattice

import (
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go/service/vpclattice"

	"github.com/aws/aws-application-networking-k8s/pkg/aws"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/pkg/utils"
)

const (
	K8SClusterNameKey      = aws.TagBase + "ClusterName"
	K8SServiceNameKey      = aws.TagBase + "ServiceName"
	K8SServiceNamespaceKey = aws.TagBase + "ServiceNamespace"
	K8SRouteNameKey        = aws.TagBase + "RouteName"
	K8SRouteNamespaceKey   = aws.TagBase + "RouteNamespace"
	K8SSourceTypeKey       = aws.TagBase + "SourceTypeKey"
	K8SProtocolVersionKey  = aws.TagBase + "ProtocolVersion"

	// Service specific tags
	K8SRouteTypeKey = aws.TagBase + "RouteType"

	MaxNamespaceLength = 55
	MaxNameLength      = 55
	RandomSuffixLength = 10
)

type TargetGroup struct {
	core.ResourceMeta `json:"-"`
	Spec              TargetGroupSpec    `json:"spec"`
	Status            *TargetGroupStatus `json:"status,omitempty"`
	IsDeleted         bool               `json:"isdeleted"`
}

type TargetGroupSpec struct {
	VpcId             string                        `json:"vpcid"`
	Type              TargetGroupType               `json:"type"`
	Port              int32                         `json:"port"`
	Protocol          string                        `json:"protocol"`
	ProtocolVersion   string                        `json:"protocolversion"`
	IpAddressType     string                        `json:"ipaddresstype"`
	HealthCheckConfig *vpclattice.HealthCheckConfig `json:"healthcheckconfig"`
	TargetGroupTagFields
}
type TargetGroupTagFields struct {
	K8SClusterName      string        `json:"k8sclustername"`
	K8SSourceType       K8SSourceType `json:"k8ssourcetype"`
	K8SServiceName      string        `json:"k8sservicename"`
	K8SServiceNamespace string        `json:"k8sservicenamespace"`
	K8SRouteName        string        `json:"k8sroutename"`
	K8SRouteNamespace   string        `json:"k8sroutenamespace"`
	K8SProtocolVersion  string        `json:"k8sprotocolversion"`
}

type TargetGroupStatus struct {
	Name string `json:"name"`
	Arn  string `json:"arn"`
	Id   string `json:"id"`
}

type TargetGroupType string
type K8SSourceType string
type RouteType string

const (
	TargetGroupTypeIP TargetGroupType = "IP"

	SourceTypeSvcExport K8SSourceType = "ServiceExport"
	SourceTypeHTTPRoute K8SSourceType = "HTTPRoute"
	SourceTypeGRPCRoute K8SSourceType = "GRPCRoute"
	SourceTypeTLSRoute  K8SSourceType = "TLSRoute"
	SourceTypeInvalid   K8SSourceType = "INVALID"
)

func TGTagFieldsFromTags(tags map[string]*string) TargetGroupTagFields {
	return TargetGroupTagFields{
		K8SClusterName:      getMapValue(tags, K8SClusterNameKey),
		K8SSourceType:       GetParentRefType(getMapValue(tags, K8SSourceTypeKey)),
		K8SServiceName:      getMapValue(tags, K8SServiceNameKey),
		K8SServiceNamespace: getMapValue(tags, K8SServiceNamespaceKey),
		K8SRouteName:        getMapValue(tags, K8SRouteNameKey),
		K8SRouteNamespace:   getMapValue(tags, K8SRouteNamespaceKey),
		K8SProtocolVersion:  getMapValue(tags, K8SProtocolVersionKey),
	}
}

func TagsFromTGTagFields(tagFields TargetGroupTagFields) map[string]*string {
	st := string(tagFields.K8SSourceType)

	tags := map[string]*string{
		K8SClusterNameKey:      &tagFields.K8SClusterName,
		K8SServiceNameKey:      &tagFields.K8SServiceName,
		K8SServiceNamespaceKey: &tagFields.K8SServiceNamespace,
		K8SSourceTypeKey:       &st,
		K8SProtocolVersionKey:  &tagFields.K8SProtocolVersion,
	}
	if tagFields.K8SSourceType != SourceTypeSvcExport {
		tags[K8SRouteNameKey] = &tagFields.K8SRouteName
		tags[K8SRouteNamespaceKey] = &tagFields.K8SRouteNamespace
	}
	return tags
}

func getMapValue(m map[string]*string, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	return *v
}

func GetParentRefType(s string) K8SSourceType {
	if s == "" {
		return "" // empty is OK
	}

	switch s {
	case string(SourceTypeHTTPRoute):
		return SourceTypeHTTPRoute
	case string(SourceTypeGRPCRoute):
		return SourceTypeGRPCRoute
	case string(SourceTypeTLSRoute):
		return SourceTypeTLSRoute
	case string(SourceTypeSvcExport):
		return SourceTypeSvcExport
	default:
		return SourceTypeInvalid
	}
}

func TagFieldsMatch(spec TargetGroupSpec, tags TargetGroupTagFields) bool {
	return spec.TargetGroupTagFields == tags
}

func NewTargetGroup(stack core.Stack, spec TargetGroupSpec) (*TargetGroup, error) {
	if err := spec.Validate(); err != nil {
		return nil, err
	}

	id, err := core.IdFromHash(spec)
	if err != nil {
		return nil, err
	}

	tg := &TargetGroup{
		ResourceMeta: core.NewResourceMeta(stack, "AWS:VPCServiceNetwork::TargetGroup", id),
		Spec:         spec,
		Status:       nil,
	}

	stack.AddResource(tg)

	return tg, nil
}

func (t *TargetGroupTagFields) IsSourceTypeServiceExport() bool {
	return t.K8SSourceType == SourceTypeSvcExport
}

func (t *TargetGroupTagFields) IsSourceTypeRoute() bool {
	return t.K8SSourceType == SourceTypeHTTPRoute ||
		t.K8SSourceType == SourceTypeGRPCRoute ||
		t.K8SSourceType == SourceTypeTLSRoute
}

func (t *TargetGroupSpec) Validate() error {
	requiredFields := []string{t.K8SServiceName, t.K8SServiceNamespace,
		t.Protocol, t.VpcId, t.K8SClusterName, t.IpAddressType,
		string(t.K8SSourceType)}

	if t.Protocol != "TCP" {
		requiredFields = append(requiredFields, t.ProtocolVersion)
	}

	for _, s := range requiredFields {
		if s == "" {
			return errors.New("one or more required fields are missing")
		}
	}

	if t.IsSourceTypeRoute() {
		if t.K8SRouteName == "" || t.K8SRouteNamespace == "" {
			return errors.New("route name or namespace missing for route-based target group")
		}
	}

	return nil
}

func TgNamePrefix(spec TargetGroupSpec) string {
	truncSvcNamespace := utils.Truncate(spec.K8SServiceNamespace, MaxNamespaceLength)
	truncSvcName := utils.Truncate(spec.K8SServiceName, MaxNameLength)
	return fmt.Sprintf("k8s-%s-%s", truncSvcNamespace, truncSvcName)
}

func GenerateTgName(spec TargetGroupSpec) string {
	// tg max name length 128
	prefix := TgNamePrefix(spec)
	randomSuffix := utils.RandomAlphaString(RandomSuffixLength)
	return fmt.Sprintf("%s-%s", prefix, randomSuffix)
}
