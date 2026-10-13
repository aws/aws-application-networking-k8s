# Migrate to EKS with weighted routing

This guide shows how to migrate a VPC Lattice service's targets from EC2, ECS, ALB, or Lambda to EKS. The controller registers your EKS pods as a new target group on the existing service, and you shift weighted traffic from the old target group to the EKS one in steps you control. The service keeps the same URL and DNS records throughout. This guide covers the two annotations that make this work, a pre-flight checklist, a worked example for each route type, the limitations to understand first, and how to read the route status when something is wrong. It does not cover cross-account migration, which needs additional IAM and RAM configuration.

VPC Lattice can split traffic across several target groups on one listener rule, and those target groups can point at any compute type. The controller adds an EKS target group alongside your existing one and shifts the weights over time. You keep control of the existing target group the whole way, because the controller never modifies it.

```
   Before                  During migration            After - Cutover done

 service "orders"          service "orders"             service "orders"
        |                    |          |                      |
        v                    v          v                      v
  external TG @100     external TG @90  EKS TG @10         EKS TG @100
  (EC2/ECS/ALB/                                       
   Lambda)                                             
```

## How the migration works

Two annotations carry the feature, and you need both. The first, `application-networking.k8s.aws/target-group-arn`, goes on a `ServiceImport` and references your existing target group by its ARN. The controller treats the referenced target group as read only. It never tags it, registers targets on it, or deletes it, so you keep full ownership of that resource during the migration and after it.

The second annotation, `application-networking.k8s.aws/service-name-override`, goes on the Route and tells the controller to take over the existing Lattice service of that name instead of creating a new one. The takeover preserves the service's DNS name, its service network association, and its listeners, which is what lets the migrated workload keep the same URL. The rest of this guide refers to this as the controller claiming the service.

The examples below keep the Gateway and Route in the same namespace, the simplest setup, because the Route's `parentRefs` defaults to its own namespace. To split them across namespaces, set `parentRefs[].namespace` on the Route and allow its namespace in the Gateway listener's `allowedRoutes`.

## Read this before you start

Three behaviors are destructive or irreversible once the controller claims an existing service. Read all three before you run any command, because each can damage a production service.

**Claiming a service is permanent, and deleting the Route deletes the service.** On the first reconcile the controller stamps the `application-networking.k8s.aws/ManagedBy` tag on your Lattice service and owns it from then on. The Route's lifecycle becomes the service's: deleting the Route deletes the service (the external target group survives). Delete the Route only when you intend to destroy the service itself. The delete time safety net only protects services that are still untagged, so it will not save one you have already claimed.

**The Route and Gateway become the source of truth for the service's rules and listeners.** Once the controller claims a service, the Route's rules define its rules and the Gateway's listeners define its listeners. On every reconcile the controller deletes any rule or listener on the claimed service that the Route and Gateway do not define. Reproduce every rule you want to keep, and make sure your Gateway covers every listener on the service. Match each Gateway listener's protocol as well as its port, because the controller reuses an existing listener by port alone but deletes by the full port and protocol tuple. See the multi-listener example below.

**A deploy failure after the claim step leaves the service half claimed.** The controller deploys in this order: target group, then targets, then the service claim (where it stamps the ownership tag), then listeners, then rules. A failure at the rule step, such as a quota breach, leaves the service already claimed and any listener the Gateway does not define already deleted, with no automatic revert. Check your quotas before you apply to avoid this.

## Pre-flight checklist

Run these checks before you apply anything, to avoid leaving the service in a broken, partly migrated state.

First, confirm the environment. The Lattice service must live in the same AWS account as the EKS cluster. The service must already be associated with the cluster's VPC through a Service Network VPC Association (SNVA). The service must not already carry the controller's `ManagedBy` tag, because that would mean another controller owns it. Point the EKS Gateway at the same service network your existing service already uses. Claiming a service adds the Gateway's service network to it without removing the one it already has, so a Gateway on a different service network would leave the service associated with two service networks at once. Set your region with `export AWS_REGION=<cluster_region>`, and install the controller first if you have not (see [Controller Installation](deploy.md)).

Next, note your existing service network, service, listeners, and rules. Your Gateway must cover every listener and your Route must reproduce every rule you want to keep, because the controller deletes any the Route and Gateway do not define. Confirm your existing rules use match types the controller supports, because an unsupported match fails the whole Route. The controller supports `Exact` and `PathPrefix` path matches and `Exact` header matches (up to five per rule). It does not support query-parameter matches.

Then check the service against the limits below. The controller does not pre-validate these, so a breach surfaces as a Lattice API error during deploy, after the service is already claimed. If any projected total exceeds the default, request a quota increase first. Confirm the current values for your account in the Service Quotas console.

Project what the service will hold after you apply your Route, because migration roughly doubles its target groups. Each migrating rule that had one backend now forwards to two target groups: the external one plus the new EKS one. So a listener with N rules goes from N to 2N target groups while you run weighted, and the service total grows by one target group per EKS Service backend you add.

| Resource | Default limit |
|---|---|
| Rules per listener | 10 |
| Listeners per service | 2 |
| Target groups per service | 10 |
| Targets per target group | 1000 |

Finally, read the [Limitations](#limitations) before you start.

## Example 1: service with single listener and single rule

The simplest shape is a service with one listener and one rule that forwards to a single target group. The example adds the EKS workload as a second backend at 10 percent and claims the service named `orders`. Apply it, confirm the split, then raise the EKS weight in steps.

```
Before                              After
service "orders"                    service "orders"
  listener :80                        listener :80
    / -> legacy TG @100                 / -> legacy TG @90 + EKS @10
```

First apply a Gateway.

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: my-service-network
  namespace: production
spec:
  gatewayClassName: amazon-vpc-lattice
  listeners:
    - name: http
      protocol: HTTP
      port: 80
```

```bash
kubectl get gateway my-service-network -n production   # wait for PROGRAMMED=True
```

Next, create a `ServiceImport` that references your existing target group by ARN.

```yaml
apiVersion: application-networking.k8s.aws/v1alpha1
kind: ServiceImport
metadata:
  name: orders-legacy-tg
  namespace: production
  annotations:
    application-networking.k8s.aws/target-group-arn: "arn:aws:vpc-lattice:us-east-2:123456789012:targetgroup/tg-0abc123def4567890"
spec:
  type: ClusterSetIP
  ports:
    - port: 80
      protocol: TCP
```

Finally, apply the `HTTPRoute`. It claims the `orders` service and splits traffic 90/10 between the external target group and the EKS Service.

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: orders
  namespace: production
  annotations:
    application-networking.k8s.aws/service-name-override: "orders"
spec:
  parentRefs:
    - name: my-service-network
  rules:
    - backendRefs:
        - name: orders-legacy-tg
          kind: ServiceImport
          weight: 90
        - name: orders-eks
          kind: Service
          port: 80
          weight: 10
```

The `ServiceImport` references the external target group by ARN; see the [ServiceImport reference](../api-types/service-import.md) for the annotation contract and the placeholder `spec`. On the Route, `service-name-override: "orders"` claims the service, and the two weighted backends split traffic in proportion to their weights, so 90 and 10 sends 90 percent to the external target group and 10 percent to EKS.

To shift traffic, edit the weights on the Route and reapply. The rule updates in place, so its id stays stable. When EKS is ready for all traffic, remove the `ServiceImport` backendRef so the rule forwards only to EKS. To roll back at any point, add it back (or raise its weight). The external target group is never touched and stays in your account.

## Example 2: service with single listener and multiple rules

A service that routes by path can migrate each path on its own timeline. Give each path its own rule, its own external target group through a separate `ServiceImport`, and its own weights. The example below migrates `/api/users` and `/api/orders` independently, so one can reach 100 percent on EKS while the other stays at 90/10.

```
Before                              After
service "orders"                    service "orders"
  listener :80                        listener :80
    /api/users  -> users TG @100        /api/users  -> users TG @90 + users EKS @10
    /api/orders -> orders TG @100       /api/orders -> orders TG @90 + orders EKS @10
```

First apply a Gateway.

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: my-service-network
  namespace: production
spec:
  gatewayClassName: amazon-vpc-lattice
  listeners:
    - name: http
      protocol: HTTP
      port: 80
```

Next, create one `ServiceImport` per external target group.

```yaml
apiVersion: application-networking.k8s.aws/v1alpha1
kind: ServiceImport
metadata:
  name: users-legacy-tg
  namespace: production
  annotations:
    application-networking.k8s.aws/target-group-arn: "arn:aws:vpc-lattice:us-east-2:123456789012:targetgroup/tg-0aaa111aaa111aaaa"
spec:
  type: ClusterSetIP
  ports:
    - port: 80
      protocol: TCP
---
apiVersion: application-networking.k8s.aws/v1alpha1
kind: ServiceImport
metadata:
  name: orders-legacy-tg
  namespace: production
  annotations:
    application-networking.k8s.aws/target-group-arn: "arn:aws:vpc-lattice:us-east-2:123456789012:targetgroup/tg-0bbb222bbb222bbbb"
spec:
  type: ClusterSetIP
  ports:
    - port: 80
      protocol: TCP
```

Finally, apply the `HTTPRoute`. Each rule matches a path and splits that path's traffic 90/10 between its external target group and its EKS Service.

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: orders
  namespace: production
  annotations:
    application-networking.k8s.aws/service-name-override: "orders"
spec:
  parentRefs:
    - name: my-service-network
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /api/users
      backendRefs:
        - name: users-legacy-tg
          kind: ServiceImport
          weight: 90
        - name: users-eks
          kind: Service
          port: 80
          weight: 10
    - matches:
        - path:
            type: PathPrefix
            value: /api/orders
      backendRefs:
        - name: orders-legacy-tg
          kind: ServiceImport
          weight: 90
        - name: orders-eks
          kind: Service
          port: 80
          weight: 10
```

Shift and promote each path independently, the same way as Example 1.

## Example 3: service with multiple listeners

When your existing service has more than one listener, use a single Route that sets no `sectionName`. Such a Route attaches to every Gateway listener, which puts every listener in the Route's stack and protects them all from deletion. The Gateway must declare every listener your service has. The controller writes the same rules onto every listener, so this shape fits when your listeners serve the same routing.

```
Before                              After
service "orders"                    service "orders"
  listener :80                        listener :80
    /v1 -> v1 TG @100                   /v1 -> v1 TG @90 + v1 EKS @10
    /v2 -> v2 TG @100                   /v2 -> v2 TG @90 + v2 EKS @10
  listener :8080                      listener :8080
    /v1 -> v1 TG @100                   /v1 -> v1 TG @90 + v1 EKS @10
    /v2 -> v2 TG @100                   /v2 -> v2 TG @90 + v2 EKS @10
```

First apply a Gateway that declares every listener.

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: my-service-network
  namespace: production
spec:
  gatewayClassName: amazon-vpc-lattice
  listeners:
    - name: http
      protocol: HTTP
      port: 80
    - name: http-admin
      protocol: HTTP
      port: 8080
```

Next, create one `ServiceImport` per external target group.

```yaml
apiVersion: application-networking.k8s.aws/v1alpha1
kind: ServiceImport
metadata:
  name: orders-v1-tg
  namespace: production
  annotations:
    application-networking.k8s.aws/target-group-arn: "arn:aws:vpc-lattice:us-east-2:123456789012:targetgroup/tg-0aaa111aaa111aaaa"
spec:
  type: ClusterSetIP
  ports:
    - port: 80
      protocol: TCP
---
apiVersion: application-networking.k8s.aws/v1alpha1
kind: ServiceImport
metadata:
  name: orders-v2-tg
  namespace: production
  annotations:
    application-networking.k8s.aws/target-group-arn: "arn:aws:vpc-lattice:us-east-2:123456789012:targetgroup/tg-0bbb222bbb222bbbb"
spec:
  type: ClusterSetIP
  ports:
    - port: 80
      protocol: TCP
```

Finally, apply a single `HTTPRoute` with no `sectionName`, so it attaches to both listeners.

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: orders
  namespace: production
  annotations:
    application-networking.k8s.aws/service-name-override: "orders"
spec:
  parentRefs:
    - name: my-service-network
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /v1
      backendRefs:
        - name: orders-v1-tg
          kind: ServiceImport
          weight: 90
        - name: orders-v1-eks
          kind: Service
          port: 80
          weight: 10
    - matches:
        - path:
            type: PathPrefix
            value: /v2
      backendRefs:
        - name: orders-v2-tg
          kind: ServiceImport
          weight: 90
        - name: orders-v2-eks
          kind: Service
          port: 80
          weight: 10
```

The controller writes the same rule set onto each listener. Do not split this into two Routes that each pin one listener with `sectionName`. The controller evaluates each Route's listeners on its own, so each Route would delete the listener the other one claims, and the two would delete each other's listener on every reconcile.

## Example 4: gRPC service

A GRPCRoute migration follows the same weighted backend pattern as the HTTP examples, with two protocol differences. Your gRPC listeners on Lattice use the HTTPS protocol, so the Gateway listeners are HTTPS. Lattice has no native gRPC match, so the controller translates a gRPC method match into an HTTP match: it sets the method to `POST` and the path to `/<service>/<method>`, matched case sensitively. Your existing rules must already use that shape.

```
Before                              After
service "echo" (HTTPS)              service "echo" (HTTPS)
  listener :443                       listener :443
    EchoService/V1 -> v1 TG @100        EchoService/V1 -> v1 TG @90 + v1 EKS @10
    EchoService/V2 -> v2 TG @100        EchoService/V2 -> v2 TG @90 + v2 EKS @10
```

First apply a Gateway with HTTPS listeners.

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: my-service-network
  namespace: production
spec:
  gatewayClassName: amazon-vpc-lattice
  listeners:
    - name: https
      protocol: HTTPS
      port: 443
```

Next, create one `ServiceImport` per external target group.

```yaml
apiVersion: application-networking.k8s.aws/v1alpha1
kind: ServiceImport
metadata:
  name: echo-v1-tg
  namespace: production
  annotations:
    application-networking.k8s.aws/target-group-arn: "arn:aws:vpc-lattice:us-east-2:123456789012:targetgroup/tg-0aaa111aaa111aaaa"
spec:
  type: ClusterSetIP
  ports:
    - port: 80
      protocol: TCP
---
apiVersion: application-networking.k8s.aws/v1alpha1
kind: ServiceImport
metadata:
  name: echo-v2-tg
  namespace: production
  annotations:
    application-networking.k8s.aws/target-group-arn: "arn:aws:vpc-lattice:us-east-2:123456789012:targetgroup/tg-0bbb222bbb222bbbb"
spec:
  type: ClusterSetIP
  ports:
    - port: 80
      protocol: TCP
```

Finally, apply the `GRPCRoute`. Each rule matches a gRPC method and splits its traffic 90/10.

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: GRPCRoute
metadata:
  name: echo
  namespace: production
  annotations:
    application-networking.k8s.aws/service-name-override: "echo"
spec:
  parentRefs:
    - name: my-service-network
  rules:
    - matches:
        - method:
            service: echo.EchoService
            method: V1
      backendRefs:
        - name: echo-v1-tg
          kind: ServiceImport
          weight: 90
        - name: echo-v1-eks
          kind: Service
          port: 80
          weight: 10
    - matches:
        - method:
            service: echo.EchoService
            method: V2
      backendRefs:
        - name: echo-v2-tg
          kind: ServiceImport
          weight: 90
        - name: echo-v2-eks
          kind: Service
          port: 80
          weight: 10
```

The EKS target groups here come up with health checks off, because the controller enables them by default only for HTTP/1.1. Attach a [`TargetGroupPolicy`](../api-types/target-group-policy.md) with `healthCheck.enabled: true` to turn them on.

## Example 5: TLS passthrough service

TLS passthrough has no rules, so the weighted forward lives on the listener's default action, and claiming the service updates that default action. TLS passthrough has three setup requirements. Your Lattice service needs a `CustomDomainName`, since Lattice requires one for a TLS passthrough listener. The Route's `spec.hostnames` must be set and match that domain; a TLSRoute without hostnames also cannot be deleted (see the [Limitations](#limitations)). The existing default action must be a forward, not a fixed response.

```
Before                              After
service "orders" (TLS)              service "orders" (TLS)
  listener :443                       listener :443
    default -> legacy TG @100           default -> legacy TG @90 + EKS @10
```

First apply a Gateway with a TLS passthrough listener.

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: my-service-network
  namespace: production
spec:
  gatewayClassName: amazon-vpc-lattice
  listeners:
    - name: tls
      protocol: TLS
      port: 443
      tls:
        mode: Passthrough
```

Next, create a `ServiceImport` for the external target group.

```yaml
apiVersion: application-networking.k8s.aws/v1alpha1
kind: ServiceImport
metadata:
  name: orders-legacy-tg
  namespace: production
  annotations:
    application-networking.k8s.aws/target-group-arn: "arn:aws:vpc-lattice:us-east-2:123456789012:targetgroup/tg-0abc123def4567890"
spec:
  type: ClusterSetIP
  ports:
    - port: 443
      protocol: TCP
```

Finally, apply the `TLSRoute`. It has a single rule with no path match, and `hostnames` must match the service's custom domain name.

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: TLSRoute
metadata:
  name: orders
  namespace: production
  annotations:
    application-networking.k8s.aws/service-name-override: "orders"
spec:
  parentRefs:
    - name: my-service-network
  hostnames:
    - orders.example.com
  rules:
    - backendRefs:
        - name: orders-legacy-tg
          kind: ServiceImport
          weight: 90
        - name: orders-eks
          kind: Service
          port: 443
          weight: 10
```

The EKS target group here comes up with health checks off, because TLS backends are TCP and the controller enables checks by default only for HTTP/1.1. Attach a [`TargetGroupPolicy`](../api-types/target-group-policy.md) with `healthCheck.enabled: true` to turn them on.

## Limitations

These are the limitations to know before you migrate.

- **The controller does not change the default action on HTTP and HTTPS listeners.** Unmatched requests keep going wherever your listener already sent them. If that is a target group, the controller does not manage it.

- **A TLSRoute without `spec.hostnames` cannot be deleted and gets stuck in `Terminating`.** The controller rejects it on every reconcile, including delete, so its finalizer is never removed. Always set `spec.hostnames` (Example 5 shows this). Such a Route never claimed a service, so it is safe to clear its finalizer: `kubectl patch tlsroute <name> -n <ns> --type=merge -p '{"metadata":{"finalizers":[]}}'`.

- **Health checks are off by default on TCP, HTTP/2, and gRPC target groups.** Only HTTP/1.1 enables them automatically. Attach a [`TargetGroupPolicy`](../api-types/target-group-policy.md) with `healthCheck.enabled: true` to turn them on for the other types.

- **The controller creates path matches as case sensitive.** If your existing rules use case insensitive path matching, check them before you migrate, because the claimed rules will start matching case sensitively.

## See also

- [ServiceImport](../api-types/service-import.md)
- [HTTPRoute](../api-types/http-route.md)
- [GRPCRoute](../api-types/grpc-route.md)
- [TLSRoute](../api-types/tls-route.md)
- [TargetGroupPolicy](../api-types/target-group-policy.md)
- [Service Name Override](service-name-override.md)
- [TLS Passthrough](tls-passthrough.md)
- [Controller Installation](deploy.md)
