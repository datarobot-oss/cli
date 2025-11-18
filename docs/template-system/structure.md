# Template system structure

This page provides an understanding of how DataRobot organizes and configures application templates.

## Overview

DataRobot templates are Git repositories that contain application code, configuration, and metadata to deploy custom applications to DataRobot. The CLI provides tools to clone, configure, and manage these templates.

## Template repository structure

A typical template repository:

```
my-datarobot-template/
├── .datarobot/              # Template metadata
│   ├── prompts.yaml         # Configuration prompts
│   ├── config.yaml          # Template settings
│   └── cli/                 # CLI-specific files
│       └── bin/             # Quickstart scripts
│           └── quickstart.sh
├── .env.template            # Environment variable template
├── .taskfile-data.yaml      # Taskfile configuration (optional)
├── .gitignore
├── README.md
├── Taskfile.gen.yaml        # Generated task definitions
├── src/                     # Application source code
│   ├── app/
│   │   └── main.py
│   └── tests/
├── requirements.txt         # Python dependencies
└── package.json             # Node dependencies (if applicable)
```

## Template metadata

### .datarobot directory

The `.datarobot` directory contains template-specific configuration:

```
.datarobot/
├── prompts.yaml        # User prompts for setup wizard
├── config.yaml         # Template metadata
└── README.md          # Template-specific docs
```

### prompts.yaml

Defines interactive configuration prompts. See [Interactive configuration](interactive-config.md) for more details.

Review the example prompt yaml configuration below.

```yaml
prompts:
  - key: "app_name"
    env: "APP_NAME"
    help: "Enter your application name"
    default: "my-app"
    optional: false

  - key: "deployment_target"
    env: "DEPLOYMENT_TARGET"
    help: "Select deployment target"
    options:
      - name: "Development"
        value: "dev"
      - name: "Production"
        value: "prod"
```

### config.yaml

Template metadata and settings:

```yaml
name: "My DataRobot Template"
version: "1.0.0"
description: "A sample DataRobot application template"
author: "DataRobot"
repository: "https://github.com/datarobot/template-example"

# Minimum CLI version required
min_cli_version: "0.1.0"

# Tags for discovery
tags:
  - python
  - streamlit
  - machine-learning

# Required DataRobot features
requirements:
  features:
    - custom_applications
  permissions:
    - CREATE_CUSTOM_APPLICATION
```

## Environment configuration

### .env.template

Review a template for environment variables. Note that the commented lines are optional.

```bash
# Required configuration
APP_NAME=
DATAROBOT_ENDPOINT=

# Optional configuration (commented out by default)
# DEBUG=false
# LOG_LEVEL=info

# Database configuration (conditional)
# DATABASE_URL=postgresql://localhost:5432/mydb
# DATABASE_POOL_SIZE=10

# Authentication
# AUTH_ENABLED=false
# AUTH_PROVIDER=oauth2
```

### .env (Generated)

Created by the CLI during setup, the `.env` file contains actual values. Note that `.env` should be in `.gitignore` and never committed.

```bash
# Required configuration
APP_NAME=my-awesome-app
DATAROBOT_ENDPOINT=https://app.datarobot.com

# Optional configuration
DEBUG=true
LOG_LEVEL=debug

# Database configuration
DATABASE_URL=postgresql://localhost:5432/mydb
DATABASE_POOL_SIZE=5
```

## Quickstart scripts

Templates can optionally provide quickstart scripts to automate application initialization. These scripts are executed by the `dr start` command.

Quickstart scripts must be placed in `.datarobot/cli/bin/`.

### Naming conventions

Scripts must start with `quickstart` (case-sensitive):

- ✅ `quickstart`
- ✅ `quickstart.sh`
- ✅ `quickstart.py`
- ✅ `quickstart-dev`
- ❌ `Quickstart.sh` (wrong casing)
- ❌ `start.sh` (wrong name)

If there are multiple scripts matching the pattern, the first one found in lexicographical order will be executed.

### Platform requirements

Review the requirements for different platforms below.

#### Unix/Linux/macOS

- Must have executable permissions (`chmod +x`)
- Can be any executable file (shell script, Python script, compiled binary, etc.)

#### Windows

- Must have an executable extension: `.exe`, `.bat`, `.cmd`, or `.ps1`

### When to use quickstart scripts

Quickstart scripts are useful for:

- Multi-step initialization: When your application requires several setup steps
- Dependency management: Install packages or tools before starting
- Environment validation: Check prerequisites before launch
- Custom workflows: Template-specific initialization logic

### Fallback behavior

If `dr start` does not find a quickstart, it automatically launches the interactive `dr templates setup` wizard instead to ensure that you can always get started even without a custom script.

## Task definitions

### Taskfile.gen.yaml

The CLI automatically generates `Taskfile.gen.yaml` to aggregate component tasks. This file includes a `dotenv` directive to load environment variables from `.env`.

**Important:** Component taskfiles cannot have their own `dotenv` directives. The CLI detects conflicts and prevents generation if a component taskfile already has a `dotenv` declaration.

The generated structure is shown below.

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

### Component taskfiles

Component directories define their own tasks:

Review the structure of `backend/Taskfile.yaml` below.

```yaml
version: '3'

# Note: No dotenv directive are allowed here

tasks:
  dev:
    desc: Start development server
    cmds:
      - python -m uvicorn src.app.main:app --reload

  test:
    desc: Run tests
    cmds:
      - pytest src/tests/

  build:
    desc: Build application
    cmds:
      - docker build -t {{.APP_NAME}} .
```

### Running tasks

The `dr run` command requires a `.env` file to be present:

```bash
# List all available tasks
dr run --list

# Run a specific task
dr run dev

# Run multiple tasks
dr run lint test

# Run tasks in parallel
dr run lint test --parallel
```

If you're not in a DataRobot template directory (no `.env` file), you'll see the following message:

```
You don't seem to be in a DataRobot Template directory.
This command requires a .env file to be present.
```

### Taskfile configuration data

Template authors can optionally provide a `.taskfile-data.yaml` file to configure the generated Taskfile. This file allows specifying port numbers for development servers and other configuration data.

See [dr task compose documentation](../commands/task.md#taskfile-data-configuration) for complete details on the file format and usage.

## Multi-level configuration

Templates can have nested `.datarobot` directories for component-specific configuration:

```
my-template/
├── .datarobot/
│   └── prompts.yaml          # Root level prompts
├── backend/
│   ├── .datarobot/
│   │   └── prompts.yaml      # Backend prompts
│   └── src/
├── frontend/
│   ├── .datarobot/
│   │   └── prompts.yaml      # Frontend prompts
│   └── src/
└── .env.template
```

### Discovery order

The CLI discovers prompts in this order:

1. Root `.datarobot/prompts.yaml`
2. Subdirectory prompts (depth-first search, up to depth 2)
3. Merged and deduplicated

### Example: backend prompts

`backend/.datarobot/prompts.yaml`:

```yaml
prompts:
  - key: "api_port"
    env: "API_PORT"
    help: "Backend API port"
    default: "8000"
    section: "backend"

  - key: "database_url"
    env: "DATABASE_URL"
    help: "Database connection string"
    section: "backend"
```

### Example: frontend prompts

`frontend/.datarobot/prompts.yaml`:

```yaml
prompts:
  - key: "ui_port"
    env: "UI_PORT"
    help: "Frontend UI port"
    default: "3000"
    section: "frontend"

  - key: "api_endpoint"
    env: "API_ENDPOINT"
    help: "Backend API endpoint"
    default: "http://localhost:8000"
    section: "frontend"
```

## Template lifecycle

### 1. Discovery

Templates are discovered from DataRobot:

```bash
# List available templates
dr templates list
```

Output:
```
Available templates:
* python-streamlit     - Streamlit application template
* react-frontend       - React frontend template
* fastapi-backend      - FastAPI backend template
```

### 2. Cloning

Clone a template to your local machine:

```bash
# Clone a specific template
dr templates clone python-streamlit

# Clone to a custom directory
dr templates clone python-streamlit my-app
```

This:
- Clones the Git repository
- Sets up directory structure
- Initializes configuration files

### 3. Configuration

Configure the template interactively:

```bash
# Full setup wizard
dr templates setup

# Or configure existing template
cd my-template
dr dotenv setup
```

### 4. Development

Work on your application:

```bash
# Run development server (requires .env file)
dr run dev

# Run tests
dr run test

# Build for deployment
dr run build
```

**Note:** All `dr run` commands require a `.env` file in the current directory. If you see an error about not being in a template directory, run `dr dotenv setup` to create your `.env` file.

### 5. Deployment

Deploy to DataRobot:

```bash
dr run deploy
```

## Template types

### Python templates

```
python-template/
├── .datarobot/
├── requirements.txt
├── setup.py
├── src/
│   └── app/
│       └── main.py
├── tests/
└── .env.template
```

#### Key features
- Python dependencies in `requirements.txt`
- Source code in `src/`
- Tests in `tests/`

### Node.js templates

```
node-template/
├── .datarobot/
├── package.json
├── src/
│   └── index.js
├── tests/
└── .env.template
```

#### Key features

- Node dependencies in `package.json`
- Source code in `src/`
- npm scripts integration

### Multi-language templates

```
full-stack-template/
├── .datarobot/
├── backend/
│   ├── .datarobot/
│   ├── requirements.txt
│   └── src/
├── frontend/
│   ├── .datarobot/
│   ├── package.json
│   └── src/
├── docker-compose.yml
└── .env.template
```

#### Key features

- Separate backend and frontend
- Component-specific configuration
- Docker composition

## Best practices

### Version control

**Note:** Always exclude `.env` and `Taskfile.gen.yaml` from version control. The CLI generates `Taskfile.gen.yaml` automatically.

```bash
# .gitignore should include:
.env
Taskfile.gen.yaml
*.log
__pycache__/
node_modules/
dist/
```

### Documentation

Include a clear README.

```markdown
# My template

## Quick start

1. Clone: `dr templates clone my-template`
2. Configure: `dr templates setup`
3. Run: `dr run dev`

## Available tasks

- `dr run dev`: development server.
- `dr run test`: run tests.
- `dr run build`: build for production.
```

### Sensible defaults

Provide defaults in `.env.template`.

```bash
# Good defaults for local development
API_PORT=8000
DEBUG=true
LOG_LEVEL=info
```

### Clear prompts

Use descriptive help text.

```yaml
prompts:
  - key: "database_url"
    help: "PostgreSQL connection string (format: postgresql://user:pass@host:5432/dbname)"
```

### 5. Organized structure

Keep related files together.

```
src/
├── api/          # API endpoints
├── models/       # Data models
├── services/     # Business logic
└── utils/        # Utilities
```

## Template updates

### Checking for updates

```bash
# Check current template status
dr templates status

# Shows:
# - Current version
# - Latest available version
# - Modified files
# - Available updates
```

### Updating templates

```bash
# Update to latest version
git pull origin main

# Re-run configuration if needed
dr dotenv setup
```

## Creating your own template

### 1. Start with base structure

```bash
mkdir my-new-template
cd my-new-template
git init
```

### 2. Add template files

Create the necessary files:

```bash
# Configuration
mkdir .datarobot
touch .datarobot/prompts.yaml
touch .env.template

# Application structure
mkdir -p src/app
touch src/app/main.py

# Tasks
touch Taskfile.gen.yaml
```

### 3. Define prompts

`.datarobot/prompts.yaml`:

```yaml
prompts:
  - key: "app_name"
    env: "APP_NAME"
    help: "Enter your application name"
    optional: false
```

### 4. Create an environment template

`.env.template`:

```bash
APP_NAME=
DATAROBOT_ENDPOINT=
```

### 5. Define tasks

Create component Taskfiles (e.g., `backend/Taskfile.yaml`):

```yaml
version: '3'

tasks:
  dev:
    desc: Start development server
    cmds:
      - echo "Starting {{.APP_NAME}}"
```

### 6. Configure Taskfile data

Optional. Create `.taskfile-data.yaml` to provide additional configuration for the generated root taskfile:

```yaml
# .taskfile-data.yaml
# Optional configuration for dr task compose

# Ports to display when running dev task
ports:
  - name: Backend
    port: 8080
  - name: Frontend
    port: 5173
```

This allows developers using your template to see which ports services run on when they execute `task dev`.

### 7. Test the template

```bash
# Test the setup locally
dr templates setup

# Verify configuration
dr run --list
```

### 8. Publish the template

```bash
# Push to GitHub
git add .
git commit -m "Initial template"
git push origin main

# Register with DataRobot (contact your admin)
```

## See also

- [Interactive configuration](interactive-config.md): Configuration wizard details.
- [Environment variables](environment-variables.md): Manage .env files.
- [dr run](../commands/run.md): Task execution.
- [dr task compose](../commands/task.md): Taskfile composition and configuration.
- [Command reference: templates](../commands/templates.md): Template commands.
