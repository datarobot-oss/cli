# Template System Structure

Understanding how DataRobot application templates are organized and configured.

## Overview

DataRobot templates are Git repositories that contain application code, configuration, and metadata for deploying custom applications to DataRobot. The CLI provides tools to clone, configure, and manage these templates.

## Template Repository Structure

A typical template repository:

```
my-datarobot-template/
├── .datarobot/               # Template metadata
│   ├── prompts.yaml         # Configuration prompts
│   └── config.yaml          # Template settings
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

## Template Metadata

### .datarobot Directory

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

## Environment Configuration

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

## Task Definitions

### Taskfile.gen.yaml

Generated task definitions for running common operations:

```yaml
version: '3'

tasks:
  dev:
    desc: Start development server
    cmds:
      - python -m uvicorn src.app.main:app --reload

  build:
    desc: Build application
    cmds:
      - docker build -t {{.APP_NAME}} .

  test:
    desc: Run tests
    cmds:
      - pytest src/tests/

  lint:
    desc: Run linters
    cmds:
      - pylint src/
      - black --check src/

  deploy:
    desc: Deploy to DataRobot
    deps: [build]
    cmds:
      - dr deploy
```

Usage:

```bash
# List tasks
dr run --list

# Run a task
dr run dev

# Run multiple tasks
dr run lint test
```

## Multi-Level Configuration

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

### Discovery Order

The CLI discovers prompts in this order:

1. Root `.datarobot/prompts.yaml`
2. Subdirectory prompts (depth-first search, up to depth 2)
3. Merged and deduplicated

### Example: Backend Prompts

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

### Example: Frontend Prompts

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

## Template Lifecycle

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
* react-frontend      - React frontend template
* fastapi-backend     - FastAPI backend template
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
# Run development server
dr run dev

# Run tests
dr run test

# Build for deployment
dr run build
```

### 5. Deployment

Deploy to DataRobot:

```bash
dr run deploy
```

## Template Types

### Python Templates

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

### Node.js Templates

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

### Multi-Language Templates

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

## Best Practices

### 1. Version Control

```bash
# .gitignore should include:
.env
*.log
__pycache__/
node_modules/
dist/
```

### 2. Documentation

Include clear README:

```markdown
# My Template

## Quick Start

1. Clone: `dr templates clone my-template`
2. Configure: `dr templates setup`
3. Run: `dr run dev`

## Available Tasks

- `dr run dev` - Development server
- `dr run test` - Run tests
- `dr run build` - Build for production
```

### 3. Sensible Defaults

Provide defaults in `.env.template`:

```bash
# Good defaults for local development
API_PORT=8000
DEBUG=true
LOG_LEVEL=info
```

### 4. Clear Prompts

Use descriptive help text:

```yaml
prompts:
  - key: "database_url"
    help: "PostgreSQL connection string (format: postgresql://user:pass@host:5432/dbname)"
```

### 5. Organized Structure

Keep related files together:

```
src/
├── api/          # API endpoints
├── models/       # Data models
├── services/     # Business logic
└── utils/        # Utilities
```

## Template Updates

### Checking for Updates

```bash
# Check current template status
dr templates status

# Shows:
# - Current version
# - Latest available version
# - Modified files
# - Available updates
```

### Updating Templates

```bash
# Update to latest version
git pull origin main

# Re-run configuration if needed
dr dotenv --wizard
```

## Creating Your Own Template

### 1. Start with Base Structure

```bash
mkdir my-new-template
cd my-new-template
git init
```

### 2. Add Template Files

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

### 3. Define Prompts

`.datarobot/prompts.yaml`:

```yaml
prompts:
  - key: "app_name"
    env: "APP_NAME"
    help: "Enter your application name"
    optional: false
```

### 4. Create Environment Template

`.env.template`:

```bash
APP_NAME=
DATAROBOT_ENDPOINT=
```

### 5. Define Tasks

`Taskfile.gen.yaml`:

```yaml
version: '3'

tasks:
  dev:
    desc: Start development server
    cmds:
      - echo "Starting {{.APP_NAME}}"
```

### 6. Test Template

```bash
# Test setup locally
dr templates setup

# Verify configuration
dr run --list
```

### 7. Publish Template

```bash
# Push to GitHub
git add .
git commit -m "Initial template"
git push origin main

# Register with DataRobot (contact your admin)
```

## See Also

- [Interactive Configuration](interactive-config.md) - Configuration wizard details
- [Environment Variables](environment-variables.md) - Managing .env files
- [Command Reference: templates](../commands/templates.md) - Template commands
