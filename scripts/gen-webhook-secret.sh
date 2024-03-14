#!/usr/bin/env bash

# Use this script to update the webhook secret post-deployment. Alternatively,
# you can use patch-deploy-yaml.sh prior to deployment.
#
# Generates a certificate for use by the controller webhook
# You can also manually re-create the webhook secret using your own PKI

TEMP_KEY=`mktemp`
TEMP_CERT=`mktemp`

WEBHOOK_SVC_NAME=webhook-service
WEBHOOK_NAME=aws-appnet-gwc-mutating-webhook
WEBHOOK_NAMESPACE=aws-application-networking-system
WEBHOOK_SECRET_NAME=webhook-cert

echo "Generating certificate for webhook"
HOST=${WEBHOOK_SVC_NAME}.${WEBHOOK_NAMESPACE}.svc
openssl req -x509 -nodes -days 36500 -newkey rsa:2048 -keyout ${TEMP_KEY} -out ${TEMP_CERT} -subj "/CN=${HOST}/O=${HOST}" \
   -addext "subjectAltName = DNS:${HOST}, DNS:${HOST}.cluster.local"

CERT_B64=`cat $TEMP_CERT | base64`

echo "Recreating webhook secret"
kubectl delete secret $WEBHOOK_SECRET_NAME --namespace $WEBHOOK_NAMESPACE
kubectl create secret tls $WEBHOOK_SECRET_NAME --namespace $WEBHOOK_NAMESPACE --cert=${TEMP_CERT} --key=${TEMP_KEY}

echo "Patching webhook with new cert"
kubectl patch mutatingwebhookconfigurations.admissionregistration.k8s.io $WEBHOOK_NAME \
    --namespace $WEBHOOK_NAMESPACE --type='json' \
    -p="[{'op': 'replace', 'path': '/webhooks/0/clientConfig/caBundle', 'value': '${CERT_B64}'}]"

rm $TEMP_KEY $TEMP_CERT
echo "Done"