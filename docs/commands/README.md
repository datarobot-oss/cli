# Command Reference

Complete reference documentation for all DataRobot CLI commands.

## Global flags

These flags are available for all commands:

```bash
  -v, --verbose       Enable verbose output (info level logging)
      --debug         Enable debug output (debug level logging)
      --skip-auth     Skip authentication checks (for advanced users)
      --force-interactive  Force the setup wizard to run even if already completed
  -h, --help          Show help information
```

> [!WARNING]
> ⚠️ The `--skip-auth` flag is intended for advanced use cases only. Using this flag will bypass all authentication checks, which may cause API calls to fail. Use with caution.

> [!NOTE]
> The `--force-interactive` flag forces commands to behave as if setup has never been completed, while still updating the state file. This is useful for testing or forcing re-execution of setup steps.

## Commands

### Main commands

| Command | Description |
|---------|-------------|
| [`auth`](auth.md) | Authenticate with DataRobot. |
| [`templates`](templates.md) | Manage application templates. |
| [`start`](start.md) | Run the application quickstart process. |
| [`run`](run.md) | Execute application tasks. |
| [`task`](task.md) | Manage Taskfile composition and task execution. |
| [`dotenv`](dotenv.md) | Manage environment variables. |
| [`self`](self.md) | CLI utility commands (update, version, completion). |

### Command tree

```
dr
├── auth                Authentication management
│   ├── login          Log in to DataRobot
│   ├── logout         Log out from DataRobot
│   └── set-url        Set DataRobot URL
├── templates          Template management
│   ├── list           List available templates
│   ├── clone          Clone a template
│   ├── setup          Interactive setup wizard
│   └── status         Show template status
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

# Clone template
dr templates clone python-streamlit

# Interactive setup
dr templates setup

# Check status
dr templates status
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
  - `login`&mdash;OAuth authentication.
  - `logout`&mdash;remove credentials.
  - `set-url`&mdash;configure DataRobot URL.

- **[templates](templates.md)**&mdash;template operations.
  - `list`&mdash;list available templates.
  - `clone`&mdash;clone a template repository.
  - `setup`&mdash;interactive wizard for full setup.
  - `status`&mdash;show current template status.

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

- **[completion](completion.md)**&mdash;shell completions.
  - Bash, Zsh, Fish, PowerShell support.
  - Auto-complete commands and flags.

- **[version](version.md)**&mdash;version information.
  - Show CLI version.
  - Build information.
  - Runtime details.

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
dr templates clone --help
```

## Environment variables

Global environment variables that affect all commands:

```bash
# Configuration
DATAROBOT_ENDPOINT             # DataRobot URL
DATAROBOT_API_TOKEN            # API token (not recommended)
```

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | Success. |
| 1 | General error. |
| 2 | Command usage error. |
| 130 | Interrupted (Ctrl+C). |

## See also

- [Quick start](../../README.md#quick-start)
- [User guide](../user-guide/)
- [Template system](../template-system/)
