# plugin

The `dr plugin` command manages CLI plugins.

## dr plugin install

```bash
dr plugin install
dr plugin install PLUGIN_NAME
dr plugin install PLUGIN_NAME --version 1.0.0
dr plugin install PLUGIN_NAME --version "^1.0.0"
dr plugin install --list
dr plugin install PLUGIN_NAME --versions
dr plugin install --list --registry-url http://localhost:8000/plugins
```

Installs a plugin from the remote plugin registry.

When you run the command without arguments, it displays the list of available plugins (equivalent to `--list`).

### Flags

- `--version`&mdash;version constraint (default: `latest`).
  - Exact version: `1.2.3`
  - Caret (compatible): `^1.2.3` (any 1.x.x >= 1.2.3)
  - Tilde (patch-level): `~1.2.3` (any 1.2.x >= 1.2.3)
  - Minimum: `>=1.0.0`
  - Latest: `latest`
- `--registry-url`&mdash;URL of the plugin registry (default: `https://cli.datarobot.com/plugins/index.json`).
- `--versions`&mdash;list available versions for a specific plugin.
- `--list`&mdash;list available plugins from the registry without installing.

### Examples

```bash
# List available plugins (shows latest version for each).
dr plugin install
dr plugin install --list

# List plugins from local development server.
dr plugin install --list --registry-url http://127.0.0.1:8000/cli/dev-docs/plugins

# List available versions for a plugin.
dr plugin install assist --versions
dr plugin install assist --versions --registry-url http://127.0.0.1:8000/cli/dev-docs/plugins

# Install latest version of assist plugin.
dr plugin install assist

# Install specific version.
dr plugin install assist --version 0.1.6

# Install with semver constraint.
dr plugin install assist --version "^0.1.0"

# Install from custom registry.
dr plugin install assist --registry-url http://127.0.0.1:8000/cli/dev-docs/plugins
```

## dr plugin uninstall

```bash
dr plugin uninstall PLUGIN_NAME
```

Removes a plugin that was installed via `dr plugin install`.

This command only works for managed plugins (installed through the plugin registry). To remove manually installed plugins, delete the executable from your `.dr/plugins/` directory or `PATH`.

### Examples

```bash
# Uninstall the assist plugin.
dr plugin uninstall assist
```

## dr plugin update

```bash
dr plugin update PLUGIN_NAME
dr plugin update --all
dr plugin update PLUGIN_NAME --registry-url http://localhost:8000/plugins
```

Updates an installed plugin to the latest available version.

When you run the command with `--all`, it checks all installed managed plugins for updates and upgrades them to their latest versions.

### Flags

- `--all`&mdash;update all installed plugins.
- `--registry-url`&mdash;URL of the plugin registry (default: `https://cli.datarobot.com/plugins/index.json`).

### Examples

```bash
# Update a specific plugin to the latest version.
dr plugin update assist

# Update all installed plugins.
dr plugin update --all

# Update from a custom registry.
dr plugin update assist --registry-url http://127.0.0.1:8000/cli/dev-docs/plugins
```

## dr plugin list

```bash
dr plugin list
```

Lists plugins discovered by the CLI at startup. Discovery results are cached for the duration of the CLI invocation.

### Plugin requirements

A plugin is an executable that:

- Is named `dr-*`.
- Implements `--dr-plugin-manifest` (used to fetch metadata like name, version, and description).

### Plugin discovery

The CLI discovers plugins from:

1. Project-local `.dr/plugins/` directory (highest priority).
2. All directories on your `PATH`.

If multiple executables declare the same manifest `name`, the CLI uses only the first discovered plugin.

### Output

When plugins are found, `dr plugin list` displays a table with:

- **NAME**&mdash;plugin command name (from the plugin manifest).
- **VERSION**&mdash;plugin version (from the manifest; `-` if empty).
- **DESCRIPTION**&mdash;plugin description (from the manifest; `-` if empty).
- **PATH**&mdash;full path to the executable.

When no plugins are found, the command displays a message and the discovery locations.

### Notes

- Plugin manifest retrieval has its own timeout (see `plugin.manifest_timeout_ms` in configuration).
- The global flag `--plugin-discovery-timeout` controls overall discovery time and disables discovery when set to `0s`.
