# Command reference

Complete reference documentation for all DataRobot CLI commands.

This document provides a comprehensive overview of all available commands, their flags, and usage examples. For getting started with the CLI, see the [Quick start guide](../../README.md#quick-start).

## Global flags

These flags are available for all commands:

```bash
  -V, --version           Display version information
  -v, --verbose           Enable verbose output (info level logging)
      --debug             Enable debug output (debug level logging)
      --config string     Path to config file (default: $HOME/.datarobot/drconfig.yaml)
      --skip-auth         Skip authentication checks (for advanced users)
      --force-interactive Force the setup wizard to run even if already completed
      --all-commands      Display all available commands and their flags in tree format
  -h, --help              Show help information
```

> [!WARNING]
> ⚠️ The `--skip-auth` flag is intended for advanced use cases only. Using this flag will bypass all authentication checks, which may cause API calls to fail. Use with caution.

> [!NOTE]
> The `--force-interactive` flag forces commands to behave as if setup has never been completed, while still updating the state file. This is useful for testing or forcing re-execution of setup steps.

## Commands

### Main commands

| Command               | Description                                         |
|-----------------------|-----------------------------------------------------|
| [`auth`](auth.md)     | Authenticate with DataRobot.                        |
| `component`           | Manage template components.                         |
| `templates`           | Manage application templates.                       |
| [`start`](start.md)   | Run the application quickstart process.             |
| [`run`](run.md)       | Execute application tasks.                          |
| [`task`](task.md)     | Manage Taskfile composition and task execution.     |
| [`dotenv`](dotenv.md) | Manage environment variables.                       |
| [`self`](self.md)     | CLI utility commands (update, version, completion). |

### Command tree

```text
dr
├── auth                Authentication management
│   ├── check          Check if credentials are valid
│   ├── login          Log in to DataRobot
│   ├── logout         Log out from DataRobot
│   └── set-url        Set DataRobot URL
├── component          Component management
│   ├── add            Add a component to your template
│   ├── list           List installed components
│   └── update         Update a component
├── templates          Template management
│   ├── list           List available templates
│   └── setup          Interactive setup wizard
├── start              Run quickstart process (alias: quickstart)
├── run                Task execution
├── task               Taskfile composition and execution
│   ├── compose        Compose unified Taskfile
│   ├── list           List available tasks
│   └── run            Execute tasks
├── dotenv             Environment configuration
└── self               CLI utility commands
    ├── completion     Shell completion
    │   ├── bash       Generate bash completion
    │   ├── zsh        Generate zsh completion
    │   ├── fish       Generate fish completion
    │   └── powershell Generate PowerShell completion
    ├── config         Display configuration settings
    ├── update         Update CLI to latest version
    └── version        Version information
```

## Quick examples

### Authentication

```bash
# Set URL and login
dr auth set-url https://app.datarobot.com
dr auth login

# Logout
dr auth logout
```

### Templates

```bash
# List templates
dr templates list

# Interactive setup
dr templates setup
```

### Components

```bash
# List installed components
dr component list

# Add a component
dr component add <component-url>

# Update a component
dr component update
```

### Quickstart

```bash
# Run quickstart process (interactive)
dr start

# Run with auto-yes
dr start --yes

# Using the alias
dr quickstart
```

### Environment configuration

```bash
# Interactive wizard
dr dotenv setup

# Editor mode
dr dotenv edit

# Validate configuration
dr dotenv validate
```

### Running tasks

```bash
# List available tasks
dr run --list

# Run a task
dr run dev

# Run multiple tasks
dr run lint test --parallel
```

### Shell completions

```bash
# Bash (Linux)
dr self completion bash | sudo tee /etc/bash_completion.d/dr

# Zsh
dr self completion zsh > "${fpath[1]}/_dr"

# Fish
dr self completion fish > ~/.config/fish/completions/dr.fish
```

### CLI management

```bash
# Update to latest version
dr self update

# Check version
dr self version
```

## Command details

For detailed documentation on each command, see:

- **[auth](auth.md)**&mdash;authentication management.
  - `check`&mdash;verify credentials are valid.
  - `login`&mdash;OAuth authentication.
  - `logout`&mdash;remove credentials.
  - `set-url`&mdash;configure DataRobot URL.

- **component**&mdash;component management (alias: `c`).
  - `add`&mdash;add a component to your template.
  - `list`&mdash;list installed components.
  - `update`&mdash;update a component.
  - Note: Components are reusable pieces that can be added to templates to extend functionality.

- **templates**&mdash;template operations.
  - `list`&mdash;list available templates.
  - `setup`&mdash;interactive wizard for full setup.

- **[run](run.md)**&mdash;task execution.
  - Execute template tasks.
  - List available tasks.
  - Parallel execution support.
  - Watch mode for development.

- **[task](task.md)**&mdash;Taskfile composition and management.
  - `compose`&mdash;generate unified Taskfile from components.
  - `list`&mdash;list all available tasks.
  - `run`&mdash;execute tasks.

- **[dotenv](dotenv.md)**&mdash;environment management.
  - Interactive configuration wizard.
  - Direct file editing.
  - Variable validation.

- **[self](self.md)**&mdash;CLI utility commands.
  - `completion`&mdash;shell completions (Bash, Zsh, Fish, PowerShell).
  - `config`&mdash;display configuration settings.
  - `update`&mdash;update CLI to latest version.
  - `version`&mdash;show CLI version and build information.

## Getting help

```bash
# General help
dr --help
dr -h

# Command help
dr auth --help
dr templates --help
dr run --help

# Subcommand help
dr auth login --help
dr templates setup --help
dr component add --help
```

## Environment variables

Global environment variables that affect all commands:

```bash
# Configuration
DATAROBOT_ENDPOINT             # DataRobot URL
DATAROBOT_API_TOKEN            # API token (not recommended)
DATAROBOT_CLI_CONFIG           # Path to config file
VISUAL                         # External editor for file editing
EDITOR                         # External editor for file editing (fallback)
```

## Exit codes

| Code | Meaning               |
|------|-----------------------|
| 0    | Success.              |
| 1    | General error.        |
| 2    | Command usage error.  |
| 130  | Interrupted (Ctrl+C). |

## See also

- [Quick start](../../README.md#quick-start)
- [User guide](../user-guide/)
- [Template system](../template-system/)
