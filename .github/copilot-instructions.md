# GitHub Copilot Instructions

## Development Workflow

- Use Taskfile tasks to complete developer tasks rather than direct go tasks
- Ensure you lint and format code using `task lint`
- Specifically run `task build` to build the system instead of `go build`
- Output the summary of changes in Markdown format, using the template in .github/PULL_REQUEST_TEMPLATE.md, in a a copyable text block

## Coding Standards

### Go Style Requirements

**Critical**: All code must pass `golangci-lint` with zero errors. Follow these whitespace rules:

1. **Never cuddle declarations**: Always add a blank line before `var`, `const`, `type` declarations when they follow other statements
2. **Separate statement types**: Add blank lines between different statement types (assign, if, for, return, etc.)
3. **Blank line after block start**: Add blank line after opening braces of functions/blocks when followed by declarations
4. **Blank line before multi-line statements**: Add blank line before if/for/switch statements

**Example of correct spacing:**
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

- Always wrap new TUI models with the InterruptibleModel from the `tui` package to ensure global Ctrl-C handling.
- Attempt to use existing TUI components before creating new ones. TUI components can be found in the `tui` package, or from the Bubbles library (https://github.com/charmbracelet/bubbles).
- Common lipgloss styles are defined in tui/theme.go - reuse these styles where possible for consistency.


### Quality Tools

We use these tools - all code must pass without errors:
- `go mod tidy` - dependency management
- `go fmt` - basic formatting
- `go vet` - suspicious constructs
- `golangci-lint` - comprehensive linting (includes wsl, revive, staticcheck, etc.)
- `goreleaser check` - release configuration validation

**Before submitting code, mentally verify it follows wsl (whitespace) rules.**
