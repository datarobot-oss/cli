# Development guide

This guide outlines how to build, test, and develop with the DataRobot CLI.

## Table of contents

- [Build from source](#build-from-source)
- [Directory structure](#directory-structure)
- [Coding standards](#coding-standards)
- [Development workflow](#development-workflow)
- [Testing](#testing)
- [Debugging](#debugging)
- [Release process](#release-process)

## Build from source

### Prerequisites

- [Go 1.25.6+](https://golang.org/dl/)
- Git version control
- [Task](https://taskfile.dev/installation/) (The task runner)

### Quick build

```bash
# Clone repository
git clone https://github.com/datarobot-oss/cli.git
cd cli

# Install development tools
task dev-init

# Build binary
task build

# Binary is at ./dist/dr
./dist/dr version
```

### Available tasks

```bash
# Show all tasks
task --list

# Common tasks
task build              # Build the CLI binary
task test               # Run all tests
task test-coverage      # Run tests with coverage
task lint               # Run linters (includes formatting)
task clean              # Clean build artifacts
task dev-init           # Setup development environment
task install-tools      # Install development tools
task run                # Run CLI without building
```

### Build options

Always use `task build` for building the CLI to ensure that the proper version information and build flags are applied.

```bash
# Standard build (recommended)
task build

# Run without building (for quick testing)
task run -- templates list
```

The `task build` command automatically includes:

- Version information from Git
- The Git commit hash
- A build timestamp
- Proper `ldflags` configuration

For cross-platform builds and releases, task build includes GoReleaser (see [Release Process](#release-process)).

## Directory structure

```sh
cli/
├── cmd/                     # Command implementations (Cobra)
│   ├── root.go              # Root command and global flags
│   ├── auth/                # Authentication commands
│   │   ├── cmd.go           # Auth command group
│   │   ├── login.go         # Login command
│   │   ├── logout.go        # Logout command
│   │   └── setURL.go        # Set URL command
│   ├── dotenv/              # Environment variable management
│   │   ├── cmd.go           # Dotenv command
│   │   ├── model.go         # TUI model (Bubble Tea)
│   │   ├── promptModel.go   # Prompt handling
│   │   ├── template.go      # Template parsing
│   │   └── variables.go     # Variable handling
│   ├── run/                 # Task execution
│   │   └── cmd.go           # Run command
│   ├── templates/           # Template management
│   │   ├── cmd.go           # Template command group
│   │   ├── clone/           # Clone subcommand
│   │   ├── list/            # List subcommand
│   │   ├── setup/           # Setup wizard
│   │   └── status.go        # Status command
│   └── self/                # CLI utility commands
│       ├── cmd.go           # Self command group
│       ├── completion.go    # Completion generation
│       └── version.go       # Version command
├── internal/                # Private packages (not importable)
│   ├── assets/              # Embedded assets
│   │   └── templates/       # HTML templates
│   ├── config/              # Configuration management
│   │   ├── config.go        # Config loading/saving
│   │   ├── auth.go          # Auth config
│   │   └── constants.go     # Constants
│   ├── drapi/               # DataRobot API client
│   │   ├── llmGateway.go    # LLM Gateway API
│   │   └── templates.go     # Templates API
│   ├── envbuilder/          # Environment configuration
│   │   ├── builder.go       # Env file building
│   │   └── discovery.go     # Prompt discovery
│   ├── task/                # Task runner integration
│   │   ├── discovery.go     # Taskfile discovery
│   │   └── runner.go        # Task execution
│   └── version/             # Version information
│       └── version.go
├── tui/                     # Terminal UI shared components
│   ├── banner.go            # ASCII banner
│   └── theme.go             # Color theme
├── docs/                    # Documentation
├── main.go                  # Application entry point
├── go.mod                   # Go module dependencies
├── go.sum                   # Dependency checksums
├── Taskfile.yaml            # Task definitions
└── goreleaser.yaml          # Release configuration
```

### Key components

#### Command layer (cmd/)

The CLI is built using the [Cobra](https://github.com/spf13/cobra) framework.

Commands are hierarchically organized. There should be a one-to-one mapping between commands and files/directories. For example, the `templates` command group is in `cmd/templates/`, with subcommands in their own directories.

Code in the `cmd/` folder should primarily handle command-line parsing, argument validation, and the orchestration of calls to internal packages. There should be minimal to no business logic here. **Consider this the UI layer of the application.**

```go
// cmd/root.go - Root command definition
var RootCmd = &cobra.Command{
    Use:   "dr",
    Short: "DataRobot CLI",
    Long:  "Command-line interface for DataRobot",
}

// Register subcommands
RootCmd.AddCommand(
    auth.Cmd(),
    templates.Cmd(),
    // ...
)
```

#### TUI layer

The TUI layer contains the `cmd/dotenv/` and `cmd/templates/setup/` directories. It uses [Bubble Tea](https://github.com/charmbracelet/bubbletea) for interactive UIs.

```go
// Bubble Tea model
type Model struct {
    // State
    screen screens

    // Sub-models
    textInput textinput.Model
    list      list.Model
}

// Required methods
func (m Model) Init() tea.Cmd
func (m Model) Update(tea.Msg) (tea.Model, tea.Cmd)
func (m Model) View() string
```

#### Internal packages

Internal packages (`internal/`) house core business logic, API clients, configuration management, and more.

#### Configuration

Configuration is found in `internal/config/`. The CLI uses [Viper](https://github.com/spf13/viper) for configuration as well as a state registry.

```go
// Load config
viper.SetConfigName("config")
viper.SetConfigType("yaml")
viper.AddConfigPath("~/.datarobot")
viper.ReadInConfig()

// Access values
endpoint := viper.GetString("datarobot.endpoint")
```

#### API client

Using the API client directory, `internal/drapi/`, you can make requests to the HTTP client for DataRobot APIs.

```go
// Make API request
func GetTemplates() (*TemplateList, error) {
    resp, err := http.Get(endpoint + "/api/v2/templates")
    // ... handle response
}
```

### Design patterns

#### Command pattern

Each command is self-contained:

```go
// cmd/templates/list/cmd.go
var Cmd = &cobra.Command{
    Use:     "list",
    Short:   "List templates",
    GroupID: "core",
    RunE: func(cmd *cobra.Command, args []string) error {
        // Implementation
        return listTemplates()
    },
}
```

`RunE` is the main execution function. Cobra also provides `PreRunE`, `PostRunE`, and other hooks. DataRobot recommends using these functions for setup, teardown, and validation.

```go
PersistPreRunE: func(cmd *cobra.Command, args []string) error {
    // Setup logging
    return setupLogging()
},
PreRunE: func(cmd *cobra.Command, args []string) error {
    // Validate args
    return validateArgs(args)
},
PostRunE: func(cmd *cobra.Command, args []string) error {
    // Cleanup
    return nil
},
```

Each command can be assigned to a group via `GroupID` for better organization in `dr help` views. Commands without a `GroupID` are listed under Additional commands.

#### Model-view-update

Interactive UIs like Bubble Tea use the Model-view-update (MVU) pattern:

```go
// Update handles events
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyMsg:
        return m.handleKey(msg)
    case dataLoadedMsg:
        return m.handleData(msg)
    }
    return m, nil
}

// View renders current state
func (m Model) View() string {
    return lipgloss.JoinVertical(
        lipgloss.Left,
        m.header(),
        m.content(),
        m.footer(),
    )
}
```

## Coding standards

### Go style requirements

**Critical**: All code must pass `golangci-lint` with zero errors. Strictly follow these whitespace rules:

1. Never cuddle declarations Always add a blank line before `var`, `const`, and `type` declarations when they follow other statements.
2. Separate statement types: Add blank lines between different statement types (assign, if, for, return, etc.).
3. Blank line after block start: Add a blank line after the opening braces of functions/blocks when they follow declarations.
4. Blank line before multi-line statements: Add a blank line before if/for/switch statements.

Review an example of correct spacing below.

```go
func example() {
    x := 1

    if x > 0 {
        y := 2

        fmt.Println(y)
    }

    var result string

    result = "done"

    return result
}
```

The example below outlines common mistakes to avoid.

```go
// ❌ BAD: Cuddled declaration
func bad() {
    x := 1
    var y int  // Missing a blank line before the declaration
}

// ✅ GOOD: Properly spaced
func good() {
    x := 1

    var y int
}
```

### TUI development standards

Consider the following when building terminal user interfaces.

1. **Always use the `tui.Run` wrapper to execute TUI models**. This ensures global `Ctrl-C` handling and sets up logging to `dr-tui-debug.log`.

   ```go
   import "github.com/datarobot/cli/tui"

   // Run your model
   _, err := tui.Run(yourModel)
   ```

2. **Reuse existing TUI components**. Check `tui/` package first before creating new components. Also explore the [Bubbles library](https://github.com/charmbracelet/bubbles) for pre-built components.

3. **Use common lipgloss styles**. The styles are defined in [tui/styles.go](../../tui/styles.go) for visual consistency:

   ```go
   import "github.com/datarobot/cli/tui"

   // Use theme styles
   title := tui.TitleStyle.Render("My Title")
   error := tui.ErrorStyle.Render("Error message")
   ```

### Quality tools

All code must pass these tools without errors:

- `go mod tidy`: Dependency management
- `go fmt`: Basic formatting
- `go vet`: Suspicious constructs
- `golangci-lint`: Comprehensive linting (includes wsl, revive, staticcheck, etc.)
- `goreleaser check`: Release configuration validation

**Before committing code, verify it follows [wsl](https://github.com/bombsimon/wsl?tab=readme-ov-file#wsl---whitespace-linter) (whitespace) rules.**

### Run quality checks

```bash
# Run all quality checks at once
task lint

# Individual checks
go mod tidy
go fmt ./...
go vet ./...
task install-tools  # Install golangci-lint
./tmp/bin/golangci-lint run ./...
./tmp/bin/goreleaser check
```

## Development workflow

**Always use Taskfile tasks for development operations and not direct `go` commands**. This ensures consistency, proper build flags, and a correct environment setup.

```bash
# ✅ CORRECT: Use task commands
task build
task test
task lint

# ❌ INCORRECT: Don't use direct go commands
go build
go test
```

### 1. Set up the development environment

```bash
# Clone and setup
git clone https://github.com/datarobot-oss/cli.git
cd cli
task dev-init
```

### 2. Create a feature branch

```bash
git checkout -b feature/my-feature
```

### 3. Make changes

```bash
# Edit code
vim cmd/templates/new-feature.go

# Run linters (includes formatting)
task lint
```

### 4. Test changes

```bash
# Run tests
task test

# Run a specific test (direct go test is acceptable for specific tests)
go test -run TestMyFeature ./cmd/templates

# Test manually using task run
task run -- templates list

# Or build and test the binary
task build
./dist/dr templates list
```

### 5. Commit and push

```bash
git add .
git commit -m "feat: add new feature"
git push origin feature/my-feature
```

## Testing

### Unit tests

Unit tests are `*_test.go` files co-located with the code they test.

```go
// cmd/auth/login_test.go
package auth

import (
    "testing"
    "github.com/stretchr/testify/assert"
)

func TestLogin(t *testing.T) {
    // Arrange
    mockAPI := &MockAPI{}

    // Act
    err := performLogin(mockAPI)

    // Assert
    assert.NoError(t, err)
}
```

### Integration tests

Integration tests are also written as Go tests (`*_test.go`). They typically exercise interactions between multiple packages and/or use temporary on-disk state.

```go
// internal/config/config_test.go
func TestConfigReadWrite(t *testing.T) {
    // Create temp config
    tmpDir := t.TempDir()
    configPath := filepath.Join(tmpDir, "config.yaml")

    // Write config
    err := SaveConfig(configPath, &Config{
        Endpoint: "https://test.datarobot.com",
    })
    assert.NoError(t, err)

    // Read config
    config, err := LoadConfig(configPath)
    assert.NoError(t, err)
    assert.Equal(t, "https://test.datarobot.com", config.Endpoint)
}
```

### TUI tests

Using [teatest](https://github.com/charmbracelet/x/tree/main/exp/teatest):

```go
// cmd/dotenv/model_test.go
func TestDotenvModel(t *testing.T) {
    m := Model{
        // Setup model
    }

    tm := teatest.NewTestModel(t, m)

    // Send keypress
    tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

    // Wait for update
    teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
        return bytes.Contains(bts, []byte("Expected output"))
    })
}
```

### Running tests

```bash
# All tests (recommended)
task test

# With coverage (opens an HTML report)
task test-coverage

# Specific package (direct go test is fine for targeted testing)
go test ./internal/config

# Verbose
go test -v ./...

# With race detection (task test already includes this)
go test -race ./...

# Specific test
go test -run TestLogin ./cmd/auth
```

**Note**: `task test` automatically runs tests with race detection and coverage enabled.

### Go version requirements for race detection

The `-race` flag requires the race runtime library to match your Go compiler version exactly. If you see an error like:

```text
compile: version "go1.X.Y" does not match go tool version "go1.X.Z"
```

This means your installed Go version doesn't match the version specified in `go.mod`. Go's `GOTOOLCHAIN=auto` setting (the default) automatically downloads the required toolchain, but the race runtime comes from your local `GOROOT` installation.

**To resolve:**

- **Upgrade Go** to match `go.mod`: `brew upgrade go` (macOS)
- **Or downgrade `go.mod`**: `go mod edit -go=1.X.Z` (where `1.X.Z` is your installed version)
- **Or force the downloaded toolchain**: `export GOTOOLCHAIN=go1.X.Y` (where `1.X.Y` is the version in `go.mod`)

### Run smoke tests using GitHub Actions

DataRobot has smoke tests that are not currently run on Pull Requests. However you can use PR comments to trigger them.

These are the appropriate comments to trigger respective tests:

- `/trigger-smoke-test` or `/trigger-test-smoke`: Run smoke tests on a PR.
- `/trigger-install-test` or `/trigger-test-install`: Run installation tests on a PR.

## Debugging

### Use Delve

```bash
# Install delve
go install github.com/go-delve/delve/cmd/dlv@latest

# Debug with arguments
dlv debug main.go -- templates list

# In debugger
(dlv) break main.main
(dlv) continue
(dlv) print variableName
(dlv) next
```

### Debug logging

Enable debug logging to see detailed execution information:

```bash
# Enable debug mode (use task run)
task run -- --debug templates list

# Or with built binary
task build
./dist/dr --debug templates list
```

When you enable debug mode, the CLI:

- Prints detailed log messages to stderr and `dr-tui-debug.log` file in the current directory.

When adding new debug output:

- Never log user-provided input (including prompt responses), and avoid logging secrets (tokens, passwords, etc.).

### Add debug statements

```go
import (
	"github.com/datarobot/cli/internal/log"
)

// Debug logging
log.Debug("Variable value", "key", value)
log.Info("Processing started")
log.Warn("Unexpected condition")
log.Error("Operation failed", "error", err)
```

## Release process

See [Releasing documentation](releasing.md) for a detailed overview of the release process.

### Quick release

```bash
# Tag version
git tag v1.0.0
git push --tags

# GitHub Actions will:
# 1. Build for all platforms
# 2. Run tests
# 3. Create GitHub release
# 4. Upload binaries
```

## See also

- [Contributing guide](https://github.com/datarobot-oss/cli/blob/main/CONTRIBUTING.md)
- [Project structure](structure.md)&mdash;code organization and design
- [Release process](releasing.md)&mdash;how releases are created and published
