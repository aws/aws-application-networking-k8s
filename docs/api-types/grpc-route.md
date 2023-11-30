# GRPCRoute API Reference

## Introduction

With integration of the Gateway API, AWS Gateway API Controller supports `GRPCRoute`.
This allows you to define and manage the routing of gRPC traffic within your Kubernetes cluster.

### GRPCRoute Key Features & Limitations

**Features**:

- **Routing Traffic**: Enables routing gRPC traffic to servers within your Kubernetes cluster.
- **Service and Method Matching**: The `GRPCRoute` allows for matching by:
    - An exact gRPC service and method.
    - An exact gRPC service without specifying a method.
    - All gRPC services and methods.
- **Header Matching**: Enables matching based on specific headers in the gRPC request.

**Limitations**:

- **Listener Protocol**: The `GRPCRoute` sectionName must refer to an HTTPS listener in the parent `Gateway`.
- **Service Export**: The `GRPCRoute` does not support integration with `ServiceExport`.
- **Method Matches**: One method match is allowed within a single rule.
- **Header Matches Limit**: A maximum of 5 header matches per rule is supported.
- **No Method Without Service**: Matching only by a gRPC method without specifying a service is not supported.
- **Case Insensitivity**: All method matches are currently case-insensitive.

### Annotations

- `application-networking.k8s.aws/lattice-assigned-domain-name`  
  Represents a VPC Lattice generated domain name for the resource. This annotation will automatically set
  when a `GRPCRoute` is programmed and ready.

## Example Configuration

Here is a sample configuration that demonstrates how to set up a `GRPCRoute` for a HelloWorld gRPC service:

```yaml
apiVersion: gateway.networking.k8s.io/v1alpha2
kind: GRPCRoute
metadata:
  name: greeter-grpc-route
spec:
  parentRefs:
    - name: my-hotel
      sectionName: https
  rules:
    - matches:
        - headers:
            - name: testKey1
              value: testValue1
      backendRefs:
        - name: greeter-grpc-server
          kind: Service
          port: 50051
          weight: 10
    - matches:
        - method:
            service: helloworld.Greeter
            method: SayHello
      backendRefs:
        - name: greeter-grpc-server
          kind: Service
          port: 443
```

In this example:

- The `GRPCRoute` is named `greeter-grpc-route` and is associated with a parent gateway named `my-hotel` that has
  a section named `https`.
- The first routing rule is set up to forward traffic to a backend service named `greeter-grpc-server` on port `50051`.
  The rule also specifies a header match condition, where traffic must have a header with the name `testKey1` and
  value `testValue1` for the routing rule to apply.
- The second rule matches gRPC traffic for the service `helloworld.Greeter` and method `SayHello`, forwarding it to
  the `greeter-grpc-server` on port `443`.

---

This `GRPCRoute` documentation provides a detailed introduction, feature set, and a basic example of how to configure
and use the resource within AWS Gateway API Controller project. For in-depth details and specifications, you can refer to the
official [Gateway API documentation](https://gateway-api.sigs.k8s.io/references/spec/#networking.x-k8s.io/v1alpha2.GRPCRoute).