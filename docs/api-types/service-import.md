# ServiceImport API Reference

## Introduction

`ServiceImport` is a resource referring to a Service or VPC Lattice target group outside the cluster. It can be paired with a [`ServiceExport`](service-export.md) resource defined in another cluster, or pointed directly at an externally owned VPC Lattice target group via annotation.

Just like Services, ServiceImports can be a backend reference of HTTPRoutes, GRPCRoutes, and TLSRoutes. Along with the cluster's own Services (and ServiceImports from even more clusters), you can distribute traffic across multiple VPCs, multiple clusters, or between an EKS native backend and a pre existing VPC Lattice target group that points at non EKS workloads (ECS, Lambda, EC2, ALB).

Note that ServiceImport is not the implementation of Kubernetes [Multicluster Service APIs](https://multicluster.sigs.k8s.io/concepts/multicluster-services-api/); instead AWS Gateway API Controller uses its own version of the resource for the purpose of Gateway API integration.

The controller resolves a ServiceImport to a VPC Lattice target group in one of three mutually exclusive modes, selected by which annotations are present:

| Mode | Annotations on ServiceImport | Resolution |
|---|---|---|
| Mode 1 — Default name | _(none)_ | The controller finds a target group created by a `ServiceExport`, matched by the exported service name and namespace carried in the target group's tags. The exported service name defaults to the ServiceImport's own name, so a matching `ServiceExport` must exist for that name. |
| Mode 2 — Tag discovery | Any of `export-name`, `aws-vpc`, `aws-eks-cluster-name` | Same `ServiceExport` tag matching as Mode 1. The difference is that `export-name` overrides which exported service name the controller looks up. It no longer has to equal the ServiceImport's own name (Mode 1's default) and can target a `ServiceExport` published under a different name. `aws-vpc` / `aws-eks-cluster-name` further narrow the match to a specific VPC or cluster (typically a `ServiceExport` in another cluster). |
| Mode 3 — Direct external target group | `target-group-arn` | The controller resolves directly to the supplied target group ARN, regardless of who owns it. No `ServiceExport` is required. |

### Limitations
* ServiceImport shares the limitations of [ServiceExport](service-export.md).
* ServiceImport can only be used as a backendRef in a route (HTTPRoute, GRPCRoute, or TLSRoute); sending traffic directly is not supported.
* BackendRef ports pointing to ServiceImport are not respected. Use the [port annotation](service-export.md#annotations) of ServiceExport instead.
* Under Mode 3, the external target group is treated as reference only: the controller never tags it with `ManagedBy`, never registers targets on it, and never deletes it. Lifecycle remains the responsibility of whoever originally created it.

### Annotations
* `application-networking.k8s.aws/export-name`<br>
  (Optional, Mode 2) When specified, the controller will find the target group created by the named ServiceExport instead of matching by the ServiceImport's own name. See [ServiceExport](service-export.md) for the corresponding `service-name` annotation.

* `application-networking.k8s.aws/aws-eks-cluster-name`<br>
  (Optional, Mode 2) When specified, the controller will only find target groups exported from the named cluster.

* `application-networking.k8s.aws/aws-vpc`<br>
  (Optional, Mode 2) When specified, the controller will only find target groups exported from the cluster with the provided VPC ID.

* `application-networking.k8s.aws/target-group-arn`<br>
  (Optional, Mode 3) When specified, the controller resolves this ServiceImport directly to the supplied VPC Lattice target group ARN. Mutually exclusive with the Mode 2 annotations above. See [Mode 3 — Direct external target group reference](#mode-3-direct-external-target-group-reference).

## Example Configuration

### Mode 1 / Mode 2 — Importing a `ServiceExport`

The following yaml imports `service-1` exported from the designated cluster.
```yaml
apiVersion: application-networking.k8s.aws/v1alpha1
kind: ServiceImport
metadata:
  name: service-1
  annotations:
    application-networking.k8s.aws/aws-eks-cluster-name: "service-1-owner-cluster"
    application-networking.k8s.aws/aws-vpc: "service-1-owner-vpc-id"
spec:
  type: ClusterSetIP
  ports:
  - port: 80
    protocol: TCP
```

The following example HTTPRoute directs traffic to the above ServiceImport.
```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: my-route
spec:
  parentRefs:
    - name: my-gateway
      sectionName: http
  rules:
    - backendRefs:
        - name: service-1
          kind: ServiceImport
```

The following example GRPCRoute directs gRPC traffic to the above ServiceImport.
```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: GRPCRoute
metadata:
  name: my-grpc-route
spec:
  parentRefs:
    - name: my-gateway
      sectionName: https
  rules:
    - backendRefs:
        - name: service-1
          kind: ServiceImport
```

The following example TLSRoute directs TLS traffic to the above ServiceImport.
```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: TLSRoute
metadata:
  name: my-tls-route
spec:
  hostnames:
    - my-service.example.com
  parentRefs:
    - name: my-gateway
      sectionName: tls
  rules:
    - backendRefs:
        - name: service-1
          kind: ServiceImport
```

The following yaml imports a ServiceExport named `checkout-cluster1` using the `export-name` annotation.
```yaml
apiVersion: application-networking.k8s.aws/v1alpha1
kind: ServiceImport
metadata:
  name: checkout-import-1
  annotations:
    application-networking.k8s.aws/export-name: "checkout-cluster1"
    application-networking.k8s.aws/aws-eks-cluster-name: "cluster1"
spec:
  type: ClusterSetIP
  ports:
  - port: 80
    protocol: TCP
```

For a complete multi-cluster example, see [Cross-Cluster Routing with Same Service Names](../guides/advanced-configurations.md#cross-cluster-routing-with-same-service-names).

## Mode 3 — Direct external target group reference

When the `application-networking.k8s.aws/target-group-arn` annotation is present, the controller resolves the ServiceImport directly to the supplied VPC Lattice target group ARN. No `ServiceExport` is involved.

Mode 3 exists mainly to migrate traffic from a non EKS workload to an EKS backend with weighted routing. For the full migration guide see [Migrate to EKS with weighted routing](../guides/migrate-to-eks.md).

### Annotation

* `application-networking.k8s.aws/target-group-arn`<br>
  Full ARN of the VPC Lattice target group, e.g. `arn:aws:vpc-lattice:us-east-2:123456789012:targetgroup/tg-0abc123def4567890`. <br> Region and account are not validated against the controller's running region/account by ARN parsing. However, the controller's Lattice client is pinned to its configured region, so a cross region ARN will not resolve today and surfaces as `ExternalTargetGroupNotFound`. A cross account target group only resolves if it is reachable by the controller (e.g. RAM shared) and the controller's IAM principal can read it.

### Example Configuration

The following yaml references a pre existing VPC Lattice target group by ARN.
```yaml
apiVersion: application-networking.k8s.aws/v1alpha1
kind: ServiceImport
metadata:
  name: orders-legacy-tg
  namespace: production
  annotations:
    application-networking.k8s.aws/target-group-arn: "arn:aws:vpc-lattice:us-east-2:123456789012:targetgroup/tg-0abc123def4567890"
spec:
  type: ClusterSetIP
  ports:
  - port: 80
    protocol: TCP
```

## See also

* [Migrate to EKS with weighted routing](../guides/migrate-to-eks.md) 
* [Service Name Override](../guides/service-name-override.md)
* [ServiceExport](service-export.md)
