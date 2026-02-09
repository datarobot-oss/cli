# Project structure

This page describes the organization of the DataRobot CLI codebase.

## Directory overview

```text
cli/
├── cmd/                     # Command implementations (Cobra)
│   ├── root.go              # Root command and global flags
│   ├── auth/                # Authentication commands
│   ├── component/           # Component management commands
│   ├── dotenv/              # Environment variable management
│   ├── run/                 # Task execution
│   ├── self/                # Self-management commands
│   ├── start/               # Application startup
│   ├── task/                # Task commands
│   └── templates/           # Template management
├── internal/                # Private application code
│   ├── assets/              # Embedded assets
│   ├── config/              # Configuration management
│   ├── copier/              # Template copying utilities
│   ├── drapi/               # DataRobot API client
│   ├── envbuilder/          # Environment builder
│   ├── misc/                # Miscellaneous utilities
│   ├── repo/                # Repository detection
│   ├── shell/               # Shell utilities
│   ├── task/                # Task discovery and execution
│   ├── tools/               # Tool prerequisites
│   └── version/             # Version information
├── tui/                     # Terminal UI components
│   ├── banner.go            # Banner display
│   ├── interrupt.go         # Interrupt handling
│   ├── program.go           # TUI execution wrapper
│   └── theme.go             # Visual theme
├── docs/                    # Documentation
│   ├── commands/            # Command reference
│   ├── development/         # Development guides
│   ├── template-system/     # Template system docs
│   └── user-guide/          # User documentation
├── smoke_test_scripts/      # Smoke tests
├── main.go                  # Application entry point
├── Taskfile.yaml            # Task definitions
├── go.mod                   # Go module definition
└── goreleaser.yaml          # Release configuration
```

## Key directories

### cmd/

Contains all CLI command implementations using the Cobra framework. Each subdirectory represents a command or command group.

#### Structure

- `root.go` is the root command setup and global flags
- Each command has its own subdirectory with `cmd.go` as the entry point
- Commands that have subcommands are in the same directory

#### Example

- `cmd/auth/cmd.go`: The auth command group
- `cmd/auth/login.go`: The login subcommand
- `cmd/auth/logout.go`: The logout subcommand

### internal/

Private application code that cannot be imported by other projects. This follows Go's convention for internal packages.

#### config/

The configuration management directory, including:

- Reading/writing configuration files
- Authentication states
- User preferences

#### drapi/

DataRobot API client implementation for:

- Template listing and retrieval
- API authentication
- API endpoint communication

#### envbuilder/

An environment configuration builder that:

- Discovers environment variables from templates
- Validates configuration
- Generates `.env` files
- Provides interactive prompts

#### task/

Task discovery and execution:

- Taskfile detection
- Task parsing
- Task running
- Output handling

### tui/

Terminal UI components built with Bubble Tea:

- Reusable UI models
- Theme definitions
- Interrupt handling for graceful exits
- Banner displays

### docs/

Documentation organized by audience:

- `commands/`: Detailed command reference
- `development/`: Development guides for contributors
- `template-system/`: Template configuration system
- `user-guide/`: End-user documentation

## Code organization patterns

### Command structure

Each command follows this pattern:

```go
// cmd/example/cmd.go
package example

import "github.com/spf13/cobra"

var Cmd = &cobra.Command{
    Use:   "example",
    Short: "Example command",
    Long:  `Detailed description`,
    PreRunE: func(cmd *cobra.Command, args []string) error {
        // Validation and setup
        return nil
    },
    RunE: func(cmd *cobra.Command, args []string) error {
        // Command implementation
        return nil
    },
}

func init() {
    // Flag definitions
    Cmd.Flags().StringP("flag", "f", "", "Flag description")
}
```

### TUI models

TUI components use the Bubble Tea framework and are executed using the `tui.Run` wrapper,
which handles `Ctrl-C` signals and pauses stderr logging (but not `dr-tui-debug.log`) while program is running:

```go
// cmd/example/model.go
package example

import (
    tea "github.com/charmbracelet/bubbletea"
    "github.com/datarobot/cli/tui"
)

type model struct {
    // State fields
}

func (m model) Init() tea.Cmd {
    return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    // Handle messages
    return m, nil
}

func (m model) View() string {
    // Render UI
    return ""
}

// Usage in command
func runInteractive() error {
    m := model{}
    _, err := tui.Run(m)
    return err
}
```

### Configuration

Configuration is managed through Viper and stored in:

- `~/.config/datarobot/drconfig.yaml`: Global configuration and authentication tokens

Access configuration through the `internal/config` package:

```go
import "github.com/datarobot/cli/internal/config"

// Get configuration values
apiKey := config.GetAPIKey()
endpoint := config.GetEndpoint()

// Set configuration values
config.SetAPIKey("new-key")
config.SaveConfig()
```

## Testing structure

Tests are colocated with the code they test:

- Unit tests: `*_test.go` files in the same package
- Test helpers in same directory when needed
- Smoke tests in the `smoke_test_scripts/` directory

## Build artifacts

Generated files and artifacts:

- `dist/`: Build outputs (created by Task/GoReleaser)
- `tmp/`: Temporary build files
- `coverage.txt`: Test coverage reports

## Next steps

- [Setup guide](setup.md): setting up your development environment
- [Build guide](building.md): Detailed build information and architecture
- [Contributions](../../CONTRIBUTING.md): Contribution guidelines
