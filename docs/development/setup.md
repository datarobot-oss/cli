# Development setup

This page outlines how to set up your development environment to build and develop with the DataRobot CLI.

## Prerequisites

- [Go 1.25.5](https://golang.org/dl/)
- Git for version control
- [Task](https://taskfile.dev/installation/) (A task runner)

## Installation

### Install task

Task is required for running development tasks.

#### macOS

```bash
brew install go-task/tap/go-task
```

#### Linux

```bash
sh -c "$(curl --location https://taskfile.dev/install.sh)" -- -d -b /usr/local/bin
```

#### Windows

```powershell
choco install go-task
```

## Set up the development environment

### Clone the repository

```bash
git clone https://github.com/datarobot-oss/cli.git
cd cli
```

### Install development tools

```bash
task dev-init
```

This will install all necessary development tools including linters and code formatters.

### Build the CLI

```bash
task build
```

The binary will be available at `./dist/dr`.

### Verify the build

```bash
./dist/dr version
```

## Available development tasks

To view all available tasks:

```bash
task --list
```

### Common tasks

| Task | Description |
|------|-------------|
| `task build` | Build the CLI binary. |
| `task test` | Run all tests. |
| `task test-coverage` | Run tests with a coverage report. |
| `task lint` | Run linters and code formatters. |
| `task fmt` | Format code. |
| `task clean` | Cleanly build artifacts. |
| `task dev-init` | Set up a development environment. |
| `task install-tools` | Install development tools. |
| `task run` | Run the CLI without building (e.g., `task run -- templates list`). |

## Build the CLI

**Always use `task build` for building the CLI.** This ensures that:

- The version information from git is included
- The git commit hash is embedded
- The build timestamp is recorded
- The proper `ldflags` configuration is applied

```bash
# Standard build (recommended)
task build

# Run without building (for quick testing)
task run -- templates list
```

## Running tests

```bash
# Run all tests (both unit and integration)
task test

# Run tests with coverage
task test-coverage

# Run specific test
go test ./cmd/auth/...
```

## Linting and formatting

For linting and formatting, this project uses the following tools:

- `golangci-lint` for comprehensive linting
- `go fmt` for basic formatting
- `go vet` for suspicious constructs
- `goreleaser check` for release configuration validation

```bash
# Run all linters (includes formatting)
task lint

# Format code only
task fmt
```

## Next steps

- [Project structure](structure.md): Understand the codebase organization.
- [Build guide](building.md): Detailed build information and architecture.
- [Release process](releasing.md): Create and publish releases.
