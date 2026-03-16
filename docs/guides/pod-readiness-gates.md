# Pod readiness gate

AWS Gateway API controller supports [»Pod readiness gates«](https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#pod-readiness-gate) to indicate that pod is registered to the VPC Lattice and healthy to receive traffic.
The controller automatically injects the necessary readiness gate configuration to the pod spec via mutating webhook during pod creation.

For readiness gate configuration to be injected to the pod spec, you need to apply the label `application-networking.k8s.aws/pod-readiness-gate-inject: enabled` to the pod namespace. 

The pod readiness gate is needed under certain circumstances to achieve full zero downtime rolling deployments. Consider the following example:

* Low number of replicas in a deployment
* Start a rolling update of the deployment
* Rollout of new pods takes less time than it takes the AWS Gateway API controller to register the new pods and for their health state turn »Healthy« in the target group
* At some point during this rolling update, the target group might only have registered targets that are in »Initial« or »Draining« state; this results in service outage

In order to avoid this situation, the AWS Gateway API controller can set the readiness condition on the pods that constitute your ingress or service backend. The condition status on a pod will be set to `True` only when the corresponding target in the VPC Lattice target group shows a health state of »Healthy«.
This prevents the rolling update of a deployment from terminating old pods until the newly created pods are »Healthy« in the VPC Lattice target group and ready to take traffic.

## Setup
Pod readiness gates rely on [»admission webhooks«](https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/), where the Kubernetes API server makes calls to the AWS Gateway API controller as part of pod creation. This call is made using TLS, so the controller must present a TLS certificate. This certificate is stored as a standard Kubernetes secret. If you are using Helm, the certificate will automatically be configured as part of the Helm install.

For Helm installs, certificate validity is controlled by `webhookTLS.validityDays`.
The default is `36500` days to preserve existing behavior. You can opt in to
`365` days for annual certificate rotation:

```bash
helm upgrade --install gateway-api-controller \
  oci://public.ecr.aws/aws-application-networking-k8s/aws-gateway-controller-chart \
  --namespace aws-application-networking-system \
  --set webhookTLS.validityDays=365
```

When using `365`, plan and automate annual certificate rotation.

If you are manually deploying the controller using the ```deploy.yaml``` file, you will need to either patch the ```deploy.yaml``` file (see ```scripts/patch-deploy-yaml.sh```) or generate the secret following installation (see ```scripts/gen-webhook-secret.sh```) and manually enable the webhook via the ```WEBHOOK_ENABLED``` environment variable.

Note that, without the secret in place, the controller cannot start successfully, and you will see an error message like the following:
```
{"level":"error","ts":"...","logger":"setup","caller":"workspace/main.go:240","msg":"tls: failed to find any PEM data in certificate inputproblem running manager"}
```
For this reason, the webhook is ```DISABLED``` by default in the controller for the non-Helm install. You can enable the webhook by setting the ```WEBHOOK_ENABLED``` environment variable to "true" in the ```deploy.yaml``` file.
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gateway-api-controller
  namespace: aws-application-networking-system
  labels:
    control-plane: gateway-api-controller
spec:
  ...
  template:
    metadata:
      annotations:
        kubectl.kubernetes.io/default-container: manager
      labels:
        control-plane: gateway-api-controller
    spec:
      securityContext:
        runAsNonRoot: true
      containers:
      - command:
        ...
        name: manager
        ...
        env:
          - name: WEBHOOK_ENABLED
            value: "true"   # <-- value of "true" enables the webhook in the controller
```
If you run ```scripts/patch-deploy-yaml.sh``` prior to installing ```deploy.yaml```, the script will create the necessary TLS certificates and configuration and will enable the webhook in the controller. Note that, even with the webhook enabled, the webhook will only run for namespaces labeled with `application-networking.k8s.aws/pod-readiness-gate-inject: enabled`.  

## Enabling the readiness gate
After a Helm install or manually configuring and enabling the webhook, you are ready to begin using pod readiness gates. Apply a label to each namespace you would like to use this feature. You can create and label a namespace as follows -

```
$ kubectl create namespace example-ns
namespace/example-ns created

$ kubectl label namespace example-ns application-networking.k8s.aws/pod-readiness-gate-inject=enabled
namespace/example-ns labeled

$ kubectl describe namespace example-ns
Name:         example-ns
Labels:       application-networking.k8s.aws/pod-readiness-gate-inject=enabled
              kubernetes.io/metadata.name=example-ns
Annotations:  <none>
Status:       Active
```

Once labelled, the controller will add the pod readiness gates to all subsequently created pods in the namespace.

The readiness gates have the condition type ```application-networking.k8s.aws/pod-readiness-gate``` and the controller injects the config to the pod spec only during pod creation.

## Object Selector
The default webhook configuration matches all pods in the namespaces containing the label `application-networking.k8s.aws/pod-readiness-gate-inject=enabled`. You can modify the webhook configuration further to select specific pods from the labeled namespace by specifying the `objectSelector`. For example, in order to select ONLY pods with `application-networking.k8s.aws/pod-readiness-gate-inject: enabled` label instead of all pods in the labeled namespace, you can add the following `objectSelector` to the webhook:
```
  objectSelector:
    matchLabels:
      application-networking.k8s.aws/pod-readiness-gate-inject: enabled
```
To edit,
```
$ kubectl edit mutatingwebhookconfigurations aws-appnet-gwc-mutating-webhook
  ...
  name: mpod.gwc.k8s.aws
  namespaceSelector:
    matchExpressions:
    - key: application-networking.k8s.aws/pod-readiness-gate-inject
      operator: In
      values:
      - enabled
  objectSelector:
    matchLabels:
      application-networking.k8s.aws/pod-readiness-gate-inject: enabled
  ...
```
When you specify multiple selectors, pods matching all the conditions will get mutated.

## Checking the pod condition status

The status of the readiness gates can be verified with `kubectl get pod -o wide`:
```
NAME                          READY   STATUS    RESTARTS   AGE   IP         NODE                       READINESS GATES
nginx-test-5744b9ff84-7ftl9   1/1     Running   0          81s   10.1.2.3   ip-10-1-2-3.ec2.internal   0/1
```

When the target is registered and healthy in the VPC Lattice target group, the output will look like:
```
NAME                          READY   STATUS    RESTARTS   AGE   IP         NODE                       READINESS GATES
nginx-test-5744b9ff84-7ftl9   1/1     Running   0          81s   10.1.2.3   ip-10-1-2-3.ec2.internal   1/1
```

If a readiness gate doesn't get ready, you can check the reason via:

```console
$ kubectl get pod nginx-test-545d8f4d89-l7rcl -o yaml | grep -B7 'type: application-networking.k8s.aws/pod-readiness-gate'
status:
  conditions:
  - lastProbeTime: null
    lastTransitionTime: null
    reason: HEALTHY
    status: "True"
    type: application-networking.k8s.aws/pod-readiness-gate
```
