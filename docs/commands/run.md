# `dr run` - Task execution

Execute tasks defined in application templates.

## Quick start

For most users, running tasks is straightforward:

```bash
# List available tasks
dr run --list

# Run a task (e.g., start development server)
dr run dev
```

The command automatically discovers tasks from your template's Taskfiles and executes them with your environment configuration.

> [!NOTE]
> **First time?** If you're new to the CLI, start with the [Quick start](../../README.md#quick-start) for step-by-step setup instructions.

## Synopsis

```bash
dr run [TASK_NAME...] [flags]
```

## Description

The `dr run` command executes tasks defined in Taskfiles within DataRobot application templates. It automatically discovers component Taskfiles and aggregates them into a unified task execution environment.

The command provides a convenient way to execute common application tasks such as starting development servers, running tests, building containers, and deploying applications. It works by discovering Taskfiles in your template directory and generating a consolidated task runner configuration.

**Key features:**

- **Automatic discovery**&mdash;finds and aggregates Taskfiles from template components.
- **Template validation**&mdash;verifies you're in a DataRobot template directory.
- **Conflict detection**&mdash;prevents dotenv directive conflicts in nested Taskfiles.
- **Parallel execution**&mdash;run multiple tasks simultaneously.
- **Watch mode**&mdash;automatically re-run tasks when files change.

## Template requirements

To use `dr run`, your directory must meet these requirements:

1. **Contains a .env file**&mdash;indicates you're in a DataRobot template directory.
2. **Contains Taskfiles**&mdash;component directories with `Taskfile.yaml` or `Taskfile.yml` files.
3. **No dotenv conflicts**&mdash;component Taskfiles cannot have their own `dotenv` directives.

If these requirements aren't met, the command provides clear error messages explaining the issue.

## Options

```bash
  -l, --list              List all available tasks
  -d, --dir string        Directory to look for tasks (default ".")
  -p, --parallel          Run tasks in parallel
  -C, --concurrency int   Number of concurrent tasks to run (default 2)
  -w, --watch             Enable watch mode for the given task
  -y, --yes               Assume "yes" as answer to all prompts
  -x, --exit-code         Pass-through the exit code of the task command
  -s, --silent            Disable echoing
  -h, --help              Help for run
```

## Global options

```bash
  -v, --verbose    Enable verbose output
      --debug      Enable debug output
```

## Examples

### List available tasks

```bash
dr run --list
```

Output:

```text
Available tasks:
* dev        Start development server
* test       Run tests  
* lint       Run linters
* build      Build Docker container
* deploy     Deploy to DataRobot
```

### Run a single task

```bash
dr run dev
```

Starts the development server defined in your template's Taskfile.

### Run multiple tasks sequentially

```bash
dr run lint test
```

Runs the lint task, then the test task in sequence.

### Run multiple tasks in parallel

```bash
dr run lint test --parallel
```

Runs lint and test tasks simultaneously.

### Run with watch mode

```bash
dr run dev --watch
```

Runs the development server and automatically restarts it when source files change.

### Control concurrency

```bash
dr run task1 task2 task3 --parallel --concurrency 3
```

Runs up to 3 tasks concurrently.

### Silent execution

```bash
dr run build --silent
```

Runs the build task without echoing commands.

### Pass-through exit codes

```bash
dr run test --exit-code
```

Exits with the same code as the task command (useful in CI/CD).

## Task discovery

The `dr run` command discovers tasks in this order:

1. **Check for .env file**&mdash;verifies you're in a template directory.
2. **Scan for Taskfiles**&mdash;finds `Taskfile.yaml` or `Taskfile.yml` files up to 2 levels deep.
3. **Validate dotenv directives**&mdash;ensures component Taskfiles don't have conflicting `dotenv` declarations.
4. **Generate Taskfile.gen.yaml**&mdash;creates a unified task configuration.
5. **Execute tasks**&mdash;runs the requested tasks using the `task` binary.

### Directory structure

```text
my-template/
├── .env                          # Required: template marker
├── Taskfile.gen.yaml            # Generated: consolidated tasks
├── backend/
│   ├── Taskfile.yaml            # Component tasks (no dotenv)
│   └── src/
└── frontend/
    ├── Taskfile.yaml            # Component tasks (no dotenv)
    └── src/
```

### Generated Taskfile

The CLI generates `Taskfile.gen.yaml` with this structure:

```yaml
version: '3'

dotenv: [".env"]

includes:
  backend:
    taskfile: ./backend/Taskfile.yaml
    dir: ./backend
  frontend:
    taskfile: ./frontend/Taskfile.yaml
    dir: ./frontend
```

This allows you to run component tasks with prefixes:

```bash
dr run backend:build
dr run frontend:dev
```

## Error handling

### Not in a template directory

If you run `dr run` outside a DataRobot template:

```text
You don't seem to be in a DataRobot Template directory.
This command requires a .env file to be present.
```

**Solution:** Navigate to a template directory or run `dr templates setup` to create one.

### Dotenv directive conflict

If a component Taskfile has its own `dotenv` directive:

```text
Error: Cannot generate Taskfile because an existing Taskfile already has a dotenv directive.
existing Taskfile already has dotenv directive: backend/Taskfile.yaml
```

**Solution:** Remove the `dotenv` directive from component Taskfiles. The generated `Taskfile.gen.yaml` handles environment variables.

### Task binary not found

If the `task` binary isn't installed:

```text
"task" binary not found in PATH. Please install Task from https://taskfile.dev/installation/
```

**Solution:** Install Task following the instructions at [taskfile.dev/installation](https://taskfile.dev/installation/).

### No tasks found

If no Taskfiles exist in component directories:

```text
file does not exist
Error: failed to list tasks: exit status 1
```

**Solution:** Add Taskfiles to your template components or use `dr templates setup` to start with a pre-configured template.

## Task definitions

Tasks are defined in component `Taskfile.yaml` files using Task's syntax.

### Basic task

```yaml
version: '3'

tasks:
  dev:
    desc: Start development server
    cmds:
      - python -m uvicorn src.app.main:app --reload
```

### Task with dependencies

```yaml
tasks:
  build:
    desc: Build Docker container
    cmds:
      - docker build -t {{.APP_NAME}} .

  deploy:
    desc: Deploy application
    deps: [build]
    cmds:
      - docker push {{.APP_NAME}}
      - kubectl apply -f deploy.yaml
```

### Task with environment variables

```yaml
tasks:
  test:
    desc: Run tests with coverage
    env:
      PYTEST_ARGS: "--cov=src --cov-report=html"
    cmds:
      - pytest {{.PYTEST_ARGS}}
```

### Task with preconditions

```yaml
tasks:
  deploy:
    desc: Deploy to production
    preconditions:
      - sh: test -f .env
        msg: ".env file is required"
      - sh: test -n "$DATAROBOT_ENDPOINT"
        msg: "DATAROBOT_ENDPOINT must be set"
    cmds:
      - ./deploy.sh
```

## Best practices

### Descriptive task names

Use clear, action-oriented task names:

```yaml
tasks:
  dev:           # ✅ Clear and concise
    desc: Start development server
  
  test:unit:     # ✅ Namespaced for organization
    desc: Run unit tests
  
  lint:python:   # ✅ Specific and descriptive
    desc: Run Python linters
```

### Useful descriptions

Provide helpful task descriptions:

```yaml
tasks:
  deploy:
    desc: Deploy application to DataRobot (requires authentication)
    cmds:
      - ./deploy.sh
```

### Common task names

Use standard names for common operations:

- `dev`&mdash;start development server.
- `build`&mdash;build application or container.
- `test`&mdash;run test suite.
- `lint`&mdash;run linters and formatters.
- `deploy`&mdash;deploy to target environment.
- `clean`&mdash;clean build artifacts.

### Environment variable usage

Reference `.env` variables in tasks:

```yaml
tasks:
  deploy:
    desc: Deploy {{.APP_NAME}} to {{.DEPLOYMENT_TARGET}}
    cmds:
      - echo "Deploying to $DATAROBOT_ENDPOINT"
      - ./deploy.sh
```

### Silent tasks

Use `silent: true` for tasks that don't need output:

```yaml
tasks:
  check:version:
    desc: Check CLI version
    silent: true
    cmds:
      - dr version
```

## Integration with other commands

### With dr templates

```bash
# Set up template (clones and configures)
dr templates setup
cd my-app

# Configure environment
dr dotenv setup

# Run tasks
dr run dev
```

### With dr dotenv

```bash
# Update environment variables
dr dotenv setup

# Run with updated configuration
dr run deploy
```

### In CI/CD pipelines

```bash
#!/bin/bash
# ci-pipeline.sh

set -e

# Run tests
dr run test --exit-code --silent

# Run linters
dr run lint --exit-code --silent

# Build
dr run build --silent
```

## Troubleshooting

### Tasks not found

**Problem:** `dr run --list` shows no tasks.

**Causes:**

- No Taskfiles in component directories.
- Taskfiles at wrong depth (deeper than 2 levels).

**Solution:**

```bash
# Check for Taskfiles
find . -name "Taskfile.y*ml" -maxdepth 3

# Ensure Taskfiles are in component directories
# Correct: ./backend/Taskfile.yaml
# Wrong: ./backend/src/Taskfile.yaml
```

### Environment variables not loading

**Problem:** Tasks can't access environment variables.

**Causes:**

- Missing `.env` file.
- Variables not exported.

**Solution:**

```bash
# Verify .env exists
ls -la .env

# Check variables are set
source .env
env | grep DATAROBOT
```

### Task execution fails

**Problem:** Task runs but fails with errors.

**Solution:**

```bash
# Enable verbose output
dr run task-name --verbose

# Enable debug output
dr run task-name --debug

# Check task definition
cat component/Taskfile.yaml
```

### Permission denied errors

**Problem:** Tasks fail with permission errors.

**Solution:**

```bash
# Make scripts executable
chmod +x scripts/*.sh

# Check file permissions
ls -l scripts/
```

## See also

- [Template system overview](../template-system/README.md)&mdash;understanding templates.
- [Task definitions](../template-system/structure.md#task-definitions)&mdash;creating Taskfiles.
- [Environment variables](../template-system/environment-variables.md)&mdash;managing configuration.
- [dr dotenv](dotenv.md)&mdash;environment variable management.
- [Task documentation](https://taskfile.dev/)&mdash;official Task runner docs.
