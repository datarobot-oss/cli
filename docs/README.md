# DataRobot CLI Documentation

Welcome to the DataRobot CLI documentation. This directory contains comprehensive guides and references for using and developing the DataRobot CLI tool.

## Quick Install

Install the latest version with a single command that auto-detects your operating system:

**macOS/Linux:**
######

    curl https://datarobot-oss.github.io/cli/install | sh

**Windows (PowerShell):**
######
    irm https://datarobot-oss.github.io/cli/winstall | iex

For more installation options, see [Getting Started](user-guide/getting-started.md).

## Documentation structure

### 📚 User Guide

End-user documentation for using the CLI:
- [Getting started](user-guide/getting-started.md)&mdash;installation and initial setup.
- [Authentication](user-guide/authentication.md)&mdash;setting up DataRobot credentials.
- [Working with templates](user-guide/templates.md)&mdash;clone and manage application templates.
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
- [templates](commands/templates.md)&mdash;template operations.
- [run](commands/run.md)&mdash;task execution.
- [dotenv](commands/dotenv.md)&mdash;environment variable management.
- [self](commands/self.md)&mdash;CLI utility commands (version, completion).

### 🔧 Development Guide

For contributors and developers:

- [Building from source](development/building.md)&mdash;compile and build the CLI.
- [Architecture](development/architecture.md)&mdash;project structure and design.
- [Testing](development/testing.md)&mdash;running and writing tests.
- [Release process](development/release.md)&mdash;how releases are created.

## Quick links

- [Main README](../README.md)&mdash;project overview.
- [Contributing guidelines](../CONTRIBUTING.md)&mdash;how to contribute.
- [Code of conduct](../CODE_OF_CONDUCT.md)&mdash;community guidelines.
- [Changelog](../CHANGELOG.md)&mdash;version history.

## Getting help

If you can't find what you're looking for:

1. Check the [FAQ](user-guide/faq.md).
2. Search [existing issues](https://github.com/datarobot/cli/issues).
3. Open a [new issue](https://github.com/datarobot/cli/issues/new).
4. Email: oss-community-management@datarobot.com.

## Contributing to documentation

Found an error or want to improve the docs? Please see our [Contributing Guidelines](../CONTRIBUTING.md) for information on submitting documentation improvements.

All documentation is written in Markdown and follows the [Markdown Style Guide](development/markdown-style-guide.md).
