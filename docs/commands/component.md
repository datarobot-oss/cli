# `dr component` - Component management

The `dr component` command (alias: `c`) manages reusable application components. Components are modular pieces of functionality — such as a FastAPI backend, a React frontend, or an LLM integration — that can be added to your DataRobot application template.

> [!NOTE]
> These commands must be run from your repository's root directory (where `.datarobot/` is located).

## Quick start

```bash
# List components currently installed in your project
dr component list

# Add a new component from a repository URL
dr component add COMPONENT_URL

# Update an existing component
dr component update
```

## Synopsis

```bash
dr component <command> [flags]
dr c <command> [flags]
```

## Subcommands

### `add`

Add a component to your application template. You can specify a component URL directly as an argument, or run the command interactively.

```bash
dr component add [COMPONENT_URL] [flags]
```

**Arguments:**

- `COMPONENT_URL` (optional)&mdash;URL of the component repository to add. If omitted, the command enters interactive mode.

**Flags:**

| Flag | Short | Description |
|------|-------|-------------|
| `--data key=value` | `-d` | Provide answer data in `key=value` format. Can be specified multiple times. |
| `--data-file FILE` | | Path to a YAML file with default answers (follows copier `data_file` semantics). |
| `--trust` | | Trust the template repository (required for migrations). Default: `true`. |

**Example:**

```bash
# Add a component from a GitHub repository
dr component add https://github.com/datarobot-oss/af-component-fastapi-backend

# Add with pre-supplied answers
dr component add https://github.com/datarobot-oss/af-component-react -d app_name=MyApp
```

After adding, the component files are copied into your project and the Taskfile is recomposed automatically.

### `list`

List all components installed in the current project.

```bash
dr component list [--output-format text|json]
```

**Flags:**

- `--output-format <text|json>`&mdash;output format. Defaults to `text`.

**Example output (text):**

```text
  NAME             FILE                                      REPO
  fastapi-backend  .datarobot/answers/fastapi-backend.yaml   https://github.com/datarobot-oss/af-component-fastapi-backend
  react            .datarobot/answers/react.yaml             https://github.com/datarobot-oss/af-component-react
```

**Example output (json):**

```bash
dr component list --output-format json
```

```json
{
  "components": [
    {"name": "fastapi-backend", "file": ".datarobot/answers/fastapi-backend.yaml", "repo": "https://github.com/datarobot-oss/af-component-fastapi-backend"},
    {"name": "react", "file": ".datarobot/answers/react.yaml", "repo": "https://github.com/datarobot-oss/af-component-react"}
  ]
}
```

### `update`

Update an installed component to pick up the latest changes from its source repository.

```bash
dr component update [answers_file] [flags]
```

**Arguments:**

- `answers_file` (optional)&mdash;path to the component answers YAML file to update. If omitted, the command lists installed components and prompts you to select one.

**Flags:**

| Flag | Short | Description |
|------|-------|-------------|
| `--data key=value` | `-d` | Provide answer data in `key=value` format. Can be specified multiple times. |
| `--data-file FILE` | | Path to a YAML file with default answers. |
| `--recopy` | `-r` | Regenerate an existing component with different answers. |
| `--vcs-ref REF` | | Git reference to check out in the component source. |
| `--quiet` | `-q` | Suppress status output. |
| `--overwrite` | `-w` | Overwrite files even if they already exist. |
| `--trust` | | Trust the template repository (required for migrations). Default: `true`. |

**Example:**

```bash
# Update a component interactively
dr component update

# Update a specific component answers file
dr component update .datarobot/answers/fastapi-backend.yaml

# Update and check out a specific git ref
dr component update --vcs-ref v1.2.0
```

## Examples

### Add a component

```bash
# Add a FastAPI backend component
dr component add https://github.com/datarobot-oss/af-component-fastapi-backend

# Add a React frontend component
dr component add https://github.com/datarobot-oss/af-component-react
```

### Inspect installed components

```bash
dr component list
```

### Update a component

```bash
# Interactive selection
dr component update

# Target specific answers file
dr component update .datarobot/answers/react.yaml
```

## See also

- [Quick start](../../README.md#quick-start)&mdash;initial setup guide.
- [Component managed updates](component-managed-updates.md)&mdash;automated component update mechanism.
- [Template system](../template-system/)&mdash;template configuration reference.
- [Templates command](templates.md)&mdash;browse and set up application templates.
