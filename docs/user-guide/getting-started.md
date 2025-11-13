# Getting Started with DataRobot CLI

This guide will help you install and start using the DataRobot CLI (`dr`) for managing custom applications.

## Prerequisites

Before you begin, ensure you have:

- **DataRobot account**&mdash;access to a DataRobot instance (cloud or self-managed).
- **Git**&mdash;for cloning templates (version 2.0+).
- **Terminal**&mdash;command-line interface access.

## Installation

### Option 1: Download binary (recommended)

Download the latest release for your operating system:

#### macOS

```bash
# Intel Macs
curl -LO https://github.com/datarobot/cli/releases/latest/download/dr-darwin-amd64
chmod +x dr-darwin-amd64
sudo mv dr-darwin-amd64 /usr/local/bin/dr

# Apple Silicon (M1/M2)
curl -LO https://github.com/datarobot/cli/releases/latest/download/dr-darwin-arm64
chmod +x dr-darwin-arm64
sudo mv dr-darwin-arm64 /usr/local/bin/dr
```

#### Linux

```bash
# x86_64
curl -LO https://github.com/datarobot/cli/releases/latest/download/dr-linux-amd64
chmod +x dr-linux-amd64
sudo mv dr-linux-amd64 /usr/local/bin/dr

# ARM64
curl -LO https://github.com/datarobot/cli/releases/latest/download/dr-linux-arm64
chmod +x dr-linux-arm64
sudo mv dr-linux-arm64 /usr/local/bin/dr
```

#### Windows

Download `dr-windows-amd64.exe` from the [releases page](https://github.com/datarobot/cli/releases/latest) and add it to your PATH.

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

```bash
dr version
```

You should see output similar to:

```
DataRobot CLI version 0.1.0 (commit: abc1234, built date: 2025-10-23, runtime: go1.25.3)
```

## Initial Setup

### 1. Configure DataRobot URL

Set your DataRobot instance URL:

```bash
dr auth set-url
```

You'll be prompted to enter your DataRobot URL. You can use shortcuts for cloud instances:

- Enter `1` for `https://app.datarobot.com`
- Enter `2` for `https://app.eu.datarobot.com`
- Enter `3` for `https://app.jp.datarobot.com`
- Or enter your custom URL (e.g., `https://your-instance.datarobot.com`)

Alternatively, set the URL directly:

```bash
dr auth set-url https://app.datarobot.com
```

### 2. Authenticate

Log in to DataRobot using OAuth:

```bash
dr auth login
```

This will:
1. Open your default web browser.
2. Redirect you to the DataRobot login page.
3. Request authorization.
4. Automatically save your credentials.

Your API key will be securely stored in `~/.datarobot/config.yaml`.

### 3. Verify authentication

Check that you're logged in:

```bash
dr templates list
```

This should display a list of available templates from your DataRobot instance.

## Your first template

Now that you're set up, let's create your first application from a template.

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
dr dotenv
```

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
dr run --list

# Run the development server
dr run dev

# Or execute specific tasks
dr run build
dr run test
```

## Next steps

- **[Authentication guide](authentication.md)**&mdash;learn about authentication options.
- **[Working with templates](templates.md)**&mdash;detailed template management.
- **[Shell completions](shell-completions.md)**&mdash;set up command auto-completion.
- **[Command reference](../commands/)**&mdash;complete command documentation.

## Common issues

### "dr: command not found"

Ensure the binary is in your PATH:

```bash
# Check if dr is in PATH
which dr

# If not found, verify the binary location
ls -l /usr/local/bin/dr

# You may need to add it to your PATH in ~/.bashrc or ~/.zshrc
export PATH="/usr/local/bin:$PATH"
```

### "Failed to read config file"

The config file might be missing. Run:

```bash
dr auth set-url https://app.datarobot.com
dr auth login
```

### "Authentication failed"

Your credentials may have expired. Log in again:

```bash
dr auth logout
dr auth login
```

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

- **Linux/macOS**&mdash;`~/.datarobot/config.yaml`.
- **Windows**&mdash;`%USERPROFILE%\.datarobot\config.yaml`.

See [Configuration Files](configuration.md) for more details.
