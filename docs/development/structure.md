# Project Structure

This document describes the organization of the DataRobot CLI codebase.

## Directory Overview

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

## Key Directories

### cmd/

Contains all CLI command implementations using the Cobra framework. Each subdirectory represents a command or command group.

**Structure:**

- `root.go`&mdash;root command setup and global flags
- Each command has its own subdirectory with `cmd.go` as the entry point
- Commands that have subcommands organize them in the same directory

**Example:**

- `cmd/auth/cmd.go`&mdash;auth command group
- `cmd/auth/login.go`&mdash;login subcommand
- `cmd/auth/logout.go`&mdash;logout subcommand

### internal/

Private application code that cannot be imported by other projects. This follows Go's convention for internal packages.

#### config/

Configuration management including:

- Reading/writing configuration files
- Authentication state
- User preferences

#### drapi/

DataRobot API client implementation for:

- Template listing and retrieval
- API authentication
- API endpoint communication

#### envbuilder/

Environment configuration builder that:

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

- `commands/`&mdash;detailed command reference
- `development/`&mdash;development guides for contributors
- `template-system/`&mdash;template configuration system
- `user-guide/`&mdash;end-user documentation

## Code Organization Patterns

### Command Structure

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

### TUI Models

TUI components use the Bubble Tea framework and are wrapped with `InterruptibleModel` for consistent Ctrl-C handling:

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
    wrapped := tui.NewInterruptibleModel(m)
    _, err := tea.NewProgram(wrapped).Run()
    return err
}
```

### Configuration

Configuration is managed through Viper and stored in:

- `~/.config/datarobot/config.yaml`&mdash;global configuration
- `~/.config/datarobot/credentials.json`&mdash;authentication tokens

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

## Testing Structure

Tests are colocated with the code they test:

- Unit tests: `*_test.go` files in the same package
- Test helpers in same directory when needed
- Smoke tests in `smoke_test_scripts/` directory

## Build Artifacts

Generated files and artifacts:

- `dist/`&mdash;build output (created by Task/GoReleaser)
- `tmp/`&mdash;temporary build files
- `coverage.txt`&mdash;test coverage report

## Next Steps

- [Setup Guide](setup.md)&mdash;setting up your development environment
- [Building Guide](building.md)&mdash;detailed build information and architecture
- [Contributing](../../CONTRIBUTING.md)&mdash;contribution guidelines
