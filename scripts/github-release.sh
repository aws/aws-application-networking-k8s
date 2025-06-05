#!/bin/bash

USAGE="
Usage:
  $(basename "$0")

The project maintainers can run this script from local to push the release branch, tag and send a PR for new version release.


Environment variables:
  $RELEASE_VERSION:         The version to be released. The value need to in the format '^v\d+\.\d+\.\d+$'
"

source ./scripts/lib/common.sh

SCRIPTS_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
WORKSPACE_DIR=$(realpath "$SCRIPTS_DIR/..")

# Check if SSH agent is running
if [ -z "$SSH_AUTH_SOCK" ]; then
  echo "SSH agent is not running. This script requires SSH agent to avoid multiple passphrase prompts."
  echo "Please run 'eval \$(ssh-agent -s)' and 'ssh-add' before running this script."
  exit 1
fi

if [ -z "$RELEASE_VERSION" ]; then
  echo "Environment variable RELEASE_VERSION is not set. Please set it to run this script."
  exit 1
fi

OLD_VERSION=$(grep 'version:' "$WORKSPACE_DIR"/helm/Chart.yaml | awk '{print $2}')
echo "Old version is $OLD_VERSION"

if  [ "$OLD_VERSION" == "$RELEASE_VERSION" ]; then
  echo "No new version to release"
  exit 0
fi

git fetch origin --tags --force
git pull origin main
git checkout main

if git rev-parse "$RELEASE_VERSION" >/dev/null 2>&1; then
    echo "Tag '$RELEASE_VERSION' exists locally, deleting it"
    git tag -d "$RELEASE_VERSION"
fi

if git rev-parse --verify release-$RELEASE_VERSION >/dev/null 2>&1; then
    echo "Branch 'release-$RELEASE_VERSION' exists locally, deleting it"
    git branch -D release-$RELEASE_VERSION
fi

if git ls-remote --tags origin | grep -q "refs/tags/$RELEASE_VERSION$"; then
    echo "Tag '$RELEASE_VERSION' exists in remote, you should manually delete it from github"
    exit 1
fi

if git ls-remote --heads origin | grep -q "refs/heads/release-$RELEASE_VERSION$"; then
    echo "Branch 'release-$RELEASE_VERSION' exists in remote, you should manually delete it from github"
    exit 1
fi

# Substitute the version number in files
sed_inplace "tag/$OLD_VERSION" "tag/$RELEASE_VERSION" "$WORKSPACE_DIR"/README.md
sed_inplace "newTag: $OLD_VERSION" "newTag: $RELEASE_VERSION" "$WORKSPACE_DIR"/config/manager/kustomization.yaml
sed_inplace "version: $OLD_VERSION" "version: $RELEASE_VERSION" "$WORKSPACE_DIR"/helm/Chart.yaml
sed_inplace "appVersion: $OLD_VERSION" "appVersion: $RELEASE_VERSION" "$WORKSPACE_DIR"/helm/Chart.yaml
sed_inplace "tag: $OLD_VERSION" "tag: $RELEASE_VERSION" "$WORKSPACE_DIR"/helm/values.yaml
sed_inplace "deploy-$OLD_VERSION.yaml" "deploy-$RELEASE_VERSION.yaml" "$WORKSPACE_DIR"/docs/guides/deploy.md
sed_inplace "--version=$OLD_VERSION" "--version=$RELEASE_VERSION" "$WORKSPACE_DIR"/docs/guides/deploy.md
sed_inplace "--version=$OLD_VERSION" "--version=$RELEASE_VERSION" "$WORKSPACE_DIR"/docs/guides/getstarted.md
sed_inplace "mike deploy $OLD_VERSION" "mike deploy $RELEASE_VERSION" "$WORKSPACE_DIR"/.github/workflows/publish-doc.yaml


# Build the deploy.yaml
make build-deploy
cp "deploy.yaml" "files/controller-installation/deploy-$RELEASE_VERSION.yaml"

#only keep 4 recent versions deploy.yaml, removing the oldest one
VERSION_TO_REMOVE=$(git tag --sort=v:refname | grep -v 'rc' | tail -n 5 | head -n 1)
if [ -f "$WORKSPACE_DIR/files/controller-installation/deploy-$VERSION_TO_REMOVE.yaml" ]; then
  echo "Removing old deploy file: deploy-$VERSION_TO_REMOVE.yaml"
  rm -f "$WORKSPACE_DIR/files/controller-installation/deploy-$VERSION_TO_REMOVE.yaml"
else
  echo "Old deploy file not found: deploy-$VERSION_TO_REMOVE.yaml. Skipping removal."
fi

# Create a new release branch, tag and push the changes
git checkout -b release-$RELEASE_VERSION

# Add all modified files
git add "$WORKSPACE_DIR/README.md" \
  "$WORKSPACE_DIR/config/manager/kustomization.yaml" \
  "$WORKSPACE_DIR/helm/Chart.yaml" \
  "$WORKSPACE_DIR/helm/values.yaml" \
  "$WORKSPACE_DIR/files/controller-installation/deploy-$RELEASE_VERSION.yaml" \
  "$WORKSPACE_DIR/docs/guides/deploy.md" \
  "$WORKSPACE_DIR/docs/guides/getstarted.md" \
  "$WORKSPACE_DIR/.github/workflows/publish-doc.yaml"

# Add the old deploy file if it exists and was removed
if [ -f "$WORKSPACE_DIR/files/controller-installation/deploy-$VERSION_TO_REMOVE.yaml" ]; then
  git add "$WORKSPACE_DIR/files/controller-installation/deploy-$VERSION_TO_REMOVE.yaml"
fi

git commit -m "Release artifacts for release $RELEASE_VERSION"
git push origin release-$RELEASE_VERSION
git tag -a $RELEASE_VERSION -m "Release artifacts for release $RELEASE_VERSION"
git push origin $RELEASE_VERSION

# Check if GitHub CLI is authenticated and has necessary permissions
if ! gh auth status &>/dev/null; then
  echo "GitHub CLI is not authenticated. Please run 'gh auth login' before running this script."
  exit 1
fi

# Create a PR
echo "Creating a PR for release $RELEASE_VERSION..."
gh pr create --title "Release artifacts for $RELEASE_VERSION" --body "Release artifacts for $RELEASE_VERSION" \
 --base aws:main --head aws:release-$RELEASE_VERSION \
  --repo aws/aws-application-networking-k8s --web

echo "Release process completed successfully!"
