# VpcAssociationPolicy API Reference

## VpcAssociationPolicy

VpcAssociationPolicy is a Custom Resource Definition (CRD) that can be attached to a Gateway to define the ServiceNetworkVpcAssociation configuration.

### Fields of VpcAssociationPolicy

| Field Name	  | Type                                                                                                    | Required  | Description                                         | 
|--------------|---------------------------------------------------------------------------------------------------------|-----------|-----------------------------------------------------|
| `apiVersion` | *string*	                                                                                               | yes       | ``application-networking.k8s.aws/v1alpha1`` 	       |
| `kind`       | *string*	                                                                                               | yes       | ``VpcAssociationPolicy``                            |
| `metadata`   | [*ObjectMeta*](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.26/#objectmeta-v1-meta) | yes     	 | Kubernetes metadata for the resource.               |
| `spec`       | *VpcAssociationPolicySpec*	                                                                             | yes       | Defines the desired state of VpcAssociationPolicy.	 |



### Fields of VpcAssociationPolicySpec

Appears on: VpcAssociationPolicy

VpcAssociationPolicySpec defines the desired state of VpcAssociationPolicy.



| Field Name	        | Type                                                                                          | Required | Description                                                                                                                                                                                                                                                                                         |
|--------------------|-----------------------------------------------------------------------------------------------|----------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `targetRef`        | *[PolicyTargetReference](https://gateway-api.sigs.k8s.io/geps/gep-713/#policy-targetref-api)* | Yes	     | Points to the Kubernetes Gateway resource that will have this policy attached, following the guidelines of [Kubernetes Gateway API policy attachment](https://gateway-api.sigs.k8s.io/geps/gep-713/#policy-targetref-api).                                                                          |
| `associateWithVpc` | *bool*	                                                                                       | No       | Indicates whether the targetRef Gateway is associated with the current k8s cluster VPC. By default, the Gateway API controller sets this to true if it's not defined in VpcAssociationPolicy.                                                                                                       |
| `securityGroupIds` | *string[]*	                                                                                   | No       | Defines security groups applied to the gateway (ServiceNetworkVpcAssociation), it controls the inbound traffic from current cluster workloads to the gateway listeners. Please check the [VPC Lattice doc](https://docs.aws.amazon.com/vpc-lattice/latest/ug/security-groups.html) for more detail. |


Recommended security group inbound rules:

| Source                                                  | Protocol                                            | Port Range                                      | Comment                                                   |
|---------------------------------------------------------|-----------------------------------------------------|-------------------------------------------------|-----------------------------------------------------------|
| Kubernetes cluster VPC CIDR or security group reference | Protocols defined in the gateway's listener section | Ports defined in the gateway's listener section | Allow inbound traffic from current cluster vpc to gateway |


### Limitations and Considerations

When attaching a VpcAssociationPolicy to a resource, the following restrictions apply:

* Policies must be attached to *Gateway* resource.
* The attached resource must exist in the same namespace as the policy resource.

The security group will not take effect if:

* The `targetRef` gateway does not exist.
* The `associateWithVpc` field is set to false.


**WARNING**

The VPC Lattice `UpdateServiceNetworkVpcAssociation` API cannot be used to remove all security groups.
If you have a VpcAssociationPolicy attached to a gateway that already has security groups applied, updating the VpcAssociationPolicy with empty security group ids or deleting the VpcAssociationPolicy will NOT remove the security groups from the gateway.

To remove security groups, instead, you should delete VPC Association and re-create a new VPC Association without security group ids by following steps:
1. Update the VpcAssociationPolicy by setting `associateWithVpc` to false and empty security group ids.
2. Update the VpcAssociationPolicy by setting `associateWithVpc` to true and empty security group ids.
`
Note: Setting `associateWithVpc` to false will disable traffic from the current cluster workloads to the gateway.

## Example Configuration

This example shows how to configure a gateway with `associateWithVpc` set to true and apply security group sg-1234567890 and sg-0987654321 
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
