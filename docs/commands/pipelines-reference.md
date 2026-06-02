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
  accepts. Flags shared by many commands (`--output-format`, `--scope`,
  `--version`, `--from-file`) are described once at the bottom under
  "Shared flag semantics".

---

## Pipeline lifecycle

| Command | API endpoint | Usage | Inputs |
|---|---|---|---|
| `dr pipeline create` | `POST /pipelines` | `dr pipeline create ./my_pipeline.py` <br> `dr pipeline create --from-file=./my_pipeline.py` <br> `dr pipeline create ./my_pipeline.py --description "First draft" --mode draft` <br> `dr pipeline create --from-file=./my_pipeline.py --output-format json` | **Positional:** `<file>` (Python file; mutually exclusive with `--from-file`). <br> **Flags:** `--from-file=<path>`, `--description <text>`, `--mode draft\|locked`, `--output-format json`. |
| `dr pipeline list` | `GET /pipelines` | `dr pipeline list` <br> `dr pipeline list --mode draft` <br> `dr pipeline list --offset 50 --limit 10 --output-format json` | **Flags:** `--mode draft\|locked`, `--offset <n>`, `--limit <n>`, `--output-format json`. |
| `dr pipeline get` | `GET /pipelines/{pipeline_id}` | `dr pipeline get <pipeline-id>` <br> `dr pipeline get <pipeline-id> --output-format json` | **Positional:** `<pipeline-id>` (required). <br> **Flags:** `--output-format json`. |
| `dr pipeline update` | `PATCH /pipelines/{pipeline_id}` | `dr pipeline update <pipeline-id> ./my_pipeline.py` <br> `dr pipeline update <pipeline-id> --from-file=./my_pipeline.py` | **Positional:** `<pipeline-id>` (required), `<file>` (mutually exclusive with `--from-file`). <br> **Flags:** `--from-file=<path>`, `--output-format json`. |
| `dr pipeline delete` | `DELETE /pipelines/{pipeline_id}` | `dr pipeline delete <pipeline-id>` | **Positional:** `<pipeline-id>` (required). |
| `dr pipeline lock` | `PATCH /pipelines/{pipeline_id}/mode` | `dr pipeline lock <pipeline-id>` <br> `dr pipeline lock <pipeline-id> --output-format json` | **Positional:** `<pipeline-id>` (required). <br> **Flags:** `--output-format json`. |

---

## Versions

| Command | API endpoint | Usage | Inputs |
|---|---|---|---|
| `dr pipeline version list` | `GET /pipelines/{pipeline_id}/versions` | `dr pipeline version list --pipeline <id>` <br> `dr pipeline version list --pipeline <id> --offset 10 --limit 5 --output-format json` | **Flags:** `--pipeline <id>` (required), `--offset <n>`, `--limit <n>`, `--output-format json`. |
| `dr pipeline version get` | `GET /pipelines/{pipeline_id}/versions/{version_id}` | `dr pipeline version get --pipeline <id> 2` <br> `dr pipeline version get --pipeline <id> 2 --output-format json` | **Positional:** `<version-id>` (positive integer, required). <br> **Flags:** `--pipeline <id>` (required), `--output-format json`. |
| `dr pipeline graph` | `GET /pipelines/{pipeline_id}/graph` (draft) <br> `GET /pipelines/{pipeline_id}/versions/{version_id}/graph` (locked) | `dr pipeline graph --pipeline <id>` (draft) <br> `dr pipeline graph --pipeline <id> --version=2` (locked) <br> `dr pipeline graph --pipeline <id> --version=2 --output-format json` | **Flags:** `--pipeline <id>` (required), `--scope draft\|locked`, `--version <n>`, `--output-format json`. |

---

## Shared flag semantics

### `--scope` / `--version` (inputs, runs, graph)

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

### `--output-format`

Every read/write verb that produces a payload accepts `--output-format json` to
emit the underlying response struct as indented JSON. Any other value is
rejected with `invalid output format: <value> (supported: json)`.

### `auth` / `--skip-auth`

All verbs run `auth.EnsureAuthenticatedE` as their `PreRunE`. Pass the
global `--skip-auth` flag (or set `DATAROBOT_CLI_SKIP_AUTH=true`) when
exercising a local API stub that doesn't implement `/version/`.

---

## Inputs (`dr pipeline input …`)

Inputs exist in two scopes — **draft** and **locked** — selected via `--scope` / `--version`.

| Command | API endpoint | Usage | Inputs |
|---|---|---|---|
| `dr pipeline input create` | `POST /pipelines/{id}/inputs` (draft) <br> `POST /pipelines/{id}/versions/{ver}/inputs` (locked) | `dr pipeline input create --pipeline <id> ./payload.json` <br> `dr pipeline input create --pipeline <id> --version=2 ./payload.json --output-format json` | **Positional:** `<payload-file>` (JSON object; mutually exclusive with `--from-file`). <br> **Flags:** `--pipeline <id>` (required), `--scope`, `--version`, `--from-file=<path>`, `--output-format json`. |
| `dr pipeline input list` | `GET /pipelines/{id}/inputs` (draft) <br> `GET /pipelines/{id}/versions/{ver}/inputs` (locked) | `dr pipeline input list --pipeline <id>` | **Flags:** `--pipeline <id>` (required), `--scope`, `--version`, `--offset <n>`, `--limit <n>`, `--output-format json`. |
| `dr pipeline input get` | `GET /pipelines/{id}/inputs/{input_id}` (draft) <br> `GET /pipelines/{id}/versions/{ver}/inputs/{input_id}` (locked) | `dr pipeline input get --pipeline <id> <input-id>` | **Positional:** `<input-id>` (required). **Flags:** `--pipeline <id>` (required), `--scope`, `--version`, `--output-format json`. |
| `dr pipeline input update` | `PATCH /pipelines/{id}/inputs/{input_id}` (draft only) | `dr pipeline input update --pipeline <id> <input-id> ./payload.json` | **Positional:** `<input-id>` (required), `<payload-file>`. **Flags:** `--pipeline <id>` (required), `--from-file=<path>`, `--output-format json`. |
| `dr pipeline input delete` | `DELETE /pipelines/{id}/inputs/{input_id}` (draft) <br> `DELETE /pipelines/{id}/versions/{ver}/inputs/{input_id}` (locked) | `dr pipeline input delete --pipeline <id> <input-id>` | **Positional:** `<input-id>` (required). **Flags:** `--pipeline <id>` (required), `--scope`, `--version`. |

---


## Runs (`dr pipeline run …`)

Same draft/locked scope rules as graph. The wire-level URLs still use the legacy
term `dispatches` / `dispatch_id`, but the CLI's `--output-format json` remaps these to
`run_id` / `covalent_run_id`.

| Command | API endpoint | Usage | Inputs |
|---|---|---|---|
| `dr pipeline run create` | `POST /pipelines/{id}/dispatches` (draft) <br> `POST /pipelines/{id}/versions/{ver}/dispatches` (locked) | `dr pipeline run create --pipeline <id> --input <input-id>` <br> `dr pipeline run create --pipeline <id> --version=2 --input <input-id> --output-format json` | **Flags:** `--pipeline <id>` (required), `--input <input-id>` (required), `--scope`, `--version`, `--output-format json`. |
| `dr pipeline run list` | `GET /pipelines/{id}/dispatches` (draft) <br> `GET /pipelines/{id}/versions/{ver}/dispatches` (locked) | `dr pipeline run list --pipeline <id>` <br> `dr pipeline run list --pipeline <id> --version=2 --output-format json` | **Flags:** `--pipeline <id>` (required), `--scope`, `--version`, `--offset <n>`, `--limit <n>`, `--output-format json`. |
| `dr pipeline run get` | `GET /pipelines/{id}/dispatches/{dispatch_id}` (draft) <br> `GET /pipelines/{id}/versions/{ver}/dispatches/{dispatch_id}` (locked) | `dr pipeline run get --pipeline <id> <run-id>` | **Positional:** `<run-id>` (required). **Flags:** `--pipeline <id>` (required), `--scope`, `--version`, `--output-format json`. |
| `dr pipeline run status` | `GET /pipelines/{id}/dispatches/{dispatch_id}/status` (draft) <br> `GET /pipelines/{id}/versions/{ver}/dispatches/{dispatch_id}/status` (locked) | `dr pipeline run status --pipeline <id> <run-id>` | **Positional:** `<run-id>` (required). **Flags:** `--pipeline <id>` (required), `--scope`, `--version`, `--output-format json`. |
| `dr pipeline run cancel` | `DELETE /pipelines/{id}/dispatches/{dispatch_id}` (draft) <br> `DELETE /pipelines/{id}/versions/{ver}/dispatches/{dispatch_id}` (locked) | `dr pipeline run cancel --pipeline <id> <run-id>` | **Positional:** `<run-id>` (required). **Flags:** `--pipeline <id>` (required), `--scope`, `--version`. |

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
| `POST /pipelines/{id}/dispatches` | `dr pipeline run create` (draft) |
| `GET /pipelines/{id}/dispatches` | `dr pipeline run list` (draft) |
| `GET /pipelines/{id}/dispatches/{dispatch_id}` | `dr pipeline run get` (draft) |
| `DELETE /pipelines/{id}/dispatches/{dispatch_id}` | `dr pipeline run cancel` (draft) |
| `POST /pipelines/{id}/inputs` | `dr pipeline input create` (draft) |
| `GET /pipelines/{id}/inputs` | `dr pipeline input list` (draft) |
| `GET /pipelines/{id}/inputs/{input_id}` | `dr pipeline input get` (draft) |
| `PATCH /pipelines/{id}/inputs/{input_id}` | `dr pipeline input update` |
| `DELETE /pipelines/{id}/inputs/{input_id}` | `dr pipeline input delete` (draft) |
