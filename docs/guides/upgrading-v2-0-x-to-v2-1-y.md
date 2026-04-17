# Update the Amazon VPC Lattice Gateway API Controller from v2.0.x to v2.1.y

Release `v2.1.0` of the Amazon VPC Lattice Gateway API Controller is built against `v1.5` of the Gateway API spec. It is *not* compatible with Gateway API CRD versions prior to `v1.5`.

Previous `v2.0.x` builds of the controller (up to `v2.0.2`) were built against `v1.2` of the Gateway API spec. This guide outlines the controller upgrade process from `v2.0.x` to `v2.1.y`. Please note that users are required to install Gateway API CRDs themselves. The latest Gateway API CRDs are available [here](https://gateway-api.sigs.k8s.io/). Please [follow this installation](https://gateway-api.sigs.k8s.io/guides/#installing-gateway-api) process.

## Prerequisites

- **Kubernetes 1.31 or higher** is required. Gateway API v1.5.0 uses CEL validation rules for TLSRoute that require Kubernetes 1.31+.

## Upgrade Process

1. Back up configuration, in particular TLSRoute objects
2. Disable `v2.0.x` controller (e.g. scale to zero)
3. Update Gateway API CRDs to `v1.5.0`
4. Deploy and launch `v2.1.y` controller version

The old controller must be disabled before upgrading CRDs because it watches `v1alpha2` TLSRoute, which is no longer served by the v1.5 standard CRDs. Running the old controller against v1.5 CRDs will cause it to crash.

### TLSRoute `v1alpha2` to `v1`

Gateway API `v1.5` promoted TLSRoute to the Standard channel as `v1`. The `v1alpha2` version of TLSRoute is no longer served by the standard CRDs. TLSRoute is now included in the standard CRD bundle and no longer needs to be installed separately.

Existing TLSRoutes stored as `v1alpha2` in etcd will continue working. The API server converts them to `v1` on read. When the controller reconciles and updates their status, the objects are re-stored as `v1` in etcd.

Users must update their local YAML manifests from `apiVersion: gateway.networking.k8s.io/v1alpha2` to `apiVersion: gateway.networking.k8s.io/v1` for future applies. Applying manifests with `v1alpha2` will be rejected by the API server since that version is no longer served.

### Stricter CRD Validation

The `v1` TLSRoute CRD introduces stricter validation. The controller already enforced most of these at reconciliation time, but they are now rejected at admission by the API server:

- `hostnames` is now required with at least 1 entry (was optional)
- `rules` is limited to exactly 1 rule (was up to 16)
- `rules[].backendRefs` is required with at least 1 entry (was optional)
- Hostnames cannot contain IP addresses (enforced via CEL validation)
- Gateway listeners with `protocol: TLS` must explicitly set `tls.mode` to `Passthrough`

Review your existing TLSRoute and Gateway objects before upgrading.

### ValidatingAdmissionPolicy

Gateway API v1.5.0 installs a ValidatingAdmissionPolicy (`safe-upgrades.gateway.networking.k8s.io`) that prevents downgrading CRDs to a version prior to v1.5. If you need to rollback, delete this policy first:

```
kubectl delete validatingadmissionpolicy safe-upgrades.gateway.networking.k8s.io
```
