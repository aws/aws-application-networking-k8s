# VpcAssociationPolicy API Reference

## Introduction

VpcAssociationPolicy is a Custom Resource Definition (CRD) that can be attached to a Gateway to define the configuration
of the ServiceNetworkVpcAssociation between the Gateway's associated VPC Lattice Service Network and the cluster VPC.

The policy controls whether the association exists (`associateWithVpc`), the security groups it carries
(`securityGroupIds`) and whether VPC Lattice provisions private DNS in the cluster VPC for the custom domain names of
resource configurations associated with the service network (`privateDnsEnabled`, `dnsOptions`).

### Fields

| Field | Applied by VPC Lattice | Notes |
|---|---|---|
| `targetRef` | - | Must reference a `Gateway` in the same namespace as the policy. |
| `associateWithVpc` | Any time | Considered `true` when unset. `false` deletes the association created by the controller. |
| `securityGroupIds` | On creation and in place | Cannot be removed down to an empty list, see [Recreating the VPC association](#recreating-the-vpc-association). |
| `privateDnsEnabled` | **On creation only** | Unset means the association is created with private DNS disabled (the VPC Lattice default) and the setting is not compared afterwards. |
| `dnsOptions.privateDnsPreference` | **On creation only** | Requires `privateDnsEnabled: true`. Defaults to `VERIFIED_DOMAINS_ONLY` on the VPC Lattice side. |
| `dnsOptions.privateDnsSpecifiedDomains` | **On creation only** | 1 to 10 names, wildcards allowed. Required for `VERIFIED_DOMAINS_AND_SPECIFIED_DOMAINS` and `SPECIFIED_DOMAINS_ONLY`, rejected for the other preferences. |

The full schema is documented in the [API reference](../api-reference.md#application-networking.k8s.aws/v1alpha1.VpcAssociationPolicySpec).

### Recommended Security Group Inbound Rules

| Source                                                  | Protocol                                            | Port Range                                      | Comment                                                   |
|---------------------------------------------------------|-----------------------------------------------------|-------------------------------------------------|-----------------------------------------------------------|
| Kubernetes cluster VPC CIDR or security group reference | Protocols defined in the gateway's listener section | Ports defined in the gateway's listener section | Allow inbound traffic from current cluster vpc to gateway |

### Limitations and Considerations

When attaching a VpcAssociationPolicy to a resource, the following restrictions apply:

* Policies must be attached to *Gateway* resource.
* The attached resource must exist in the same namespace as the policy resource.

The security group and private DNS settings will not take effect if:

* The `targetRef` gateway does not exist.
* The `associateWithVpc` field is set to false.

Additionally:

* On a RAM-shared service network the controller creates a missing association with the policy's settings, but it
  never updates security groups or tags of an existing association, and never updates private DNS settings of or
  deletes an association it did not create.
* The association created at controller start-up for `DEFAULT_SERVICE_NETWORK` is created without private DNS.

### Private DNS for custom domain names

When a service network carries [resource configurations](https://docs.aws.amazon.com/vpc-lattice/latest/ug/resource-configuration.html)
of type domain name or IP address with a custom domain name, VPC Lattice provisions the Route 53 private hosted zone for
that name in a consumer VPC only when the VPC association has private DNS enabled. Without it the custom name does not
resolve from the cluster, while the VPC Lattice generated names still work. Resource configurations of type ARN (for
example RDS databases) get their hosted zones regardless of this setting.

!!! note "Both sides must enable private DNS"
    The service network owner must also enable private DNS on the service network *resource* association. Enabling it on
    the VPC association alone does not create a hosted zone.

    The hosted zone is created by the VPC Lattice service-linked role. The controller needs no Route 53 permissions.

Set `privateDnsEnabled: true` and, optionally, `dnsOptions` to choose which domain names get a private hosted zone:

| `privateDnsPreference` | Private hosted zones are provisioned for |
|---|---|
| `VERIFIED_DOMAINS_ONLY` (default, recommended by AWS) | custom domain names verified by the resource owner |
| `ALL_DOMAINS` | all custom domain names on the service network |
| `VERIFIED_DOMAINS_AND_SPECIFIED_DOMAINS` | verified custom domain names plus the names listed in `privateDnsSpecifiedDomains` |
| `SPECIFIED_DOMAINS_ONLY` | only the names listed in `privateDnsSpecifiedDomains` |

`privateDnsSpecifiedDomains` accepts fully qualified names such as `db.example.com` or wildcards such as
`*.example.com`. The API server rejects a policy that sets `dnsOptions` without `privateDnsEnabled: true`, that lists
domains with a preference other than the two `*_SPECIFIED_DOMAINS*` values, or that uses one of those preferences
without domains. See the
[VPC Lattice documentation](https://docs.aws.amazon.com/vpc-lattice/latest/ug/resource-configuration.html#custom-domain-name-resource-consumers)
for the semantics of each preference.

!!! warning "Changes apply to the existing association"
    If the association already exists with different settings, the controller updates it in place, including when the
    settings were changed outside the controller: a policy with `privateDnsEnabled: false` turns off private DNS that
    was enabled by other means. Private hosted zones are added to or removed from the cluster VPC accordingly, which
    changes how those names resolve.

    When `privateDnsEnabled` is unset nothing is sent or compared. When `privateDnsPreference` is unset the preference
    of an existing association is not compared, so removing `dnsOptions` does not reset it: set the preference you want
    explicitly.

On a RAM-shared service network the controller only updates associations it created. Any other association is left
unchanged: the `Programmed` condition is set to `False` with reason `PrivateDnsMismatch` and a `Warning` event is emitted
with the same message. Update the association out of band, or align the policy with it.

### Recreating the VPC association

Removing all security groups cannot be applied in place by the VPC Lattice `UpdateServiceNetworkVpcAssociation` API: if
you have a VpcAssociationPolicy attached to a gateway that already has security groups applied, updating the
VpcAssociationPolicy with empty security group ids or deleting the VpcAssociationPolicy will NOT remove the security
groups from the gateway.

To apply such a change, delete the VPC association and re-create it by following these steps:

1. Update the VpcAssociationPolicy by setting `associateWithVpc` to `false`, together with the desired security group
   ids.
2. Wait until the association is gone: `kubectl describe vap <name>` shows the `Programmed` condition `True` with the
   message `No VPC association is managed`, and `aws vpc-lattice list-service-network-vpc-associations --vpc-identifier <vpc-id>`
   no longer lists the association.
3. Update the VpcAssociationPolicy by setting `associateWithVpc` back to `true`.

Note: Setting `associateWithVpc` to false will disable traffic from the current cluster workloads to the gateway until the
association is recreated.

On a RAM-shared service network the controller only deletes associations it created.

### Upgrading the CRD

`privateDnsEnabled` and `dnsOptions` require the VpcAssociationPolicy CRD shipped with the release that introduced
them. Helm does not upgrade the CRDs in the chart's `crds/` directory, so after `helm upgrade` apply it explicitly from
the release tag:

```bash
kubectl apply -f https://raw.githubusercontent.com/aws/aws-application-networking-k8s/<release-tag>/config/crds/bases/application-networking.k8s.aws_vpcassociationpolicies.yaml
kubectl explain vpcassociationpolicy.spec.privateDnsEnabled
```

The controller logs a warning at start-up when the installed CRD does not define `spec.privateDnsEnabled`.
With a stale CRD, `kubectl apply` rejects the policy with `unknown field "spec.privateDnsEnabled"`. Clients that skip
field validation (Helm-rendered resources, GitOps tools, raw API clients) get the fields pruned instead: `kubectl get vap
<name> -o yaml` shows no `privateDnsEnabled` and the association keeps private DNS disabled. Installations that apply the release `deploy.yaml` get the CRD with it.

## Status

`kubectl get vap` shows the `PrivateDNS` value from the spec and the status of the `Programmed` condition;
`kubectl describe vap <name>` shows the reason, the message and the events. The policy reports two conditions:

| Type | Status / Reason | Meaning |
|---|---|---|
| `Accepted` | `True` / `Accepted` | The `targetRef` is valid and this policy wins conflict resolution. Other reasons follow the Gateway API policy attachment conventions (`Invalid`, `TargetNotFound`, `Conflicted`). |
| `Programmed` | `True` / `Programmed` | The association is active and matches the policy (on a RAM-shared service network only the private DNS settings are compared), or `associateWithVpc` is `false` and no association is managed. |
| `Programmed` | `Unknown` / `Pending` | VPC Lattice is still creating, updating or deleting the association, the service network does not exist yet, or a transient VPC Lattice error (throttling, internal error, conflict) is being retried. |
| `Programmed` | `Unknown` / `NotAccepted` | The policy is not accepted; see the `Accepted` condition. Nothing is created or deleted for it. |
| `Programmed` | `False` / `PrivateDnsMismatch` | An association the controller did not create on a RAM-shared service network has private DNS settings that differ from the policy. The message lists the difference. |
| `Programmed` | `False` / `ReconcileFailed` | The last create, update or delete failed: a VPC Lattice error (for example `ValidationException`), or an existing association that is not owned by the controller. The message carries the cause. |

Example after changing `dnsOptions` on a policy whose association was created outside the controller on a RAM-shared
service network:

```yaml
status:
  conditions:
  - type: Accepted
    status: "True"
    reason: Accepted
  - type: Programmed
    status: "False"
    reason: PrivateDnsMismatch
    message: 'VPC association arn:aws:vpc-lattice:...:servicenetworkvpcassociation/snva-0123 has private DNS settings that differ from this policy: dnsOptions.privateDnsPreference (policy: ALL_DOMAINS, association: VERIFIED_DOMAINS_ONLY). The controller only changes private DNS settings of associations it created on a RAM-shared service network. ...'
```

## Example Configuration

This configuration attaches a policy to the Gateway, `default/my-hotel`. The ServiceNetworkVpcAssociation between the
Gateway's corresponding VPC Lattice Service Network and the cluster VPC is updated based on the policy contents.

If the expected ServiceNetworkVpcAssociation does not exist, it is created since `associateWithVpc` is set to `true`.
This allows traffic from clients in the cluster VPC to VPC Lattice Services in the associated Service Network.
Additionally, two security groups (`sg-1234567890` and `sg-0987654321`) are attached to the ServiceNetworkVpcAssociation.

```
apiVersion: application-networking.k8s.aws/v1alpha1
kind: VpcAssociationPolicy
metadata:
    name: test-vpc-association-policy
spec:
    targetRef:
        group: "gateway.networking.k8s.io"
        kind: Gateway
        name: my-hotel
    securityGroupIds:
        - sg-1234567890
        - sg-0987654321
    associateWithVpc: true
```

### Example: private DNS for a RAM-shared service network

This policy makes the controller create the association to the (RAM-shared) service network behind `my-hotel` with
private DNS enabled, so that the verified custom domain names of the resource configurations on that network, plus
`*.internal.example.com` and `db.example.com`, resolve from the cluster VPC.

```
apiVersion: application-networking.k8s.aws/v1alpha1
kind: VpcAssociationPolicy
metadata:
    name: shared-network-private-dns
spec:
    targetRef:
        group: "gateway.networking.k8s.io"
        kind: Gateway
        name: my-hotel
    associateWithVpc: true
    privateDnsEnabled: true
    dnsOptions:
        privateDnsPreference: VERIFIED_DOMAINS_AND_SPECIFIED_DOMAINS
        privateDnsSpecifiedDomains:
            - "*.internal.example.com"
            - db.example.com
```
