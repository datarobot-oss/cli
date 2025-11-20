# GitHub Actions Workflows

This directory contains GitHub Actions workflows for the DR CLI project. We use reusable workflows to reduce duplication and improve maintainability.

## Reusable Workflows

Reusable workflows (prefixed with `.`) contain common patterns used across multiple workflows:

### `.build-matrix.yaml`
Builds the DR CLI binary across multiple operating systems (Linux, macOS, Windows).

**Inputs:**
- `os-matrix` (string, default: `["ubuntu-latest", "macos-latest", "windows-latest"]`) - OS matrix for builds
- `go-version` (string, default: `1.25.3`) - Go version to use
- `upload-artifact` (boolean, default: `false`) - Whether to upload artifacts
- `artifact-name-prefix` (string, default: `dr`) - Prefix for artifact names

**Usage:**
```yaml
jobs:
  build:
    uses: ./.github/workflows/.build-matrix.yaml
    with:
      go-version: '1.25.3'
      upload-artifact: true
    secrets: inherit
```

### `.build-windows.yaml`
Builds Windows binary using GoReleaser (cross-compiled from Ubuntu).

**Inputs:**
- `go-version` (string, default: `1.25.3`) - Go version to use
- `artifact-name` (string, default: `dr-windows`) - Name for the artifact
- `ref` (string, optional) - Git ref to checkout (useful for fork PRs)

**Usage:**
```yaml
jobs:
  build-windows:
    uses: ./.github/workflows/.build-windows.yaml
    with:
      artifact-name: 'dr-windows'
    secrets: inherit
```

### `.smoke-tests-matrix.yaml`
Runs smoke tests on Linux and macOS.

**Inputs:**
- `os-matrix` (string, default: `["ubuntu-latest", "macos-latest"]`) - OS matrix
- `go-version` (string, default: `1.25.3`) - Go version to use
- `ref` (string, optional) - Git ref to checkout

**Secrets (required):**
- `DR_API_TOKEN` - DataRobot API token for testing
- `GITHUB_TOKEN` - GitHub token for authentication

**Usage:**
```yaml
jobs:
  smoke-test:
    uses: ./.github/workflows/.smoke-tests-matrix.yaml
    with:
      go-version: '1.25.3'
    secrets:
      DR_API_TOKEN: ${{ secrets.DR_API_TOKEN }}
      GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

### `.windows-smoke-test.yaml`
Runs smoke tests on Windows using a pre-built binary artifact.

**Inputs:**
- `artifact-name` (string, default: `dr-windows`) - Name of the Windows binary artifact
- `ref` (string, optional) - Git ref to checkout

**Secrets (required):**
- `DR_API_TOKEN` - DataRobot API token for testing
- `GITHUB_TOKEN` - GitHub token for authentication

**Usage:**
```yaml
jobs:
  windows-smoke-test:
    needs: build-windows
    uses: ./.github/workflows/.windows-smoke-test.yaml
    with:
      artifact-name: 'dr-windows'
    secrets:
      DR_API_TOKEN: ${{ secrets.DR_API_TOKEN }}
      GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

### `.installation-tests-matrix.yaml`
Tests installation scripts across all platforms (Linux, macOS, Windows).

**Inputs:**
- `os-matrix` (string, default: `["ubuntu-latest", "macos-latest", "windows-latest"]`) - OS matrix

**Secrets (required):**
- `GITHUB_TOKEN` - GitHub token for authentication

**Usage:**
```yaml
jobs:
  installation-tests:
    uses: ./.github/workflows/.installation-tests-matrix.yaml
    secrets:
      GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

### `.setup.yaml`
Basic environment setup (checkout, Go, Taskfile, caching).

**Inputs:**
- `go-version` (string, default: `1.25.3`) - Go version to use
- `install-taskfile` (boolean, default: `true`) - Whether to install Taskfile
- `setup-cache` (boolean, default: `false`) - Whether to setup Go cache

**Note:** This workflow is currently not heavily used as most workflows need more specialized setup steps.

## Main Workflows

### `checks.yaml`
Runs on pull requests to main. Performs:
- Linting (golangci-lint, goreleaser check)
- Unit tests
- Copyright/license checks
- Code generation verification
- Multi-platform builds (using `.build-matrix.yaml`)
- Conditional completion tests (when completion code changes)

### `smoke-tests.yaml`
Daily smoke tests (every 4 hours on weekdays) and runs on pushes to main:
- Builds Windows binary with GoReleaser
- Runs smoke tests on Linux and macOS
- Runs smoke tests on Windows
- **Tests installation scripts on all platforms** (validates the public install scripts)
- **Notifies Slack on failure** - Sends alert when any smoke test job fails

### `smoke-tests-on-demand.yaml`
Triggered by PR labels (`run-smoke-tests` or `go`):
- Builds Windows binary
- Runs smoke tests on Linux and Windows
- Posts results as PR comments
- Auto-removes `run-smoke-tests` label after completion
- **Note:** Does NOT run installation tests (those run only on main/schedule to avoid testing unreleased code)

### `fork-smoke-tests.yaml`
Runs on fork PRs when labeled with `approved-for-smoke-tests`:
- Performs security scans (Trivy, gosec)
- Builds Windows binary from fork PR code
- Runs smoke tests on Linux and Windows with manual approval
- Posts results and removes label

### `release.yaml`
Triggered by version tags (`v*.*.*`):
- Builds and releases binaries with GoReleaser
- Updates Homebrew tap
- Creates GitHub release
- **Notifies Slack on success** - Announces new release with version and release notes link
- **Notifies Slack on failure** - Alerts when release process fails

### `security-scan.yaml`
Runs on PRs and pushes to main:
- Trivy vulnerability scanning
- Uploads results to GitHub Security tab

## Slack Notifications

Several workflows send Slack notifications to keep the team informed:

- **`release.yaml`**: Sends notifications on both successful releases and failures
- **`smoke-tests.yaml`**: Sends notifications only when smoke tests fail on main/schedule

To enable Slack notifications, add `SLACK_WEBHOOK_URL` as a repository secret:
1. Go to your Slack workspace → Apps → Incoming Webhooks
2. Create a new webhook for your desired channel
3. Add the webhook URL as `SLACK_WEBHOOK_URL` in GitHub repository secrets (Settings → Secrets and variables → Actions → New repository secret)

## Benefits of Reusable Workflows

1. **DRY Principle**: Common patterns defined once, used everywhere
2. **Consistency**: All workflows use the same setup steps and configurations
3. **Maintainability**: Update once, apply everywhere
4. **Readability**: Main workflows focus on orchestration, not implementation details
5. **Testing**: Reusable workflows can be tested independently
