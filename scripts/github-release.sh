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

if git ls-remote --tags origin | grep -q "refs/tags/$RELEASE_VERSION"; then
    echo "Tag '$RELEASE_VERSION' exists in remote, you should manually delete it from github"
    exit 1
fi

if git ls-remote --heads origin | grep -q "refs/heads/release-$RELEASE_VERSION"; then
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

# Build the deploy.yaml
make build-deploy
cp "deploy.yaml" "files/controller-installation/deploy-$RELEASE_VERSION.yaml"

#only keep 4 recent versions deploy.yaml, removing the oldest one
VERSION_TO_REMOVE=$(git tag --sort=v:refname | grep -v 'rc' | tail -n 5 | head -n 1)
rm -f "$WORKSPACE_DIR/files/controller-installation/deploy-$VERSION_TO_REMOVE.yaml"

# Crete a new release branch, tag and push the changes
git checkout -b release-$RELEASE_VERSION
git add "$WORKSPACE_DIR/README.md" \
  "$WORKSPACE_DIR/config/manager/kustomization.yaml" \
  "$WORKSPACE_DIR/helm/Chart.yaml" \
  "$WORKSPACE_DIR/helm/values.yaml" \
  "$WORKSPACE_DIR/files/controller-installation/deploy-$RELEASE_VERSION.yaml" \
  "$WORKSPACE_DIR/files/controller-installation/deploy-$VERSION_TO_REMOVE.yaml" \
  "$WORKSPACE_DIR/docs/guides/deploy.md"
git commit -m "Release artifacts for release $RELEASE_VERSION"
git push origin release-$RELEASE_VERSION
git tag -a  $RELEASE_VERSION -m "Release artifacts for release $RELEASE_VERSION"
git push origin $RELEASE_VERSION

# Create a PR
gh pr create --title "Release artifacts for $RELEASE_VERSION" --body "Release artifacts for $RELEASE_VERSION" \
 --base aws:main --head aws:release-$RELEASE_VERSION \
  --repo aws/aws-application-networking-k8s --web


