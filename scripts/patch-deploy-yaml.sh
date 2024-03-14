#!/usr/bin/env bash

# Updates deploy.yaml with a certificate for use by the controller webhook
# AND enables the webhook via environment variable.
# You can also provide a certificate with using own PKI, or use the
# gen-webhook-secret.sh script after deploying.
#
# Usage:
#  ./patch-deploy-yaml.sh [-k KEY_FILE -c CERT_FILE -a CA_FILE] <path/to/deploy.yaml>

KEY=""
CERT=""
CA=""
TEMP_KEY=`mktemp`
TEMP_CERT=`mktemp`

while getopts "k:c:a:" opt; do
  case $opt in
    k)
      KEY=${OPTARG}
      ;;
    c)
      CERT=${OPTARG}
      ;;
    a)
      CA=${OPTARG}
      ;;
    \?)
      echo "Invalid option: -$OPTARG" >&2
      exit 1
      ;;
    :)
      echo "Option -$OPTARG requires an argument." >&2
      exit 1
      ;;
  esac
done

shift $((OPTIND - 1))
DEPLOY_YAML=$1

if [ -z "${DEPLOY_YAML}" ]; then
  echo "Target deploy.yaml file not specified" >&2
  exit 1
fi

generate_cert() {
  KEY=$TEMP_KEY
  CERT=$TEMP_CERT
  # point the CA to the cert itself, this will ensure it is trusted by API server
  CA=$CERT

  WEBHOOK_SVC_NAME=webhook-service
  WEBHOOK_NAME=aws-appnet-gwc-mutating-webhook
  WEBHOOK_NAMESPACE=aws-application-networking-system
  WEBHOOK_SECRET_NAME=webhook-cert

  echo "Generating certificate for webhook"
  HOST=${WEBHOOK_SVC_NAME}.${WEBHOOK_NAMESPACE}.svc
  openssl req -x509 -nodes -days 36500 -newkey rsa:2048 -keyout ${KEY} -out ${CERT} -subj "/CN=${HOST}/O=${HOST}" \
     -addext "subjectAltName = DNS:${HOST}, DNS:${HOST}.cluster.local"
}

if [ -z "${KEY}" ]; then
  echo "Key not specified. Will generate..."
  REMOVE_KEY_FILES="1"
  generate_cert
fi

export KEY_B64=`cat $KEY | base64`
export CERT_B64=`cat $CERT | base64`
export CA_B64=`cat $CA | base64`

echo "Patching webhook secret"
yq -i e '(.[] as $item | select(.metadata.name == "webhook-cert" and .kind == "Secret") | .data."tls.crt") = env(CERT_B64)' $DEPLOY_YAML 2>&1
yq -i e '(.[] as $item | select(.metadata.name == "webhook-cert" and .kind == "Secret") | .data."tls.key") = env(KEY_B64)' $DEPLOY_YAML 2>&1
yq -i e '(.[] as $item | select(.metadata.name == "webhook-cert" and .kind == "Secret") | .data."ca.crt") = env(CA_B64)' $DEPLOY_YAML 2>&1

echo "Patching webhook"
yq -i e '(.[] as $item | select(.metadata.name == "aws-appnet-gwc-mutating-webhook" and .kind == "MutatingWebhookConfiguration") | .webhooks[0].clientConfig.caBundle) = env(CA_B64)' $DEPLOY_YAML 2>&1

echo "Enabling webhook"
yq -i -e '(.[] as $item | select(.metadata.name == "gateway-api-controller" and .kind == "Deployment") | .spec.template.spec.containers[] | select(.name == "manager") | .env[] | select(.name == "WEBHOOK_ENABLED") | .value) = "true"' $DEPLOY_YAML 2>&1

rm $TEMP_KEY $TEMP_CERT
echo "Done"