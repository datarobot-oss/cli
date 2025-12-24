# AGENTS.md

## Project Overview

DataRobot CLI (`dr`) - A Go-based command-line interface for managing DataRobot custom applications with OAuth integration, template management, and task execution capabilities.

## Build & Development Commands

Use Taskfile tasks rather than raw Go commands:

| Command         | Description                                 |
| --------------- | ------------------------------------------- |
| `task build`    | Build the CLI binary (outputs to ./dist/dr) |
| `task lint`     | Lint and format all code                    |
| `task test`     | Run tests with race detection and coverage  |
| `task dev-init` | Initialize development environment          |
| `task run`      | Run CLI directly via `go run`               |

## Testing

- Run tests: `task test`
- Tests use `testify/assert` for assertions
- Test files follow `*_test.go` naming convention
- If DR_API_TOKEN is set, run smoke tests: `task smoke-test` (but ask for permission before using a real API token)

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
- Reuse existing TUI components from `tui` package or Bubbles library (https://github.com/charmbracelet/bubbles)
- Use styles from `tui/styles.go` for consistency
- When `--debug` is enabled, TUI debug logs are written to `dr-tui-debug.log`

## Quality Tools

All code must pass these tools without errors:

- `go mod tidy` - dependency management
- `go fmt` - basic formatting
- `go vet` - suspicious constructs
- `golangci-lint` - comprehensive linting (includes wsl, revive, staticcheck)
- `goreleaser check` - release configuration validation

## PR Output Format

Output change summaries in Markdown format using the template in `.github/PULL_REQUEST_TEMPLATE.md`.
