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

At CLI startup, plugins are discovered from:

1. Project-local `.dr/plugins/` directory (highest priority)
2. Every directory on your `PATH`

Only files whose filename begins with `dr-` are considered.

The CLI also verifies the candidate is executable (via Go's runtime `exec.LookPath`).

### Deduplication

Plugins are deduplicated by `manifest.name` (not by filename). If multiple binaries report the same `manifest.name`, the first discovered one wins and later ones are skipped.

### Timeouts

- Overall discovery is bounded by the global flag `--plugin-discovery-timeout` (default `2s`).
  - Set to `0s` to disable plugin discovery entirely.
- Manifest retrieval is bounded by `plugin.manifest_timeout_ms` (default `500ms`).

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

## Execution

When a user runs:

```bash
dr <plugin-name> [args...]
```

The CLI:

1. Prints a short info line indicating which plugin is being run.
2. If the plugin manifest has `"authentication": true`, checks for valid authentication and prompts for login if needed.
3. Executes the plugin binary.
4. Passes all remaining arguments to the plugin verbatim.
5. Exits with the same exit code as the plugin.

Because plugin commands are registered as top-level commands, a plugin cannot conflict with an existing built-in command name.

### Authentication

If your plugin needs to interact with the DataRobot API, set `"authentication": true` in your manifest. This ensures users are authenticated before your plugin runs.

**Example manifest with authentication:**

```json
{
  "name": "apps",
  "version": "11.1.0",
  "description": "Host custom applications in DataRobot",
  "authentication": true
}
```

When `authentication` is enabled:
- The CLI checks for valid credentials from environment variables (`DATAROBOT_ENDPOINT`, `DATAROBOT_API_TOKEN`) or the config file.
- If no valid credentials exist, the user is automatically prompted to log in via `dr auth login`.
- Authentication can be bypassed with the global `--skip-auth` flag (for advanced users).
- Your plugin will receive a clean environment with authentication already validated
2. Executes the plugin binary.
3. Passes all remaining arguments to the plugin verbatim.
4. Exits with the same exit code as the plugin.

Because plugin commands are registered as top-level commands, a plugin cannot conflict with an existing built-in command name.

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

2. **Is the plugin executable accessible on `PATH`?** The CLI discovers plugins from `.dr/plugins/` and from `PATH`.
   - Ensure the plugin binary is named `dr-<something>` and is executable.
   - Ensure the directory containing `dr-<something>` is on your `PATH`.
   - You can verify with your shell, e.g.:

     ```bash
     which dr-<something>
     ```

3. **Did you disable or time out discovery?** If `--plugin-discovery-timeout` is `0s` (disabled) or too low, plugins may not be registered.

## Related commands

- `dr plugin list` / `dr plugins list`: show discovered plugins and their manifest metadata.

## Packaging and Publishing Plugins

The CLI provides tools to help package and publish plugins to a plugin index.

### Quick Start: Publish Command (Recommended)

The easiest way to package and publish a plugin is the all-in-one `publish` command:

```bash
dr self plugin publish <plugin-dir> [flags]
```

This command does everything in one step:
1. Validates the plugin manifest
2. Creates a `.tar.xz` archive
3. Copies it to `plugins/<plugin-name>/<plugin-name>-<version>.tar.xz`
4. Updates the `index.json` file

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
#    Index: docs/plugins/index.json
```

### Advanced: Manual Workflow

For more control over the packaging process, you can use the individual commands:

#### Packaging a Plugin

Use `dr self plugin package` to create a distributable `.tar.xz` archive:

```bash
dr self plugin package <plugin-dir> [flags]
```

**Flags:**
- `-o, --output`: Output file path or directory (default: current directory)
  - If path ends with `.tar.xz`, uses exact filename
  - Otherwise treats as directory and creates `<plugin-name>-<version>.tar.xz` inside
- `--index-output`: Save index JSON fragment to file for use with `dr self plugin add --from-file`

Requirements:
- Plugin directory must contain a valid `manifest.json` with `name` and `version` fields

The command will:
1. Validate the manifest
2. Create a compressed `.tar.xz` archive
3. Calculate SHA256 checksum
4. Optionally save metadata to a file for easy index updates
5. Output a JSON snippet ready for your plugin index

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
# 📝 Index fragment saved to: /tmp/my-plugin.json
# 
# Add to index.json:
# ```json
# {
#   "version": "1.0.0",
#   "url": "my-plugin/my-plugin-1.0.0.tar.xz",
#   "sha256": "abc123...",
#   "releaseDate": "2026-01-28"
# }
# ```
```

#### Adding to Plugin Index

Use `dr self plugin add` to add the packaged version to your plugin index.

**Option 1: Using saved metadata (recommended):**

```bash
# Package and save metadata
dr self plugin package ./my-plugin --index-output /tmp/my-plugin.json

# Add to index using the saved file
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
- Create the index file if it doesn't exist
- Add a new plugin entry or append a new version to an existing plugin
- Validate that the version doesn't already exist
- Format the index with proper JSON indentation

**Complete workflow example:**

```bash
# Quick: One command to do it all
dr self plugin publish ./my-plugin

# Or manual workflow:

# 1. Package the plugin and save metadata
dr self plugin package ./my-plugin -o docs/plugins/ --index-output /tmp/my-plugin.json

# 2. Add to index using saved metadata
dr self plugin add docs/plugins/index.json --from-file /tmp/my-plugin.json

# 3. Commit and publish
git add docs/plugins/
git commit -m "Add my-plugin v1.0.0"
git push
```
