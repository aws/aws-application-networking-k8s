# TLSRoute API Reference

## Introduction

With integration of the Gateway API, AWS Gateway API Controller supports `TLSRoute`.
This allows you to define and manage end-to-end TLS encrypted traffic routing to your Kubernetes clusters.

### TLSRoute Key Features & Limitations

**Features**:

- **Routing Traffic**: Enables routing end-to-end TLS encrypted traffic from your client workload to server workload.


**Limitations**:

- **Listener Protocol**: The `TLSRoute` sectionName must refer to an TLS protocol listener with mode: Passthrough in the parent `Gateway`.

- `TLSRoute` only supports to have one rule.
- `TLSRoute` don't support `matches` field in the rule.
- The `hostnames` field with exactly one host name is required. This domain name is used as a vpc lattice's Service Name Indication (SNI) match.


## Example Configuration

Here is a sample configuration that demonstrates how to set up  a `TLSRoute` resource to route end-to-end TLS encrypted traffic to a nginx service:

```yaml
apiVersion: gateway.networking.k8s.io/v1alpha2
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
- The `hostnames` field is set to `nginx-test.my-test.com`. The customer must use this domain name to send traffic to the nginx service.

This `TLSRoute` documentation provides a detailed introduction, feature set, and a basic example of how to configure
and use the resource within AWS Gateway API Controller project. For in-depth details and specifications, you can refer to the
official [Gateway API documentation](https://gateway-api.sigs.k8s.io/reference/spec/#gateway.networking.k8s.io/v1alpha2.TLSRoute).