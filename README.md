# DataRobot CLI

[![Go Report Card](https://goreportcard.com/badge/github.com/datarobot/cli)](https://goreportcard.com/report/github.com/datarobot/cli)
[![License](https://img.shields.io/badge/License-Proprietary-blue.svg)](LICENSE.txt)

The DataRobot CLI (`dr`) is a command-line interface for managing DataRobot custom applications. It provides an interactive experience for cloning, configuring, and deploying DataRobot application templates with built-in authentication, environment configuration, and task execution capabilities.

## Features

- 🔐 **Authentication Management** - Seamless OAuth integration with DataRobot
- 📦 **Template Management** - Clone and configure application templates interactively
- ⚙️ **Interactive Configuration** - Smart wizard for environment setup with validation
- 🚀 **Task Runner** - Execute application tasks with built-in Taskfile integration
- 🐚 **Shell Completions** - Support for Bash, Zsh, Fish, and PowerShell
- 🎨 **Beautiful TUI** - Terminal UI built with Bubble Tea for an enhanced user experience

## Table of Contents

- [Installation](#installation)
- [Quick Start](#quick-start)
- [Documentation](#documentation)
- [Commands](#commands)
- [Shell Completion](#shell-completion)
- [Development](#development)
- [Release](#release)
- [Contributing](#contributing)
- [License](#license)

## Installation

### Prerequisites

- Go 1.24.7 or later (for building from source)
- Git
- [Task](https://taskfile.dev/) (for development and task running)

### From Source

```bash
# Clone the repository
git clone https://github.com/datarobot/cli.git
cd cli

# Build the CLI
task build

# The binary will be available at ./dist/dr
./dist/dr version
```

### Binary Installation

Download the latest release for your platform from the [releases page](https://github.com/datarobot/cli/releases).

```bash
# macOS (example)
curl -LO https://github.com/datarobot/cli/releases/latest/download/dr-darwin-amd64
chmod +x dr-darwin-amd64
sudo mv dr-darwin-amd64 /usr/local/bin/dr

# Verify installation
dr version
```

## Quick Start

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

This will guide you through:
- Selecting a template from available options
- Cloning the template repository
- Configuring environment variables interactively
- Setting up application-specific settings

### 3. Run tasks

Execute tasks defined in your template's Taskfile:

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

- **[User Guide](docs/user-guide/)** - Complete usage guide for all features
  - [Getting Started](docs/user-guide/getting-started.md)
  - [Authentication](docs/user-guide/authentication.md)
  - [Working with Templates](docs/user-guide/templates.md)
  - [Shell Completions](docs/user-guide/shell-completions.md)
  - [Configuration Files](docs/user-guide/configuration.md)
  
- **[Template System](docs/template-system/)** - Understanding the template configuration system
  - [Template Structure](docs/template-system/structure.md)
  - [Interactive Configuration](docs/template-system/interactive-config.md)
  - [Environment Variables](docs/template-system/environment-variables.md)
  
- **[Command Reference](docs/commands/)** - Detailed command documentation
  - [auth](docs/commands/auth.md) - Authentication commands
  - [templates](docs/commands/templates.md) - Template management
  - [run](docs/commands/run.md) - Task execution
  - [dotenv](docs/commands/dotenv.md) - Environment file management
  - [completion](docs/commands/completion.md) - Shell completion setup
  
- **[Development Guide](docs/development/)** - For contributors
  - [Building from Source](docs/development/building.md)
  - [Architecture](docs/development/architecture.md)
  - [Testing](docs/development/testing.md)
  - [Release Process](docs/development/release.md)

## Commands

### Main Commands

| Command | Description |
|---------|-------------|
| `dr auth` | Authentication management (login, logout, set-url) |
| `dr templates` | Template operations (list, clone, setup, status) |
| `dr run` | Execute application tasks |
| `dr dotenv` | Manage environment variables interactively |
| `dr completion` | Generate shell completion scripts |
| `dr version` | Show version information |

### Examples

```bash
# Authentication
dr auth login
dr auth logout
dr auth set-url https://app.datarobot.com

# Template Management
dr templates list                    # List available templates
dr templates clone <template-name>   # Clone a specific template
dr templates setup                   # Interactive template setup wizard
dr templates status                  # Show current template status

# Environment Configuration
dr dotenv                           # Interactive environment editor
dr dotenv --wizard                  # Configuration wizard mode

# Task Execution
dr run --list                       # List available tasks
dr run dev                          # Run development server
dr run build deploy --parallel      # Run multiple tasks in parallel
dr run test --watch                 # Run tests in watch mode

# Get Help
dr --help
dr templates --help
dr run --help
```

## Shell Completion

The CLI supports shell completions for Bash, Zsh, Fish, and PowerShell. See [Shell Completion Guide](docs/user-guide/shell-completions.md) for detailed setup instructions.

### Quick Setup

**Bash (Linux)**
```bash
dr completion bash | sudo tee /etc/bash_completion.d/dr
```

**Bash (macOS)**
```bash
dr completion bash > /usr/local/etc/bash_completion.d/dr
```

**Zsh**
```bash
dr completion zsh > "${fpath[1]}/_dr"
```

**Fish**
```bash
dr completion fish > ~/.config/fish/completions/dr.fish
```

**PowerShell**
```powershell
dr completion powershell | Out-String | Invoke-Expression
```

## Development

### Setting Up Development Environment

1. **Install Prerequisites**

```bash
# Install Task (task runner)
# macOS
brew install go-task/tap/go-task

# Linux
sh -c "$(curl --location https://taskfile.dev/install.sh)" -- -d -b /usr/local/bin

# Windows
choco install go-task
```

2. **Clone and Build**

```bash
git clone https://github.com/datarobot/cli.git
cd cli

# Install development tools
task install-tools

# Build the CLI
task build

# Run tests
task test

# Run linters
task lint
```

3. **Available Development Tasks**

```bash
task --list           # Show all available tasks
task test             # Run tests
task test-coverage    # Run tests with coverage
task lint             # Run linters
task fmt              # Format code
task build            # Build binary
task clean            # Clean build artifacts
```

### Project Structure

```
.
├── cmd/                    # Command implementations
│   ├── auth/              # Authentication commands
│   ├── completion/        # Shell completion
│   ├── dotenv/            # Environment management
│   ├── run/               # Task runner
│   ├── templates/         # Template commands
│   └── version/           # Version command
├── internal/              # Private application code
│   ├── config/           # Configuration management
│   ├── drapi/            # DataRobot API client
│   ├── envbuilder/       # Environment builder
│   ├── task/             # Task discovery and execution
│   └── version/          # Version information
├── tui/                   # Terminal UI components
├── docs/                  # Documentation
└── main.go               # Application entry point
```

## Release

### Creating a Release

This project uses [goreleaser](https://goreleaser.com/) for automated releases.

1. **Ensure all changes are merged** to the main branch

2. **Determine the next version** following [Semantic Versioning](https://semver.org/):
   - `MAJOR.MINOR.PATCH` (e.g., `v1.2.3`)
   - Pre-release: `v1.2.3-rc.1`, `v1.2.3-beta.1`

3. **Create and push a tag**:

```bash
# Create a new version tag
git tag v0.1.0

# Push the tag
git push --tags
```

4. **Automated Release**: The GitHub Actions workflow will automatically:
   - Build binaries for multiple platforms
   - Generate release notes
   - Create a GitHub release
   - Upload artifacts

### Release Testing

To test the release process without publishing:

```bash
# Dry run
goreleaser release --snapshot --clean
```

## Contributing

We welcome contributions! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for details on:

- Code of Conduct
- Development workflow
- Submitting pull requests
- Coding standards
- Testing requirements

### Quick Contribution Guide

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Make your changes
4. Run tests and linters (`task test && task lint`)
5. Commit your changes (`git commit -m 'Add amazing feature'`)
6. Push to the branch (`git push origin feature/amazing-feature`)
7. Open a Pull Request

## License

Copyright 2025 DataRobot, Inc. and its affiliates. All rights reserved.

This is proprietary source code of DataRobot, Inc. See [LICENSE.txt](LICENSE.txt) for details.

## Support

- 📖 [Documentation](docs/)
- 🐛 [Issue Tracker](https://github.com/datarobot/cli/issues)
- 💬 [Discussions](https://github.com/datarobot/cli/discussions)
- 📧 Email: oss-community-management@datarobot.com

## Acknowledgments

Built with:
- [Cobra](https://github.com/spf13/cobra) - CLI framework
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - Terminal UI framework
- [Viper](https://github.com/spf13/viper) - Configuration management
- [Task](https://taskfile.dev/) - Task runner
