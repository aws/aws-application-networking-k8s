# IAMAuthPolicy API Reference

## Introduction

VPC Lattice Auth Policies are IAM policy documents that are attached to VPC Lattice Service Networks or Services to control
authorization of principal's access the attached Service Network's Services, or the specific attached Service.

IAMAuthPolicy implements Direct Policy Attachment of Gateway APIs [GEP-713: Metaresources and Policy Attachment](https://gateway-api.sigs.k8s.io/geps/gep-713). 
An IAMAuthPolicy can be attached to a Gateway, HTTPRoute, or GRPCRoute.

Please visit the [VPC Lattice Auth Policy documentation page](https://docs.aws.amazon.com/vpc-lattice/latest/ug/auth-policies.html)
for more details about Auth Policies.

### Features

- Attaching a policy to a Gateway results in an AuthPolicy being applied to the Gateway's associated
VPC Lattice Service Network.
- Attaching a policy to an HTTPRoute or GRPCRoute results in an AuthPolicy being applied to
the Route's associated VPC Lattice Service.

**Note:** IAMAuthPolicy can only do authorization for traffic that travels through Gateways, HTTPRoutes, and GRPCRoutes.
The authorization will not take effect if the client directly sends traffic to the k8s service DNS.

[This article](https://aws.amazon.com/blogs/containers/implement-aws-iam-authentication-with-amazon-vpc-lattice-and-amazon-eks/)
is also a good reference on how to set up VPC Lattice Auth Policies in Kubernetes.

## Status

| Type | Status / Reason | Meaning |
|---|---|---|
| `Accepted` | `True` / `Accepted` | The `targetRef` is valid and VPC Lattice accepted the auth policy. Other reasons follow the Gateway API policy attachment conventions (`Invalid`, `TargetNotFound`, `Conflicted`). |
| `Accepted` | `False` / `Invalid` | Also set when VPC Lattice rejects the policy document (`ValidationException`), for example invalid syntax or an IAM principal that does not exist. The message carries the VPC Lattice error, and the controller tries again every 15 minutes, since the cause can be outside the policy spec (such as an IAM role that is created later). |

Example after VPC Lattice rejects a policy that names an IAM role that does not exist:

```yaml
status:
  conditions:
  - type: Accepted
    status: "False"
    reason: Invalid
    message: 'VPC Lattice rejected the auth policy: ValidationException: Policy principal is not found.'
```

When VPC Lattice rejects an edit, the controller also records a `Warning` event with reason `FailedReconcile` on
each attempt, and the previously accepted policy is not changed. If a policy was accepted before and is then edited
to an invalid document, the last accepted policy stays in effect in VPC Lattice while the status shows `Invalid`.
The status reports the result of the last attempt to apply the policy, not which policy is in effect. Fix the document to clear the `Invalid` status.

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
