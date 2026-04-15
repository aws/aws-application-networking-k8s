# ServiceImport API Reference

## Introduction

`ServiceImport` is a resource referring to a Service outside the cluster, paired with [`ServiceExport`](service-export.md)
resource defined in the other clusters.

Just like Services, ServiceImports can be a backend reference of HTTPRoutes, GRPCRoutes, and TLSRoutes. Along with the
cluster's own Services (and ServiceImports from even more clusters), you can distribute the traffic across multiple VPCs
and clusters.

Note that ServiceImport is not the implementation of Kubernetes [Multicluster Service APIs](https://multicluster.sigs.k8s.io/concepts/multicluster-services-api/);
instead AWS Gateway API Controller uses its own version of the resource for the purpose of Gateway API integration.


### Limitations
* ServiceImport shares the limitations of [ServiceExport](service-export.md).
* ServiceImport can only be used as a backendRef in a route (HTTPRoute, GRPCRoute, or TLSRoute); sending traffic directly is not supported.
* BackendRef ports pointing to ServiceImport is not respected. Use [port annotation](service-export.md#annotations) of ServiceExport instead.

### Annotations
* `application-networking.k8s.aws/export-name`  
  (Optional) When specified, the controller will find the target group created by the named ServiceExport instead of
  matching by the ServiceImport's own name. See [ServiceExport](service-export.md) for the corresponding `service-name` annotation.

* `application-networking.k8s.aws/aws-eks-cluster-name`  
  (Optional) When specified, the controller will only find target groups exported from the cluster.
* `application-networking.k8s.aws/aws-vpc`  
  (Optional) When specified, the controller will only find target groups exported from the cluster with the provided VPC ID.

## Example Configuration

The following yaml imports `service-1` exported from the designated cluster.
```yaml
apiVersion: application-networking.k8s.aws/v1alpha1
kind: ServiceImport
metadata:
  name: service-1
  annotations:
    application-networking.k8s.aws/aws-eks-cluster-name: "service-1-owner-cluster"
    application-networking.k8s.aws/aws-vpc: "service-1-owner-vpc-id"
spec: {}
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
apiVersion: gateway.networking.k8s.io/v1alpha2
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
