# Environment Variables

Managing environment variables and `.env` files in DataRobot templates.

## Overview

DataRobot templates use `.env` files to store configuration variables needed by your application. The CLI provides tools to:

- Create `.env` files from templates
- Edit variables interactively
- Validate configuration
- Manage secrets securely

## File structure

### .env.template

The template provided by the repository (committed to Git):

```bash
# Required Configuration
APP_NAME=
DATAROBOT_ENDPOINT=
DATAROBOT_API_TOKEN=

# Optional Configuration
# DEBUG=false
# LOG_LEVEL=info
# PORT=8080

# Database Configuration
# DATABASE_URL=
# DATABASE_POOL_SIZE=10

# Cache Configuration
# CACHE_ENABLED=false
# CACHE_URL=
```

**Characteristics:**
- Committed to version control.
- Contains empty required variables.
- Comments indicate optional variables.
- Includes documentation comments.

### .env

The actual configuration file (never committed):

```bash
# Required Configuration
APP_NAME=my-awesome-app
DATAROBOT_ENDPOINT=https://app.datarobot.com
DATAROBOT_API_TOKEN=***

# Optional Configuration
DEBUG=true
LOG_LEVEL=debug
PORT=8000

# Database Configuration
DATABASE_URL=postgresql://localhost:5432/mydb
DATABASE_POOL_SIZE=5
```

**Characteristics:**
- Generated from `.env.template`.
- Contains actual values.
- **Never committed** (in `.gitignore`).
- User-specific configuration.

## Creating environment files

### Using the wizard

The interactive wizard guides you through configuration:

```bash
# In a template directory
dr dotenv setup
```

or

```bash
# During template setup
dr templates setup
```

**Wizard flow:**
1. Loads `.env.template`.
2. Discovers configuration prompts.
3. Shows interactive questions.
4. Validates inputs.
5. Generates `.env` file.

### Manual creation

Copy and edit manually:

```bash
# Copy template
cp .env.template .env

# Edit with your preferred editor
vim .env

# Or use the CLI editor
dr dotenv
```

## Managing Variables

### Interactive Editor

Launch the built-in editor:

```bash
dr dotenv
```

**Features:**
- List all variables
- Mask secrets (passwords, API keys)
- Start wizard mode
- Edit directly

**Commands:**
```
Variables found in .env:

APP_NAME: my-awesome-app
DATAROBOT_ENDPOINT: https://app.datarobot.com
DATAROBOT_API_TOKEN: ***
DEBUG: true

Press w to set up variables interactively.
Press e to edit the file directly.
Press enter to finish and exit.
```

### Wizard Mode

Interactive configuration with prompts:

```bash
dr dotenv setup
```

**Advantages:**
- Guided setup
- Validation built-in
- Conditional prompts
- Help text for each variable

### Direct Editing

Edit the file directly:

```bash
dr dotenv edit
# Press 'e' to enter editor mode

# Or use external editor
vim .env
```

## Variable Types

### Required Variables

Must be set before running the application:

```bash
# .env.template shows these without comments
APP_NAME=
DATAROBOT_ENDPOINT=
DATAROBOT_API_TOKEN=
```

The wizard enforces that these are filled:

```
Enter your application name
> _
(Cannot proceed without entering a value)
```

### Optional Variables

Can be left empty (shown as comments):

```bash
# .env.template shows these with # prefix
# DEBUG=false
# LOG_LEVEL=info
```

The wizard allows skipping:

```
Enable debug mode? (optional)
  > None (leave blank)
    Yes
    No
```

### Secret Variables

Sensitive values that should be masked during input and display.

**Defining secret variables:**

```yaml
# In .datarobot/prompts.yaml
prompts:
  - key: "api_key"
    env: "API_KEY"
    type: "secret_string"
    help: "Enter your API key"
```

**Auto-detection:** Variables with names containing `PASSWORD`, `SECRET`, `KEY`, or `TOKEN` are automatically treated as secrets.

**Display behavior:**
- Wizard input is masked with bullet characters (••••).
- Editor view shows as `***`.
- Actual file contains plain text value.

**Security best practices:**
- Always add `.env` to `.gitignore`.
- Use `secret_string` type for all sensitive values.
- Never commit `.env` files to version control.

### Auto-generated Secrets

Cryptographically secure random values for application secrets:

```yaml
prompts:
  - key: "session_secret"
    env: "SESSION_SECRET"
    type: "secret_string"
    generate: true
    help: "Session encryption key (auto-generated)"
```

**Features:**
- Generates 32-character random string if no value exists.
- Uses base64 URL-safe encoding.
- Preserves existing values (only generates when empty).
- User can override with custom value.

### Conditional Variables

Only shown/required based on other selections:

```yaml
# In .datarobot/prompts.yaml
prompts:
  - key: "enable_database"
    options:
      - name: "Yes"
        requires: "database_config"
      - name: "No"

  - key: "database_url"
    section: "database_config"
    env: "DATABASE_URL"
    help: "Database connection string"
```

If "Enable database" = No, then `DATABASE_URL` is not shown.

## Environment Variable Discovery

The CLI discovers variables from multiple sources:

### 1. Template File (.env.template)

```bash
# Variables defined in template
APP_NAME=
PORT=8080
```

### 2. Prompt Definitions (.datarobot/prompts.yaml)

```yaml
prompts:
  - key: "app_name"
    env: "APP_NAME"
    help: "Application name"
```

### 3. Existing .env File

```bash
# Previously configured values
APP_NAME=my-app
```

### 4. Current Environment

```bash
# Shell environment variables
export PORT=3000
```

### Merge Priority

The CLI merges these in order (highest priority first):

1. User input from wizard
2. Current shell environment
3. Existing `.env` values
4. Template defaults

## Common Patterns

### Database Configuration

```bash
# PostgreSQL
DATABASE_URL=postgresql://user:password@localhost:5432/dbname
DATABASE_POOL_SIZE=10
DATABASE_TIMEOUT=30

# MySQL
DATABASE_URL=mysql://user:password@localhost:3306/dbname

# MongoDB
DATABASE_URL=mongodb://localhost:27017/dbname
```

### Authentication

```bash
# API Key
DATAROBOT_API_TOKEN=your_api_token_here

# OAuth
AUTH_PROVIDER=oauth2
AUTH_CLIENT_ID=client_id
AUTH_CLIENT_SECRET=***
AUTH_REDIRECT_URL=http://localhost:8080/callback

# JWT
JWT_SECRET=***
JWT_EXPIRATION=3600
```

### Feature Flags

```bash
# Enable/disable features
FEATURE_ANALYTICS=true
FEATURE_MONITORING=false
FEATURE_CACHING=true

# Or as comma-separated list
ENABLED_FEATURES=analytics,caching
```

### Logging

```bash
# Log level
LOG_LEVEL=debug  # debug, info, warn, error

# Log format
LOG_FORMAT=json  # json, text

# Log output
LOG_OUTPUT=stdout  # stdout, file

# Log file path
LOG_FILE=/var/log/app.log
```

## Security Best Practices

### 1. Never Commit .env Files

Ensure `.gitignore` includes:

```gitignore
# Environment variables
.env
.env.local
.env.*.local

# Keep templates
!.env.template
!.env.example
```

### 2. Use Strong Secrets

```bash
# ✓ Good - strong random secret
JWT_SECRET=a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6

# ✗ Bad - weak secret
JWT_SECRET=secret123
```

Generate secure secrets:

```bash
# Random 32-byte hex string
openssl rand -hex 32
```

### 3. Restrict File Permissions

```bash
# Only owner can read/write
chmod 600 .env

# Verify
ls -la .env
# Should show: -rw------- (600)
```

### 4. Use Different Configs Per Environment

```bash
# Development
.env.development

# Staging
.env.staging

# Production
.env.production
```

Load based on environment:

```bash
export ENV=production
dr run deploy
```

### 5. Avoid Hardcoding in Code

```python
# ✗ Bad
api_token = "abc123"

# ✓ Good
import os
api_token = os.getenv("DATAROBOT_API_TOKEN")
```

## Validation

### Using dr dotenv validate

Validate your environment configuration against template requirements:

```bash
dr dotenv validate
```

**Validates:**
- All required variables defined in `.datarobot/prompts.yaml`.
- Core DataRobot variables (`DATAROBOT_ENDPOINT`, `DATAROBOT_API_TOKEN`).
- Conditional requirements based on selected options.
- Both `.env` file and environment variables.

**Example output:**

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
- Pre-flight checks before running tasks.
- CI/CD pipeline validation.
- Debugging missing configuration.
- Troubleshooting application startup issues.

### Required Variables Check

Commands like `dr run` automatically validate required variables:

```bash
$ dr run dev
Error: Missing required environment variables:
  - APP_NAME
  - DATAROBOT_API_TOKEN

Please run: dr dotenv setup
```

### Format Validation

For variables with specific formats:

```bash
# URL validation
DATAROBOT_ENDPOINT=https://app.datarobot.com  # ✓ Valid
DATAROBOT_ENDPOINT=not-a-url                   # ✗ Invalid

# Port validation
PORT=8080    # ✓ Valid
PORT=99999   # ✗ Invalid (out of range)

# Email validation
EMAIL=user@example.com  # ✓ Valid
EMAIL=invalid           # ✗ Invalid
```

## Advanced Features

### Variable Substitution

Reference other variables:

```bash
# Base URL
BASE_URL=https://app.datarobot.com

# API endpoint uses base URL
API_ENDPOINT=${BASE_URL}/api/v2

# Full URL becomes: https://app.datarobot.com/api/v2
```

### Multi-line Values

For long values:

```bash
# Single line
PRIVATE_KEY="-----BEGIN PRIVATE KEY-----\nMIIE..."

# Or use actual newlines
PRIVATE_KEY="-----BEGIN PRIVATE KEY-----
MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQC...
-----END PRIVATE KEY-----"
```

### Comments

Document your configuration:

```bash
# Application Configuration
APP_NAME=my-app          # Application identifier
PORT=8080                # HTTP server port

# Database Configuration
# Format: protocol://user:password@host:port/database
DATABASE_URL=postgresql://localhost:5432/mydb
```

## Troubleshooting

### Variables Not Loading

```bash
# Check .env exists
ls -la .env

# Verify format
cat .env

# Check for syntax errors
# Each line should be: KEY=value
```

### Secrets Exposed

```bash
# Check .gitignore includes .env
cat .gitignore | grep .env

# Check Git status
git status
# Should NOT show .env

# If .env is tracked, remove it
git rm --cached .env
git commit -m "Remove .env from tracking"
```

### Permission Errors

```bash
# Fix permissions
chmod 600 .env

# Verify
ls -la .env
```

### Variables Not Expanding

```bash
# Ensure proper syntax for variable substitution
# Works:
API_URL=${BASE_URL}/api

# Doesn't work:
API_URL=$BASE_URL/api  # Missing braces
```

### Configuration Not Working

Use `dr dotenv validate` to diagnose issues:

```bash
# Validate configuration
dr dotenv validate

# If validation passes but issues persist, check:
# 1. Environment variables override .env
env | grep DATAROBOT

# 2. Ensure .env is in correct location (repository root)
pwd
ls -la .env

# 3. Check if application is loading .env file
# Some applications need explicit .env loading
```

## Common workflows

### Initial setup

```bash
cd my-template
dr dotenv setup
dr dotenv validate
dr run dev
```

### Update credentials

```bash
dr auth login
dr dotenv update
dr dotenv validate
```

### Validate before deployment

```bash
dr dotenv validate && dr run deploy
```

### Edit and validate

```bash
dr dotenv edit
dr dotenv validate
```

## See also

- [Interactive configuration](interactive-config.md)&mdash;configuration wizard details.
- [Template structure](structure.md)&mdash;template organization.
- [dotenv command](../commands/dotenv.md)&mdash;dotenv command reference.
