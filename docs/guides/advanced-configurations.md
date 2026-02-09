# Advanced configurations

The section below covers advanced configuration techniques for installing and using the Amazon VPC Lattice Gateway API Controller. This includes things such as running the controller on a self-hosted cluster on AWS or using an IPv6 EKS cluster.

### Using a self-managed Kubernetes cluster

You can install the Amazon VPC Lattice Gateway API Controller to a self-managed Kubernetes cluster in AWS.

However, the controller utilizes [IMDS](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ec2-instance-metadata.html) to get necessary information from instance metadata, such as AWS account ID and VPC ID. So:

- **If your cluster is using IMDSv2.** ensure the hop limit is 2 or higher to allow the access from the controller:

    ```bash
    aws ec2 modify-instance-metadata-options --http-put-response-hop-limit 2 --region <region> --instance-id <instance-id>
    ```

- **If your cluster cannot access to IMDS.** ensure to specify the[configuration variables](environment.md) when installing the controller.

### Rule Priority Configuration

You can manually assign priorities to rules using the custom annotation `application-networking.k8s.aws/rule-{index}-priority`. This annotation allows you to explicitly set the priority for specific rules in your route configurations.

For example, to set priorities for multiple rules in an HTTPRoute:

```yaml
apiVersion: gateway.networking.k8s.io/v1beta1
kind: HTTPRoute
metadata:
  name: example-route
  annotations:
    application-networking.k8s.aws/rule-0-priority: "200"  # First rule gets higher priority
    application-networking.k8s.aws/rule-1-priority: "100"  # Second rule gets lower priority
spec:
  rules:
  - matches:                                               # This is rule[0]
    - path:
        type: PathPrefix
        value: /api/v2
  - matches:                                               # This is rule[1]
    - path:
        type: PathPrefix
        value: /api
```

The `{index}` in the annotation corresponds to the zero-based index of the rule in the rules array. In this example:
- `rule-0-priority: "200"` applies to the first rule matching `/api/v2`
- `rule-1-priority: "100"` applies to the second rule matching `/api`

Higher priority values indicate higher precedence, so requests to `/api/v2` will be matched by the first rule (priority 200) before the second rule (priority 100) is considered.

#### Configuring Health Checks for ServiceExport

When you apply a TargetGroupPolicy to a ServiceExport, the health check configuration is automatically propagated to all target groups across all clusters that participate in the service mesh:

```yaml
apiVersion: application-networking.k8s.aws/v1alpha1
kind: TargetGroupPolicy
metadata:
  name: multi-cluster-health-policy
spec:
  targetRef:
    group: "application-networking.k8s.aws"
    kind: ServiceExport
    name: my-service
  healthCheck:
    enabled: true
    intervalSeconds: 10
    timeoutSeconds: 5
    healthyThresholdCount: 2
    unhealthyThresholdCount: 3
    path: "/health"
    port: 8080
    protocol: HTTP
    protocolVersion: HTTP1
    statusMatch: "200-299"
```

### Standalone VPC Lattice Services

You can create VPC Lattice services without automatic service network association using the `application-networking.k8s.aws/standalone` annotation. This provides more flexibility for independent service management scenarios.

For detailed information about standalone services, see the [Standalone VPC Lattice Services](standalone-services.md) guide.

#### Quick Example

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: standalone-api
  annotations:
    application-networking.k8s.aws/standalone: "true"
spec:
  parentRefs:
  - name: my-gateway
  rules:
  - matches:
    - path:
        type: PathPrefix
        value: /api
    backendRefs:
    - name: api-service
      port: 8080
```

The service ARN will be available in the route annotations for integration with external systems:

```bash
kubectl get httproute standalone-api -o jsonpath='{.metadata.annotations.application-networking\.k8s\.aws/lattice-service-arn}'
```

### IPv6 support

IPv6 address type is automatically used for your services and pods if
[your cluster is configured to use IPv6 addresses](https://docs.aws.amazon.com/eks/latest/userguide/cni-ipv6.html).

If your cluster is configured to be dual-stack, you can set the IP address type
of your service using the `ipFamilies` field. For example:

```yaml title="parking_service.yaml"
apiVersion: v1
kind: Service
metadata:
  name: ipv4-target-in-dual-stack-cluster
spec:
  ipFamilies:
    - "IPv4"
  selector:
    app: parking
  ports:
    - protocol: TCP
      port: 80
      targetPort: 8090
```

### Blue/Green Multi-Cluster Migration with Service Takeover

For blue/green cluster migrations, the controller supports automated takeover of VPC Lattice services using the `application-networking.k8s.aws/allow-takeover-from` annotation. The annotation value must match the value of the `application-networking.k8s.aws/ManagedBy` tag on the VPC Lattice Service, which has the format `{AWS_ACCOUNT_ID}/{CLUSTER_NAME}/{VPC_ID}` (e.g., "123456789012/blue-cluster/vpc-0abc123def456789"). This eliminates the need for manual ManagedBy tag changes during cluster migrations.

#### Migration Workflow

1. Blue cluster creates HTTPRoute
2. Blue cluster exports service using ServiceExport (creates standalone target group for cross-cluster access)
3. Green cluster imports blue service using ServiceImport (references the exported target group from blue cluster)
4. Green cluster creates HTTPRoute with takeover annotation to claim the existing VPC Lattice service:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: inventory-service
  annotations:
    application-networking.k8s.aws/allow-takeover-from: "123456789012/blue-cluster/vpc-0abc123def456789"
spec:
  parentRefs:
  - name: my-gateway
  rules:
  - matches:
    - path:
        type: PathPrefix
        value: /inventory
    backendRefs:
    - name: inventory-ver1 
      kind: ServiceImport
      weight: 90
    - name: inventory-ver2
      kind: Service
      port: 80
      weight: 10
```

5. Controller takes over the VPC Lattice service and updates it to reflect traffic weights in green HTTPRoute
6. Controller updates ManagedBy tag on service, service network service association, listeners, and rules to transfer ownership
