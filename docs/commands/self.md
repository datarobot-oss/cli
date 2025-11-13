# `dr self` - CLI Utility Commands

Commands for managing and configuring the DataRobot CLI itself.

## Synopsis

```bash
dr self <command>
```

## Description

The `self` command provides utility functions for managing the CLI tool itself, including version information and shell completion setup.

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

### Check CLI version

```bash
$ dr self version
DataRobot CLI version: 1.0.0
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
- [Getting started](../user-guide/getting-started.md)&mdash;initial CLI setup
