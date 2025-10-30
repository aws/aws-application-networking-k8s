# Additional Tags

The AWS Gateway API Controller automatically applies some tags to resources it creates. In addition, you can use annotations to specify additional tags.

The `application-networking.k8s.aws/tags` annotation specifies additional tags that will be applied to AWS resources created.

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

## Required IAM Permissions

For the additional tags functionality to work properly, the IAM role linked to the controller's service account must also include these permissions:

```json
{
    "Effect": "Allow",
    "Action": [
        "tag:TagResources",
        "tag:UntagResources",
        "tag:GetResources"
    ],
    "Resource": "*"
}
```

### How to Ensure You Have These Permissions

The `config/iam/recommended-inline-policy.json` file contains all the required permissions including these tagging permissions.

- **If you are setting up for the first time**: The recommended inline policy already includes all the required permissions.
- **If you used the setup steps in the [deploy guide](https://www.gateway-api-controller.eks.aws.dev/latest/guides/deploy/#setup)**: You need to update the existing `VPCLatticeControllerIAMPolicy` to include the updated permissions.


## Supported Resources

- **HTTPRoute** - Tags applied to VPC Lattice Services, Listeners, Rules, Target Groups, and Service Network Service Associations
- **ServiceExport** - Tags applied to VPC Lattice Target Groups
- **AccessLogPolicy** - Tags applied to VPC Lattice Access Log Subscriptions
- **VpcAssociationPolicy** - Tags applied to VPC Lattice Service Network VPC Associations
