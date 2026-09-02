# `dr auth` - Authentication management

The `dr auth` command manages your authentication with DataRobot. Before you can use the CLI to work with templates and applications, you need to authenticate with your DataRobot instance.

## Quick start

For most users, authentication is a two-step process:

```bash
# 1. Set your DataRobot instance URL
dr auth set-url [YOUR_DATA_ROBOT_INSTANCE_URL] # e.g. https://app.datarobot.com

# 2. Log in (opens your browser to authorize the CLI)
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

The `auth` command provides authentication management for the DataRobot CLI. It handles login, logout, URL configuration, and exporting credentials as environment variables for connecting to your DataRobot instance.

## Subcommands

### `login`

Authenticate by authorizing the CLI in your browser, so you never have to copy an API key by hand.

```bash
dr auth login
```

**Options:**

| Flag           | Description                                                |
| -------------- | ---------------------------------------------------------- |
| `--no-browser` | Print the login link instead of opening a browser (useful over SSH) |

**What happens:**

1. The CLI starts a temporary local web server on `localhost:51164`.
2. Your default web browser opens to DataRobot's developer tools page.
3. You log in to DataRobot (if not already logged in) and authorize the CLI.
4. DataRobot redirects back to the local server with your API key.
5. The CLI stores the API key in your configuration file.

> [!NOTE]
> Despite the name, this is not an OAuth grant: there is no `client_id`, no PKCE, and
> no authorization-code exchange. DataRobot returns the API key directly to the local
> callback server. The port is fixed at `51164` because the web app's redirect target
> is part of the contract, so there is no fallback port.

**Example:**

```bash
$ dr auth login
⠋ Opening your browser to sign in to DataRobot…

  Didn't open? Use this link:

  ╭────────────────────────────────────────────────────────────────────╮
  │ https://app.datarobot.com/account/developer-tools?cliRedirect=true │
  ╰────────────────────────────────────────────────────────────────────╯
```

**Stored Credentials:**

- Location: `~/.config/datarobot/drconfig.yaml` (Linux/macOS) or `%USERPROFILE%\.config\datarobot\drconfig.yaml` (Windows)
- Format: plaintext YAML, written with `0600` (owner read/write only). The token is
  **not** encrypted — treat the file as a secret.

**Troubleshooting:**

If the browser cannot be opened, the CLI says so and the link becomes the instruction
rather than a footnote. The layout is otherwise identical, so the two paths read as the
same command:

```bash
$ dr auth login
⠋ Waiting for authorization…

  ⚠ Couldn't open your browser automatically.
  Open this link to finish signing in:

  ╭────────────────────────────────────────────────────────────────────╮
  │ https://app.datarobot.com/account/developer-tools?cliRedirect=true │
  ╰────────────────────────────────────────────────────────────────────╯
```

If another `dr` process is already waiting on `localhost:51164`, the new one asks it to
release the port and takes over. The wait times out after 5 minutes.

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
> - Verify authentication status with `dr auth check`

### `check`

Verify that your DataRobot credentials are properly configured and valid without triggering the login flow.

```bash
dr auth check
```

**What it checks (in order):**

1. **Project `.env` file** (if in a repository with `.env`):
   - Validates `DATAROBOT_ENDPOINT` and `DATAROBOT_API_TOKEN` in `.env`

2. **Environment variables**:
   - Checks `DATAROBOT_ENDPOINT` (or `DATAROBOT_API_ENDPOINT` fallback)
   - Checks `DATAROBOT_API_TOKEN`

3. **CLI config file**:
   - Falls back to `~/.config/datarobot/drconfig.yaml`

**Example output:**

```bash
# Valid environment credentials
$ dr auth check
✅ Environment variable authentication is valid.

# Valid .env credentials (in a project directory)
$ dr auth check
✅ '.env' credentials are valid.

# Invalid or missing credentials
$ dr auth check
❌ No DataRobot URL configured.
Run dr auth set-url to configure your DataRobot URL.
❌ No valid API key found in CLI config.
Run dr auth login to authenticate.

# Invalid environment token
$ dr auth check
❌ DATAROBOT_API_TOKEN environment variable is invalid or expired.
Unset it and try again:
  unset DATAROBOT_API_TOKEN (or Remove-Item Env:\DATAROBOT_API_TOKEN on Windows)

# Unreachable endpoint (the token was never checked, so it is not blamed)
$ dr auth check
❌ Could not connect to https://app.example.com: dial tcp: lookup app.example.com: no such host
Check DATAROBOT_ENDPOINT and your network, then try again.

# The instance answered, but not with a credential verdict (only 401/403 blame the token)
$ dr auth check
❌ https://app.example.com answered HTTP 503, so the CLI could not verify your credentials.
Check DATAROBOT_ENDPOINT, and the instance's status if it persists.

# Endpoint scheme the CLI cannot use
$ dr auth check
❌ DATAROBOT_ENDPOINT environment variable is invalid: unsupported URL scheme "ftp", use https://
Set it to a valid DataRobot URL and try again.
```

> [!TIP]
> Use `dr auth check` in CI/CD pipelines to verify credentials before running other commands.

### `export`

Print the canonical DataRobot environment variables for the credentials the CLI is currently using, as shell statements you can source into your session.

```bash
dr auth export [--shell <bash|zsh|fish|powershell|cmd>]
```

**Output:**

```bash
$ dr auth export
export DATAROBOT_ENDPOINT='https://app.datarobot.com/api/v2'
export DATAROBOT_API_TOKEN='<token>'
```

Only the statements go to stdout — every error, hint, and warning goes to stderr — so the output is always safe to evaluate.

**Sourcing into your shell:**

```bash
# bash / zsh
eval "$(dr auth export)"

# fish
dr auth export | source

# PowerShell
dr auth export | Out-String | Invoke-Expression

# cmd.exe
for /f "usebackq delims=" %i in (`dr auth export --shell cmd`) do @%i

# Save for later sourcing
dr auth export --shell bash > ~/.datarobot-env && source ~/.datarobot-env
```

The output syntax is chosen from the detected parent shell. Use `--shell` to override it — useful when generating a file for a different shell, or when detection fails (unrecognized shells fall back to POSIX `export` syntax).

**Where credentials come from:**

1. `DATAROBOT_ENDPOINT` (or `DATAROBOT_API_ENDPOINT`) and `DATAROBOT_API_TOKEN`, if both are set
2. Otherwise the CLI config file written by `dr auth login`

The endpoint is normalized to its canonical `/api/v2` form, so a config or environment value of `app.datarobot.com` is exported as `https://app.datarobot.com/api/v2`. A URL that already has a path (self-managed installs serving the API under a custom prefix) is left alone.

A project's `.env` file is never consulted — this exports the CLI's own credentials. Use [`dr dotenv setup`](dotenv.md) to manage `.env` files.

**Machine-readable output:**

```bash
$ dr auth export --output-format json
{
  "environment": {
    "DATAROBOT_API_TOKEN": "<token>",
    "DATAROBOT_ENDPOINT": "https://app.datarobot.com/api/v2"
  },
  "source": "drconfig.yaml"
}
```

The `source` field names where the credentials were read from: `drconfig.yaml` (the CLI config file) or `DATAROBOT_ENDPOINT environment variable` (when the env pair takes precedence).

**No credentials configured:**

```bash
$ dr auth export
❌ No DataRobot credentials found.
Run dr auth login to authenticate.
```

Nothing is written to stdout and the command exits non-zero, so `eval "$(dr auth export)"` cannot evaluate a partial result.

**Malformed endpoint:**

```bash
$ dr auth export
❌ Invalid DataRobot URL from DATAROBOT_ENDPOINT environment variable: parse "'https://app.datarobot.com/api/v2'": first path segment in URL cannot contain colon
If you ran `$(dr auth export)`, use `eval "$(dr auth export)"` instead.
Unset the invalid variable(s) and try again:
  unset DATAROBOT_ENDPOINT DATAROBOT_API_TOKEN (or Remove-Item Env:\DATAROBOT_ENDPOINT, Env:\DATAROBOT_API_TOKEN on Windows)
```

When the bad value comes from the environment, the hint recommends `unset` because env vars take precedence over `drconfig.yaml` — `dr auth set-url` only writes the config file and cannot clear poisoned env vars. For a malformed endpoint sourced from `drconfig.yaml`, the error suggests `dr auth set-url` instead.

Surrounding quotes are not stripped from `DATAROBOT_ENDPOINT` — the DataRobot Python and R SDKs read it verbatim, so stripping would let the CLI silently succeed where other SDKs fail. The most common cause is running `$(dr auth export)` instead of `eval "$(dr auth export)"`, which bakes the output's single quotes into the value; the hint is shown for bash/zsh. The same endpoint-vs-token distinction is applied in `dr auth check` and the shared `EnsureAuthenticated` flow.

> [!NOTE]
> This command never starts a login flow and never calls the API, so it is safe to run from a shell startup file. It does not validate the credentials — run `dr auth check` for that.

> [!WARNING]
> The output contains your API token in plain text. Avoid piping it into a shared terminal, a log, or a file that is checked into version control.

### `set-url`

Configure the DataRobot instance URL.

```bash
dr auth set-url [url]
```

**Arguments:**

- `url` (optional) - DataRobot instance URL. For example: `https://app.datarobot.com`

**Interactive mode:**

If you run `dr auth set-url` without providing a URL, the CLI shows a picker. Move with
the arrow keys, confirm with Enter, cancel with Esc. US Cloud is preselected, so the
common case is a single keystroke:

```text
  DataRobot Environment

▶ 🌎 US Cloud
  https://app.datarobot.com

  🌍 EU Cloud
  https://app.eu.datarobot.com

  🌏 Japan Cloud
  https://app.jp.datarobot.com

  🏢 Custom/On-Prem
  Enter your custom DataRobot URL
```

Choosing **Custom/On-Prem** opens a text field for a self-managed instance URL.

**Non-interactive terminals:**

When stdin is not a terminal, or `DATAROBOT_CLI_NON_INTERACTIVE` is set, the picker is
replaced by a plain numbered prompt read from stdin, so redirected input and scripted
runs keep working:

- Enter `1` for US cloud (`https://app.datarobot.com`)
- Enter `2` for EU cloud (`https://app.eu.datarobot.com`)
- Enter `3` for Japan cloud (`https://app.jp.datarobot.com`)
- Type your custom URL for self-managed instances

Passing the URL as an argument (below) avoids the prompt entirely and is the best option
for scripts.

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

### `profile`

Inspect the named profiles stored in `drconfig.yaml`. Read-only: there is no `create` or
`delete` subcommand. A profile is created the first time `dr --profile <name> auth login`
(or `auth set-url`) is run for a name that doesn't exist yet; remove one by editing
`drconfig.yaml` directly.

```bash
dr auth profile list
dr auth profile show [name]
```

**`profile list`** shows the default profile plus every named profile, marking the active
one (selected via `--profile` or `DATAROBOT_CLI_PROFILE`):

```bash
$ dr auth profile list
DataRobot Profiles
──────────────────
╭───────────┬───────────────────────────────────────┬───────┬────────╮
│ NAME      │ ENDPOINT                               │ TOKEN │ ACTIVE │
├───────────┼───────────────────────────────────────┼───────┼────────┤
│ default   │ https://app.datarobot.com/api/v2       │ set   │ ✓      │
│ eu-mtsaas │ https://app.eu.datarobot.com/api/v2    │ set   │        │
╰───────────┴───────────────────────────────────────┴───────┴────────╯
```

**`profile show [name]`** shows one profile's resolved settings &mdash; its own values, falling
back to the default profile's for anything it doesn't define. Defaults to the active
profile when no name is given; pass `default` to see the top-level profile explicitly. The
token value itself is never printed, only whether one is set.

```bash
$ dr auth profile show eu-mtsaas
eu-mtsaas
─────────
  endpoint: https://app.eu.datarobot.com/api/v2
  token: set
  ca-cert: - (inherited from default)
  ssl_verify: - (inherited from default)
```

Both support `--output-format json`. To work against a specific profile with any command,
not just `auth profile`, use `--profile <name>` or `DATAROBOT_CLI_PROFILE=<name>`:

```bash
dr --profile eu-mtsaas auth login
dr --profile eu-mtsaas templates list
DATAROBOT_CLI_PROFILE=eu-mtsaas dr templates list
```

See [Named profiles](../user-guide/configuration.md#named-profiles) for the full
`drconfig.yaml` shape and precedence rules.

## Global options

These options work with all `auth` commands:

```bash
  -v, --verbose        Enable verbose output
      --debug          Enable debug output
      --skip-auth      Skip authentication checks (for advanced users)
      --profile string Named profile from drconfig.yaml to use
  -h, --help           Show help for command
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

If you switch back and forth between the same instances often, a
[named profile](#profile) avoids re-authenticating each time:

```bash
$ dr --profile staging auth set-url https://staging.datarobot.com
$ dr --profile staging auth login
$ dr --profile staging templates list

# Later, back to the default instance — no re-login needed:
$ dr templates list
```

### Debug authentication issues

```bash
# Use debug flag for details
$ dr auth login --debug
[DEBUG] Config file: /Users/username/.config/datarobot/drconfig.yaml
[DEBUG] Current URL: https://app.datarobot.com
[DEBUG] Could not open the browser automatically: ...
[DEBUG] Successfully consumed API key from callback request
...
```

## How authentication works

The CLI hands the browser off to DataRobot and waits on a local callback:

```text
┌──────────┐
│   User   │
└────┬─────┘
     │
     │ dr auth login
     │
     v
┌──────────────────────┐       ┌──────────────┐
│  Local Server        │◄──────┤   Browser    │
│  (localhost:51164)   │       │              │
└────┬─────────────────┘       └──────▲───────┘
     │                                 │
     │                                 │ Opens
     │                                 │
     v                                 │
┌──────────────────────┐               │
│  DataRobot           │───────────────┘
│  /account/           │
│  developer-tools     │
└────┬─────────────────┘
     │
     │ Redirects to localhost:51164/?key=<apiKey>
     │
     v
┌──────────────────┐
│  Config File     │
│  (~/.config/     │
│   datarobot/     │
│   drconfig.yaml) │
└──────────────────┘
```

**Step-by-step:**

1. You run `dr auth login`
2. CLI binds `localhost:51164` **before** opening the browser, so a fast redirect cannot
   arrive before the listener exists
3. Your browser opens to DataRobot's developer tools page
4. You log in and authorize the CLI
5. DataRobot redirects to `http://localhost:51164/?key=<apiKey>`
6. CLI saves the key to your config file (mode `0600`) and shuts the server down

Properties of this flow:

- You authenticate directly with DataRobot, never handing credentials to the CLI
- No password is stored locally
- The callback listener is bound to localhost only

> [!IMPORTANT]
> The API key arrives as a URL query parameter and is stored in plaintext. There is no
> `state` parameter, so any local process able to reach `localhost:51164` while a login
> is in flight could deliver a key. Treat `drconfig.yaml` as a secret and prefer
> `DATAROBOT_API_TOKEN` in shared or automated environments.

## Configuration file

After authentication, credentials are stored in:

**Location:**

- Linux/macOS: `~/.config/datarobot/drconfig.yaml`
- Windows: `%USERPROFILE%\.config\datarobot\drconfig.yaml`

**Format:**

Keys are flat and top-level (the default profile); there is no `datarobot:` or
`preferences:` nesting. The only nested key is `profiles:`, one section per
[named profile](#profile):

```yaml
endpoint: https://app.datarobot.com/api/v2
token: <plaintext_api_key>
api-consumer-tracking-enabled: true

profiles:
  eu-mtsaas:
    endpoint: https://app.eu.datarobot.com/api/v2
    token: <plaintext_api_key>
```

Only allowlisted keys are ever written back (see `config.PersistableKeys`), so transient
flags such as `--yes` never leak into the file.

**Permissions:**

- Created with, and tightened on every write to, `0600` — owner read/write only
- The token is stored in plaintext, so the file permissions are the only thing
  protecting it

> [!NOTE]
> CLI versions before this fix created the file with `0644`, leaving the token readable
> by other users on the machine. Any `dr` command that writes credentials now corrects
> the mode automatically; you can also run `chmod 600` yourself, as shown below.

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
> - The contents of any `.env` file containing `DATAROBOT_API_TOKEN`

### Use per-environment authentication

Prefer a [named profile](#profile) for multiple DataRobot environments &mdash; one
`drconfig.yaml`, no re-authenticating when you switch back:

```bash
# Development
dr --profile dev auth set-url https://dev.datarobot.com
dr --profile dev auth login

# Production
dr --profile prod auth set-url https://prod.datarobot.com
dr --profile prod auth login

dr --profile dev templates list
dr --profile prod templates list
```

Separate config files remain available via `--config`/`DATAROBOT_CLI_CONFIG` for cases
that genuinely need a different file, such as a self-contained CI config:

```bash
export DATAROBOT_CLI_CONFIG=~/.config/datarobot/ci-config.yaml
dr auth set-url https://app.datarobot.com --config $DATAROBOT_CLI_CONFIG
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

# Named profile to use (see 'profile' above)
export DATAROBOT_CLI_PROFILE=eu-mtsaas
```

To go the other way — take the credentials the CLI already has and put them in your shell environment for the DataRobot SDKs and other tools — use [`dr auth export`](#export):

```bash
eval "$(dr auth export)"
```

## Common issues

### Browser doesn't open

**Problem:** Browser fails to open automatically.

**Solution:** The CLI detects this and shows the link in a box — open it yourself, on any
machine that can reach both DataRobot and this machine's `localhost:51164`. Use
`--no-browser` to skip the launch attempt entirely:

```bash
$ dr auth login --no-browser
⠋ Waiting for authorization…

  Open this link to finish signing in:

  ╭────────────────────────────────────────────────────────────────────╮
  │ https://app.datarobot.com/account/developer-tools?cliRedirect=true │
  ╰────────────────────────────────────────────────────────────────────╯
```

`--no-browser` is not reported as a failure: it shows the same framed link as the error
path, without the warning line.

Run with `--debug` to see why the launch failed.

### Port already in use

**Problem:** `localhost:51164` is already in use.

**Solution:** If the holder is another `dr` login, the new process asks it to release the
port and takes over automatically. If an unrelated process holds it, free that port —
there is no fallback, because the redirect target is fixed on the DataRobot side.

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
