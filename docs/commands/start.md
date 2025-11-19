# `dr start` - Application quickstart

Run the application quickstart process for the current template.

## Quick start

For most users, getting started is a single command:

```bash
# Run the quickstart process (interactive)
dr start
```

The command automatically detects your template's configuration and either runs a custom quickstart script or launches the interactive setup wizard.

> [!NOTE]
> **First time?** If you're new to the CLI, start with the [Quick start](../../README.md#quick-start) for step-by-step setup instructions.

## Synopsis

```bash
dr start [flags]
```

## Description

The `start` command (also available as `quickstart`) provides an automated way to initialize and launch your DataRobot application. It performs several checks and either executes a template-specific quickstart script or seamlessly launches the interactive template setup wizard.

The command streamlines the process of getting your DataRobot application up and running. It automates the following workflow:

1. **Prerequisite checks**&mdash;verifies that required tools are installed.
2. **Quickstart script detection**&mdash;searches for template-specific quickstart scripts in `.datarobot/cli/bin/` (if in a DataRobot repository).
3. **Execution**&mdash;either:
   - Runs the quickstart script if found (after user confirmation, unless `--yes` is specified).
   - Launches the interactive `dr templates setup` wizard if no script is found or not in a repository.

This command is designed to work intelligently with your template's structure. Templates can optionally provide custom quickstart scripts to automate their specific initialization needs. If you're not in a DataRobot repository or no script exists, the command gracefully falls back to the standard setup wizard.

## Aliases

- `dr start`
- `dr quickstart`

## Options

```bash
  -y, --yes     Skip confirmation prompts and execute immediately
  -h, --help    Show help information
```

### Global options

All [global options](README.md#global-flags) are also available.

## Quickstart scripts

### Location

Quickstart scripts must be placed in:

```text
.datarobot/cli/bin/
```

### Naming convention

Scripts must start with `quickstart` (case-sensitive):

- ✅ `quickstart`
- ✅ `quickstart.sh`
- ✅ `quickstart.py`
- ✅ `quickstart-dev`
- ❌ `Quickstart.sh` (wrong case)
- ❌ `start.sh` (wrong name)

If there are multiple scripts matching the pattern, the first one found in lexicographical order will be executed.

### Platform-specific requirements

**Unix/Linux/macOS:**

- Script must have executable permissions (`chmod +x`)
- Can be any executable file (shell script, Python, compiled binary, etc.)

**Windows:**

- Must have executable extension: `.exe`, `.bat`, `.cmd`, or `.ps1`

## Examples

### Basic usage

Run the quickstart process interactively:

```bash
dr start
```

If a quickstart script is found:

```text
DataRobot Quickstart

  ✓ Starting application quickstart process...
  ✓ Checking template prerequisites...
  ✓ Locating quickstart script...
  → Executing quickstart script...

Quickstart found at: .datarobot/cli/bin/quickstart.sh. Will proceed with execution...

Press 'y' or ENTER to confirm, 'n' to cancel
```

If no quickstart script is found:

```text
DataRobot Quickstart

  ✓ Starting application quickstart process...
  ✓ Checking template prerequisites...
  ✓ Locating quickstart script...
  → Executing quickstart script...

No quickstart script found. Will proceed with template setup...
```

The command will then seamlessly launch the interactive setup wizard.

### Non-interactive mode

Skip all prompts and execute immediately:

```bash
dr start --yes
```

or

```bash
dr start -y
```

This is useful for:

- CI/CD pipelines
- Automated deployments
- Scripted workflows

### Using the alias

```bash
dr quickstart
```

## Behavior

### State tracking

The `dr start` command automatically tracks when it runs successfully by updating a state file with:

- Timestamp of when the command last started (ISO 8601 format)
- CLI version used

This state information is stored in `.datarobot/state/info.yml` within the repository. State tracking is automatic and transparent. No manual intervention is required.

The state file helps other commands (like `dr templates setup`) know that you've already run `dr start`, allowing them to skip redundant setup steps.

### When a quickstart script exists

1. Script is detected in `.datarobot/cli/bin/`
2. User is prompted for confirmation (unless `--yes` or `-y` is used)
3. If user confirms (or `--yes` is specified), script executes with full terminal control
4. Command completes when script finishes
5. State file is updated with current timestamp and CLI version

If the user declines to execute the script, the command exits gracefully and still updates the state file.

### When no quickstart script exists

1. No script is found in `.datarobot/cli/bin/` (or not in a DataRobot repository)
2. User is notified
3. User is prompted for confirmation (unless `--yes` or `-y` is used)
4. If user confirms (or `--yes` is specified), interactive `dr templates setup` wizard launches automatically
5. User completes template configuration through the wizard
6. State file is updated with current timestamp and CLI version

If the user declines, the command exits gracefully and still updates the state file.

### Prerequisites checked

Before proceeding, the command verifies:

- ✅ Required tools are installed (Git, etc.)

When searching for a quickstart script, the command checks:

- ✅ Current directory is within a DataRobot repository (contains `.datarobot/` directory)

If the repository check fails, the command automatically launches the template setup wizard instead of exiting with an error.

## Error handling

### Not in a DataRobot repository

If you're not in a DataRobot repository, the command automatically launches the template setup wizard:

```bash
$ dr start
# Automatically launches: dr templates setup
```

No manual intervention is needed - the command handles this gracefully.

### Missing prerequisites

```bash
$ dr start
Error: required tool 'git' not found

# Solution: Install the missing tool
```

### Script execution failure

If a quickstart script fails, the error is displayed and the command exits. Check the script's output for details.

## When to use `dr start`

### ✅ Good use cases

- **First-time setup**&mdash;initializing a newly cloned template or starting from scratch.
- **Quick restart**&mdash;restarting development after a break.
- **Onboarding**&mdash;helping new team members get started quickly.
- **CI/CD**&mdash;automating application initialization in pipelines.
- **General entry point**&mdash;universal command that works whether you have a template or not.

### ❌ When not to use

- **Making configuration changes**&mdash;use `dr dotenv` to modify environment variables.
- **Running specific tasks**&mdash;use `dr run <task>` for targeted task execution.

## See also

- [`dr templates setup`](templates.md#setup)&mdash;interactive template setup wizard.
- [`dr run`](run.md)&mdash;execute specific application tasks.
- [`dr dotenv`](dotenv.md)&mdash;manage environment configuration.
- [Template Structure](../template-system/structure.md)&mdash;understanding template organization.

## Tips

### Creating a custom quickstart script

1. **Create the directory structure:**

   ```bash
   mkdir -p .datarobot/cli/bin
   ```

2. **Create your script:**

   ```bash
   # Create the script
   cat > .datarobot/cli/bin/quickstart.sh <<'EOF'
   #!/bin/bash
   echo "Starting my custom quickstart..."
   dr run build
   dr run dev
   EOF
   ```

3. **Make it executable:**

   ```bash
   chmod +x .datarobot/cli/bin/quickstart.sh
   ```

4. **Test it:**

   ```bash
   dr start --yes
   ```

### Best practices

- **Keep scripts simple**&mdash;focus on essential initialization steps.
- **Provide clear output**&mdash;use echo statements to show progress.
- **Handle errors gracefully**&mdash;use `set -e` in bash scripts to exit on errors.
- **Check prerequisites**&mdash;verify .env exists and required tools are installed.
- **Make it idempotent**&mdash;script should be safe to run multiple times.
- **Document behavior**&mdash;add comments explaining what the script does.
