# TLSRoute API Reference

## Introduction

With integration of the Gateway API, AWS Gateway API Controller supports `TLSRoute`.
This allows you to define and manage end-to-end TLS encrypted traffic routing to your Kubernetes clusters.

### Considerations

- `TLSRoute` sectionName must refer to a `TLS` protocol listener with `mode: Passthrough` in the parentRefs `Gateway`. The `tls.mode` field must be explicitly set on the listener.
- `TLSRoute` only supports exactly one rule.
- Each rule must have at least one `backendRef`.
- `TLSRoute` does not support any rule matching condition.
- The `hostnames` field with exactly one host name is required. IP addresses are not allowed.


### Annotations

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

Here is a sample configuration that demonstrates how to set up  a `TLSRoute` resource to route end-to-end TLS encrypted traffic to a nginx service:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: TLSRoute
metadata:
  name: nginx-tls-route
spec:
  hostnames:
    - nginx-test.my-test.com
  parentRefs:
    - name: my-hotel-tls-passthrough
      sectionName: tls
  rules:
    - backendRefs:
        - name: nginx-tls
          kind: Service
          port: 443
```

In this example:

- The `TLSRoute` is named ` nginx-tls-route` and is associated with a parent gateway named `my-hotel-tls-passthrough` that has
  a listener section named `tls`:
```
    - name: tls
      protocol: TLS
      port: 443
      tls:
        mode: Passthrough
```
- The `TLSRoute` is configured to route traffic to a k8s service named `nginx-tls` on port 443.
- The `hostnames` field is set to `nginx-test.my-test.com`. The customer must use this hostname to send traffic to the nginx service.


## Cross-Cluster Routing with ServiceImport

`TLSRoute` supports routing to services in other clusters via [`ServiceImport`](service-import.md).
The remote cluster must export the service using a [`ServiceExport`](service-export.md) with `routeType: TLS`.

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: TLSRoute
metadata:
  name: nginx-tls-route
spec:
  hostnames:
    - nginx-test.my-test.com
  parentRefs:
    - name: my-hotel-tls-passthrough
      sectionName: tls
  rules:
    - backendRefs:
        - name: nginx-tls
          kind: ServiceImport
```

For the detailed tls passthrough traffic connectivity setup, please refer the user guide [here](../guides/tls-passthrough.md).

For the detailed Gateway API `TLSRoute` resource specifications, you can refer to the
Kubernetes official [documentation](https://gateway-api.sigs.k8s.io/reference/spec/#gateway.networking.k8s.io/v1.TLSRoute).

For the VPC Lattice tls passthrough Listener configuration details, you can refer to the VPC Lattice [documentation](https://docs.aws.amazon.com/vpc-lattice/latest/ug/tls-listeners.html).
