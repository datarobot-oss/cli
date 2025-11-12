# DataRobot CLI

[![Go Report Card](https://goreportcard.com/badge/github.com/datarobot/cli)](https://goreportcard.com/report/github.com/datarobot/cli)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE.txt)

The DataRobot CLI (`dr`) is a command-line interface for managing DataRobot custom applications. It provides an interactive experience for cloning, configuring, and deploying DataRobot application templates with built-in authentication, environment configuration, and task execution capabilities.

## Features

- 🔐 **Authentication management**&mdash;seamless OAuth integration with DataRobot.
- 📦 **Template management**&mdash;clone and configure application templates interactively.
- ⚙️ **Interactive configuration**&mdash;smart wizard for environment setup with validation.
- 🚀 **Task runner**&mdash;execute application tasks with built-in Taskfile integration.
- 🐚 **Shell completions**&mdash;support for Bash, Zsh, Fish, and PowerShell.
- 🎨 **Beautiful TUI**&mdash;terminal UI built with Bubble Tea for an enhanced user experience.

## Table of Contents

- [Installation](#installation)
- [Quick Start](#quick-start)
- [Documentation](#documentation)
- [Commands](#commands)
- [Shell Completion](#shell-completion)
- [Contributing](#contributing)
- [License](#license)

## Installation

### Quick Install (Recommended)

Install the latest version with a single command:

#### macOS/Linux

```bash
curl https://cli.datarobot.com/install | sh
```

#### Windows (PowerShell)

```powershell
irm https://cli.datarobot.com/winstall | iex
```

### Install Specific Version

#### macOS/Linux (Specific Version)

```bash
curl https://cli.datarobot.com/install | sh -s -- v0.1.0
```

#### Windows (Specific Version)

```powershell
$env:VERSION = "v0.1.0"; irm https://cli.datarobot.com/winstall | iex
```


## Quick start

### 1. Set up authentication

Configure your DataRobot credentials:

```bash
# Set your DataRobot URL (interactive)
dr auth set-url

# Or specify directly
dr auth set-url https://app.datarobot.com

# Log in to DataRobot (opens browser for OAuth)
dr auth login
```

### 2. Set up a template

Use the interactive setup wizard to clone and configure a template:

```bash
dr templates setup
```

Follow the prompts to select a template, clone it, and configure environment variables.

### 3. Start your application

Once configured, launch your application with the quickstart command:

```bash
dr start
```

This will either execute your template's quickstart script or guide you through the setup process if one hasn't been completed yet.

### 4. Run tasks

Execute tasks defined in your template Taskfile:

```bash
# List available tasks
dr run --list

# Run a specific task
dr run dev

# Run multiple tasks in parallel
dr run lint test --parallel
```

## Documentation

Comprehensive documentation is available in the [docs/](docs/) directory:

- **[User guide](docs/user-guide/)**&mdash;complete usage guide for all features.
  - [Getting started](docs/user-guide/getting-started.md)
  - [Authentication](docs/user-guide/authentication.md)
  - [Working with templates](docs/user-guide/templates.md)
  - [Shell completions](docs/user-guide/shell-completions.md)
  - [Configuration files](docs/user-guide/configuration.md)

- **[Template system](docs/template-system/)**&mdash;understanding the template configuration system.
  - [Template structure](docs/template-system/structure.md)
  - [Interactive configuration](docs/template-system/interactive-config.md)
  - [Environment variables](docs/template-system/environment-variables.md)

- **[Command reference](docs/commands/)**&mdash;detailed command documentation.
  - [auth](docs/commands/auth.md)&mdash;authentication commands.
  - [templates](docs/commands/templates.md)&mdash;template management.
  - [run](docs/commands/run.md)&mdash;task execution.
  - [dotenv](docs/commands/dotenv.md)&mdash;environment file management.
  - [completion](docs/commands/completion.md)&mdash;shell completion setup.

- **[Development guide](docs/development/)**&mdash;for contributors, see [CONTRIBUTING.md](CONTRIBUTING.md)

## Commands

### Main commands

| Command | Description |
|---------|-------------|
| `dr auth` | Authentication management (login, logout, set-url). |
| `dr templates` | Template operations (list, clone, setup, status). |
| `dr run` | Execute application tasks. |
| `dr dotenv` | Manage environment variables interactively. |
| `dr completion` | Generate shell completion scripts. |
| `dr version` | Show version information. |

### Examples

```bash
# Authentication
dr auth login
dr auth logout
dr auth set-url https://app.datarobot.com

# Template management
dr templates list                    # List available templates.
dr templates clone TEMPLATE_NAME     # Clone a specific template.
dr templates setup                   # Interactive template setup wizard.
dr templates status                  # Show current template status.

# Environment configuration
dr dotenv                           # Interactive environment editor.
dr dotenv --wizard                  # Configuration wizard mode.

# Task execution
dr run --list                       # List available tasks.
dr run dev                          # Run development server.
dr run build deploy --parallel      # Run multiple tasks in parallel.
dr run test --watch                 # Run tests in watch mode.

# Get help
dr --help
dr templates --help
dr run --help
```

## Shell completion

The CLI supports shell completions for Bash, Zsh, Fish, and PowerShell with automatic installation and configuration.

### Quick setup (Recommended)

The easiest way to install completions is using the interactive installer:

```bash
# Install completions for your current shell
dr completion install --yes

# Preview what would be installed (default behavior)
dr completion install

# Install for a specific shell
dr completion install bash --yes
dr completion install zsh --yes
```

The installer will:

- Detect your shell automatically
- Install completions to the correct location
- Configure your shell profile
- Clear completion caches

**Note:** Installation scripts (`install.sh` and `install.ps1`) automatically prompt to install completions during initial setup.

### Manual setup

If you prefer manual installation, generate the completion script:

#### Bash

```bash
# Linux
dr completion bash | sudo tee /etc/bash_completion.d/dr

# macOS (requires bash-completion from Homebrew)
dr completion bash > $(brew --prefix)/etc/bash_completion.d/dr
```

#### Zsh

```bash
# Oh-My-Zsh
dr completion zsh > ~/.oh-my-zsh/custom/completions/_dr

# Standard Zsh
dr completion zsh > ~/.zsh/completions/_dr
```

#### Fish

```bash
dr completion fish > ~/.config/fish/completions/dr.fish
```

#### PowerShell

```powershell
dr completion powershell | Out-String | Invoke-Expression
# To persist, add to your PowerShell profile
```

### Uninstalling completions

```bash
dr completion uninstall --yes
```

See the [Shell Completion Guide](docs/user-guide/shell-completions.md) for detailed instructions and troubleshooting.

## Contributing

We welcome contributions! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for details on:

- Development environment setup
- Coding standards and guidelines
- Testing requirements
- Submitting pull requests
- Release process

## License

Copyright 2025 DataRobot, Inc. and its affiliates. All rights reserved.

This is proprietary source code of DataRobot, Inc. See [LICENSE.txt](LICENSE.txt) for details.

## Support

- 📖 [Documentation](docs/)
- 🐛 [Issue Tracker](https://github.com/datarobot/cli/issues)
- 💬 [Discussions](https://github.com/datarobot/cli/discussions)
- 📧 Email: <oss-community-management@datarobot.com>

## Acknowledgments

Built with:

- [Cobra](https://github.com/spf13/cobra)&mdash;CLI framework
- [Bubble Tea](https://github.com/charmbracelet/bubbletea)&mdash;terminal UI framework
- [Viper](https://github.com/spf13/viper)&mdash;configuration management
- [Task](https://taskfile.dev/)&mdash;task runner
