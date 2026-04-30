/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +genclient:nonNamespaced
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=gateway-api,shortName=sn
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="ARN",type=string,JSONPath=`.status.serviceNetworkARN`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:subresource:status

// ServiceNetwork is a cluster scoped resource that manages VPC Lattice Service Network lifecycle.
type ServiceNetwork struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec ServiceNetworkSpec `json:"spec,omitempty"`

	// Status defines the current state of ServiceNetwork.
	//
	// +kubebuilder:default={conditions: {{type: "Accepted", status: "Unknown", reason:"NotReconciled", message:"Waiting for controller", lastTransitionTime: "1970-01-01T00:00:00Z"}}}
	Status ServiceNetworkStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// ServiceNetworkList contains a list of ServiceNetworks.
type ServiceNetworkList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServiceNetwork `json:"items"`
}

// ServiceNetworkSpec defines the desired state of ServiceNetwork.
type ServiceNetworkSpec struct {
	// Reserved for future use.
}

// ServiceNetworkStatus defines the observed state of ServiceNetwork.
type ServiceNetworkStatus struct {
	// Conditions describe the current conditions of the ServiceNetwork.
	//
	// Known condition types are:
	//
	// * "Accepted"
	// * "Programmed"
	//
	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=8
	// +kubebuilder:default={{type: "Accepted", status: "Unknown", reason:"Pending", message:"Waiting for controller", lastTransitionTime: "1970-01-01T00:00:00Z"},{type: "Programmed", status: "Unknown", reason:"Pending", message:"Waiting for controller", lastTransitionTime: "1970-01-01T00:00:00Z"}}
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ARN of the VPC Lattice service network.
	// +optional
	ServiceNetworkARN string `json:"serviceNetworkARN,omitempty"`

	// ID of the VPC Lattice service network.
	// +optional
	ServiceNetworkID string `json:"serviceNetworkID,omitempty"`
}
