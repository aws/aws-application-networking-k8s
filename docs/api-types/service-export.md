# ServiceExport API Reference

## Introduction

In AWS Gateway API Controller, `ServiceExport` is the way of enabling a Service for multi-cluster traffic setup.
Once a Service is exported, other clusters can refer to it using [`ServiceImport`](service-import.md) resource.

ServiceExport is a term originated from Kubernetes [Multicluster Service APIs](https://multicluster.sigs.k8s.io/concepts/multicluster-services-api/);
but AWS Gateway API Controller uses its own version of ServiceExport resource with a certain set of features.

Internally, creating a ServiceExport creates a VPC Lattice [target group](https://docs.aws.amazon.com/vpc-lattice/latest/ug/target-groups.html).
You can take advantage of this by using the created target group in VPC Lattice setup outside Kubernetes.

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
