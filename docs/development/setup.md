# Development Setup

This guide covers setting up your development environment for building and developing the DataRobot CLI.

## Prerequisites

- **Go 1.25.3+**&mdash;[Download](https://golang.org/dl/)
- **Git**&mdash;version control
- **Task**&mdash;task runner ([install](https://taskfile.dev/installation/))

## Installation

### Installing Task

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

## Setting Up the Development Environment

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

## Available Development Tasks

View all available tasks:

```bash
task --list
```

### Common Tasks

| Task | Description |
|------|-------------|
| `task build` | Build the CLI binary |
| `task test` | Run all tests |
| `task test-coverage` | Run tests with coverage report |
| `task lint` | Run linters and code formatters |
| `task fmt` | Format code |
| `task clean` | Clean build artifacts |
| `task dev-init` | Setup development environment |
| `task install-tools` | Install development tools |
| `task run` | Run CLI without building (e.g., `task run -- templates list`) |

## Building

**Always use `task build` for building the CLI.** This ensures:

- Version information from git is included
- Git commit hash is embedded
- Build timestamp is recorded
- Proper ldflags configuration is applied

```bash
# Standard build (recommended)
task build

# Run without building (for quick testing)
task run -- templates list
```

## Running Tests

```bash
# Run all tests
task test

# Run tests with coverage
task test-coverage

# Run specific test
go test ./cmd/auth/...
```

## Linting and Formatting

```bash
# Run all linters (includes formatting)
task lint

# Format code only
task fmt
```

The project uses:

- `golangci-lint` for comprehensive linting
- `go fmt` for basic formatting
- `go vet` for suspicious constructs
- `goreleaser check` for release configuration validation

## Next Steps

- [Project Structure](structure.md)&mdash;understand the codebase organization
- [Building Guide](building.md)&mdash;detailed build information and architecture
- [Release Process](releasing.md)&mdash;creating releases and publishing
