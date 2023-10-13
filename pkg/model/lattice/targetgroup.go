package lattice

import (
	"errors"
	"fmt"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/pkg/utils"
	"github.com/aws/aws-sdk-go/service/vpclattice"
	"math/rand"
	"reflect"
)

const (
	EKSClusterNameKey      = "EKSClusterName"
	K8SServiceNameKey      = "K8SServiceName"
	K8SServiceNamespaceKey = "K8SServiceNamespace"
	K8SRouteNameKey        = "K8SRouteName"
	K8SRouteNamespaceKey   = "K8SRouteNamespace"

	K8SParentRefTypeKey = "K8SParentRefTypeKey"

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
	EKSClusterName      string        `json:"eksclustername"`
	K8SParentRefType    ParentRefType `json:"k8sparentreftype"`
	K8SServiceName      string        `json:"k8sservicename"`
	K8SServiceNamespace string        `json:"k8sservicenamespace"`
	K8SRouteName        string        `json:"k8sroutename"`
	K8SRouteNamespace   string        `json:"k8sroutenamespace"`
}

type TargetGroupStatus struct {
	Name string `json:"name"`
	Arn  string `json:"arn"`
	Id   string `json:"id"`
}

type TargetGroupType string
type ParentRefType string
type RouteType string

const (
	TargetGroupTypeIP TargetGroupType = "IP"

	ParentRefTypeSvcExport ParentRefType = "ServiceExport"
	ParentRefTypeHTTPRoute ParentRefType = "HTTPRoute"
	ParentRefTypeGRPCRoute ParentRefType = "GRPCRoute"
	ParentRefTypeInvalid   ParentRefType = "INVALID"
)

func TGTagFieldsFromTags(tags map[string]*string) TargetGroupTagFields {
	return TargetGroupTagFields{
		EKSClusterName:      getMapValue(tags, EKSClusterNameKey),
		K8SParentRefType:    GetParentRefType(getMapValue(tags, K8SParentRefTypeKey)),
		K8SServiceName:      getMapValue(tags, K8SServiceNameKey),
		K8SServiceNamespace: getMapValue(tags, K8SServiceNamespaceKey),
		K8SRouteName:        getMapValue(tags, K8SRouteNameKey),
		K8SRouteNamespace:   getMapValue(tags, K8SRouteNamespaceKey),
	}
}

func getMapValue(m map[string]*string, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	return *v
}

func GetParentRefType(s string) ParentRefType {
	if s == "" {
		return "" // empty is OK
	}

	switch s {
	case string(ParentRefTypeHTTPRoute):
		return ParentRefTypeHTTPRoute
	case string(ParentRefTypeGRPCRoute):
		return ParentRefTypeGRPCRoute
	case string(ParentRefTypeSvcExport):
		return ParentRefTypeSvcExport
	default:
		return ParentRefTypeInvalid
	}
}

func TagFieldsMatch(spec TargetGroupSpec, tags TargetGroupTagFields) bool {
	specTags := TargetGroupTagFields{
		EKSClusterName:      spec.EKSClusterName,
		K8SParentRefType:    spec.K8SParentRefType,
		K8SServiceName:      spec.K8SServiceName,
		K8SServiceNamespace: spec.K8SServiceNamespace,
		K8SRouteName:        spec.K8SRouteName,
		K8SRouteNamespace:   spec.K8SRouteNamespace,
	}
	return reflect.DeepEqual(specTags, tags)
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

func (t *TargetGroupTagFields) IsServiceExport() bool {
	return t.K8SParentRefType == ParentRefTypeSvcExport
}

func (t *TargetGroupTagFields) IsRoute() bool {
	return t.K8SParentRefType == ParentRefTypeHTTPRoute ||
		t.K8SParentRefType == ParentRefTypeGRPCRoute
}

func (t *TargetGroupSpec) Validate() error {
	requiredFields := []string{t.K8SServiceName, t.K8SServiceNamespace,
		t.Protocol, t.ProtocolVersion, t.VpcId, t.EKSClusterName, t.IpAddressType,
		string(t.K8SParentRefType)}

	for _, s := range requiredFields {
		if s == "" {
			return errors.New("one or more required fields are missing")
		}
	}

	if t.IsRoute() {
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
	randomSuffix := make([]rune, RandomSuffixLength)
	for i := range randomSuffix {
		randomSuffix[i] = rune(rand.Intn(26) + 'a')
	}
	return fmt.Sprintf("%s-%s", prefix, string(randomSuffix))
}
