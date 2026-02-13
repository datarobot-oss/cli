# Interactive configuration system

The DataRobot CLI features a powerful interactive configuration system that guides users through setting up application templates with smart prompts, validation, and conditional logic.

## Overview

The interactive configuration system is built using [Bubble Tea](https://github.com/charmbracelet/bubbletea), a Go framework for building terminal user interfaces. It provides:

- Guided setup: A step-by-step wizard for configuration
- Smart prompts: Context-aware questions with validation
- Conditional logic: Show/hide prompts based on previous answers
- Multiple input types: Text fields, checkboxes, and selection lists
- Visual feedback: Beautiful terminal UI with progress indicators

## Architecture

### Components

The configuration system consists of three main layers:

```
┌─────────────────────────────────────────┐
│         User interface layer            │
│  (Bubble Tea models and views)          │
├─────────────────────────────────────────┤
│         Business logic layer            │
│  (Prompt processing and validation)     │
├─────────────────────────────────────────┤
│             Data layer                  │
│  (Environment discovery and storage)    │
└─────────────────────────────────────────┘
```

### Key files

- `cmd/dotenv/model.go`: The main dotenv editor model
- `cmd/dotenv/promptModel.go`: Individual prompt handling
- `internal/envbuilder/discovery.go`: Prompt discovery from templates
- `cmd/templates/setup/model.go`: Template setup wizard orchestration

## Configuration flow

### 1. Template setup wizard

When you run `dr templates setup`, the wizard flow is:

```
Welcome screen
    ↓
DataRobot URL configuration (if needed)
    ↓
Authentication (if needed)
    ↓
Template selection
    ↓
Template cloning
    ↓
Environment configuration (skipped if previously completed)
    ↓
Completion
```

**State-aware behavior**: If `dr dotenv setup` has been successfully run in the past (tracked via state file), the Environment configuration step is automatically skipped. This allows you to re-run the template setup without re-configuring your environment variables. See [Configuration - State tracking](../user-guide/configuration.md#state-tracking) for details.

### 2. Environment configuration

The environment configuration phase (dotenv wizard):

```
Load .env template
    ↓
Discover user prompts (from .datarobot files)
    ↓
Initialize response map
    ↓
For each required prompt:
    ├── Display prompt with help text
    ├── Show options (if applicable)
    ├── Capture user input
    ├── Validate input
    ├── Update required sections (conditional)
    └── Move to next prompt
    ↓
Generate .env file
    ↓
Save configuration
```

## Prompt types

### Text input prompts

Simple text entry for values:

```yaml
# Example text prompt for database URL
prompts:
  - key: "database_url"
    env: "DATABASE_URL"
    help: "Enter your database connection string"
    default: "postgresql://localhost:5432/mydb"
    optional: false
```

User experience:

```
Enter your database connection string
> postgresql://localhost:5432/mydb█

Default: postgresql://localhost:5432/mydb
```

### Secret string prompts

Secure text entry with input masking for sensitive values:

```yaml
prompts:
  - key: "api_key"
    env: "API_KEY"
    type: "secret_string"
    help: "Enter your API key"
    optional: false
```

User experience:

```
Enter your API key
> ••••••••••••••••••█

Input is masked for security
```

Features:

- Input is masked with bullets (•).
- Prevents shoulder-surfing and accidental exposure.
- Stored as plain text in `.env` file (file should be in `.gitignore`).

### Auto-generated secrets

Secret strings can be automatically generated:

```yaml
prompts:
  - key: "session_secret"
    env: "SESSION_SECRET"
    type: "secret_string"
    generate: true
    help: "Session encryption key (auto-generated)"
    optional: false
```

Behavior:

- If no value exists, a cryptographically secure random string is generated.
- Generated secrets are 32 characters long.
- Uses base64 URL-safe encoding.
- Only generates when value is empty (preserves existing secrets).

User experience:

```
Session encryption key (auto-generated)
> ••••••••••••••••••••••••••••••••█

A random secret was generated. Press Enter to accept or type a custom value.
```

### Single selection prompts

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

User experience:

```
Select your deployment environment

  > Development
    Staging
    Production
```

### Multiple selection prompts

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

User experience:

```
Select features to enable (Use Space to toggle and Enter to confirm)

  > [x] Analytics
    [ ] Monitoring
    [x] Caching
```

### Optional prompts

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

## Conditional prompts

Prompts can be shown or hidden based on previous selections using the `requires` fields.

### Section-based conditions

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

database_config:  # Only shown if enabled
  - key: "database_type"
    help: "Select database type"
    options:
      - name: "PostgreSQL"
        value: "postgres"
      - name: "MySQL"
        value: "mysql"

  - env: "DATABASE_URL"
    help: "Enter database connection string"
```

### How it works

1. Initial state: All sections start as disabled
2. User selection: When you select an option with `requires: "section_name"`
3. Section activation: That section becomes enabled
4. Prompt display: Prompts in matching `section_name:` are shown
5. Cascade: Newly shown prompts can activate additional sections

### Example flow

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

## Prompt discovery

The CLI automatically discovers prompts from `.datarobot` directories in your template.

### Discovery process

```go
// From internal/envbuilder/discovery.go
func GatherUserPrompts(rootDir string) ([]UserPrompt, []string, error) {
    // 1. Recursively find all .datarobot directories
    // 2. Load prompts.yaml from each directory
    // 3. Parse and validate prompt definitions
    // 4. Build dependency graph (requires: "section")
    // 5. Return ordered prompts with root sections
}
```

### Prompt file structure

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
section_name: # Optional: Only show if section enabled
  - env: "ENV_VAR_NAME"      # Optional: Environment variable to set
    type: "secret_string"     # Optional: "string" (default) or "secret_string"
    help: "Help text shown to user"
    default: "default value"  # Optional
    optional: false           # Optional: Can be skipped
    multiple: false           # Optional: Allow multiple selections
    generate: false           # Optional: Auto-generate random value (secret_string only)
    always_prompt: false      # Optional: Always show prompt even if default is set
    options:                  # Optional: List of choices
      - name: "Display Name"
        value: "actual_value"
        requires: "other_section"  # Optional: Enable section if selected
```

## UI components

### Prompt model

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

### List rendering

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

### State management

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

## Keyboard controls

### List navigation

- `↑/↓` or `j/k` - Navigate list items
- `Space` - Toggle checkbox (multiple selection)
- `Enter` - Confirm selection
- `Esc` - Go back to previous screen

### Text input

- Type normally to enter text
- `Enter` - Confirm input
- `Esc` - Go back to previous screen

### Editor mode

- `w` - Start wizard mode
- `e` - Open text editor
- `Enter` - Finish and save
- `Esc` - Save and exit editor

## Advanced features

### Default values

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

**Note:** Prompts with default values are automatically skipped during the wizard unless:
- The user has modified the value from the default
- The prompt has `always_prompt: true` set
- The prompt has options with `requires` fields (which control conditional sections)

Use `dr dotenv setup --all` to force all prompts to be shown.

### Always prompt

To force a prompt to always be shown even when it has a default value:

```yaml
prompts:
  - key: "port"
    env: "PORT"
    help: "Application port"
    default: "8080"
    always_prompt: true  # Always prompt user even though default exists
```

This is useful for prompts where you want users to consciously confirm or change the default value.

**Note:** Prompts with `requires` options are always shown regardless of default values, since they control which conditional sections are enabled.

**Note:** Prompts with `hidden: true` will never be shown, even if `always_prompt` is set.

### Secret values

The CLI provides secure handling for sensitive values using the `secret_string` type:

```yaml
prompts:
  - key: "api_key"
    env: "API_KEY"
    type: "secret_string"
    help: "Enter your API key"
```

Features:

- Input is masked with bullet characters (••••) during entry.
- Prevents accidental exposure of sensitive data.
- Cryptographic auto-generation secures random values with `generate: true`.

Auto-detection: Variables with names containing "PASSWORD", "SECRET", "KEY", or "TOKEN" are automatically treated as secrets in the editor view, displaying as `***` instead of the actual value.

### Generated secrets

You can automatically generate secrets.

```yaml
prompts:
  - key: "session_secret"
    env: "SESSION_SECRET"
    type: "secret_string"
    generate: true
    help: "Session encryption key"
```

When `generate: true` is set:

- A 32-character cryptographically secure random string is generated if no value exists.
- Uses base64 URL-safe encoding.
- Preserves existing values (only generates for empty fields).
- User can still override with a custom value.

### Merge environment variables

The wizard intelligently merges:

1. Existing values from an `.env` file
2. Environment variables from the current shell
3. User responses from the wizard
4. Template defaults from `.env.template`

Priority (highest to lowest):

1. User wizard responses
2. Current environment variables
3. Existing .env values
4. Template defaults

## Error handling

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

### User feedback

```go
// Visual feedback for errors
if err != nil {
    sb.WriteString(errorStyle.Render("❌ " + err.Error()))
}

// Success indicators
sb.WriteString(successStyle.Render("✓ Configuration saved"))
```

## Integration example

To add the interactive wizard to your template:

### 1. Create a prompts file

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

auth_config:
  - env: "AUTH_PROVIDER"
    help: "Select authentication provider"
    options:
      - name: "OAuth2"
        value: "oauth2"
      - name: "SAML"
        value: "saml"

db_config:
  - env: "DATABASE_URL"
    help: "Enter database connection string"
    default: "postgresql://localhost:5432/myapp"
```

### 2. Create an environment template

`.env.template`:

```bash
# Application settings
APP_NAME=

# Features
ENABLED_FEATURES=

# Authentication (if enabled)
# AUTH_PROVIDER=

# Database (if enabled)
# DATABASE_URL=
```

### 3. Run setup

```bash
dr templates setup
```

The wizard automatically discovers and uses your prompts.

## Best practices

### 1. Clear help text

```yaml
# ✓ Good
help: "Enter your PostgreSQL connection string (e.g., postgresql://user:pass@host:5432/db)"

# ✗ Bad
help: "Database URL"
```

### 2. Sensible defaults

```yaml
# Provide reasonable defaults
default: "postgresql://localhost:5432/myapp"
```

### 3. Organize with sections

```yaml
# Group related prompts
prompts:
  - key: "enable_monitoring"
    options:
      - name: "Yes"
        requires: "monitoring_config"

monitoring_config:
  - env: "monitoring_url"
    help: "Monitoring service URL"
```

### 4. Use descriptive keys

```yaml
# ✓ Good
key: "database_connection_pool_size"

# ✗ Bad
key: "pool"
```

### 5. Validate input

Use `optional: false` for required fields:

```yaml
prompts:
  - key: "api_token"
    env: "API_TOKEN"
    type: "secret_string"
    help: "Enter your DataRobot API key"
    optional: false  # Required!
```

### 6. Use secret types for sensitive data

Always use `secret_string` for passwords, API keys, and tokens.

```yaml
# ✓ Good
prompts:
  - key: "database_password"
    env: "DATABASE_PASSWORD"
    type: "secret_string"
    help: "Database password"

# ✗ Bad (exposes password during input)
prompts:
  - key: "database_password"
    env: "DATABASE_PASSWORD"
    help: "Database password"
```

### 7. Auto-generate secrets when possible

Use `generate: true` for application secrets that don't need to be memorized.

```yaml
# ✓ Good for session keys, encryption keys
prompts:
  - key: "jwt_secret"
    env: "JWT_SECRET"
    type: "secret_string"
    generate: true
    help: "JWT signing key"

# ✗ Don't auto-generate user credentials
prompts:
  - key: "admin_password"
    env: "ADMIN_PASSWORD"
    type: "secret_string"
    help: "Administrator password"

```

## Testing prompts

Test your prompt configuration:

```bash
# Dry run without saving
dr dotenv setup

# View generated .env
cat .env
```

## See also

- [Template structure](structure.md): How templates are organized
- [Environment variables](environment-variables.md): Manage .env files
- [Command reference: dotenv](../commands/dotenv.md): dotenv command documentation
