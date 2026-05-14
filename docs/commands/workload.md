# `dr workload` - Workload artifacts and code management

Manage DataRobot workload artifacts and the local code that backs them.

> [!IMPORTANT]
> All `dr workload` subcommands are gated behind the `DATAROBOT_CLI_FEATURE_WORKLOAD` feature flag. The group is hidden from `dr --help` until the flag is enabled.
>
> ```bash
> export DATAROBOT_CLI_FEATURE_WORKLOAD=true
> ```
>
> See [Feature gates](../development/feature-gates.md) for the general mechanism.

## Quick start

Enable the feature gate, list existing artifacts, and create a new one from a spec file:

```bash
export DATAROBOT_CLI_FEATURE_WORKLOAD=true
dr workload artifact list
dr workload artifact create --spec-file spec.json
```

Link a project directory to an existing artifact and sync code:

```bash
dr workload code init <artifact-id>
dr workload code sync
```

## Synopsis

```bash
dr workload <command> [flags]
```

## Description

The group is split into two sub-groups:

- **`dr workload artifact`**&mdash;create, list, and inspect workload artifacts on the server.
- **`dr workload code`**&mdash;link a local project directory to an artifact and sync code in both directions.

The `code` commands maintain a `.wapi/` state directory at the project root that tracks which artifact, catalog, and version a directory is bound to. The model is conceptually similar to `.git/`&mdash;local work happens in the project root, while `.wapi/` captures the remote binding and last-synced state used to detect drift on each operation.

## Subcommands

### dr workload artifact create

Create a workload artifact from a JSON spec file.

```bash
dr workload artifact create --spec-file <path> [--output-format text|json]
```

**Required flags:**

- `--spec-file <path>`&mdash;path to a JSON spec file. Required.

**Optional flags:**

- `--output-format <text|json>`&mdash;output format. Defaults to `text`.

**Validation:**

- Client-side: `name` must be non-empty; `spec.containerGroups` must contain at least one entry; each group's `containers` array must contain at least one entry.
- Server-side: the Workload API validates field-level shape and returns `422` with a JSON-path detail on a mismatch.
- Unknown fields are passed through to the server, so any field the server accepts is accepted here. The spec file is not parsed strictly.

**Container lifecycles:**

1. **Prebuilt image**&mdash;set `imageUri` (plus `port` and `primary`) on the entry container.
2. **Build from a provided Dockerfile**&mdash;set `imageBuildConfig.dockerfile.source = "provided"` to build from `./Dockerfile` in your synced code.
3. **Build from a generated Dockerfile**&mdash;set `imageBuildConfig.dockerfile.source = "generated"` together with `executionEnvironmentId`, `executionEnvironmentVersionId`, and `entrypoint` to have the server generate a Dockerfile from a base image.

> [!NOTE]
> `dr workload code sync` fills in `imageBuildConfig.codeRef` automatically after the first upload, so a freshly created artifact typically does not carry a `codeRef` until you sync code into it.

**Minimal spec (prebuilt image):**

```json
{
  "name": "my-agent",
  "spec": {
    "containerGroups": [{
      "containers": [{
        "imageUri": "nginx:latest",
        "port": 8080,
        "primary": true
      }]
    }]
  }
}
```

**Minimal spec (build from a provided Dockerfile):**

```json
{
  "name": "my-agent",
  "spec": {
    "containerGroups": [{
      "containers": [{
        "primary": true,
        "port": 8080,
        "imageBuildConfig": { "dockerfile": { "source": "provided" } }
      }]
    }]
  }
}
```

**Fuller spec (wire to an existing catalog version):**

```json
{
  "name": "my-agent",
  "description": "Optional description shown in the DataRobot UI.",
  "spec": {
    "containerGroups": [{
      "containers": [{
        "primary": true,
        "port": 8080,
        "imageBuildConfig": {
          "dockerfile": { "source": "provided" },
          "codeRef": {
            "datarobot": {
              "catalogId": "67890abcdef1234567890abc",
              "catalogVersionId": "67890abcdef1234567890def"
            }
          }
        }
      }]
    }]
  }
}
```

**Examples:**

Create with default human-readable output:

```bash
dr workload artifact create --spec-file spec.json
```

```text
ID:          67890abcdef1234567890abc
Name:        my-agent
Status:      draft
Catalog ID:  —
Version ID:  —
Created:     2026-05-14 10:00 UTC
Updated:     2026-05-14 10:00 UTC
```

Create with machine-parseable JSON output:

```bash
dr workload artifact create --spec-file spec.json --output-format json
```

```json
{
  "id": "67890abcdef1234567890abc",
  "name": "my-agent",
  "status": "draft",
  "catalogId": "",
  "versionId": "",
  "createdAt": "2026-05-14T10:00:00Z",
  "updatedAt": "2026-05-14T10:00:00Z"
}
```

**Common errors:**

- `invalid spec: required field 'name' is missing or empty`
- `invalid spec: 'spec.containerGroups' must contain at least one entry`
- `invalid spec: 'spec.containerGroups[0].containers' must contain at least one entry`
- `file not found: <path>`

### dr workload artifact list

List workload artifacts with optional filtering.

```bash
dr workload artifact list [--limit <n>] [--status draft|locked] [--output-format text|json]
```

**Flags:**

- `--limit <n>`&mdash;maximum number of artifacts to return. Defaults to `100`. Must be a positive integer.
- `--status <draft|locked>`&mdash;filter by status. Optional; if omitted, all statuses are returned. Validator accepts `draft` and `locked` (case-insensitive); any other value returns `invalid status "...": use draft or locked`.
- `--output-format <text|json>`&mdash;output format. Defaults to `text`.

**Examples:**

```bash
dr workload artifact list
```

Renders as a bordered table:

```text
╭──────────────────────────┬───────────┬────────┬──────────────────────────┬──────────────────────────┬──────────────────────╮
│ ARTIFACT ID              │ NAME      │ STATUS │ CATALOG ID               │ VERSION ID               │ UPDATED              │
├──────────────────────────┼───────────┼────────┼──────────────────────────┼──────────────────────────┼──────────────────────┤
│ 67890abcdef1234567890abc │ my-agent  │ draft  │ —                        │ —                        │ 2026-05-14 10:00 UTC │
│ 67890abcdef1234567890aaa │ chat-app  │ locked │ 67890abcdef1234567890cat │ 67890abcdef1234567890ver │ 2026-05-13 14:32 UTC │
╰──────────────────────────┴───────────┴────────┴──────────────────────────┴──────────────────────────┴──────────────────────╯
```

When no artifacts match, the text output is `No artifacts found.` and the JSON output is `[]`.

Filter by status with JSON output:

```bash
dr workload artifact list --status draft --output-format json
```

```json
[
  {
    "id": "67890abcdef1234567890abc",
    "name": "my-agent",
    "status": "draft",
    "catalogId": "",
    "versionId": "",
    "createdAt": "2026-05-14T10:00:00Z",
    "updatedAt": "2026-05-14T10:00:00Z"
  }
]
```

### dr workload artifact get

Display details for a single workload artifact by ID.

```bash
dr workload artifact get <artifact-id> [--output-format text|json]
```

**Arguments:**

- `<artifact-id>`&mdash;the artifact ID. Required.

**Flags:**

- `--output-format <text|json>`&mdash;output format. Defaults to `text`.

**Examples:**

```bash
dr workload artifact get 67890abcdef1234567890abc
```

```text
ID:          67890abcdef1234567890abc
Name:        my-agent
Status:      draft
Catalog ID:  —
Version ID:  —
Created:     2026-05-14 10:00 UTC
Updated:     2026-05-14 10:00 UTC
```

```bash
dr workload artifact get 67890abcdef1234567890abc --output-format json
```

```json
{
  "id": "67890abcdef1234567890abc",
  "name": "my-agent",
  "status": "draft",
  "catalogId": "",
  "versionId": "",
  "createdAt": "2026-05-14T10:00:00Z",
  "updatedAt": "2026-05-14T10:00:00Z"
}
```

### dr workload code init

Link a project directory to an existing workload artifact. Creates a `.wapi/` state directory at the project root that records which artifact, catalog, and version the directory is bound to. Required before any other `code` command.

```bash
dr workload code init [<artifact-id>] [--dir <path>] [-y|--yes] [--output-format text|json]
```

**Arguments:**

- `<artifact-id>`&mdash;optional. If omitted in interactive mode, you'll be prompted.

**Flags:**

- `--dir <path>`&mdash;project directory. Defaults to the current directory.
- `-y, --yes`&mdash;skip interactive prompts and use defaults. Can also be enabled via `DATAROBOT_CLI_NON_INTERACTIVE=true`.
- `--output-format <text|json>`&mdash;output format. Defaults to `text`.

**Prerequisites:**

- The artifact must already exist. Create it via `dr workload artifact create` or in the DataRobot UI.
- The artifact must be in `draft` status. Locked artifacts are immutable.

**Examples:**

```bash
# Interactive: prompts for the directory
dr workload code init 67890abcdef1234567890abc

# Non-interactive: link the current directory
dr workload code init 67890abcdef1234567890abc --yes

# Link a sibling directory
dr workload code init 67890abcdef1234567890abc --dir ./service
```

**Common errors:**

- `artifact <id> not found`
- `artifact is locked (immutable); cannot init on a registered artifact`
- `init aborted: project already linked`

### dr workload code sync

Push local edits and pull remote changes between this directory and the linked artifact. Computes a three-way diff against the last known state, auto-resolves conflicts (remote wins; the local version is saved as a `*.LOCAL.<timestamp>` copy), and applies the resulting plan in a single versioned step.

```bash
dr workload code sync [--dir <path>] [--dry-run | --diff] [-y|--yes] [--output-format text|json]
```

**Flags:**

- `--dir <path>`&mdash;project directory. Defaults to the current directory.
- `--dry-run`&mdash;show the plan without writing anything. Exits before any remote write.
- `--diff`&mdash;show the plan plus per-file unified diffs. Mutually exclusive with `--dry-run`. Exits before any remote write.
- `-y, --yes`&mdash;auto-confirm the post-plan prompt; also skips the interactive directory prompt.
- `--output-format <text|json>`&mdash;output format. Defaults to `text`.

**Prerequisites:**

- The directory must already be linked via `dr workload code init`. Otherwise the command returns `not linked: run 'dr workload code init <artifact-id>' first`.

**Conflict handling:**

When the same file has diverged on both sides, `sync` auto-resolves by taking the remote version and writing the local copy alongside as `<path>.LOCAL.<timestamp>`. In interactive mode the command pauses on conflicts so you can review the plan and abort; pass `--yes` to skip the prompt and apply unconditionally.

**JSON mode:**

In JSON mode the plan is always emitted as the first document. If neither `--dry-run` nor `--diff` is set and the plan does not require explicit confirmation, the executed result is emitted as a second JSON document.

**Examples:**

```bash
# Preview what would change
dr workload code sync --dry-run

# Preview with per-file diffs
dr workload code sync --diff

# Push and pull; prompt on conflicts
dr workload code sync

# Push and pull non-interactively
dr workload code sync --yes
```

### dr workload code versions

List the catalog version history for the artifact this project directory is linked to.

```bash
dr workload code versions [--dir <path>] [--limit <n>] [--output-format text|json]
```

**Flags:**

- `--dir <path>`&mdash;project directory. Defaults to the current directory.
- `--limit <n>`&mdash;maximum number of versions to return. Defaults to `100`. Must be a positive integer.
- `--output-format <text|json>`&mdash;output format. Defaults to `text`.

**Output:**

The output marks the version that the artifact's `codeRef` currently points to with `*` and reports which version the local `.wapi/` state was last synced to. This makes drift visible at a glance: a `*` next to a version other than the last-synced one means a `sync` would update local files.

**Prerequisites:**

- The directory must be linked via `dr workload code init`. Otherwise: `not linked to an artifact. Run 'dr workload code init <id>' first`.
- At least one sync must have happened so the linked catalog has a version. Otherwise: `no code has been synced yet. Run 'dr workload code sync' first`.

**Examples:**

```bash
dr workload code versions
dr workload code versions --limit 10
dr workload code versions --output-format json
```

### dr workload code checkout

Download a specific catalog version into `.wapi/.checkouts/<version-id>/` for read-only inspection. The working directory and `.wapi/` sync state are never modified.

```bash
dr workload code checkout [<version-id>] [--dir <path>] [--clean] [-y|--yes] [--output-format text|json]
```

**Arguments:**

- `<version-id>`&mdash;full version ID or any unique prefix. If omitted (and `--yes` is not set), you'll be prompted.

**Flags:**

- `--dir <path>`&mdash;project directory. Defaults to the current directory.
- `--clean`&mdash;remove checkout directories instead of downloading. No positional argument removes all checkouts; a positional argument removes only the matching one.
- `-y, --yes`&mdash;skip interactive prompts.
- `--output-format <text|json>`&mdash;output format. Defaults to `text`.

**Examples:**

```bash
# Prompt for a version, then download
dr workload code checkout

# Download a specific version (full ID or any unique prefix)
dr workload code checkout abcdef12

# Download into a different project directory
dr workload code checkout abcdef12 --dir ./service

# Remove all checkouts
dr workload code checkout --clean

# Remove a single checkout
dr workload code checkout abcdef12 --clean
```

## Feature gate

The `dr workload` group is hidden from `dr --help` unless the feature gate is enabled:

```bash
export DATAROBOT_CLI_FEATURE_WORKLOAD=true
dr workload --help
```

See [Feature gates](../development/feature-gates.md) for the general mechanism (gate names map to env vars by uppercasing the name and replacing `-` with `_`).

## Error handling

| Error | Cause and resolution |
|---|---|
| `not authenticated` | Run `dr auth login` first; every workload command requires authentication. |
| `invalid spec: required field 'name' is missing or empty` | The spec file's `name` is missing or empty. Add a non-empty string. |
| `invalid spec: 'spec.containerGroups' must contain at least one entry` | The `spec.containerGroups` array is empty. Add at least one group. |
| `file not found: <path>` | The `--spec-file` path does not exist. Check the path. |
| `invalid status "...": use draft or locked` | The `--status` value passed to `artifact list` is not one of the accepted values. |
| `artifact <id> not found` | The artifact ID does not exist. Check the ID with `dr workload artifact list`. |
| `artifact is locked (immutable); cannot init on a registered artifact` | Locked artifacts cannot be linked or modified. Create a new draft artifact. |
| `not linked: run 'dr workload code init <artifact-id>' first` | The directory has no `.wapi/` state. Run `dr workload code init` first. |
| `init aborted: project already linked` | The directory already has a `.wapi/` state. Use `dr workload code sync` or remove `.wapi/` to re-link. |
| `no code has been synced yet. Run 'dr workload code sync' first` | The linked artifact has no catalog version yet. Run `sync` to upload code. |

## Exit codes

| Code | Meaning |
|------|---------|
| 0    | Success. |
| 1    | Error (validation failed, API error, conflict, not authenticated, etc.). |
| 130  | Interrupted (Ctrl+C). |

## Integration with other commands

### With `auth`

Every `dr workload` command requires authentication:

```bash
dr auth login
DATAROBOT_CLI_FEATURE_WORKLOAD=true dr workload artifact list
```

### Typical flow

```bash
# Create the artifact on the server
dr workload artifact create --spec-file spec.json

# Link a local directory to it (use the printed ID)
dr workload code init <artifact-id>

# Upload code and let sync fill in the codeRef
dr workload code sync

# Inspect the catalog versions that have been pushed
dr workload code versions
```

## See also

- [auth command](auth.md)&mdash;required before any workload command.
- [Feature gates](../development/feature-gates.md)&mdash;how to enable gated commands.
- [Development structure](../development/structure.md)&mdash;CLI architecture.
