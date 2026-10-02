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

A [devcontainer](https://containers.dev/) configuration is provided at `.devcontainer/devcontainer.json`. It pins Go 1.27 and installs Task automatically.

- **VS Code**: Open the repo and select "Reopen in Container"
- **Zed**: Open the repo and click **Open in Container** when prompted. If you modify `.devcontainer/devcontainer.json`, Zed does not auto-rebuild — kill the container manually (`docker kill <container-id>`) and use **Project: Open Remote** to reconnect.
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

## Functional QA

Run `/qa` for diff-targeted functional testing, or `/qa-cli` for the CLI flow
menu. Configuration lives in `skills/qa/config.yaml`; the default target is an
isolated local CLI built from the current branch. These skills do not run unit
tests or lint, and the existing smoke workflows remain unchanged.

This change installs local skills only. CI support is a separate follow-up;
local use needs no GitHub repository secrets.

Droid Control is declared in `.factory/settings.json`. A shared active user-scope
installation can satisfy this dependency locally; fresh environments can install
it at project scope. Local terminal QA needs tuistory. Recording/Compose tools are
resolved conditionally from the runtime configuration.

Staging is disabled until dedicated QA accounts and secret references are
configured. Each staging run also requires explicit approval of its target and
mutations. Never reuse personal credentials or existing resources. Public
downloads do not authorize execution of downloaded plugins or dependency
installation. Missing prerequisites are reported BLOCKED, not passed.

Failure learning only suggests updates in reports. Changing to an automatic
write mode requires a separately configured workflow and permissions.

## Related Documentation

- [Droid Computers](https://docs.factory.ai/cli/features/droid-computers)
- [Factory Missions](https://docs.factory.ai/features/missions/overview)
- [Task Runner](https://taskfile.dev/)
- [Development Setup](../docs/development/setup.md)
- [AGENTS.md](../AGENTS.md) - Project coding guidelines
