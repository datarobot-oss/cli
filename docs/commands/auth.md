# `dr auth` - Authentication Management

Manage authentication with DataRobot.

## Synopsis

```bash
dr auth <command> [flags]
```

## Description

The `auth` command provides authentication management for the DataRobot CLI. It handles login, logout, and URL configuration for connecting to your DataRobot instance.

## Commands

### `login`

Authenticate with DataRobot using OAuth.

```bash
dr auth login
```

**Behavior:**
1. Starts a local web server (typically on port 8080)
2. Opens your default browser to DataRobot's OAuth page
3. Prompts you to authorize the CLI
4. Receives and stores the API key
5. Closes the browser and server automatically

**Example:**
```bash
$ dr auth login
Opening browser for authentication...
Waiting for authentication...
✓ Successfully authenticated!
```

**Stored Credentials:**
- Location: `~/.datarobot/config.yaml` (Linux/macOS) or `%USERPROFILE%\.datarobot\config.yaml` (Windows)
- Format: Encrypted API key

**Troubleshooting:**
```bash
# If browser doesn't open automatically
# The CLI will display a URL to visit manually:
$ dr auth login
Failed to open browser automatically.
Please visit: https://app.datarobot.com/oauth/authorize?client_id=...

# Port already in use
# The CLI will try alternative ports automatically
```

### `logout`

Remove stored authentication credentials.

```bash
dr auth logout
```

**Example:**
```bash
$ dr auth logout
✓ Successfully logged out
```

**Effect:**
- Removes API key from config file
- Keeps DataRobot URL configuration
- Next API call will require re-authentication

### `set-url`

Configure the DataRobot instance URL.

```bash
dr auth set-url [url]
```

**Arguments:**
- `url` (optional) - DataRobot instance URL

**Interactive Mode:**

If no URL is provided, enters interactive mode:

```bash
$ dr auth set-url
Please specify your DataRobot URL, or enter the numbers 1 - 3 if you are using that multi tenant cloud offering
Please enter 1 if you're using https://app.datarobot.com
Please enter 2 if you're using https://app.eu.datarobot.com
Please enter 3 if you're using https://app.jp.datarobot.com
Otherwise, please enter the URL you use

> _
```

**Direct Mode:**

Specify URL directly:

```bash
# Using cloud shortcuts
$ dr auth set-url 1          # Sets to https://app.datarobot.com
$ dr auth set-url 2          # Sets to https://app.eu.datarobot.com
$ dr auth set-url 3          # Sets to https://app.jp.datarobot.com

# Using full URL
$ dr auth set-url https://app.datarobot.com
$ dr auth set-url https://my-company.datarobot.com
```

**Validation:**
```bash
$ dr auth set-url invalid-url
Error: Invalid URL format
```

## Global Flags

These flags work with all `auth` commands:

```bash
  -v, --verbose    Enable verbose output
      --debug      Enable debug output
  -h, --help       Show help for command
```

## Examples

### Initial Setup

```bash
# Set URL and login (recommended workflow)
$ dr auth set-url https://app.datarobot.com
✓ DataRobot URL set to: https://app.datarobot.com

$ dr auth login
Opening browser for authentication...
✓ Successfully authenticated!
```

### Using Cloud Instance Shortcuts

```bash
# US Cloud
$ dr auth set-url 1
$ dr auth login

# EU Cloud
$ dr auth set-url 2
$ dr auth login

# Japan Cloud
$ dr auth set-url 3
$ dr auth login
```

### Self-Managed Instance

```bash
$ dr auth set-url https://datarobot.mycompany.com
$ dr auth login
```

### Re-authentication

```bash
# Logout and login again
$ dr auth logout
✓ Successfully logged out

$ dr auth login
Opening browser for authentication...
✓ Successfully authenticated!
```

### Switching Instances

```bash
# Switch to different DataRobot instance
$ dr auth set-url https://staging.datarobot.com
$ dr auth login
```

### Debug Authentication Issues

```bash
# Use verbose flag for details
$ dr auth login --verbose
[INFO] Starting OAuth server on port 8080
[INFO] Opening browser to: https://app.datarobot.com/oauth/...
[INFO] Waiting for callback...
[INFO] Received authorization code
[INFO] Exchanging code for token...
[INFO] Token saved successfully
✓ Successfully authenticated!

# Use debug flag for even more details
$ dr auth login --debug
[DEBUG] Config file: /Users/username/.datarobot/config.yaml
[DEBUG] Current URL: https://app.datarobot.com
[DEBUG] Starting server on: 127.0.0.1:8080
...
```

## Authentication Flow

```
┌──────────┐
│   User   │
└────┬─────┘
     │
     │ dr auth login
     │
     v
┌─────────────────┐       ┌──────────────┐
│  Local Server   │◄──────┤   Browser    │
│  (Port 8080)    │       │              │
└────┬────────────┘       └──────▲───────┘
     │                            │
     │                            │ Opens
     │                            │
     v                            │
┌─────────────────┐               │
│  DataRobot      │───────────────┘
│  OAuth Server   │
└────┬────────────┘
     │
     │ Returns API Key
     │
     v
┌─────────────────┐
│  Config File    │
│  (~/.datarobot/ │
│   config.yaml)  │
└─────────────────┘
```

## Configuration File

After authentication, credentials are stored in:

**Location:**
- Linux/macOS: `~/.datarobot/config.yaml`
- Windows: `%USERPROFILE%\.datarobot\config.yaml`

**Format:**
```yaml
datarobot:
  endpoint: https://app.datarobot.com
  token: <encrypted_key>

# User preferences
preferences:
  default_timeout: 30
  verify_ssl: true
```

**Permissions:**
- File is created with restricted permissions (0600)
- Only the user who created it can read/write

## Security Best Practices

### 1. Protect Your Config File

```bash
# Verify permissions
ls -la ~/.datarobot/config.yaml
# Should show: -rw------- (600)

# Fix if needed
chmod 600 ~/.datarobot/config.yaml
```

### 2. Don't Share Credentials

Never commit or share:
- `~/.datarobot/config.yaml`
- API keys
- OAuth tokens

### 3. Use Per-Environment Authentication

```bash
# Development
export DATAROBOT_CLI_CONFIG=~/.datarobot/dev-config.yaml
dr auth set-url https://dev.datarobot.com --config $DATAROBOT_CLI_CONFIG
dr auth login

# Production
export DATAROBOT_CLI_CONFIG=~/.datarobot/prod-config.yaml
dr auth set-url https://prod.datarobot.com --config $DATAROBOT_CLI_CONFIG
dr auth login
```

### 4. Regular Re-authentication

```bash
# Logout when finished
dr auth logout

# Login only when needed
dr auth login
```

## Environment Variables

Override configuration with environment variables:

```bash
# Override URL
export DATAROBOT_ENDPOINT=https://app.datarobot.com

# Override API key (not recommended)
export DATAROBOT_API_TOKEN=your-api-token

# Custom config file location
export DATAROBOT_CLI_CONFIG=~/.datarobot/custom-config.yaml
```

## Common Issues

### Browser Doesn't Open

**Problem:** Browser fails to open automatically.

**Solution:**
```bash
# Copy the URL from the output and open manually
$ dr auth login
Failed to open browser automatically.
Please visit: https://app.datarobot.com/oauth/authorize?...
```

### Port Already in Use

**Problem:** Port 8080 is already in use.

**Solution:**
The CLI automatically tries alternative ports (8081, 8082, etc.)

### Invalid Credentials

**Problem:** "Authentication failed" error.

**Solution:**
```bash
# Clear credentials and try again
dr auth logout
dr auth login
```

### Connection Refused

**Problem:** Cannot connect to DataRobot.

**Solution:**
```bash
# Verify URL is correct
cat ~/.datarobot/config.yaml

# Try setting URL again
dr auth set-url https://app.datarobot.com

# Check network connectivity
ping app.datarobot.com
```

### SSL Certificate Issues

**Problem:** SSL verification fails.

**Solution:**
```bash
# For self-signed certificates (not recommended for production)
export DATAROBOT_VERIFY_SSL=false
dr auth login
```

## See Also

- [Getting Started](../user-guide/getting-started.md) - Initial setup guide
- [Configuration](../user-guide/configuration.md) - Configuration file details
- [templates](templates.md) - Template management commands
