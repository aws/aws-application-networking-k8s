# Additional Tags

The AWS Gateway API Controller automatically applies some tags to resources it creates. In addition, you can use annotations to specify additional tags.

The `application-networking.k8s.aws/tags` annotation specifies additional tags that will be applied to AWS resources created.

## Supported Resources

- **HTTPRoute** - Tags applied to VPC Lattice Services, Listeners, Rules, Target Groups, and Service Network Service Associations
- **ServiceExport** - Tags applied to VPC Lattice Target Groups
- **AccessLogPolicy** - Tags applied to VPC Lattice Access Log Subscriptions
- **VpcAssociationPolicy** - Tags applied to VPC Lattice Service Network VPC Associations

## Usage

Add comma separated key=value pairs to the annotation:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: inventory-route
  annotations:
    application-networking.k8s.aws/tags: "Environment=Production,Team=Backend"
spec:
  # ... rest of spec
```

```yaml
apiVersion: application-networking.k8s.aws/v1alpha1
kind: ServiceExport
metadata:
  name: payment-service
  annotations:
    application-networking.k8s.aws/tags: "Environment=Production,Service=Payment" 
spec:
  # ... rest of spec
```
