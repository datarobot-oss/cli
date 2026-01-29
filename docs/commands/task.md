# `dr task`

Manage Taskfile composition and task execution for DataRobot templates.

## Quick start

For most users, working with Taskfiles is straightforward:

```bash
# Compose a unified Taskfile from components
dr task compose

# List available tasks
dr task list

# Execute tasks
dr task run dev
```

The command automatically discovers Taskfiles in your template components and aggregates them into a unified configuration.

> [!NOTE]
> **First time?** If you're new to the CLI, start with the [Quick start](../../README.md#quick-start) for step-by-step setup instructions.

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
├── .taskfile-data.yaml          # Optional: template configuration
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

#### Configure development ports

Optionally create a `.taskfile-data.yaml` file to display service URLs in the dev task. See [Taskfile data configuration](#taskfile-data-configuration) for complete documentation.

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

### Taskfile data configuration

Template authors can provide additional configuration for Taskfile generation by creating a `.taskfile-data.yaml` file in the template root directory.

#### File location

```
my-template/
├── .env
├── .taskfile-data.yaml          # Configuration file
├── Taskfile.yaml                # Generated
└── components/
```

#### Configuration format

```yaml
# .taskfile-data.yaml
# Optional configuration for dr task compose

# Development server ports
# Displayed when running the dev task
ports:
  - name: Backend API
    port: 8080
  - name: Frontend
    port: 5173
  - name: Worker Service
    port: 8842
  - name: MCP Server
    port: 9000
```

#### Port configuration

**Purpose:**

The `ports` array allows template authors to specify which ports their services use. When developers run `task dev`, they see URLs for each service.

**Example output:**

When developers run `task dev` with port configuration:

```
task mcp_server:dev &
sleep 3
task web:dev &
sleep 3
task agent:dev &
sleep 3
task frontend_web:dev &
sleep 8
✅ All servers started!
🔗 Backend API: http://localhost:8080
🔗 Frontend: http://localhost:5173
🔗 Agent Service: http://localhost:8842
🔗 MCP Server: http://localhost:9000
```

**DataRobot Notebook integration:**

The generated dev task automatically detects DataRobot Notebook environments and adjusts URLs:

```
🔗 Backend API: https://app.datarobot.com/notebook-sessions/abc123/ports/8080
🔗 Frontend: https://app.datarobot.com/notebook-sessions/abc123/ports/5173
```

This happens automatically when the `NOTEBOOK_ID` environment variable is present.

**Benefits:**

- **Improved onboarding**&mdash;new developers immediately know where services are running.
- **Self-documenting**&mdash;ports are visible in generated Taskfile and command output.
- **Notebook support**&mdash;URLs work correctly in DataRobot Notebooks.
- **Reduced confusion**&mdash;no need to check logs or documentation for port numbers.

**Best practices:**

1. **List all services**&mdash;include every service that starts in dev mode.
2. **Use descriptive names**&mdash;"Backend API" is clearer than "Backend".
3. **Match actual ports**&mdash;ensure ports match what's in component Taskfiles.
4. **Update when changing**&mdash;keep configuration in sync with service changes.

#### When to use this file

**Use `.taskfile-data.yaml` when:**

- Your template has multiple services with different ports.
- Services use non-standard ports that aren't obvious.
- You want to improve developer experience.
- Your template targets DataRobot Notebooks.

**You can skip it when:**

- Your template has a single service.
- Ports are obvious or standard (e.g., 3000 for Node.js).
- You use custom Taskfile templates with hardcoded values.
- Port information is already well-documented elsewhere.

#### File is optional

The `.taskfile-data.yaml` file is completely optional. If not present:

- The dev task still works correctly.
- Services start normally.
- Port URLs simply aren't displayed.

This allows template authors to add port configuration incrementally without breaking existing templates.

#### Future extensibility

The `.taskfile-data.yaml` file uses an extensible format. Future CLI versions may support additional configuration options such as:

- Custom environment variables for templates.
- Service metadata (descriptions, dependencies).
- Deployment configuration.
- Build optimization hints.

Template authors can future-proof their templates by using this configuration file even if only specifying ports initially.

#### Example templates

**Minimal example:**

```yaml
# .taskfile-data.yaml
ports:
  - name: App
    port: 8000
```

**Full-stack application:**

```yaml
# .taskfile-data.yaml
ports:
  - name: Backend API
    port: 8080
  - name: Frontend
    port: 5173
  - name: Database Admin
    port: 8081
  - name: Redis Commander
    port: 8082
```

**Microservices architecture:**

```yaml
# .taskfile-data.yaml
ports:
  - name: API Gateway
    port: 8080
  - name: Auth Service
    port: 8081
  - name: User Service
    port: 8082
  - name: Order Service
    port: 8083
  - name: Frontend
    port: 3000
  - name: Admin Dashboard
    port: 3001
```

### Workflow integration

#### Initial setup

```bash
# Set up template (clones and configures)
dr templates setup
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
