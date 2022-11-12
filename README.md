AWS Application Networking is an implementation of the Kubernetes [Gateway API](https://gateway-api.sigs.k8s.io/). This project is designed to run in a Kubernetes cluster and orchestrates AWS VPC Lattice resources using Kubernetes Custom Resource Definitions like Gateway and HTTPRoute.

### Developer Guide

```bash
# Learn available `make` commands
make help

# This only needs to be run once after checking out the repo, and will install tools/codegen required for development
# If you see this err "Go workspace's "bin" directory is not in PATH. Run 'export PATH="$PATH:${GOPATH:-$HOME/go}/bin"'."
# fix it and rerun following. 
make toolchain

# Run this before submitting code
make presubmit

# Install CRDs (which only need once) 
kubectl apply -f CRDs/k8s-gateway-v1alpha2.yaml
kubectl apply -f CRDs/multicluster.x-k8s.io_serviceexports.yaml
kubectl apply -f CRDs/multicluster.x-k8s.io_serviceimports.yaml

# Run the controller against the Kubernetes cluster pointed to by `kubectl config current-context`
make run
```

### End-to-End Testing

```
# Add models to AWS CLI
aws configure add-model --service-model file://scripts/aws_sdk_model_override/models/apis/mercury/2021-08-17/api-2.json --service-name ec2-mercury

# List Services
aws ec2-mercury list-services --endpoint-url=https://vpc-lattice.us-west-2.amazonaws.com

```

Check [Detail Notes](https://code.amazon.com/packages/MercuryK8SController/blobs/mainline/--/developer.md) on how to run end-to-end test

## Security

See [CONTRIBUTING](CONTRIBUTING.md#security-issue-notifications) for more information.

## License

This project is licensed under the Apache-2.0 License.
