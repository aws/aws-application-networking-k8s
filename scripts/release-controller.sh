#!/usr/bin/env bash

set -eo pipefail

DEFAULT_IMAGE_REPOSITORY="public.ecr.aws/aws-application-networking-k8s/aws-gateway-controller"
DEFAULT_CHART_REGISTRY="public.ecr.aws/aws-application-networking-k8s"
DEFAULT_CHART_REPOSITORY="aws-gateway-controller-chart"

USAGE="
Usage:
  $(basename "$0")

Publishes the controller image and helm chart for the AWS Gateway controller.

Environment variables:
  SKIP_BUILD_IMAGE:         If set to 1, does not build and publish the
                            controller container image, only the Helm chart.
  SKIP_BUILD_CHART:         If set to 1, does not build and publish the
                            Helm chart, only the controller container image.
  ECR_PUBLISH_ROLE_ARN:     A role ARN on the ECR repository account to assume.
                            Requires IAM permissions that allows pushing images/charts.
  PULL_BASE_REF:            The value of tag on the source code repository that triggered the
                            postsubmit CI job. The value will either be in the format '^v\d+\.\d+\.\d+$'
                            or 'stable'.
  IMAGE_REPOSITORY:         Name for the ECR Public repository to push controller images to
                            Default: $DEFAULT_IMAGE_REPOSITORY
  IMAGE_TAG:                Controller container image tag
                            Default: \$PULL_BASE_REF
  QUIET:                    Build controller container image quietly (<true|false>)
                            Default: false
  CHART_REGISTRY:           Name for the OCI Artifact Registry to push to
                            Default: $DEFAULT_CHART_REGISTRY
  CHART_REPOSITORY:         Name for the Helm repository to push to
                            Default: $DEFAULT_CHART_REPOSITORY
"

# find out the service name and semver tag from the CI environment variable if not specified 
VERSION=${VERSION:-$PULL_BASE_REF}

IMAGE_REPOSITORY=${IMAGE_REPOSITORY:-$DEFAULT_IMAGE_REPOSITORY}
IMAGE_TAG=${IMAGE_TAG:-$VERSION}
CHART_REPOSITORY=${CHART_REPOSITORY:-$DEFAULT_CHART_REPOSITORY}
CHART_REGISTRY=${CHART_REGISTRY:-$DEFAULT_CHART_REGISTRY}

echo "VERSION is $VERSION"

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
echo "release-controller.sh] Assumed $ECR_PUBLISH_ROLE_ARN"

# Setup the destination repository for docker and helm
perform_docker_and_helm_login $IMAGE_REPOSITORY

if [[ "$SKIP_BUILD_IMAGE" != "1" ]]; then
  # Determine parameters for docker-build command

  cd "$WORKSPACE_DIR"

  CONTROLLER_GIT_COMMIT=$(git rev-parse HEAD)
  QUIET=${QUIET:-"false"}
  BUILD_DATE=$(date +%Y-%m-%dT%H:%M)
  CONTROLLER_IMAGE_DOCKERFILE_PATH=$WORKSPACE_DIR/Dockerfile

  IMG="$IMAGE_REPOSITORY:$IMAGE_TAG"
  DOCKER_BUILD_CONTEXT="$WORKSPACE_DIR"

  if [[ $QUIET = "false" ]]; then
      echo "building and pushing aws-gateway-controller image with tag: ${IMG}"
      echo " git commit: $CONTROLLER_GIT_COMMIT"
  fi

  # build controller image
  if [[ $CI = "true" ]]; then
    docker buildx build -t "$IMG" \
      -f "$CONTROLLER_IMAGE_DOCKERFILE_PATH" \
      --build-arg service_controller_git_version="$VERSION" \
      --build-arg service_controller_git_commit="$CONTROLLER_GIT_COMMIT" \
      --build-arg build_date="$BUILD_DATE" \
      --cache-from type=gha \
      --cache-to type=gha,mode=max \
      --push \
      "${DOCKER_BUILD_CONTEXT}"
  else
    docker buildx build -t "$IMG" \
      -f "$CONTROLLER_IMAGE_DOCKERFILE_PATH" \
      --build-arg service_controller_git_version="$VERSION" \
      --build-arg service_controller_git_commit="$CONTROLLER_GIT_COMMIT" \
      --build-arg build_date="$BUILD_DATE" \
      --push \
      "${DOCKER_BUILD_CONTEXT}"
  fi

  if [ $? -ne 0 ]; then
    exit 2
  fi
fi

if [[ "$SKIP_BUILD_CHART" != "1" ]]; then
  export HELM_EXPERIMENTAL_OCI=1

  echo "Generating Helm chart package for $CHART_REGISTRY/$CHART_REPOSITORY@$VERSION ... "
  helm package "$WORKSPACE_DIR"/helm/
  # Path to tarballed package (eg. `aws-application-networking-controller-v0.0.1.tgz`)
  CHART_PACKAGE="$CHART_REPOSITORY-$VERSION.tgz"

  helm push "$CHART_PACKAGE" "oci://$CHART_REGISTRY"
fi
