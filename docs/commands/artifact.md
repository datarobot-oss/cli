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

- **Prebuilt**: set `imageUri` to an existing image. There is nothing to build.
- **Built from source**: set `imageBuildConfig`, push your code with `dr artifact code sync`, and produce an image with `dr artifact build create`. The Dockerfile is either one you provide (`./Dockerfile`) or one the server generates from a base environment.

The `code` subcommands keep a local directory in sync with an artifact's source. They store a `.datarobot/workload/` state directory at the project root, much like `.git/`: local work happens in the project root while `.datarobot/workload/` records the remote binding and last-synced state. Once a directory is linked with `dr artifact code init`, the `build` and `code` subcommands read the artifact id from `.datarobot/workload/config.json`, so you can leave it off. A project linked by an older CLI keeps its state in a root `.wapi/`; the next command that touches state relocates it and prints one line saying so.

A locked, fully built artifact is what you hand to `dr workload create` to deploy. See [`dr workload`](workload.md).

> [!NOTE]
> **First time?** If you're new to the CLI, start with the [Quick start](https://github.com/datarobot-oss/cli/blob/main/README.md#quick-start) for step-by-step setup instructions.

## Quick start

```bash
# Register a draft artifact from a spec file
dr artifact create --spec-file spec.yaml

# Link the current directory to it, then push your code
dr artifact code init <artifact-id>
dr artifact code sync

# Build the container image and wait for it to finish
dr artifact build create --wait

# Lock the artifact once the build succeeds
dr artifact lock <artifact-id>
```

The same path with the deploy on the end, and with what each step writes, is in the [spec reference](workload-spec.md#deploy-a-service-from-source-end-to-end).

## Command groups

| Command               | Endpoint                               | Purpose                                                                    |
| --------------------- | -------------------------------------- | -------------------------------------------------------------------------- |
| `dr artifact create`  | `POST   /api/v2/artifacts/`            | Register a draft artifact from a JSON/YAML spec.                           |
| `dr artifact get`     | `GET    /api/v2/artifacts/{id}/`       | Show a single artifact.                                                    |
| `dr artifact list`    | `GET    /api/v2/artifacts/`            | List artifacts, optionally filtered by status.                             |
| `dr artifact lock`    | `PATCH  /api/v2/artifacts/{id}/`       | Promote a draft to locked (immutable).                                     |
| `dr artifact delete`  | `DELETE /api/v2/artifacts/{id}/`       | Delete an artifact.                                                        |
| `dr artifact build …`    | `…/artifacts/{id}/builds[/{build-id}]` | Trigger and inspect image builds.                                       |
| `dr artifact build logs` | `GET /api/v2/otel/artifact/{id}/logs/` | Read a build's logs, served by the telemetry route rather than builds.  |
| `dr artifact code …`  | DataRobot catalog (Files API)          | Sync local code with an artifact (`init`, `sync`, `versions`, `checkout`). |

## Subcommands

### `create`

Register a new draft artifact from a JSON or YAML spec file. The spec needs a `name` and at least one container group with at least one container. JSON is sent to the server byte-for-byte; YAML is converted to JSON first, so quote any value that must stay a string (for example `"0644"`). On a shape mismatch the server returns a `422` naming the offending JSON path. Every field the spec accepts is documented in the [spec reference](workload-spec.md#artifact-spec).

```bash
dr artifact create --spec-file <path> [--output-format text|json]
```

**Flags:**

- `--spec-file <path>`: path to the JSON or YAML spec (required).
- `--output-format <text|json>`: output format. Defaults to `text`.

**Example:**

```yaml
# spec.yaml - a single prebuilt container
name: my-agent
spec:
  containerGroups:
    - name: default
      containers:
        - name: primary
          imageUri: nginx:latest
          port: 8080
          primary: true
```

```bash
dr artifact create --spec-file spec.yaml
```

Name the group and the container. Both are optional to the server, but a workload's `runtime` block addresses them by name, so an unnamed container cannot be given a replica count or a CPU allocation later.

To build from source instead of using a prebuilt image, replace the container's `imageUri` with an `imageBuildConfig`:

```yaml
name: my-agent
spec:
  containerGroups:
    - name: default
      containers:
        - name: primary
          primary: true
          port: 8080
          imageBuildConfig:
            dockerfile:
              source: provided   # build from ./Dockerfile in your synced code
```

#### Service or agent

An artifact is a `service` unless it says otherwise. The kind sits **beside** the spec, never inside it:

```yaml
name: my-agent
type: agent
spec:
  containerGroups:
    - name: default
      containers: [...]
```

The platform pops any `type` it finds under `spec` and re-derives the kind from the artifact's own `type`, so a type written one level down is discarded without a word: the agent silently becomes a service, and agent-only fields are then rejected as unknown. An artifact repository takes its kind from whichever artifact opened it, so a later version cannot change it.

#### Environment variables and secrets

`environmentVars` is a list on the container, beside `port` and the image. A plain setting carries its value; a secret is stored as a credential and referenced by id, so the value never reaches the spec or your repository:

```yaml
environmentVars:
  - name: LOG_LEVEL
    value: info
  - name: OPENAI_API_KEY
    source: dr-credential
    drCredentialId: 68b0c1d2e3f4a5b6c7d8e9f0
    key: apiToken
```

`key` names which field of the credential to use, not the variable itself. There is no `dr credential` command yet, so credentials are created in the DataRobot UI or through the API; the [spec reference](workload-spec.md#environment-variables-and-secrets) has the call and the rotation caveat.

### `get`

Show a single artifact: its name, status, repository and version, code reference, and timestamps.

```bash
dr artifact get <artifact-id> [--output-format text|json]
```

The repository is the lineage the artifact belongs to. Successive versions of one workload share it, which is what tells them apart from unrelated artifacts that happen to carry the same name. The version numbers the artifact within that repository and is assigned only on locking, so a draft shows none.

### `list`

List artifacts, most useful with a status filter.

```bash
dr artifact list [--status draft|locked] [--limit N] [--offset N] [--output-format text|json]
```

**Flags:**

- `--status <draft|locked>`: only show artifacts in this status.
- `--limit <N>`: maximum number to return. Defaults to `100`.
- `--offset <N>`: number of artifacts to skip before returning results. Defaults to `0`.
- `--output-format <text|json>`: output format. Defaults to `text`.

### `lock`

Promote a draft artifact to locked. Locking is one-way: the artifact gets a version number, its name and spec become immutable, and it can no longer be edited, unlocked or deleted.

```bash
dr artifact lock <artifact-id> [--output-format text|json]
```

> [!IMPORTANT]
> **Check the build before you lock.** The server rejects some incomplete artifacts and names what is missing, but that check is not a guarantee: an artifact whose code reference has never been built can be locked, and nothing says so until a workload tries to run an image that was never produced. Build first, then confirm the build reached `COMPLETED` and produced an image (`dr artifact build create --wait` reports both, and `dr artifact build list <artifact-id>` says so after the fact), then lock. A locked artifact cannot be repaired, so the way out of a bad lock is a new artifact.

A second `lock` on an artifact that is already locked answers `403`.

### `delete`

Delete an artifact by id. Locked artifacts cannot be deleted, and an artifact still referenced by a workload cannot be deleted either (the error names the blocking workloads, so delete those first). You are asked to confirm unless `--yes` is set.

```bash
dr artifact delete <artifact-id> [--yes]
```

**Flags:**

- `--yes`, `-y`: skip the confirmation prompt. Also honored via `DATAROBOT_CLI_NON_INTERACTIVE=1`.

### `build`

Trigger and inspect container image builds for an artifact. Inside a linked directory the `<artifact-id>` argument can be omitted; it is read from `.datarobot/workload/config.json`.

```bash
dr artifact build create [<artifact-id>] [--wait]             # trigger a build
dr artifact build list   [<artifact-id>] [--limit N]          # list builds, newest first
dr artifact build get    [<artifact-id>] <build-id> [--wait]  # show one build
dr artifact build logs   [<artifact-id>] <build-id> [--level debug|info|warn|error]
```

`build create` prints the new build id(s) and returns right away. With `--wait` it polls until each build reaches a terminal status (`COMPLETED`, `FAILED`, or `CANCELLED`), prints a summary with the duration and resulting image, and on failure dumps the tail of the build log. `build logs` shows one structured record per line and hides anything below `info` unless you lower `--level`.

### `code`

Synchronize a local project directory with an artifact's source code. Run `init` once to link a directory, then `sync` to push and pull changes.

```bash
dr artifact code init     [<artifact-id>] [--dir <path>] [--yes]
dr artifact code sync     [--dir <path>] [--dry-run | --diff] [--accept-remote] [--yes]
dr artifact code versions [--dir <path>] [--limit N]
dr artifact code checkout [<ver>] [--dir <path>] [--clean]
```

- `init` creates the `.datarobot/workload/` state directory and binds it to an existing draft artifact. The artifact must already exist (`dr artifact create` or the DataRobot UI); these commands manage an artifact's code, not its lifecycle. It also drops a starter `.drignore` at the project root, in gitignore syntax, listing what `sync` should leave out. Edit it and commit it. A project that already has an ignore file under either name keeps it, and no new one is written.
- Projects created before the file was renamed have a `.wapiignore` instead. It is still read when there is no `.drignore` beside it, and `sync` says so once per run, but the name is deprecated: rename the file when convenient. If both are present, `.drignore` is the one that applies and `sync` warns that the other file's patterns are not in effect. Merge them and delete the old one: two ignore files at a project root is a state where patterns you wrote silently stop filtering. The ignore file is uploaded with your code, so if you work with others, agree on the rename rather than doing it alone.
- `sync` computes a three-way diff against the last synced state and applies it in one versioned step. A file changed both locally and on the remote is a conflict: an interactive run asks before the remote copy is pulled over yours, and a non-interactive run (`--yes`, `DATAROBOT_CLI_NON_INTERACTIVE=1`, or no terminal) is refused unless you pass `--accept-remote`, so an automated sync fails loudly instead of overwriting your edits. A plain pull of a remote change you had not touched still applies without prompting. Whenever the remote wins, your version is kept as a `<path>.LOCAL.<timestamp>` file, and those copies are excluded from the next sync. Preview with `--dry-run`, or use `--diff` to also see per-file diffs. Both exit before any remote write.
- The first `sync` of a project creates the File Registry entry that holds its code, named `Artifact: <id>` so the row identifies its project in the Registry's Files list. It carries the artifact id rather than the artifact name because naming is create-time only: the entry keeps whatever name it was created with, so a name the project later changes would go stale in place, and artifact names are not unique to begin with (a deploy derives one from the directory). The id is the artifact that first pushed the code; after a deploy locks that artifact and mints its successor, the entry keeps naming the first version of the lineage, which still resolves. Later syncs add versions to the entry and leave its name alone, so an entry created before the CLI sent a name keeps whatever the platform picked (`Untitled Dataset`, or `wapi-sync.zip` if the first sync was large enough to take the zip route). Rename it in the UI, or with `PATCH /api/v2/catalogItems/<catalog-id>/`; the catalog id is in `.datarobot/workload/config.json`.
- For Python projects, the image build requires a `uv.lock` next to `pyproject.toml`. When your project has `pyproject.toml` but no `uv.lock`, `sync` generates one automatically by running your local `uv lock` (your uv configuration, private indexes, and credentials apply) and uploads it with the rest of your code — commit the generated file to your repo. If `uv` is not installed or lock generation fails, sync still completes and prints what to do (`uv lock`, then re-sync); the image build will fail until a lock file is added. This also happens on `--dry-run`/`--diff`, so the preview matches what a real sync would upload.
- An existing `uv.lock` is kept in step with `pyproject.toml`. `sync` checks the two against each other and re-runs `uv lock` when they have diverged, uploading the refreshed lock — commit it. This matters because nothing downstream catches a stale lock: the image build installs the lockfile as given, so editing `pyproject.toml` without re-locking used to produce an image missing the dependency you added, with sync and the build both reporting success and the failure arriving at container start as an `ImportError`. The check needs your whole project to be accurate, which is why it lives here rather than in the build. If the lock is out of date and `uv lock` cannot put it right, sync stops and uploads nothing, rather than shipping code the image will not match. If `uv` is not installed there is nothing to compare with, so sync says so and continues — a project whose lock is already current is unaffected. Like generation, this runs on `--dry-run`/`--diff` too, so the preview matches what a real sync would upload: those modes make no remote writes, but they can refresh `uv.lock`, and a lockfile that cannot be put right stops them as well. A deploy runs the same comparison before it asks you anything. `sync` also warns if your `.drignore` excludes `uv.lock`.
- `versions` lists the artifact's catalog versions, marking the one the artifact currently points at (`*`) and noting the one you last synced.
- `checkout` downloads a version into `.datarobot/workload/.checkouts/<version-id>/` for read-only inspection; your working directory is left untouched. `--clean` removes checkout directories instead of downloading.

## Shared flags

### `--output-format`

Every subcommand that prints a resource accepts `--output-format json` for machine-parseable output. The default, `text`, is human-readable.

### `--yes`

Commands that prompt (`delete`, `code init`, `code sync`, `code checkout`) accept `--yes` / `-y` to skip the prompt. Setting `DATAROBOT_CLI_NON_INTERACTIVE=1` does the same, and prompts are skipped automatically when stdin is not a terminal. The one exception is a `code sync` conflict: with the prompt skipped, the sync is refused rather than applied, unless `--accept-remote` says the remote may win.

### Global options

All [global flags](README.md#global-flags) are available, notably `--debug` for protocol-level tracing.

## Examples

### Build and lock an artifact from source

```bash
dr artifact create --spec-file spec.yaml   # prints the new artifact id
dr artifact code init <artifact-id>
dr artifact code sync                       # upload your code
dr artifact build create --wait             # build the image, wait for it
dr artifact lock <artifact-id>              # freeze it for deployment
```

### Inspect builds and logs

```bash
dr artifact build list <artifact-id>
dr artifact build get  <artifact-id> <build-id> --wait
dr artifact build logs <artifact-id> <build-id> --level debug
```

## Error handling

| Status | Cause                                                                                                             |
| ------ | ----------------------------------------------------------------------------------------------------------------- |
| `403`  | Tried to lock an already-locked artifact, or the Workload API is not enabled for your account ([see below](workload.md#every-command-answers-403)). |
| `404`  | The artifact, build, or version does not exist.                                                                   |
| `409`  | Tried to delete a locked artifact, or to delete one still referenced by a workload; the detail names the blocking workloads. |
| `422`  | The spec failed server validation; the response names the offending JSON path.                                    |

A `403` on the very first request, including a plain `dr artifact list`, is about access rather than about the artifact. `dr artifact` and `dr workload` sit behind the same platform entitlement: see [Every command answers `403`](workload.md#every-command-answers-403).

## See also

- [`dr workload`](workload.md): deploy a locked artifact as a running workload.
- [Spec reference](workload-spec.md): every field an artifact and workload spec accepts, and one end-to-end walkthrough.
- [Authentication](auth.md): how `dr auth login` and `--skip-auth` interact.
- [Configuration](../user-guide/configuration.md): config file and environment-variable precedence.
