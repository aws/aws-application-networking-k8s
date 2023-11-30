# ServiceExport API Reference

## Introduction

In AWS Gateway API Controller, `ServiceExport` enables a Service for multi-cluster traffic setup.
Clusters can import the exported service with [`ServiceImport`](service-import.md) resource.

Internally, creating a ServiceExport creates a standalone VPC Lattice [target group](https://docs.aws.amazon.com/vpc-lattice/latest/ug/target-groups.html).
Even without ServiceImports, creating ServiceExports can be useful in case you only need the target groups created;
for example, using target groups in the VPC Lattice setup outside Kubernetes.

Note that ServiceExport is not the implementation of Kubernetes [Multicluster Service APIs](https://multicluster.sigs.k8s.io/concepts/multicluster-services-api/);
instead AWS Gateway API Controller uses its own version of the resource for the purpose of Gateway API integration.

For more multi-cluster use cases and detailed explanation, please refer to [our recommended multi-cluster architecture](../guides/multi-cluster.md).

### Limitations
* The exported Service can only be used in HTTPRoutes. GRPCRoute is currently not supported.
* Limited to one ServiceExport per Service. If you need multiple exports representing each port,
  you should create multiple Service-ServiceExport pairs.

### Annotations

* `application-networking.k8s.aws/port`  
  Represents which port of the exported Service will be used.
  When a comma-separated list of ports is provided, the traffic will be distributed to all ports in the list.

## Example Configuration

The following yaml will create a ServiceExport for a Service named `service-1`:
```yaml
apiVersion: application-networking.k8s.aws/v1alpha1
kind: ServiceExport
metadata:
  name: service-1
  annotations:
    application-networking.k8s.aws/port: "9200"
spec: {}
```
