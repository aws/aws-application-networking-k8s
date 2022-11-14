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
kubectl apply -f config/crds/bases/k8s-gateway-v1alpha2.yaml
kubectl apply -f config/crds/bases/multicluster.x-k8s.io_serviceexports.yaml
kubectl apply -f config/crds/bases/multicluster.x-k8s.io_serviceimports.yaml

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

### Make Docker Image

```
make docker-build
```

### Deploy Controller inside a Kubernetes Cluster

#### Generate deploy.yaml

```
make build-deploy
```

####  Configure IAM role for k8s pod ONLY if runs MercuryK8SController inside cluster
##### Configure role for k8s pod to invoke mercury api

Step 1: Create an IAM OIDC provider for your cluster:
https://docs.aws.amazon.com/eks/latest/userguide/enable-iam-roles-for-service-accounts.html
```
eksctl utils associate-iam-oidc-provider --cluster <my-cluster> --approve
```

Step 2: Create a policy in IAM that can invoke mercury API and copy the policy arn for later use
(iam-policy.json is under /code) :

```
cd code
aws iam create-policy \
    --policy-name AWSMercuryControllerIAMPolicy \
    --policy-document file://iam-policy.json
```

```
# a sample iam-policy.json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "vpc-lattice:*",
                "iam:CreateServiceLinkedRole",
                "ec2:DescribeVpcs",
                "ec2:DescribeSubnets"
            ],
            "Resource": "*"
        }
    ]
}
```

Step 3: Create iamserviceaccount for pod level permission
```
eksctl create iamserviceaccount \
--cluster=<my-cluster-name> \
--namespace=system \
--name=gateway-api-controller \
--attach-policy-arn=<AWSMercuryControllerIAMPolicy ARN CREATED IN STEP 2> \
--override-existing-serviceaccounts \
--region us-west-2 \
--approve
```

Step 4: deploy into cluster

```
kubectl apply -f deploy.yaml
```


Check [Detail Notes](https://code.amazon.com/packages/MercuryK8SController/blobs/mainline/--/developer.md) on how to run end-to-end test

## Security

See [CONTRIBUTING](CONTRIBUTING.md#security-issue-notifications) for more information.

## License

This project is licensed under the Apache-2.0 License.
