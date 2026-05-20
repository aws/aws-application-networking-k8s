# Drift Detection

The AWS Gateway API Controller can periodically re-reconcile resources to detect and recover from out-of-band changes to VPC Lattice resources. This is useful when Lattice resources are modified or deleted outside of Kubernetes, for example via the AWS Console, CLI, or other automation.

## How it works

By default, the controller only reconciles when Kubernetes objects change (create, update, delete). If a VPC Lattice resource is modified or deleted out-of-band, the controller has no way to detect the change because nothing changed in Kubernetes.

When drift detection is enabled, the controller schedules a periodic re-reconciliation after each successful reconcile. On each pass, the controller compares the desired state (defined in Kubernetes) against the live state in VPC Lattice and restores any discrepancies. This covers:

- **Services** — recreated if deleted out-of-band
- **Listeners** — recreated if deleted out-of-band
- **Rules** — recreated if deleted, reverted if modified out-of-band
- **Target groups** — recreated if deleted out-of-band
- **Targets** (pod registrations) — re-registered if deregistered out-of-band
- **Service network service associations** — restored if removed
- **Access log subscriptions** — restored if removed
- **IAM auth policies** — restored if deleted, `AWS_IAM` auth type re-applied if reverted
- **VPC associations** — recreated if deleted, security-group drift reverted

The following resources are **not** currently covered by drift detection:

- **Service networks** — the gateway controller checks for existence but does not create or modify service networks

A random jitter of 0-20% is added to each requeue interval to prevent all resources from reconciling at the same instant (thundering herd), particularly at controller startup.

## Configuration

Set the `RECONCILE_DEFAULT_RESYNC_SECONDS` environment variable to enable drift detection. The value is the interval in seconds between periodic reconciliations. The minimum value is 60 seconds.

See [Configuration](environment.md#reconcile_default_resync_seconds) for details on setting this variable.

### Helm

```bash
helm install gateway-api-controller \
    oci://public.ecr.aws/aws-application-networking-k8s/aws-gateway-controller-chart \
    --set=reconcileDefaultResyncSeconds=300
```

Or in `values.yaml`:

```yaml
reconcileDefaultResyncSeconds: 300
```

### Non-Helm (deploy.yaml)

Add the environment variable to the controller container in `config/manager/manager.yaml`:

```yaml
env:
  - name: RECONCILE_DEFAULT_RESYNC_SECONDS
    value: "300"
```

## Important considerations

### DNS domain changes on service recreation

When a VPC Lattice service is deleted and recreated by drift detection, the new service receives a different ID and a new auto-generated DNS domain name. Any external DNS records (such as CNAME or alias records) pointing to the old Lattice-generated domain will need to be updated to point to the new domain.

If you use custom domain names with your Lattice services, the custom domain configuration will be restored by the controller, but the underlying Lattice-generated domain will still change.

### RAM sharing

Cross-account RAM sharing associations tied to the original service will not carry over to a recreated service. If your setup relies on RAM sharing, be aware that a service recreation will require re-establishing the sharing configuration.

### API usage

Each periodic reconciliation makes multiple VPC Lattice API calls per resource. For clusters with many routes or policies, shorter intervals will result in higher API call volume. Choose an interval that balances recovery time against API usage for your environment.

Most managers diff the live state against the desired state on each pass and skip mutation calls when nothing has drifted. In the common case where a resource is already in sync, a periodic reconcile makes only read calls (e.g. `GetAuthPolicy`, `GetServiceNetworkVpcAssociation`); the corresponding `Put`/`Update` calls only fire when drift is detected. There are two known exceptions where a mutation fires on every pass regardless of drift: target registration (the route controller calls `RegisterTargets` with the full desired set each time) and access log subscriptions (the access log policy controller calls `UpdateAccessLogSubscription` once the source matches). These are pre-existing patterns and may be tightened in follow-up work.

### Controller coverage

Drift detection applies to all controllers that use the shared `HandleReconcileError` function. The route controller provides the most comprehensive coverage, reconciling services, listeners, rules, target groups, targets, and service network associations on each pass. The access log policy controller restores access log subscriptions. The IAM auth policy controller restores deleted auth policies and re-applies the `AWS_IAM` auth type if reverted. The VPC association policy controller recreates the VPC association if deleted and reverts security-group drift.

The gateway controller receives periodic requeues but only verifies service network existence — it does not create or modify service networks.

### Policy controllers and target-resource lifecycle

Drift detection on the IAM auth policy and VPC association policy controllers operates on the policy-managed Lattice resource (auth policy, SNVA). It does not extend to the underlying service network or service that the policy targets — those are owned by other controllers or by the user. If the target resource is deleted out-of-band, the policy controller will surface a status condition rather than recreate the target.
