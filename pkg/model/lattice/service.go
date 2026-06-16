package lattice

import (
	"github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
	"github.com/aws/aws-application-networking-k8s/pkg/utils"
)

type Service struct {
	core.ResourceMeta `json:"-"`
	Spec              ServiceSpec    `json:"spec"`
	Status            *ServiceStatus `json:"status,omitempty"`
	IsDeleted         bool           `json:"isdeleted"`
}

type ServiceSpec struct {
	ServiceTagFields
	ServiceNetworkNames []string      `json:"servicenetworkhnames"`
	CustomerDomainName  string        `json:"customerdomainname"`
	CustomerCertARN     string        `json:"customercertarn"`
	AdditionalTags      services.Tags `json:"additionaltags,omitempty"`
	AllowTakeoverFrom   string        `json:"allowtakeoverfrom,omitempty"`
	ServiceNameOverride string        `json:"servicenameoverride,omitempty"`
}

type ServiceStatus struct {
	Arn string `json:"arn"`
	Id  string `json:"id"`
	Dns string `json:"dns"`
}

type ServiceTagFields struct {
	RouteName      string
	RouteNamespace string
	RouteType      core.RouteType
}

func ServiceTagFieldsFromTags(tags map[string]string) ServiceTagFields {
	return ServiceTagFields{
		RouteName:      tags[K8SRouteNameKey],
		RouteNamespace: tags[K8SRouteNamespaceKey],
		RouteType:      core.RouteType(tags[K8SRouteTypeKey]),
	}
}

func (t *ServiceTagFields) ToTags() services.Tags {
	return services.Tags{
		K8SRouteNameKey:      t.RouteName,
		K8SRouteNamespaceKey: t.RouteNamespace,
		K8SRouteTypeKey:      string(t.RouteType),
	}
}

func NewLatticeService(stack core.Stack, spec ServiceSpec) (*Service, error) {
	id := spec.LatticeServiceName()

	service := &Service{
		ResourceMeta: core.NewResourceMeta(stack, "AWS::VPCServiceNetwork::Service", id),
		Spec:         spec,
		Status:       nil,
	}

	err := stack.AddResource(service)
	if err != nil {
		return nil, err
	}

	return service, nil
}

func (s *Service) LatticeServiceName() string {
	return s.Spec.LatticeServiceName()
}

func (s *ServiceSpec) LatticeServiceName() string {
	return utils.LatticeServiceName(s.RouteName, s.RouteNamespace, s.ServiceNameOverride)
}
