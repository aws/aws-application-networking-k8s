# HTTPRoute API Reference

## Introduction

With integration of the Gateway API, AWS Gateway API Controller supports `HTTPRoute`.
This allows you to define and manage the routing of HTTP and HTTPS traffic within your Kubernetes cluster.

### HTTPRoute Key Features & Limitations

**Features**:

- **Routing Traffic**: Enables routing HTTP traffic to servers within your Kubernetes cluster.
- **Path and Method Matching**: The `HTTPRoute` allows for matching by:
    - An exact path.
    - Any path with a specified prefix.
    - A specific HTTP Method.
- **Header Matching**: Enables matching based on specific headers in the HTTP request.

**Limitations**:

- **Listener Protocol**: The `HTTPRoute` sectionName must refer to an HTTP or HTTPS listener in the parent `Gateway`.
- **Method Matches**: One method match is allowed within a single rule.
- **QueryParam Matches**: Matching by QueryParameters is not supported.
- **Header Matches Limit**: A maximum of 5 header matches per rule is supported.
- **Case Insensitivity**: All path matches are currently case-insensitive.

### Annotations

- `application-networking.k8s.aws/lattice-assigned-domain-name`  
  Represents a VPC Lattice generated domain name for the resource. This annotation will automatically set
  when a `HTTPRoute` is programmed and ready.

## Example Configuration

### Example 1

Here is a sample configuration that demonstrates how to set up an `HTTPRoute` that forwards HTTP traffic to a
Service and ServiceImport, using rules to determine which backendRef to route traffic to.

```yaml
apiVersion: gateway.networking.k8s.io/v1beta1
kind: HTTPRoute
metadata:
  name: inventory
spec:
  parentRefs:
    - name: my-hotel
      sectionName: http
  rules:
    - backendRefs:
        - name: inventory-ver1
          kind: Service
          port: 80
      matches:
        - path:
            type: PathPrefix
            value: /ver1
    - backendRefs:
        - name: inventory-ver2
          kind: ServiceImport
          port: 80
      matches:
        - path:
            type: PathPrefix
            value: /ver2
```

In this example:

- The `HTTPRoute` is named `inventory` and is associated with a parent gateway named `my-hotel` that has
  a section named `http`.
- The first routing rule forwards traffic to a backend Service named `inventory-ver1` on port `80`.
  The rule also specifies a path match condition, where traffic must have a path starting with `/ver1` for the routing
  rule to apply.
- The second routing rule forwards traffic to a backend ServiceImport named `inventory-ver2` on port `80`.
  The rule also specifies a path match condition, where traffic must have a path starting with `/ver2` for the routing
  rule to apply.

### Example 2

Here is a sample configuration that demonstrates how to set up a `HTTPRoute` that forwards HTTP and HTTPS traffic to a
Service and ServiceImport, using weighted rules to route more traffic to one backendRef than the other. Weighted rules
simplify the process of creating blue/green deployments by shifting rule weight from one backendRef to another.

```yaml
apiVersion: gateway.networking.k8s.io/v1beta1
kind: HTTPRoute
metadata:
  name: inventory
spec:
  parentRefs:
    - name: my-hotel
      sectionName: http
    - name: my-hotel
      sectionName: https
  rules:
    - backendRefs:
        - name: inventory-ver1
          kind: Service
          port: 80
          weight: 10
        - name: inventory-ver2
          kind: ServiceImport
          port: 80
          weight: 90
```

In this example:

- The `HTTPRoute` is named `inventory` and is associated with a parent gateway named `my-hotel` that has
  two sections, named `http` and `https`.
- The first routing rule forwards traffic to a backend Service named `inventory-ver1` on port `80`.
  The rule also specifies a weight of `10`.
- The second routing rule forwards traffic to a backend ServiceImport named `inventory-ver2` on port `80`.
  The rule also specifies a weight of `90`.
- The amount of traffic forwarded to a backendRef is `(rule weight / total weight) * 100%`. Thus, 10% of the traffic is
  forwarded to `inventory-ver1` at port `80` and 90% of the traffic is forwarded to `inventory-ver2` at the default port.

---

This `HTTPRoute` documentation provides a detailed introduction, feature set, and a basic example of how to configure
and use the resource within AWS Gateway API Controller project. For in-depth details and specifications, you can refer to the
official [Gateway API documentation](https://gateway-api.sigs.k8s.io/reference/spec/#gateway.networking.k8s.io/v1alpha2.HTTPRoute).