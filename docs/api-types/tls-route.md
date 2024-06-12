# TLSRoute API Reference

## Introduction

With integration of the Gateway API, AWS Gateway API Controller supports `TLSRoute`.
This allows you to define and manage end-to-end TLS encrypted traffic routing to your Kubernetes clusters.

### Considerations

- `TLSRoute` sectionName must refer to an `TLS` protocol listener with `mode: Passthrough` in the parentRefs `Gateway`.
- `TLSRoute` only supports to have one rule.
- `TLSRoute` doesn't support any rule matching condition.
- The `hostnames` field with exactly one host name is required. This domain name is used as a vpc lattice's Service Name Indication (SNI) match to route the traffic to the correct backend service.


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


For the detailed tls passthrough traffic connectivity setup, please refer the user guide [here](../guides/tls-passthrough.md).

For the detailed Gateway API `TLSRoute` resource specifications, you can refer to the
Kubernetes official [documentation](https://gateway-api.sigs.k8s.io/reference/spec/#gateway.networking.k8s.io/v1alpha2.TLSRoute).

For the VPC Lattice tls passthrough Listener configuration details, you can refer to the VPC Lattice [documentation](https://docs.aws.amazon.com/vpc-lattice/latest/ug/tls-listeners.html).