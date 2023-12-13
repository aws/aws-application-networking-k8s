#!/usr/bin/env bash

set -eo pipefail

DEFAULT_IMAGE_REPOSITORY="public.ecr.aws/aws-application-networking-k8s/aws-gateway-controller"
DEFAULT_CHART_REGISTRY="public.ecr.aws/aws-application-networking-k8s"
DEFAULT_CHART_REPOSITORY="aws-gateway-controller-chart"

USAGE="
Usage:
  $(basename "$0")

Publishes the controller image and helm chart for the AWS Gateway controller.

When run as part of a CI job, when the source code repo is tagged with a semver
format '^v\d+\.\d+\.\d+$'

Environment variables:
  SKIP_BUILD_IMAGE:         If set to 1, does not build and publish the
                            controller container image, only the Helm chart.
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
  CHART_TAG:                Controller Helm Chart tag
                            Default: Based on \$PULL_BASE_REF
"

# find out the service name and semver tag from the CI environment variable if not specified 
VERSION=${VERSION:-$PULL_BASE_REF}

# Important Directory references based on prowjob configuration.
SCRIPTS_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
WORKSPACE_DIR=$SCRIPTS_DIR/..

# Check all the dependencies are present in container.
source "$SCRIPTS_DIR/lib/common.sh"
source "$SCRIPTS_DIR/lib/login.sh"
check_is_installed git
check_is_installed jq
check_is_installed yq

if [[ $PULL_BASE_REF = stable ]]; then
  pushd $WORKSPACE_DIR 1>/dev/null
  echo "Triggering for the stable branch"
  _semver_tag=$(git describe --tags --abbrev=0 2>/dev/null)
  if [[ $? -ne 0 ]]; then
    echo "Unable to find semver tag on the 'stable' branch"
    exit 2
  fi
  echo "Semver tag on stable branch is $_semver_tag"

  if ! (echo "$_semver_tag" | grep -Eq "^v[0-9]+\.[0-9]+\.[0-9]+$"); then
    echo "semver tag on stable branch should have format ^v[0-9]+\.[0-9]+\.[0-9]+$"
    exit 2
  fi

  _major_version=$(echo "$_semver_tag" | cut -d"." -f1)
  if [[ -z "$_major_version" ]]; then
    echo "Unable to determine major version from latest semver tag on 'stable' branch"
    exit 2
  fi

  VERSION="$_major_version-stable"
  popd 1>/dev/null
else
  echo "release-controller.sh] Validating that controller image repository & tag in release artifacts is consistent with semver tag"
  pushd $WORKSPACE_DIR/helm 1>/dev/null
    _repository=$(yq eval ".image.repository" values.yaml)
    _image_tag=$(yq eval ".image.tag" values.yaml)
    if [[ $_repository != public.ecr.aws/aws-application-networking-k8s/aws-gateway-controller ]]; then
      echo "release-controller.sh] [ERROR] 'image.repository' value in release artifacts should be $DEFAULT_IMAGE_REPOSITORY. Current value: $_repository"
      exit 1
    fi
    if [[ $_image_tag != $VERSION ]]; then
      echo "release-controller.sh] [ERROR] 'image.tag' value in release artifacts should be $VERSION. Current value: $_image_tag"
      exit 1
    fi
    echo "release-controller.sh] Validation successful"
  popd 1>/dev/null
fi

IMAGE_REPOSITORY=${IMAGE_REPOSITORY:-$DEFAULT_IMAGE_REPOSITORY}
IMAGE_TAG=${IMAGE_TAG:-$VERSION}
CHART_REPOSITORY=${CHART_REPOSITORY:-$DEFAULT_CHART_REPOSITORY}
CHART_REGISTRY=${CHART_REGISTRY:-$DEFAULT_CHART_REGISTRY}

echo "VERSION is $VERSION"

#  This Role has permission to publish to the image and Helm chart repositories on ECR public registry in 606627242267 account.
#  Make sure the aws principal you use to run this script has permission to assume this role
ECR_PUBLISH_ROLE_ARN=arn:aws:iam::606627242267:role/ECRPublisher
ASSUME_COMMAND=$(aws --output json sts assume-role --role-arn $ECR_PUBLISH_ROLE_ARN --role-session-name 'publish-images' --duration-seconds 3600 | jq -r '.Credentials | "export AWS_ACCESS_KEY_ID=\(.AccessKeyId)\nexport AWS_SECRET_ACCESS_KEY=\(.SecretAccessKey)\nexport AWS_SESSION_TOKEN=\(.SessionToken)\n"')
eval $ASSUME_COMMAND
echo "release-controller.sh] [SETUP] Assumed ECR_PUBLISH_ROLE_ARN"

# Setup the destination repository for docker and helm
perform_docker_and_helm_login

# Do not rebuild controller image for stable releases
if [[ "$SKIP_IMAGE_BUILD" != "1" ]]; then
    if ! (echo "$VERSION" | grep -Eq "stable$"); then
      # Determine parameters for docker-build command

      cd "$WORKSPACE_DIR"

      CONTROLLER_GIT_COMMIT=$(git rev-parse HEAD)
      QUIET=${QUIET:-"false"}
      BUILD_DATE=$(date +%Y-%m-%dT%H:%M)
      CONTROLLER_IMAGE_DOCKERFILE_PATH=$WORKSPACE_DIR/Dockerfile

      IMG="$IMAGE_REPOSITORY:$IMAGE_TAG"
      DOCKER_BUILD_CONTEXT="$WORKSPACE_DIR"

      if [[ $QUIET = "false" ]]; then
          echo "building aws-gateway-controller image with tag: ${IMG}"
          echo " git commit: $CONTROLLER_GIT_COMMIT"
      fi

      # build controller image
      docker build -t "$IMG" \
        -f "$CONTROLLER_IMAGE_DOCKERFILE_PATH" \
        --build-arg service_controller_git_version="$VERSION" \
        --build-arg service_controller_git_commit="$CONTROLLER_GIT_COMMIT" \
        --build-arg build_date="$BUILD_DATE" \
        "${DOCKER_BUILD_CONTEXT}"

      if [ $? -ne 0 ]; then
        exit 2
      fi

      echo "Pushing aws-gateway-controller image with tag: ${IMG}"

      docker push "${IMG}"

      if [ $? -ne 0 ]; then
        exit 2
      fi
    fi
fi

export HELM_EXPERIMENTAL_OCI=1

echo "Generating Helm chart package for $CHART_REGISTRY/$CHART_REPOSITORY@$VERSION ... "
helm package "$WORKSPACE_DIR"/helm/
# Path to tarballed package (eg. `aws-application-networking-controller-v0.0.1.tgz`)
CHART_PACKAGE="$CHART_REPOSITORY-$VERSION.tgz"

helm push "$CHART_PACKAGE" "oci://$CHART_REGISTRY"
