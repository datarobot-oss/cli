# Development

This section provides comprehensive documentation for contributors and developers working on the DataRobot CLI.

## Getting started

If you're new to developing the CLI, start here:

1. **[Setup](setup.md)**&mdash;set up your development environment, install prerequisites, and clone the repository.
2. **[Building](building.md)**&mdash;learn how to build the CLI from source, understand the build process, and explore available development tasks.
3. **[Structure](structure.md)**&mdash;understand the codebase organization, key packages, and design patterns used throughout the project.

## Development guides

### Core concepts

- **[Project structure](structure.md)**&mdash;understand the codebase organization, command structure, internal packages, and key design patterns.
- **[Building](building.md)**&mdash;detailed guide on building the CLI, available tasks, and build configuration.
- **[Authentication](authentication.md)**&mdash;learn about the OAuth authentication implementation, token management, and API integration.

### Advanced topics

- **[Plugins](plugins.md)**&mdash;develop and test CLI plugins, understand the plugin system architecture.
- **[Remote plugins](remote-plugins.md)**&mdash;create and distribute remote plugins, plugin registry management.
- **[Releasing](releasing.md)**&mdash;release process, versioning strategy, and GoReleaser configuration.

## Quick reference

### Essential commands

```bash
# Setup development environment
task dev-init

# Build the CLI
task build

# Run tests
task test

# Run linters
task lint

# Run CLI without building
task run -- templates list
```

### Development workflow

1. Clone the repository and run `task dev-init`
2. Create a feature branch
3. Make your changes
4. Run `task lint` to format and lint code
5. Run `task test` to verify tests pass
6. Commit and push your changes
7. Open a pull request

See [Building](building.md) for detailed workflow documentation.

## Prerequisites

Before you begin development:

- **Go 1.25.7 or later**&mdash;required for building the CLI.
- **Git**&mdash;version control.
- **Task**&mdash;task runner for development commands.
- **golangci-lint**&mdash;installed automatically via `task dev-init`.

See [Setup](setup.md) for detailed installation instructions.

## Project structure overview

```
cli/
├── cmd/                    # Command implementations
│   ├── auth/              # Authentication commands
│   ├── component/         # Component management
│   ├── dotenv/            # Environment variable management
│   ├── plugin/            # Plugin management
│   ├── self/              # CLI utility commands
│   ├── start/             # Quickstart command
│   ├── task/              # Task execution
│   └── templates/         # Template management
├── internal/              # Private packages
│   ├── auth/              # Authentication logic
│   ├── config/            # Configuration management
│   ├── drapi/             # DataRobot API client
│   ├── envbuilder/        # Environment configuration
│   └── task/              # Task runner integration
├── tui/                   # Terminal UI components
├── docs/                  # Documentation
├── main.go                # Application entry point
├── Taskfile.yaml          # Task definitions
└── goreleaser.yaml        # Release configuration
```

See [Structure](structure.md) for comprehensive documentation.

## Coding standards

All code must:

- Pass `golangci-lint` with zero errors
- Follow Go whitespace rules (wsl linter)
- Include tests for new functionality
- Use the Taskfile for build operations

See [Building](building.md#coding-standards) for detailed requirements.

## Testing

```bash
# Run all tests
task test

# Run tests with coverage
task test-coverage

# Run specific package tests
go test ./cmd/auth/...

# Run with race detection (included in task test)
go test -race ./...
```

See [Building](building.md#testing) for comprehensive testing documentation.

## Getting help

- **Documentation**: Review the guides linked above
- **Issues**: Check existing [GitHub issues](https://github.com/datarobot-oss/cli/issues)
- **Discussions**: Ask questions in [GitHub discussions](https://github.com/datarobot-oss/cli/discussions)
- **Contributing**: Read the [contributing guide](https://github.com/datarobot-oss/cli/blob/main/CONTRIBUTING.md)

## See also

- [Contributing guide](https://github.com/datarobot-oss/cli/blob/main/CONTRIBUTING.md)&mdash;contribution guidelines and code of conduct.
- [User guide](../user-guide/README.md)&mdash;end-user documentation.
- [Command reference](../commands/README.md)&mdash;detailed command documentation.
