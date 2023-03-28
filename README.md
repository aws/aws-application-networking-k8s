AWS Application Networking is an implementation of the Kubernetes [Gateway API](https://gateway-api.sigs.k8s.io/). This project is designed to run in a Kubernetes cluster and orchestrates AWS VPC Lattice resources using Kubernetes Custom Resource Definitions like Gateway and HTTPRoute.

## Developer Guide

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
kubectl apply -f config/crds/bases/k8s-gateway-v0.6.1.yaml
kubectl apply -f config/crds/bases/multicluster.x-k8s.io_serviceexports.yaml
kubectl apply -f config/crds/bases/multicluster.x-k8s.io_serviceimports.yaml

# Run the controller against the Kubernetes cluster pointed to by `kubectl config current-context`
make run

# or run it in debug mode
GATEWAY_API_CONTROLLER_LOGLEVEL=debug make run

# to run it against specific lattice service endpoint
LATTICE_ENDPOINT=https://mercury-gamma.us-west-2.amazonaws.com/ make run
```

## End-to-End Testing

### Install VPC lattice CLIs

```
# Add models to AWS CLI
aws configure add-model --service-model file://scripts/aws_sdk_model_override/models/apis/vpc-lattice/2022-11-30/api-2.json --service-name vpc-lattice

# List Services
aws vpc-lattice list-services --endpoint-url=https://vpc-lattice.us-west-2.amazonaws.com

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

####  Configure IAM role for k8s pod ONLY if runs gateway-api-controller inside cluster

##### Configure role for k8s pod to invoke lattice api

Step 1: Create an EKS cluster:

```
eksctl create cluster --name <my-cluster> --region us-west-2
```

Step 2: Create an IAM OIDC provider for your cluster:
https://docs.aws.amazon.com/eks/latest/userguide/enable-iam-roles-for-service-accounts.html
```
eksctl utils associate-iam-oidc-provider --cluster <my-cluster> --approve
```

Step 3: Create a policy in IAM that can invoke vpc-lattice API and copy the policy arn for later use :

```
aws iam create-policy \
    --policy-name AWSVPCLatticeControllerIAMPolicy \
    --policy-document file://config/iam/recommended-inline-policy.json
```


Step 4: Create iamserviceaccount for pod level permission
```
eksctl create iamserviceaccount \
--cluster=<my-cluster-name> \
--namespace=system \
--name=gateway-api-controller \
--attach-policy-arn=<AWSVPCLatticeControllerIAMPolicy ARN CREATED IN STEP 2> \
--override-existing-serviceaccounts \
--region us-west-2 \
--approve
```

Step 5: Deploy into cluster using generated deploy.yaml..

```
kubectl apply -f deploy.yaml
```

Step 5: ..Or Deploy using helm Chart

```
# login ECR
aws ecr-public get-login-password --region us-east-1 | helm registry login --username AWS --password-stdin public.ecr.aws
```

```
helm install(or upgrade) gateway-api-controller \
oci://public.ecr.aws/aws-application-networking-k8s/aws-gateway-controller-chart \
 --version=v0.0.2 \
 --set=aws.region=us-west-2 --set=serviceAccount.create=false --namespace system
```


You can find more details in  [Detail Notes](https://code.amazon.com/packages/MercuryK8SController/blobs/mainline/--/developer.md) and [end-to-end Smoke Test](https://quip-amazon.com/FaquAsssAitb/Testing-Manual-end-to-end-Smoke-Testing-for-Kubernetes-Controllers).

## Release

To cut a new release, you will want to follow these steps:

1. Create a new Git branch for the new release.

```bash
export RELEASE_VERSION=v0.0.1  # Change this to the next release version you want
git checkout main
git fetch --all --tags && git rebase upstream/main
git checkout -b release-$RELEASE_VERSION
```

2. Update the Helm Chart's version and appVersion to the new release version.

Open `helm/Chart.yaml` and change the `version` and `appVersion` to match the `$RELEASE_VERSION`.

Open `helm/values.yaml` and change the `image.tag` value to match the `$RELEASE_VERSION`.

3. Create a Git commit for the new release artifacts.

```bash
git commit -a -m "release artifacts for release $RELEASE_VERSION"
git push origin release-$RELEASE_VERSION
```

4. Create a pull request from the release branch and have someone review and
   merge that for you.

5. Create a Git tag on the repository's main branch that points to the commit
   that you just got merged.

```bash
git checkout main
git fetch --all --tags && git rebase upstream/main
git tag -a $RELEASE_VERSION
git push origin $RELEASE_VERSION
```

6. Package and publish the controller container image and Helm chart.

```
PULL_BASE_REF=$RELEASE_VERSION ./scripts/release-controller.sh
```

NOTE: You will need to have exported an environment variable called
`ECR_PUBLISH_ROLE_ARN` that contains an IAM Role that your AWS user has a trust
relationship with and permission to publish to the ECR Public repositories. I
personally have a file in `~/.aws/gateway-publisher` that contains the
following:

```bash
export ECR_PUBLISH_ROLE_ARN="arn:aws:iam::606627242267:role/ECRPublisher"
```

which I `source` before running the `scripts/release-controller.sh` script.

## Security

See [CONTRIBUTING](CONTRIBUTING.md#security-issue-notifications) for more information.

## License

This project is licensed under the Apache-2.0 License.
