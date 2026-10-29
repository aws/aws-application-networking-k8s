# ReferenceGrant

## Introduction

`ReferenceGrant` is the Gateway API resource that allows a reference to cross a namespace
boundary. Per the Gateway API specification, a route may only forward traffic to a backend
(`Service` or [`ServiceImport`](service-import.md)) in another namespace when a
`ReferenceGrant` in the backend's namespace permits it. See
[Current behavior](#current-behavior) for how this controller handles it today.

A `ReferenceGrant` lives in the **target** namespace (the backend's namespace) and names,
in its `from` section, the namespace and kind allowed to reference into it.

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: ReferenceGrant
metadata:
  name: allow-foo-httproutes
  namespace: bar
spec:
  from:
    - group: gateway.networking.k8s.io
      kind: HTTPRoute
      namespace: foo
  to:
    - group: ""
      kind: Service
    - group: application-networking.k8s.aws
      kind: ServiceImport
```

With the grant above, an `HTTPRoute` in namespace `foo` may use a `backendRef` to a
`Service` or a `ServiceImport` in namespace `bar`. For a `ServiceImport`, the grant must
name `group: application-networking.k8s.aws`, even if the route's `backendRef` leaves the
group out. A grant with `group: ""` matches only `Service`.

Same-namespace references never require a `ReferenceGrant`.

## Current behavior

Per the Gateway API specification, a route may only forward traffic to a backend in another
namespace when a `ReferenceGrant` permits it. This controller does not yet block
unpermitted cross-namespace references; it **warns** about them. The reference is still
accepted and traffic still flows. For each unpermitted cross-namespace `backendRef` the
controller:

* records a Warning Event on the route (`CrossNamespaceRefNotPermitted`), visible in
  `kubectl describe <httproute|grpcroute|tlsroute> <name>`. The Event is best-effort: it
  is emitted when the set of unpermitted references first appears or changes (and again
  after a controller restart), and it ages out of `kubectl describe` after the API
  server's event TTL, so a missing Event does not by itself mean the reference is
  permitted; and
* logs a warning line identifying the route and the unpermitted target.

The grant's `from.kind` must match the kind of the referencing route (HTTPRoute,
GRPCRoute, or TLSRoute).

A future release will block these references. To prepare, review the controller logs and
route Events for `CrossNamespaceRefNotPermitted` and, for each flagged reference, either
create a `ReferenceGrant` in the target namespace or repoint the `backendRef`.

The warning signal requires the `gateway.networking.k8s.io/v1` `ReferenceGrant` CRD to be
installed when the controller starts. If it is not, the controller logs that it is
skipping the watch and emits no warnings, so the absence of warnings does not by itself
mean the cluster is ready for the blocking release — it can also mean the warning signal
is not running.

## Upgrading

The warning signal reads `ReferenceGrant` resources, so the controller's `ClusterRole`
grants `get`/`list`/`watch` on `referencegrants`. This rule ships with the controller
image that introduces the feature. When upgrading, apply the updated RBAC (Helm chart or
manifests) together with the new image — do not point the new image at an older
`ClusterRole`. If the CRD is installed but the controller lacks permission to watch it,
the watch fails to start and the controller will not come up healthy.

