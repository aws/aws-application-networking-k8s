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

- Standalone services are not discoverable through service network DNS resolution
- Service network policies do not apply to standalone services
- Cross-VPC communication requires explicit service sharing or alternative connectivity