# AGENTS.md

## Project Overview

DataRobot CLI (`dr`) - A Go-based command-line interface for managing DataRobot custom applications with OAuth integration, template management, and task execution capabilities.

## Build & Development Commands

Use Taskfile tasks rather than raw Go commands:

| Command         | Description                                 |
| --------------- | ------------------------------------------- |
| `task build`    | Build the CLI binary (outputs to ./dist/dr) |
| `task lint`     | Check for lint issues (read-only)           |
| `task delint`   | Fix lint and formatting issues              |
| `task test`     | Run tests with race detection and coverage  |
| `task dev-init` | Initialize development environment          |
| `task run`      | Run CLI directly via `go run`               |

## Testing

- Run tests: `task test`
- Tests use `testify/assert` for assertions
- Test files follow `*_test.go` naming convention
- If DR_API_TOKEN is set, run smoke tests: `task smoke-test` (but ask for permission before using a real API token)

**Go Version Requirement:** Tests run with the `-race` flag for data race detection. The race runtime must match your Go compiler version exactly. If you see errors like `compile: version "go1.X.Y" does not match go tool version "go1.X.Z"`, ensure your installed Go version matches the version in `go.mod` (run `brew upgrade go` or adjust `go.mod` accordingly).

## Command Naming Conventions

- **Commands must use singular names** (e.g., `template`, `dependency`, `plugin`)
- Plural aliases are acceptable for backward compatibility (e.g., `templates`, `dependencies`, `plugins`)
- Maintain consistency across all top-level and sub-commands

## Code Style Requirements

### Go Whitespace Rules (Critical)

All code must pass `golangci-lint` with zero errors. Follow these whitespace rules:

1. **Never cuddle declarations** - Always add a blank line before `var`, `const`, `type` declarations when they follow other statements
2. **Separate statement types** - Add blank lines between different statement types (assign, if, for, return, etc.)
3. **Blank line after block start** - Add blank line after opening braces of functions/blocks when followed by declarations
4. **Blank line before multi-line statements** - Add blank line before if/for/switch statements

Example of correct spacing:

```go
func example() {
    x := 1

    if x > 0 {
        y := 2

        fmt.Println(y)
    }

    var result string

    result = "done"

    return result
}
```

### TUI Standards

- Always use `tui.Run()` to execute TUI models for global Ctrl-C handling and debug logging
- Always wrap new TUI models with the InterruptibleModel from the `tui` package to ensure global Ctrl-C handling
- Reuse existing TUI components from `tui` package or Bubbles library (https://github.com/charmbracelet/bubbles)
- Use styles from `tui/styles.go` for consistency
- When `--debug` is enabled, logs are written to `.dr-tui-debug.log`

## Quality Tools

**`task lint`** (check-only, non-modifying):
- `go mod tidy -diff` - checks if go.mod/go.sum need updates
- `gofumpt -l -d` - lists formatting issues and shows diffs
- `go vet` - checks for suspicious constructs
- `golangci-lint run` - comprehensive linting checks (includes wsl, revive, staticcheck)
- `goreleaser check` - validates release configuration

**`task delint`** (auto-fixes):
- `go mod tidy` - fixes go.mod/go.sum
- `go fmt` - fixes basic formatting
- `gofumpt -l -w` - fixes aggressive Go formatting
- `golangci-lint run --fix` - fixes linting issues where possible

All code must pass `task lint` before submitting.

### Git Hooks (Lefthook)

Lefthook enforces quality checks before each commit. It is a Go binary installed
automatically by `task install-tools` and wired up by `task dev-init`.

Hooks run automatically on `git commit`. Run manually with `task precommit`.
Bypass with `LEFTHOOK=0 git commit` (use sparingly). Configured hooks:

- **`task precommit`**: formats via gofumpt, verifies Go files are formatted, runs `go mod tidy`, `go vet`, and `golangci-lint run --new-from-rev HEAD` (new changes only), verifies golangci-lint config, and checks go.mod/go.sum are tidy
- **`task dupcheck`**: duplicate code detection via jscpd (threshold and exclusions in `.jscpd.json`)

Both hooks reuse Taskfile tasks so there is a single source of truth for quality checks.
Configuration lives in `lefthook.yml` at the repository root.

### Updating golangci-lint

When upgrading the Go version in `go.mod`, you may need to update golangci-lint to ensure compatibility:

1. Check the [golangci-lint releases](https://github.com/golangci/golangci-lint/releases) for a version that supports your target Go version
2. Update the `GOLANGCI_LINT_VERSION` variable in `Taskfile.yaml`
3. Run `task install-tools` to download the pre-built binary
4. Run `task delint` to auto-fix any issues, then `task lint` to verify compatibility

The `GOLANGCI_LINT_VERSION` is pinned to ensure reproducible builds across all development environments. The binary is installed as a standalone pre-built artifact, not via `go install`, so version mismatches between your project's Go version and golangci-lint's internal Go version are handled automatically.

## Configuration & Flag Binding

The CLI persists user configuration to `drconfig.yaml` via viper. To prevent
transient command flags from leaking into the config file, follow these rules.
For full details, see [docs/development/configuration.md](docs/development/configuration.md).

**Quick reference:**

- **Outside `internal/config/`, do not import `github.com/spf13/viper` directly.**
  Use `internal/config/viperx`. Direct imports are blocked by `depguard`.
- **Never call `viper.WriteConfig()` directly** (and `viperx` does not expose it).
  Use `config.UpdateConfigFile(keys ...string)`, which only writes keys listed
  in `config.PersistableKeys` (`internal/config/write.go`).
- **Never bulk-bind subcommand flags to viper.** `viperx` does not expose
  `BindPFlags`. Bind only specific persistent flags explicitly via
  `viperx.BindPFlag` in `cmd/root.go::init()`.
- **Read transient flags directly from cobra**: `cmd.Flags().GetBool("yes")`.
  Do not bind them with `viperx.BindPFlag`.
- **Env-var override for a transient flag:** register only the env var via
  `viperx.BindEnv(key, "DATAROBOT_CLI_…")` and OR the two sources at the call site:
  `yesFlag, _ := cmd.Flags().GetBool("yes"); yes := yesFlag || viperx.GetBool("yes")`.
- **To make a key persistable**, add it to `config.PersistableKeys` and have the
  write site call `config.UpdateConfigFile("my-key")`.

## Code Review Guidelines

All PRs are reviewed against **bugbot rules** in [.cursor/BUGBOT.md](.cursor/BUGBOT.md). Rules are organized by risk level:

**High-Risk** (catches silent failures, data corruption, poor error handling):

- **Concurrency** — Goroutine panic recovery, loop variable capture, WaitGroup pairing, channel closure
- **Error Handling** — Wrapping errors with context, user-facing messages, not silently ignoring errors
- **Security** — Input validation, security boundaries, threat models
- **Testing** — Race detector, error path coverage, test seams

**Resource & Operations** (prevents hangs, leaks, platform bugs):

- **Resources** — Lock lifecycle, timeouts, cleanup, disk space checks
- **Paths** — Validation, normalization, Unicode, symlinks
- **Cross-Platform** — Build tags, case sensitivity, line endings, symlink handling

**Design** (prevents tight coupling, premature abstraction):

- **Architecture** — Code organization, separation of concerns, dependency injection, phase orchestration
- **Package APIs** — Contracts between packages, documentation, limitations

**Quality** (consistency and maintainability):

- **Testing** — Test coverage, mocking, pagination tests
- **Commands** — Table rendering, file organization, output formatting, JSON output purity

**CI & Workflows** (informational callouts, not correctness bugs):

- **Composite Actions** — Changes to `.github/actions/*/action.yaml` can't be validated by their own PR's CI

**When working on code**, apply these quick principles:

- **Concurrency**: Recover from panics, capture loop variables, pair WaitGroups, close channels safely
- **Errors**: Wrap with context, specialize messages (404 vs 500), log before returning
- **Commands**: Use `lipgloss/table` + `tui.TableBorderStyle`, consistent styling, test output formatting
- **JSON output purity**: When `--output-format json` is set, stdout must contain only valid JSON. All deprecation warnings, logs, usage, and update hints go to stderr
- **Platforms**: Add build tags, match signatures, test on target platforms

Refer to [.cursor/BUGBOT.md](.cursor/BUGBOT.md) for detailed rules and examples during PR review.

> [!NOTE]
> **Composite action changes need special testing.** Workflows like `_smoke.yaml` and
> `_pre-release-smoke.yaml` reference `.github/actions/*` via a pinned `@main` ref, not
> the calling ref — so a PR that edits a composite action's `action.yaml` does not
> exercise that change in its own CI run; the fix only takes effect after merge. Either
> temporarily point the ref at the PR branch to get a real CI run (reverting before
> merge), or verify immediately after merge via a manual `workflow_dispatch` run. See
> [.cursor/bugbot-ci.md](.cursor/bugbot-ci.md).

## Feature Gates

Feature gates allow commands to be hidden until ready for release. For comprehensive documentation including implementation details, see [docs/development/feature-gates.md](docs/development/feature-gates.md).

**Quick reference:**
- Gate a command via `features.SetGate(cmd, "feature-name")` (sets the annotation on the command)
- Enable via env var: `DATAROBOT_CLI_FEATURE_<NAME>=true` (e.g., `DATAROBOT_CLI_FEATURE_WORKLOAD=true`)
- Currently supported: environment variables only (config file support planned)
- Filtering happens via `cli.CommandAdder.AddCommand` at registration time — `CommandAdder` is the only filtering mechanism
- To gate a **nested** subcommand, wrap the parent with `&cli.CommandAdder{Command: parent}` and call `adder.AddCommand(...)` instead of `parent.AddCommand(...)`

## Pull Request Workflow

**Always open PRs as draft first.** This repo uses [Review Router](https://github.com/datarobot-oss/review-router) to automatically route PRs to the right reviewing teams based on changed files. Review Router only fires when a PR transitions to "Ready for Review" (either by flipping the draft flag or applying the "Ready for Review" label). PRs that are opened as non-draft and never get the label will **not** be routed, leaving reviewers unaware.

Workflow:
1. Open your PR as a **draft** while you are still working on it
2. Push updates, iterate, and run `task lint` + `task test`
3. When the PR is genuinely ready for review, mark it **"Ready for Review"** (or add the "Ready for Review" label)
4. Review Router will read `.github/CODEOWNERS`, request reviews from the appropriate teams, and post a summary comment

Do not open non-draft PRs directly unless the change is trivially ready. Early draft PRs help reviewers see work in progress and ensure routing works correctly when the PR is ready.

## PR Output Format

Output change summaries in Markdown format using the template in `.github/PULL_REQUEST_TEMPLATE.md`.
