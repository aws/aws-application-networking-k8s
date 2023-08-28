# TargetGroupPolicy API Reference

## TargetGroupPolicy

By default, AWS Gateway API Controller assumes plaintext HTTP/1 traffic for backend Kubernetes resources.
TargetGroupPolicy is a CRD that can be attached to a Service, which allows the users to define protocol and
health check configurations of those backend resources.

When attaching a policy to a resource, the following restrictions apply:

* A policy can be only attached to `Service` resources.
* The attached resource can only be `backendRef` of `HTTPRoute` and `GRPCRoute`.
* The attached resource should exist in the same namespace as the policy resource.

The policy will not take effect if:

* The resource does not exist
* The resource is not referenced by any route
* The resource is referenced by a route of unsupported type

These restrictions are not forced; for example, users may create a policy that targets a service that is not created yet.
However, the policy will not take effect unless the target is valid.

### Notes

* Attaching TargetGroupPolicy to a resource that is already referenced by a route will result in a replacement
of VPC Lattice TargetGroup resource, except for health check updates.
* Removing TargetGroupPolicy of a resource will roll back protocol configuration to default setting. (HTTP1/HTTP plaintext)

|Field	|Description	|
|---	|---	|
|`apiVersion` *string*	|``application-networking.k8s.aws/v1alpha1``	|
|`kind` *string*	|``TargetGroupPolicy``	|
|`metadata` [*ObjectMeta*](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.26/#objectmeta-v1-meta)	|Kubernetes metadata for the resource.	|
|`spec` *TargetGroupPolicySpec*	|Defines the desired state of TargetGroupPolicy.	|

## TargetGroupPolicySpec

Appears on: TargetGroupPolicy

TargetGroupPolicySpec defines the desired state of TargetGroupPolicy.

Updates to this configuration result in a replacement of VPC Lattice TargetGroup resource, except for `healthCheck` field.

|Field	| Description|
|---	|---|
|`targetRef` *[PolicyTargetReference](https://gateway-api.sigs.k8s.io/geps/gep-713/#policy-targetref-api)*	| TargetRef points to the kubernetes `Service` resource that will have this policy attached. This field is following the guidelines of Kubernetes Gateway API policy attachment. |
|`protocol` *string*	| (Optional) The protocol to use for routing traffic to the targets. Supported values are `HTTP` (default) and `HTTPS`.<br/> Changes to this value results in a replacement of VPC Lattice target group.	|
|`protocolVersion` *string*	| (Optional) The protocol version to use. Supported values are `HTTP1` (default) and `HTTP2`. When a policy is behind GRPCRoute, this field value will be ignored as GRPC is only supported through HTTP/2.<br/> Changes to this value results in a replacement of VPC Lattice target group.	 |
|`healthCheck` *HealthCheckConfig*	| (Optional) The health check configuration.<br/> Changes to this value will update VPC Lattice resource in place. |

## HealthCheckConfig

Appears on: TargetGroupPolicySpec

HealthCheckConfig defines health check configuration for given VPC Lattice target group. For the detailed explanation and supported values, please refer to [VPC Lattice documentation](https://docs.aws.amazon.com/vpc-lattice/latest/ug/target-group-health-checks.html) on health checks.

|Field	|Description	|
|---	|---	|
|`enabled` *boolean*	|(Optional) Indicates whether health checking is enabled.	|
|`intervalSeconds` *integer*	|(Optional) The approximate amount of time, in seconds, between health checks of an individual target.	|
|`timeoutSeconds` *integer*	|(Optional) The amount of time, in seconds, to wait before reporting a target as unhealthy.	|
|`healthyThresholdCount` *integer*	|(Optional) The number of consecutive successful health checks required before considering an unhealthy target healthy.	|
|`statusMatch` *string*	|(Optional) A regular expression to match HTTP status codes when checking for a successful response from a target.	|
|`path` *string*	|(Optional) The destination for health checks on the targets.	|
|`port` *integer*	|(Optional) The port used when performing health checks on targets. If not specified, health check defaults to the port that a target receives traffic on.	|
|`protocol` *string*	|(Optional) The protocol used when performing health checks on targets.	|
|`protocolVersion` *string*	|(Optional) The protocol version used when performing health checks on targets. Defaults to HTTP/1.	|
|`unhealthyThresholdCount` *integer*	|(Optional) The number of consecutive failed health checks required before considering a target unhealthy.	|

## Example Configuration

This will enable TLS traffic between the gateway and Kubernetes service, with customized health check configuration.

Note that the TLS traffic is always terminated at the gateway, so it will be re-encrypted in this case. The gateway does not perform any certificate validations to the certificate on targets.

```
apiVersion: application-networking.k8s.aws/v1alpha1
kind: TargetGroupPolicy
metadata:
    name: test-policy
spec:
    targetRef:
        group: ""
        kind: Service
        name: my-parking-service
    protocol: HTTPS
    protocolVersion: HTTP1
    healthCheck:
        enabled: true
        intervalSeconds: 5
        timeoutSeconds: 1
        healthyThresholdCount: 3
        unhealthyThresholdcount: 2
        path: "/healthcheck"
        port: 80
        protocol: HTTP
        protocolVersion: HTTP
        statusMatch: "200"
```
