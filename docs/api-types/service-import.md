# ServiceImport API Reference

## Introduction

`ServiceImport` is a resource referring to a Service outside the cluster, paired with [`ServiceExport`](service-export.md)
resource defined in the other clusters.

Just like Services, ServiceImports can be a backend reference of HTTPRoutes. Along with the cluster's own Services
(and ServiceImports from even more clusters), you can distribute the traffic across multiple VPCs and clusters.

Note that ServiceImport is not the implementation of Kubernetes [Multicluster Service APIs](https://multicluster.sigs.k8s.io/concepts/multicluster-services-api/);
instead AWS Gateway API Controller uses its own version of the resource for the purpose of Gateway API integration.


### Limitations
* ServiceImport shares the limitations of [ServiceExport](service-export.md).
* The controller only supports ServiceImport through HTTPRoute; sending traffic directly is not supported.
* BackendRef ports pointing to ServiceImport is not respected. Use [port annotation](service-export.md#annotations) of ServiceExport instead.

### Annotations
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