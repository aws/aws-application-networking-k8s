# IAMAuthPolicy API Reference

## Introduction

VPC Lattice Auth Policies are IAM policy documents that are attached to VPC Lattice Service Networks or Services to control
authorization of principal's access the attached Service Network's Services, or the specific attached Service.

IAMAuthPolicy implements Direct Policy Attachment of Gateway APIs [GEP-713: Metaresources and Policy Attachment](https://gateway-api.sigs.k8s.io/geps/gep-713). 
An IAMAuthPolicy can be attached to a Gateway, HTTPRoute, or GRPCRoute.

Please visit the [VPC Lattice Auth Policy documentation page](https://docs.aws.amazon.com/vpc-lattice/latest/ug/auth-policies.html)
for more details about Auth Policies.

## Features

- Attaching a policy to a Gateway results in an AuthPolicy being applied to the Gateway's associated
VPC Lattice Service Network.
- Attaching a policy to an HTTPRoute or GRPCRoute results in an AuthPolicy being applied to
the Route's associated VPC Lattice Service.

**Note:** IAMAuthPolicy can only do authorization for traffic that travels through Gateways, HTTPRoutes, and GRPCRoutes.
The authorization will not take effect if the client directly sends traffic to the k8s service DNS.

[This article](https://aws.amazon.com/blogs/containers/implement-aws-iam-authentication-with-amazon-vpc-lattice-and-amazon-eks/)
is also a good reference on how to set up VPC Lattice Auth Policies in Kubernetes.

## Example Configuration

### Example 1

This configuration attaches a policy to the Gateway, `default/my-hotel`. The policy only allows traffic
with the header, `header1=value1`, through the Gateway. This means, for every child HTTPRoute and GRPCRoute of the
Gateway, only traffic with the specified header will be authorized to access it.

```yaml
apiVersion: application-networking.k8s.aws/v1alpha1
kind: IAMAuthPolicy
metadata:
    name: test-iam-auth-policy
spec:
    targetRef:
        group: "gateway.networking.k8s.io"
        kind: Gateway
        name: my-hotel
    policy: |
        {
            "Version": "2012-10-17",
            "Statement": [
                {
                    "Effect": "Allow",
                    "Principal": "*",
                    "Action": "vpc-lattice-svcs:Invoke",
                    "Resource": "*",
                    "Condition": {
                        "StringEquals": {
                            "vpc-lattice-svcs:RequestHeader/header1": "value1"
                        }
                    }
                }
            ]
        }
```

### Example 2

This configuration attaches a policy to the HTTPRoute, `examplens/my-route`. The policy only allows
traffic from the principal, `123456789012`, to the HTTPRoute. Note that the traffic from the specified principal must
be SIGv4-signed to be authorized.

```yaml
apiVersion: application-networking.k8s.aws/v1alpha1
kind: IAMAuthPolicy
metadata:
    name: test-iam-auth-policy
spec:
    targetRef:
        group: "gateway.networking.k8s.io"
        kind: HTTPRoute
        namespace: examplens
        name: my-route
    policy: |
        {
            "Version": "2012-10-17",
            "Statement": [
                {
                    "Effect": "Allow",
                    "Principal": "123456789012",
                    "Action": "vpc-lattice-svcs:Invoke",
                    "Resource": "*"
                }
            ]
        }
```
