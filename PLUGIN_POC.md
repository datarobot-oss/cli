# DR CLI Plugin System POC

This branch (`chas/plugin-poc-v2`) implements a plugin discovery and execution system for the DR CLI.

## Overview

The plugin system allows external executables (like `dr-agentassist`) to be discovered and invoked as subcommands of the `dr` CLI. Plugins are discovered via:

1. Project-local directory: `.datarobot/cli/bin/`
2. System PATH

Plugins must follow the naming convention `dr-<name>` and respond to `--dr-plugin-manifest` with a JSON manifest.

## Installation

### 1. Build the CLI

```bash
cd /path/to/cli
git checkout chas/plugin-poc-v2
task build
```

The binary is built to `./dist/dr`.

### 2. Install the Agent-Assist Plugin

The `dr-agentassist` plugin must be installed and available in PATH:

```bash
cd /path/to/dr-agent-cli
git checkout chas/plugin-poc-v2
cd mdb
uv tool install --force --editable .
```

This installs `dr-agentassist` to `~/.local/bin/` (ensure this is in your PATH).

### 3. Verify Installation

```bash
# Check plugin is in PATH
which dr-agentassist

# Test manifest output
dr-agentassist --dr-plugin-manifest

# Verify CLI discovers the plugin
./dist/dr --help
# Should show "Plugin Commands:" section with "agentassist"

# Run the plugin via CLI
./dist/dr agentassist
```

## How It Works

### Plugin Discovery

On CLI startup, the plugin system:

1. Scans `.datarobot/cli/bin/` for `dr-*` executables
2. Scans each directory in PATH for `dr-*` executables
3. For each found executable, calls `<executable> --dr-plugin-manifest`
4. Parses the JSON manifest to get command name, description, etc.
5. Registers discovered plugins as subcommands

### Plugin Manifest Format

```json
{
  "name": "agentassist",
  "version": "0.1.0",
  "description": "AI agent design, coding, and deployment assistant"
}
```

The manifest requires three fields:
- `name` (required): The command name used to invoke the plugin (e.g., `dr agentassist`)
- `version`: Semantic version of the plugin
- `description`: Short description shown in `dr --help` output

### Plugin Execution

When a plugin command is invoked (e.g., `dr agentassist --help`):

1. CLI finds the registered plugin executable
2. Spawns it as a subprocess with all arguments passed through
3. Forwards stdin/stdout/stderr
4. Forwards SIGINT/SIGTERM signals
5. Propagates the plugin's exit code

## Configuration Sharing

The plugin reads credentials from the DR CLI config file as a fallback:

**Priority order:**
1. Environment variables (`DATAROBOT_API_TOKEN`, `DATAROBOT_ENDPOINT`)
2. `.env` file
3. DR CLI config (`~/.config/datarobot/drconfig.yaml`)
4. Field defaults

This means if you've authenticated with `dr auth login`, the agent-assist plugin will use those credentials automatically.

## New Files

```
cli/
├── internal/plugin/
│   ├── types.go          # Manifest structs
│   ├── discover.go       # Plugin discovery logic
│   ├── discover_test.go  # Discovery unit tests
│   ├── exec.go           # Plugin execution with signal handling
│   └── exec_test.go      # Execution unit tests
├── internal/repo/
│   └── paths.go          # Modified: added LocalPluginDir constant
└── cmd/
    └── root.go           # Modified: plugin registration
```

## Testing

```bash
# Run linter
task lint

# Run tests
task test

# Build and test plugin discovery
task build
./dist/dr --help
./dist/dr agentassist --dr-plugin-manifest
```

