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
	Name          string        `json:"name"`
	Namespace     string        `json:"namespace"`
	Port          int64         `json:"port"`
	Protocol      string        `json:"protocol"`
	DefaultAction DefaultAction `json:"defaultaction"`
}

type DefaultAction struct {
	BackendServiceName      string `json:"backendservicename"`
	BackendServiceNamespace string `json:"backendservicenamespace"`
}
type ListenerStatus struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	ListenerARN string `json:"listenerARN"`
	ListenerID  string `json:"listenerID"`
	ServiceID   string `json:"serviceID"`
	Port        int64  `json:"port"`
	Protocol    string `json:"protocol"`
}

func NewListener(stack core.Stack, id string, port int64, protocol string, name string, namespace string, action DefaultAction) *Listener {

	listener := &Listener{
		ResourceMeta: core.NewResourceMeta(stack, "AWS::VPCServiceNetwork::Listener", id),
		Spec: ListenerSpec{
			Name:          name,
			Namespace:     namespace,
			Port:          port,
			Protocol:      protocol,
			DefaultAction: action,
		},
		Status: nil,
	}

	stack.AddResource(listener)

	return listener
}
