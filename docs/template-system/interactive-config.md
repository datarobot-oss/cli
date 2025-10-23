# Interactive Configuration System

The DataRobot CLI features a powerful interactive configuration system that guides users through setting up application templates with smart prompts, validation, and conditional logic.

## Overview

The interactive configuration system is built using [Bubble Tea](https://github.com/charmbracelet/bubbletea), a Go framework for building terminal user interfaces. It provides:

- **Guided Setup**: Step-by-step wizard for configuration
- **Smart Prompts**: Context-aware questions with validation
- **Conditional Logic**: Show/hide prompts based on previous answers
- **Multiple Input Types**: Text fields, checkboxes, and selection lists
- **Visual Feedback**: Beautiful terminal UI with progress indicators

## Architecture

### Components

The configuration system consists of three main layers:

```
┌─────────────────────────────────────────┐
│         User Interface Layer            │
│  (Bubble Tea Models & Views)           │
├─────────────────────────────────────────┤
│         Business Logic Layer            │
│  (Prompt Processing & Validation)      │
├─────────────────────────────────────────┤
│         Data Layer                      │
│  (Environment Discovery & Storage)     │
└─────────────────────────────────────────┘
```

### Key Files

- `cmd/dotenv/model.go` - Main dotenv editor model
- `cmd/dotenv/promptModel.go` - Individual prompt handling
- `internal/envbuilder/discovery.go` - Prompt discovery from templates
- `cmd/templates/setup/model.go` - Template setup wizard orchestration

## Configuration Flow

### 1. Template Setup Wizard

When you run `dr templates setup`, the wizard flow is:

```
Welcome Screen
    ↓
DataRobot URL Configuration (if needed)
    ↓
Authentication (if needed)
    ↓
Template Selection
    ↓
Template Cloning
    ↓
Environment Configuration
    ↓
Completion
```

### 2. Environment Configuration

The environment configuration phase (dotenv wizard):

```
Load .env Template
    ↓
Discover User Prompts (from .datarobot files)
    ↓
Initialize Response Map
    ↓
For Each Required Prompt:
    ├── Display Prompt with Help Text
    ├── Show Options (if applicable)
    ├── Capture User Input
    ├── Validate Input
    ├── Update Required Sections (conditional)
    └── Move to Next Prompt
    ↓
Generate .env File
    ↓
Save Configuration
```

## Prompt Types

### Text Input Prompts

Simple text entry for values:

```yaml
# Example from .datarobot/prompts.yaml
prompts:
  - key: "database_url"
    env: "DATABASE_URL"
    help: "Enter your database connection string"
    default: "postgresql://localhost:5432/mydb"
    optional: false
```

**User Experience:**
```
Enter your database connection string
> postgresql://localhost:5432/mydb█

Default: postgresql://localhost:5432/mydb
```

### Single Selection Prompts

Choose one option from a list:

```yaml
prompts:
  - key: "environment"
    env: "ENVIRONMENT"
    help: "Select your deployment environment"
    optional: false
    multiple: false
    options:
      - name: "Development"
        value: "dev"
      - name: "Staging"
        value: "staging"
      - name: "Production"
        value: "prod"
```

**User Experience:**
```
Select your deployment environment

  > Development
    Staging
    Production
```

### Multiple Selection Prompts

Choose multiple options (checkboxes):

```yaml
prompts:
  - key: "features"
    env: "ENABLED_FEATURES"
    help: "Select features to enable (space to toggle, enter to confirm)"
    optional: false
    multiple: true
    options:
      - name: "Analytics"
        value: "analytics"
      - name: "Monitoring"
        value: "monitoring"
      - name: "Caching"
        value: "caching"
```

**User Experience:**
```
Select features to enable (space to toggle, enter to confirm)

  > [x] Analytics
    [ ] Monitoring
    [x] Caching
```

### Optional Prompts

Prompts that can be skipped:

```yaml
prompts:
  - key: "cache_url"
    env: "CACHE_URL"
    help: "Enter cache server URL (optional)"
    optional: true
    options:
      - name: "None (leave blank)"
        blank: true
      - name: "Redis"
        value: "redis://localhost:6379"
      - name: "Memcached"
        value: "memcached://localhost:11211"
```

## Conditional Prompts

Prompts can be shown or hidden based on previous selections using the `requires` and `section` fields.

### Section-Based Conditions

```yaml
prompts:
  - key: "enable_database"
    help: "Do you want to use a database?"
    multiple: true
    options:
      - name: "Yes"
        value: "yes"
        requires: "database_config"  # Enables this section
      - name: "No"
        value: "no"

  - key: "database_type"
    section: "database_config"  # Only shown if enabled
    help: "Select database type"
    options:
      - name: "PostgreSQL"
        value: "postgres"
      - name: "MySQL"
        value: "mysql"

  - key: "database_url"
    section: "database_config"  # Only shown if enabled
    env: "DATABASE_URL"
    help: "Enter database connection string"
```

### How It Works

1. **Initial State**: All sections start as disabled
2. **User Selection**: When user selects an option with `requires: "section_name"`
3. **Section Activation**: That section becomes enabled
4. **Prompt Display**: Prompts with matching `section: "section_name"` are shown
5. **Cascade**: Newly shown prompts can activate additional sections

### Example Flow

```
Q: Do you want to use a database?
   [x] Yes  ← User selects this (requires: "database_config")
   
   → Section "database_config" is now enabled

Q: Select database type
   (Now shown because section is enabled)
   > PostgreSQL
   
Q: Enter database connection string
   (Also shown because section is enabled)
   > postgresql://localhost:5432/db
```

## Prompt Discovery

The CLI automatically discovers prompts from `.datarobot` directories in your template.

### Discovery Process

```go
// From internal/envbuilder/discovery.go
func GatherUserPrompts(rootDir string) ([]UserPrompt, []string, error) {
    // 1. Recursively find all .datarobot directories
    // 2. Load prompts.yaml from each directory
    // 3. Parse and validate prompt definitions
    // 4. Build dependency graph (sections and requires)
    // 5. Return ordered prompts with root sections
}
```

### Prompt File Structure

Create `.datarobot/prompts.yaml` in any directory:

```
my-template/
├── .datarobot/
│   └── prompts.yaml          # Root level prompts
├── backend/
│   └── .datarobot/
│       └── prompts.yaml      # Backend-specific prompts
├── frontend/
│   └── .datarobot/
│       └── prompts.yaml      # Frontend-specific prompts
└── .env.template
```

Each `prompts.yaml`:

```yaml
prompts:
  - key: "unique_key"
    env: "ENV_VAR_NAME"      # Optional: Environment variable to set
    help: "Help text shown to user"
    default: "default value"  # Optional
    optional: false           # Optional: Can be skipped
    multiple: false           # Optional: Allow multiple selections
    section: "section_name"   # Optional: Only show if section enabled
    options:                  # Optional: List of choices
      - name: "Display Name"
        value: "actual_value"
        requires: "other_section"  # Optional: Enable section if selected
```

## UI Components

### Prompt Model

Each prompt is rendered by a `promptModel` that handles:

- Input capture (text field or list)
- Visual rendering
- State management
- Validation
- Success callback

```go
type promptModel struct {
    prompt     envbuilder.UserPrompt
    input      textinput.Model      // For text prompts
    list       list.Model           // For selection prompts
    Values     []string             // Captured values
    successCmd tea.Cmd              // Callback when complete
}
```

### List Rendering

Custom item delegate for beautiful list rendering:

```go
type itemDelegate struct {
    multiple bool  // Show checkboxes
}

func (d itemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
    // Renders items with:
    // - Checkboxes for multiple selection
    // - Highlighting for current selection
    // - Proper spacing and styling
}
```

### State Management

The main model manages screen transitions:

```go
type Model struct {
    screen             screens      // Current screen
    variables          []variable   // Loaded variables
    prompts            []envbuilder.UserPrompt
    requires           map[string]bool  // Active sections
    envResponses       map[string]string  // User responses
    currentPromptIndex int
    currentPrompt      promptModel
}
```

## Keyboard Controls

### List Navigation

- `↑/↓` or `j/k` - Navigate list items
- `Space` - Toggle checkbox (multiple selection)
- `Enter` - Confirm selection
- `Esc` - Go back to previous screen

### Text Input

- Type normally to enter text
- `Enter` - Confirm input
- `Esc` - Go back to previous screen

### Editor Mode

- `w` - Start wizard mode
- `e` - Open text editor
- `Enter` - Finish and save
- `Esc` - Save and exit editor

## Advanced Features

### Default Values

Prompts can have default values:

```yaml
prompts:
  - key: "port"
    env: "PORT"
    help: "Application port"
    default: "8080"
```

Shown as:
```
Application port
> 8080█

Default: 8080
```

### Secret Values

The CLI detects secret values (API keys, passwords) and masks them:

```go
// Variables with names containing:
// - "PASSWORD", "SECRET", "KEY", "TOKEN"
// are automatically marked as secrets

if v.secret {
    fmt.Fprintf(&sb, "***\n")  // Displayed as ***
} else {
    fmt.Fprintf(&sb, "%s\n", v.value)
}
```

### Environment Variable Merging

The wizard intelligently merges:

1. **Existing values** from .env file
2. **Environment variables** from current shell
3. **User responses** from wizard
4. **Template defaults** from .env.template

Priority (highest to lowest):
1. User wizard responses
2. Current environment variables
3. Existing .env values
4. Template defaults

## Error Handling

### Validation

Prompts can validate input:

```go
func (pm promptModel) submitInput() (promptModel, tea.Cmd) {
    pm.Values = pm.GetValues()
    
    // Don't submit if required and empty
    if !pm.prompt.Optional && len(pm.Values[0]) == 0 {
        return pm, nil  // Stay on prompt
    }
    
    return pm, pm.successCmd  // Proceed
}
```

### User Feedback

```go
// Visual feedback for errors
if err != nil {
    sb.WriteString(errorStyle.Render("❌ " + err.Error()))
}

// Success indicators
sb.WriteString(successStyle.Render("✓ Configuration saved"))
```

## Integration Example

To add the interactive wizard to your template:

### 1. Create Prompts File

`.datarobot/prompts.yaml`:

```yaml
prompts:
  - key: "app_name"
    env: "APP_NAME"
    help: "Enter your application name"
    optional: false

  - key: "features"
    help: "Select features to enable"
    multiple: true
    options:
      - name: "Authentication"
        value: "auth"
        requires: "auth_config"
      - name: "Database"
        value: "database"
        requires: "db_config"

  - key: "auth_provider"
    section: "auth_config"
    env: "AUTH_PROVIDER"
    help: "Select authentication provider"
    options:
      - name: "OAuth2"
        value: "oauth2"
      - name: "SAML"
        value: "saml"

  - key: "database_url"
    section: "db_config"
    env: "DATABASE_URL"
    help: "Enter database connection string"
    default: "postgresql://localhost:5432/myapp"
```

### 2. Create Environment Template

`.env.template`:

```bash
# Application Settings
APP_NAME=

# Features
ENABLED_FEATURES=

# Authentication (if enabled)
# AUTH_PROVIDER=

# Database (if enabled)
# DATABASE_URL=
```

### 3. Run Setup

```bash
dr templates setup
```

The wizard will automatically discover and use your prompts!

## Best Practices

### 1. Clear Help Text

```yaml
# ✓ Good
help: "Enter your PostgreSQL connection string (e.g., postgresql://user:pass@host:5432/db)"

# ✗ Bad
help: "Database URL"
```

### 2. Sensible Defaults

```yaml
# Provide reasonable defaults
default: "postgresql://localhost:5432/myapp"
```

### 3. Organize with Sections

```yaml
# Group related prompts
- key: "enable_monitoring"
  options:
    - name: "Yes"
      requires: "monitoring_config"

- key: "monitoring_url"
  section: "monitoring_config"
  help: "Monitoring service URL"
```

### 4. Use Descriptive Keys

```yaml
# ✓ Good
key: "database_connection_pool_size"

# ✗ Bad
key: "pool"
```

### 5. Validate Input

Use optional: false for required fields:

```yaml
prompts:
  - key: "api_key"
    env: "API_KEY"
    help: "Enter your DataRobot API key"
    optional: false  # Required!
```

## Testing Prompts

Test your prompt configuration:

```bash
# Dry run without saving
dr dotenv --wizard

# Check discovered prompts
dr templates status

# View generated .env
cat .env
```

## See Also

- [Template Structure](structure.md) - How templates are organized
- [Environment Variables](environment-variables.md) - Managing .env files
- [Command Reference: dotenv](../commands/dotenv.md) - dotenv command documentation
