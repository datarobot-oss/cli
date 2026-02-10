# Template system documentation

DataRobot templates are pre-configured application scaffolds that help you quickly build and deploy custom applications to DataRobot. Each template includes:

- Application source code
- Configuration prompts
- Environment setup tools
- Task definitions
- Documentation

## Documentation

### Core concepts

- [Template structure](structure.md): How templates are organized.
  - Repository layout
  - Metadata files
  - Multi-component templates
  - Best practices

- [Interactive configuration](interactive-config.md): The configuration wizard.
  - Prompt system architecture
  - Input types (text, selection, multi-select)
  - Conditional prompts
  - Validation and error handling

- [Environment variables](environment-variables.md): Managing .env files.
  - .env.template format
  - Variable types (required, optional, secret)
  - Security best practices
  - Advanced features

## Quickstart

### Using a template

```bash
# List available templates
dr templates list

# Interactive setup (recommended)
dr templates setup

# Manual setup (if you already have a template directory)
cd my-template
dr dotenv setup
dr run dev
```

### Create a template

```bash
# 1. Create structure
mkdir my-template
cd my-template

# 2. Add metadata
mkdir .datarobot
cat > .datarobot/prompts.yaml <<EOF
prompts:
  - key: "app_name"
    env: "APP_NAME"
    help: "Enter your application name"
EOF

# 3. Create environment template
cat > .env.template <<EOF
APP_NAME=
DATAROBOT_ENDPOINT=
EOF

# 4. Add tasks
cat > Taskfile.gen.yaml <<EOF
version: '3'
tasks:
  dev:
    desc: Start development server
    cmds:
      - echo "Starting {{.APP_NAME}}"
EOF

# 5. Test it
dr templates setup
```

## Template types

### Single-page applications

Create simple applications with one component.

```
my-spa-template/
├── .datarobot/
│   └── prompts.yaml
├── src/
├── .env.template
└── Taskfile.gen.yaml
```

### Full-stack applications

Create applications with multiple components.

```
my-fullstack-template/
├── .datarobot/
│   └── prompts.yaml
├── backend/
│   ├── .datarobot/
│   │   └── prompts.yaml
│   └── src/
├── frontend/
│   ├── .datarobot/
│   │   └── prompts.yaml
│   └── src/
└── .env.template
```

### Microservices

Use multiple independent services:

```
my-microservices-template/
├── .datarobot/
├── service-a/
│   ├── .datarobot/
│   └── src/
├── service-b/
│   ├── .datarobot/
│   └── src/
└── docker-compose.yml
```

## Common patterns

### Database configuration

```yaml
prompts:
  - key: "use_database"
    help: "Enable database?"
    options:
      - name: "Yes"
        requires: "database_config"
      - name: "No"

database_config:
  - env: "DATABASE_URL"
    help: "Database connection string"
```

### Feature flags

```yaml
prompts:
  - key: "enabled_features"
    env: "ENABLED_FEATURES"
    help: "Select features to enable"
    multiple: true
    options:
      - name: "Analytics"
        value: "analytics"
      - name: "Monitoring"
        value: "monitoring"
```

### Authentication

```yaml
prompts:
  - key: "auth_provider"
    env: "AUTH_PROVIDER"
    help: "Select authentication provider"
    options:
      - name: "OAuth2"
        value: "oauth2"
        requires: "oauth_config"
      - name: "SAML"
        value: "saml"
        requires: "saml_config"
```

## Best practices

### Clear documentation

Includes a README file with:
- A quickstart guide
- Available tasks
- Configuration options
- Deployment instructions

### Sensible defaults

Provide defaults in `.env.template`:

```bash
# Good defaults for local development
PORT=8080
DEBUG=true
LOG_LEVEL=info
```

### Helpful prompts

Use descriptive help text:

```yaml
prompts:
  - key: "database_url"
    help: "PostgreSQL connection string (format: postgresql://user:pass@host:5432/dbname)"
```

### Organized structure

Keep related files together.

```
src/
├── api/          # API endpoints
├── models/       # Data models
├── services/     # Business logic
└── utils/        # Utilities
```

### Security first

> [!WARNING]
> Follow these security guidelines:
>
> - Never commit `.env` files.
> - Use strong secrets.
> - Restrict file permissions.
> - Mask sensitive values.

## Examples

Browse the [DataRobot template gallery](https://github.com/datarobot/templates) to view example templates:

- **python-streamlit**: Streamlit dashboard
- **react-frontend**: React web application
- **fastapi-backend**: FastAPI REST API
- **full-stack-app**: complete web application

## See also

- [Quick start](../../README.md#quick-start)&mdash;installation and initial setup
- [User guide](../user-guide/README.md)&mdash;complete usage guide
- [Command reference: dotenv](../commands/dotenv.md)&mdash;environment variable management
