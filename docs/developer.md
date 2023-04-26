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
# specify REGION where your cluster is running
REGION=us-west-2 make run

# or run it in debug mode
GATEWAY_API_CONTROLLER_LOGLEVEL=debug make run

# to run it against specific lattice service endpoint
LATTICE_ENDPOINT=https://vpc-lattice.us-west-2.amazonaws.com/ make run
```

To easier load environment variables, if you hope to run the controller by GoLand IDE locally, you could run the `scripts/load_env_variables.sh`
And use "EnvFile" GoLand plugin to read the env variables from the generated `.env` file.

### End-to-End Testing

Run the following command to run the end-to-end tests against the Kubernetes cluster pointed to by `kubectl config current-context`:
You should set up the correct `REGION` env variable
```
export REGION=us-west-2
make e2etest
```

Pass `FOCUS` environment variable to run some specific test cases based on filter condition:
```bash
export REGION=us-west-2
export FOCUS="HTTPRoute should support multiple path matches" 
make e2etest
```
For more detail on filter condition for ginkgo
https://onsi.github.io/ginkgo/#focused-specs
https://onsi.github.io/ginkgo/#description-based-filtering

Notice: the prerequisites for running the end-to-end tests success are:
- Current eks cluster don't have any k8s resource
- The vpc used by current eks cluster don't have any vpc service network association

After all test cases running finished, in the `AfterSuite()` function, it will clean up k8s and vpc lattice resource created by current test cases running.

Before sending a Pull Request, usually you should run the `make e2etest` to make sure all e2e tests pass.

### Make Docker Image

```
make docker-build
```

### Deploy Controller inside a Kubernetes Cluster

#### Generate deploy.yaml

```
make build-deploy
```
Then follow [Deploying the AWS Gateway API Controller](https://github.com/aws/aws-application-networking-k8s/blob/main/docs/deploy.md) to configure and deploy the docker image 
