# `dr auth` - Authentication management

The `dr auth` command manages your authentication with DataRobot. Before you can use the CLI to work with templates and applications, you need to authenticate with your DataRobot instance.

## Quick start

For most users, authentication is a two-step process:

```bash
# 1. Set your DataRobot instance URL
dr auth set-url [YOUR_DATA_ROBOT_INSTANCE_URL] # e.g. https://app.datarobot.com

# 2. Log in (opens browser for OAuth)
dr auth login
```

Your credentials are automatically saved and you're ready to use the CLI.

> [!NOTE]
> **First time?** If you're new to the CLI, start with the [Quick start](../../README.md#quick-start) for step-by-step setup instructions.

## Synopsis

```bash
dr auth <command> [flags]
```

## Description

The `auth` command provides authentication management for the DataRobot CLI. It handles login, logout, and URL configuration for connecting to your DataRobot instance.

## Subcommands

### `login`

Authenticate with DataRobot using OAuth (Open Authorization). This is the recommended way to authenticate as it's secure and doesn't require you to manually copy API keys.

```bash
dr auth login
```

**What happens:**

1. The CLI starts a temporary local web server (typically on port 8080).
2. Your default web browser opens to DataRobot's authorization page.
3. You log in to DataRobot (if not already logged in) and authorize the CLI.
4. DataRobot sends an API key back to the CLI.
5. The CLI securely stores the API key in your configuration file.

> [!NOTE]
> OAuth is a secure authentication method that allows the CLI to access DataRobot on your behalf without you needing to manually manage API keys.

**Example:**

```bash
$ dr auth login
Opening browser for authentication...
Waiting for authentication...
✓ Successfully authenticated!
```

**Stored Credentials:**

- Location: `~/.config/datarobot/drconfig.yaml` (Linux/macOS) or `%USERPROFILE%\.config\datarobot\drconfig.yaml` (Windows)
- Format: Encrypted API key

**Troubleshooting:**

If your browser doesn't open automatically, the CLI will display a URL to visit manually. For example:

```bash
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

> [!TIP]
> **What's next?** After logging out, you can:
>
> - Log in again with `dr auth login` to re-authenticate
> - Switch to a different DataRobot instance with `dr auth set-url` followed by `dr auth login`
> - Verify authentication status with `dr templates list` (will prompt for login if not authenticated)

### `set-url`

Configure the DataRobot instance URL.

```bash
dr auth set-url [url]
```

**Arguments:**

- `url` (optional) - DataRobot instance URL. For example: `https://app.datarobot.com`

**Interactive mode:**

If you run `dr auth set-url` without providing a URL, the CLI enters interactive mode and guides you through selecting your DataRobot instance:

```bash
$ dr auth set-url
🌐 DataRobot URL Configuration

Choose your DataRobot environment:

┌────────────────────────────────────────────────────────┐
│  [1] 🇺🇸 US Cloud        https://app.datarobot.com      │
│  [2] 🇪🇺 EU Cloud        https://app.eu.datarobot.com   │
│  [3] 🇯🇵 Japan Cloud     https://app.jp.datarobot.com   │
│      🏢 Custom          Enter your custom URL          │
└────────────────────────────────────────────────────────┘

🔗 Don't know which one? Check your DataRobot login page URL.

Enter your choice:
```

**Quick selection:**

- Enter `1` for US cloud (`https://app.datarobot.com`)
- Enter `2` for EU cloud (`https://app.eu.datarobot.com`)
- Enter `3` for Japan cloud (`https://app.jp.datarobot.com`)
- Type your custom URL for self-managed instances

**Direct mode:**

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

> [!NOTE]
> The URL must be a valid HTTP or HTTPS URL. Common issues include:
>
> - Missing protocol (`https://`)
> - Invalid characters or spaces
> - Malformed domain names
> - For self-managed instances, ensure the URL includes the full domain (e.g., `https://datarobot.company.com`)

## Global options

These options work with all `auth` commands:

```bash
  -v, --verbose      Enable verbose output
      --debug        Enable debug output
      --skip-auth    Skip authentication checks (for advanced users)
  -h, --help         Show help for command
```

> [!WARNING]
> The `--skip-auth` flag bypasses all authentication checks. This is intended for advanced use cases where authentication is handled externally or not required. When this flag is used, commands that require authentication may fail with API errors.

## Examples

### First-time setup

This is the most common scenario for new users:

```bash
# Step 1: Set your DataRobot instance URL
$ dr auth set-url https://app.datarobot.com # Or your own instance URL, if different.
✓ DataRobot URL set to: https://app.datarobot.com

# Step 2: Log in (browser will open automatically)
$ dr auth login
Opening browser for authentication...
Waiting for authentication...
✓ Successfully authenticated!
```

After this, you're ready to use the CLI. Your credentials are saved automatically.

### Using interactive mode

If you're not sure which URL to use, let the CLI guide you:

```bash
# Start interactive mode
$ dr auth set-url

# Follow the prompts to select your instance
# Then log in
$ dr auth login
```

### Using cloud instance shortcuts

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

### Self-managed instance

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

### Switching instances

```bash
# Switch to different DataRobot instance
$ dr auth set-url https://staging.datarobot.com
$ dr auth login
```

### Debug authentication issues

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
[DEBUG] Config file: /Users/username/.config/datarobot/drconfig.yaml
[DEBUG] Current URL: https://app.datarobot.com
[DEBUG] Starting server on: 127.0.0.1:8080
...
```

## How authentication works

The authentication process uses OAuth, a secure standard for authorization. Here's what happens behind the scenes:

```text
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
│  (~/.config/    │
│   datarobot/     │
│   drconfig.yaml) │
└─────────────────┘
```

**Step-by-step:**

1. You run `dr auth login`
2. CLI starts a local server to receive the authorization response
3. Your browser opens to DataRobot's login page
4. You log in and authorize the CLI
5. DataRobot sends an API key to the local server
6. CLI saves the key securely to your config file
7. Browser and server close automatically

This process is secure because:

- You authenticate directly with DataRobot (not the CLI)
- The API key is transmitted securely
- No passwords are stored locally

## Configuration file

After authentication, credentials are stored in:

**Location:**

- Linux/macOS: `~/.config/datarobot/drconfig.yaml`
- Windows: `%USERPROFILE%\.config\datarobot\drconfig.yaml`

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

## Security best practices

### Protect your config file

```bash
# Verify permissions
ls -la ~/.config/datarobot/drconfig.yaml
# Should show: -rw------- (600)

# Fix if needed
chmod 600 ~/.config/datarobot/drconfig.yaml
```

### Don't share credentials

> [!WARNING]
> Never commit or share:
>
> - `~/.config/datarobot/drconfig.yaml`
> - API keys
> - OAuth tokens

### Use per-environment authentication

```bash
# Development
export DATAROBOT_CLI_CONFIG=~/.config/datarobot/dev-config.yaml
dr auth set-url https://dev.datarobot.com --config $DATAROBOT_CLI_CONFIG
dr auth login

# Production
export DATAROBOT_CLI_CONFIG=~/.config/datarobot/prod-config.yaml
dr auth set-url https://prod.datarobot.com --config $DATAROBOT_CLI_CONFIG
dr auth login
```

### Regular re-authentication

```bash
# Logout when finished
dr auth logout

# Login only when needed
dr auth login
```

## Environment variables

Override configuration with environment variables:

```bash
# Override URL
export DATAROBOT_ENDPOINT=https://app.datarobot.com

# Override API key (not recommended)
export DATAROBOT_API_TOKEN=your-api-token

# Custom config file location
export DATAROBOT_CLI_CONFIG=~/.config/datarobot/custom-config.yaml
```

## Common issues

### Browser doesn't open

**Problem:** Browser fails to open automatically.

**Solution:**

```bash
# Copy the URL from the output and open manually
$ dr auth login
Failed to open browser automatically.
Please visit: https://app.datarobot.com/oauth/authorize?...
```

### Port already in use

**Problem:** Port 8080 is already in use.

**Solution:**
The CLI automatically tries alternative ports (8081, 8082, etc.)

### Invalid credentials

**Problem:** "Authentication failed" error. This can occur when:

- Your API token has expired
- Your API token was revoked by an administrator
- The DataRobot URL has changed
- The config file is corrupted or contains invalid data

**Solution:**

```bash
# Clear credentials and try again
dr auth logout
dr auth login
```

**If the problem persists:**

```bash
# Verify your DataRobot URL is correct
dr auth set-url https://app.datarobot.com  # or your instance URL

# Check the config file for issues
cat ~/.config/datarobot/drconfig.yaml

# If config file is corrupted, you can manually edit it or delete it
# (it will be recreated on next login)
rm ~/.config/datarobot/drconfig.yaml
dr auth set-url https://app.datarobot.com
dr auth login
```

### Connection refused

**Problem:** Cannot connect to DataRobot. This typically means:

- The DataRobot instance URL is incorrect
- Network connectivity issues (firewall, VPN, proxy)
- The DataRobot instance is down or unreachable
- DNS resolution problems

**Solution:**

```bash
# Verify URL is correct
cat ~/.config/datarobot/drconfig.yaml

# Try setting URL again
dr auth set-url https://app.datarobot.com

# Check network connectivity
ping app.datarobot.com

# Test HTTPS connectivity
curl -I https://app.datarobot.com
```

**For corporate networks with proxies:**

```bash
# Set proxy environment variables if required
export HTTP_PROXY=http://proxy.company.com:8080
export HTTPS_PROXY=http://proxy.company.com:8080
dr auth login
```

### SSL certificate issues

**Problem:** SSL verification fails. This can occur with:

- Self-signed certificates (common in enterprise/self-managed instances)
- Expired certificates
- Certificate chain issues
- Corporate proxy intercepting SSL

**Solution:**

```bash
# For self-signed certificates (not recommended for production)
export DATAROBOT_VERIFY_SSL=false
dr auth login
```

**For enterprise environments:**

```bash
# If your organization provides a CA certificate bundle
export DATAROBOT_CA_CERT=/path/to/ca-bundle.crt
dr auth login

# Or configure in the config file
# See [Configuration files](../user-guide/configuration.md) for details
```

> [!WARNING]
> Disabling SSL verification (`DATAROBOT_VERIFY_SSL=false`) makes your connection vulnerable to man-in-the-middle attacks. Only use this in development environments or when you understand the security implications.

## See also

- [Quick start](../../README.md#quick-start) - Initial setup guide
- [Configuration](../user-guide/configuration.md) - Configuration file details and advanced settings
- [Templates](../template-system/) - Template management commands

> [!TIP]
> **What's next?** After setting up authentication:
>
> - Browse available templates: `dr templates list`
> - Set up your first template: `dr templates setup`
> - Learn about [configuration files](../user-guide/configuration.md) for advanced settings
