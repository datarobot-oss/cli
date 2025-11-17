# Configuration Files

The DataRobot CLI stores your authentication credentials and preferences in configuration files. This guide explains how configuration files work, where they're stored, and how to manage them.

> [!NOTE]
> **New to the CLI?** If you're setting up the CLI for the first time, you typically don't need to manually create configuration files. They're automatically created when you run `dr auth set-url` and `dr auth login`. See the [Quick start guide](../README.md#quick-start) for initial setup.

## Configuration location

The CLI automatically stores configuration files in a standard location based on your operating system:

| Platform | Location                                        |
|----------|-------------------------------------------------|
| Linux    | `~/.config/datarobot/drconfig.yaml`             |
| macOS    | `~/.config/datarobot/drconfig.yaml`             |
| Windows  | `%USERPROFILE%\.config\datarobot\drconfig.yaml` |

## Configuration structure

### Main configuration file

The main configuration file (`drconfig.yaml`) stores your DataRobot connection settings and authentication token. Here's what it looks like:

```yaml
# DataRobot Connection
endpoint: DATA_ROBOT_ENDPOINT_URL # e.g. https://app.datarobot.com
token: API_KEY_HERE
```

**Configuration fields:**

- `endpoint`: Your DataRobot instance URL (e.g., `https://app.datarobot.com`)
- `token`: Your API authentication token (automatically stored after `dr auth login`)

> [!NOTE]
> You typically don't need to edit this file manually. The CLI manages it automatically when you use `dr auth set-url` and `dr auth login`.

### Environment-specific configs

> [!TIP]
> If you work with multiple DataRobot environments (development, staging, production), you can maintain separate configuration files for each. For example:
>
> ```bash
> # Development
> ~/.config/datarobot/dev-config.yaml
>
> # Staging
> ~/.config/datarobot/staging-config.yaml
> 
> # Production
> ~/.config/datarobot/prod-config.yaml
> ```

Switch between them:

```bash
export DATAROBOT_CLI_CONFIG=~/.config/datarobot/dev-config.yaml
dr templates list
```

## Configuration options

### Connection settings

```yaml
# Required: DataRobot instance URL
endpoint: https://app.datarobot.com

# Required: API authentication key
token: api key here
```

## Environment variables

Override configuration with environment variables:

### Connection

```bash
# DataRobot endpoint URL
export DATAROBOT_ENDPOINT=https://app.datarobot.com

# API token (not recommended for security)
export DATAROBOT_API_TOKEN=your_api_token
```

### CLI behavior

```bash
# Custom config file path
export DATAROBOT_CLI_CONFIG=~/.config/datarobot/custom-config.yaml

# Editor for text editing
export EDITOR=nano

# Force setup wizard to run even if already completed
export DATAROBOT_CLI_FORCE_INTERACTIVE=true
```

### Advanced flags

The CLI supports advanced command-line flags for special use cases:

```bash
# Skip authentication checks (advanced users only)
dr templates list --skip-auth

# Force setup wizard to run (ignore completion state)
dr templates setup --force-interactive

# Enable verbose logging
dr templates list --verbose

# Enable debug logging
dr templates list --debug
```

> [!WARNING]
> The `--skip-auth` flag bypasses all authentication checks and should only be used when you understand the implications. Commands requiring API access will likely fail without valid credentials.

## Configuration priority

When the CLI needs configuration settings, it looks for them in this order (highest to lowest priority):

1. **Command-line flags** (e.g., `--config <path>`) - Overrides everything
2. **Environment variables** (e.g., `DATAROBOT_CLI_CONFIG`) - Overrides config files
3. **Config files** (e.g., `~/.config/datarobot/drconfig.yaml`) - Default location
4. **Built-in defaults** - Fallback values

This means if you set an environment variable, it will take precedence over what's in your config file. This is useful for temporarily overriding settings without editing files.

## Security best practices

### Protect configuration files

```bash
# Verify permissions (should be 600)
ls -la ~/.config/datarobot/drconfig.yaml

# Fix permissions if needed
chmod 600 ~/.config/datarobot/drconfig.yaml
chmod 700 ~/.config/datarobot/
```

### Don't commit credentials

Add to `.gitignore`:

```gitignore
# DataRobot credentials
.config/datarobot/
.datarobot/
drconfig.yaml
config.yaml
*.yaml
!.env.template
```

### Use environment-specific configs

```bash
# Never use production credentials in development
# Keep separate config files
~/.config/datarobot/
├── drconfig.yaml        # Default config
├── dev-config.yaml      # Development
├── staging-config.yaml  # Staging
└── prod-config.yaml     # Production
```

### Avoid environment variables for secrets

```bash
# ❌ Don't do this (visible in process list)
export DATAROBOT_API_TOKEN=my_secret_token

# ✅ Do this instead (use config file)
dr auth login
```

## Advanced configuration

### Custom templates directory

```yaml
templates:
  default_clone_dir: ~/workspace/datarobot
```

Or via environment:

```bash
export DR_TEMPLATES_DIR=~/workspace/datarobot
```

### Debugging configuration

Enable debug logging:

```yaml
debug: true
```

Or temporarily:

```bash
dr --debug templates list
```

## Configuration examples

### Development environment

`~/.config/datarobot/dev-config.yaml`:

```yaml
endpoint: https://dev.datarobot.com
token: api token for dev
```

Usage:

```bash
export DATAROBOT_CLI_CONFIG=~/.config/datarobot/dev-config.yaml
dr templates list
```

### Production environment

`~/.config/datarobot/prod-config.yaml`:

```yaml
endpoint: https://app.datarobot.com
token: api key for prod
```

Usage:

```bash
export DATAROBOT_CLI_CONFIG=~/.config/datarobot/prod-config.yaml
dr run deploy
```

### Enterprise with proxy

`~/.config/datarobot/enterprise-config.yaml`:

```yaml
datarobot:
  endpoint: https://datarobot.enterprise.com
  token: enterprise_key
  proxy: http://proxy.enterprise.com:3128
  verify_ssl: true
  ca_cert_path: /etc/ssl/certs/enterprise-ca.pem
  timeout: 120

preferences:
  log_level: warn
```

## Troubleshooting

### Configuration not loading

**Problem:** The CLI cannot find or read the configuration file. Common causes:

- Config file doesn't exist (first-time setup)
- Incorrect file path
- Permission issues
- Environment variable overriding the default path
- Config file in wrong location

**Solution:**

```bash
# Check if config file exists
ls -la ~/.config/datarobot/drconfig.yaml

# If file doesn't exist, create it by running:
dr auth set-url https://app.datarobot.com
dr auth login

# Verify it's readable
cat ~/.config/datarobot/drconfig.yaml

# Check environment variables that might override the path
env | grep DATAROBOT

# Verify the directory exists
ls -la ~/.config/datarobot/
```

**If using a custom config path:**

```bash
# Verify the environment variable is set correctly
echo $DATAROBOT_CLI_CONFIG

# Test with explicit path
dr templates list --config ~/.config/datarobot/drconfig.yaml
```

### Invalid configuration

**Problem:** YAML syntax errors in the configuration file. Common causes:
- Missing colons (`:`) after keys
- Incorrect indentation (YAML is sensitive to spaces)
- Invalid YAML characters
- Unclosed quotes or brackets
- Mixing tabs and spaces

**Solution:**

```bash
# The CLI will report syntax errors with line numbers
$ dr templates list
Error: Failed to parse config file: yaml: line 5: could not find expected ':'

# Fix syntax and try again
vim ~/.config/datarobot/drconfig.yaml
# or
nano ~/.config/datarobot/drconfig.yaml
```

**Example of correct YAML format:**

```yaml
# Correct format
datarobot:
  endpoint: https://app.datarobot.com
  token: your-api-token-here

# Common mistakes:
# ❌ Missing colon: endpoint https://app.datarobot.com
# ❌ Wrong indentation (must use spaces, not tabs)
# ❌ Missing quotes for values with special characters
```

**Validate YAML syntax:**

```bash
# Use a YAML validator or check manually
python3 -c "import yaml; yaml.safe_load(open('~/.config/datarobot/drconfig.yaml'))"
```

### Permission denied

**Problem:** The CLI cannot read or write the configuration file due to file system permissions. This can occur when:

- File permissions are too restrictive for the current user
- Directory permissions prevent file access
- File was created by a different user (e.g., with `sudo`)
- SELinux or AppArmor restrictions (Linux)

**Solution:**

```bash
# Fix file permissions (owner read/write only)
chmod 600 ~/.config/datarobot/drconfig.yaml

# Fix directory permissions (owner read/write/execute)
chmod 700 ~/.config/datarobot/

# Verify permissions
ls -la ~/.config/datarobot/drconfig.yaml
# Should show: -rw------- (600)
```

**If file was created with sudo:**

```bash
# Change ownership to your user
sudo chown $USER:$USER ~/.config/datarobot/drconfig.yaml
chmod 600 ~/.config/datarobot/drconfig.yaml
```

**For Windows:**

```powershell
# Check file permissions
icacls %USERPROFILE%\.config\datarobot\drconfig.yaml

# If needed, grant full control to your user
icacls %USERPROFILE%\.config\datarobot\drconfig.yaml /grant %USERNAME%:F
```

### Multiple configs

**Problem:** Managing multiple environments (dev, staging, production) with separate configurations.

**Solution:**

```bash
# List all config files
find ~/.config/datarobot -name "*.yaml"

# Switch between them using environment variable
export DATAROBOT_CLI_CONFIG=~/.config/datarobot/dev-config.yaml
dr templates list

# Or use inline for single commands
DATAROBOT_CLI_CONFIG=~/.config/datarobot/prod-config.yaml dr templates list
```

**Create a helper script for easy switching:**

```bash
# Add to ~/.bashrc or ~/.zshrc
alias dr-dev='export DATAROBOT_CLI_CONFIG=~/.config/datarobot/dev-config.yaml'
alias dr-prod='export DATAROBOT_CLI_CONFIG=~/.config/datarobot/prod-config.yaml'
alias dr-staging='export DATAROBOT_CLI_CONFIG=~/.config/datarobot/staging-config.yaml'

# Usage:
dr-dev
dr templates list  # Uses dev config

dr-prod
dr templates list  # Uses prod config
```

> [!TIP]
> Always verify which config is active before running commands in production:
>
>```bash
> echo "Current config: $DATAROBOT_CLI_CONFIG"
> cat $DATAROBOT_CLI_CONFIG
>```
>
> Example output:
>
>```bash
> Current config: ~/.config/datarobot/prod-config.yaml
>```

## State tracking

The CLI maintains state information about your interactions with repositories to provide a better user experience. State is tracked per-repository and stores metadata about command executions.

### State file location

The CLI stores state locally within each repository:

- `.datarobot/state/info.yml` in the current working directory

### Tracked information

The state file tracks:

- **CLI version**: Version of the CLI used for the last successful execution
- **Last start**: Timestamp of the last successful `dr start` execution
- **Last dotenv setup**: Timestamp of the last successful `dr dotenv setup` execution

### State file format

```yaml
cli_version: 1.0.0
last_start: 2025-11-13T00:02:07.615186Z
last_dotenv_setup: 2025-11-13T00:15:30.123456Z
```

All timestamps are in ISO 8601 format (UTC).

### How state is used

- **`dr start`**: Updates state after successful execution
- **`dr dotenv setup`**: Records when environment setup was completed
- **`dr templates setup`**: Skips dotenv setup if it was already completed (based on state)

### Managing state

State files are automatically created and updated. To reset state for a repository:

```bash
# Remove repository state
rm .datarobot/state/info.yml
```

You can also force the wizard to run without deleting the state file by using the `--force-interactive` flag:

```bash
# Force re-execution of setup wizard while preserving state
dr templates setup --force-interactive

# Or via environment variable
export DATAROBOT_CLI_FORCE_INTERACTIVE=true
dr templates setup
```

This flag makes commands behave as if setup has never been completed, while still updating the state file. This is useful for:

- Testing setup flows
- Forcing reconfiguration without losing state history
- Development and debugging

State files are small and do not require manual management under normal circumstances. Each repository maintains its own state independently.

## See also

- [Quick start](../../README.md#quick-start)&mdash;initial setup and first-time configuration
- [Authentication](authentication.md)&mdash;managing credentials and authentication flow
- [auth command](../commands/auth.md)&mdash;authentication commands and troubleshooting

> **What's next?** After understanding configuration:
>
> - Set up authentication: `dr auth login` (see [auth command](../commands/auth.md))
> - Browse templates: `dr templates list`
> - Set up your first template: `dr templates setup`
