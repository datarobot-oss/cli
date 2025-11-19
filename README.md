<p align="center">
  <a href="https://github.com/datarobot-community/datarobot-agent-templates">
    <img src="./.github/datarobot_logo.avif" width="600px" alt="DataRobot Logo"/>
  </a>
</p>
<p align="center">
    <span style="font-size: 1.5em; font-weight: bold; display: block;">DataRobot CLI</span>
</p>

<p align="center">
  <a href="https://datarobot.com">Homepage</a>
  ·
  <a href="https://docs.datarobot.com/">Documentation</a>
  ·
  <a href="https://docs.datarobot.com/en/docs/get-started/troubleshooting/general-help.html">Support</a>
</p>

<p align="center">
  <a href="https://github.com/datarobot-oss/cli/tags">
    <img src="https://img.shields.io/github/v/tag/datarobot-oss/cli?label=version" alt="Latest Release">
  </a>
  <a href="LICENSE.txt">
    <img src="https://img.shields.io/github/license/datarobot-oss/cli" alt="License">
  </a>
</p>

# DataRobot CLI

[![Go Report Card](https://goreportcard.com/badge/github.com/datarobot/cli)](https://goreportcard.com/report/github.com/datarobot/cli)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE.txt)

The DataRobot CLI (`dr`) is a command-line interface for managing DataRobot custom applications.
It provides an interactive experience for cloning, configuring, and deploying DataRobot application templates with built-in authentication, environment configuration, and task execution capabilities.

If you're new to DataRobot, visit the [DataRobot documentation](https://docs.datarobot.com/) to learn more about the platform.

## Features

- 🔐 **Authentication management**&mdash;seamless OAuth integration with DataRobot.
- 📦 **Template management**&mdash;clone and configure application templates interactively.
- ⚙️ **Interactive configuration**&mdash;smart wizard for environment setup with validation.
- 🚀 **Task runner**&mdash;execute application tasks with built-in Taskfile integration.
- 🐚 **Shell completions**&mdash;support for Bash, Zsh, Fish, and PowerShell.
- 🔄 **Self-update capability**&mdash;easily update to the latest version with a single command.

## Table of contents

- [Prerequisites](#prerequisites)
- [Installation](#installation)
- [Quick start](#quick-start)
- [Next steps](#next-steps)
- [Contributing](#contributing)
- [Support](#support)
- [Acknowledgments](#acknowledgments)

## Prerequisites

Before you begin, ensure you have:

- **DataRobot account**&mdash;Access to a DataRobot instance (cloud or self-managed). If you don't have an account, sign up at [DataRobot](https://www.datarobot.com/) or contact your organization's DataRobot administrator.
- **Git**&mdash;For cloning templates (version 2.0+). Install Git from [git-scm.com](https://git-scm.com/downloads) if not already installed. Verify installation: `git --version`
- **Task**&mdash;For running tasks. Install Task from [taskfile.dev](https://taskfile.dev/installation/) if not already installed. Verify installation: `task --version`
- **Terminal**&mdash;Command-line interface access.
  - **macOS/Linux:** Use Terminal, iTerm2, or your preferred terminal emulator.
  - **Windows:** Use PowerShell, Command Prompt, or Windows Terminal.

## Installation

Install the latest version with a single command:

### macOS/Linux

```bash
curl https://cli.datarobot.com/install | sh
```

### Windows (PowerShell)

```powershell
irm https://cli.datarobot.com/winstall | iex
```

<details><summary><em>Click here for alternative installation methods</em></summary>
<br/>
The following are alternative installation methods for the DataRobot CLI.
You can choose to download a binary directly, install a specific version, or build and install from source.

### Download binary (recommended)

Download the latest release for your operating system:

#### macOS

```bash
# Intel Macs
curl -LO https://github.com/datarobot-oss/cli/releases/latest/download/dr-darwin-amd64
chmod +x dr-darwin-amd64
sudo mv dr-darwin-amd64 /usr/local/bin/dr

# Apple Silicon (M1/M2)
curl -LO https://github.com/datarobot-oss/cli/releases/latest/download/dr-darwin-arm64
chmod +x dr-darwin-arm64
sudo mv dr-darwin-arm64 /usr/local/bin/dr
```

#### Linux

```bash
# x86_64
curl -LO https://github.com/datarobot-oss/cli/releases/latest/download/dr-linux-amd64
chmod +x dr-linux-amd64
sudo mv dr-linux-amd64 /usr/local/bin/dr

# ARM64
curl -LO https://github.com/datarobot-oss/cli/releases/latest/download/dr-linux-arm64
chmod +x dr-linux-arm64
sudo mv dr-linux-arm64 /usr/local/bin/dr
```

#### Windows

Download `dr-windows-amd64.exe` from the [releases page](https://github.com/datarobot-oss/cli/releases/latest) and add it to your PATH.

### Install a specific version

If you'd like to install a specific version, you can do so by passing the version number to the installer, as shown below:

#### macOS/Linux

```bash
curl  https://cli.datarobot.com/install | sh -s -- v0.1.0
```

#### Windows (PowerShell)

```powershell
$env:VERSION = "v0.1.0"; irm https://cli.datarobot.com/winstall | iex
```

### Build and install from source

If you would like to build and install from source, you can do so by following the instructions below:

#### Prerequisites

- Go 1.25.3 or later (for building from source)
- Git
- [Task](https://taskfile.dev/) (for development and task running)

#### Build and install from source

```bash
# Clone the repository
git clone https://github.com/datarobot-oss/cli.git
cd cli

# Install Task (if not already installed)
go install github.com/go-task/task/v3/cmd/task@latest

# Build the binary
task build

# The binary will be at ./dist/dr
# Install it to your PATH (example for macOS/Linux)
sudo mv ./dist/dr /usr/local/bin/dr
```

#### Windows (install a specific version)

```powershell
# Clone the repository
git clone https://github.com/datarobot-oss/cli.git
cd cli

# Install Task (if not already installed)
go install github.com/go-task/task/v3/cmd/task@latest

# Build the binary
task build

# The binary will be at .\dist\dr.exe
# Add it to your PATH or move it to a directory in your PATH
```

</details>

### Verify installation

You can verify the installation by checking the version:

```bash
dr --version
```

You should see output similar to:

```text
DataRobot CLI (version v0.2.9)
```

### Updating the CLI

To update to the latest version of the DataRobot CLI, use the built-in update command:

```bash
dr self update
```

This command will automatically:

- Detect your installation method (Homebrew, manual installation, etc.)
- Download the latest version
- Install it using the appropriate method for your system
- Preserve your existing configuration and credentials

The update process supports:

- **Homebrew (macOS)**&mdash;automatically upgrades via `brew upgrade --cask dr-cli`
- **Windows**&mdash;runs the latest PowerShell installation script
- **macOS/Linux**&mdash;runs the latest shell installation script

After updating, verify the new version:

```bash
dr self version
```

## Quick start

Now that you have installed the DataRobot CLI, you can start using it to manage your DataRobot applications.
The following sections will walk you through configuring the CLI, setting up a template, and running tasks.

### Set up authentication

First, configure your DataRobot credentials by setting your DataRobot URL.
Refer to [DataRobot's API keys and tools page](https://docs.datarobot.com/en/docs/platform/acct-settings/api-key-mgmt.html) for steps to locate your DataRobot URL, also known as your DataRobot API endpoint.

```bash
# Set your DataRobot URL (interactive)
dr auth set-url
```

<details><summary><em>Click here for more information about configuration files</em></summary>
<br/>
Configuration files are stored in:

- **Linux/macOS:** `~/.config/datarobot/drconfig.yaml`
- **Windows:** `%USERPROFILE%\.config\datarobot\drconfig.yaml`

See [Configuration files](docs/user-guide/configuration.md) for more details.
</details>

You'll be prompted to enter your DataRobot URL. You can use shortcuts for cloud instances:

- Enter `1` for `https://app.datarobot.com`
- Enter `2` for `https://app.eu.datarobot.com`
- Enter `3` for `https://app.jp.datarobot.com`
- Enter a valid URL if you have a custom/self-managed instance

Alternatively, set the URL directly:

```bash
dr auth set-url https://app.datarobot.com
```

Once you have configured the URL, log in to DataRobot using OAuth:

```bash
dr auth login
```

This will:

1. Open your default web browser
2. Redirect you to the DataRobot login page
3. Request authorization
4. Automatically save your credentials

Your API key will be securely stored in `~/.config/datarobot/drconfig.yaml`.

### Verify authentication

Check that you're logged in:

```bash
dr templates list
```

This command displays a list of available templates from your DataRobot instance.

> [!TIP]
> **What's next?** Now that you're authenticated, you can:
>
> - Browse available templates: `dr templates list`
> - Start the setup wizard: `dr templates setup`
> - See the [Command reference](docs/commands/) for all available commands

### Set up a template

Next, load the interactive setup wizard to clone and configure a template.
A **template** is a pre-configured application scaffold that you can customize.
When you clone and configure a template, it becomes your **application**&mdash;a customized instance ready to run and deploy.

```bash
dr templates setup
```

After a few moments, the setup wizard displays the application templates available:

<img src="./images/templates-list.png" alt="Templates list" width="500px"/>

> [!NOTE]
> You can navigate through the list of templates using the arrow keys, or filter by pressing the `/` key and entering a search term. The setup wizard will only display templates that are available to you.

Select a template by pressing the `Enter` key.
At the subsequent prompt, specify the desired directory name for the template and press `Enter` to have the setup wizard clone the template repository to your local machine.

<details><summary><em>Click here for manual setup instructions</em></summary>
<br/>

**Manual setup:** If you prefer manual control:

```bash
# 1. List available templates.
dr templates list

# 2. Clone a specific template.
dr templates clone TEMPLATE_NAME

# 3. Navigate to the template directory.
cd TEMPLATE_NAME

# 4. Configure environment variables.
dr dotenv setup
```

> [!TIP]
> **What's next?** After configuring your template:
>
> - Start your application: `dr start` or `dr run dev`
> - Explore available tasks: `dr task list`
> - See [Run tasks](#run-tasks) below
>
</details>

Follow the instructions when prompted to continue configuring the template.
The prompts vary depending on which template you selected.
When all steps are finished, press `Enter` to exit the wizard and proceed to the next section.

> [!TIP]
> **What's next?** After the setup wizard completes, navigate to your new application directory with `cd [template-name]` and start your application with `dr start` or `dr run dev`.

### Run tasks

Now that you've cloned and configured a template, you can start running tasks defined in the template Taskfile.

**Quick start (recommended):**

Use the `start` command for automated initialization:

```bash
dr start
```

This command will:

- Check prerequisites and validate your environment.
- Execute a template-specific quickstart script if available.
- Fall back to the setup wizard if no script exists.

> [!TIP]
> You can use the `--yes` flag to skip all prompts and execute immediately. This is useful in scripts or CI/CD pipelines.

**Running specific tasks:**

For more control, execute individual tasks with the `run` command:

```bash
# List available tasks
dr task list

# Run the development server
dr run dev

# Or execute specific tasks
dr run build
dr run test
```

> [!TIP]
> **What's next?** Your application is now running! Explore the [Template system](docs/template-system/) documentation, set up [shell completions](docs/user-guide/shell-completions.md), or review the [Command reference](docs/commands/) for detailed command documentation.

## Next steps

From here, refer to the repository of the template you selected to start customizing it.
Refer to the [Docs](/docs/) section of this repository for more details on using the DataRobot CLI.
See the links below for specific details:

- **[User guide](docs/user-guide/README.md)**&mdash;complete usage guide covering installation, authentication, working with templates, configuration management, and shell completions.
- **[Template system](docs/template-system/)**&mdash;deep dive into how templates work, the interactive configuration wizard, and environment variable management.
- **[Command reference](docs/commands/)**&mdash;detailed documentation for all CLI commands and subcommands, including flags, options, and usage examples.
- **[Auth command](docs/commands/auth.md)**&mdash;detailed authentication management guide.
- **[Development guide](docs/development/)**&mdash;for contributors: building from source, development setup, project structure, and release process.

## Common issues

### "dr: command not found"

**Why it happens:** The CLI binary isn't in your system's PATH, so your shell can't find it.

**How to fix:**

```bash
# Check if dr is in PATH
which dr

# If not found, verify the binary location
ls -l /usr/local/bin/dr

# Add it to your PATH (for current session)
export PATH="/usr/local/bin:$PATH"

# For permanent fix, add to your shell config file:
# Bash: echo 'export PATH="/usr/local/bin:$PATH"' >> ~/.bashrc
# Zsh:  echo 'export PATH="/usr/local/bin:$PATH"' >> ~/.zshrc
```

**How to prevent:** Re-run the installation script or ensure the binary is installed to a directory in your PATH.

### "Failed to read config file"

**Why it happens:** The configuration file doesn't exist yet or is in an unexpected location. This typically occurs on first use before authentication.

**How to fix:**

```bash
# Set your DataRobot URL (creates config file if missing)
dr auth set-url https://app.datarobot.com

# Authenticate (saves credentials to config file)
dr auth login
```

**How to prevent:** Run `dr auth set-url` and `dr auth login` as part of your initial setup. The config file is automatically created at `~/.config/datarobot/drconfig.yaml`.

### "Authentication failed"

**Why it happens:** Your API token may have expired, been revoked, or the DataRobot URL may have changed. This can also occur if the config file is corrupted.

**How to fix:**

```bash
# Clear existing credentials
dr auth logout

# Re-authenticate
dr auth login

# If issues persist, verify your DataRobot URL
dr auth set-url https://app.datarobot.com  # or your instance URL
dr auth login
```

**How to prevent:** Regularly update the CLI (`dr self update`) and re-authenticate if you change DataRobot instances or if your organization rotates API keys.

## Getting help

For additional help:

```bash
# General help
dr --help

# Command-specific help
dr auth --help
dr templates --help
dr run --help

# Enable verbose output for debugging
dr --verbose templates list

# Enable debug output for detailed information
dr --debug templates list
```

## Contributing

We welcome contributions! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for details on:

- Code of conduct.
- Development workflow.
- Submitting pull requests.
- Coding standards.
- Testing requirements.

## Support

- 📖 [Documentation](docs/)
- 🐛 [Issue Tracker](https://github.com/datarobot/cli/issues)
- 💬 [Discussions](https://github.com/datarobot/cli/discussions)
- 📧 Email: <oss-community-management@datarobot.com>

## Acknowledgments

Built with:

- [Cobra](https://github.com/spf13/cobra)&mdash;CLI framework.
- [Bubble Tea](https://github.com/charmbracelet/bubbletea)&mdash;terminal UI framework.
- [Viper](https://github.com/spf13/viper)&mdash;configuration management.
- [Task](https://taskfile.dev/)&mdash;task runner.
