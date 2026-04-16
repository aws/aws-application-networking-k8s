# ServiceNetwork API Reference

## Introduction

ServiceNetwork is a cluster scoped Custom Resource Definition (CRD) that manages the lifecycle of VPC Lattice Service Networks.
It provides a Kubernetes native way to create and delete service networks, as an alternative to the [`DEFAULT_SERVICE_NETWORK`](../guides/environment.md#default_service_network) environment variable.

The ServiceNetwork CRD only manages service network creation and deletion. VPC association is managed by
[VpcAssociationPolicy](vpc-association-policy.md) and IAM auth is managed by [IAMAuthPolicy](iam-auth-policy.md).

### Prerequisites

The ServiceNetwork CRD is optional. To use it, install the CRD:

```bash
kubectl apply -f config/crds/bases/application-networking.k8s.aws_servicenetworks.yaml
```

If the CRD is not installed, the controller will start normally and skip ServiceNetwork functionality.

### Key Behaviors

- **Cluster scoped**: Matches VPC Lattice's account level resource model. The service network name comes from `.metadata.name`.
- **Adoption**: If a Lattice service network with the same name already exists, the controller adopts it by adding a `ManagedBy` tag.
- **Self healing**: If the Lattice service network is deleted out of band, the controller re-creates it on the next reconcile.
- **Backward compatible**: Gateway works with or without a ServiceNetwork CR. [`DEFAULT_SERVICE_NETWORK`](../guides/environment.md#default_service_network) continues to work.

### Deletion Behavior

The controller uses a finalizer to ensure Lattice resources are cleaned up before the CR is removed. Deletion is blocked if:

- A Gateway with the same name exists and is controlled by the Lattice gateway controller.
- The Lattice service network still has active associations (VPC, service, resource, or endpoint). The Lattice API error message is surfaced in the CR status.

### User Workflow

1. Create a ServiceNetwork CR (creates the service network in Lattice).
2. Create a Gateway with a matching name.
3. Optionally create a VpcAssociationPolicy for VPC association.
4. Optionally create an IAMAuthPolicy for auth.

## Example Configuration

```yaml
apiVersion: application-networking.k8s.aws/v1alpha1
kind: ServiceNetwork
metadata:
  name: my-network
  annotations:
    application-networking.k8s.aws/tags: "Environment=Dev,Team=Platform"
```
