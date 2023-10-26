# AccessLogPolicy API Reference

## Introduction

The AccessLogPolicy custom resource allows you to define access logging configurations on
Gateways, HTTPRoutes, and GRPCRoutes by specifying a destination for the access logs to be published to.

## Features
- When an AccessLogPolicy is created for a Gateway target, VPC Lattice traffic to any Route that is a child of that Gateway will have access logs published to the provided destination
- When an AccessLogPolicy is created for an HTTPRoute or GRPCRoute target, VPC Lattice traffic to that Route will have access logs published to the provided destination

## Definition

| Field        | Type                                                                                                     | Description                                      |
|--------------|----------------------------------------------------------------------------------------------------------|--------------------------------------------------|
| `apiVersion` | *string*                                                                                                 | `application-networking.k8s.aws/v1alpha1`       |
| `kind`       | *string*                                                                                                 | `AccessLogPolicy`                                |
| `metadata`   | [*ObjectMeta*](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.26/#objectmeta-v1-meta) | Kubernetes metadata for the resource.            |
| `spec`       | *AccessLogPolicySpec*                                                                                    | Defines the desired state of AccessLogPolicy.    |

### AccessLogPolicySpec

| Field                                       | Type                                                                                           | Description                                                                                                                                           |
|---------------------------------------------|------------------------------------------------------------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------|
| `destinationArn`                            | *string*                                                                                       | The ARN of the Amazon S3 Bucket, Amazon CloudWatch Log Group, or Amazon Kinesis Data Firehose Delivery Stream that will have access logs published to it. |
| `targetRef`                                 | *[PolicyTargetReference](https://gateway-api.sigs.k8s.io/geps/gep-713/#policy-targetref-api)* | TargetRef points to the kubernetes `Gateway`, `HTTPRoute`, or `GRPCRoute` resource that will have this policy attached. This field is following the guidelines of Kubernetes Gateway API policy attachment. |

## Example Configurations

### Example 1

This configuration results in access logs being published to the S3 Bucket, `my-bucket`, when traffic
is sent to any HTTPRoute or GRPCRoute that is a child of Gateway `my-hotel`.

```yaml
apiVersion: application-networking.k8s.aws/v1alpha1
kind: AccessLogPolicy
metadata:
  name: my-access-log-policy
spec:
  destinationArn: "arn:aws:s3:::my-bucket"
  targetRef:
    group: gateway.networking.k8s.io
    kind: Gateway
    name: my-hotel
```

### Example 2

This configuration results in access logs being published to the CloudWatch Log Group, `myloggroup`, when traffic
is sent to HTTPRoute `inventory`.

```yaml
apiVersion: application-networking.k8s.aws/v1alpha1
kind: AccessLogPolicy
metadata:
  name: my-access-log-policy
spec:
  destinationArn: "arn:aws:logs:us-west-2:123456789012:log-group:myloggroup:*"
  targetRef:
    group: gateway.networking.k8s.io
    kind: HTTPRoute
    name: inventory
```

## AWS Permissions Required

Per the [VPC Lattice documentation](https://docs.aws.amazon.com/vpc-lattice/latest/ug/monitoring-access-logs.html#monitoring-access-logs-IAM),
 IAM permissions are required to enable access logs:

```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Sid": "ManageVPCLatticeAccessLogSetup",
            "Action": [
                "logs:CreateLogDelivery",
                "logs:GetLogDelivery",
                "logs:UpdateLogDelivery",
                "logs:DeleteLogDelivery",
                "logs:ListLogDeliveries",
                "vpc-lattice:CreateAccessLogSubscription",
                "vpc-lattice:GetAccessLogSubscription",
                "vpc-lattice:UpdateAccessLogSubscription",
                "vpc-lattice:DeleteAccessLogSubscription",
                "vpc-lattice:ListAccessLogSubscriptions"
            ],
            "Resource": [
                "*"
            ]
        }
    ]
}
```

## Statuses

AccessLogPolicies fit under the definition of [Gateway API Policy Objects](https://gateway-api.sigs.k8s.io/geps/gep-713/#on-policy-objects).
As a result, status conditions are applied on every modification of an AccessLogPolicy, and can be viewed by describing it.

### Status Condition Reasons

#### Accepted

The spec of the AccessLogPolicy is valid and has been successfully processed.

#### Conflicted

The target already has an AccessLogPolicy for the same destination type
(i.e. a target can have 1 AccessLogPolicy for an S3 Bucket, 1 for a CloudWatch Log Group,
and 1 for a Firehose Delivery Stream at a time).

#### Invalid

Any of the following:
- The target's `Group` is not `gateway.networking.k8s.io`
- The target's `Kind` is not `Gateway`, `HTTPRoute`, or `GRPCRoute`
- The target's namespace does not match the AccessLogPolicy's namespace
- The destination with the given `destinationArn` could not be found
- The controller is missing AWS permissions

#### TargetNotFound

The target does not exist.

## Annotations

Upon successful creation or modification of an AccessLogPolicy, the controller may add or update an annotation in the
AccessLogPolicy. The annotation applied by the controller has the key
`application-networking.k8s.aws/accessLogSubscription`, and its value is the corresponding VPC Lattice Access Log
Subscription's ARN.

When an AccessLogPolicy's `destinationArn` is changed such that the resource type changes (e.g. from S3 Bucket to CloudWatch Log Group),
or the AccessLogPolicy's `targetRef` is changed, the annotation's value will be updated because a new Access Log Subscription will be created to replace the previous one.

When creation of an AccessLogPolicy fails, no annotation is added to the AccessLogPolicy because no corresponding Access Log Subscription exists.

When modification or deletion of an AccessLogPolicy fails, the previous value of the annotation is left unchanged because the
corresponding Access Log Subscription is also left unchanged.
