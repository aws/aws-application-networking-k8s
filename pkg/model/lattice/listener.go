package lattice

import (
	"github.com/aws/aws-application-networking-k8s/pkg/model/core"
)

type Listener struct {
	core.ResourceMeta `json:"-"`
	Spec              ListenerSpec    `json:"spec"`
	Status            *ListenerStatus `json:"status,omitempty"`
}

type ListenerSpec struct {
	StackServiceId    string `json:"stackserviceid"`
	K8SRouteName      string `json:"k8sroutename"`
	K8SRouteNamespace string `json:"k8sroutenamespace"`
	Port              int64  `json:"port"`
	Protocol          string `json:"protocol"`
}

type ListenerStatus struct {
	Name        string `json:"name"`
	ListenerArn string `json:"listenerarn"`
	Id          string `json:"listenerid"`
	ServiceId   string `json:"serviceid"`
}

func NewListener(stack core.Stack, spec ListenerSpec) (*Listener, error) {
	id, err := core.IdFromHash(spec)
	if err != nil {
		return nil, err
	}

	listener := &Listener{
		ResourceMeta: core.NewResourceMeta(stack, "AWS::VPCServiceNetwork::Listener", id),
		Spec:         spec,
		Status:       nil,
	}

	stack.AddResource(listener)

	return listener, nil
}
