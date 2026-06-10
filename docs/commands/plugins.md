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
dr plugin install PLUGIN_NAME --file ./plugin-0.2.0.tar.xz
dr plugin install --file ./plugin-0.2.0.tar.xz
dr plugin install PLUGIN_NAME --url https://example.com/plugin-0.2.0.tar.xz
dr plugin install --url https://example.com/plugin-0.2.0.tar.xz
```

Installs a plugin from the remote plugin registry, a local archive, or an HTTP/HTTPS URL.

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
- `--file`&mdash;install from a local `.tar.xz` archive instead of the registry.
- `--url`&mdash;install from an HTTP/HTTPS URL instead of the registry.

`--file` and `--url` are mutually exclusive with each other and with `--version`, `--versions`, `--list`, and `--registry-url`.

When no plugin name argument is given with `--file` or `--url`, the name is read from `manifest.json` inside the archive. You can pass an explicit name as the first argument to override this.

> **Note:** URL installs bypass registry checksum verification. Only install from URLs you trust.

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

# Install from a local archive (name read from manifest.json inside the archive).
dr plugin install --file ./assist-0.2.0.tar.xz

# Install from a local archive with an explicit plugin name.
dr plugin install assist --file ./assist-0.2.0.tar.xz

# Install from an HTTP/HTTPS URL (name read from manifest.json inside the archive).
dr plugin install --url https://example.com/assist-0.2.0.tar.xz

# Install from an HTTP/HTTPS URL with an explicit plugin name.
dr plugin install assist --url https://example.com/assist-0.2.0.tar.xz
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

## Automatic update check

When you invoke a managed plugin (one installed via `dr plugin install`), the CLI automatically
checks for a newer version in the background before running the plugin. If an update is
available you will be prompted:

```
 Plugin "assist" update available: v0.1.15 → v0.2.0
 Do you want to update? [Y/n]
```

- Press **Enter** or type **y** to update immediately (backup → install → validate → rollback on failure).
- Type **n** to skip and continue running the current version.

Either way, the check is not repeated until the configured cooldown interval has elapsed.

### Update check behavior

| Situation | Behavior |
|---|---|
| No internet / registry unreachable | Silently skipped — plugin runs normally |
| Plugin is already up to date | No prompt — plugin runs normally |
| PATH-based or project-local plugin | Skipped — only managed plugins are checked |
| Cooldown period has not elapsed | Skipped — plugin runs normally |

### Configuring the update check

```bash
# Change the cooldown interval (default 24h)
# Accepts Go duration strings: 30m, 6h, 48h, 0s
dr --plugin-update-check-interval 6h assist

# Disable the automatic check entirely for one invocation
dr --skip-plugin-update-check assist
```

To permanently disable the check, set the flag via your shell profile:

```bash
# ~/.zshrc or ~/.bashrc
alias dr='dr --skip-plugin-update-check'
```

### Resetting the cooldown

The cooldown is stored in `~/.config/datarobot/state.yaml`. Delete the file (or the relevant
entry) to force an immediate check on next run:

```bash
# Reset all plugin cooldowns
rm ~/.config/datarobot/state.yaml

# Reset a single plugin's cooldown
yq -i 'del(.plugin_update_checks.assist)' ~/.config/datarobot/state.yaml
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
