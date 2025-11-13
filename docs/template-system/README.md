# Template System Documentation

This section covers the DataRobot template system in detail.

## What are templates?

DataRobot templates are pre-configured application scaffolds that help you quickly build and deploy custom applications to DataRobot. Each template includes:

- Application source code.
- Configuration prompts.
- Environment setup.
- Task definitions.
- Documentation.

## Documentation

### Core concepts

- **[Template structure](structure.md)**&mdash;how templates are organized.
  - Repository layout.
  - Metadata files.
  - Multi-component templates.
  - Best practices.

- **[Interactive configuration](interactive-config.md)**&mdash;the configuration wizard.
  - Prompt system architecture.
  - Input types (text, selection, multi-select).
  - Conditional prompts.
  - Validation and error handling.

- **[Environment variables](environment-variables.md)**&mdash;managing .env files.
  - .env.template format.
  - Variable types (required, optional, secret).
  - Security best practices.
  - Advanced features.

## Quick start

### Using a template

```bash
# List available templates
dr templates list

# Interactive setup (recommended)
dr templates setup

# Manual setup
dr templates clone my-template
cd my-template
dr dotenv setup
dr run dev
```

### Creating a template

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

Simple applications with one component:

```
my-spa-template/
├── .datarobot/
│   └── prompts.yaml
├── src/
├── .env.template
└── Taskfile.gen.yaml
```

### Full-stack applications

Applications with multiple components:

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

Multiple independent services:

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

  - key: "database_url"
    section: "database_config"
    env: "DATABASE_URL"
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

### 1. Clear documentation

Include README with:
- Quick start guide.
- Available tasks.
- Configuration options.
- Deployment instructions.

### 2. Sensible defaults

Provide defaults in `.env.template`:

```bash
# Good defaults for local development
PORT=8080
DEBUG=true
LOG_LEVEL=info
```

### 3. Helpful prompts

Use descriptive help text:

```yaml
prompts:
  - key: "database_url"
    help: "PostgreSQL connection string (format: postgresql://user:pass@host:5432/dbname)"
```

### 4. Organized structure

Keep related files together:

```
src/
├── api/          # API endpoints
├── models/       # Data models
├── services/     # Business logic
└── utils/        # Utilities
```

### 5. Security first

- Never commit `.env` files.
- Use strong secrets.
- Restrict file permissions.
- Mask sensitive values.

## Examples

Browse the [DataRobot Template Gallery](https://github.com/datarobot/templates) for example templates:

- **python-streamlit**&mdash;Streamlit dashboard.
- **react-frontend**&mdash;React web application.
- **fastapi-backend**&mdash;FastAPI REST API.
- **full-stack-app**&mdash;complete web application.

## See also

- [Getting Started](../user-guide/getting-started.md)
- [Working with Templates](../user-guide/templates.md)
- [Command Reference: templates](../commands/templates.md)
- [Command Reference: dotenv](../commands/dotenv.md)
