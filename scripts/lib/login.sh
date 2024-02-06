#!/usr/bin/env bash

# login.sh contains functions for logging into container repositories.
# Note: These functions are not placed inside tools specific file like
# helm.sh, because those files ensure binaries(ex: kind) that are not
# really needed for simple actions like repository login.
# A future refactor can make those files more modular and then login
# functions can be refactored in tool specific file like docker.sh
# and helm.sh

perform_docker_and_helm_login() {
  local full_url="$1"
  local registry_url=$(echo "$full_url" | cut -d'/' -f1)

  # Check if the registry URL is for the public ECR
  if [ "$registry_url" = "public.ecr.aws" ]; then
    local __pw=$(aws ecr-public get-login-password --region us-east-1)
  else
    local __pw=$(aws ecr get-login-password)
  fi
  echo "$__pw" | docker login --username AWS --password-stdin $registry_url
  export HELM_EXPERIMENTAL_OCI=1
  echo "$__pw" | helm registry login -u AWS --password-stdin $registry_url
}

ensure_binaries() {
    check_is_installed "aws"
    check_is_installed "docker"
    check_is_installed "helm"
}

ensure_binaries
