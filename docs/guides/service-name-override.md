# Service Name Override

Override default generated VPC Lattice service names for Routes to avoid conflicts or implement naming conventions.

## Usage

Add the annotation to HTTPRoute or GRPCRoute:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: api-service
  namespace: production
  annotations:
    application-networking.k8s.aws/service-name-override: "custom-service-name"
spec:
  parentRefs:
    - name: my-gateway
  rules:
    - backendRefs:
        - name: backend-service
          port: 80
```

## Before You Start

Service name overrides must be set during route creation and cannot be modified. Adding or changing the annotation on existing routes does not work.

**To add or change a service name override for existing route:**
1. Delete the route: `kubectl delete httproute my-route`  
2. Update annotation and recreate: `kubectl apply -f updated-route.yaml`

## Validation Rules

Service names must:
- Be 3-40 characters long
- Use only lowercase letters, numbers, and hyphens
- Start and end with alphanumeric characters  
- Not start with `svc-` 
- Not contain `--` or start/end with `-`
