# Pod readiness gate

# TODO: Update to reflect final implementation of readiness gate logic. Replace <TBD>

AWS Gateway API controller supports [»Pod readiness gates«](https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#pod-readiness-gate) to indicate that pod is registered to the VPC Lattice and healthy to receive traffic.
The controller automatically injects the necessary readiness gate configuration to the pod spec via mutating webhook during pod creation.

For readiness gate configuration to be injected to the pod spec, you need to apply the label `aws-application-networking-k8s/pod-readiness-gate-inject: enabled` to the pod namespace. 

The pod readiness gate is needed under certain circumstances to achieve full zero downtime rolling deployments. Consider the following example:

* Low number of replicas in a deployment
* Start a rolling update of the deployment
* Rollout of new pods takes less time than it takes the AWS Gateway API controller to register the new pods and for their health state turn »Healthy« in the target group
* At some point during this rolling update, the target group might only have registered targets that are in »Initial« or »Draining« state; this results in service outage

In order to avoid this situation, the AWS Gateway API controller can set the readiness condition on the pods that constitute your ingress or service backend. The condition status on a pod will be set to `True` only when the corresponding target in the VPC Lattice target group shows a health state of »Healthy«.
This prevents the rolling update of a deployment from terminating old pods until the newly created pods are »Healthy« in the VPC Lattice target group and ready to take traffic.

## Setup
Pod readiness gates rely on [»admission webhooks«](https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/), where the Kubernetes API server makes calls to the AWS Gateway API controller as part of pod creation. This call is made using TLS, so the controller must present a TLS certificate. This certificate is stored as a standard Kubernetes secret. If you are using Helm, the certificate will automatically be configured as part of the Helm install.

If you are manually deploying the controller, for example using the ```deploy.yaml``` file, you will need to create the tls secret for the webhook in the controller namespace. The ```deploy.yaml``` file includes a placeholder secret, but it must be updated if you wish to use the webhook. The placeholder secret _will not_ pass API server validations, but will ensure the controller container is able to start.

### Webhook secret requirements
The webhook requires a specific kubernetes secret to exist in the same namespace as the webhook itself:
* secret name: ```webhook-cert```
* default controller namespace: ```aws-application-networking-system```
```console
# example create-secret command, assumes tls.crt and tls.key exist in current directory
# if the placeholder secret exists, you will need to delete it before setting the new value
kubectl create secret tls webhook-cert --namespace aws-application-networking-system --cert=tls.crt --key=tls.key
```

### Webhook secret configuration example
The below example creates an unsigned certificate, adds it as the webhook secret, then patches the webhook configuration so the API server trusts the certificate.

If your cluster uses its own PKI and includes appropriate trust configuration for the API server, the certificate issued would be signed by your internal certificate authority and therefore not require the ```kubectl patch``` command below.
```console
# Example commands to configure the webhook to use an unsigned certificate
CERT_FILE=tls.crt
KEY_FILE=tls.key

WEBHOOK_SVC_NAME=webhook-service
WEBHOOK_NAME=aws-appnet-gwc-mutating-webhook
WEBHOOK_NAMESPACE=aws-application-networking-system
WEBHOOK_SECRET_NAME=webhook-cert

# Step 1: generate a certificate if needed, can also be provisioned through orgnanizational PKI, etc
# This cert includes a 100 year expiry
HOST=${WEBHOOK_SVC_NAME}.${WEBHOOK_NAMESPACE}.svc
openssl req -x509 -nodes -days 36500 -newkey rsa:2048 -keyout ${KEY_FILE} -out ${CERT_FILE} -subj "/CN=${HOST}/O=${HOST}" \
   -addext "subjectAltName = DNS:${HOST}, DNS:${HOST}.cluster.local"
   
# Step 2: replace the placeholder secret from deploy.yaml
kubectl delete secret $WEBHOOK_SECRET_NAME --namespace $WEBHOOK_NAMESPACE
kubectl create secret tls $WEBHOOK_SECRET_NAME --namespace $WEBHOOK_NAMESPACE --cert=${CERT_FILE} --key=${KEY_FILE}

# Step 3: Patch the webhook CA bundle to exactly the cert being used.
# This will ensure Kubernetes API server is able to trust the certificate presented by the webhook.
# This step would not be required if you are using a signed certificate that is already trusted by the API server
CERT_B64=$(cat tls.crt | base64)
kubectl patch mutatingwebhookconfigurations.admissionregistration.k8s.io $WEBHOOK_NAME \
    --namespace $WEBHOOK_NAMESPACE --type='json' \
    -p="[{'op': 'replace', 'path': '/webhooks/0/clientConfig/caBundle', 'value': '${CERT_B64}'}]"
```

## Configuration
Pod readiness gate support is enabled by default on the AWS Gateway API controller. To enable the feature, you must apply a label to each of the namespaces you would like to use this feature. You can create and label a namespace as follows -

```
$ kubectl create namespace example-ns
namespace/example-ns created

$ kubectl label namespace example-ns aws-application-networking-k8s/pod-readiness-gate-inject=enabled
namespace/example-ns labeled

$ kubectl describe namespace example-ns
Name:         example-ns
Labels:       aws-application-networking-k8s/pod-readiness-gate-inject=enabled
              kubernetes.io/metadata.name=example-ns
Annotations:  <none>
Status:       Active
```

Once labelled, the controller will add the pod readiness gates to all subsequently created pods.

The readiness gates have the prefix `<TBD>` and the controller injects the config to the pod spec only during pod creation.

## Object Selector
The default webhook configuration matches all pods in the namespaces containing the label `aws-application-networking-k8s/pod-readiness-gate-inject=enabled`. You can modify the webhook configuration further to select specific pods from the labeled namespace by specifying the `objectSelector`. For example, in order to select ONLY pods with `aws-application-networking-k8s/pod-readiness-gate-inject: enabled` label instead of all pods in the labeled namespace, you can add the following `objectSelector` to the webhook:
```
  objectSelector:
    matchLabels:
      aws-application-networking-k8s/pod-readiness-gate-inject: enabled
```
To edit,
```
$ kubectl edit mutatingwebhookconfigurations aws-appnet-gwc-mutating-webhook
  ...
  name: mpod.gwc.k8s.aws
  namespaceSelector:
    matchExpressions:
    - key: aws-application-networking-k8s/pod-readiness-gate-inject
      operator: In
      values:
      - enabled
  objectSelector:
    matchLabels:
      aws-application-networking-k8s/pod-readiness-gate-inject: enabled
  ...
```
When you specify multiple selectors, pods matching all the conditions will get mutated.

## Disabling the readiness gate inject
You can specify the controller flag `--enable-pod-readiness-gate-inject=false` during controller startup to disable the controller from modifying the pod spec.

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
$ kubectl get pod nginx-test-545d8f4d89-l7rcl -o yaml | grep -B7 'type: <TBD>'
status:
  conditions:
  - lastProbeTime: null
    lastTransitionTime: null
    message: <TBD>
    reason: HEALTHY
    status: "True"
    type: <TBD>
```
