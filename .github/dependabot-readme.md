# Dependabot Configuration for Go Modules

This repository is configured to use Dependabot for automated dependency updates with `go mod tidy` support.

## Configuration Files

1. `.github/dependabot.yml` - Configures Dependabot to check for Go module updates weekly
2. `.github/workflows/dependabot-go-mod-tidy.yml` - GitHub Actions workflow that runs `go mod tidy` on Dependabot PRs

## How It Works

1. Dependabot creates PRs to update Go dependencies according to the schedule in `dependabot.yml`
2. When a PR is created that modifies `go.mod` or `go.sum`, the workflow is triggered
3. The workflow checks if the PR was created by Dependabot
4. If so, it runs `go mod tidy` and commits any changes back to the PR

## Required Repository Settings

For the workflow to function properly, you need to configure the repository to allow Dependabot to trigger workflows with write permissions:

1. Go to the repository on GitHub
2. Navigate to Settings > Code and automation > Actions > General
3. Scroll down to "Workflow permissions"
4. Enable "Read and write permissions"
5. Check "Allow GitHub Actions to create and approve pull requests"
6. Save changes

Additionally, you need to configure Dependabot to have write access to PRs:

1. Go to the repository on GitHub
2. Navigate to Settings > Code and automation > Actions > General
3. Scroll down to "Workflow permissions from pull requests"
4. Select "Allow Dependabot to run workflows"
5. Save changes

## Troubleshooting

If the workflow isn't running or isn't able to commit changes:

1. Check that the repository settings are configured as described above
2. Verify that the PR was created by Dependabot
3. Check the workflow run logs for any errors
