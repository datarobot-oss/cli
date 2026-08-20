# `dr self` - CLI utility commands

Commands for managing and configuring the DataRobot CLI itself.

## Quick start

For most users, common CLI management tasks are straightforward:

```bash
# Check CLI version
dr self version

# Update to latest version
dr self update

# View current configuration
dr self config
```

These commands help you manage the CLI tool itself, including updates, version information, and configuration.

> [!NOTE]
> **First time?** If you're new to the CLI, start with the [Quick start](../../README.md#quick-start) for step-by-step setup instructions.

## Synopsis

```bash
dr self <command>
```

## Description

The `self` command provides utility functions for managing the CLI tool itself, including updating to the latest version, checking version information, setting up shell completion, and (for plugin authors) packaging and publishing plugins.

## Subcommands

### `completion`

Generate or manage shell completion scripts for command auto-completion.

```bash
dr self completion <shell>
```

See the [completion documentation](completion.md) for detailed usage.

**Quick examples:**

```bash
# Install completions interactively
dr self completion install

# Generate completions for bash
dr self completion bash > /etc/bash_completion.d/dr

# Generate completions for zsh
dr self completion zsh > "${fpath[1]}/_dr"
```

### `config`

Display current configuration settings from config file and environment variables.

```bash
dr self config
```

This command shows all configuration values currently in use by the CLI, including settings from:

- Configuration file (`~/.config/datarobot/drconfig.yaml` or custom path)
- Environment variables (prefixed with `DATAROBOT_CLI_` or standard `DATAROBOT_` variables)
- Command-line flags

Sensitive values like API tokens are automatically redacted for security.

**Examples:**

```bash
# Display current configuration
dr self config
```

**Sample output:**

```text
Configuration initialized. Using config file: /Users/username/.config/datarobot/drconfig.yaml

  debug: false
  endpoint: https://app.datarobot.com/api/v2
  external_editor: vim
  token: ****
  verbose: false
```

**Use cases:**

- Verify which configuration file is being used
- Check that environment variables are being recognized
- Debug configuration issues
- Confirm API endpoint and settings before deployment

### `plugin`

Plugin packaging and development tools for creating and publishing CLI plugins. These subcommands are for plugin authors and registry maintainers.

| Subcommand | Description |
|------------|-------------|
| `add <path-to-index.json>` | Add a packaged plugin version to a registry file (index.json). |
| `publish <plugin-dir>` | Package and publish a plugin in one step (package, copy to plugins dir, update index). |
| `package <plugin-dir>` | Package a plugin directory into a distributable .tar.xz archive. |

See the [Plugin development](../development/plugins.md) guide for details. For installing and managing plugins as a user, use the top-level [`dr plugin`](plugins.md) command.

### `update`

Update the DataRobot CLI to the latest version.

```bash
dr self update
```

This command automatically detects your installation method and uses the appropriate update mechanism:

- **Homebrew (macOS/Linux)**&mdash;uses `brew update && upgrade dr-cli` if installed via Homebrew
- **Windows**&mdash;runs the PowerShell installation script
- **macOS/Linux**&mdash;runs the shell installation script

The update process will download and install the latest version while preserving your configuration and credentials.

**Options:**

- `--version <version>`&mdash;install a specific released version instead of the latest. Accepts `vX.Y.Z` or `X.Y.Z`, optionally with a `-prerelease` and/or `+build` suffix. Refuses to install a version older than the one currently running unless `--force`/`-f` is also set. Not available when `dr` was installed via the Homebrew cask&mdash;Homebrew always installs the latest release and cannot pin versions, so the command exits with an error showing the manual install command to run instead.
- `--force`/`-f`&mdash;force the update even if the installed version already satisfies requirements, and (combined with `--version`) allow installing an older version than the one currently running.

**Examples:**

```bash
# Update to latest version
dr self update

# Install a specific version
dr self update --version v0.12.3
dr self update --version 0.12.3
```

> [!NOTE]
> This command requires an active internet connection and appropriate permissions to install software on your system.

### `version`

Display version information about the CLI.

```bash
dr self version
```

**Options:**

- `-o, --output-format`&mdash;output format (`text` or `json`, default: `text`)
- `-s, --short`&mdash;print just the version number (text format only)

**Examples:**

```bash
# Show version (default text format)
dr self version

# Show version in JSON format
dr self version --output-format json

# Show just the version number
dr self version --short
```

## Global options

All `dr` global options are available:

- `-v, --verbose`&mdash;enable verbose output
- `--debug`&mdash;enable debug output
- `-h, --help`&mdash;show help information

## Examples

### Update CLI to latest version

```bash
$ dr self update
Downloading latest version...
Installing DataRobot CLI...
✓ Successfully updated to v0.2.55
```

### Check CLI version

```bash
$ dr self version
DataRobot CLI version: v0.2.55
```

### View current configuration

```bash
$ dr self config
Configuration initialized. Using config file: /Users/username/.config/datarobot/drconfig.yaml

  debug: false
  endpoint: https://app.datarobot.com/api/v2
  external_editor: vim
  token: ****
  verbose: false
```

### Install shell completions

```bash
# Interactive installation
$ dr self completion install
✓ Detected shell: zsh
✓ Installing completions to: ~/.zsh/completions/_dr
✓ Completions installed successfully!

# Manual installation
$ dr self completion bash | sudo tee /etc/bash_completion.d/dr
```

### Get version in JSON

```bash
$ dr self version --output-format json
{
  "version": "v0.2.55",
  "commit": "abc1234def5678",
  "build_date": "2025-03-30T12:00:00Z"
}
```

## See also

- [Shell completions guide](../user-guide/shell-completions.md)&mdash;detailed completion setup
- [Completion command](completion.md)&mdash;completion command reference
- [Plugins command](plugins.md)&mdash;install and manage plugins (user-facing)
- [Plugin development](../development/plugins.md)&mdash;creating and publishing plugins
- [Quick start](../../README.md#quick-start)&mdash;initial CLI setup
