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

The following resources are **not** currently covered by drift detection:

- **Service networks** — the gateway controller checks for existence but does not create or modify service networks
- **IAM auth policies** — uses a different reconciliation path
- **VPC association policies** — uses a different reconciliation path

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

Each periodic reconciliation makes multiple VPC Lattice API calls per resource (FindService, ListListeners, ListRules, ListTargetGroups, etc.). For clusters with many routes, shorter intervals will result in higher API call volume. Choose an interval that balances recovery time against API usage for your environment.

### Controller coverage

Drift detection applies to all controllers that use the shared `HandleReconcileError` function. The route controller provides the most comprehensive coverage, reconciling services, listeners, rules, target groups, targets, and service network associations on each pass. The access log policy controller also benefits, restoring access log subscriptions.

The gateway controller receives periodic requeues but only verifies service network existence — it does not create or modify service networks. The IAM auth policy and VPC association policy controllers use a different reconciliation path and are not currently covered.
