#!/usr/bin/env bash

set -eo pipefail

DEFAULT_IMAGE_REPOSITORY="public.ecr.aws/aws-application-networking-k8s/aws-gateway-controller"
DEFAULT_CHART_REGISTRY="public.ecr.aws/aws-application-networking-k8s"
DEFAULT_CHART_REPOSITORY="aws-gateway-controller-chart"

USAGE="
Usage:
  $(basename "$0")

Publishes the controller image and helm chart for the AWS Gateway API controller.

Required environment variables:
  ECR_PUBLISH_ROLE_ARN:     A role ARN on the ECR repository account to assume.
                            Requires IAM permissions that allows pushing images/charts.
  RELEASE_VERSION:          Determines the tag published for controller image and chart.
                            Could be a semantic version number for release, or any other
                            unique string such as git commit SHA.

Optional environment variables:
  SKIP_BUILD_IMAGE:         If set to \"true\", does not build and publish the controller image.
  SKIP_BUILD_CHART:         If set to \"true\", does not build and publish the helm chart.
  IMAGE_REPOSITORY:         Overrides name for the ECR repository to push controller images to.
                            Default: $DEFAULT_IMAGE_REPOSITORY
  CHART_REGISTRY:           Overrides name for the OCI Artifact Registry to push to.
                            Default: $DEFAULT_CHART_REGISTRY
  CHART_REPOSITORY:         Overrides name for the Helm repository to push to.
                            Default: $DEFAULT_CHART_REPOSITORY
"

if [ -z "$ECR_PUBLISH_ROLE_ARN" ]; then
  echo "$USAGE"
  exit 1
fi

IMAGE_REPOSITORY=${IMAGE_REPOSITORY:-$DEFAULT_IMAGE_REPOSITORY}
CHART_REPOSITORY=${CHART_REPOSITORY:-$DEFAULT_CHART_REPOSITORY}
CHART_REGISTRY=${CHART_REGISTRY:-$DEFAULT_CHART_REGISTRY}


# Important Directory references
SCRIPTS_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
WORKSPACE_DIR=$SCRIPTS_DIR/..

# Check all the dependencies are present in container.
source "$SCRIPTS_DIR/lib/common.sh"
source "$SCRIPTS_DIR/lib/login.sh"
check_is_installed git
check_is_installed jq
check_is_installed yq

ASSUME_COMMAND=$(aws --output json sts assume-role --role-arn $ECR_PUBLISH_ROLE_ARN --role-session-name 'publish-images' --duration-seconds 3600 | jq -r '.Credentials | "export AWS_ACCESS_KEY_ID=\(.AccessKeyId)\nexport AWS_SECRET_ACCESS_KEY=\(.SecretAccessKey)\nexport AWS_SESSION_TOKEN=\(.SessionToken)\n"')
eval $ASSUME_COMMAND
echo "Assumed $ECR_PUBLISH_ROLE_ARN"

# Setup the destination repository for docker and helm
perform_docker_and_helm_login $IMAGE_REPOSITORY

if [[ "$SKIP_BUILD_IMAGE" != "true" ]]; then
  # Determine parameters for docker-build command

  cd "$WORKSPACE_DIR"

  CONTROLLER_IMAGE_DOCKERFILE_PATH=$WORKSPACE_DIR/Dockerfile

  IMG="$IMAGE_REPOSITORY:$RELEASE_VERSION"
  DOCKER_BUILD_CONTEXT="$WORKSPACE_DIR"

  echo "Building and pushing controller image ${IMG}"

  # build controller image
  if [[ $CI = "true" ]]; then
    docker buildx build -t "$IMG" \
      -f "$CONTROLLER_IMAGE_DOCKERFILE_PATH" \
      --cache-from type=gha \
      --cache-to type=gha,mode=max \
      --push \
      "${DOCKER_BUILD_CONTEXT}"
  else
    docker buildx build -t "$IMG" \
      -f "$CONTROLLER_IMAGE_DOCKERFILE_PATH" \
      --push \
      "${DOCKER_BUILD_CONTEXT}"
  fi

  if [ $? -ne 0 ]; then
    exit 2
  fi
fi

if [[ "$SKIP_BUILD_CHART" != "true" ]]; then
  export HELM_EXPERIMENTAL_OCI=1

  echo "Tarballing and pushing Helm chart package $CHART_REGISTRY/$CHART_REPOSITORY:$RELEASE_VERSION"
  helm package "$WORKSPACE_DIR"/helm/
  CHART_PACKAGE="$CHART_REPOSITORY-$RELEASE_VERSION.tgz"

  helm push "$CHART_PACKAGE" "oci://$CHART_REGISTRY"
fi
