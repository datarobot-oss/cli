# Template System Structure

Understanding how DataRobot application templates are organized and configured.

## Overview

DataRobot templates are Git repositories that contain application code, configuration, and metadata for deploying custom applications to DataRobot. The CLI provides tools to clone, configure, and manage these templates.

## Template repository structure

A typical template repository:

```
my-datarobot-template/
├── .datarobot/               # Template metadata
│   ├── prompts.yaml         # Configuration prompts
│   ├── config.yaml          # Template settings
│   └── cli/                 # CLI-specific files
│       └── bin/             # Quickstart scripts
│           └── quickstart.sh
├── .env.template            # Environment variable template
├── .gitignore
├── README.md
├── Taskfile.gen.yaml        # Generated task definitions
├── src/                     # Application source code
│   ├── app/
│   │   └── main.py
│   └── tests/
├── requirements.txt         # Python dependencies
└── package.json            # Node dependencies (if applicable)
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

Defines interactive configuration prompts. See [Interactive Configuration](interactive-config.md) for detailed documentation.

Example:

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

The template for environment variables. Commented lines are optional:

```bash
# Required Configuration
APP_NAME=
DATAROBOT_ENDPOINT=

# Optional Configuration (commented out by default)
# DEBUG=false
# LOG_LEVEL=info

# Database Configuration (conditional)
# DATABASE_URL=postgresql://localhost:5432/mydb
# DATABASE_POOL_SIZE=10

# Authentication
# AUTH_ENABLED=false
# AUTH_PROVIDER=oauth2
```

### .env (Generated)

Created by the CLI during setup, contains actual values:

```bash
# Required Configuration
APP_NAME=my-awesome-app
DATAROBOT_ENDPOINT=https://app.datarobot.com

# Optional Configuration
DEBUG=true
LOG_LEVEL=debug

# Database Configuration
DATABASE_URL=postgresql://localhost:5432/mydb
DATABASE_POOL_SIZE=5
```

**Note:** `.env` should be in `.gitignore` and never committed.

## Quickstart scripts

Templates can optionally provide quickstart scripts to automate application initialization. These scripts are executed by the `dr start` command.

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

### Platform requirements

**Unix/Linux/macOS:**

- Must have executable permissions (`chmod +x`)
- Can be any executable file (shell script, Python script, compiled binary, etc.)

**Windows:**

- Must have executable extension: `.exe`, `.bat`, `.cmd`, or `.ps1`

### Example quickstart script

**`.datarobot/cli/bin/quickstart.sh`:**

```bash
#!/bin/bash
set -e

echo "🚀 Starting DataRobot application..."

# Verify .env exists
if [ ! -f .env ]; then
    echo "❌ .env file not found. Please run 'dr templates setup' first."
    exit 1
fi

# Load environment variables
source .env

# Install dependencies
echo "📦 Installing dependencies..."
pip install -r requirements.txt

# Start the application
echo "✨ Launching $APP_NAME..."
dr run dev
```

Make it executable:

```bash
chmod +x .datarobot/cli/bin/quickstart.sh
```

### When to use quickstart scripts

Quickstart scripts are useful for:

- **Multi-step initialization**&mdash;when your application requires several setup steps.
- **Dependency management**&mdash;installing packages or tools before starting.
- **Environment validation**&mdash;checking prerequisites before launch.
- **Custom workflows**&mdash;template-specific initialization logic.

### Fallback behavior

If no quickstart script is found, `dr start` automatically launches the interactive `dr templates setup` wizard instead, ensuring users can always get started even without a custom script.

## Task definitions

### Taskfile.gen.yaml

The CLI automatically generates `Taskfile.gen.yaml` to aggregate component tasks. This file includes a `dotenv` directive to load environment variables from `.env`.

**Generated structure:**

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

**Important:** Component Taskfiles cannot have their own `dotenv` directives. The CLI detects conflicts and prevents generation if a component Taskfile already has a `dotenv` declaration.

### Component Taskfiles

Component directories define their own tasks:

**backend/Taskfile.yaml:**

```yaml
version: '3'

# Note: No dotenv directive allowed here

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

If you're not in a DataRobot template directory (no `.env` file), you'll see:

```
You don't seem to be in a DataRobot Template directory.
This command requires a .env file to be present.
```

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
* python-streamlit     - Streamlit application template.
* react-frontend       - React frontend template.
* fastapi-backend      - FastAPI backend template.
```

### 2. Cloning

Clone a template to your local machine:

```bash
# Clone specific template
dr templates clone python-streamlit

# Clone to custom directory
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
dr dotenv --wizard
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

**Note:** All `dr run` commands require a `.env` file in the current directory. If you see an error about not being in a template directory, run `dr dotenv --wizard` to create your `.env` file.

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

**Key features:**
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

**Key features:**
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

**Key features:**
- Separate backend and frontend
- Component-specific configuration
- Docker composition

## Best practices

### 1. Version control

```bash
# .gitignore should include:
.env
Taskfile.gen.yaml
*.log
__pycache__/
node_modules/
dist/
```

**Note:** Always exclude `.env` and `Taskfile.gen.yaml` from version control. The CLI generates `Taskfile.gen.yaml` automatically.

### 2. Documentation

Include clear README:

```markdown
# My template

## Quick start

1. Clone: `dr templates clone my-template`
2. Configure: `dr templates setup`
3. Run: `dr run dev`

## Available tasks

- `dr run dev`&mdash;development server.
- `dr run test`&mdash;run tests.
- `dr run build`&mdash;build for production.
```

### 3. Sensible defaults

Provide defaults in `.env.template`:

```bash
# Good defaults for local development
API_PORT=8000
DEBUG=true
LOG_LEVEL=info
```

### 4. Clear prompts

Use descriptive help text:

```yaml
prompts:
  - key: "database_url"
    help: "PostgreSQL connection string (format: postgresql://user:pass@host:5432/dbname)"
```

### 5. Organized structure

Keep related files together:

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
dr dotenv --wizard
```

## Creating your own template

### 1. Start with base structure

```bash
mkdir my-new-template
cd my-new-template
git init
```

### 2. Add template files

Create necessary files:

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

### 4. Create environment template

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

### 6. Test template

```bash
# Test setup locally
dr templates setup

# Verify configuration
dr run --list
```

### 7. Publish template

```bash
# Push to GitHub
git add .
git commit -m "Initial template"
git push origin main

# Register with DataRobot (contact your admin)
```

## See also

- [Interactive configuration](interactive-config.md)&mdash;configuration wizard details.
- [Environment variables](environment-variables.md)&mdash;managing .env files.
- [dr run](../commands/run.md)&mdash;task execution.
- [Command reference: templates](../commands/templates.md)&mdash;template commands.
