# GitHub Copilot Instructions

## Development Workflow

- Use Taskfile tasks to complete developer tasks rather than direct go tasks
- Ensure you lint and format code using `task lint`.
- Specifically run `task build` to build the system instead of `go build`

## Coding Standards

- Always wrap new TUI models with the InterruptibleModel from the `tui` package to ensure global Ctrl-C handling.
- We use `go mod tidy` `go fmt` `go vet` `golangci-lint` and `goreleaser check` to ensure code quality and consistency. Only write code that meets those standards.