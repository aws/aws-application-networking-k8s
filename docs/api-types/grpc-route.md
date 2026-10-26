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
- **Method Matches**: One method match is allowed within a single rule.
- **Header Matches Limit**: A maximum of 5 header matches per rule is supported.
- **No Method Without Service**: Matching only by a gRPC method without specifying a service is not supported.
- **Case Insensitivity**: All method matches are currently case-insensitive.

### Annotations

- `application-networking.k8s.aws/lattice-assigned-domain-name`  
  Represents a VPC Lattice generated domain name for the resource. This annotation will automatically set
  when a `GRPCRoute` is programmed and ready.

- `application-networking.k8s.aws/dns-endpoint`  
  Set to `disabled` to stop the controller from managing a `DNSEndpoint` for this route. Use it when DNS
  records for the route's hostname are managed outside the controller. If a `DNSEndpoint` created by the
  controller for this route already exists, it is deleted. A `DNSEndpoint` with the same name that is not
  owned by the route is left untouched. Removing the annotation makes the controller create the `DNSEndpoint` again.
  See [Managing DNS records using ExternalDNS](../guides/custom-domain-name.md#managing-dns-records-using-externaldns).

- `application-networking.k8s.aws/idle-timeout-seconds`  
  Sets the idle connection timeout, in seconds, for the VPC Lattice Service that this route maps to.
  The idle timeout is the amount of time a connection can remain idle (no data sent) before VPC Lattice
  closes it. Valid range is `60`–`600`. If the annotation has never been set, VPC Lattice applies
  its default of `60` seconds. The value must be quoted, e.g. `"120"`. An out-of-range or non-integer value is ignored (left unset) and logged as a warning.

    !!! note
        The idle timeout is distinct from the fixed maximum connection duration (10 minutes) enforced by
        VPC Lattice, which is not configurable. Raising the idle timeout only affects connections that are
        idle, not long-lived active connections.

    !!! note
        The controller manages this value only while the annotation is present. Removing the
        annotation leaves the last applied value on the VPC Lattice Service rather than reverting it,
        so a value set out-of-band is preserved. To change it back, set the annotation to the desired
        value (e.g. `"60"`).

## Example Configuration

Here is a sample configuration that demonstrates how to set up a `GRPCRoute` for a HelloWorld gRPC service:

```yaml
apiVersion: gateway.networking.k8s.io/v1
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

## Cross-Cluster Routing with ServiceImport

`GRPCRoute` supports routing to services in other clusters via [`ServiceImport`](service-import.md).
The remote cluster must export the service using a [`ServiceExport`](service-export.md) with `routeType: GRPC`.

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: GRPCRoute
metadata:
  name: greeter-grpc-route
spec:
  parentRefs:
    - name: my-hotel
      sectionName: https
  rules:
    - matches:
        - method:
            service: helloworld.Greeter
            method: SayHello
      backendRefs:
        - name: greeter-grpc-server
          kind: ServiceImport
```

---

This `GRPCRoute` documentation provides a detailed introduction, feature set, and a basic example of how to configure
and use the resource within AWS Gateway API Controller project. For in-depth details and specifications, you can refer to the
official [Gateway API documentation](https://gateway-api.sigs.k8s.io/reference/spec/#gateway.networking.k8s.io/v1.GRPCRoute).