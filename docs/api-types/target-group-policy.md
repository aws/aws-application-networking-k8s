# TargetGroupPolicy API Reference

## Introduction

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

**Limitations and Considerations**
* Attaching TargetGroupPolicy to a resource that is already referenced by a route will result in a replacement
of VPC Lattice TargetGroup resource, except for health check updates.
* Removing TargetGroupPolicy of a resource will roll back protocol configuration to default setting. (HTTP1/HTTP plaintext)

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
        unhealthyThresholdCount: 2
        path: "/healthcheck"
        port: 80
        protocol: HTTP
        protocolVersion: HTTP
        statusMatch: "200"
```
