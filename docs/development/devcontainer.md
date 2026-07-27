# Devcontainer

This repo includes a [devcontainer](https://containers.dev/) configuration at `.devcontainer/devcontainer.json` that provides a fully configured development environment with Go, Task, and all required tools preinstalled. It works with VS Code, GitHub Codespaces, Zed, and Factory Droid Computers.

## What it provides

- **Go 1.26** (pinned via `mcr.microsoft.com/devcontainers/go:2-1.26-bookworm`)
- **Task** (installed via the [go-task devcontainer feature](https://github.com/eitsupi/devcontainer-features/tree/main/src/go-task))
- **Automatic Go version sync** — if the image's Go patch version doesn't match `go.mod`, it is upgraded automatically during container creation
- **One-command bootstrap** — `task bootstrap` runs automatically, installing all development tools (golangci-lint, goreleaser, jscpd, lefthook), setting up git hooks, and building the CLI
- **uv** (installed via the [uv devcontainer feature](https://github.com/va-h/devcontainers-features/tree/main/src/uv)) — enables `task docs-serve` for local documentation preview with MkDocs

## Local development

### VS Code

1. Install the [Dev Containers extension](https://marketplace.visualstudio.com/items?itemName=ms-vscode-remote.remote-containers)
2. Open the cloned repo in VS Code
3. When prompted, click **Reopen in Container** (or run the command via `Cmd+Shift+P` > "Dev Containers: Reopen in Container")

VS Code rebuilds the container and runs all lifecycle commands. The terminal is available once `postCreateCommand` completes.

### Zed

1. Open the cloned repo in Zed
2. When prompted, click **Open in Container**
3. If you modify `.devcontainer/devcontainer.json`, Zed does not auto-rebuild. Kill the container manually and reconnect:
   ```bash
   docker kill <container-id>
   ```
   Then use **Project: Open Remote** in the command palette to reconnect.

### Devcontainer CLI (no IDE)

```bash
# Build and start the container
npx @devcontainers/cli up --workspace-folder .

# Get a shell inside
npx @devcontainers/cli exec --workspace-folder . bash
```

Add `--remove-existing-container` to force a full rebuild after changing `devcontainer.json`.

## GitHub Codespaces

1. Go to the repo on GitHub
2. Click **Code** > **Codespaces** > **Create codespace on main**
3. Codespaces reads `.devcontainer/devcontainer.json` automatically and provisions the environment

The devcontainer works with Codespaces out of the box — no additional configuration needed.

## Factory Droid Computers

[Droid Computers](https://docs.factory.ai/cli/features/droid-computers) are persistent cloud compute environments that Factory can connect to across sessions. To use this repo with a Droid Computer:

1. **Create a managed Droid Computer** in Settings > Droid Computers (Factory provisions an Ubuntu environment)
2. **Clone the repo** inside the Droid Computer:
   ```bash
   git clone https://github.com/datarobot-oss/cli.git
   cd cli
   ```
3. **Run bootstrap**:
   ```bash
   task bootstrap
   ```

For BYOM (Bring Your Own Machine) Droid Computers, the devcontainer is optional — the machine's existing toolchain is used directly. See the [BYOM docs](https://docs.factory.ai/cli/features/droid-computers-byom) for setup.

To run larger Factory Missions on a Droid Computer, the persistent environment means the Go toolchain only needs to be installed once. Subsequent sessions reuse the same installed packages, built binary, and configuration.

## Lifecycle commands

The devcontainer uses three lifecycle hooks that run in order during container creation:

| Hook | Command | Purpose |
| --- | --- | --- |
| `updateContentCommand` | `bash .devcontainer/sync-go.sh` | Upgrades Go to match `go.mod` if the image version differs |
| `postCreateCommand` | `task bootstrap` | Installs tools, sets up git hooks, builds the CLI |
| `waitFor` | `postCreateCommand` | Tools wait for bootstrap to finish before connecting |

The `waitFor` property ensures the terminal is not available until all setup is complete.

## Troubleshooting

### Go version mismatch

The image may ship a slightly older Go patch version than `go.mod` requires. The `sync-go.sh` script handles this automatically by downloading and installing the exact version from `go.dev`. If you see a version mismatch error, verify the script ran:

```bash
go version  # should match go.mod
```

### Task not found

Task is installed via the `ghcr.io/eitsupi/devcontainer-features/go-task:1` feature during the Docker build. If Task is missing, the feature may have failed. Install it manually:

```bash
sh -c "$(curl --location https://taskfile.dev/install.sh)" -- -d -b /usr/local/bin
```

### Container not picking up config changes

After editing `devcontainer.json`, you must rebuild the container. In VS Code, use "Dev Containers: Rebuild Container". With the CLI:

```bash
npx @devcontainers/cli up --workspace-folder . --remove-existing-container
```

### Bare-repo worktrees

The devcontainer mounts a single directory and expects a standard git clone. Bare-repo worktree layouts (where `.git` is a file pointing to an external bare repo) will not work because the gitdir reference resolves to a host path that doesn't exist inside the container. Use a normal clone for devcontainer-based development.
