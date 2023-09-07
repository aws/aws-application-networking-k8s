# Developer Guide

## Tools

Before you start you need to have following:

- aws account - https://aws.amazon.com/
- aws cli - https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html
- eksctl - https://github.com/eksctl-io/eksctl/blob/main/README.md#installation
- kubectl - https://docs.aws.amazon.com/eks/latest/userguide/install-kubectl.html
- go v1.20.x - https://go.dev/doc/install
- yq - https://github.com/mikefarah/yq#install
- jq - https://jqlang.github.io/jq/
- make 

After pulling repo toolchain.sh script will install other dependencies.

```bash
make toolchain
```

## Cluster Setup

To run controller in cluster or in development mode you need an EKS cluster. It's a one time setup.
Running controller in development mode will start controller locally and connects to EKS cluster, 
this is preferable way for local development.

Once you have eksctl and aws account you can create EKS cluster. It's handy to set env variables, since many places relies on them.

```bash
export AWS_ACCOUNT= {your account}
export AWS_REGION= {region with eks and lattice}
export CLUSTER_NAME=dev-cluster
```

Create an EKS cluster and allow Lattice traffic into cluster.

```bash
eksctl create cluster --name $CLUSTER_NAME --region $AWS_REGION

/ update security group to allow lattice traffic
PREFIX_LIST_ID=$(aws ec2 describe-managed-prefix-lists --query "PrefixLists[?PrefixListName=="\'com.amazonaws.$AWS_REGION.vpc-lattice\'"].PrefixListId" | jq -r '.[]')
MANAGED_PREFIX=$(aws ec2 get-managed-prefix-list-entries --prefix-list-id $PREFIX_LIST_ID --output json  | jq -r '.Entries[0].Cidr')
CLUSTER_SG=$(aws eks describe-cluster --name $CLUSTER_NAME --output json| jq -r '.cluster.resourcesVpcConfig.clusterSecurityGroupId')
aws ec2 authorize-security-group-ingress --group-id $CLUSTER_SG --cidr $MANAGED_PREFIX --protocol -1

eksctl utils associate-iam-oidc-provider --cluster $CLUSTER_NAME --approve --region $AWS_REGION

aws iam create-policy \
   --policy-name VPCLatticeControllerIAMPolicy \
   --policy-document file://examples/recommended-inline-policy.json
   
export VPCLatticeControllerIAMPolicyArn=$(aws iam list-policies --query 'Policies[?PolicyName==`VPCLatticeControllerIAMPolicy`].Arn' --output text)

eksctl create iamserviceaccount \
   --cluster=$CLUSTER_NAME \
   --namespace=aws-application-networking-system \
   --name=gateway-api-controller \
   --attach-policy-arn=$VPCLatticeControllerIAMPolicyArn \
   --override-existing-serviceaccounts \
   --region $AWS_REGION \
   --approve
```

Once cluster is ready. We need to apply CRDs for gateway-api resources.

```bash
$kubectl apply -f config/crds/bases/k8s-gateway-v0.6.1.yaml
kubectl apply -f config/crds/bases/multicluster.x-k8s.io_serviceexports.yaml
kubectl apply -f config/crds/bases/multicluster.x-k8s.io_serviceimports.yaml
kubectl apply -f config/crds/bases/externaldns.k8s.io_dnsendpoints.yaml
kubectl apply -f config/crds/bases/application-networking.k8s.aws_targetgrouppolicies.yaml
kubectl apply -f examples/gatewayclass.yaml
```

And create non-default namespace for e2etest. In case you want to run them.

```bash
kubectl create namespace non-default
```

## Local Development

A minimal sanity check on changes can be done with make presubmit. This command will also run on PR.

```
make presubmit
```

Start controller in development mode, that will point to cluster (see setup above).

```
// should be region of the cluster
REGION=us-west-2 make run
```

You can explore a collection of different yaml configurations in the examples folder that can be applied to cluster.

To run it against specific lattice service endpoint.

```
LATTICE_ENDPOINT=https://vpc-lattice.us-west-2.amazonaws.com/ make run
```

To easier load environment variables, if you hope to run the controller by GoLand IDE locally, you could run the `./scripts/load_env_variables.sh`
And use "EnvFile" GoLand plugin to read the env variables from the generated `.env` file.

## End-to-End Testing

For larger changes it's recommended to run e2e suites on your local cluster.

```
REGION=us-west-2 make e2etest
```

You can use `FOCUS` environment variable to run some specific test cases based on filter condition.
You could assign the string in the Describe("xxxxxx") or It("xxxxxx") to the FOCUS environment variable to run the specific test cases.
```go
var _ = Describe("HTTPRoute path matches", func() {
	It("HTTPRoute should support multiple path matches", func() {
        // test case body
    })
```

```
export FOCUS="HTTPRoute should support multiple path matches"
export REGION=us-west-2
make e2etest
```

For example, to run the test case "HTTPRoute should support multiple path matches", you could run the following command:

For more detail on filter condition for ginkgo
https://onsi.github.io/ginkgo/#focused-specs
https://onsi.github.io/ginkgo/#description-based-filtering

Notice: the prerequisites for running the end-to-end tests success are:
- Current eks cluster don't have any k8s resource
- The vpc used by current eks cluster don't have any vpc service network association

After all test cases running finished, in the `AfterSuite()` function, it will clean up k8s and vpc lattice resource created by current test cases running.

## Contributing

Before sending a Pull Request, you should run unit tests:

```sh
make presubmit
```

For larger, functional changes, run e2e tests:
```sh
make e2etest
```

It is recommended to run `make e2etest` in both environments where `DNSEndpoint` CRD exists and does not exist,
as the controller is designed to support both use cases.

## Make Docker Image

```
make docker-build
```

## Deploy Controller inside a Kubernetes Cluster

### Generate deploy.yaml

```
make build-deploy
```
Then follow [Deploying the AWS Gateway API Controller](https://github.com/aws/aws-application-networking-k8s/blob/main/docs/deploy.md) to configure and deploy the docker image
