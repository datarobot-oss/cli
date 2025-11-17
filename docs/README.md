# DataRobot CLI Documentation

Welcome to the DataRobot CLI documentation. This directory contains comprehensive guides and references for using and developing the DataRobot CLI tool.

## Quick Install

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

### 📚 User Guide

End-user documentation for using the CLI:

- [Getting started](user-guide/getting-started.md)&mdash;installation and initial setup.
- [Shell completions](user-guide/shell-completions.md)&mdash;set up command auto-completion.
- [Configuration files](user-guide/configuration.md)&mdash;understanding config file structure.

### 🎯 Template System

Understanding the interactive template configuration:

- [Template structure](template-system/structure.md)&mdash;how templates are organized.
- [Interactive configuration](template-system/interactive-config.md)&mdash;the wizard system explained.
- [Environment variables](template-system/environment-variables.md)&mdash;managing .env files.

### 📖 Command Reference

Detailed documentation for each command:

- [auth](commands/auth.md)&mdash;authentication management.
- [start](commands/start.md)&mdash;quickstart process.
- [run](commands/run.md)&mdash;task execution.
- [task](commands/task.md)&mdash;Taskfile composition and management.
- [dotenv](commands/dotenv.md)&mdash;environment variable management.
- [self](commands/self.md)&mdash;CLI utility commands (version, completion).

For template operations, see the [Template System](template-system/) documentation and use `dr templates --help` for command details.

### 🔧 Development Guide

For contributors and developers:

- [Development setup](development/setup.md)&mdash;setting up your development environment.
- [Building from source](development/building.md)&mdash;compile and build the CLI.
- [Project structure](development/structure.md)&mdash;code organization and design.
- [Authentication](development/authentication.md)&mdash;authentication implementation details.
- [Release process](development/releasing.md)&mdash;how releases are created.

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
