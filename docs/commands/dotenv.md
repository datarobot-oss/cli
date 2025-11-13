# dotenv command

Manage environment variables and `.env` files in DataRobot templates.

## Overview

The `dr dotenv` command provides tools for creating, editing, validating, and updating environment configuration files. It includes an interactive wizard for guided setup and a text editor for direct file manipulation.

## Commands

### dr dotenv setup

Launch the interactive wizard to configure environment variables.

```bash
dr dotenv setup
```

**Features:**

- Interactive prompts for all required variables.
- Context-aware questions based on template configuration.
- Automatic discovery of configuration from `.datarobot/prompts.yaml` files.
- Smart defaults from `.env.template`.
- Secure handling of secret values.
- DataRobot authentication integration.
- Automatic state tracking of completion timestamp.

**Prerequisites:**

- Must be run inside a git repository.
- Requires authentication with DataRobot.

**State tracking:**

Upon successful completion, `dr dotenv setup` records the timestamp in the state file. This allows `dr templates setup` to intelligently skip dotenv configuration if it has already been completed. The state is stored in the same location as other CLI state (see [Configuration - State tracking](../user-guide/configuration.md#state-tracking)). Keep in mind that
`dr dotenv setup` will always prompt for configuration if run manually, regardless of state.

To force the setup wizard to run again (ignoring the state file), use the `--force-interactive` flag:

```bash
dr templates setup --force-interactive
```

This is useful for testing or when you need to reconfigure your environment from scratch.

**Example:**

```bash
cd my-template
dr dotenv setup
```

The wizard guides you through:
1. DataRobot credentials (auto-populated if authenticated).
2. Application-specific configuration.
3. Optional features and integrations.
4. Validation of all inputs.
5. Generation of `.env` file.

### dr dotenv edit

Open the `.env` file in an interactive editor or wizard.

```bash
dr dotenv edit
```

**Behavior:**
- If `.env` exists, opens it in the editor.
- If no extra variables are detected, opens text editor mode.
- If template prompts are found, offers wizard mode.
- Can switch between editor and wizard modes.

**Editor mode controls:**
- `e`&mdash;edit in text editor.
- `w`&mdash;switch to wizard mode.
- `Enter`&mdash;save and exit.
- `Esc`&mdash;save and exit.

**Wizard mode controls:**
- Navigate prompts with arrow keys.
- Enter values or select options.
- `Esc`&mdash;return to previous screen.

**Example:**

```bash
cd my-template
dr dotenv edit
```

### dr dotenv update

Automatically refresh DataRobot credentials in the `.env` file.

```bash
dr dotenv update
```

**Features:**
- Updates `DATAROBOT_ENDPOINT` and `DATAROBOT_API_TOKEN`.
- Preserves all other environment variables.
- Automatically authenticates if needed.
- Uses current authentication session.

**Prerequisites:**
- Must be run inside a git repository.
- Must have a `.env` or `.env.template` file.
- Requires authentication with DataRobot.

**Example:**

```bash
cd my-template
dr dotenv update
```

**Use cases:**
- Refresh expired API tokens.
- Switch DataRobot environments.
- Update credentials after re-authentication.

### dr dotenv validate

Validate that all required environment variables are properly configured.

```bash
dr dotenv validate
```

**Features:**
- Validates against template requirements defined in `.datarobot/prompts.yaml`.
- Checks both `.env` file and environment variables.
- Verifies core DataRobot variables (`DATAROBOT_ENDPOINT`, `DATAROBOT_API_TOKEN`).
- Reports missing or invalid variables with helpful error messages.
- Respects conditional requirements based on selected options.

**Prerequisites:**
- Must be run inside a git repository.
- Must have a `.env` file.

**Example:**

```bash
cd my-template
dr dotenv validate
```

**Output:**

Successful validation:
```
Validating required variables:
  APP_NAME: my-app
  DATAROBOT_ENDPOINT: https://app.datarobot.com
  DATAROBOT_API_TOKEN: ***
  DATABASE_URL: postgresql://localhost:5432/db

Validation passed: all required variables are set.
```

Validation errors:
```
Validating required variables:
  APP_NAME: my-app
  DATAROBOT_ENDPOINT: https://app.datarobot.com

Validation errors:

Error: required variable DATAROBOT_API_TOKEN is not set
  Description: DataRobot API token for authentication
  Set this variable in your .env file or run `dr dotenv setup` to configure it.

Error: required variable DATABASE_URL is not set
  Description: PostgreSQL database connection string
  Set this variable in your .env file or run `dr dotenv setup` to configure it.
```

**Use cases:**
- Verify configuration before running tasks.
- Debug missing environment variables.
- CI/CD pipeline checks.
- Troubleshoot application startup issues.

## File structure

### .env.template

Template file committed to version control:

```bash
# Required Configuration
APP_NAME=
DATAROBOT_ENDPOINT=
DATAROBOT_API_TOKEN=

# Optional Configuration
# DEBUG=false
# PORT=8080
```

### .env

Generated configuration file (never committed):

```bash
# Required Configuration
APP_NAME=my-awesome-app
DATAROBOT_ENDPOINT=https://app.datarobot.com
DATAROBOT_API_TOKEN=***

# Optional Configuration
DEBUG=true
PORT=8000
```

## Interactive configuration

### Prompt types

The wizard supports multiple input types defined in `.datarobot/prompts.yaml`:

**Text input:**
```yaml
prompts:
  - key: "app_name"
    env: "APP_NAME"
    help: "Enter your application name"
```

**Secret string:**
```yaml
prompts:
  - key: "api_key"
    env: "API_KEY"
    type: "secret_string"
    help: "Enter your API key"
    generate: true  # Auto-generate a random secret
```

**Single selection:**
```yaml
prompts:
  - key: "environment"
    env: "ENVIRONMENT"
    help: "Select deployment environment"
    options:
      - name: "Development"
        value: "dev"
      - name: "Production"
        value: "prod"
```

**Multiple selection:**
```yaml
prompts:
  - key: "features"
    env: "ENABLED_FEATURES"
    help: "Select features to enable"
    multiple: true
    options:
      - name: "Analytics"
      - name: "Monitoring"
```

### Conditional prompts

Prompts can be shown based on previous selections:

```yaml
prompts:
  - key: "enable_database"
    help: "Enable database?"
    options:
      - name: "Yes"
        requires: "database_config"
      - name: "No"

  - key: "database_url"
    section: "database_config"
    env: "DATABASE_URL"
    help: "Database connection string"
```

## Common workflows

### Initial setup

Set up a new template with all configuration:

```bash
cd my-template
dr dotenv setup
```

### Quick updates

Update just the DataRobot credentials:

```bash
dr dotenv update
```

### Manual editing

Edit variables directly:

```bash
dr dotenv edit
# Press 'e' for editor mode
# Make changes
# Press Enter to save
```

### Validation

Check configuration before running tasks:

```bash
dr dotenv validate
dr run dev
```

### Switch wizard to editor

Start with wizard, switch to editor:

```bash
dr dotenv edit
# Press 'w' for wizard mode
# Complete some prompts
# Press 'e' to switch to editor for fine-tuning
```

## Configuration discovery

The CLI automatically discovers configuration from:

1. **`.env.template`**&mdash;base template with variable names.
2. **`.datarobot/prompts.yaml`**&mdash;interactive prompts and validation.
3. **Existing `.env`**&mdash;current values (if present).
4. **Environment variables**&mdash;system environment (override `.env`).

Priority order (highest to lowest):
1. System environment variables.
2. User input from wizard.
3. Existing `.env` file values.
4. Default values from prompts.
5. Template values from `.env.template`.

## Security

### Secret handling

- Secret values are masked in the UI.
- Variables containing "PASSWORD", "SECRET", "KEY", or "TOKEN" are automatically treated as secrets.
- The `secret_string` prompt type enables secure input with masking.
- `.env` files should never be committed (add to `.gitignore`).

### Auto-generation

Secret strings with `generate: true` are automatically generated:

```yaml
prompts:
  - key: "session_secret"
    env: "SESSION_SECRET"
    type: "secret_string"
    generate: true
    help: "Session encryption key"
```

This generates a cryptographically secure random string when no value exists.

## Error handling

### Not in repository

```
Error: not inside a git repository

Run this command from within an application template git repository.
To create a new template, run `dr templates setup`.
```

**Solution:** Navigate to a git repository or use `dr templates setup`.

### Missing .env file

```
Error: .env file does not exist at /path/to/.env

Run `dr dotenv setup` to create one.
```

**Solution:** Run `dr dotenv setup` to create the file.

### Authentication required

```
Error: not authenticated

Run `dr auth login` to authenticate.
```

**Solution:** Authenticate with `dr auth login`.

### Validation failures

```
Validation errors:

Error: required variable DATABASE_URL is not set
  Description: PostgreSQL database connection string
  Set this variable in your .env file or run `dr dotenv setup` to configure it.
```

**Solution:** Set the missing variables or run `dr dotenv setup`.

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | Success. |
| 1 | Error (file not found, validation failed, not in repo). |
| 130 | Interrupted (Ctrl+C). |

## Examples

### Create configuration from scratch

```bash
cd my-template
dr dotenv setup
```

### Update after re-authentication

```bash
dr auth login
dr dotenv update
```

### Validate before deployment

```bash
dr dotenv validate && dr run deploy
```

### Edit specific variables

```bash
dr dotenv edit
# Press 'e' for editor mode
# Update DATABASE_URL
# Press Enter to save
```

### Check configuration

```bash
cat .env
dr dotenv validate
```

## Integration with other commands

### With templates

```bash
dr templates setup
# Automatically runs dotenv setup
```

### With run

```bash
dr dotenv validate
dr run dev
```

### With auth

```bash
dr auth login
dr dotenv update
```

## See also

- [Environment variables guide](../template-system/environment-variables.md)&mdash;managing `.env` files.
- [Interactive configuration](../template-system/interactive-config.md)&mdash;configuration wizard details.
- [Template structure](../template-system/structure.md)&mdash;template organization.
- [auth command](auth.md)&mdash;authentication management.
- [run command](run.md)&mdash;executing tasks.
