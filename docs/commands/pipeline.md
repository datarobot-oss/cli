# `dr pipeline` - Pipelines API management

Manage AI/ML pipelines orchestrated by Covalent through the DataRobot
pipelines service. The `dr pipeline` group is a thin CLI wrapper over
the pipelines REST API: every subcommand maps directly to a single
endpoint.

## Synopsis

```bash
dr pipeline <command> [subcommand] [flags]
```

## Description

A **pipeline** is a versioned bundle of Python source defining a DataRobot pipeline (one or more tasks). Each
top-level `dr pipeline` subcommand operates on one of four resources:

- the **pipeline** itself (create, list, get, update, delete, lock),
- pipeline **versions** (list, get, graph),
- pipeline **inputs** — JSON payloads supplied to a run,
- pipeline **runs** — concrete executions on Covalent,
- pipeline **schedules** — recurring runs on a cron expression,
- pipeline **environments** — named, immutable-versioned bags of pip
  packages that pipelines can be built against.

Versions are created automatically:

- The first `create` call registers the source as **v1** in `draft`
  mode.
- `update` re-uploads the same file (or an edited copy) and appends
  **v2**, **v3**, etc., as long as the pipeline name still matches and
  the pipeline is still in `draft` mode.
- `lock` promotes a draft to **locked** mode. Locked pipelines are
  immutable; their inputs and schedules become valid.

Inputs, runs, and the graph endpoint exist in two scopes —
**draft** (mutable, no version pinned) and **locked** (immutable, tied
to a frozen version) — selected via the shared `--scope` and
`--version` flags. Schedules are locked-only.

> [!NOTE]
> The `pipeline` command is currently behind a feature gate. Enable it
> by exporting `DATAROBOT_CLI_FEATURE_PIPELINE=true` before running any
> `dr pipeline` subcommand. See
> [Feature gates](../development/feature-gates.md) for details.

> [!NOTE]
> **First time?** If you're new to the CLI, start with the
> [Quick start](../../README.md#quick-start) for step-by-step setup
> instructions.

## Quick start

```bash
# List pipelines registered with the pipelines service
dr pipeline list

# Register a new draft pipeline by uploading a DataRobot pipeline source file
dr pipeline create ./my_pipeline.py --description "First draft"

# Append a new version after editing the file
dr pipeline update <pipeline-id> ./my_pipeline.py

# Promote the draft to locked when you are happy with it
dr pipeline lock <pipeline-id>
```

## Command groups

| Group                    | Endpoint(s)                          | Purpose                                          |
|--------------------------|--------------------------------------|--------------------------------------------------|
| `dr pipeline create`    | `POST   /api/v2/pipelines`           | Upload a Python file to register a new pipeline. |
| `dr pipeline list`      | `GET    /api/v2/pipelines`           | Paginated list with mode filtering.              |
| `dr pipeline get`       | `GET    /api/v2/pipelines/{id}`      | Pipeline detail including all versions.          |
| `dr pipeline update`    | `PATCH  /api/v2/pipelines/{id}`      | Re-upload a file to append a new version.        |
| `dr pipeline delete`    | `DELETE /api/v2/pipelines/{id}`      | Remove a pipeline and all of its versions.       |
| `dr pipeline lock`      | `PATCH  /api/v2/pipelines/{id}/mode` | Promote a draft to locked mode.                  |
| `dr pipeline version …` | `…/versions[/{ver}]`                 | Inspect pipeline versions.                       |
| `dr pipeline graph`     | `…/graph` (draft or locked)          | Render the pipeline/task DAG.                    |

## Subcommands

### `create`

Upload a Python file defining a DataRobot pipeline (one or more tasks) and
register a new pipeline. The pipeline name is extracted from the file and used as the
pipeline name.

```bash
dr pipeline create <file> [flags]
dr pipeline create --from-file=<file> [flags]
```

**Arguments:**

- `<file>` — path to a `.py` file containing a single DataRobot pipeline.
  Mutually exclusive with `--from-file`.

**Flags:**

- `--from-file <path>` — alternative to the positional file argument.
- `--description <text>` — optional human-readable description stored on
  the pipeline.
- `--mode <draft|locked>` — pipeline lifecycle mode. Defaults to `draft`.
- `--output <json>` — emit machine-parseable JSON instead of the
  human-readable summary.

**Example:**

```bash
$ dr pipeline create ./confluence_to_vdb.py --description "test"
Pipeline ID:  683c2a1b4f8e1a2b3c4d5e6f
Name:         confluence_to_vdb
Version:      1
Status:       READY
Mode:         draft
Tasks:        create_vector_database, ingest_confluence_files, setup_credential_and_datastore
Created:      2026-04-28T11:42:28Z
```

### `list`

List pipelines registered with the pipelines service, with optional
mode filtering and pagination.

```bash
dr pipeline list [flags]
```

**Flags:**

- `--mode <draft|locked>` — filter by pipeline mode.
- `--offset <N>` — pagination offset. Default `0`.
- `--limit <N>` — pagination limit (1-200). Default `50`.
- `--output <json>` — emit machine-parseable JSON instead of a table.

**Example:**

```bash
$ dr pipeline list
Showing 1 of 1 (offset=0 limit=50)

ID                        NAME               MODE   ACTIVE  VERSION  UPDATED
683c2a1b4f8e1a2b3c4d5e6f  confluence_to_vdb  draft  true    v3       2026-04-28T12:25:11Z
```

### `get`

Display full details of a single pipeline including all versions.

```bash
dr pipeline get <pipeline-id> [flags]
```

**Arguments:**

- `<pipeline-id>` — the ObjectId returned by `create` / shown in `pipeline list`.

**Flags:**

- `--output <json>` — emit machine-parseable JSON.

**Example:**

```bash
$ dr pipeline get 683c2a1b4f8e1a2b3c4d5e6f
ID:          683c2a1b4f8e1a2b3c4d5e6f
Name:        confluence_to_vdb
Mode:        draft
Active:      true
Created:     2026-04-28T11:42:28Z
Updated:     2026-04-28T12:25:11Z

Versions (3):
  VERSION  STATUS  PYTHON  CREATED               TASKS
  v1       READY   3.12    2026-04-28T11:42:28Z  create_vector_database
  v2       READY   3.12    2026-04-28T12:24:54Z  create_vector_database
  v3       READY   3.12    2026-04-28T12:25:11Z  create_vector_database
```

If the pipeline doesn't exist, `get` prints
`No pipeline found with id: <id>` and exits 0.

### `update`

Re-upload a Python file to update a draft pipeline. A new version is
appended.

```bash
dr pipeline update <pipeline-id> <file> [flags]
dr pipeline update <pipeline-id> --from-file=<file> [flags]
```

**Constraints:**

- The pipeline name encoded in the uploaded file **must match** the pipeline's
  existing name.
- Locked pipelines cannot be updated (API responds `409 Conflict`).

**Flags:**

- `--from-file <path>` — alternative to the positional file argument.
- `--output <json>` — emit machine-parseable JSON.

### `delete`

Delete a pipeline and all of its versions.

```bash
dr pipeline delete <pipeline-id>
```

If the pipeline doesn't exist, `delete` prints
`No pipeline found with id: <id>` and exits 0.

### `lock`

Promote a draft pipeline to locked mode. Once locked, the pipeline can
no longer be updated.

```bash
dr pipeline lock <pipeline-id> [flags]
```

**Flags:**

- `--output <json>` — emit machine-parseable JSON.

### `version`

Read-only access to pipeline versions.

```bash
dr pipeline version list --pipeline <id> [--offset N] [--limit N] [--output json]
dr pipeline version get  --pipeline <id> <version-id>     [--output json]
```

### `graph`

Display the pipeline/task DAG as either a JSON payload or a human-readable summary.

```bash
dr pipeline graph --pipeline <id>                       # draft graph
dr pipeline graph --pipeline <id> --version=N           # locked-version graph
dr pipeline graph --pipeline <id> --output json
```

## Shared flags

### `--from-file` / positional file

`pipeline create` and `pipeline update` accept the input file in two equivalent ways:

```bash
dr pipeline create ./my_pipeline.py
dr pipeline create --from-file=./my_pipeline.py
```

### `--output`

Every verb that produces a payload accepts `--output json` to emit the response struct as indented JSON.

### Global options

All [global flags](README.md#global-flags) are available, notably
`--debug` for protocol-level tracing and `--skip-auth` for advanced scenarios.

## Local development

While iterating against a locally running pipelines-api (default port `8100`), point the CLI at
`http://localhost:8100` and bypass token verification:

```bash
export DATAROBOT_CLI_FEATURE_PIPELINE=true
export DATAROBOT_CLI_ENDPOINT=http://localhost:8100/api/v2
export DATAROBOT_CLI_TOKEN=local
export DATAROBOT_CLI_SKIP_AUTH=true

./dist/dr pipeline list
```

## Examples

### Pipeline lifecycle

```bash
# Register a draft, append a version, lock it, then delete it
dr pipeline create ./my_pipeline.py --description "Initial draft"
dr pipeline update <pipeline-id> ./my_pipeline.py
dr pipeline lock   <pipeline-id>
dr pipeline delete <pipeline-id>
```

### Inspect versions and graph

```bash
dr pipeline version list --pipeline <pipeline-id>
dr pipeline version get  --pipeline <pipeline-id> 2
dr pipeline graph        --pipeline <pipeline-id> --version=2 --output json
```

## Error handling

| Status | Cause                                                                          |
|--------|--------------------------------------------------------------------------------|
| `400`  | Invalid Python file or mismatched pipeline name.                               |
| `404`  | The provided `<pipeline-id>` or version does not exist.                        |
| `409`  | Tried to update a `locked` pipeline.                                           |

## See also

- [Authentication](auth.md) — how `dr auth login` and `--skip-auth`
  interact.
- [Configuration](../user-guide/configuration.md) — config file and
  environment-variable precedence.
- [Feature gates](../development/feature-gates.md) — flipping
  `DATAROBOT_CLI_FEATURE_PIPELINE` on and off.
