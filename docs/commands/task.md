# dr task

Manage Taskfile composition and task execution for DataRobot templates.

## Synopsis

```bash
dr task [command] [flags]
```

## Description

The `task` command group provides utilities for working with Taskfiles in DataRobot application templates. It includes subcommands for composing unified Taskfiles from multiple component Taskfiles and executing tasks.

## Subcommands

- **[compose](#dr-task-compose)**&mdash;compose a unified Taskfile from component Taskfiles.
- **[list](#dr-task-list)**&mdash;list available tasks.
- **[run](#dr-task-run)**&mdash;execute template tasks.

## dr task compose

Generate a root `Taskfile.yaml` by discovering and aggregating Taskfiles from subdirectories.

### Synopsis

```bash
dr task compose [flags]
```

### Description

The `compose` command automatically discovers Taskfiles in component directories and generates a unified root `Taskfile.yaml` that includes all components and aggregates common tasks. This allows you to run tasks across multiple components from a single entry point.

**Key features:**

- **Automatic discovery**&mdash;finds Taskfiles up to 2 levels deep in subdirectories.
- **Task aggregation**&mdash;discovers common tasks (lint, install, dev, deploy) and creates top-level tasks that delegate to components.
- **Template support**&mdash;uses customizable Go templates for flexible Taskfile generation.
- **Auto-discovery**&mdash;automatically detects `.Taskfile.template` in the root directory.
- **Gitignore integration**&mdash;adds generated Taskfile to `.gitignore` automatically.

### Options

```bash
  -t, --template string   Path to custom Taskfile template
  -h, --help              Help for compose
```

### Global options

```bash
  -v, --verbose    Enable verbose output
      --debug      Enable debug output
```

### Template requirements

To use `dr task compose`, your directory must meet these requirements:

1. **Contains a .env file**&mdash;indicates you're in a DataRobot template directory.
2. **Contains component Taskfiles**&mdash;subdirectories with `Taskfile.yaml` or `Taskfile.yml` files.
3. **No dotenv conflicts**&mdash;component Taskfiles cannot have their own `dotenv` directives.

### Directory structure

Expected directory structure:

```
my-template/
├── .env                          # Required: template marker
├── .Taskfile.template           # Optional: custom template
├── Taskfile.yaml                # Generated: unified taskfile
├── backend/
│   ├── Taskfile.yaml            # Component tasks
│   └── src/
├── frontend/
│   ├── Taskfile.yaml            # Component tasks
│   └── src/
└── infra/
    ├── Taskfile.yaml            # Component tasks
    └── terraform/
```

### Examples

#### Basic composition

Generate a Taskfile using the default embedded template:

```bash
dr task compose
```

Output:
```
Generated file saved to: Taskfile.yaml
Added /Taskfile.yaml line to .gitignore
```

This creates a `Taskfile.yaml` with:
- Environment configuration
- Includes for all discovered components
- Aggregated tasks (lint, install, dev, deploy)

#### With auto-discovered template

If `.Taskfile.template` exists in the root directory:

```bash
dr task compose
```

Output:
```
Using auto-discovered template: .Taskfile.template
Generated file saved to: Taskfile.yaml
```

The command automatically uses your custom template.

#### With custom template

Specify a custom template explicitly:

```bash
dr task compose --template templates/custom.yaml
```

This uses your custom template for generation instead of the embedded default.

#### Template in subdirectory

```bash
dr task compose --template .datarobot/taskfile.template
```

### Generated Taskfile structure

The default generated `Taskfile.yaml` includes:

```yaml
---
# https://taskfile.dev
version: '3'
env:
  ENV: testing
dotenv: ['.env', '.env.{{.ENV}}']

includes:
  backend:
    taskfile: ./backend/Taskfile.yaml
    dir: ./backend
  frontend:
    taskfile: ./frontend/Taskfile.yaml
    dir: ./frontend
  infra:
    taskfile: ./infra/Taskfile.yaml
    dir: ./infra

tasks:
  default:
    desc: "ℹ️ Show all available tasks (run `task --list-all` to see hidden tasks)"
    cmds:
      - task --list --sort none
    silent: true

  start:
    desc: "💻 Prepare local development environment"
    cmds:
      - dr dotenv setup
      - task: install

  lint:
    desc: "🧹 Run linters"
    cmds:
      - task: backend:lint
      - task: frontend:lint

  install:
    desc: "🛠️ Install all dependencies"
    cmds:
      - task: backend:install
      - task: frontend:install
      - task: infra:install

  test:
    desc: "🧪 Run tests across all components"
    cmds:
      - task: backend:test
      - task: frontend:test

  dev:
    desc: "🚀 Run all services together"
    cmds:
      - |
        task backend:dev &
        sleep 3
        task frontend:dev &
        sleep 8
        echo "✅ All servers started!"
        wait

  deploy:
    desc: "🚀 Deploy all services"
    cmds:
      - task: infra:deploy
      - task: backend:deploy

  deploy-dev:
    desc: "🚀 Deploy all services to development"
    cmds:
      - task: infra:deploy-dev
      - task: backend:deploy-dev
```

### Task aggregation

The compose command discovers these common tasks in component Taskfiles:

- **lint**&mdash;code linting and formatting.
- **install**&mdash;dependency installation.
- **test**&mdash;running test suites.
- **dev**&mdash;development server startup.
- **deploy**&mdash;production deployment operations.
- **deploy-dev**&mdash;development deployment operations.

For each discovered task type, it creates a top-level task that delegates to all components that have that task.

### Custom templates

Create a custom template to control the generated Taskfile structure.

#### Template file example

Save as `.Taskfile.template`:

```yaml
---
version: '3'
env:
  ENV: production
dotenv: ['.env', '.env.{{.ENV}}']

includes:
  {{- range .Includes }}
  {{ .Name }}:
    taskfile: {{ .Taskfile }}
    dir: {{ .Dir }}
  {{- end }}

tasks:
  default:
    desc: "Show available tasks"
    cmds:
      - task --list

  {{- if .HasLint }}
  lint:
    desc: "Run linters"
    cmds:
      {{- range .LintComponents }}
      - task: {{ . }}:lint
      {{- end }}
  {{- end }}

  {{- if .HasInstall }}
  install:
    desc: "Install dependencies"
    cmds:
      {{- range .InstallComponents }}
      - task: {{ . }}:install
      {{- end }}
  {{- end }}

  {{- if .HasTest }}
  test:
    desc: "Run tests"
    cmds:
      {{- range .TestComponents }}
      - task: {{ . }}:test
      {{- end }}
  {{- end }}

  # Custom task
  check:
    desc: "Run all checks"
    cmds:
      - task: lint
      - task: test
```

#### Template variables

Templates have access to these variables:

**Includes (array):**
- `.Name`&mdash;component name (e.g., "backend").
- `.Taskfile`&mdash;relative path to Taskfile (e.g., "./backend/Taskfile.yaml").
- `.Dir`&mdash;relative directory path (e.g., "./backend").

**Task flags (boolean):**
- `.HasLint`&mdash;true if any component has a lint task.
- `.HasInstall`&mdash;true if any component has an install task.
- `.HasTest`&mdash;true if any component has a test task.
- `.HasDev`&mdash;true if any component has a dev task.
- `.HasDeploy`&mdash;true if any component has a deploy task.
- `.HasDeployDev`&mdash;true if any component has a deploy-dev task.

**Task components (arrays):**
- `.LintComponents`&mdash;component names with lint tasks.
- `.InstallComponents`&mdash;component names with install tasks.
- `.TestComponents`&mdash;component names with test tasks.
- `.DevComponents`&mdash;component names with dev tasks.
- `.DeployComponents`&mdash;component names with deploy tasks.
- `.DeployDevComponents`&mdash;component names with deploy-dev tasks.

**Development ports (array):**
- `.DevPorts[].Name`&mdash;service name.
- `.DevPorts[].Port`&mdash;port number.

#### Example: minimal template

```yaml
version: '3'
dotenv: ['.env']

includes:
  {{- range .Includes }}
  {{ .Name }}:
    taskfile: {{ .Taskfile }}
    dir: {{ .Dir }}
  {{- end }}

tasks:
  default:
    cmds:
      - task --list
```

#### Example: extensive aggregation

```yaml
version: '3'
dotenv: ['.env']

includes:
  {{- range .Includes }}
  {{ .Name }}:
    taskfile: {{ .Taskfile }}
    dir: {{ .Dir }}
  {{- end }}

tasks:
  {{- if .HasLint }}
  lint:
    desc: "Run all linters"
    cmds:
      {{- range .LintComponents }}
      - task: {{ . }}:lint
      {{- end }}
  {{- end }}

  {{- if .HasInstall }}
  install:
    desc: "Install all dependencies"
    cmds:
      {{- range .InstallComponents }}
      - task: {{ . }}:install
      {{- end }}
  {{- end }}

  {{- if .HasTest }}
  test:
    desc: "Run all tests"
    cmds:
      {{- range .TestComponents }}
      - task: {{ . }}:test
      {{- end }}
  {{- end }}

  {{- if .HasDev }}
  dev:
    desc: "Start all services"
    cmds:
      {{- range .DevComponents }}
      - task: {{ . }}:dev
      {{- end }}
  {{- end }}

  {{- if .HasDeploy }}
  deploy:
    desc: "Deploy to production"
    cmds:
      {{- range .DeployComponents }}
      - task: {{ . }}:deploy
      {{- end }}
  {{- end }}

  {{- if .HasDeployDev }}
  deploy-dev:
    desc: "Deploy to development"
    cmds:
      {{- range .DeployDevComponents }}
      - task: {{ . }}:deploy-dev
      {{- end }}
  {{- end }}

  ci:
    desc: "Run CI pipeline"
    cmds:
      - task: lint
      - task: test
      - task: build
```

### Gitignore integration

The compose command automatically adds the generated Taskfile to `.gitignore`:

```gitignore
/Taskfile.yaml
```

This prevents committing the generated file to version control. Each developer generates their own version based on their local component structure.

If you want to commit the generated Taskfile, remove it from `.gitignore`.

### Error handling

#### Not in a template directory

```
You don't seem to be in a DataRobot Template directory.
This command requires a .env file to be present.
```

**Solution:** Navigate to a template directory or run `dr templates setup`.

#### No Taskfiles found

```
no Taskfiles found in child directories
```

**Solution:** Add Taskfiles to component directories or adjust your directory structure.

#### Dotenv conflict

```
Error: Cannot generate Taskfile because an existing Taskfile already has a dotenv directive.
existing Taskfile already has dotenv directive: backend/Taskfile.yaml
```

**Solution:** Remove `dotenv` directives from component Taskfiles. The root Taskfile handles environment loading.

#### Template not found

```
Error: template file not found: /path/to/template.yaml
```

**Solution:** Check the template path and ensure the file exists.

### Best practices

#### Keep components independent

Each component Taskfile should be self-contained:

```yaml
# backend/Taskfile.yaml
version: '3'

tasks:
  dev:
    desc: Start backend server
    cmds:
      - python -m uvicorn src.app.main:app --reload

  test:
    desc: Run tests
    cmds:
      - pytest

  lint:
    desc: Run linters
    cmds:
      - ruff check .
      - mypy .
```

#### Use consistent task names

Use the same task names across components for automatic aggregation:

- `lint`&mdash;linting.
- `install`&mdash;dependency installation.
- `test`&mdash;testing.
- `dev`&mdash;development server.
- `build`&mdash;building artifacts.
- `deploy`&mdash;production deployment.
- `deploy-dev`&mdash;development deployment.

#### Commit custom templates

If using a custom template, commit it to version control:

```bash
git add .Taskfile.template
git commit -m "Add custom Taskfile template"
```

#### Document custom variables

If your template uses custom variables, document them:

```yaml
# .Taskfile.template
# 
# Custom variables:
# - PROJECT_NAME: Set in .env
# - DEPLOY_TARGET: Set in .env
#
version: '3'
# ...
```

#### Test template changes

After modifying a template, regenerate and test:

```bash
dr task compose --template .Taskfile.template
task --list
task dev
```

### Workflow integration

#### Initial setup

```bash
# Clone template
dr templates clone python-fullstack my-app
cd my-app

# Set up environment
dr dotenv setup

# Generate Taskfile
dr task compose

# View available tasks
task --list
```

#### Development workflow

```bash
# Add new component
mkdir new-service
cat > new-service/Taskfile.yaml << 'EOF'
version: '3'
tasks:
  dev:
    desc: Start new service
    cmds:
      - echo "Starting service..."
EOF

# Regenerate Taskfile
dr task compose

# Run all services
task dev
```

#### Template updates

When components change:

```bash
# Regenerate Taskfile
dr task compose

# Verify new structure
task --list
```

## dr task list

List all available tasks from composed Taskfile.

### Synopsis

```bash
dr task list [flags]
```

### Description

Lists all tasks available in the current template, including tasks from all component Taskfiles.

### Examples

```bash
# List all tasks
dr task list

# Show with full task tree
task --list-all
```

## dr task run

Execute template tasks. This is an alias for `dr run`.

### Synopsis

```bash
dr task run [TASK_NAME...] [flags]
```

### Description

Execute one or more tasks defined in component Taskfiles. See [dr run](run.md) for full documentation.

### Examples

```bash
# Run single task
dr task run dev

# Run multiple tasks
dr task run lint test

# Run in parallel
dr task run lint test --parallel
```

## See also

- [dr run](run.md)&mdash;task execution documentation.
- [Template system](../template-system/README.md)&mdash;template structure overview.
- [Environment variables](../template-system/environment-variables.md)&mdash;configuration management.
- [Task documentation](https://taskfile.dev/)&mdash;official Task runner documentation.
