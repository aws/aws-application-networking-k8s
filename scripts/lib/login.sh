#!/usr/bin/env bash

# login.sh contains functions for logging into container repositories.
# Note: These functions are not placed inside tools specific file like
# helm.sh, because those files ensure binaries(ex: kind) that are not
# really needed for simple actions like repository login.
# A future refactor can make those files more modular and then login
# functions can be refactored in tool specific file like docker.sh
# and helm.sh

perform_docker_and_helm_login() {
  #ecr-public only exists in us-east-1 so use that region specifically
  local __pw=$(aws ecr-public get-login-password --region us-east-1)
  echo "$__pw" | docker login -u AWS --password-stdin public.ecr.aws
  export HELM_EXPERIMENTAL_OCI=1
  echo "$__pw" | helm registry login -u AWS --password-stdin public.ecr.aws
}

ensure_binaries() {
    check_is_installed "aws"
    check_is_installed "docker"
    check_is_installed "helm"
}

ensure_binaries
