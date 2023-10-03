package lattice

import (
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
)

const (
	K8SServiceNetworkOwnedByVPC = "K8SServiceNetworkOwnedByVPC"
	K8SServiceOwnedByVPC        = "K8SServiceOwnedByVPC"
)

type ServiceNetwork struct {
	core.ResourceMeta `json:"-"`
	Spec              ServiceNetworkSpec    `json:"spec"`
	Status            *ServiceNetworkStatus `json:"status,omitempty"`
}

type ServiceNetworkSpec struct {
	// The name of the ServiceNetwork
	Name             string    `json:"name"`
	Namespace        string    `json:"namespace"`
	Account          string    `json:"account"`
	SecurityGroupIds []*string `json:"securityGroupIds"`
	AssociateToVPC   bool
	IsDeleted        bool
}

type ServiceNetworkStatus struct {
	ServiceNetworkARN    string    `json:"servicenetworkARN"`
	ServiceNetworkID     string    `json:"servicenetworkID"`
	SnvaSecurityGroupIds []*string `json:"securityGroupIds"`
}

func NewServiceNetwork(stack core.Stack, id string, spec ServiceNetworkSpec) *ServiceNetwork {

	servicenetwork := &ServiceNetwork{
		//TODO right name
		ResourceMeta: core.NewResourceMeta(stack, "AWS::VPCServiceNetwork::ServiceNetwork", id),
		Spec:         spec,
		Status:       nil,
	}

	stack.AddResource(servicenetwork)
	// TODO: servicenetwork.registerDependencies(stack)

	return servicenetwork

}
