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
  "description": "Adds extra commands to dr"
}
```

#### Required fields

- `name` (string): The command name the CLI will register.
  - Example: `{"name":"my-plugin",...}` becomes the top-level command `dr my-plugin`.
  - Must be non-empty (plugins missing this field are rejected).

#### Optional fields

- `version` (string): Displayed in `dr plugin list` (shown as `-` if empty).
- `description` (string): Displayed in `dr plugin list` and used as the command short help when registered as `dr <name>`.

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
