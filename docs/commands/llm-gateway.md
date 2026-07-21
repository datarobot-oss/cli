# `dr llm-gateway` — LLM Gateway model management

List active LLM Gateway models and configure which one the CLI uses by default.

## Synopsis

```bash
dr llm-gateway <command> [flags]
dr llm [flags]           # alias
```

## Description

The `dr llm-gateway` group exposes two subcommands:

- **`list`** — fetch all active LLMs from `/api/v2/genai/llmgw/catalog/` and display them as a table or JSON.
- **`select`** — choose a default LLM, either by ID or through an interactive TUI picker. The selection is persisted to `drconfig.yaml` and read by other CLI commands.

**Aliases:** `llm`, `llm-gateways`

## Subcommands

### `list`

Fetch all active LLMs from the LLM Gateway catalog and display them.

```bash
dr llm-gateway list [flags]
dr llm ls               # shortest alias
```

**Flags:**

- `--output-format <json>` — emit machine-parseable JSON instead of a table.

**Output columns (table):**

| Column     | Description                                      |
|------------|--------------------------------------------------|
| `ID`       | LLM identifier. Prefixed with `*` if selected, `  ` otherwise. |
| `NAME`     | Human-readable model name.                       |
| `PROVIDER` | Provider (e.g. `azure`, `anthropic`, `google`).  |
| `MODEL`    | Underlying model identifier.                     |
| `CONTEXT`  | Context-window size in tokens. `-` when the catalog does not report it. |

The table width is content-driven and capped at the terminal width to prevent overflow. `description` is omitted from the table (it is long enough to wrap unreadably across a full catalog) and appears in JSON output only.

**JSON output** (`--output-format json`) returns an envelope with a `llms` array. Each entry includes:

```json
{
  "id":           "llm-abc123",
  "name":         "GPT-4o",
  "provider":     "azure",
  "model":        "gpt-4o",
  "description":  "OpenAI's flagship multimodal model.",
  "context_size": 128000,
  "selected":     true
}
```

**Examples:**

```bash
# Table view
dr llm-gateway list

# JSON output
dr llm-gateway list --output-format json

# Aliases
dr llm list
dr llm ls
```

---

### `select`

Set the default LLM Gateway model. The chosen ID is written to `drconfig.yaml` under the key `default-llm-id` and is also readable via `DATAROBOT_CLI_DEFAULT_LLM_ID`.

```bash
dr llm-gateway select [llm-id]
dr llm select [llm-id]   # alias
```

**Arguments:**

- `[llm-id]` — optional. When provided, the ID is validated against the active catalog and persisted immediately. When omitted, an interactive TUI picker is launched.

**Interactive picker:**

- Arrow keys to navigate, `/` to filter by name.
- `Enter` to confirm selection.
- `Ctrl-C` or `Esc` to cancel without saving.

**Examples:**

```bash
# Interactive TUI picker
dr llm-gateway select

# Set directly by ID
dr llm-gateway select llm-abc123

# Error — ID not found in active catalog
dr llm-gateway select unknown-id
# Error: LLM "unknown-id" not found in the active catalog
```

---

## Configuration

The selected LLM ID is stored in `drconfig.yaml`:

```yaml
default-llm-id: llm-abc123
```

It can also be set or overridden via the environment variable:

```bash
export DATAROBOT_CLI_DEFAULT_LLM_ID=llm-abc123
```

The `dr llm-gateway list` output uses this value to mark the currently selected model with `*`.

## Authentication

Both subcommands require valid DataRobot credentials. Run `dr auth login` first if you haven't already.

## See also

- [auth](auth.md) — authenticate with DataRobot.
- [Command reference](README.md) — overview of all commands.
