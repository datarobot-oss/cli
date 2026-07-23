# Factory Configuration - DataRobot CLI

This directory contains configuration for Factory integration with the DataRobot CLI project.

## Droid Computers

[Droid Computers](https://docs.factory.ai/cli/features/droid-computers) are persistent cloud compute environments that Factory can connect to across sessions. Unlike the legacy Cloud Templates (now superseded), Droid Computers retain installed packages, files, and configuration between sessions.

### Setting up a Droid Computer for this repo

1. Navigate to **Settings > Droid Computers** in the Factory App
2. Click **Create** and give your computer a name
3. Factory provisions an Ubuntu environment (4 CPU, 8GB RAM)
4. Once active, clone this repo and run `task bootstrap` to install the full toolchain

For BYOM (Bring Your Own Machine), see the [BYOM docs](https://docs.factory.ai/cli/features/droid-computers-byom).

## Development Environment Setup

### Option A: Devcontainer (VS Code, Codespaces, Droid Computers)

A [devcontainer](https://containers.dev/) configuration is provided at `.devcontainer/devcontainer.json`. It pins Go 1.26 and installs Task automatically.

- **VS Code**: Open the repo and select "Reopen in Container"
- **GitHub Codespaces**: Create a codespace from this repo
- **Droid Computers**: The devcontainer can be used to provision the environment

### Option B: Local setup (no container)

If you prefer not to use a devcontainer, install the prerequisites manually (see `docs/development/setup.md`), then run:

```bash
task bootstrap
```

This verifies your Go version matches `go.mod`, installs all development tools (golangci-lint, goreleaser, jscpd, lefthook), sets up git hooks, and builds the CLI binary.

### Common commands

```bash
task build          # Build the CLI binary to ./dist/dr
task test           # Run tests with race detection and coverage
task lint           # Run linters and formatters (read-only)
task run            # Run the CLI via go run
task run -- --help  # Run CLI with arguments
```

## Related Documentation

- [Droid Computers](https://docs.factory.ai/cli/features/droid-computers)
- [Factory Missions](https://docs.factory.ai/features/missions/overview)
- [Task Runner](https://taskfile.dev/)
- [Development Setup](../docs/development/setup.md)
- [AGENTS.md](../AGENTS.md) - Project coding guidelines
