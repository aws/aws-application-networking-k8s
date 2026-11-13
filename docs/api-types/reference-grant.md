# ReferenceGrant

## Introduction

`ReferenceGrant` is the Gateway API resource that allows a reference to cross a namespace
boundary. Per the Gateway API specification, a route may only forward traffic to a backend
(`Service` or [`ServiceImport`](service-import.md)) in another namespace when a
`ReferenceGrant` in the backend's namespace permits it. This controller blocks
cross-namespace references that no `ReferenceGrant` permits.

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

## Blocked references

When no `ReferenceGrant` permits a cross-namespace `backendRef`, the controller:

* sets the route's `ResolvedRefs` condition to `False` with reason `RefNotPermitted`. The
  route stays `Accepted`, and the condition does not reveal whether the target exists;
* sends no traffic to that backend. A rule whose backends are all blocked returns HTTP
  500. If some of a rule's backends are permitted, all of the rule's traffic goes to
  those, because VPC Lattice cannot return 500 for a weighted share of requests; and
* records a Warning Event on the route (`CrossNamespaceRefNotPermitted`) and logs a
  warning line naming the target.

The grant's `from.kind` must match the kind of the referencing route (HTTPRoute,
GRPCRoute, or TLSRoute). Creating, changing or deleting a grant takes effect without any
change to the route.

The controller reads grants from the `gateway.networking.k8s.io/v1` `ReferenceGrant` CRD.
If that CRD is not installed when the controller starts, no grant can permit a reference,
so every cross-namespace `backendRef` is blocked. After installing the CRD, restart the
controller.

## Upgrading

Earlier releases only warned about these references. Before upgrading, find the routes
that will be blocked and create a `ReferenceGrant` or repoint the `backendRef` for each:

```bash
kubectl get events -A --field-selector reason=CrossNamespaceRefNotPermitted
```

Events expire after the API server's event TTL, so also check the controller logs for
`cross-namespace backendRefs not permitted by any ReferenceGrant`. Earlier releases emit no
warnings when the `ReferenceGrant` CRD is not installed or the controller cannot list
ReferenceGrants, so check the controller logs for `skipping ReferenceGrant watch` too.

The controller's `ClusterRole` grants `get`/`list`/`watch` on `referencegrants`. Apply the
updated RBAC (Helm chart or manifests) together with the new image. If the CRD is
installed but the controller cannot list ReferenceGrants, it exits at startup with an error
naming the missing permission. Traffic already programmed in VPC Lattice is not affected,
and during a rolling update the previous pods keep running until the RBAC is applied.
