# Remote Plugin Download & Installation System

This document describes the design and implementation plan for the remote plugin download and installation system, which allows users to install, update, and uninstall plugins from a central registry hosted at `cli.datarobot.com/plugins/index.json`.

## Overview

The system enables:
- Downloading and installing versioned plugins from a remote registry
- Semver-based version constraints (exact, caret, tilde, range)
- Platform-agnostic plugin packages containing both Posix and Windows scripts
- Python-based plugins wrapped in shell scripts using `uv run`

## Architecture

### Plugin Index (`index.json`)

A minimal JSON schema pointing to plugin archives:

```json
{
  "version": "1",
  "plugins": {
    "apps": {
      "name": "apps",
      "description": "Host custom applications in DataRobot",
      "repository": "https://github.com/datarobot/dr-apps",
      "versions": [
        {
          "version": "1.0.0",
          "url": "dr-apps/dr-apps-1.0.0.tar.xz",
          "sha256": "...",
          "releaseDate": "2026-01-27"
        }
      ]
    }
  }
}
```

**Note**: URLs can be relative (e.g., `dr-apps/dr-apps-1.0.0.tar.xz`) or absolute (e.g., `https://example.com/plugins/dr-apps-1.0.0.tar.xz`). Relative URLs are resolved against the base URL of the index. This enables the same index to work with both local development servers and production CDNs
```

### Plugin Package Structure

Each `.tar.xz` archive contains:

```
manifest.json          # Plugin metadata and platform script mappings
scripts/
  dr-<name>.sh         # Posix wrapper script
  dr-<name>.ps1        # Windows PowerShell script
```

### Manifest Schema

The `manifest.json` inside each package defines platform-specific executables:

```json
{
  "name": "apps",
  "version": "1.0.0",
  "description": "Host custom applications in DataRobot",
  "minCLIVersion": "0.2.0",
  "scripts": {
    "posix": "scripts/dr-apps.sh",
    "windows": "scripts/dr-apps.ps1"
  }
}
```

## Implementation Steps

### 1. Create Plugin Index Schema

- Add `docs/plugins/index.json` with minimal schema
- Support plugin names, semver versions, descriptions, and download URLs
- Include SHA256 checksums per artifact for verification

### 2. Build dr-apps Plugin Package (PoC)

- Create wrapper scripts:
  - `dr-apps.sh` (Posix): Executes `uv run --with drapps drapps "$@"`
  - `dr-apps.ps1` (Windows): PowerShell equivalent
- Package with `manifest.json` defining platform script mappings
- Create `.tar.xz` archive and publish to `docs/plugins/<plugin>/`

### 3. Implement Plugin Install Command

- Create `cmd/plugin/install/cmd.go`
- Fetch `index.json` from remote URL
- Parse semver constraints using version comparison logic
- Select appropriate platform (windows vs posix)
- Download and verify SHA256 checksums
- Extract to `~/.config/datarobot/plugins/<name>/`
- Make scripts executable
- Persist installation metadata in `.installed.json`

### 4. Update Plugin Discovery

- Modify `internal/plugin/discover.go`
- Search `~/.config/datarobot/plugins/` first (higher priority than PATH)
- Scan subdirectories for `manifest.json`
- Resolve platform-specific executable from manifest's `scripts` field
- Maintain backward compatibility with PATH-based plugins

### 5. Add Update/Uninstall Commands

- Create `cmd/plugin/update/cmd.go`:
  - Compare installed vs available versions
  - Re-download and reinstall if newer version exists
  - Support `--all` flag for bulk updates
- Create `cmd/plugin/uninstall/cmd.go`:
  - Remove plugin directory from managed plugins
  - Clean up installation metadata

## Semver Support

Version constraints supported:
- **Exact**: `1.2.3` - Match exactly this version
- **Caret**: `^1.2.3` - Any version compatible with major (1.x.x >= 1.2.3)
- **Tilde**: `~1.2.3` - Any version compatible with minor (1.2.x >= 1.2.3)
- **Range**: `>=1.0.0` - Any version at or above
- **Latest**: `latest` or empty - Most recent version

## Directory Structure

```
~/.config/datarobot/
  plugins/
    apps/                    # Managed plugin directory
      manifest.json          # Plugin manifest
      .installed.json        # Installation metadata
      scripts/
        dr-apps.sh
        dr-apps.ps1
```

## Plugin Execution

When a managed plugin is invoked:
1. Discovery finds the plugin in `~/.config/datarobot/plugins/`
2. Platform-specific script is resolved from `manifest.json`
3. On Posix: Execute `.sh` script with bash
4. On Windows: Execute `.ps1` script with PowerShell

## Python Plugin Pattern

For Python-based plugins, wrapper scripts use `uv` for dependency management:

```bash
#!/usr/bin/env bash
exec uv run --with drapps drapps "$@"
```

This pattern:
- Eliminates bundling Python dependencies
- Leverages `uv`'s fast package resolution
- Works in air-gapped environments when pre-cached

## Files Created

| File | Purpose |
|------|---------|
| `docs/plugins/index.json` | Remote plugin registry |
| `docs/plugins/dr-apps/manifest.json` | Plugin manifest |
| `docs/plugins/dr-apps/scripts/dr-apps.sh` | Posix wrapper |
| `docs/plugins/dr-apps/scripts/dr-apps.ps1` | Windows wrapper |
| `docs/plugins/dr-apps/dr-apps-1.0.0.tar.xz` | Packaged plugin archive |
| `cmd/plugin/install/cmd.go` | Install command |
| `cmd/plugin/uninstall/cmd.go` | Uninstall command |
| `cmd/plugin/update/cmd.go` | Update command |
| `internal/plugin/remote.go` | Remote fetch & install logic |
| `internal/plugin/types.go` | Extended type definitions |
| `cmd/self/plugin/package/cmd.go` | Package command |
| `cmd/self/plugin/add/cmd.go` | Add to index command |
| `cmd/self/plugin/publish/cmd.go` | Publish command (all-in-one) |

## Files Modified

| File | Changes |
|------|---------|
| `cmd/plugin/cmd.go` | Register new subcommands |
| `internal/plugin/discover.go` | Scan managed plugins directory |
| `internal/repo/paths.go` | Added `ManagedPluginsDir()` |

## Usage Examples

```bash
# List available plugins from registry
dr plugin install --list
dr plugin install --list --index-url http://127.0.0.1:8000/cli/dev-docs/plugins

# Install a plugin
dr plugin install apps
dr plugin install apps --version 1.0.0
dr plugin install apps --version "^1.0.0"
dr plugin install apps --index-url http://127.0.0.1:8000/cli/dev-docs/plugins

# Update plugins
dr plugin update apps
dr plugin update --all

# Uninstall a plugin
dr plugin uninstall apps

# List installed plugins
dr plugin list
```

## Future Considerations

1. **UV prerequisite checking**: Add `uv` to prerequisites validation
2. **Plugin update notifications**: Show "(update available)" in `dr plugin list`
3. **GitHub Actions automation**: Automate plugin publishing workflow
4. **Code signing**: Add verification for macOS/Windows binaries
