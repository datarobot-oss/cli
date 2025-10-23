# Contributing to DataRobot CLI

Thank you for your interest in contributing to the DataRobot CLI! This document provides guidelines and instructions for contributing to this project.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Setup](#development-setup)
- [Making Changes](#making-changes)
- [Testing](#testing)
- [Code Style](#code-style)
- [Submitting Changes](#submitting-changes)
- [Reporting Bugs](#reporting-bugs)
- [Requesting Features](#requesting-features)
- [Project Maintainers](#project-maintainers)

## Code of Conduct

Please read and follow our [Code of Conduct](CODE_OF_CONDUCT.md) to maintain a welcoming and inclusive community.

## Getting Started

### Prerequisites

Before contributing, ensure you have:

- **Go 1.24.7 or later** - [Installation guide](https://golang.org/doc/install)
- **Git** - Version control
- **Task** - Task runner ([installation](https://taskfile.dev/installation/))
- **Make** (optional) - For some build tasks

### Development Setup

1. **Fork the repository**

   Visit https://github.com/datarobot/cli and click "Fork"

2. **Clone your fork**

   ```bash
   git clone https://github.com/YOUR_USERNAME/cli.git
   cd cli
   ```

3. **Add upstream remote**

   ```bash
   git remote add upstream https://github.com/datarobot/cli.git
   ```

4. **Install development tools**

   ```bash
   task install-tools
   ```

   This installs:
   - `golangci-lint` - Linter
   - `gofumpt` - Formatter
   - `goreleaser` - Release tool

5. **Verify setup**

   ```bash
   # Build the CLI
   task build
   
   # Run tests
   task test
   
   # Run linters
   task lint
   ```

## Making Changes

### Branching Strategy

1. **Keep your fork up to date**

   ```bash
   git fetch upstream
   git checkout main
   git merge upstream/main
   ```

2. **Create a feature branch**

   ```bash
   git checkout -b feature/your-feature-name
   # or
   git checkout -b fix/your-bug-fix
   ```

   Branch naming conventions:
   - `feature/` - New features
   - `fix/` - Bug fixes
   - `docs/` - Documentation changes
   - `refactor/` - Code refactoring
   - `test/` - Test additions/changes

### Development Workflow

1. **Make your changes**

   Edit the relevant files in your feature branch.

2. **Format code**

   ```bash
   task fmt
   ```

3. **Run linters**

   ```bash
   task lint
   ```

4. **Run tests**

   ```bash
   # Run all tests
   task test
   
   # Run tests with coverage
   task test-coverage
   
   # Run specific tests
   go test ./cmd/auth/...
   ```

5. **Build and test locally**

   ```bash
   task build
   ./dist/dr --help
   ```

## Testing

### Running Tests

```bash
# All tests
task test

# With coverage
task test-coverage

# Specific package
go test ./internal/config

# Verbose output
go test -v ./...

# Run specific test
go test -run TestFunctionName ./cmd/auth
```

### Writing Tests

Follow Go testing conventions:

```go
// cmd/auth/login_test.go
package auth

import (
    "testing"
    "github.com/stretchr/testify/assert"
)

func TestLogin(t *testing.T) {
    // Arrange
    expected := "success"
    
    // Act
    result := performLogin()
    
    // Assert
    assert.Equal(t, expected, result)
}
```

### Test Coverage

- Aim for **>80%** test coverage for new code
- All exported functions should have tests
- Test both success and error cases

```bash
# View coverage report
task test-coverage

# Generate HTML report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## Code Style

### Go Conventions

Follow [Effective Go](https://golang.org/doc/effective_go.html) and [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments).

Key points:

1. **Formatting**: Use `gofumpt` (more strict than `gofmt`)
   ```bash
   task fmt
   ```

2. **Naming**:
   - Use `camelCase` for private variables/functions
   - Use `PascalCase` for exported variables/functions
   - Use descriptive names (avoid abbreviations)

3. **Comments**:
   - Document all exported functions, types, and constants
   - Use complete sentences
   - Start with the name being documented

   ```go
   // Execute runs the root command and returns an error if it fails.
   // This is called by main.main() and only needs to be called once.
   func Execute() error {
       return RootCmd.Execute()
   }
   ```

4. **Error Handling**:
   - Always check errors
   - Wrap errors with context
   - Use `fmt.Errorf` with `%w` for wrapping

   ```go
   if err != nil {
       return fmt.Errorf("failed to read config: %w", err)
   }
   ```

### Linting

All code must pass linting:

```bash
task lint
```

Configuration is in `.golangci.yml`. Common issues:

- Unused variables/imports
- Missing error checks
- Inefficient code
- Style violations

### Copyright Headers

All source files must include the copyright header:

```go
// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package yourpackage
```

## Submitting Changes

### Commit Messages

Write clear, descriptive commit messages:

```
<type>: <subject>

<body>

<footer>
```

Types:
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `style`: Code style changes (formatting, etc.)
- `refactor`: Code refactoring
- `test`: Test additions/changes
- `chore`: Build process or auxiliary tool changes

Example:

```
feat: add shell completion for task names

Add dynamic completion for task names when using the 'run' command.
Task names are discovered from the current Taskfile.gen.yaml.

Closes #123
```

### Pull Request Process

1. **Update your branch**

   ```bash
   git fetch upstream
   git rebase upstream/main
   ```

2. **Push to your fork**

   ```bash
   git push origin feature/your-feature-name
   ```

3. **Create Pull Request**

   - Visit your fork on GitHub
   - Click "Pull Request"
   - Fill out the PR template
   - Link related issues

4. **PR Requirements**

   - [ ] Tests pass (`task test`)
   - [ ] Linters pass (`task lint`)
   - [ ] Code is formatted (`task fmt`)
   - [ ] Documentation updated (if needed)
   - [ ] Commit messages follow convention
   - [ ] PR description explains changes

5. **Code Review**

   - Address review comments
   - Push updates to your branch
   - Request re-review when ready

6. **Merge**

   - Maintainers will merge after approval
   - Delete your branch after merge

### Documentation

Update documentation when making changes:

- **Code comments** - For internal documentation
- **README.md** - For project overview changes
- **docs/** - For user-facing documentation
- **CHANGELOG.md** - For notable changes

## Reporting Bugs

### Before Reporting

1. Check existing issues
2. Update to latest version
3. Search discussions

### Bug Report Template

Open an issue with:

**Title**: Brief description of the bug

**Description**:
- What happened
- What you expected
- Steps to reproduce

**Environment**:
- OS: macOS/Linux/Windows
- CLI version: `dr version`
- Go version: `go version`

**Additional Context**:
- Error messages
- Log output (`dr --debug`)
- Screenshots (if applicable)

### Security Vulnerabilities

**DO NOT** open a GitHub issue for security vulnerabilities.

Instead:
1. Email: oss-community-management@datarobot.com
2. Include detailed description
3. Include steps to reproduce
4. Wait for maintainer response

## Requesting Features

### Feature Request Template

**Title**: Clear feature description

**Problem Statement**:
- What problem does this solve?
- Who benefits from this feature?

**Proposed Solution**:
- How should it work?
- Example usage

**Alternatives Considered**:
- Other approaches
- Why this solution is preferred

**Additional Context**:
- Use cases
- Similar features in other tools

## Development Tips

### Useful Commands

```bash
# Run CLI in development
go run main.go templates list

# Build with race detection
go build -race -o dist/dr

# Profile performance
go test -cpuprofile cpu.prof
go tool pprof cpu.prof

# Generate mocks (if using mockery)
mockery --all

# Update dependencies
go get -u ./...
go mod tidy
```

### Debugging

```bash
# Use delve debugger
dlv debug main.go -- templates list

# Add debug prints
log.Debug("Variable value", "key", value)

# Enable verbose output
dr --debug templates list
```

### Testing Locally

```bash
# Build and install locally
task build
sudo mv dist/dr /usr/local/bin/dr-dev

# Test with real DataRobot instance
dr-dev auth login
dr-dev templates list
```

## Project Structure

```
cli/
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
│   ├── task/             # Task discovery
│   └── version/          # Version info
├── tui/                   # Terminal UI components
├── docs/                  # Documentation
├── main.go               # Entry point
├── Taskfile.yaml         # Task definitions
└── go.mod                # Go module definition
```

## Project Maintainers

- DataRobot CLI Team

## Getting Help

- 📖 [Documentation](docs/)
- 💬 [GitHub Discussions](https://github.com/datarobot/cli/discussions)
- 🐛 [Issue Tracker](https://github.com/datarobot/cli/issues)
- 📧 Email: oss-community-management@datarobot.com

## Response Times

Maintainers will make every effort to respond to:
- Issues: Within 3-5 business days
- Pull Requests: Within 5-7 business days
- Security Issues: Within 1-2 business days

If you don't receive a response within these timeframes, please email oss-community-management@datarobot.com.

## License

By contributing to this project, you agree that your contributions will be licensed under the same license as the project (see [LICENSE.txt](LICENSE.txt)).

---

Thank you for contributing to the DataRobot CLI! 🎉
