# Plugin system (development)

This document describes how the DataRobot CLI plugin system works and how to build a plugin.

See more information on Confluence at [DataRobot CLI Integration Analysis](https://datarobot.atlassian.net/wiki/spaces/CFX/pages/7220985894/DataRobot+CLI+Integration+Analysis).

## Overview

Plugins are external executables that extend the `dr` CLI with additional top-level commands.

- A plugin executable is discovered under the name pattern `dr-*`.
- When discovered, the CLI queries the executable for a JSON manifest.
- The manifest declares the command name and metadata shown in `dr plugin list`.
- When a user runs `dr <plugin-command> ...`, the CLI executes the plugin binary and forwards all arguments.

## Discovery

At CLI startup, plugins are discovered from the following locations, in priority order:

1. **Managed plugins directories** (highest priority) — plugins installed via `dr plugin install`.
   - Primary: `$XDG_CONFIG_HOME/datarobot/plugins/` (or `~/.config/datarobot/plugins/` when `XDG_CONFIG_HOME` is not set).
   - Additional directories from `$XDG_CONFIG_DIRS` if set (e.g., `/etc/xdg/datarobot/plugins` for system-wide plugins).
   - The primary directory always takes precedence: if the same plugin name exists in multiple locations, the first discovered one wins and later ones are skipped.
2. **Project-local** `.dr/plugins/` directory.
3. **Every directory on your `PATH``.

Only files whose filename begins with `dr-` are considered.

The CLI also verifies the candidate is executable (via Go's runtime `exec.LookPath`).

**Note**: `XDG_CONFIG_DIRS` is only used when explicitly set by the user (no default system paths). This allows system administrators to provide plugins for all users while maintaining security: `export XDG_CONFIG_DIRS=/etc/xdg:/usr/local/etc`.

### Deduplication

Plugins are deduplicated by `manifest.name` (not by filename). If multiple binaries report the same `manifest.name`, the first discovered one wins and later ones are skipped.

### Timeouts

- Overall discovery is bounded by the global flag `--plugin-discovery-timeout` (default `2s`).
  - Set to `0s` to disable plugin discovery entirely.
- Manifest retrieval is bounded by `plugin.manifest_timeout_ms` (default `500ms`).

#### Testing notes

The default `500ms` timeout is occasionally exceeded under heavy test load (e.g. `task test` with `-race`).
Test suites that exercise plugin discovery should:

1. Call `viperx.Reset()` in `SetupTest`/`TearDownTest` to prevent config-file or env-var
   values from leaking into the manifest timeout.
2. Set a generous test-specific timeout before discovering:
   `viperx.Set("plugin.manifest_timeout_ms", 5000)` (5 seconds).

## Manifest protocol

To be recognized as a plugin, the executable **must** respond to the special argument:

```bash
dr-myplugin --dr-plugin-manifest
```

The command must write a single JSON object to **stdout** and exit with code `0`.

### Manifest JSON schema

The CLI currently understands the following fields:

```json
{
  "name": "my-plugin",
  "version": "1.2.3",
  "description": "Adds extra commands to dr",
  "authentication": true
}
```

#### Required fields

- `name` (string): The command name the CLI will register.
  - Example: `{"name":"my-plugin",...}` becomes the top-level command `dr my-plugin`.
  - Must be non-empty (plugins missing this field are rejected).

#### Optional fields

- `version` (string): Displayed in `dr plugin list` (shown as `-` if empty).
- `description` (string): Displayed in `dr plugin list` and used as the command short help when registered as `dr <name>`.
- `authentication` (boolean): When `true`, the CLI will check for valid DataRobot authentication before executing the plugin.
  - If no valid credentials exist, the user will be prompted to log in.
  - Respects the global `--skip-auth` flag.
  - Defaults to `false` if omitted.

### Notes / recommendations

- Keep manifest output small and fast; it is called during discovery.
- The manifest should be deterministic and should not require network access.
- The plugin should handle `--dr-plugin-manifest` before doing any other work (and should not print extra output in this mode).

## Plugin dependencies

A managed plugin can declare the external tools it requires by shipping a `versions.yaml` file at the root of its `.tar.xz` archive. The format is identical to the project-level `.datarobot/cli/versions.yaml`.

### File format

```yaml
docker:
  name: Docker
  minimum-version: "20.10.0"
  command: "docker --version"
  url: https://docs.docker.com/get-docker/
  install:
    macos: "brew install --cask docker"
    linux: "curl -fsSL https://get.docker.com | sh"
    windows: "winget install Docker.DockerDesktop"

kubectl:
  name: kubectl
  minimum-version: "1.28.0"
  command: "kubectl version --client --output=yaml"
  url: https://kubernetes.io/docs/tasks/tools/
  install:
    macos: "brew install kubectl"
    linux: "curl -LO https://dl.k8s.io/release/$(curl -Ls https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl && chmod +x kubectl && sudo mv kubectl /usr/local/bin/"
    windows: "winget install Kubernetes.kubectl"
```

Each key is a unique tool identifier. Required fields are `name`, `minimum-version`, `command`, `url`, and platform install commands (`install.macos`, `install.linux`, `install.windows`).

### When checks run

The CLI checks plugin dependencies at two points:

1. **After `dr plugin install`** — once the archive is extracted, the CLI reads `versions.yaml` from the installed plugin directory and prompts the user to install any missing or outdated tools.
2. **Before each plugin invocation** — when the user runs `dr <plugin-name> [args...]`, the CLI performs the same check before launching the plugin binary.

Both checks are silently skipped when no `versions.yaml` is present (PATH-based and project-local plugins are never checked).

### Confirmation modes

Users can confirm dependency installation in three ways (checked in this order):

1. **`-y` / `--yes` argument** — pass `-y` or `--yes` when running a plugin (`dr <plugin> -y`) or when installing (`dr plugin install <name> -y`).
2. **`DATAROBOT_CLI_NON_INTERACTIVE=1`** — set this environment variable to auto-confirm in CI or automation scripts.
3. **Interactive prompt** — the default: the CLI prints the missing tools and asks `Install missing dependencies? [Y/n]:`. Pressing Enter (or typing `y`) proceeds; typing `n` skips.

## Execution

When a user runs:

```bash
dr <plugin-name> [args...]
```

The CLI:

1. Checks plugin dependencies (see [Plugin dependencies](#plugin-dependencies)) and prompts to install any missing tools.
2. Prints a short info line indicating which plugin is being run.
3. If the plugin manifest has `"authentication": true`, checks for valid authentication and prompts for login if needed.
4. Executes the plugin binary.
5. Passes all remaining arguments to the plugin verbatim.
6. Exits with the same exit code as the plugin.

Because plugin commands are registered as top-level commands, a plugin cannot conflict with an existing built-in command name.

### Authentication

If your plugin needs to interact with the DataRobot API, set `"authentication": true` in your manifest. This ensures users are authenticated before your plugin runs.

**Example manifest with authentication:**

```json
{
  "name": "assist",
  "version": "0.1.6",
  "description": "AI agent design, coding, and deployment assistant",
  "authentication": true
}
```

When `authentication` is enabled:
- The CLI checks for valid credentials from environment variables (`DATAROBOT_ENDPOINT`, `DATAROBOT_API_TOKEN`) or the config file.
- If no valid credentials exist, the user is automatically prompted to log in via `dr auth login`.
- Authentication can be bypassed with the global `--skip-auth` flag (for advanced users).
- Your plugin will receive a clean environment with authentication already validated

### Private CA / TLS support

When a user passes `-k`/`--skip-certificate-check` or `--ca-cert <path>` before
your plugin's name (e.g. `dr --ca-cert /path/to/ca.pem myplugin [args...]`), the
CLI applies the TLS configuration to its own HTTP client and forwards the
equivalent runtime configuration to your plugin subprocess via these environment
variables:

| Variable | Set when | Value |
|---|---|---|
| `NODE_TLS_REJECT_UNAUTHORIZED` | `--skip-certificate-check`/`-k` is used | `0` |
| `NODE_EXTRA_CA_CERTS` | `--ca-cert <path>` is used | `<path>` |
| `SSL_CERT_FILE` | `--ca-cert <path>` is used | `<path>` |

Plugins written in Node.js/Bun automatically honour `NODE_TLS_REJECT_UNAUTHORIZED`
and `NODE_EXTRA_CA_CERTS` without any extra code. Other runtimes that respect
`SSL_CERT_FILE` (e.g. many Go/OpenSSL-based tools) will pick up the custom CA
bundle the same way. If your plugin uses a different HTTP client, read these
variables at startup and configure your client's TLS trust store accordingly.

### Environment variables

The CLI sets the following environment variables on the plugin subprocess. Your plugin inherits the full parent environment plus these additions (which override any inherited values of the same name).

#### Always present

| Variable | Value | Description |
|---|---|---|
| `DR_PLUGIN_MODE` | `1` | Signals to the plugin that it was invoked by the `dr` CLI. |
| `DR_PLUGIN_PATH` | `/path/to/dr-myplugin` | Absolute path to the plugin executable. |
| `DATAROBOT_CONFIG` | `/path/to/drconfig.yaml` | Path to the active config file (if one is loaded). |

#### Present when `authentication: true`

| Variable | Value | Description |
|---|---|---|
| `DATAROBOT_ENDPOINT` | `https://app.datarobot.com` | The DataRobot API endpoint. |
| `DATAROBOT_API_TOKEN` | `<token>` | The user's API token, already validated. |

#### Universal CLI flags (`DATAROBOT_CLI_*`)

When a user passes a universal root-level flag **before** the plugin name, the CLI
consumes it internally and also forwards it to the plugin as a `DATAROBOT_CLI_*`
environment variable so your plugin can optionally honour the same behaviour.

> **Important:** Only flags placed **before** the plugin name are forwarded.
> Flags placed after the plugin name are passed verbatim as command-line arguments
> and are never seen by the core CLI. This matches the kubectl / helm model.

```bash
# Correct — --debug is consumed by core AND forwarded to the plugin:
dr --debug myplugin [args...]

# Not forwarded — --debug is passed to the plugin as a raw argument:
dr myplugin --debug [args...]
```

| CLI flag | Environment variable | Value |
|---|---|---|
| `--debug` | `DATAROBOT_CLI_DEBUG` | `1` |
| `--disable-telemetry` | `DATAROBOT_CLI_DISABLE_TELEMETRY` | `1` |
| `--verbose` | `DATAROBOT_CLI_VERBOSE` | `1` |
| `--skip-certificate-check` | `DATAROBOT_CLI_SKIP_CERTIFICATE_CHECK` | `1` |
| `--ca-cert <path>` | `DATAROBOT_CLI_CA_CERT` | `<path>` |

The `DATAROBOT_CLI_` prefix is the canonical namespace for flags forwarded from
the core CLI. As new universal flags are added, they follow the same convention:
the flag name is upper-cased and hyphens are replaced with underscores
(e.g. `--ca-cert <path>` produces `DATAROBOT_CLI_CA_CERT=<path>`).

##### Consuming forwarded flags in your plugin

Check for the variable at startup and apply the corresponding behaviour:

```bash
# Example: shell plugin honouring DATAROBOT_CLI_DEBUG
if [ "${DATAROBOT_CLI_DEBUG}" = "1" ]; then
  set -x   # enable shell trace / verbose output
fi
```

```python
# Example: Python plugin
import os
DEBUG = os.getenv("DATAROBOT_CLI_DEBUG") == "1"
```

##### Adding a new universal flag (for CLI contributors)

To forward a new root-level flag to plugins, only two files need to change:

1. **Register the flag and mark it universal** — in `cmd/root.go`, add it as a
   persistent flag on `RootCmd` and call `bindUniversal` in the universal flags
   block. That single call binds it to viper and annotates it for forwarding:

   ```go
   RootCmd.PersistentFlags().Bool("my-flag", false, "description")
   // ...
   bindUniversal("my-flag")  // emits DATAROBOT_CLI_MY_FLAG=1
   ```

   `internal/plugin` discovers the annotation automatically — no changes needed there.

2. **Update the docs** — add a row to the table above and the same row to the
   table in `docs/commands/plugins.md` under "Passing global flags to plugins".

## Developing a plugin

Minimum requirements:

1. Name the executable `dr-<something>`.
2. Ensure it is executable (`chmod +x`).
3. Implement `--dr-plugin-manifest` to print valid JSON with at least `name`.
4. Put it in `.dr/plugins/` or on `PATH`.

## Troubleshooting: `dr <command>` not found

If you run `dr <command>` expecting `<command>` to be provided by a plugin, but the CLI reports it as an unknown command, check:

1. **Is the plugin discoverable?** Run:

   ```bash
   dr plugin list
   ```

   If the plugin is not listed, it was not discovered during startup.

2. **Is the plugin executable accessible?** The CLI discovers plugins from managed plugin directories, `.dr/plugins/`, and `PATH`.
   - For managed plugins (installed via `dr plugin install`), they live under `$XDG_CONFIG_HOME/datarobot/plugins/` (or `~/.config/datarobot/plugins/`).
   - For PATH-based plugins, ensure the plugin binary is named `dr-<something>` and is executable.
   - Ensure the directory containing `dr-<something>` is on your `PATH`.
   - You can verify with your shell, e.g.:

     ```bash
     which dr-<something>
     ```

3. **Did you disable or time out discovery?** If `--plugin-discovery-timeout` is `0s` (disabled) or too low, plugins may not be registered.

## Related commands

- `dr plugin list` / `dr plugins list`: show discovered plugins and their manifest metadata.

## Packaging and publishing plugins

The CLI provides tools to help package and publish plugins to a plugin registry.

### Quick start: publish command (recommended)

The easiest way to package and publish a plugin is the all-in-one `publish` command:

```bash
dr self plugin publish <plugin-dir> [flags]
```

This command does everything in one step:
1. Validates the plugin manifest
2. Creates a `.tar.xz` archive
3. Copies it to `plugins/<plugin-name>/<plugin-name>-<version>.tar.xz`
4. Updates the registry file (`index.json`)

**Example:**

```bash
# Publish to default location (docs/plugins/)
dr self plugin publish ./my-plugin

# Publish to custom location
dr self plugin publish ./my-plugin --plugins-dir dist/plugins --index dist/plugins/index.json

# Output:
# ✅ Published my-plugin version 1.0.0
#    Archive: docs/plugins/my-plugin/my-plugin-1.0.0.tar.xz
#    SHA256: abc123...
#    Registry: docs/plugins/index.json
```

### Advanced: manual workflow

For more control over the packaging process, you can use the individual commands:

#### Packaging a plugin

Use `dr self plugin package` to create a distributable `.tar.xz` archive:

```bash
dr self plugin package <plugin-dir> [flags]
```

**Flags:**
- `-o, --output`: Output file path or directory (default: current directory)
  - If path ends with `.tar.xz`, uses exact filename
  - Otherwise treats as directory and creates `<plugin-name>-<version>.tar.xz` inside
- `--index-output`: Save registry JSON fragment to file for use with `dr self plugin add --from-file`

Requirements:
- Plugin directory must contain a valid `manifest.json` with `name` and `version` fields

The command will:
1. Validate the manifest
2. Create a compressed `.tar.xz` archive
3. Calculate SHA256 checksum
4. Optionally save metadata to a file for easy registry updates
5. Output a JSON snippet ready for your plugin registry

**Examples:**

```bash
# Package to current directory (creates my-plugin-1.0.0.tar.xz)
dr self plugin package ./my-plugin

# Package to specific directory
dr self plugin package ./my-plugin -o dist/

# Package with custom filename
dr self plugin package ./my-plugin -o dist/custom-name.tar.xz

# Package and save metadata for later
dr self plugin package ./my-plugin -o dist/ --index-output /tmp/my-plugin.json

# Output:
# ✅ Package created: dist/my-plugin-1.0.0.tar.xz
#    SHA256: abc123...
# 📝 Registry fragment saved to: /tmp/my-plugin.json
#
# Add to registry (index.json):
# ```json
# {
#   "version": "1.0.0",
#   "url": "my-plugin/my-plugin-1.0.0.tar.xz",
#   "sha256": "abc123...",
#   "releaseDate": "2026-01-28"
# }
# ```
```

#### Adding to Plugin Registry

Use `dr self plugin add` to add the packaged version to your plugin registry.

**Option 1: Using saved metadata (recommended):**

```bash
# Package and save metadata
dr self plugin package ./my-plugin --index-output /tmp/my-plugin.json

# Add to registry using the saved file
dr self plugin add docs/plugins/index.json --from-file /tmp/my-plugin.json
```

**Option 2: Manual entry:**

```bash
dr self plugin add <path-to-index.json> \
  --name my-plugin \
  --version 1.0.0 \
  --url my-plugin/my-plugin-1.0.0.tar.xz \
  --sha256 abc123... \
  --release-date 2026-01-28
```

The `add` command will:
- Create the registry file if it doesn't exist
- Add a new plugin entry or append a new version to an existing plugin
- Validate that the version doesn't already exist
- Format the registry with proper JSON indentation

**Complete workflow example:**

```bash
# Quick: One command to do it all
dr self plugin publish ./my-plugin

# Or manual workflow:

# 1. Package the plugin and save metadata
dr self plugin package ./my-plugin -o docs/plugins/ --index-output /tmp/my-plugin.json

# 2. Add to registry using saved metadata
dr self plugin add docs/plugins/index.json --from-file /tmp/my-plugin.json

# 3. Commit and publish
git add docs/plugins/
git commit -m "Add my-plugin v1.0.0"
git push
```
