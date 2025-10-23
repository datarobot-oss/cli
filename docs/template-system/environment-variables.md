# Environment Variables

Managing environment variables and `.env` files in DataRobot templates.

## Overview

DataRobot templates use `.env` files to store configuration variables needed by your application. The CLI provides tools to:

- Create `.env` files from templates
- Edit variables interactively
- Validate configuration
- Manage secrets securely

## File Structure

### .env.template

The template provided by the repository (committed to Git):

```bash
# Required Configuration
APP_NAME=
DATAROBOT_ENDPOINT=
DATAROBOT_API_KEY=

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
- Committed to version control
- Contains empty required variables
- Comments indicate optional variables
- Includes documentation comments

### .env

The actual configuration file (never committed):

```bash
# Required Configuration
APP_NAME=my-awesome-app
DATAROBOT_ENDPOINT=https://app.datarobot.com
DATAROBOT_API_KEY=***

# Optional Configuration
DEBUG=true
LOG_LEVEL=debug
PORT=8000

# Database Configuration
DATABASE_URL=postgresql://localhost:5432/mydb
DATABASE_POOL_SIZE=5
```

**Characteristics:**
- Generated from `.env.template`
- Contains actual values
- **Never committed** (in `.gitignore`)
- User-specific configuration

## Creating Environment Files

### Using the Wizard

The interactive wizard guides you through configuration:

```bash
# In a template directory
dr dotenv --wizard
```

or

```bash
# During template setup
dr templates setup
```

**Wizard Flow:**
1. Loads `.env.template`
2. Discovers configuration prompts
3. Shows interactive questions
4. Validates inputs
5. Generates `.env` file

### Manual Creation

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
DATAROBOT_API_KEY: ***
DEBUG: true

Press w to set up variables interactively.
Press e to edit the file directly.
Press enter to finish and exit.
```

### Wizard Mode

Interactive configuration with prompts:

```bash
dr dotenv --wizard
```

**Advantages:**
- Guided setup
- Validation built-in
- Conditional prompts
- Help text for each variable

### Direct Editing

Edit the file directly:

```bash
dr dotenv
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
DATAROBOT_API_KEY=
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

Automatically detected by name patterns:
- `*PASSWORD*`
- `*SECRET*`
- `*KEY*`
- `*TOKEN*`

**Displayed as masked:**
```
DATAROBOT_API_KEY: ***
DATABASE_PASSWORD: ***
JWT_SECRET: ***
```

**Editor shows values:**
```
DATAROBOT_API_KEY=abc123def456...█
```

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
DATAROBOT_API_KEY=your_api_key_here

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
api_key = "abc123"

# ✓ Good
import os
api_key = os.getenv("DATAROBOT_API_KEY")
```

## Validation

### Required Variables Check

The CLI validates that required variables are set:

```bash
$ dr run dev
Error: Missing required environment variables:
  - APP_NAME
  - DATAROBOT_API_KEY

Please run: dr dotenv --wizard
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

## See Also

- [Interactive Configuration](../template-system/interactive-config.md) - Configuration wizard
- [Template Structure](../template-system/structure.md) - Template organization
- [dotenv command](../commands/dotenv.md) - dotenv command reference
