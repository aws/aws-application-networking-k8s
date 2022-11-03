AWS Application Networking is an implementation of the Kubernetes [Gateway API](https://gateway-api.sigs.k8s.io/). This project is designed to run in a Kubernetes cluster and orchestrates AWS VPC Lattice resources using Kubernetes Custom Resource Definitions like Gateway and HTTPRoute.

### Developer Guide

```bash
# Learn available `make` commands
make help

# Run this before submitting code
make presubmit

# Run the controller against the Kubernetes cluster pointed to by `kubectl config current-context`
make run

# This only needs to be run once after checking out the repo, and will install tools/codegen required for development
make toolchain
```

### End-to-End Testing

```
# Add models to AWS CLI
aws configure add-model --service-model file://scripts/aws_sdk_model_override/models/apis/mercury/2021-08-17/api-2.json --service-name ec2-mercury

# List Services
aws ec2-mercury list-services --endpoint-url=https://vpc-service-network.us-west-2.amazonaws.com
```

Check [Detail Notes](https://code.amazon.com/packages/MercuryK8SController/blobs/mainline/--/developer.md) on how to run end-to-end test

## Security

See [CONTRIBUTING](CONTRIBUTING.md#security-issue-notifications) for more information.

## License

This project is licensed under the Apache-2.0 License.
