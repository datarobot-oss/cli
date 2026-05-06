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

## Concurrency Patterns

When working with goroutines and channels:

- **Goroutine panics** — Always recover from panics in goroutines; silent panics cause data corruption
- **Error channels** — Buffer only for the expected number of errors; over-buffering wastes memory
- **Loop variable capture** — Use the `fa := fa` pattern before passing loop variables to goroutines
- **WaitGroup pairing** — Verify every `wg.Add()` has a corresponding `defer wg.Done()` and all writers finish before `wg.Wait()`
- **Channel closure** — Only the sender should close channels, and only after all goroutines have finished writing
- **Error handling** — Multiple concurrent errors should be logged; don't hide errors by returning only the first one

Detailed patterns and red flags are in [BUGBOT.md](BUGBOT.md).

## Error Handling Best Practices

- **Wrap user-facing errors** — Always provide context ("timeout after 300s" not "context deadline exceeded")
- **Log before returning** — Orchestrators should log errors before returning them to maintain debugging visibility
- **Don't ignore errors** — Never use `_ = someFunc()` silently; log or handle every error path
- **Specialize error messages** — Distinguish error types (404 → "artifact X not found" vs generic "API error")
- **Multiple error scenarios** — If multiple errors can occur, be explicit: return first error, collect all, or fail-fast? Document the choice.

## Command Structure

### File Organization for Interactive Commands

- **Interactive models** use `model.go` (implements `tea.Model` from bubbletea)
- **Sub-models** for complex UIs use `<specific>Model.go` naming (e.g., `promptModel.go`, `hostModel.go`)
- **Non-interactive render logic** stays in `cmd.go` or splits to `render.go` if needed
- **Don't invent conventions** — Review existing patterns first before creating new file names (`cmd/plugin/list`, `cmd/task/list`, `cmd/templates/list/model.go`)
- **Size check** — If cmd + render logic > 350 lines, consider consolidating or splitting thoughtfully

### Table Rendering for List Commands

- **Use `charmbracelet/lipgloss/table`** for non-interactive list output (not `text/tabwriter`)
- **Add borders** — `.Border(lipgloss.RoundedBorder())` and use `tui.TableBorderStyle` for consistency
- **Adaptive colors** — Use `StyleFunc` with colors from `tui/styles.go` (supports light/dark themes via `tui.GetAdaptiveColor()`)
- **Reference existing patterns** — See `cmd/plugin/list`, `cmd/task/list`, `cmd/templates/list/model.go` for examples
- **Interactive selection** — If the table needs user interaction, use bubbletea; otherwise use lipgloss for display-only rendering

### Output Formatting

- **Use tui design system** — Apply `tui.SubTitleStyle`, `tui.BaseTextStyle`, `tui.DimStyle`, etc. for consistency
- **Adaptive dark mode** — Use `tui.GetAdaptiveColor(light, dark)` to support both light and dark terminal themes
- **Test actual formatting** — Text output tests should verify rendered formatting, not just string presence

## Platform-Specific Code

- **Build tags** — Files like `*_unix.go`, `*_windows.go` must have `//go:build` comments at the top
- **Identical signatures** — All platform implementations must have matching function signatures
- **Test thoroughly** — Don't assume `_unix.go` works on darwin; test on all platforms explicitly
- **Use stdlib portably** — Use `golang.org/x/sys/unix` instead of raw syscalls for better portability
- **Track stubs** — Windows stubs should have corresponding JIRA tickets and be documented in CLI help (not silent)
- **Make assumptions explicit** — If code has platform-specific behavior, it should be obvious to users, not hidden

## Feature Gates

Feature gates allow commands to be hidden until ready for release. For comprehensive documentation including implementation details, see [docs/development/feature-gates.md](docs/development/feature-gates.md).

**Quick reference:**
- Gate a command via `features.SetGate(cmd, "feature-name")` (sets the annotation on the command)
- Enable via env var: `DATAROBOT_CLI_FEATURE_<NAME>=true` (e.g., `DATAROBOT_CLI_FEATURE_WORKLOAD=true`)
- Currently supported: environment variables only (config file support planned)
- Filtering happens via `cli.CommandAdder.AddCommand` at registration time — `CommandAdder` is the only filtering mechanism
- To gate a **nested** subcommand, wrap the parent with `&cli.CommandAdder{Command: parent}` and call `adder.AddCommand(...)` instead of `parent.AddCommand(...)`

## PR Output Format

Output change summaries in Markdown format using the template in `.github/PULL_REQUEST_TEMPLATE.md`.
