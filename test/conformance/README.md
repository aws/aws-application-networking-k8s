# Gateway API Conformance Tests

Runs the upstream [Gateway API conformance suite](https://gateway-api.sigs.k8s.io/concepts/conformance/) against the VPC Lattice controller.

## Prerequisites

- `kubectl` access to an EKS cluster
- Go 1.24+ (newer versions auto-downloaded as needed)
- Helm 3 (if using `INSTALL_CONTROLLER`)
- **Service Network**: The cluster's VPC must have a Service Network VPC Association (SNVA) with the service network used for the tests. Without it, traffic tests fail with connection resets.
  - With `INSTALL_CONTROLLER`: a service network named `conformance-test` must exist
  - Without: uses whatever service network the locally running controller is configured with

## Usage

```bash
# Controller already running
GATEWAY_API_VERSION=v1.4.0 CONTROLLER_VERSION=v2.0.1 make conformance-test

# Install controller via Helm
GATEWAY_API_VERSION=v1.4.0 CONTROLLER_VERSION=v2.0.1 INSTALL_CONTROLLER=true make conformance-test

# Manual cleanup
make conformance-clean
```

Report saved to `conformance-report.yaml`.

## Running with Local Controller

In one terminal, run the controller from source:

```bash
AWS_EC2_METADATA_DISABLED=false \
AWS_REGION=us-west-2 \
CLUSTER_VPC_ID=<vpc-id> \
AWS_ACCOUNT_ID=<account-id> \
CLUSTER_NAME=<cluster-name> \
DEFAULT_SERVICE_NETWORK=conformance-test \
ENABLE_SERVICE_NETWORK_OVERRIDE=true \
DEV_MODE=1 \
LOG_LEVEL=debug \
go run cmd/aws-application-networking-k8s/main.go
```

In another terminal:

```bash
GATEWAY_API_VERSION=v1.5.1 CONTROLLER_VERSION=v2.0.1 make conformance-test
```

## Files

- `conformance_test.go` — Test wrapper configuring timeouts, skip list, and Lattice settings
- `skip-tests.yaml` — Skipped tests with categories: `architectural-limitation`, `lattice-feature-limitation`, `controller-bug`
- `patches/001-gateway-address.patch` — Patches gateway address check for Lattice DNS model

## Updating the Patch

If the patch fails on a new Gateway API version, regenerate it:

```bash
git clone --branch <version> --depth 1 https://github.com/kubernetes-sigs/gateway-api.git
cd gateway-api
# Edit conformance/utils/kubernetes/helpers.go
git diff > ../test/conformance/patches/001-gateway-address.patch
```
