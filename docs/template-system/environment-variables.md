# Environment variables

This page outlines how to manage environment variables and `.env` files in DataRobot templates. DataRobot templates use `.env` files to store configuration variables needed by your application. The CLI provides tools to:

- Create `.env` files from templates
- Interactively edit variables
- Validate configuration
- Securely manage secrets

## File structure

### .env.template

The template provided by the repository (committed to Git):

```bash
# Required configuration
APP_NAME=
DATAROBOT_ENDPOINT=
DATAROBOT_API_TOKEN=

# Optional configuration
# DEBUG=false
# LOG_LEVEL=info
# PORT=8080

# Database configuration
# DATABASE_URL=
# DATABASE_POOL_SIZE=10

# Cache configuration
# CACHE_ENABLED=false
# CACHE_URL=
```

#### Characteristics

- Committed to version control
- Contains empty required variables
- Comments indicate optional variables
- Includes documentation comments

### .env

The actual configuration file (never committed):

```bash
# Required configuration
APP_NAME=my-awesome-app
DATAROBOT_ENDPOINT=https://app.datarobot.com
DATAROBOT_API_TOKEN=***

# Optional configuration
DEBUG=true
LOG_LEVEL=debug
PORT=8000

# Database configuration
DATABASE_URL=postgresql://localhost:5432/mydb
DATABASE_POOL_SIZE=5
```

**Characteristics:**
- Generated from `.env.template`.
- Contains actual values.
- **Never committed** (in `.gitignore`).
- User-specific configuration.

## Create environment files

### Use the wizard

The interactive wizard guides you through configuration.

```bash
# In a template directory
dr dotenv setup
```

or

```bash
# During template setup
dr templates setup
```

#### Wizard workflow

1. Loads `.env.template`.
2. Discovers configuration prompts.
3. Shows interactive questions.
4. Validates inputs.
5. Generates an `.env` file.

### Manual creation

To copy and edit a template manually:

```bash
# Copy the template
cp .env.template .env

# Edit the template with your preferred editor
vim .env

# Alternatively, use the CLI editor
dr dotenv
```

## Manage variables

### Interactive editor

Launch the built-in editor to manage variables:

```bash
dr dotenv
```

#### Features

- List all variables
- Mask secrets (passwords, API keys)
- Start wizard mode
- Directly edit variables

#### Commands

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

### Wizard mode

You can also interactively configure a template with prompts.

```bash
dr dotenv setup
```

#### Advantages

- Guided setup
- Built-in validation
- Conditional prompts
- Help text for each variable

### Direct editing

To edit the file directly:

```bash
dr dotenv edit
# Press 'e' to enter editor mode

# Or use external editor
vim .env
```

## Variable types

### Required variables

The following variables must be set before running the application:

```bash
# .env.template shows these without comments
APP_NAME=
DATAROBOT_ENDPOINT=
DATAROBOT_API_TOKEN=
```

The wizard enforces that an application name must be provided.

```
Enter your application name
> _
(Cannot proceed without entering a value)
```

### Optional variables

The following variables are optional and can be left empty (shown as comments):

```bash
# .env.template shows these with # prefix
# DEBUG=false
# LOG_LEVEL=info
```

The wizard allows you to skip binding these variables:

```
Enable debug mode? (optional)
  > None (leave blank)
    Yes
    No
```

### Secret variables

Sensitive values that should be masked during input and display.

To define secret variables:

```yaml
# In .datarobot/prompts.yaml
prompts:
  - key: "api_key"
    env: "API_KEY"
    type: "secret_string"
    help: "Enter your API key"
```
#### Auto-detection

Variables with names containing `PASSWORD`, `SECRET`, `KEY`, or `TOKEN` are automatically treated as secrets.

#### Display behavior

- The wizard input's secrets are masked with bullet characters (••••).
- The editor view displays secrets as `***`.
- The actual file contains secrets as plain text values.

#### Security best practices

- Always add `.env` to `.gitignore`.
- Use `secret_string` type for all sensitive values.
- Never commit `.env` files to version control.

### Auto-generated secrets

You can cryptographically secure random values for application secrets:

```yaml
prompts:
  - key: "session_secret"
    env: "SESSION_SECRET"
    type: "secret_string"
    generate: true
    help: "Session encryption key (auto-generated)"
```

#### Features

- Generates 32-character random string if no value exists.
- Uses base64 URL-safe encoding.
- Preserves existing values (only generates when empty).
- User can override secrets with a custom value.

### Conditional variables

These variables are only shown or required based on your other selections:

```yaml
# In .datarobot/prompts.yaml
prompts:
  - key: "enable_database"
    options:
      - name: "Yes"
        requires: "database_config"
      - name: "No"

database_config:
  - env: "DATABASE_URL"
    help: "Database connection string"
```

If `Enable database = No`, then `DATABASE_URL` is not shown.

## Environment variable discovery

The CLI discovers variables from multiple sources:

### 1. Template file (.env.template)

```bash
# Variables defined in template
APP_NAME=
PORT=8080
```

### 2. Prompt definitions (.datarobot/prompts.yaml)

```yaml
prompts:
  - key: "app_name"
    env: "APP_NAME"
    help: "Application name"
```

### 3. Existing .env file

```bash
# Previously configured values
APP_NAME=my-app
```

### 4. Current environment

```bash
# Shell environment variables
export PORT=3000
```

### Merge priority

The CLI merges in the following order of priority (highest priority first):

1. User input from wizard.
2. Current shell environment.
3. Existing `.env` values.
4. Template defaults.

## Common patterns

### Database configuration

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

### Feature flags

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

## Security best practices

### Never commit .env files

Ensure that `.gitignore` includes:

```gitignore
# Environment variables
.env
.env.local
.env.*.local

# Keep templates
!.env.template
!.env.example
```

### Use strong secrets

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

### Restrict file permissions

```bash
# Only the owner can read/write
chmod 600 .env

# Verify
ls -la .env
# Should show: -rw------- (600)
```

### Use different configs per environment

```bash
# Development
.env.development

# Staging
.env.staging

# Production
.env.production
```

Load based on the environment:

```bash
export ENV=production
dr run deploy
```

### Avoid hardcoding in code

```python
# ✗ Bad
api_token = "abc123"

# ✓ Good
import os
api_token = os.getenv("DATAROBOT_API_TOKEN")
```

## Validation

### Validate with dr dotenv

To validate your environment configuration against template requirements:

```bash
dr dotenv validate
```

This command validates the following:

- All required variables defined in `.datarobot/prompts.yaml`.
- Core DataRobot variables (`DATAROBOT_ENDPOINT`, `DATAROBOT_API_TOKEN`).
- Conditional requirements based on selected options.
- Both `.env` file and environment variables.

#### Example output

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

#### Use cases

- Pre-flight checks before running tasks.
- CI/CD pipeline validation.
- Debugging missing configuration.
- Troubleshooting application startup issues.

### Required variables check

Commands like `dr run` automatically validate required variables.

```bash
$ dr run dev
Error: Missing required environment variables:
  - APP_NAME
  - DATAROBOT_API_TOKEN

Please run: dr dotenv setup
```

### Format validation

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

## Advanced features

### Variable substitution

Reference other variables:

```bash
# Base URL
BASE_URL=https://app.datarobot.com

# API endpoint uses base URL
API_ENDPOINT=${BASE_URL}/api/v2

# Full URL becomes: https://app.datarobot.com/api/v2
```

### Multi-line values

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

### Variables not loading

```bash
# Check .env exists
ls -la .env

# Verify format
cat .env

# Check for syntax errors
# Each line should be: KEY=value
```

### Secrets exposed

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

### Permission errors

```bash
# Fix permissions
chmod 600 .env

# Verify
ls -la .env
```

### Variables not expanding

```bash
# Ensure proper syntax for variable substitution
# Works:
API_URL=${BASE_URL}/api

# Doesn't work:
API_URL=$BASE_URL/api  # Missing braces
```

### Configuration not working

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

- [Interactive configuration](interactive-config.md): Configuration wizard details.
- [Template structure](structure.md): Template organization.
- [dotenv command](../commands/dotenv.md): dotenv command reference.
