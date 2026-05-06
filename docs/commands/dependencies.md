# `dr dependencies` - Dependency management

The `dr dependencies` command checks and installs the prerequisite tools required by your DataRobot template.

## Synopsis

```bash
dr dependencies <command> [flags]
```

## Description

The `dependencies` command verifies that required tools (such as Python, uv, Task, Pulumi, NodeJS ...) are installed and meet the minimum version requirements declared in your project's `.datarobot/cli/versions.yaml` file. Use it to diagnose missing tools before running other commands, or to install missing tools in one step.

## Subcommands

### `check`

Verify that all required template dependencies are installed and meet the minimum version requirements.

```bash
dr dependencies check
```

**Example output:**

```bash
# All dependencies satisfied
$ dr dependencies check
✅ All dependencies are already up to date.

# Missing or wrong-version tools
$ dr dependencies check
 ❌ Missing required tools:

	- uv  (https://docs.astral.sh/uv/getting-started/installation/)

 ⚠️ Wrong versions of tools:

	- Python (minimal: v3.9.0, installed: v3.8.0)
	  https://www.python.org/downloads/
```

Exit code is non-zero when any tool is missing or has an insufficient version.

### `install`

Install missing or out-of-date template dependencies. The command first checks prerequisites, reports what is missing, then prompts you for confirmation before running each tool's platform-specific install command.

```bash
dr dependencies install [flags]
```

**Flags:**

| Flag | Short | Description |
|------|-------|-------------|
| `--yes` | `-y` | Assume "yes" as answer to the install prompt &mdash; skip interactive confirmation. |

**Environment variables:**

| Variable | Description |
|----------|-------------|
| `DATAROBOT_CLI_NON_INTERACTIVE` | Equivalent to `--yes`. Set to any truthy value to skip the install prompt. |

**Example — set for a single command:**

```bash
DATAROBOT_CLI_NON_INTERACTIVE=true dr dependencies install
```

**Example — export for the current shell session:**

```bash
export DATAROBOT_CLI_NON_INTERACTIVE=true
dr dependencies install
```

**Example — interactive:**

```bash
$ dr dependencies install

 ❌ Missing required tools:

	- uv  (https://docs.astral.sh/uv/getting-started/installation/)

Install now? (y/n): y
Installing uv...
```

**Example — non-interactive:**

```bash
$ dr dependencies install --yes
# or
$ DATAROBOT_CLI_NON_INTERACTIVE=true dr dependencies install
```

**Example — nothing to install:**

```bash
$ dr dependencies install
✅ All dependencies are already up to date.
```

## Examples

### Check before running tasks

```bash
# Verify dependencies are in place before starting development
dr dependencies check
dr run dev
```

### Install all missing tools at once

```bash
# Install without being prompted for confirmation
dr dependencies install -y
```

### CI/CD usage

```bash
# In a CI pipeline where stdin is not available
DATAROBOT_CLI_NON_INTERACTIVE=true dr dependencies install
```

## `versions.yaml` schema

The CLI reads tool requirements from `.datarobot/cli/versions.yaml` in your repository. Each top-level key is a tool identifier; the value is a map with the following fields:

```yaml
<tool-key>:
  name: string            # Display name shown in messages
  minimum-version: semver # Minimum acceptable version (e.g. "3.9.0")
  command: string         # Command used to check/run the tool (e.g. "python3")
  url: string             # URL shown when the tool is missing or outdated
  install:
    macos: string         # Install command for macOS (e.g. "brew install python")
    linux: string         # Install command for Linux (e.g. "sudo apt-get install python3")
    windows: string       # Install command for Windows (optional)
```

**Field rules:**

| Field | Required | Format | Notes |
|-------|----------|--------|-------|
| `name` | Yes | string | — |
| `minimum-version` | Yes | semver | Must be a valid semantic version, e.g. `3.9.0` |
| `command` | Yes | string | — |
| `url` | Yes | string | — |
| `install.macos` | Yes | string | — |
| `install.linux` | Yes | string | — |
| `install.windows` | No | string | Optional for Milestone 1 |

**Validation behavior:**

The CLI validates the file on load and logs diagnostics without failing the command:

- Missing required field (`name`, `minimum-version`, `command`, `url`, `install.macos`, `install.linux`) → **WARN** logged.
- `minimum-version` present but not a valid semantic version → **WARN** logged.
- `install` block entirely absent → **WARN** logged.
- Required install command missing **for the current platform** → **ERROR** logged.
- Required install command missing for a non-current platform → **WARN** logged.

**Example:**

```yaml
---
dr:
  name: DataRobot CLI
  minimum-version: 0.2.0
  command: dr self version
  url: https://github.com/datarobot-oss/cli
  install:
    macos: brew install datarobot-cli
    linux: curl -fsSL https://get.datarobot.com/cli | sh
python:
  name: Python
  minimum-version: 3.9.0
  command: python3
  url: https://www.python.org/downloads/
  install:
    macos: brew install python
    linux: sudo apt-get install python3
uv:
  name: uv Python package manager
  minimum-version: 1.7.0
  command: uv self version
  url: https://docs.astral.sh/uv/getting-started/installation/
  install:
    macos: brew install uv
    linux: curl -Ls https://astral.sh/uv/install.sh | sh
```

## See also

- [Quick start](../../README.md#quick-start) - Initial setup guide
- [run](run.md) - Execute template tasks
