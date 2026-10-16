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
```

With the grant above, an `HTTPRoute` in namespace `foo` may use a `backendRef` to a
`Service` in namespace `bar`. To permit a cross-namespace `ServiceImport`, set the `to`
entry to `group: application-networking.k8s.aws`, `kind: ServiceImport`.

Same-namespace references never require a `ReferenceGrant`.

## Current behavior

Per the Gateway API specification, a route may only forward traffic to a backend in another
namespace when a `ReferenceGrant` permits it. This controller does not yet block
unpermitted cross-namespace references; it **warns** about them. The reference is still
accepted and traffic still flows. For each unpermitted cross-namespace `backendRef` the
controller:

* records a Warning Event on the route (`CrossNamespaceRefNotPermitted`), visible in
  `kubectl describe <httproute|grpcroute|tlsroute> <name>`,
* logs a warning line identifying the route and the target, and
* sets the `referencegrant_cross_namespace_refs_not_permitted` gauge, labeled by route
  kind, route namespace, route name, and backend kind (`Service`, `ServiceImport`, or
  `Other`).

The grant's `from.kind` must match the kind of the referencing route (HTTPRoute,
GRPCRoute, or TLSRoute).

A future release will block these references. To prepare, find the warned references and
for each one either create a `ReferenceGrant` in the target namespace or repoint the
`backendRef`, until no `referencegrant_cross_namespace_refs_not_permitted` series remain.
A route's series are removed once its references are permitted, rather than being set to
zero, so alert on the presence of the series rather than on a zero value.

The warning signal requires the `gateway.networking.k8s.io/v1` `ReferenceGrant` CRD to be
installed when the controller starts. If it is not, the controller logs that it is
skipping the watch and emits no warnings, so an absent or zero gauge does not by itself
mean the cluster is ready for the blocking release.
