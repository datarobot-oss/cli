# Quick reference

A one-page reference for the most common DataRobot CLI commands.

## Authentication

Manage your DataRobot credentials and connection settings. See the [auth command documentation](../commands/auth.md) for detailed information.

```bash
# ⭐ Set DataRobot URL (interactive)
dr auth set-url

# Set DataRobot URL directly
dr auth set-url https://app.datarobot.com

# ⭐ Log in (opens browser)
dr auth login

# Log out
dr auth logout

# Check authentication status
dr auth check
```

## Templates

Discover, clone, and configure application templates. See the [template system documentation](../template-system/README.md) for detailed information.

```bash
# List available templates
dr templates list

# ⭐ Interactive setup wizard (recommended)
dr templates setup
```

## Environment configuration

Manage environment variables and `.env` files for your templates. See the [dotenv command documentation](../commands/dotenv.md) and [environment variables guide](../template-system/environment-variables.md) for detailed information.

```bash
# ⭐ Interactive environment setup
dr dotenv setup

# ⭐ Edit environment variables
dr dotenv edit

# Update DataRobot credentials
dr dotenv update

# Validate environment configuration
dr dotenv validate
```

## Running tasks

Execute tasks defined in your application templates. See the [run command documentation](../commands/run.md) and [start command documentation](../commands/start.md) for detailed information.

```bash
# ⭐ Quickstart (automated initialization)
dr start

# Quickstart (non-interactive)
dr start --yes

# List available tasks
dr run --list
dr task list

# ⭐ Run development server
dr run dev

# Run specific tasks
dr run build
dr run test
dr run lint

# Run multiple tasks in parallel
dr run lint test --parallel

# Run with watch mode
dr run dev --watch
```

## Common workflows

Step-by-step guides for typical tasks.

### First-time setup

```bash
# 1. Authenticate
dr auth set-url https://app.datarobot.com
dr auth login

# 2. Set up template
dr templates setup

# 3. Start application
cd [template-name]
dr start
```

### Daily development

```bash
# Navigate to project
cd my-template

# Start development server
dr run dev

# Run tests
dr run test

# Update environment variables
dr dotenv edit
```

### Update credentials

```bash
# Re-authenticate
dr auth login

# Update .env file
dr dotenv update
```

## CLI utilities

Manage the CLI itself: version, updates, and completions. See the [self command documentation](../commands/self.md) for detailed information.

```bash
# Show version
dr --version
dr self version

# Update CLI
dr self update

# Enable shell completions
dr self completion install [bash|zsh|fish|powershell]

# ⭐ Show help
dr --help
dr [command] --help
```

## Common flags

Useful flags that work with multiple commands. See the [global flags documentation](../commands/README.md#global-flags) for complete details.

```bash
# Verbose output
dr --verbose [command]

# Debug output
dr --debug [command]

# Skip confirmation prompts
dr start --yes
dr run [task] --yes

# List tasks
dr run --list

# Run tasks in parallel
dr run --parallel [task1] [task2]

# Custom config file
dr --config /path/to/config.yaml [command]
```

## File locations

Important files and where to find them. See the [configuration files documentation](configuration.md) for detailed information about file locations and management.

| File/Directory       | Location                                            |
|----------------------|-----------------------------------------------------|
| Config file          | `~/.config/datarobot/drconfig.yaml`                 |
| State file           | `.datarobot/cli/state.yaml` (in template directory) |
| Environment file     | `.env` (in template directory)                      |
| Environment template | `.env.template` (in template directory)             |

## Getting help

Find help and debug issues. See [Getting help](../../README.md#getting-help) in the main README for additional resources.

```bash
# General help
dr --help

# Command-specific help
dr auth --help
dr templates --help
dr run --help

# Enable verbose output
dr --verbose [command]

# Enable debug output
dr --debug [command]
```

## See also

- [Full command reference](../commands/README.md) - Complete command documentation
- [User guide](README.md) - Detailed usage guides
- [Quick start](../../README.md#quick-start) - Step-by-step setup instructions
