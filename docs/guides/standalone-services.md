# Standalone VPC Lattice Services

This guide explains how to create standalone VPC Lattice services that are not automatically associated with service networks, providing more flexibility for independent service management scenarios.

## Overview

By default, the AWS Gateway API Controller creates VPC Lattice services and automatically associates them with service networks based on the Gateway configuration. Standalone mode allows you to create VPC Lattice services without this automatic association, enabling:

- Independent service lifecycle management
- Selective service network membership
- Integration with external service discovery systems
- Custom service sharing workflows using AWS RAM

## Configuration

Standalone mode is controlled using the `application-networking.k8s.aws/standalone` annotation, which can be applied to Gateway or Route resources.

### Annotation Placement Options

#### Gateway-Level Configuration

Apply the annotation to a Gateway resource to make all routes referencing that gateway create standalone services:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: my-gateway
  annotations:
    application-networking.k8s.aws/standalone: "true"
spec:
  gatewayClassName: amazon-vpc-lattice
  listeners:
  - name: http
    protocol: HTTP
    port: 80
```

#### Route-Level Configuration

Apply the annotation to individual Route resources for granular control:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: my-route
  annotations:
    application-networking.k8s.aws/standalone: "true"
spec:
  parentRefs:
  - name: my-gateway
  rules:
  - matches:
    - path:
        type: PathPrefix
        value: /
    backendRefs:
    - name: my-service
      port: 80
```

### Annotation Precedence Rules

The controller follows a hierarchical precedence system for annotation processing:

1. **Route-level annotation** (highest precedence) - affects only the specific route
2. **Gateway-level annotation** (lower precedence) - affects all routes referencing the gateway
3. **Default behavior** (no annotation) - services are associated with service networks

#### Example: Mixed Configuration

```yaml
# Gateway with standalone annotation
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: my-gateway
  annotations:
    application-networking.k8s.aws/standalone: "true"  # All routes will be standalone by default
spec:
  gatewayClassName: amazon-vpc-lattice
  listeners:
  - name: http
    protocol: HTTP
    port: 80
---
# Route that inherits gateway setting (standalone)
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: standalone-route
spec:
  parentRefs:
  - name: my-gateway
  rules:
  - backendRefs:
    - name: service-a
      port: 80
---
# Route that overrides gateway setting (service network associated)
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: networked-route
  annotations:
    application-networking.k8s.aws/standalone: "false"  # Overrides gateway setting
spec:
  parentRefs:
  - name: my-gateway
  rules:
  - backendRefs:
    - name: service-b
      port: 80
```

## Service ARN Access

When a VPC Lattice service is created (standalone or networked), the controller automatically populates the route status with the service ARN for integration purposes:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: my-route
  annotations:
    application-networking.k8s.aws/standalone: "true"
    # Output annotations populated by controller:
    application-networking.k8s.aws/lattice-service-arn: "arn:aws:vpc-lattice:us-west-2:123456789012:service/svc-12345678901234567"
    application-networking.k8s.aws/lattice-assigned-domain-name: "my-route-default-12345.67890.vpc-lattice-svcs.us-west-2.on.aws"
spec:
  # ... route configuration
```

### Accessing Service ARN Programmatically

You can retrieve the service ARN using kubectl:

```bash
# Get service ARN for a specific route
kubectl get httproute my-route -o jsonpath='{.metadata.annotations.application-networking\.k8s\.aws/lattice-service-arn}'

# Get both ARN and DNS name
kubectl get httproute my-route -o json | jq -r '.metadata.annotations | {
  "service_arn": ."application-networking.k8s.aws/lattice-service-arn",
  "dns_name": ."application-networking.k8s.aws/lattice-assigned-domain-name"
}'
```

## Use Cases

### Independent Service Management

Create services that can be managed independently of service network membership:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: independent-service
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

### AWS RAM Sharing Integration

Use the service ARN for sharing services across AWS accounts:

```bash
# Extract service ARN
SERVICE_ARN=$(kubectl get httproute my-route -o jsonpath='{.metadata.annotations.application-networking\.k8s\.aws/lattice-service-arn}')

# Create RAM resource share
aws ram create-resource-share \
  --name "shared-lattice-service" \
  --resource-arns "$SERVICE_ARN" \
  --principals "123456789012"  # Target AWS account ID
```

### Selective Service Network Association

Create services first, then selectively associate them with service networks using VPC Lattice APIs:

```bash
# Get service ID from ARN
SERVICE_ID=$(echo "$SERVICE_ARN" | cut -d'/' -f2)

# Associate with service network later
aws vpc-lattice create-service-network-service-association \
  --service-network-identifier "sn-12345678901234567" \
  --service-identifier "$SERVICE_ID"
```

## Manual Service Network Associations

### Important: Protecting Manual Associations

⚠️ **Critical**: If you manually create service network associations outside of the controller (using AWS CLI, Terraform, etc.), you **must** update your Kubernetes manifests to mark the service/route as **not standalone** to prevent the controller from removing your manual associations.

When a service is marked as standalone (`application-networking.k8s.aws/standalone: "true"`), the controller will actively remove any service network associations it finds, including those created manually outside of Kubernetes.

#### Protecting Manual Associations

If you have manually associated a service with a service network:

1. **Remove or set the standalone annotation to false**:
   ```bash
   # Remove the annotation entirely
   kubectl annotate httproute my-route application-networking.k8s.aws/standalone-
   
   # OR set it to false explicitly
   kubectl annotate httproute my-route application-networking.k8s.aws/standalone=false
   ```

2. **Update your YAML manifests** to ensure the annotation is not set to "true":
   ```yaml
   apiVersion: gateway.networking.k8s.io/v1
   kind: HTTPRoute
   metadata:
     name: my-route
     annotations:
       # Either remove the standalone annotation entirely, or set it to false
       application-networking.k8s.aws/standalone: "false"
   spec:
     # ... rest of configuration
   ```

### Manual RAM Sharing Workflow

Follow these steps to manually share a VPC Lattice service using AWS RAM:

#### Step 1: Create Standalone Service

Create a standalone gateway and route that will be shared:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: shared-gateway
  annotations:
    application-networking.k8s.aws/standalone: "true"
spec:
  gatewayClassName: amazon-vpc-lattice
  listeners:
  - name: http
    protocol: HTTP
    port: 80
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: shared-service
  annotations:
    application-networking.k8s.aws/standalone: "true"
spec:
  parentRefs:
  - name: shared-gateway
  rules:
  - matches:
    - path:
        type: PathPrefix
        value: /shared
    backendRefs:
    - name: shared-backend
      port: 8080
```

#### Step 2: Extract Service ARN and Create RAM Share

Wait for the controller to create the service and populate the ARN annotation:

```bash
# Wait for service ARN to be populated
kubectl wait --for=condition=Ready httproute/shared-service --timeout=300s

# Extract the service ARN
SERVICE_ARN=$(kubectl get httproute shared-service -o jsonpath='{.metadata.annotations.application-networking\.k8s\.aws/lattice-service-arn}')

echo "Service ARN: $SERVICE_ARN"

# Create RAM resource share
aws ram create-resource-share \
  --name "shared-lattice-service" \
  --resource-arns "$SERVICE_ARN" \
  --principals "123456789012,987654321098"  # Target AWS account IDs

# Get the resource share ARN for acceptance
SHARE_ARN=$(aws ram get-resource-shares --resource-owner SELF --name "shared-lattice-service" --query 'resourceShares[0].resourceShareArn' --output text)

echo "Resource Share ARN: $SHARE_ARN"
```

#### Step 3: Accept RAM Share (in target account)

In the target AWS account, accept the resource share:

```bash
# List pending invitations
aws ram get-resource-share-invitations --query 'resourceShareInvitations[?status==`PENDING`]'

# Accept the invitation
aws ram accept-resource-share-invitation --resource-share-invitation-arn "arn:aws:ram:us-west-2:123456789012:invitation/12345678-1234-1234-1234-123456789012"
```

#### Step 4: Create Service Network Association (in target account)

In the target account, manually associate the shared service with your service network:

```bash
# Extract service ID from ARN
SERVICE_ID=$(echo "$SERVICE_ARN" | cut -d'/' -f2)

# Associate with your service network
aws vpc-lattice create-service-network-service-association \
  --service-network-identifier "sn-your-service-network-id" \
  --service-identifier "$SERVICE_ID" \
  --tags Key=ManagedBy,Value=Manual Key=SharedFrom,Value=SourceAccount

# Verify the association
aws vpc-lattice list-service-network-service-associations \
  --service-identifier "$SERVICE_ID"
```

#### Step 5: Update Manifest to Protect Manual Association

**Critical Step**: Update your Kubernetes manifest to mark the service as **not standalone** to prevent the controller from removing your manual association:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: shared-service
  annotations:
    # IMPORTANT: Set to false to protect manual associations
    application-networking.k8s.aws/standalone: "false"
    # Document the manual association for team awareness
    manual-association: "true"
    shared-accounts: "123456789012,987654321098"
spec:
  parentRefs:
  - name: shared-gateway
  rules:
  - matches:
    - path:
        type: PathPrefix
        value: /shared
    backendRefs:
    - name: shared-backend
      port: 8080
```

Apply the updated manifest:

```bash
kubectl apply -f shared-service.yaml
```

#### Step 6: Verify Protection

Verify that the controller respects the manual association:

```bash
# Check that the service is no longer marked as standalone
kubectl get httproute shared-service -o jsonpath='{.metadata.annotations.application-networking\.k8s\.aws/standalone}'

# Verify manual associations are preserved
SERVICE_ARN=$(kubectl get httproute shared-service -o jsonpath='{.metadata.annotations.application-networking\.k8s\.aws/lattice-service-arn}')
SERVICE_ID=$(echo "$SERVICE_ARN" | cut -d'/' -f2)

aws vpc-lattice list-service-network-service-associations \
  --service-identifier "$SERVICE_ID"
```

### Best Practices for Manual Associations

1. **Always document manual associations** in annotations or comments
2. **Use consistent tagging** on manual associations for tracking
3. **Monitor for drift** between Kubernetes manifests and actual AWS resources
4. **Implement alerts** for unexpected association changes
5. **Test transitions** in non-production environments first

### Troubleshooting Manual Associations

If your manual associations are being removed:

1. **Check the standalone annotation**:
   ```bash
   kubectl get httproute my-route -o jsonpath='{.metadata.annotations.application-networking\.k8s\.aws/standalone}'
   ```

2. **Review controller logs** for association removal events:
   ```bash
   kubectl logs -n aws-application-networking-system deployment/aws-gateway-controller-manager
   ```

3. **Verify annotation precedence** (route-level overrides gateway-level)

4. **Check for conflicting Gateway annotations** that might be inherited

## Transition Between Modes

### From Service Network to Standalone

Add the standalone annotation to transition an existing service:

```bash
kubectl annotate httproute my-route application-networking.k8s.aws/standalone=true
```

The controller will:
1. Remove existing service network associations
2. Keep the VPC Lattice service running
3. Update the route status

### From Standalone to Service Network

Remove the standalone annotation to enable service network association:

```bash
kubectl annotate httproute my-route application-networking.k8s.aws/standalone-
```

The controller will:
1. Create service network associations based on the gateway configuration
2. Keep the VPC Lattice service running
3. Update the route status

## Compatibility

### Environment Variables

Standalone mode works with the `ENABLE_SERVICE_NETWORK_OVERRIDE` environment variable. When this variable is set, standalone annotations take precedence over the default service network configuration.

### Gateway API Compliance

Standalone services maintain full compatibility with Gateway API specifications:
- Standard Gateway and Route resources are used
- All existing Gateway API features remain available
- Policy attachments (IAMAuthPolicy, TargetGroupPolicy, etc.) work normally

## Best Practices

1. **Use Gateway-level annotations** for consistent behavior across multiple routes
2. **Use Route-level annotations** for granular control when needed
3. **Monitor service ARNs** in route annotations for integration workflows
4. **Plan transitions carefully** to avoid service disruption
5. **Document annotation usage** in your deployment configurations

## Limitations

- Standalone services are not discoverable through service network DNS resolution from the service networks they are disconnected from. Explicit service sharing through RAM and/or association with service networks is required for discoverability.
- Service network policies do not apply to standalone services **only for the specific service networks they are not associated with**. If the same service is manually associated with other service networks (either through different Gateways or manual associations), the policies from those connected service networks will still apply
- Cross-VPC communication requires explicit association with service networks