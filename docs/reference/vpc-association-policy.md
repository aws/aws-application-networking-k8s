# VpcAssociationPolicy API Reference

## VpcAssociationPolicy

VpcAssociationPolicy is a CRD that can be attached to a Gateway to define the ServiceNetworkVpcAssociation configuration.



One of its option is `securityGroupIds`. it can control the inbound traffic from current cluster workloads to the gateway listeners. Please check the VPC lattice doc for more detail of this option. https://docs.aws.amazon.com/vpc-lattice/latest/ug/security-groups.html

Recommended security group inbound rules:

| Source                                                  | Protocol                                            | Port Range                                      | Comment                                                   |
|---------------------------------------------------------|-----------------------------------------------------|-------------------------------------------------|-----------------------------------------------------------|
| Kubernetes cluster VPC CIDR or security group reference | protocols defined in the gateway's listener section | ports defined in the gateway's listener section | Allow inbound traffic from current cluster vpc to gateway |


**Limitations and Considerations**

When attaching a policy to a resource, the following restrictions apply:

* A policy can be only attached to `Gateway` resources.
* The attached resource should exist in the same namespace as the policy resource.

The security Group will not take effect if:

* The targetRef `Gateway` does not exist
* AssociateWithVpc field set to false


**WARNING**

Current VPC Lattice updateServiceNetworkVpcAssociation api have a limitation that it cannot remove all security groups.
That means, if you have a VpcAssociationPolicy attached to a gateway that already applied security groups, following operations will NOT take effect to remove the security groups:
* Update the VPCAssociationPolicy to empty the security group ids (even though the updated VPCAssociationPolicy can be accepted by the API server)
* Delete the VPCAssociationPolicy (even though the VPCAssociationPolicy can be deleted from k8s successfully)

To remove security groups, instead, you should delete VPC Association and then create a new VPC Association without security group ids by following steps:
1. Update the VPCAssociationPolicy with AssociateWithVpc is false and empty security group ids
2. Update the VPCAssociationPolicy with AssociateWithVpc is true and empty security group ids

Be cautious to set AssociateWithVpc to false. That can break traffic from the current cluster workloads to the gateway.


| Field	                                                                                                                      | Description	                                        |
|-----------------------------------------------------------------------------------------------------------------------------|-----------------------------------------------------|
| `apiVersion` *string*	                                                                                                      | ``application-networking.k8s.aws/v1alpha1`` 	       |
| `kind` *string*	                                                                                                            | ``VpcAssociationPolicy``                            |
| `metadata` [*ObjectMeta*](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.26/#objectmeta-v1-meta)        	 | Kubernetes metadata for the resource.               |
| `spec` *VpcAssociationPolicySpec*	                                                                                          | Defines the desired state of VpcAssociationPolicy.	 |


## VpcAssociationPolicySpec

Appears on: VpcAssociationPolicy

VpcAssociationPolicySpec defines the desired state of VpcAssociationPolicy.



| Field	                                                                                                     | Description                                                                                                                                                                                                                |
|------------------------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `targetRef` *[PolicyTargetReference](https://gateway-api.sigs.k8s.io/geps/gep-713/#policy-targetref-api)*	 | TargetRef points to the kubernetes `Gateway` resource that will have this policy attached. This field is following the guidelines of Kubernetes Gateway API policy attachment.                                             |
| `associateWithVpc` *bool*	                                                                                 | (Optional)  This field indicates whether targetRef Gateway associate with current k8s cluster VPC. the gateway api controller by default set associateWithVpc to true if this field is not defined in VpcAssociationPolicy |
| `securityGroupIds` *string[]*	                                                                             | (Optional) This field defines security groups applied to the gateway (ServiceNetworkVpcAssociation) 	                                                                                                                      |


## Example Configuration

This example shows how to configure a Gateway with associateWithVpc set to true and apply security group sg-1234567890 and sg-0987654321 
```
apiVersion: application-networking.k8s.aws/v1alpha1
kind: VpcAssociationPolicy
metadata:
    name: test-vpc-association-policy
spec:
    targetRef:
        group: "gateway.networking.k8s.io"
        kind: Gateway
        name: my-hotel
    securityGroupIds:
        - sg-1234567890
        - sg-0987654321
    associateWithVpc: true
```
