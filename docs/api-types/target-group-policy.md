# TargetGroupPolicy API Reference

## Introduction

By default, AWS Gateway API Controller assumes plaintext HTTP/1 traffic for backend Kubernetes resources.
TargetGroupPolicy is a CRD that can be attached to Service or ServiceExport, which allows the users to define protocol, protocol version and
health check configurations of those backend resources.

When attaching a policy to a resource, the following restrictions apply:

- A policy can be attached to `Service` that being `backendRef` of `HTTPRoute`, `GRPCRoute` and `TLSRoute`.
- A policy can be attached to `ServiceExport`.
- The attached resource should exist in the same namespace as the policy resource.

### Multi-Cluster Health Check Configuration

In multi-cluster deployments, TargetGroupPolicy health check configurations are automatically propagated across all clusters that participate in the service mesh. When a TargetGroupPolicy is applied to a ServiceExport, all target groups created for that service across different clusters will use the same health check configuration, ensuring consistent health monitoring regardless of which cluster contains the route resource.

The policy will not take effect if:
- The resource does not exist
- The resource is not referenced by any route
- The resource is referenced by a route of unsupported type
- The ProtocolVersion is non-empty if the TargetGroupPolicy protocol is TCP

Please check the TargetGroupPolicy API Reference for more details. [TargetGroupPolicy API Reference](../api-reference.md#application-networking.k8s.aws/v1alpha1.TargetGroupPolicy)


These restrictions are not forced; for example, users may create a policy that targets a service that is not created yet.
However, the policy will not take effect unless the target is valid.



### ServiceExport with ExportedPorts

When using ServiceExport with the `exportedPorts` field, TargetGroupPolicy protocol and protocolVersion settings will override the default protocol inferred from the `routeType`. This allows you to configure HTTPS or other protocols even when the routeType is HTTP.

Example:

```yaml
apiVersion: application-networking.k8s.aws/v1alpha1
kind: ServiceExport
metadata:
  name: my-service
spec:
  exportedPorts:
  - port: 80
    routeType: HTTP  # Default would be HTTP/1
---
apiVersion: application-networking.k8s.aws/v1alpha1
kind: TargetGroupPolicy
metadata:
  name: https-override
spec:
  targetRef:
    group: "application-networking.k8s.aws"
    kind: ServiceExport
    name: my-service
  protocol: HTTPS        # Overrides HTTP from routeType
  protocolVersion: HTTP2 # Overrides HTTP1 default
  healthCheck:
    enabled: true
    path: "/health"
    protocol: HTTPS
    protocolVersion: HTTP2
```

In this example, even though the ServiceExport specifies `routeType: HTTP`, the TargetGroupPolicy will configure the target group to use HTTPS with HTTP/2, providing secure communication between the VPC Lattice service and your backend pods.

### Limitations and Considerations

- Attaching TargetGroupPolicy to an existing Service that is already referenced by a route will result in a replacement
  of VPC Lattice TargetGroup resource, except for health check updates.
- Attaching TargetGroupPolicy to an existing ServiceExport will result in a replacement of VPC Lattice TargetGroup resource, except for health check updates.
- Removing TargetGroupPolicy of a resource will roll back protocol configuration to default setting. (HTTP1/HTTP plaintext)
- In multi-cluster deployments, TargetGroupPolicy changes will automatically propagate to all clusters participating in the service mesh, ensuring consistent configuration across the deployment.

## Example Configurations

### Single Cluster Configuration

This will enable HTTPS traffic between the gateway and Kubernetes service, with customized health check configuration.

```yaml
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
        protocolVersion: HTTP1
        statusMatch: "200"
```

### Multi-Cluster Configuration

This example shows how to configure health checks for a ServiceExport in a multi-cluster deployment. The health check configuration will be automatically applied to all target groups across all clusters that participate in the service mesh.

```yaml
apiVersion: application-networking.k8s.aws/v1alpha1
kind: TargetGroupPolicy
metadata:
    name: multi-cluster-policy
spec:
    targetRef:
        group: "application-networking.k8s.aws"
        kind: ServiceExport
        name: inventory-service
    protocol: HTTP
    protocolVersion: HTTP2
    healthCheck:
        enabled: true
        intervalSeconds: 10
        timeoutSeconds: 5
        healthyThresholdCount: 2
        unhealthyThresholdCount: 3
        path: "/health"
        port: 8080
        protocol: HTTP
        protocolVersion: HTTP2
        statusMatch: "200-299"
```

In this multi-cluster example:
- The policy targets a `ServiceExport` named `inventory-service`
- All clusters with target groups for this service will use HTTP/2 for traffic and the specified health check configuration
- Health checks will use HTTP/1 on port 8080 with the `/health` endpoint
- The configuration ensures consistent health monitoring across all participating clusters
