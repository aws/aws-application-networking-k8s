# Frequently Asked Questions (FAQ)



**How can I get involved with Amazon VPC Lattice Gateway API Controller?**

We welcome general feedback, questions, feature requests, or bug reports by creating a [Github issue](https://github.com/aws/aws-application-networking-k8s/issues/new).

**Where can I find Amazon VPC Lattice Gateway API Controller releases?**

The Amazon VPC Lattice Gateway API Controller releases are tags of the Github repository. The [Github releases page](https://github.com/aws/aws-application-networking-k8s/releases) shows all the releases.

**Which EKS CNI versions are supported?**

Your AWS VPC CNI must be v1.8.0 or later to work with VPC Lattice.

**Which versions of Gateway API are supported?**

The Amazon VPC Lattice Gateway API Controller supports Gateway API CRD bundle versions `v1.1` or greater. Not all features of Gateway API are supported - for detailed features and limitation, please refer to individual API references. Please note that users are required to install Gateway API CRDs themselves as these are no longer bundled as of release `v1.1.0`. The latest Gateway API CRDs are available [here](https://gateway-api.sigs.k8s.io/). Please [follow this installation](https://gateway-api.sigs.k8s.io/guides/#installing-gateway-api) process.

**How do health checks work in multi-cluster deployments?**

In multi-cluster deployments, when you apply a TargetGroupPolicy to a ServiceExport, the health check configuration is automatically propagated to all target groups across all clusters that participate in the service mesh. This ensures consistent health monitoring behavior regardless of which cluster contains the route resource.

## Standalone VPC Lattice Services

**What are standalone VPC Lattice services?**

Standalone VPC Lattice services are services created without automatic service network association. They provide more flexibility for independent service management, selective service network membership, and integration with external systems. Use the `application-networking.k8s.aws/standalone: "true"` annotation on Gateway or Route resources to enable this mode.

**Why is my standalone service not accessible from other services?**

Standalone services are not automatically discoverable through service network DNS resolution. To enable communication:

1. **Use the VPC Lattice assigned DNS name** from the route annotation:
   ```bash
   kubectl get httproute my-route -o jsonpath='{.metadata.annotations.application-networking\.k8s\.aws/lattice-assigned-domain-name}'
   ```

2. **Manually associate the service with a service network** using AWS CLI:
   ```bash
   SERVICE_ARN=$(kubectl get httproute my-route -o jsonpath='{.metadata.annotations.application-networking\.k8s\.aws/lattice-service-arn}')
   SERVICE_ID=$(echo "$SERVICE_ARN" | cut -d'/' -f2)
   aws vpc-lattice create-service-network-service-association \
     --service-network-identifier "sn-12345678901234567" \
     --service-identifier "$SERVICE_ID"
   ```

**How do I transition between standalone and service network modes?**

To transition from service network to standalone mode:
```bash
kubectl annotate httproute my-route application-networking.k8s.aws/standalone=true
```

To transition from standalone to service network mode:
```bash
kubectl annotate httproute my-route application-networking.k8s.aws/standalone-
```

The controller handles transitions gracefully without service disruption.

**Why isn't my route-level annotation working?**

Check the annotation precedence:

1. **Route-level annotations** override Gateway-level annotations
2. **Gateway-level annotations** apply to all routes referencing that gateway
3. **Invalid annotation values** (anything other than "true" or "false") are treated as "false"

Verify your annotation syntax:
```bash
kubectl get httproute my-route -o yaml | grep -A5 -B5 standalone
```

**How do I access the VPC Lattice service ARN for AWS RAM sharing?**

The service ARN is automatically populated in the route annotations:

```bash
# Get service ARN
SERVICE_ARN=$(kubectl get httproute my-route -o jsonpath='{.metadata.annotations.application-networking\.k8s\.aws/lattice-service-arn}')

# Use for RAM sharing
aws ram create-resource-share \
  --name "shared-lattice-service" \
  --resource-arns "$SERVICE_ARN" \
  --principals "123456789012"
```

**Can I use standalone services with existing policies?**

Yes, all existing policies (IAMAuthPolicy, TargetGroupPolicy, AccessLogPolicy, VpcAssociationPolicy) work normally with standalone services. The only difference is the lack of automatic service network association.

**What happens if I have conflicting annotations on Gateway and Route?**

Route-level annotations always take precedence over Gateway-level annotations. For example:

- Gateway has `standalone: "true"`
- Route has `standalone: "false"`
- Result: The route creates a service network associated service

**Why don't I see the service ARN annotation immediately?**

The service ARN annotation is populated after the VPC Lattice service is successfully created. This typically takes 30-60 seconds. Check the route status and controller logs if the annotation doesn't appear within a few minutes.

**Can standalone services communicate across VPCs?**

Standalone services require explicit configuration for cross-VPC communication through **AWS RAM sharing** to share the service with other accounts/VPCs. Service network associated services automatically handle cross-VPC communication within the same service network.

**How do I prevent 503 errors during deployments?**

When using the Amazon VPC Lattice Gateway API Controller with EKS, customers may experience 503 errors during deployments due to a timing gap between pod termination and VPC Lattice configuration propagation, which affects the time controller takes to deregister a terminating pod. We recommend setting `terminationGracePeriod` to at least 150 seconds and implementing a preStop hook that has a sleep of 60 seconds (but no more than the `terminationGracePeriod`). For optimal performance, also consider setting `ROUTE_MAX_CONCURRENT_RECONCILES` to 10 which further accelerates the pod deregistration process, regardless of the number of targets.

## Troubleshooting

**Why do I see "vpcId is not specified: EC2MetadataError" when the controller starts?**

This error occurs when the controller cannot access the EC2 Instance Metadata Service (IMDS):

```json
{
    "level": "fatal",
    "ts": "2025-08-24T00:57:37.700Z",
    "logger": "setup",
    "caller": "runtime/proc.go:272",
    "msg": "init config failed: vpcId is not specified: EC2MetadataError: failed to make EC2Metadata request\n\n\tstatus code: 401, request id: "
}
```

**Root Cause**: EKS Managed Node Groups use IMDSv2 with a default HTTP PUT response hop limit of 1. This restricts pod access to the EC2 metadata service because containerized workloads require an additional network hop.

**Solution**: Increase the HTTP PUT response hop limit to 2 on your EKS nodes. You can do this in several ways:

1. **Modify existing instances** (temporary fix):
   ```bash
   aws ec2 modify-instance-metadata-options \
       --instance-id <instance-id> \
       --http-put-response-hop-limit 2 \
       --http-endpoint enabled
   ```

2. **Use a Launch Template** (recommended for new node groups):
   Create or update your Launch Template with the following metadata options:
   ```yaml
   MetadataOptions:
     HttpEndpoint: enabled
     HttpTokens: required
     HttpPutResponseHopLimit: 2
   ```

3. **Using eksctl** when creating a node group:
   ```yaml
   managedNodeGroups:
     - name: my-nodegroup
       instanceType: m5.large
       desiredCapacity: 2
       disableIMDSv1: true
       # eksctl automatically sets hop limit to 2 for managed node groups
   ```

For more information, see the [AWS documentation on configuring IMDS](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/configuring-instance-metadata-options.html).

**Why can't the controller discover the VPC ID automatically?**

The controller attempts to discover the VPC ID from EC2 instance metadata. If this fails (see the IMDS troubleshooting above), you can explicitly set the VPC ID using the `CLUSTER_VPC_ID` environment variable in the controller deployment:

```yaml
env:
  - name: CLUSTER_VPC_ID
    value: "vpc-0123456789abcdef0"
```

This bypasses the need for IMDS access for VPC discovery.
