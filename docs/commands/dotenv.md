# `dr dotenv` - Environment variable management

Manage environment variables and `.env` files in DataRobot templates.

## Quick start

For most users, setting up environment variables is a single command:

```bash
# Interactive wizard guides you through all configuration
dr dotenv setup
```

The wizard automatically discovers your template's requirements and prompts you for all necessary values. Your credentials are saved securely and you're ready to use the CLI.

> [!NOTE]
> **First time?** If you're new to the CLI, start with the [Quick start](https://github.com/datarobot-oss/cli/blob/main/README.md#quick-start) for step-by-step setup instructions.

## Synopsis

```bash
dr dotenv <command> [flags]
```

## Description

The `dr dotenv` command provides tools for creating, editing, validating, and updating environment configuration files. It includes an interactive wizard for guided setup and a text editor for direct file manipulation.

## Subcommands

### dr dotenv setup

Launch the interactive wizard to configure environment variables.

```bash
dr dotenv setup [--if-needed]
```

**Features:**

- Interactive prompts for all required variables.
- Context-aware questions based on template configuration.
- Automatic discovery of prompts from every YAML file under the repository's `.datarobot/` directory.
- Smart defaults from the prompt definitions.
- Secure handling of secret values.
- DataRobot authentication integration.
- Automatic state tracking of completion timestamp.
- Conditional execution with `--if-needed` flag.

**Prerequisites:**

- Must be run inside a git repository.
- Requires authentication with DataRobot.

**Flags:**

- `--if-needed`&mdash;Only run setup if the `.env` file doesn't exist or is missing required variables. This flag is useful for automation scripts and CI/CD pipelines where you want to ensure configuration exists without prompting if it's already valid. The check reads the `.env` file only; values that happen to be set in your shell environment do not satisfy it.
- `-y, --yes`&mdash;Skip interactive prompts and auto-populate all environment variables with their default values (or empty strings if no default is provided). This is useful for CI/CD pipelines, automated testing, or quick development setup where you want to use all defaults. Variables with `generate: true` will still have random values auto-generated. Can also be enabled via `DATAROBOT_CLI_NON_INTERACTIVE=true` environment variable.
- `-a, --all`&mdash;Show all prompts in the wizard, including those already answered by a default value. Mutually exclusive with `--yes`.
- `-o, --output <directory>`&mdash;Specify a custom directory where the `.env` file should be written. The directory is created automatically if it doesn't exist. By default, the `.env` file is written to the repository root. This option skips the repository check and directory up-walk, which means the directory you pass also becomes the root that is scanned for `.datarobot/` prompt files. When it is set, the setup completion timestamp is not recorded.

**State tracking:**

Upon successful completion, `dr dotenv setup` records the timestamp in the state file. This allows `dr templates setup` to intelligently skip dotenv configuration if it has already been completed.

The state is stored in the same location as other CLI state (see [Configuration - State tracking](../user-guide/configuration.md#state-tracking)). Keep in mind that manually running `dr dotenv setup` always prompts for configuration, regardless of state.

To force the setup wizard to run again (ignoring the state file), use the `--force-interactive` flag:

```bash
dr templates setup --force-interactive
```

This is useful for testing or when you need to reconfigure your environment from scratch.

**Examples:**

Standard setup:
```bash
cd my-template
dr dotenv setup
```

Conditional setup (skip if already configured):
```bash
cd my-template
dr dotenv setup --if-needed
# Output: "Configuration already exists, skipping setup." (if valid)
# Or: launches wizard (if missing or invalid)
```

Auto-populate with defaults (no interaction):
```bash
cd my-template
dr dotenv setup --yes
# Creates .env file with all defaults (or empty values), skips wizard entirely
```

Or using environment variable:
```bash
cd my-template
DATAROBOT_CLI_NON_INTERACTIVE=true dr dotenv setup
```

Custom output directory:
```bash
cd my-template
dr dotenv setup --output /path/to/config
# Creates .env file at /path/to/config/.env (directory created if needed)
```

The wizard guides you through:

1. DataRobot credentials (auto-populated if authenticated).
2. Application-specific configuration.
3. Optional features and integrations.
4. Validation of all inputs.
5. Generation of `.env` file.

**How `--if-needed` works:**

When the `--if-needed` flag is set, the command validates your existing `.env` file against every required prompt discovered under `.datarobot/`:

- ✅ **Skips setup** if `.env` exists and every required variable is set in that file (including the core DataRobot variables and all template-specific variables).
- ⚠️ **Runs setup** if `.env` doesn't exist.
- ⚠️ **Runs setup** if any required variable is missing, empty, or commented out.

> [!IMPORTANT]
> The skip decision deliberately ignores your shell environment. A variable exported in your shell but absent from `.env` still triggers the wizard, so an incomplete `.env` is never left in place. Prompt `default:` values are also ignored for this check&mdash;only values actually written to the file count.

This makes `--if-needed` ideal for:

- **Automation scripts** that need to ensure configuration without user interaction.
- **CI/CD pipelines** that should only prompt when necessary.
- **Onboarding workflows** that intelligently skip already-completed steps.
- **Idempotent operations** that can be safely run multiple times.

**How `--yes` works:**

When the `--yes` flag is set, the command skips the interactive wizard entirely and auto-populates all environment variables:

- ✅ **Uses default values** for all prompts that have a `default:` specified in the YAML configuration.
- ✅ **Sets empty strings** for prompts without defaults (unless they already have values from environment or existing `.env`).
- ✅ **Auto-generates secrets** for prompts with `generate: true` (e.g., session secrets, encryption keys).
- ✅ **Preserves existing values** from environment variables or the existing `.env` file.

This makes `--yes` ideal for:
- **CI/CD pipelines** that need predictable, non-interactive setup.
- **Development environments** where defaults are sufficient for local testing.
- **Automated testing** where you want a quick scaffold of all variables.
- **Template initialization** in contexts where manual configuration comes later.

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

- Validates against every prompt discovered under `.datarobot/`.
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

```text
Validating required variables:
  APP_NAME: my-app
  DATAROBOT_ENDPOINT: https://app.datarobot.com
  DATAROBOT_API_TOKEN: ***
  DATABASE_URL: postgresql://localhost:5432/db

Validation passed: all required variables are set.
```

Validation errors:

```text
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

```env
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

```env
# Required Configuration
APP_NAME=my-awesome-app
DATAROBOT_ENDPOINT=https://app.datarobot.com
DATAROBOT_API_TOKEN=***

# Optional Configuration
DEBUG=true
PORT=8000
```

## Prompt definition files

The wizard is driven by prompt definition files that live under the `.datarobot/`
directory at the root of your repository.

### Discovery

There is no single, fixed prompt file. The CLI walks the `.datarobot/` directory
and collects **every** `*.yaml` and `*.yml` file it finds, at any nesting level:

```text
my-template/
├── .datarobot/
│   ├── prompts.yaml           # discovered
│   ├── llm.yml                # discovered
│   └── components/
│       ├── backend.yaml       # discovered
│       └── frontend/
│           └── prompts.yaml   # discovered
└── .env
```

Discovery rules:

| Rule | Behavior |
|------|----------|
| Root | Only `.datarobot/` at the repository root (or at `--output`) is scanned. |
| Pattern | Any file matching `*.yaml` or `*.yml`. Filenames are not significant. |
| Recursion | Subdirectories are walked, up to 5 levels below `.datarobot/`. |
| Hidden directories | Skipped, apart from `.datarobot` itself. |
| Ordering | Files are sorted by full path, then processed in that order. |
| Non-prompt YAML | Files that don't match the prompt schema are silently skipped, so copier answer files, version manifests, and other config can live alongside prompts. |
| Duplicates | If two files define the same `env` (or `key`), the first one in path order wins and the rest are ignored. |

> [!TIP]
> Because non-conforming YAML is skipped rather than rejected, a malformed prompt
> file fails quietly. Run with `--debug` to see which files were parsed and which
> were skipped.

### File schema

A prompt file is a mapping of **section names** to lists of prompts. Section
names are arbitrary&mdash;`prompts` is a convention, not a keyword:

```yaml
# Top-level keys are section names.
app_config:
  - env: "APP_NAME"
    help: "Enter your application name"
```

A section is a **root section** unless some option's `requires` field names it;
root sections are always active, and the rest are activated conditionally. Root
sections are processed in alphabetical order.

> [!NOTE]
> Sections and `requires` references are scoped to the file that declares them.
> A `requires` in one file cannot activate a section in another file.

Every prompt supports the following fields:

| Field | Type | Description |
|-------|------|-------------|
| `env` | string | Environment variable to write to `.env`. Required unless `key` is given. |
| `key` | string | Identifier used when the prompt does not map to an environment variable. Written to `.env` as a comment. |
| `help` | string | Description shown to the user and emitted as a comment above the variable. Strongly recommended. |
| `type` | string | `string` (default) or `secret_string` for masked input. See also `llmgw_catalog` below. |
| `default` | string | Initial value. A prompt already sitting at its default is not shown unless `always_prompt` or `--all` is set. |
| `optional` | bool | Allows the prompt to be left blank. |
| `multiple` | bool | Renders `options` as checkboxes; selections are stored comma-separated. |
| `generate` | bool | Auto-generates a random value when empty. Only applies to `secret_string`. |
| `always_prompt` | bool | Shows the prompt even when its default already answers it. |
| `options` | list | Choices for a selection prompt. |

Each entry in `options` supports:

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Text shown in the list. |
| `value` | string | Value stored in `.env`. Defaults to `name`. |
| `requires` | string | Section in the same file to activate when this option is selected. |

## Interactive configuration

### Prompt types

**Text input:**

```yaml
app_config:
  - env: "APP_NAME"
    help: "Enter your application name"
```

**Secret string:**

```yaml
app_config:
  - env: "API_KEY"
    type: "secret_string"
    help: "Enter your API key"
    generate: true  # Auto-generate a random secret
```

**Single selection:**

```yaml
app_config:
  - env: "ENVIRONMENT"
    help: "Select deployment environment"
    options:
      - name: "Development"
        value: "dev"
      - name: "Production"
        value: "prod"
```

**Multiple selection:**

```yaml
app_config:
  - env: "ENABLED_FEATURES"
    help: "Select features to enable"
    multiple: true
    options:
      - name: "Analytics"
      - name: "Monitoring"
```

**LLM from LLM gateway:**

```yaml
app_config:
  - env: "LLM_GATEWAY_MODEL"
    type: "llmgw_catalog"
    optional: false
    help: "Choose LLM from LLM Gateway catalog."
```

The `llmgw_catalog` type fetches the live model catalog from your DataRobot
instance and renders it as a single-selection list.

### Conditional prompts

Prompts can be shown based on previous selections:

```yaml
app_config:
  - key: "enable_database"
    help: "Enable database?"
    options:
      - name: "Yes"
        requires: "database_config"
      - name: "No"

database_config:
  - env: "DATABASE_URL"
    help: "Database connection string"
```

`database_config` is not a root section, because it is named by a `requires`,
so its prompts appear only when the user picks "Yes". A prompt with any
`requires` option is always shown, even if it has a default.

## Common workflows

### Initial setup

Set up a new template with all configuration:

```bash
cd my-template
dr dotenv setup
```

### Automated/idempotent setup

Ensure configuration exists without unnecessary prompts (useful in scripts):

```bash
cd my-template
dr dotenv setup --if-needed
```

This will:
- Skip the wizard if configuration is already valid.
- Run the wizard only if configuration is missing or incomplete.

**Use case example - CI/CD pipeline:**

```bash
#!/bin/bash
# Ensure environment is configured before running tests
dr dotenv setup --if-needed
dr run test
```

**Use case example - Onboarding script:**

```bash
#!/bin/bash
# Multi-step setup that can be safely re-run
git clone https://github.com/myorg/my-app
cd my-app
npm install
dr dotenv setup --if-needed  # Only prompts if needed
dr run dev
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

The CLI assembles configuration from:

1. **Core DataRobot prompts**&mdash;`DATAROBOT_ENDPOINT` and `DATAROBOT_API_TOKEN`, always present and filled from your authenticated session.
2. **`.datarobot/**/*.{yaml,yml}`**&mdash;every prompt definition file in the repository, as described in [Prompt definition files](#prompt-definition-files).
3. **Existing `.env`**&mdash;current values, if present. If `.env` doesn't exist, `.env.template` is read instead as the starting point.
4. **Environment variables**&mdash;system environment.

Priority order for a prompt's initial value (highest to lowest):

1. System environment variable of the same name.
2. Value from `.env` (or `.env.template` when `.env` doesn't exist yet).
3. The prompt's `default:` value.
4. A generated value, for `secret_string` prompts with `generate: true`.

User input in the wizard overrides whatever was resolved above.

> [!NOTE]
> `PULUMI_CONFIG_PASSPHRASE` is an exception: it also falls back to the
> `pulumi_config_passphrase` key in `drconfig.yaml`.

## Security

### Secret handling

- Prompts declared as `type: secret_string` are masked with bullets during input.
- Masking is driven by the prompt's declared type, not by the variable's name. A variable named `MY_API_KEY` is *not* masked unless its prompt declares `type: secret_string`.
- `DATAROBOT_API_TOKEN` is masked when listed by `dr dotenv edit` and `dr dotenv validate`.
- Secrets are stored as plain text in `.env`.

> [!WARNING]
> `.env` files should never be committed. To ensure this, add it to `.gitignore`.

### Auto-generation

Secret strings with `generate: true` are automatically generated:

```yaml
app_config:
  - env: "SESSION_SECRET"
    type: "secret_string"
    generate: true
    help: "Session encryption key"
```

This generates a 32-character, base64 URL-safe random string when no value
exists. Existing values are never overwritten, and generated prompts are not
shown in the wizard unless `--all` is passed.

## Error handling

### Not in repository

```text
Error: not inside a git repository

Run this command from within an application template git repository.
To create a new template, run `dr templates setup`.
```

**Solution:** Navigate to a git repository or use `dr templates setup`.

### Missing .env file

```text
Error: .env file does not exist at /path/to/.env

Run `dr dotenv setup` to create one.
```

**Solution:** Run `dr dotenv setup` to create the file.

### Authentication required

```text
Error: not authenticated

Run `dr auth login` to authenticate.
```

**Solution:** Authenticate with `dr auth login`.

### Validation failures

```text
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
