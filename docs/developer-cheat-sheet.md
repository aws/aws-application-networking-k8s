# Developer Cheat Sheet

## Startup
The program flow is roughly as follows. On startup, ```cmd/aws-application-networking-k8s/main.go#main()``` runs. This initializes all the key components and registers various controllers (in ```controllers/```) with the Kubernetes control plane. These controllers include event handlers (in ```controllers/eventhandlers/```) whose basic function is to convert object notifications to more specific types and enqueue them for processing by the various controllers.

## Build and Deploy
Processing takes place in a controller's ```Reconcile()``` method, which will check if the object has been changed/created or deleted. Sometimes this invokes different paths, but most involve a ```buildAndDeployModel()``` step.

### Build
In the "build" step, the controller invokes a "model builder" with the changed Kubernetes object. Model builders (in ```pkg/gateway/```) convert the Kubernetes object state into an intermediate VPC Lattice object called a "model type". These model types (in ```pkg/model/lattice```) are basic structs which contain the important information about the source Kubernetes object and fields containing Lattice-related values. These models are built only using information from Kubernetes and are intended to contain all details needed to make updates against the VPC Lattice API. Once created, model ojbects are stored in a "stack" (```/pkg/model/core/stack.go```). The stack contains basic methods for storing and retrieving objects based on model type.

### Deploy
Once the "stack" is populated, it is serialized for logging then passed to the "deploy" step. Here, a "deployer" (```/pkg/deployer/stack_deployer.go```) will invoke a number of "synthesizers" (in ```/pkg/deploy/lattice/```) to read model objects from the stack and convert these to VPC Lattice API calls (via ```Synthesize()```). Most synthesizer interaction with the Lattice APIs is deferred to various "managers" (also ```/pkg/deploy/lattice```). These managers each have their own interface definition with CRUD methods that bridge model types and Lattice API types. Managers, in turn, use ```/pkg/aws/services/vpclattice.go``` for convenience methods against the Lattice API only using Lattice API types.

### Other notes

*Model <Type>Status Structs*
On each model type is a ```<Type>Status``` struct which contains information about the object from the Lattice API, such as ARNs or IDs. These are populated either on creation against the Lattice API or in cases where the API object already exists but needs updating via the model.

*Resource Cleanup*
A typical part of model synthesis (though it is also present in ) is checking all resources in the VPC Lattice API to see if they belong to deleted or missing Kubernetes resources. This is to help ensure Lattice resources are not leaked due to bugs in the reconciliation process.

## Code Consistency

To help reduce cognitive load while working through the code, it is critical to be consistent. For now, this applies most directly to imports and variable naming.

### Imports
Common imports should all use the same alias (or lack of alias) so that when we see ```aws.``` or ```vpclattice.``` or ```sdk.``` in the code we know what they refer to without having to double-check. Here are the conventions:

```go
import (
  "github.com/aws/aws-sdk-go/service/vpclattice" // no alias
  "github.com/aws/aws-sdk-go/aws" // no alias

  corev1 "k8s.io/api/core/v1"
  gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
  ctrl "sigs.k8s.io/controller-runtime"

  pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
  model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
  anv1alpha1 "github.com/aws/aws-application-networking-k8s/pkg/apis/applicationnetworking/v1alpha1"
)
```

For unit tests, this changes slightly for more readability when using the imports strictly for mocking
```go
import (
  mocks_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
  mocks "github.com/aws/aws-application-networking-k8s/pkg/aws/services"
  mock_client "github.com/aws/aws-application-networking-k8s/mocks/controller-runtime/client"
)
```

### Service, Service, or Service?
Similarly, because there is overlap between the Gateway API spec, model types, and Lattice API nouns, it is important to use differentiating names between the types in components where the name could be ambiguous. The safest approach is to use a prefix:

* ```k8s``` refers to a kubernetes object
* ```model``` refers to an intermediate model object
* ```lattice``` refers to an object from the Lattice API

An example in practice would be

```go
  var k8sSvc *corev1.Service
  var modelSvc *model.Service
  var latticeSvc *vpclattice.ServiceSummary
```

There are some objects which interact unambiguously with an underlying type, for example in ```vpclattice.go``` the types are always VPC Lattice API types, so disambiguating is less important there.
