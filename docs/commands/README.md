# Command Reference

Complete reference documentation for all DataRobot CLI commands.

## Global flags

These flags are available for all commands:

```bash
  -v, --verbose    Enable verbose output (info level logging)
      --debug      Enable debug output (debug level logging)
  -h, --help       Show help information
```

## Commands

### Main commands

| Command | Description |
|---------|-------------|
| [`auth`](auth.md) | Authenticate with DataRobot. |
| [`templates`](templates.md) | Manage application templates. |
| [`run`](run.md) | Execute application tasks. |
| [`dotenv`](dotenv.md) | Manage environment variables. |
| [`completion`](completion.md) | Generate shell completions. |
| [`version`](version.md) | Show version information. |

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
├── run                Task execution
├── dotenv             Environment configuration
├── completion         Shell completion
│   ├── bash           Generate bash completion
│   ├── zsh            Generate zsh completion
│   ├── fish           Generate fish completion
│   └── powershell     Generate PowerShell completion
└── version            Version information
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

### Environment Configuration

```bash
# Interactive wizard
dr dotenv --wizard

# Editor mode
dr dotenv
```

### Running Tasks

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
dr completion bash | sudo tee /etc/bash_completion.d/dr

# Zsh
dr completion zsh > "${fpath[1]}/_dr"

# Fish
dr completion fish > ~/.config/fish/completions/dr.fish
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
  - Run tasks defined in Taskfile.
  - Support for parallel execution.
  - Watch mode for continuous tasks.

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
DATAROBOT_CONFIG_PATH          # Custom config file location
DATAROBOT_ENDPOINT             # DataRobot URL
DATAROBOT_API_KEY              # API key (not recommended)

# Logging
DR_LOG_LEVEL                   # Log level: debug, info, warn, error
NO_COLOR                       # Disable color output

# Development
DR_DEBUG                       # Enable debug mode
```

## Exit codes

| Code | Meaning |
|------|---------|  
| 0 | Success. |
| 1 | General error. |
| 2 | Command usage error. |
| 130 | Interrupted (Ctrl+C). |## See Also

- [Getting Started Guide](../user-guide/getting-started.md)
- [User Guide](../user-guide/)
- [Template System](../template-system/)
