# Developer Guide



## Prerequisites

**Tools**

Install these tools before proceeding:

1. [AWS CLI](https://docs.aws.amazon.com/cli/latest/userguide/install-cliv2-linux.html),
2. `kubectl` - [the Kubernetes CLI](https://kubernetes.io/docs/tasks/tools/install-kubectl-linux/),
3. `helm` - [the package manager for Kubernetes](https://helm.sh/docs/intro/install/),
4. `eksctl`- [the CLI for Amazon EKS](https://docs.aws.amazon.com/eks/latest/userguide/setting-up.html),
5. `go v1.20.x` - [language](https://go.dev/doc/install),
6. `yq` - [CLI to manipulate yaml files](https://github.com/mikefarah/yq#install),
7. `jq` - [CLI to manipulate json files](https://jqlang.github.io/jq/),
8. `make`- build automation tool. 

**Cluster creation and setup**

Before proceeding to the next sections, you need to:

1. Create a and set up a cluster `dev-cluster` with the controller following the [AWS Gateway API Controller installation guide on Amazon EKS](../guides/deploy.md).

    !!! Note
        You can either install the Controller and CRDs following the [steps in the installation guide](../guides/deploy.md/#install-the-controller) or using the steps below if you prefer to create the individual CRDs.

1. Clone the [AWS Gateway API Controller](https://github.com/aws/aws-application-networking-k8s) repository.
    ```bash
    git clone git@github.com:aws/aws-application-networking-k8s.git
    cd aws-application-networking-k8s
    ```
1. Install dependencies with `toolchain.sh` script:
    ```bash
    make toolchain
    ```


## Setup

Once cluster is ready, we need to apply CRDs for `gateway-api` resources. First install core `gateway-api` CRDs:

=== "v1 CRDs (latest, recommended)"
    Install the latest `v1` CRDs:
    ```bash
    kubectl apply -f config/crds/bases/k8s-gateway-v1.0.0.yaml
    ```
    !!! Note
        Note that v1 CRDs are **not included** in `deploy-*.yaml` and `helm` chart by default. 
=== "v1beta1 CRDs"
    Install `gateway-api` `v1beta1` CRDs.
    ```bash
    kubectl apply -f config/crds/bases/k8s-gateway-v0.6.1.yaml
    ```

And install additional CRDs for the controller:

```bash
kubectl apply -f config/crds/bases/externaldns.k8s.io_dnsendpoints.yaml
kubectl apply -f config/crds/bases/application-networking.k8s.aws_serviceexports.yaml
kubectl apply -f config/crds/bases/application-networking.k8s.aws_serviceimports.yaml
kubectl apply -f config/crds/bases/application-networking.k8s.aws_targetgrouppolicies.yaml
kubectl apply -f config/crds/bases/application-networking.k8s.aws_vpcassociationpolicies.yaml
kubectl apply -f config/crds/bases/application-networking.k8s.aws_accesslogpolicies.yaml
kubectl apply -f config/crds/bases/application-networking.k8s.aws_iamauthpolicies.yaml
```

When e2e tests are terminated during execution, it might break clean-up stage and resources will leak. To delete dangling resources manually use cleanup script:

```bash
make e2e-clean
```

## Local Development

A minimal test of changes can be done with `make presubmit`. This command will also run on PR.

```sh
make presubmit
```

Start controller in development mode, that will point to cluster (see setup above).

```sh
// should be region of the cluster
REGION=us-west-2 make run
```

You can explore a collection of different yaml configurations in the examples folder that can be applied to cluster.

To run it against specific lattice service endpoint.

```sh
LATTICE_ENDPOINT=https://vpc-lattice.us-west-2.amazonaws.com/ make run
```

To easier load environment variables, if you hope to run the controller by GoLand IDE locally, you could run the `./scripts/load_env_variables.sh`
And use "EnvFile" GoLand plugin to read the env variables from the generated `.env` file.

## End-to-End Testing

For larger changes it's recommended to run e2e suites on your local cluster.
E2E tests require a service network named `test-gateway` with cluster VPC associated to run.
You can either set up service network manually or use DEFAULT_SERVICE_NETWORK option when running controller locally. (e.g. `DEFAULT_SERVICE_NETWORK=test-gateway make run`)

```sh
REGION=us-west-2 make e2e-test
```

For the `RAM Share` test suite, which runs cross-account e2e tests, you will need a secondary AWS Account with a role that
can be assumed by the primary account during test execution.
You can create an IAM Role, with a Trust Policy allowing the primary account to assume it, via the AWS IAM Console.

```sh
export SECONDARY_ACCOUNT_TEST_ROLE_ARN=arn:aws:iam::000000000000:role/MyRole
export FOCUS="RAM Share"
REGION=us-west-2 make e2e-test
```

You can use the `FOCUS` environment variable to run some specific test cases based on filter condition.
You could assign the string in the Describe("xxxxxx") or It("xxxxxx") to the FOCUS environment variable to run the specific test cases.
```go
var _ = Describe("HTTPRoute path matches", func() {
	It("HTTPRoute should support multiple path matches", func() {
        // test case body
    })
```

For example, to run the test case "HTTPRoute should support multiple path matches", you could run the following command:
```sh
export FOCUS="HTTPRoute should support multiple path matches"
export REGION=us-west-2
make e2e-test
```

Conversely, you can use the `SKIP` environment variable to skip specific test cases.

For example, to skip the same test as above, you would run the following command:
```sh
export SKIP="HTTPRoute should support multiple path matches"
```

For more detail on filter condition for ginkgo
https://onsi.github.io/ginkgo/#focused-specs
https://onsi.github.io/ginkgo/#description-based-filtering

After all test cases running finished, in the `AfterSuite()` function, it will clean up k8s and vpc lattice resource created by current test cases running.

## Documentations

The controller documentation is managed in `docs/` directory, and built with [mkdocs](https://www.mkdocs.org/).
To build and verify your changes locally:
```sh
pip install -r requirements.txt
make docs
```
The website will be located in `site/` directory. You can also run a local dev-server by running `mkdocs serve`.

## Contributing

Before sending a Pull Request, you should run unit tests:

```sh
make presubmit
```

For larger, functional changes, run e2e tests:
```sh
make e2e-test
```

## Make Docker Image

```
make docker-build
```

## Deploy Controller inside a Kubernetes Cluster

Generate `deploy.yaml`

``` sh
make build-deploy
```

Then follow [Deploying the AWS Gateway API Controller](../guides/deploy.md) to configure and deploy the docker image.