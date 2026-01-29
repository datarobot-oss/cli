# plugins / plugin

The `dr plugin` (alias: `dr plugins`) commands help you inspect and manage plugins.

## dr plugin install

```bash
dr plugin install
dr plugin install <plugin-name>
dr plugin install <plugin-name> --version 1.0.0
dr plugin install <plugin-name> --version "^1.0.0"
dr plugin install --list
dr plugin install <plugin-name> --versions
dr plugin install --list --index-url http://localhost:8000/plugins
```

Installs a plugin from the remote plugin registry.

When run without arguments, displays the list of available plugins (equivalent to using `--list`).

### Flags

- `--version`: Version constraint (default: `latest`)
  - Exact version: `1.2.3`
  - Caret (compatible): `^1.2.3` (any 1.x.x >= 1.2.3)
  - Tilde (patch-level): `~1.2.3` (any 1.2.x >= 1.2.3)
  - Minimum: `>=1.0.0`
  - Latest: `latest`
- `--index-url`: URL of the plugin index (default: `https://cli.data
- `--versions`: List available versions for a specific pluginrobot.com/plugins/index.json`)
- `--list`: List available plugins from the index without installing

### Examples(shows latest version for each)
dr plugin install
dr plugin install --list

# List plugins from local development server
dr plugin install

# List plugins from local development server
dr plugin install --list --index-url http://127.0.0.1:8000/cli/dev-docs/plugins
List available versions for a plugin
dr plugin install apps --versions
dr plugin install apps --versions --index-url http://127.0.0.1:8000/cli/dev-docs/plugins

# 
# Install latest version of apps plugin
dr plugin install apps

# Install specific version
dr plugin install apps --version 1.0.0

# Install with semver constraint
dr plugin install apps --version "^1.0.0"

# Install from custom registry
dr plugin install apps --index-url http://127.0.0.1:8000/cli/dev-docs/plugins
```

## dr plugin list

```bash
dr plugin list
# or
dr plugins list
```

Lists plugins discovered by the CLI at startup (the discovery results are cached for the duration of the CLI invocation).

### What counts as a plugin

A plugin is an executable that:

- Is named `dr-*`
- Implements `--dr-plugin-manifest` (used to fetch metadata like name/version/description)

### Where plugins are discovered

Plugins are discovered from:

1. Project-local `.dr/plugins/` directory (highest priority)
2. All directories on your `PATH`

If multiple executables declare the same manifest `name`, only the first discovered one is kept.

### Output

When plugins are found, `dr plugin list` prints a table with:

- **NAME**: plugin command name (from the plugin manifest)
- **VERSION**: plugin version (from the manifest; `-` if empty)
- **DESCRIPTION**: plugin description (from the manifest; `-` if empty)
- **PATH**: full path to the executable

When no plugins are found, it prints a short message and the discovery locations.

### Notes

- Plugin manifest retrieval has its own timeout (see `plugin.manifest_timeout_ms` in configuration).
- Global flag `--plugin-discovery-timeout` controls overall discovery time and can disable discovery when set to `0s`.
