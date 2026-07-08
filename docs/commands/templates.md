# `dr templates` - Template management

The `dr templates` command (alias: `template`) manages DataRobot AI application templates. Templates are pre-built projects that you can clone and customize to create your own AI applications.

## Quick start

```bash
# Interactive setup wizard (recommended for first-time users)
dr templates setup

# Browse available templates
dr templates list
```

> [!NOTE]
> **First time?** If you're new to the CLI, start with the [Quick start](../../README.md#quick-start) for step-by-step setup instructions.

## Synopsis

```bash
dr templates <command> [flags]
dr template <command> [flags]
```

## Subcommands

### `list`

List all available AI application templates from DataRobot.

```bash
dr templates list [--output-format text|json]
```

**Flags:**

- `--output-format <text|json>`&mdash;output format. Defaults to `text`.

**What it shows:**

Each template entry includes:

- Template ID
- Template name

**Example:**

```bash
dr templates list
```

**Example output (text):**

```text
  ID           NAME
  abc123       LLM Chatbot
  def456       Document Q&A
  ghi789       Code Assistant
```

**Example output (json):**

```bash
dr templates list --output-format json
```

```json
{
  "templates": [
    {"id": "abc123", "name": "LLM Chatbot"},
    {"id": "def456", "name": "Document Q&A"}
  ]
}
```

> [!NOTE]
> Requires authentication. Run `dr auth login` first.

### `setup`

Launch the interactive template setup wizard.

```bash
dr templates setup
```

The wizard guides you through:

1. Selecting an AI application template
2. Cloning it to your local machine
3. Configuring your environment variables

**Example:**

```bash
dr templates setup
```

> [!TIP]
> This is the recommended starting point for new projects. It handles everything interactively — no flags required.

> [!NOTE]
> Requires authentication. Run `dr auth login` first.

## Global options

All `dr` global options are available:

- `-v, --verbose`&mdash;enable verbose output
- `--debug`&mdash;enable debug output
- `--skip-auth`&mdash;skip authentication checks (advanced users)
- `-h, --help`&mdash;show help information

## Examples

### Interactive setup (recommended)

```bash
# Log in first
dr auth login

# Run the interactive wizard
dr templates setup
```

### Browse available templates

```bash
dr templates list
```

### Machine-readable output

```bash
dr templates list --output-format json | jq '.templates[].name'
```

## See also

- [Quick start](../../README.md#quick-start)&mdash;initial setup guide.
- [Authentication](auth.md)&mdash;how to log in.
- [Start command](start.md)&mdash;the quickstart flow that incorporates template setup.
- [Template system](../template-system/)&mdash;template configuration reference.
