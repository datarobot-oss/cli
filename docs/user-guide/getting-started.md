# Getting Started with DataRobot CLI

This guide will help you install and start using the DataRobot CLI (`dr`) for managing custom applications.

## Table of contents

- [Prerequisites](#prerequisites)
- [Installation](#installation)
- [Updating the CLI](#updating-the-cli)
- [Initial setup](#initial-setup)
- [Your first template](#your-first-template)
- [Running your application](#running-your-application)
- [Next steps](#next-steps)
- [Common issues](#common-issues)
- [Getting help](#getting-help)
- [Configuration location](#configuration-location)

## Prerequisites

Before you begin, ensure you have:

- **DataRobot account**&mdash;access to a DataRobot instance (cloud or self-managed).
  - If you don't have an account, sign up at [DataRobot](https://www.datarobot.com/) or contact your organization's DataRobot administrator.
  - You'll need your DataRobot instance URL (e.g., `https://app.datarobot.com`). See [DataRobot's API keys and tools page](https://docs.datarobot.com/en/docs/platform/acct-settings/api-key-mgmt.html) for help locating your endpoint.
- **Git**&mdash;for cloning templates (version 2.0+).
  - Install Git from [git-scm.com](https://git-scm.com/downloads) if not already installed.
  - Verify installation: `git --version`
- **Terminal**&mdash;command-line interface access.
  - **macOS/Linux:** Use Terminal, iTerm2, or your preferred terminal emulator.
  - **Windows:** Use PowerShell, Command Prompt, or Windows Terminal.

## Installation

Install the latest version with a single command that auto-detects your operating system:

**macOS/Linux:**

```bash
curl https://cli.datarobot.com/install | sh
```

**Windows (PowerShell):**

```powershell
irm https://cli.datarobot.com/winstall | iex
```

For alternative installation methods, see the next sections.

### Option 1: Download binary (recommended)

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

### Option 2: Build from source

If you have Go 1.25.3 or later installed:

```bash
# Clone the repository
git clone https://github.com/datarobot-oss/cli.git
cd cli

# Install Task (if not already installed)
go install github.com/go-task/task/v3/cmd/task@latest

# Build
task build

# The binary will be at ./dist/dr
sudo mv ./dist/dr /usr/local/bin/dr
```

### Verify installation

You can verify the installation by checking the version:

```bash
dr --version
```

Or use the version command:

```bash
dr self version
```

You should see output similar to:

```text
DataRobot CLI (version v0.2.9)%  
```

## Updating the CLI

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

## Initial setup

### Configure your DataRobot instance URL

Set your DataRobot instance URL:

```bash
dr auth set-url
```

You'll be prompted to enter your DataRobot URL. You can use shortcuts for cloud instances:

- Enter `1` for `https://app.datarobot.com`
- Enter `2` for `https://app.eu.datarobot.com`
- Enter `3` for `https://app.jp.datarobot.com`
- Enter `4` for a custom URL

Alternatively, set the URL directly:

```bash
dr auth set-url https://app.datarobot.com
```

### Authenticate with DataRobot

Log in to DataRobot using OAuth:

```bash
dr auth login
```

This will:

1. Open your default web browser.
2. Redirect you to the DataRobot login page.
3. Request authorization.
4. Automatically save your credentials.

Your API key will be securely stored in `~/.config/datarobot/drconfig.yaml`.

### Verify authentication

Check that you're logged in:

```bash
dr templates list
```

This command displays a list of available templates from your DataRobot instance.

> **What's next?** Now that you're authenticated, you can:
>
> - Browse available templates: `dr templates list`
> - Start the setup wizard: `dr templates setup`
> - See the [Command reference](../commands/) for all available commands

## Your first template

Now that you're set up, let's create your first application from a template.

> **Note:** A **template** is a pre-configured application scaffold that you can customize. When you clone and configure a template, it becomes your **application**&mdash;a customized instance ready to run and deploy.

### Using the setup wizard (recommended)

The easiest way to get started:

```bash
dr templates setup
```

This interactive wizard will:

1. Display available templates.
2. Help you select and clone a template.
3. Guide you through environment configuration.
4. Set up all required variables.

Follow the on-screen prompts to complete the setup.

> **What's next?** After the setup wizard completes:
>
> - Navigate to your new application directory: `cd [template-name]`
> - Start your application: `dr start` or `dr run dev`
> - See [Running your application](#running-your-application) below for more options

### Manual setup

If you prefer manual control:

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

> **What's next?** After configuring your template:
>
> - Start your application: `dr start` or `dr run dev`
> - Explore available tasks: `dr task list`
> - See [Running your application](#running-your-application) below

## Running your application

Once your template is set up, you have several options to run it:

### Quick start (recommended)

Use the `start` command for automated initialization:

```bash
dr start
```

This command will:

- Check prerequisites and validate your environment.
- Execute a template-specific quickstart script if available.
- Fall back to the setup wizard if no script exists.

For non-interactive mode (useful in scripts or CI/CD):

```bash
dr start --yes
```

### Running specific tasks

For more control, execute individual tasks:

```bash
# List available tasks
dr task list

# Run the development server
dr run dev

# Or execute specific tasks
dr run build
dr run test
```

> **What's next?** Your application is now running! You can:
>
> - Explore the [Template system](../template-system/) documentation to understand how templates work
> - Set up [shell completions](shell-completions.md) for faster command entry
> - Review the [Command reference](../commands/) for detailed command documentation
> - Learn about [configuration files](configuration.md) for advanced setup

## Next steps

Continue learning about the DataRobot CLI:

- **[Shell completions](shell-completions.md)**&mdash;set up command auto-completion for faster workflow.
- **[Configuration files](configuration.md)**&mdash;understand how configuration files work and manage multiple environments.
- **[Template system](../template-system/)**&mdash;learn how templates are structured and how the interactive configuration works.
- **[Command reference](../commands/)**&mdash;complete documentation for all CLI commands and options.
- **[Auth command](../commands/auth.md)**&mdash;detailed authentication management guide.

## Common issues

### "dr: command not found"

**Why it happens:** The CLI binary isn't in your system's PATH, so your shell can't find it.

**How to fix:**

```bash
# Check if dr is in PATH
which dr

# If not found, verify the binary location
ls -l /usr/local/bin/dr

# You may need to add it to your PATH in ~/.bashrc or ~/.zshrc
export PATH="/usr/local/bin:$PATH"

# For the current session only:
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
dr --debug templates list
```

## Configuration location

Configuration files are stored in:

- **Linux/macOS**&mdash;`~/.config/datarobot/drconfig.yaml`.
- **Windows**&mdash;`%USERPROFILE%\.config\datarobot\drconfig.yaml`.

See [Configuration Files](configuration.md) for more details.
