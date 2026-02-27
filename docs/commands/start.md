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

The command streamlines the process of getting your DataRobot application up and running. It runs the following steps in order:

1. **Starting application quickstart process**&mdash;displays the quickstart flow.
2. **Checking DataRobot CLI version**&mdash;verifies that your CLI version meets the template's minimum requirements (from `.datarobot/cli/versions.yaml`). If not, prompts to run `dr self update`.
3. **Checking template prerequisites**&mdash;verifies that required tools (e.g. Task, Git) are installed.
4. **Checking repository setup**&mdash;verifies you're in a DataRobot repository (contains `.datarobot/`). If not, the command launches the interactive `dr templates setup` wizard; after setup completes, `dr start` runs again in the cloned directory.
5. **Finding and executing start command**&mdash;searches for a start command in this order:
   - **`task start`**&mdash;if the Taskfile defines a `start` task, it runs immediately (no confirmation).
   - **Quickstart script**&mdash;if a script in `.datarobot/cli/bin/` matches `quickstart*`, you're prompted to run it (unless `--yes` is used), then it executes.
   - **Neither found**&mdash;if you're already in a repository but no start command or script exists, the command shows a message and exits. It does not launch the setup wizard in this case.

This command is designed to work intelligently with your template's structure. If you're not in a DataRobot repository, it launches template setup; once in a repository, it runs `task start` or a quickstart script when available.

## Aliases

- `dr start`
- `dr quickstart`

## Options

```bash
  -y, --yes     Skip confirmation prompts and execute immediately
  -h, --help    Show help information
```

### Global options

All [global options](README.md#global-options) are also available.

## Start command detection

The `dr start` command looks for a start command in the following order:

1. **`task start` command** (highest priority)&mdash;If your template's Taskfile defines a `start` task, it will be executed automatically.
2. **Quickstart scripts**&mdash;If no `task start` is found, the command searches for quickstart scripts.

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
DataRobot AI Application Quickstart

  ✓ Starting application quickstart process...
  ✓ Checking DataRobot CLI version...
  ✓ Checking template prerequisites...
  ✓ Checking repository setup...
  → Finding and executing start command...

Found quickstart script at: .datarobot/cli/bin/quickstart.sh

Press 'y' or ENTER to confirm, 'n' to cancel
```

If you're not in a DataRobot repository, the repository step will trigger template setup:

```text
  ✓ Starting application quickstart process...
  ✓ Checking DataRobot CLI version...
  ✓ Checking template prerequisites...
  → Checking repository setup...

Not in a DataRobot repository. Launching template setup...
```

The command then launches the interactive `dr templates setup` wizard. After you select and clone a template, `dr start` runs again in the cloned directory.

If you're already in a repository but no start command or quickstart script exists:

```text
  ✓ Starting application quickstart process...
  ✓ Checking DataRobot CLI version...
  ✓ Checking template prerequisites...
  ✓ Checking repository setup...
  → Finding and executing start command...

No start command or quickstart script found.
This template may not yet fully support the DataRobot CLI.
Please check the template README for more information on how to get started.
```

The command then exits; the setup wizard is not launched in this case.

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

### CLI version check and update

If your template defines a minimum CLI version in `.datarobot/cli/versions.yaml`, `dr start` checks the installed version before continuing.

If your CLI version does not meet the minimum, `dr start` prompts you to run `dr self update`.

Example prompt:

```text
DataRobot AI Application Quickstart

  ✓ Starting application quickstart process...
  ✓ Checking DataRobot CLI version...

DataRobot CLI (minimal: v0.2.0, installed: v0.1.0)
Do you want to update it now?

Press 'y' or ENTER to confirm, 'n' to cancel
```

### State tracking

The `dr start` command automatically tracks when it runs successfully by updating a state file with:

- Timestamp of when the command last started (ISO 8601 format)
- CLI version used

This state information is stored in `.datarobot/cli/state.yaml` within the repository. State tracking is automatic and transparent. No manual intervention is required.

The state file helps other commands (like `dr templates setup`) know that you've already run `dr start`, allowing them to skip redundant setup steps.

### When `task start` exists

1. `task start` command is detected in the Taskfile
2. Command executes immediately (no confirmation prompt)
3. Command completes when the task finishes
4. State file is updated with current timestamp and CLI version

### When a quickstart script exists (but no `task start`)

1. Script is detected in `.datarobot/cli/bin/`
2. User is prompted for confirmation (unless `--yes` or `-y` is used)
3. If user confirms (or `--yes` is specified), script executes with full terminal control
4. Command completes when script finishes
5. State file is updated with current timestamp and CLI version

If the user declines to execute the script, the command exits gracefully and still updates the state file.

### When no start command or script exists (and you're in a repository)

1. No `task start` command is found in the Taskfile
2. No executable quickstart script is found in `.datarobot/cli/bin/`
3. The command displays a message that no start command or quickstart script was found and suggests checking the template README
4. The command exits; the state file is not updated (nothing was run)

**Note:** If you're **not** in a DataRobot repository, the **repository check** step (before "Finding and executing start command") triggers the template setup wizard; that case is separate from "no start command or script."

### Prerequisites checked

The "Checking template prerequisites" step verifies that tools required by the template (e.g. Task, Git) are installed. If any are missing, the command exits with an error.

Required tools and minimum versions can be configured in the template by creating `.datarobot/cli/versions.yaml`:

```yaml
---
dr:
  name: DataRobot CLI
  minimum-version: 0.2.0
  command: dr self version
  url: https://github.com/datarobot-oss/cli
uv:
  name: uv Python package manager
  minimum-version: 1.7.0
  command: uv self version
  url: https://docs.astral.sh/uv/getting-started/installation/
```

The repository check runs before start-command detection. If the current directory is not within a DataRobot repository (no `.datarobot/` directory), the command launches the template setup wizard instead of continuing to look for a start command.

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

- [Template system](../template-system/README.md)&mdash;understanding templates and the setup wizard.
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
