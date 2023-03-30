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
LATTICE_ENDPOINT=https://vpc-lattice.us-west-2.amazonaws.com/ make run
```

## End-to-End Testing


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
