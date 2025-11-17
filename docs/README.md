# DataRobot CLI Documentation

Welcome to the DataRobot CLI documentation. This directory contains comprehensive guides and references for using and developing the DataRobot CLI tool.

## Table of contents

- [Quick install](#quick-install)
- [Documentation structure](#documentation-structure)
  - [User guide](#-user-guide)
  - [Template system](#-template-system)
  - [Command reference](#-command-reference)
  - [Development guide](#-development-guide)
- [Quick links](#quick-links)
- [Getting help](#getting-help)
- [Contributing to documentation](#contributing-to-documentation)

## Quick install

Install the latest version with a single command that auto-detects your operating system:

**macOS/Linux:**

```bash
curl https://cli.datarobot.com/install | sh
```

**Windows (PowerShell):**

```powershell
irm https://cli.datarobot.com/winstall | iex
```

For more installation options, see [Getting Started](user-guide/getting-started.md).

## Documentation structure

### 📚 User guide

End-user documentation for using the CLI:

- [Getting started](user-guide/getting-started.md)&mdash;installation and initial setup guide covering prerequisites, installation methods, authentication, and your first template.
- [Shell completions](user-guide/shell-completions.md)&mdash;set up command auto-completion for Bash, Zsh, Fish, and PowerShell.
- [Configuration files](user-guide/configuration.md)&mdash;understanding config file structure, location, and how to manage multiple environments.

### 🎯 Template system

Understanding the interactive template configuration:

- [Template structure](template-system/structure.md)&mdash;how templates are organized, including repository layout, metadata files, and multi-component templates.
- [Interactive configuration](template-system/interactive-config.md)&mdash;the wizard system explained, including prompt types, conditional logic, and state management.
- [Environment variables](template-system/environment-variables.md)&mdash;managing .env files, variable types, security best practices, and advanced features.

### 📖 Command reference

Detailed documentation for each command:

- [auth](commands/auth.md)&mdash;authentication management including login, logout, and URL configuration.
- [start](commands/start.md)&mdash;quickstart process for automated template initialization.
- [run](commands/run.md)&mdash;task execution with automatic Taskfile discovery and parallel execution support.
- [task](commands/task.md)&mdash;Taskfile composition and management, including task listing and execution.
- [dotenv](commands/dotenv.md)&mdash;environment variable management with interactive wizard and validation.
- [self](commands/self.md)&mdash;CLI utility commands including version information, completion setup, and self-update.

For template operations (list, clone, setup), see the [Template system](template-system/) documentation and use `dr templates --help` for command details.

### 🔧 Development guide

For contributors and developers:

- [Development setup](development/setup.md)&mdash;setting up your development environment with required tools and dependencies.
- [Building from source](development/building.md)&mdash;compile and build the CLI, including build options and cross-platform builds.
- [Project structure](development/structure.md)&mdash;code organization and design, including directory structure and component overview.
- [Authentication](development/authentication.md)&mdash;authentication implementation details and OAuth flow.
- [Release process](development/releasing.md)&mdash;how releases are created, versioning, and release workflow.

## Quick links

- [Main README](../README.md)&mdash;project overview.
- [Contributing guidelines](../CONTRIBUTING.md)&mdash;how to contribute.
- [Code of conduct](../CODE_OF_CONDUCT.md)&mdash;community guidelines.
- [Changelog](../CHANGELOG.md)&mdash;version history.

## Getting help

If you can't find what you're looking for:

1. Search [existing issues](https://github.com/datarobot-oss/cli/issues).
2. Open a [new issue](https://github.com/datarobot-oss/cli/issues/new).
3. Email: [oss-community-management@datarobot.com](mailto:oss-community-management@datarobot.com).

## Contributing to documentation

Found an error or want to improve the docs? Please see our [Contributing Guidelines](../CONTRIBUTING.md) for information on submitting documentation improvements.
