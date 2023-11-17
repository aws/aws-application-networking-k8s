# VpcAssociationPolicy API Reference

## Introduction

VpcAssociationPolicy is a Custom Resource Definition (CRD) that can be attached to a Gateway to define the configuration
of the ServiceNetworkVpcAssociation between the Gateway's associated VPC Lattice Service Network and the cluster VPC.

## Recommended Security Group Inbound Rules

| Source                                                  | Protocol                                            | Port Range                                      | Comment                                                   |
|---------------------------------------------------------|-----------------------------------------------------|-------------------------------------------------|-----------------------------------------------------------|
| Kubernetes cluster VPC CIDR or security group reference | Protocols defined in the gateway's listener section | Ports defined in the gateway's listener section | Allow inbound traffic from current cluster vpc to gateway |

## Limitations and Considerations

When attaching a VpcAssociationPolicy to a resource, the following restrictions apply:

* Policies must be attached to *Gateway* resource.
* The attached resource must exist in the same namespace as the policy resource.

The security group will not take effect if:

* The `targetRef` gateway does not exist.
* The `associateWithVpc` field is set to false.


### WARNING

The VPC Lattice `UpdateServiceNetworkVpcAssociation` API cannot be used to remove all security groups.
If you have a VpcAssociationPolicy attached to a gateway that already has security groups applied, updating the VpcAssociationPolicy with empty security group ids or deleting the VpcAssociationPolicy will NOT remove the security groups from the gateway.

To remove security groups, instead, you should delete VPC Association and re-create a new VPC Association without security group ids by following steps:
1. Update the VpcAssociationPolicy by setting `associateWithVpc` to false and empty security group ids.
2. Update the VpcAssociationPolicy by setting `associateWithVpc` to true and empty security group ids.
`
Note: Setting `associateWithVpc` to false will disable traffic from the current cluster workloads to the gateway.

## Example Configuration

This configuration attaches a policy to the Gateway, `default/my-hotel`. The ServiceNetworkVpcAssociation between the
Gateway's corresponding VPC Lattice Service Network and the cluster VPC is updated based on the policy contents.

If the expected ServiceNetworkVpcAssociation does not exist, it is created since `associateWithVpc` is set to `true`.
This allows traffic from clients in the cluster VPC to VPC Lattice Services in the associated Service Network.
Additionally, two security groups (`sg-1234567890` and `sg-0987654321`) are attached to the ServiceNetworkVpcAssociation.

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
