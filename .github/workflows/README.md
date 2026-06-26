# GitHub Actions Workflows

This directory contains GitHub Actions workflows for the DR CLI project. We use composite actions and reusable workflows to reduce duplication and improve maintainability.

## Composite Actions

Step-level building blocks live under [`.github/actions/`](../actions/). Unlike
reusable workflows (which share whole jobs), composite actions share *steps*, so
they replace the copy-pasted checkout→Go→Task setup across jobs. They run in the
caller's job, so the repo must be checked out **before** they are used.

> **Important: reusable workflows must use the full repo path.**
>
> In top-level workflows (e.g. `checks.yaml`), reference composite actions with
> the local path: `./.github/actions/setup`. In **reusable workflows** (those
> with `on: workflow_call`), use the full repo path instead:
> `datarobot-oss/cli/.github/actions/setup@main`.
>
> GitHub resolves `uses:` paths **before** `actions/checkout` runs, so local
> paths (`./`) don't work in reusable workflows — the `.github/actions/`
> directory isn't populated yet. This is a known GitHub Actions limitation
> ([community discussion #18601](https://github.com/orgs/community/discussions/18601)).
>
> We use `@main` rather than pinning to a SHA because these are internal
> same-repo utility actions. SHA pinning creates a chicken-and-egg problem:
> you can't know the new SHA until the change merges to `main`, so PR CI would
> test against the old version. The GitHub Actions Team is developing a native
> `$/` syntax ([discussion #26245](https://github.com/orgs/community/discussions/26245))
> to resolve same-repo actions at the same SHA automatically, but it hasn't
> shipped yet.

### `setup`
Sets up Go (version pinned by `go.mod` via `go-version-file`) and, optionally,
Taskfile, with `setup-go`'s built-in module/build caching.

**Inputs:**
- `cache` (string, default: `'true'`) - Enable `setup-go` caching
- `install-task` (string, default: `'true'`) - Whether to install Taskfile

```yaml
steps:
  - uses: actions/checkout@<sha>
  - uses: ./.github/actions/setup          # go + task + cache
  # or, when Task isn't needed (e.g. GoReleaser-driven builds):
  - uses: ./.github/actions/setup
    with:
      install-task: 'false'
```

### `install-deps`
OS-aware install of the smoke-test prerequisites: `expect` + `bash-completion`
(apt-get on Linux, Homebrew on macOS) and `yq`.

### `install-dr-bin`
Builds the `dr` CLI, installs it via `install.sh`, and adds `~/.local/bin` to
PATH (Unix only — Windows smoke tests download a prebuilt artifact instead).

## Reusable Workflows

Reusable workflows (prefixed with `.`) contain common patterns used across multiple workflows. All Go setup goes through the `setup` composite action, so the Go version comes from `go.mod` and is no longer a workflow input.

### `.build-windows.yaml`
Builds Windows binary using GoReleaser (cross-compiled from Ubuntu).

**Inputs:**
- `artifact-name` (string, default: `dr-windows`) - Name for the artifact
- `ref` (string, optional) - Git ref to checkout (useful for fork PRs)

**Usage:**
```yaml
jobs:
  build-windows:
    uses: ./.github/workflows/.build-windows.yaml
    with:
      artifact-name: 'dr-windows'
```

### `.smoke-tests-matrix.yaml`
Runs smoke tests on Linux and macOS.

**Inputs:**
- `os-matrix` (string, default: `["ubuntu-latest", "macos-latest"]`) - OS matrix
- `ref` (string, optional) - Git ref to checkout

**Secrets (required):**
- `DR_API_TOKEN` - DataRobot API token for testing
- `GITHUB_TOKEN` - GitHub token for authentication

**Usage:**
```yaml
jobs:
  smoke-test:
    uses: ./.github/workflows/.smoke-tests-matrix.yaml
    secrets:
      DR_API_TOKEN: ${{ secrets.DR_API_TOKEN }}
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

> **Note:** environment setup (checkout-less Go + Task + cache) now lives in the
> [`setup` composite action](#setup), which replaced the former `.setup.yaml`
> reusable workflow.

## Main Workflows

### `checks.yaml`
Runs on pull requests to main. Performs:
- Linting (golangci-lint, goreleaser check)
- Unit tests
- Copyright/license checks
- Code generation verification
- Multi-platform build (single Ubuntu runner cross-compiling all targets via GoReleaser)
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

### `smoke-tests-gate.yaml`
Runs on all PRs targeting `main` to set the required **Smoke Tests** commit status:
- **Non-fork PRs**: auto-sets status to `success` (smoke tests are optional, run via labels/commands)
- **Fork PRs**: skips status creation (read-only token). The missing required check blocks merge until a maintainer runs (`/approve-smoke-tests`) or skips (`/skip-smoke-tests`) the tests

### `fork-smoke-tests.yaml`
Triggered manually via `workflow_dispatch` by a maintainer (from the Actions tab):
- Accepts a PR number and optional commit SHA as inputs
- Performs security scans (Trivy, gosec)
- Builds Windows binary from fork PR code
- Runs smoke tests on Linux and Windows
- Posts results as PR comments
- Updates the **Smoke Tests** commit status to `success` or `failure`

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

## PR Automation: Comment-Commands and Labels

This repository supports automation for PRs using comment-commands (slash commands) and labels.

### Comment-Commands (Slash Commands)

Trigger workflows by commenting on a PR:

- `/trigger-smoke-test` or `/trigger-test-smoke` - Run smoke tests on this PR (non-fork PRs only)
- `/trigger-install-test` or `/trigger-test-install` - Run installation tests on this PR (non-fork PRs only)
- `/approve-smoke-tests` or `/approve-fork-tests` - Run smoke tests for a fork PR (maintainers only)
- `/skip-smoke-tests` - Mark the **Smoke Tests** required check as passed without running tests (maintainers only)

### Labels for Regular PRs

Apply labels to PRs to trigger workflows:

- `run-smoke-tests` or `go` - Triggers `smoke-tests-on-demand.yaml`
  - Builds Windows binary
  - Runs smoke tests on Linux and Windows
  - Posts results as PR comments
  - Auto-removes label after completion
  - **Note:** This only works for PRs from the main repository, not forked PRs

### Forked PRs

Forked PRs block merge via a required **Smoke Tests** commit status that is never auto-set. The `fork-smoke-tests.yaml` workflow uses `workflow_dispatch` (not `pull_request_target`) to avoid secrets leakage:

**Process for Forked PRs:**
1. External contributor opens a PR from their fork
2. The **Smoke Tests** required check appears as "Expected — Waiting for status to be reported", blocking merge
3. Maintainer reviews the code changes for security concerns, then either:
   - Comments `/approve-smoke-tests` to trigger security scans + smoke tests (results auto-update the check)
   - Comments `/skip-smoke-tests` to bypass the check without running tests
4. Results are posted as PR comments and the commit status is updated

**For external contributors:** the `run-smoke-tests` label and `/trigger-smoke-test` command won't work on fork PRs. Please comment requesting a maintainer review.

## Benefits of Reusable Workflows

1. **DRY Principle**: Common patterns defined once, used everywhere
2. **Consistency**: All workflows use the same setup steps and configurations
3. **Maintainability**: Update once, apply everywhere
4. **Readability**: Main workflows focus on orchestration, not implementation details
5. **Testing**: Reusable workflows can be tested independently
