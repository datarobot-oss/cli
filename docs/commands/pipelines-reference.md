# `dr pipeline` command reference

Complete cross-reference of every `dr pipeline …` subcommand, the
`pipelines-api` endpoint each one calls, sample invocations, and the
inputs (positional args, flags, request body fields) each command
accepts.

> All commands below assume the `pipeline` feature is enabled
> (`DATAROBOT_CLI_FEATURE_PIPELINE=true`).

## How to read this document

- **Method + path** is relative to `/api/v2`. The CLI prefixes the host
  from `DATAROBOT_CLI_ENDPOINT` (or `DATAROBOT_ENDPOINT`).
- **Usage** lists the canonical invocation plus common variants.
- **Inputs** names every positional argument and flag the command
  accepts. Flags shared by many commands (`--output`, `--scope`,
  `--version`, `--from-file`) are described once at the bottom under
  "Shared flag semantics".

---

## Pipeline lifecycle

| Command | API endpoint | Usage | Inputs |
|---|---|---|---|
| `dr pipeline create` | `POST /pipelines` | `dr pipeline create ./my_pipeline.py` <br> `dr pipeline create --from-file=./my_pipeline.py` <br> `dr pipeline create ./my_pipeline.py --description "First draft" --mode draft` <br> `dr pipeline create --from-file=./my_pipeline.py --output json` | **Positional:** `<file>` (Python file; mutually exclusive with `--from-file`). <br> **Flags:** `--from-file=<path>`, `--description <text>`, `--mode draft\|locked`, `--output json`. |
| `dr pipeline list` | `GET /pipelines` | `dr pipeline list` <br> `dr pipeline list --mode draft` <br> `dr pipeline list --offset 50 --limit 10 --output json` | **Flags:** `--mode draft\|locked`, `--offset <n>`, `--limit <n>`, `--output json`. |
| `dr pipeline get` | `GET /pipelines/{pipeline_id}` | `dr pipeline get <pipeline-id>` <br> `dr pipeline get <pipeline-id> --output json` | **Positional:** `<pipeline-id>` (required). <br> **Flags:** `--output json`. |
| `dr pipeline update` | `PATCH /pipelines/{pipeline_id}` | `dr pipeline update <pipeline-id> ./my_pipeline.py` <br> `dr pipeline update <pipeline-id> --from-file=./my_pipeline.py` | **Positional:** `<pipeline-id>` (required), `<file>` (mutually exclusive with `--from-file`). <br> **Flags:** `--from-file=<path>`, `--output json`. |
| `dr pipeline delete` | `DELETE /pipelines/{pipeline_id}` | `dr pipeline delete <pipeline-id>` | **Positional:** `<pipeline-id>` (required). |
| `dr pipeline lock` | `PATCH /pipelines/{pipeline_id}/mode` | `dr pipeline lock <pipeline-id>` <br> `dr pipeline lock <pipeline-id> --output json` | **Positional:** `<pipeline-id>` (required). <br> **Flags:** `--output json`. |

---

## Versions

| Command | API endpoint | Usage | Inputs |
|---|---|---|---|
| `dr pipeline version list` | `GET /pipelines/{pipeline_id}/versions` | `dr pipeline version list --pipeline <id>` <br> `dr pipeline version list --pipeline <id> --offset 10 --limit 5 --output json` | **Flags:** `--pipeline <id>` (required), `--offset <n>`, `--limit <n>`, `--output json`. |
| `dr pipeline version get` | `GET /pipelines/{pipeline_id}/versions/{version_id}` | `dr pipeline version get --pipeline <id> 2` <br> `dr pipeline version get --pipeline <id> 2 --output json` | **Positional:** `<version-id>` (positive integer, required). <br> **Flags:** `--pipeline <id>` (required), `--output json`. |
| `dr pipeline graph` | `GET /pipelines/{pipeline_id}/graph` (draft) <br> `GET /pipelines/{pipeline_id}/versions/{version_id}/graph` (locked) | `dr pipeline graph --pipeline <id>` (draft) <br> `dr pipeline graph --pipeline <id> --version=2` (locked) <br> `dr pipeline graph --pipeline <id> --version=2 --output json` | **Flags:** `--pipeline <id>` (required), `--scope draft\|locked`, `--version <n>`, `--output json`. |

---

## Shared flag semantics

### `--scope` / `--version` (graph)

The CLI mirrors the API's two URL shapes — `/pipelines/{id}/…` for the
mutable draft and `/pipelines/{id}/versions/{ver}/…` for a locked
version — through a pair of optional flags:

| Flags supplied | Resolved scope | URL used |
|---|---|---|
| _(none)_ | `draft` | `/pipelines/{id}/…` |
| `--version=N` | `locked` (auto) | `/pipelines/{id}/versions/N/…` |
| `--scope=draft` | `draft` | `/pipelines/{id}/…` |
| `--scope=locked --version=N` | `locked` | `/pipelines/{id}/versions/N/…` |
| `--scope=draft --version=N` | **error** | `--scope=draft cannot be combined with --version` |
| `--scope=locked` (no `--version`) | **error** | `--scope=locked requires --version=<n>` |

### `--from-file` / positional file (create + update verbs)

`pipeline create` and `pipeline update` accept the input file in two
equivalent ways:

```bash
dr pipeline create ./my_pipeline.py
dr pipeline create --from-file=./my_pipeline.py
```

Exactly one of the two must be supplied.

### `--output`

Every read/write verb that produces a payload accepts `--output json` to
emit the underlying response struct as indented JSON. Any other value is
rejected with `invalid output format: <value> (supported: json)`.

### `auth` / `--skip-auth`

All verbs run `auth.EnsureAuthenticatedE` as their `PreRunE`. Pass the
global `--skip-auth` flag (or set `DATAROBOT_CLI_SKIP_AUTH=true`) when
exercising a local API stub that doesn't implement `/version/`.

---

## Quick endpoint lookup

| API endpoint | CLI command |
|---|---|
| `POST /pipelines` | `dr pipeline create` |
| `GET /pipelines` | `dr pipeline list` |
| `GET /pipelines/{id}` | `dr pipeline get` |
| `PATCH /pipelines/{id}` | `dr pipeline update` |
| `DELETE /pipelines/{id}` | `dr pipeline delete` |
| `PATCH /pipelines/{id}/mode` | `dr pipeline lock` |
| `GET /pipelines/{id}/versions` | `dr pipeline version list` |
| `GET /pipelines/{id}/versions/{ver}` | `dr pipeline version get` |
| `GET /pipelines/{id}/graph` | `dr pipeline graph` (draft) |
| `GET /pipelines/{id}/versions/{ver}/graph` | `dr pipeline graph` (locked) |
