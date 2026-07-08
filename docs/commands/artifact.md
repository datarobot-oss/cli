# `dr artifact` - Artifact management

Build and manage the container artifacts that back your DataRobot workloads. An artifact bundles one or more container images, built from your code or pulled from a registry, into a spec that a workload can later run. The `dr artifact` group is a thin wrapper over the Workload API: each subcommand maps to a single operation on an artifact, its builds, or its code.

## Synopsis

```bash
dr artifact <command> [subcommand] [flags]
```

## Description

An **artifact** is a versioned container spec made up of one or more container groups. Every artifact moves through a one-way lifecycle:

- `create` registers a **draft** from a JSON or YAML spec.
- `code` and `build` fill in the container images (see below).
- `lock` promotes the draft to **locked**. A locked artifact gets a version number, its name and spec become immutable, and it can no longer be deleted or unlocked.

Each container in the spec is one of two kinds:

- **Prebuilt**&mdash;set `imageUri` to an existing image. There is nothing to build.
- **Built from source**&mdash;set `imageBuildConfig`, push your code with `dr artifact code sync`, and produce an image with `dr artifact build create`. The Dockerfile is either one you provide (`./Dockerfile`) or one the server generates from a base environment.

The `code` subcommands keep a local directory in sync with an artifact's source. They store a `.wapi/` state directory at the project root, much like `.git/`: local work happens in the project root while `.wapi/` records the remote binding and last-synced state. Once a directory is linked with `dr artifact code init`, the `build` and `code` subcommands read the artifact id from `.wapi/config.json`, so you can leave it off.

A locked, fully built artifact is what you hand to `dr workload create` to deploy. See [`dr workload`](workload.md).

> [!NOTE]
> The `artifact` command is currently behind a feature gate. Enable it by exporting `DATAROBOT_CLI_FEATURE_WORKLOAD=true` before running any `dr artifact` subcommand. See [Feature gates](../development/feature-gates.md) for details.
>
> **First time?** If you're new to the CLI, start with the [Quick start](../../README.md#quick-start) for step-by-step setup instructions.

## Quick start

```bash
# Register a draft artifact from a spec file
dr artifact create --spec-file spec.yaml

# Link the current directory to it, then push your code
dr artifact code init ARTIFACT_ID
dr artifact code sync

# Build the container image and wait for it to finish
dr artifact build create --wait

# Lock the artifact once the build succeeds
dr artifact lock ARTIFACT_ID
```

## Command groups

| Command               | Endpoint                               | Purpose                                                                    |
| --------------------- | -------------------------------------- | -------------------------------------------------------------------------- |
| `dr artifact create`  | `POST   /api/v2/artifacts/`            | Register a draft artifact from a JSON/YAML spec.                           |
| `dr artifact get`     | `GET    /api/v2/artifacts/{id}/`       | Show a single artifact.                                                    |
| `dr artifact list`    | `GET    /api/v2/artifacts/`            | List artifacts, optionally filtered by status.                             |
| `dr artifact lock`    | `PATCH  /api/v2/artifacts/{id}/`       | Promote a draft to locked (immutable).                                     |
| `dr artifact delete`  | `DELETE /api/v2/artifacts/{id}/`       | Delete an artifact.                                                        |
| `dr artifact build …` | `…/artifacts/{id}/builds[/{build-id}]` | Trigger, inspect, and read logs from image builds.                         |
| `dr artifact code …`  | DataRobot catalog (Files API)          | Sync local code with an artifact (`init`, `sync`, `versions`, `checkout`). |

## Subcommands

### `create`

Register a new draft artifact from a JSON or YAML spec file. The spec needs a `name` and at least one container group with at least one container. JSON is sent to the server byte-for-byte; YAML is converted to JSON first, so quote any value that must stay a string (for example `"0644"`). On a shape mismatch the server returns a `422` naming the offending JSON path.

```bash
dr artifact create --spec-file FILE_PATH [--output-format text|json]
```

**Flags:**

- `--spec-file FILE_PATH`&mdash;path to the JSON or YAML spec (required).
- `--output-format <text|json>`: output format. Defaults to `text`.

**Example:**

```yaml
# spec.yaml - a single prebuilt container
name: my-agent
spec:
  containerGroups:
    - containers:
        - imageUri: nginx:latest
          port: 8080
          primary: true
```

```bash
dr artifact create --spec-file spec.yaml
```

To build from source instead of using a prebuilt image, replace the container's `imageUri` with an `imageBuildConfig`:

```yaml
name: my-agent
spec:
  containerGroups:
    - containers:
        - primary: true
          port: 8080
          imageBuildConfig:
            dockerfile:
              source: provided   # build from ./Dockerfile in your synced code
```

### `get`

Show a single artifact: its name, status, code reference, and timestamps.

```bash
dr artifact get ARTIFACT_ID [--output-format text|json]
```

### `list`

List artifacts, most useful with a status filter.

```bash
dr artifact list [--status draft|locked] [--limit N] [--output-format text|json]
```

**Flags:**

- `--status <draft|locked>`: only show artifacts in this status.
- `--limit <N>`: maximum number to return. Defaults to `100`.
- `--output-format <text|json>`: output format. Defaults to `text`.

### `lock`

Promote a draft artifact to locked. Before locking, the server checks that every container built from source has its code uploaded and an image build completed; otherwise the lock is rejected and the error names what is missing. Locking is one-way.

```bash
dr artifact lock ARTIFACT_ID [--output-format text|json]
```

### `delete`

Delete an artifact by id. Locked artifacts cannot be deleted, and an artifact still referenced by a workload cannot be deleted either (the error names the blocking workloads, so delete those first). You are asked to confirm unless `--yes` is set.

```bash
dr artifact delete ARTIFACT_ID [--yes]
```

**Flags:**

- `--yes`, `-y`: skip the confirmation prompt. Also honored via `DATAROBOT_CLI_NON_INTERACTIVE=1`.

### `build`

Trigger and inspect container image builds for an artifact. Inside a linked directory the `ARTIFACT_ID` argument can be omitted; it is read from `.wapi/config.json`.

```bash
dr artifact build create [ARTIFACT_ID] [--wait]             # trigger a build
dr artifact build list   [ARTIFACT_ID] [--limit N]          # list builds, newest first
dr artifact build get    [ARTIFACT_ID] BUILD_ID [--wait]  # show one build
dr artifact build logs   [ARTIFACT_ID] BUILD_ID [--level debug|info|warn|error]
```

`build create` prints the new build id(s) and returns right away. With `--wait` it polls until each build reaches a terminal status (`COMPLETED`, `FAILED`, or `CANCELLED`), prints a summary with the duration and resulting image, and on failure dumps the tail of the build log. `build logs` shows one structured record per line and hides anything below `info` unless you lower `--level`.

### `code`

Synchronize a local project directory with an artifact's source code. Run `init` once to link a directory, then `sync` to push and pull changes.

```bash
dr artifact code init     [ARTIFACT_ID] [--dir FILE_PATH] [--yes]
dr artifact code sync     [--dir FILE_PATH] [--dry-run | --diff] [--yes]
dr artifact code versions [--dir FILE_PATH] [--limit N]
dr artifact code checkout [VERSION] [--dir FILE_PATH] [--clean]
```

- `init` creates the `.wapi/` state directory and binds it to an existing draft artifact. The artifact must already exist (`dr artifact create` or the DataRobot UI); these commands manage an artifact's code, not its lifecycle.
- `sync` computes a three-way diff against the last synced state and applies it in one versioned step. Conflicts resolve to the remote copy, and your version is kept as a `*.LOCAL.<timestamp>` file. Preview with `--dry-run`, or use `--diff` to also see per-file diffs. Both exit before any remote write.
- `versions` lists the artifact's catalog versions, marking the one the artifact currently points at (`*`) and noting the one you last synced.
- `checkout` downloads a version into `.wapi/.checkouts/<version-id>/` for read-only inspection; your working directory is left untouched. `--clean` removes checkout directories instead of downloading.

## Shared flags

### `--output-format`

Every subcommand that prints a resource accepts `--output-format json` for machine-parseable output. The default, `text`, is human-readable.

### `--yes`

Commands that prompt (`delete`, `code init`, `code sync`, `code checkout`) accept `--yes` / `-y` to skip the prompt. Setting `DATAROBOT_CLI_NON_INTERACTIVE=1` does the same, and prompts are skipped automatically when stdin is not a terminal.

### Global options

All [global flags](README.md#global-flags) are available, notably `--debug` for protocol-level tracing.

## Examples

### Build and lock an artifact from source

```bash
dr artifact create --spec-file spec.yaml   # prints the new artifact id
dr artifact code init ARTIFACT_ID
dr artifact code sync                       # upload your code
dr artifact build create --wait             # build the image, wait for it
dr artifact lock ARTIFACT_ID              # freeze it for deployment
```

### Inspect builds and logs

```bash
dr artifact build list ARTIFACT_ID
dr artifact build get  ARTIFACT_ID BUILD_ID --wait
dr artifact build logs ARTIFACT_ID BUILD_ID --level debug
```

## Error handling

| Status | Cause                                                                                                             |
| ------ | ----------------------------------------------------------------------------------------------------------------- |
| `404`  | The artifact, build, or version does not exist.                                                                   |
| `409`  | Tried to delete a locked artifact, delete one still referenced by a workload, or lock an already-locked artifact. |
| `422`  | The spec failed server validation; the response names the offending JSON path.                                    |

## See also

- [`dr workload`](workload.md)&mdash;deploy a locked artifact as a running workload.
- [Authentication](auth.md)&mdash;how `dr auth login` and `--skip-auth` interact.
- [Configuration](../user-guide/configuration.md)&mdash;config file and environment-variable precedence.
- [Feature gates](../development/feature-gates.md)&mdash;turning `DATAROBOT_CLI_FEATURE_WORKLOAD` on and off.
