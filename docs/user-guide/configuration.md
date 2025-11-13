# Configuration Files

Understanding DataRobot CLI configuration files and settings.

## Configuration location

The CLI stores configuration in a platform-specific location:

| Platform | Location |
|----------|----------|
| Linux | `~/.datarobot/config.yaml` |
| macOS | `~/.datarobot/config.yaml` |
| Windows | `%USERPROFILE%\.datarobot\config.yaml` |

## Configuration structure

### Main configuration file

`~/.datarobot/config.yaml`:

```yaml
# DataRobot Connection
endpoint: https://app.datarobot.com
token: api key here
```

### Environment-specific configs

You can maintain multiple configurations:

```bash
# Development
~/.datarobot/dev-config.yaml

# Staging
~/.datarobot/staging-config.yaml

# Production
~/.datarobot/prod-config.yaml
```

Switch between them:

```bash
export DATAROBOT_CLI_CONFIG=~/.datarobot/dev-config.yaml
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

## Environment Variables

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
export DATAROBOT_CLI_CONFIG=~/.datarobot/custom-config.yaml

# Editor for text editing
export EDITOR=nano

# Force setup wizard to run even if already completed
export DATAROBOT_CLI_FORCE_WIZARD=true
```

### Advanced flags

The CLI supports advanced command-line flags for special use cases:

```bash
# Skip authentication checks (advanced users only)
dr templates list --skip-auth

# Force setup wizard to run (ignore completion state)
dr templates setup --force-wizard

# Enable verbose logging
dr templates list --verbose

# Enable debug logging
dr templates list --debug
```

> **⚠️ Warning:** The `--skip-auth` flag bypasses all authentication checks and should only be used when you understand the implications. Commands requiring API access will likely fail without valid credentials.


## Configuration priority

Settings are loaded in order of precedence:

1. flags (command-line arguments, i.e. `--config <path>`)
2. environment variables (i.e. `DATAROBOT_CLI_CONFIG_PATH=...`)
3. config files (i.e. `~/.datarobot/config.yaml`)
4. defaults (built-in defaults)

## Security best practices

### 1. Protect configuration files

```bash
# Verify permissions (should be 600)
ls -la ~/.datarobot/config.yaml

# Fix permissions if needed
chmod 600 ~/.datarobot/config.yaml
chmod 700 ~/.datarobot/
```

### 2. Don't commit credentials

Add to `.gitignore`:

```gitignore
# DataRobot credentials
.datarobot/
config.yaml
*.yaml
!.env.template
```

### 3. Use environment-specific configs

```bash
# Never use production credentials in development
# Keep separate config files
~/.datarobot/
├── dev-config.yaml      # Development
├── staging-config.yaml  # Staging
└── prod-config.yaml     # Production
```

### 4. Avoid environment variables for secrets

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

`~/.datarobot/dev-config.yaml`:

```yaml
endpoint: https://dev.datarobot.com
token: api token for dev
```

Usage:

```bash
export DATAROBOT_CLI_CONFIG=~/.datarobot/dev-config.yaml
dr templates list
```

### Production environment

`~/.datarobot/prod-config.yaml`:

```yaml
endpoint: https://app.datarobot.com
token: api key for prod
```

Usage:

```bash
export DATAROBOT_CLI_CONFIG=~/.datarobot/prod-config.yaml
dr run deploy
```

### Enterprise with proxy

`~/.datarobot/enterprise-config.yaml`:

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

```bash
# Check if config file exists
ls -la ~/.datarobot/config.yaml

# Verify it's readable
cat ~/.datarobot/config.yaml

# Check environment variables
env | grep DATAROBOT
```

### Invalid configuration

```bash
# The CLI will report syntax errors
$ dr templates list
Error: Failed to parse config file: yaml: line 5: could not find expected ':'

# Fix syntax and try again
vim ~/.datarobot/config.yaml
```

### Permission denied

```bash
# Fix file permissions
chmod 600 ~/.datarobot/config.yaml

# Fix directory permissions
chmod 700 ~/.datarobot/
```

### Multiple configs

```bash
# List all config files
find ~/.datarobot -name "*.yaml"

# Switch between them
export DATAROBOT_CLI_CONFIG=~/.datarobot/dev-config.yaml
```

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

You can also force the wizard to run without deleting the state file by using the `--force-wizard` flag:

```bash
# Force re-execution of setup wizard while preserving state
dr templates setup --force-wizard

# Or via environment variable
export DATAROBOT_CLI_FORCE_WIZARD=true
dr templates setup
```

This flag makes commands behave as if setup has never been completed, while still updating the state file. This is useful for:

- Testing setup flows
- Forcing reconfiguration without losing state history
- Development and debugging

State files are small and do not require manual management under normal circumstances. Each repository maintains its own state independently.

## See also

- [Getting started](getting-started.md)&mdash;initial setup.
- [Authentication](authentication.md)&mdash;managing credentials.
- [auth command](../commands/auth.md)&mdash;authentication commands.
