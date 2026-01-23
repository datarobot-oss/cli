# plugins / plugin

The `dr plugin` (alias: `dr plugins`) commands help you inspect plugins discovered by the CLI.

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
