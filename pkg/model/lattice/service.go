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

func ServiceTagFieldsFromTags(tags map[string]*string) ServiceTagFields {
	return ServiceTagFields{
		RouteName:      getMapValue(tags, K8SRouteNameKey),
		RouteNamespace: getMapValue(tags, K8SRouteNamespaceKey),
		RouteType:      core.RouteType(getMapValue(tags, K8SRouteTypeKey)),
	}
}

func (t *ServiceTagFields) ToTags() services.Tags {
	rt := string(t.RouteType)
	return services.Tags{
		K8SRouteNameKey:      &t.RouteName,
		K8SRouteNamespaceKey: &t.RouteNamespace,
		K8SRouteTypeKey:      &rt,
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
	return utils.LatticeServiceName(s.RouteName, s.RouteNamespace)
}
