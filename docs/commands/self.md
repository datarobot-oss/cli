# `dr self` - CLI Utility Commands

Commands for managing and configuring the DataRobot CLI itself.

## Synopsis

```bash
dr self <command>
```

## Description

The `self` command provides utility functions for managing the CLI tool itself, including updating to the latest version, checking version information, and setting up shell completion.

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

### `update`

Update the DataRobot CLI to the latest version.

```bash
dr self update
```

This command automatically detects your installation method and uses the appropriate update mechanism:

- **Homebrew (macOS)**&mdash;uses `brew update && upgrade dr-cli` if installed via Homebrew
- **Windows**&mdash;runs the PowerShell installation script
- **macOS/Linux**&mdash;runs the shell installation script

The update process will download and install the latest version while preserving your configuration and credentials.

**Examples:**

```bash
# Update to latest version
dr self update
```

**Note:** This command requires an active internet connection and appropriate permissions to install software on your system.

### `version`

Display version information about the CLI.

```bash
dr self version
```

**Options:**

- `-f, --format`&mdash;output format (`text` or `json`)

**Examples:**

```bash
# Show version (default text format)
dr self version

# Show version in JSON format
dr self version --format json
```

## Global flags

All `dr` global flags are available:

- `-v, --verbose`&mdash;enable verbose output
- `--debug`&mdash;enable debug output
- `-h, --help`&mdash;show help information

## Examples

### Update CLI to latest version

```bash
$ dr self update
Downloading latest version...
Installing DataRobot CLI...
✓ Successfully updated to version 1.1.0
```

### Check CLI version

```bash
$ dr self version
DataRobot CLI version: 1.0.0
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
$ dr self version --format json
{
  "version": "1.0.0",
  "commit": "abc123",
  "buildDate": "2025-11-10T12:00:00Z"
}
```

## See also

- [Shell completions guide](../user-guide/shell-completions.md)&mdash;detailed completion setup
- [Completion command](completion.md)&mdash;completion command reference
- [Quick start](../../README.md#quick-start)&mdash;initial CLI setup
