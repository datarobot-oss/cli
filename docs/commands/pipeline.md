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
- pipeline **images** — named, immutable-versioned execution environments (pip
  packages, conda packages, base image, NVIDIA GPU support) that pipelines can
  be built against,
- pipeline **tasks** — source code, function signature, and input payload
  for individual `@task`-decorated functions.

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
| `dr pipeline source`    | `…/source` (draft or locked)         | Retrieve the pipeline's Python source code.      |
| `dr pipeline run …`     | `…/dispatches` and `…/{id}`          | Trigger, inspect, and cancel runs.               |
| `dr pipeline input …`   | `…/inputs` and `…/inputs/{input_id}` | Manage JSON payloads for runs.                   |
| `dr pipeline schedule …` | `…/versions/{ver}/schedules`        | Manage recurring (cron) runs on locked versions. |
| `dr pipeline image …` | `/pipelines/images[/{id}]` | Manage named, versioned pip-package images. |
| `dr pipeline task …`        | `…/tasks/{task_id}` (draft or locked) | Inspect individual task source, signature, and inputs. |

## Subcommands

### `create`

Upload a Python file defining a DataRobot pipeline (one or more tasks) and
register a new pipeline. The display name defaults to the title-cased `@pipeline`
function name (e.g. `my_workflow` → `My Workflow`); supply `--name` to use a
custom label instead.

```bash
dr pipeline create <file> [flags]
dr pipeline create --from-file=<file> [flags]
```

**Arguments:**

- `<file>` — path to a `.py` file containing a single DataRobot pipeline.
  Mutually exclusive with `--from-file`.

**Flags:**

- `--from-file <path>` — alternative to the positional file argument.
- `--name <text>` — optional human-readable display name. Defaults to the
  title-cased `@pipeline` function name when omitted.
- `--description <text>` — optional human-readable description stored on
  the pipeline.
- `--mode <draft|locked>` — pipeline lifecycle mode. Defaults to `draft`.
- `--image <image-id>` — optional execution image to associate with the
  pipeline. The pipeline's runs will use this image's environment. Image IDs
  are obtained from `dr pipeline image create` or `dr pipeline image list`.
- `--output-format <json>` — emit machine-parseable JSON instead of the
  human-readable summary.

**Example:**

```bash
$ dr pipeline create ./confluence_to_vdb.py --description "test"
Pipeline ID:  683c2a1b4f8e1a2b3c4d5e6f
Name:         Confluence To Vdb
Version:      1
Status:       READY
Mode:         draft
Tasks:        create_vector_database, ingest_confluence_files, setup_credential_and_datastore
Created:      2026-04-28T11:42:28Z

$ dr pipeline create ./confluence_to_vdb.py --name "My Confluence VDB Pipeline"
Pipeline ID:  683c2a1b4f8e1a2b3c4d5e70
Name:         My Confluence VDB Pipeline
Version:      1
Status:       READY
Mode:         draft
Tasks:        create_vector_database, ingest_confluence_files, setup_credential_and_datastore
Created:      2026-04-28T11:43:00Z
```

When `--image` is supplied, an `Image ID:` row is shown in the response.

```bash
$ dr pipeline create ./confluence_to_vdb.py --image 683c000000000000000000ab
Pipeline ID:  683c2a1b4f8e1a2b3c4d5e6f
Name:         confluence_to_vdb
Version:      1
Status:       READY
Mode:         draft
Tasks:        create_vector_database, ingest_confluence_files, setup_credential_and_datastore
Image ID:     683c000000000000000000ab
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
- `--output-format <json>` — emit machine-parseable JSON instead of a table.

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

- `--output-format <json>` — emit machine-parseable JSON.

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

When a pipeline has a linked execution image, an `Image:` row is shown
(format: `<imageId> (v<n>, <status>)`). When an `inputSetTemplate` is set,
an `Input template:` section is printed below the header block.

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
- `--name <text>` — new display name for the pipeline.
- `--description <text>` — new description for the pipeline.
- `--image <image-id>` — execution image to associate with the pipeline.
  Image IDs are obtained from `dr pipeline image create` or `dr pipeline image list`.
- `--output-format <json>` — emit machine-parseable JSON.

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

- `--output-format <json>` — emit machine-parseable JSON.

### `version`

Read-only access to pipeline versions.

```bash
dr pipeline version list --pipeline <id> [--offset N] [--limit N] [--output-format json]
dr pipeline version get  --pipeline <id> <version-id>     [--output-format json]
```

### `graph`

Display the pipeline/task DAG as either a JSON payload or a human-readable summary.
The human table includes a **TASK ID** column showing the stable identifier for each
task node (populated once CMPT-6040 is deployed; `—` for legacy pipelines).

```bash
dr pipeline graph --pipeline <id>                       # draft graph
dr pipeline graph --pipeline <id> --version=N           # locked-version graph
dr pipeline graph --pipeline <id> --output-format json  # includes taskId on each node
```

### `source`

Retrieve the Python source file of a pipeline as uploaded.

```bash
dr pipeline source --pipeline <id>                       # draft source
dr pipeline source --pipeline <id> --version=N           # locked-version source
dr pipeline source --pipeline <id> --output-format json  # returns {"source": "..."}
```

**Flags:**

- `--pipeline <id>` — pipeline ObjectId (required).
- `--scope <draft|locked>` — scope selector. Default `draft`.
- `--version <n>` — locked version number; implies `--scope=locked`.
- `--output-format <json>` — emit the source wrapped in a JSON object.

## Shared flags

### `--from-file` / positional file

`pipeline create` and `pipeline update` accept the input file in two equivalent ways:

```bash
dr pipeline create ./my_pipeline.py
dr pipeline create --from-file=./my_pipeline.py
```

### `--output-format`

Every verb that produces a payload accepts `--output-format json` to emit the response struct as indented JSON.

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

### Inspect versions, graph, and source

```bash
dr pipeline version list --pipeline <pipeline-id>
dr pipeline version get  --pipeline <pipeline-id> 2
dr pipeline graph        --pipeline <pipeline-id> --version=2 --output-format json
dr pipeline source       --pipeline <pipeline-id>             # draft source
dr pipeline source       --pipeline <pipeline-id> --version=2 # locked version source
```

### Inspect a task

```bash
# 1. Find task IDs via the graph (TASK ID column)
dr pipeline graph --pipeline <pipeline-id>

# 2. View source + signature for a draft task
dr pipeline task get --pipeline <pipeline-id> <task-id>

# 3. View the same task on a locked version (includes input payload)
dr pipeline task get --pipeline <pipeline-id> --version=2 <task-id>
```

### `input`

Manage JSON payloads that drive a run.

```bash
dr pipeline input create --pipeline <id> <payload-file>              # draft scope
dr pipeline input create --pipeline <id> --version=N <payload-file>  # locked scope
dr pipeline input list   --pipeline <id> [--scope|--version] [--offset N] [--limit N]
dr pipeline input get    --pipeline <id> <input-id>      [--scope|--version]
dr pipeline input update --pipeline <id> <input-id> <payload-file>   # draft only
dr pipeline input delete --pipeline <id> <input-id>      [--scope|--version]
```

The payload file must contain a JSON object. The CLI wraps it in `{"payload": …}` before sending.

### `schedule`

Manage recurring (cron) runs on locked versions only. Both `--pipeline` and `--version` are
required for every verb.

```bash
dr pipeline schedule create --pipeline <id> --version=N \
    --cron "0 * * * *" --input <input-id> [--timezone UTC]
dr pipeline schedule list   --pipeline <id> --version=N [--offset N] [--limit N]
dr pipeline schedule get    --pipeline <id> --version=N <schedule-id>
dr pipeline schedule update --pipeline <id> --version=N <schedule-id> --cron "*/15 * * * *"
dr pipeline schedule delete --pipeline <id> --version=N <schedule-id>
```

`schedule update` requires at least one of `--cron` or `--timezone`.
### `run`

Trigger, inspect, and cancel pipeline executions.

```bash
dr pipeline run create --pipeline <id> --input <input-id> --image <img-id>              # draft
dr pipeline run create --pipeline <id> --version=N --input <input-id> --image <img-id>  # locked
dr pipeline run list   --pipeline <id> [--scope|--version]
dr pipeline run get    --pipeline <id> <run-id> [--scope|--version]
dr pipeline run status --pipeline <id> <run-id> [--scope|--version]
dr pipeline run cancel --pipeline <id> <run-id> [--scope|--version]
```

`run create` requires `--image <image-id>` — the execution image to use for
this run. Image IDs are obtained from `dr pipeline image create` or
`dr pipeline image list`.

`run status` is a lighter-weight call intended for polling — returns just
the run ID, status, and Covalent dispatch ID.

`run cancel` returns `409 Conflict` if the run is already terminal.

### `image`

Manage pipeline execution images — named, immutable-versioned environments (pip
packages, conda packages, base Docker image, NVIDIA GPU support) that pipelines
can be built against. Each `update` appends a new version; individual older versions
can be removed with `image version delete`.

```bash
# pip-only image
dr pipeline image create --name <name> --package <pkg> [--package <pkg> …] [--description <text>] [--output-format json]

# conda image (channels optional; --conda-channel requires at least one --conda)
dr pipeline image create --name <name> --conda <pkg> [--conda <pkg> …] [--conda-channel <ch>]

# combined pip + conda + base image + nvidia
dr pipeline image create --name gpu-base --package torch --conda scipy \
    --base-image nvcr.io/nvidia/pytorch:24.01-py3 --nvidia

dr pipeline image list   [--offset N] [--limit N] [--output-format json]
dr pipeline image update <image-id> --package <pkg> [--package <pkg> …] [--output-format json]
dr pipeline image update <image-id> --conda <pkg> [--conda-channel <ch>] [--base-image <uri>] [--nvidia]
dr pipeline image delete <image-id>
dr pipeline image version delete --image <image-id> <version>
```

`image create` registers a new image with an initial version (v1). At least one of
`--package` (pip) or `--conda` must be supplied; `--conda-channel` requires at
least one `--conda` package. `image update` appends a new immutable version with
the supplied definition — all fields must be re-specified since the server does not
carry over the previous version's packages. `image delete` soft-deletes the latest
active version (cascading to the parent if no active versions remain).
`image version delete` targets a specific version by its integer number.

**`image create` / `image update` flags:**

- `--package <spec>` — pip package spec (repeatable, also accepts comma-separated values).
- `--conda <spec>` — conda package spec (repeatable).
- `--conda-channel <channel>` — conda channel (repeatable); requires at least one `--conda`.
- `--base-image <uri>` — Docker base image URI (e.g. `python:3.12`).
- `--nvidia` — enable NVIDIA GPU support.

### `task`

Inspect individual `@task`-decorated functions within a pipeline. Task IDs are stable
24-char identifiers minted when a pipeline is uploaded; they appear in the **TASK ID**
column of `dr pipeline graph` and are preserved across re-uploads and across the
draft-to-locked transition.

```bash
dr pipeline task get --pipeline <id> <task-id>                   # draft — source + params, inputs=null
dr pipeline task get --pipeline <id> --version=N <task-id>       # locked — source + params + latest VALID input payload
dr pipeline task get --pipeline <id> <task-id> --output-format json
```

`task get` returns the task's Python source string, its `@task` function signature
parameters (name + optional type annotation), and — for locked versions — the full
payload from the latest `VALID` `PipelineInput` record for that version.

If the task ID is not found, the command prints `Task not found: <task-id>` and exits 0.

## Error handling

| Status | Cause                                                                          |
|--------|--------------------------------------------------------------------------------|
| `400`  | Invalid Python file or mismatched pipeline name.                               |
| `404`  | The provided `<pipeline-id>`, version, or run does not exist.                  |
| `409`  | Tried to update a `locked` pipeline, or cancel an already-terminal run.        |

## See also

- [Authentication](auth.md) — how `dr auth login` and `--skip-auth`
  interact.
- [Configuration](../user-guide/configuration.md) — config file and
  environment-variable precedence.
- [Feature gates](../development/feature-gates.md) — flipping
  `DATAROBOT_CLI_FEATURE_PIPELINE` on and off.
