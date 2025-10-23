# Configuration Files

Understanding DataRobot CLI configuration files and settings.

## Configuration Location

The CLI stores configuration in a platform-specific location:

| Platform | Location |
|----------|----------|
| Linux | `~/.datarobot/config.yaml` |
| macOS | `~/.datarobot/config.yaml` |
| Windows | `%USERPROFILE%\.datarobot\config.yaml` |

## Configuration Structure

### Main Configuration File

`~/.datarobot/config.yaml`:

```yaml
# DataRobot Connection
datarobot:
  endpoint: https://app.datarobot.com
  api_key: encrypted_key_here
  api_version: v2
  
# CLI Preferences
preferences:
  default_timeout: 30
  verify_ssl: true
  log_level: warn
  
# Template Settings
templates:
  default_clone_dir: ~/datarobot-templates
  auto_update_check: true
  
# Task Runner Settings
tasks:
  default_concurrency: 2
  show_command_output: true
```

### Environment-Specific Configs

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
export DATAROBOT_CONFIG_PATH=~/.datarobot/dev-config.yaml
dr templates list
```

## Configuration Options

### Connection Settings

```yaml
datarobot:
  # Required: DataRobot instance URL
  endpoint: https://app.datarobot.com
  
  # Required: API authentication key
  api_key: your_encrypted_key
  
  # Optional: API version (default: v2)
  api_version: v2
  
  # Optional: Connection timeout in seconds (default: 30)
  timeout: 30
  
  # Optional: Verify SSL certificates (default: true)
  verify_ssl: true
  
  # Optional: Custom CA certificate path
  ca_cert_path: /path/to/ca-cert.pem
```

### User Preferences

```yaml
preferences:
  # Log level: debug, info, warn, error (default: warn)
  log_level: info
  
  # Enable color output (default: true)
  color_output: true
  
  # Default editor for text editing (default: $EDITOR or vim)
  editor: code
  
  # Auto-check for CLI updates (default: true)
  check_updates: true
```

### Template Settings

```yaml
templates:
  # Default directory for cloning templates
  default_clone_dir: ~/datarobot-projects
  
  # Auto-update check for templates (default: false)
  auto_update_check: false
  
  # Preferred Git protocol: https or ssh (default: https)
  git_protocol: https
```

### Task Runner Settings

```yaml
tasks:
  # Maximum concurrent tasks (default: 2)
  default_concurrency: 4
  
  # Show task command output (default: true)
  show_command_output: true
  
  # Continue on task error (default: false)
  continue_on_error: false
```

## Environment Variables

Override configuration with environment variables:

### Connection

```bash
# DataRobot endpoint URL
export DATAROBOT_ENDPOINT=https://app.datarobot.com

# API key (not recommended for security)
export DATAROBOT_API_KEY=your_api_key

# API version
export DATAROBOT_API_VERSION=v2

# Connection timeout (seconds)
export DATAROBOT_TIMEOUT=60

# Verify SSL
export DATAROBOT_VERIFY_SSL=true
```

### CLI Behavior

```bash
# Log level: debug, info, warn, error
export DR_LOG_LEVEL=debug

# Disable color output
export NO_COLOR=1

# Custom config file path
export DATAROBOT_CONFIG_PATH=~/.datarobot/custom-config.yaml

# Editor for text editing
export EDITOR=nano
```

### Task Runner

```bash
# Default concurrency
export DR_CONCURRENCY=4

# Task timeout
export DR_TASK_TIMEOUT=300
```

## Configuration Priority

Settings are loaded in this order (highest to lowest priority):

1. **Command-line flags**: `dr --verbose`
2. **Environment variables**: `DATAROBOT_ENDPOINT=...`
3. **Config file**: `~/.datarobot/config.yaml`
4. **Default values**: Built-in defaults

Example:

```bash
# Config file has:
# log_level: warn

# Environment variable overrides it:
export DR_LOG_LEVEL=debug

# Command flag overrides everything:
dr --verbose templates list
# Uses verbose (info) level
```

## Security Best Practices

### 1. Protect Configuration Files

```bash
# Verify permissions (should be 600)
ls -la ~/.datarobot/config.yaml

# Fix permissions if needed
chmod 600 ~/.datarobot/config.yaml
chmod 700 ~/.datarobot/
```

### 2. Don't Commit Credentials

Add to `.gitignore`:

```gitignore
# DataRobot credentials
.datarobot/
config.yaml
*.yaml
!.env.template
```

### 3. Use Environment-Specific Configs

```bash
# Never use production credentials in development
# Keep separate config files
~/.datarobot/
├── dev-config.yaml      # Development
├── staging-config.yaml  # Staging
└── prod-config.yaml     # Production
```

### 4. Avoid Environment Variables for Secrets

```bash
# ❌ Don't do this (visible in process list)
export DATAROBOT_API_KEY=my_secret_key

# ✅ Do this instead (use config file)
dr auth login
```

## Advanced Configuration

### Custom Templates Directory

```yaml
templates:
  default_clone_dir: ~/workspace/datarobot
```

Or via environment:

```bash
export DR_TEMPLATES_DIR=~/workspace/datarobot
```

### Proxy Configuration

```yaml
datarobot:
  endpoint: https://app.datarobot.com
  proxy: http://proxy.company.com:8080
  no_proxy: localhost,127.0.0.1
```

Or via standard proxy environment variables:

```bash
export HTTP_PROXY=http://proxy.company.com:8080
export HTTPS_PROXY=http://proxy.company.com:8080
export NO_PROXY=localhost,127.0.0.1
```

### Custom CA Certificates

For self-signed certificates:

```yaml
datarobot:
  endpoint: https://datarobot.mycompany.com
  verify_ssl: true
  ca_cert_path: /etc/ssl/certs/mycompany-ca.pem
```

Or via environment:

```bash
export DATAROBOT_CA_CERT=/etc/ssl/certs/mycompany-ca.pem
```

### Debugging Configuration

Enable debug logging:

```yaml
preferences:
  log_level: debug
```

Or temporarily:

```bash
dr --debug templates list
```

## Configuration Examples

### Development Environment

`~/.datarobot/dev-config.yaml`:

```yaml
datarobot:
  endpoint: https://dev.datarobot.com
  api_key: dev_encrypted_key
  
preferences:
  log_level: debug
  color_output: true
  
templates:
  default_clone_dir: ~/dev/datarobot-templates
  
tasks:
  default_concurrency: 2
  show_command_output: true
  continue_on_error: true
```

Usage:

```bash
export DATAROBOT_CONFIG_PATH=~/.datarobot/dev-config.yaml
dr templates list
```

### Production Environment

`~/.datarobot/prod-config.yaml`:

```yaml
datarobot:
  endpoint: https://app.datarobot.com
  api_key: prod_encrypted_key
  timeout: 60
  
preferences:
  log_level: error
  color_output: false
  
templates:
  default_clone_dir: ~/production/datarobot-templates
  
tasks:
  default_concurrency: 1
  show_command_output: false
  continue_on_error: false
```

Usage:

```bash
export DATAROBOT_CONFIG_PATH=~/.datarobot/prod-config.yaml
dr run deploy
```

### Enterprise with Proxy

`~/.datarobot/enterprise-config.yaml`:

```yaml
datarobot:
  endpoint: https://datarobot.enterprise.com
  api_key: enterprise_key
  proxy: http://proxy.enterprise.com:3128
  verify_ssl: true
  ca_cert_path: /etc/ssl/certs/enterprise-ca.pem
  timeout: 120
  
preferences:
  log_level: warn
```

## Troubleshooting

### Configuration Not Loading

```bash
# Check if config file exists
ls -la ~/.datarobot/config.yaml

# Verify it's readable
cat ~/.datarobot/config.yaml

# Check environment variables
env | grep DATAROBOT
```

### Invalid Configuration

```bash
# The CLI will report syntax errors
$ dr templates list
Error: Failed to parse config file: yaml: line 5: could not find expected ':'

# Fix syntax and try again
vim ~/.datarobot/config.yaml
```

### Permission Denied

```bash
# Fix file permissions
chmod 600 ~/.datarobot/config.yaml

# Fix directory permissions
chmod 700 ~/.datarobot/
```

### Multiple Configs

```bash
# List all config files
find ~/.datarobot -name "*.yaml"

# Switch between them
export DATAROBOT_CONFIG_PATH=~/.datarobot/dev-config.yaml
```

## See Also

- [Getting Started](getting-started.md) - Initial setup
- [Authentication](authentication.md) - Managing credentials
- [auth command](../commands/auth.md) - Authentication commands
