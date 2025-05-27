# ServiceExport API Reference

## Introduction

In AWS Gateway API Controller, `ServiceExport` enables a Service for multi-cluster traffic setup.
Clusters can import the exported service with [`ServiceImport`](service-import.md) resource.

Internally, creating a ServiceExport creates a standalone VPC Lattice [target group](https://docs.aws.amazon.com/vpc-lattice/latest/ug/target-groups.html).
Even without ServiceImports, creating ServiceExports can be useful in case you only need the target groups created;
for example, using target groups in the VPC Lattice setup outside Kubernetes.

Note that ServiceExport is not the implementation of Kubernetes [Multicluster Service APIs](https://multicluster.sigs.k8s.io/concepts/multicluster-services-api/);
instead AWS Gateway API Controller uses its own version of the resource for the purpose of Gateway API integration.

### Annotations (Legacy Method)

* `application-networking.k8s.aws/port`  
  Represents which port of the exported Service will be used.
  When a comma-separated list of ports is provided, the traffic will be distributed to all ports in the list.
  
  **Note:** This annotation is supported for backward compatibility. For new deployments, it's recommended to use the `spec.exportedPorts` field instead.

## Spec Fields

### exportedPorts

The `exportedPorts` field allows you to explicitly define which ports of the service should be exported and what route types they should be used with. This is useful when you have a service with multiple ports serving different protocols.

Each exported port has the following fields:
* `port`: The port number to export
* `routeType`: The type of route this port should be used with. Valid values are:
  * `HTTP`: For HTTP traffic
  * `GRPC`: For gRPC traffic
  * `TLS`: For TLS traffic

If `exportedPorts` is not specified, the controller will use the port from the annotation "application-networking.k8s.aws/port" and create HTTP target groups for backward compatibility.

## Example Configurations

### Legacy Configuration (Using Annotations)

The following yaml will create a ServiceExport for a Service named `service-1` using the legacy annotation method:
```yaml
apiVersion: application-networking.k8s.aws/v1alpha1
kind: ServiceExport
metadata:
  name: service-1
  annotations:
    application-networking.k8s.aws/port: "9200"
spec: {}
```

### Using exportedPorts

The following yaml will create a ServiceExport for a Service named `service-1` with multiple ports for different route types:
```yaml
apiVersion: application-networking.k8s.aws/v1alpha1
kind: ServiceExport
metadata:
  name: service-1
spec:
  exportedPorts:
  - port: 80
    routeType: HTTP
  - port: 8081
    routeType: GRPC
```

This configuration will:
1. Export port 80 to be used with HTTP routes
2. Export port 8081 to be used with gRPC routes
