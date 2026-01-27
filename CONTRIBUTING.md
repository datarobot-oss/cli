# Contributing Guidelines

Thank you for your interest in contributing to the DataRobot CLI! This document provides guidelines for contributing to this project.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Workflow](#development-workflow)
- [Opening Issues](#opening-issues)
- [Submitting Pull Requests](#submitting-pull-requests)
- [Project Maintainers](#project-maintainers)

## Code of Conduct

This project follows the DataRobot Code of Conduct. Please be respectful and constructive in all interactions.

## Getting Started

### Development Setup

To start contributing, you'll need to set up your development environment:

1. **Read the [Development Setup Guide](docs/development/setup.md)** for detailed instructions on:
   - Installing prerequisites (Go, Task, etc.)
   - Building the CLI from source
   - Running tests and linters

2. **Understand the [Project Structure](docs/development/structure.md)** to familiarize yourself with:
   - Code organization
   - Key directories and their purposes
   - Coding patterns used in the project

3. **Review the [Building Guide](docs/development/building.md)** for:
   - Detailed build information
   - Architecture overview
   - Coding standards and quality tools

### Documentation Preview

To preview the documentation site locally:

```bash
cd docs
uv sync
uv run mkdocs serve
```

Then open `http://localhost:8000` in your browser. The preview will auto-reload when you edit markdown files.

## Development Workflow

### Quick Start

```bash
# Clone the repository
git clone https://github.com/datarobot-oss/cli.git
cd cli

# Setup development environment
task dev-init

# Build the CLI
task build

# Run tests
task test

# Run linters
task lint
```

### Making Changes

1. **Create a feature branch**

   ```bash
   git checkout -b feature/your-feature-name
   ```

2. **Make your changes**
   - Follow the coding standards in [building.md](docs/development/building.md)
   - Write tests for new functionality
   - Update documentation as needed

3. **Test your changes**

   ```bash
   task test
   task lint
   ```

4. **Commit your changes**

   ```bash
   git commit -m "Brief description of your changes"
   ```

   Use clear, descriptive commit messages. Consider using [Conventional Commits](https://www.conventionalcommits.org/) format.

5. **Push to your fork**

   ```bash
   git push origin feature/your-feature-name
   ```

## Opening Issues

### Before Opening an Issue

- Check if there are any existing issues or pull requests that match your case
- Review any FAQ documentation if available
- Search closed issues as your question may have been answered before

### Creating an Issue

When opening an issue:

1. **Use the appropriate label**: bug, enhancement, feature request, documentation, etc.
2. **Be specific and detailed**:
   - For bugs: include steps to reproduce, expected behavior, and actual behavior
   - For features: describe the use case and proposed solution
   - Include relevant code snippets, error messages, or screenshots

### Security Vulnerabilities

**Do not open a GitHub issue for security vulnerabilities.** Instead:

1. Email the maintainers directly (see [Project Maintainers](#project-maintainers))
2. If maintainers don't respond within seven days, email <oss-community-management@datarobot.com>

## Submitting Pull Requests

### Pull Request Process

1. **Ensure your PR**:
   - Has a clear title and description
   - References any related issues (e.g., "Fixes #123")
   - Passes all tests (`task test`)
   - Passes all linters (`task lint`)
   - Includes documentation updates if needed

2. **PR Description should include**:
   - What changes were made and why
   - How to test the changes
   - Any breaking changes or migration notes
   - Screenshots for UI changes (if applicable)

3. **Review Process**:
   - Maintainers will review your PR
   - Address any feedback or requested changes
   - Once approved, a maintainer will merge your PR

### Quality Standards

All code must:

- Pass `golangci-lint` with zero errors
- Follow Go formatting standards (`go fmt`, `go vet`)
- Include appropriate tests
- Maintain or improve code coverage
- Follow the whitespace rules (wsl) defined in the project

See [building.md](docs/development/building.md) for detailed coding standards.

## Release Process

For information on creating releases, see the [Release Process Guide](docs/development/releasing.md).

## Project Maintainers

- AJ Alon <aj.alon@datarobot.com>
- Carson Gee <carson.gee@datarobot.com>
- Yuriy Hrytsyuk <yuriy.hrytsyuk@datarobot.com>

## Getting Help

If you don't get a response within seven days of creating your issue or pull request, please send us an email at <oss-community-management@datarobot.com>.

## Additional Resources

- [Development Setup](docs/development/setup.md)
- [Project Structure](docs/development/structure.md)
- [Building Guide](docs/development/building.md)
- [Release Process](docs/development/releasing.md)
- [User Documentation](docs/)

Thank you for contributing to the DataRobot CLI!
