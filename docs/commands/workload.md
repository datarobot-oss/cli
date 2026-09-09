# `dr workload` - Workload management

Deploy and operate workloads on your DataRobot infrastructure. A workload is a running deployment created from an artifact; once it is running it serves traffic on a stable endpoint URL. The `dr workload` group is a thin wrapper over the Workload API, with one subcommand per operation. It is also available under the alias `wl`.

## Synopsis

```bash
dr workload <command> [flags]
dr wl <command> [flags]
```

## Description

A **workload** runs the containers defined by an [artifact](artifact.md). You create one from a spec that either references an existing `artifactId` or defines a draft `artifact` inline.

Startup is asynchronous. A workload moves through `submitted → provisioning → launching → running`; other states include `suspended`, `interrupted`, `stopping`, `stopped`, `errored`, and `terminated`. After `create`, poll `dr workload status <id>` (or `dr workload get`) until the status is `running`, then call its endpoint.

`start` and `stop` are asynchronous and idempotent too: stopping keeps the workload so it can be started again later, and the artifact it was created from is never removed along with it.

> [!NOTE]
> The `workload` command is currently behind a feature gate. Enable it by exporting `DATAROBOT_CLI_FEATURE_WORKLOAD=true` before running any `dr workload` subcommand. See [Feature gates](../development/feature-gates.md) for details.
>
> **First time?** If you're new to the CLI, start with the [Quick start](../../README.md#quick-start) for step-by-step setup instructions.

## Quick start

```bash
# Deploy a workload from a spec file
dr workload create --spec-file workload.yaml

# Watch it come up
dr workload status <workload-id>

# Once running, grab its endpoint and call it
curl "$(dr workload endpoint <workload-id>)health"

# Tail the logs
dr workload logs <workload-id> --follow
```

## Command groups

| Command                | Endpoint                                  | Purpose                                        |
| ---------------------- | ----------------------------------------- | ---------------------------------------------- |
| `dr workload create`   | `POST   /api/v2/workloads/`               | Deploy a workload from a spec.                 |
| `dr workload get`      | `GET    /api/v2/workloads/{id}/`          | Show a single workload.                        |
| `dr workload list`     | `GET    /api/v2/workloads/`               | List workloads, optionally filtered by status. |
| `dr workload delete`   | `DELETE /api/v2/workloads/{id}/`          | Delete a workload.                             |
| `dr workload start`    | `POST   /api/v2/workloads/{id}/start`     | Start a stopped workload.                      |
| `dr workload stop`     | `POST   /api/v2/workloads/{id}/stop`      | Stop a running workload.                       |
| `dr workload status`   | `GET    /api/v2/workloads/{id}/`          | Print the bare status value.                   |
| `dr workload endpoint` | `GET    /api/v2/workloads/{id}/`          | Print the endpoint URL.                        |
| `dr workload logs`     | `GET    /api/v2/otel/workload/{id}/logs/` | Show a workload's container logs.              |

## Subcommands

### `create`

Deploy a workload from a JSON or YAML spec file. The spec needs a `name` and exactly one of `artifactId` (an existing artifact) or an inline `artifact` object. JSON is sent to the server byte-for-byte; YAML is converted to JSON first. Startup is asynchronous, and the response includes the stable endpoint URL.

```bash
dr workload create --spec-file <path> [--output-format text|json]
```

**Flags:**

- `--spec-file <path>`: path to the JSON or YAML spec (required).
- `--output-format <text|json>`: output format. Defaults to `text`.

**Example (fixed replica count):**

```yaml
# workload.yaml - deploy an existing artifact
name: my-app
artifactId: 68b0c1d2e3f4a5b6c7d8e9f0
runtime:
  containerGroups:
    - name: default
      replicaCount: 1
      containers:
        - name: primary
          resourceAllocation:
            cpu: 1
            memory: 512MB
```

**Example (autoscaling):**

Replica bounds live on `autoscaling` (`minReplicaCount` / `maxReplicaCount`). Each policy only needs `scalingMetric` and `target`. A full copy-paste spec is in [workload-autoscaling.yaml](../examples/workload-autoscaling.yaml).

```yaml
name: my-app
artifactId: 68b0c1d2e3f4a5b6c7d8e9f0
runtime:
  containerGroups:
    - name: default
      autoscaling:
        enabled: true
        minReplicaCount: 1
        maxReplicaCount: 10
        policies:
          - scalingMetric: cpuAverageUtilization
            target: 80
      containers:
        - name: primary
          resourceAllocation:
            cpu: 1
            memory: 512MB
```

Use **either** `replicaCount` (fixed scale) **or** `autoscaling.enabled: true` (dynamic scale) per container group — not both. Omit `replicaCount` when autoscaling is enabled.

> [!NOTE]
> The Workload API still accepts the legacy per-policy `minCount` / `maxCount` fields on input and hoists them automatically, but responses always use `minReplicaCount` / `maxReplicaCount` on `autoscaling`. Prefer the new shape in new specs.

```bash
dr workload create --spec-file workload.yaml
```

To define the artifact in the same call instead of referencing one, replace `artifactId` with an inline `artifact:` object; the draft artifact is created and deployed together.

### `get`

Show a single workload: name, status, endpoint, artifact, and timestamps.

```bash
dr workload get [<workload-id>] [--dir <path>] [--output-format text|json]
```

### `list`

List workloads, optionally filtered by status.

```bash
dr workload list [--status <status>] [--limit N] [--output-format text|json]
```

**Flags:**

- `--status <status>`: filter by status. Repeatable, and also accepts comma-separated values (for example `--status running --status errored`).
- `--limit <N>`: maximum number to return. Defaults to `100`.
- `--output-format <text|json>`: output format. Defaults to `text`.

### `delete`

Delete a workload by id. A running workload is stopped first and then removed. The artifact it was created from is not deleted. You are asked to confirm unless `--yes` is set.

```bash
dr workload delete [<workload-id>] [--dir <path>] [--yes]
```

**Flags:**

- `--yes`, `-y`: skip the confirmation prompt. `DATAROBOT_CLI_NON_INTERACTIVE=1` stands in for it only when you passed the workload id; a workload whose id is specified in the manifest takes the explicit flag.
- `--dir <path>`: project directory whose `.datarobot.yaml` names the workload, and holds the binding to clear, searched upward from there. Defaults to the current directory. Pass the same value you deployed with, since a manifest in a subdirectory is not visible from its parent.

If the manifest found from `--dir` is bound to the workload just deleted, the `workloadId` line the CLI wrote is removed with it, so the next `dr workload up` creates a new workload instead of pointing at one that is gone. Only a manifest naming that exact id is touched. The artifact link under `.datarobot/` is left alone, because the artifact itself survives the deletion. The command names the artifact so the link is not left invisible, and names the state directory to remove if you want to unlink from it. These notes go to stderr, so stdout stays the command's result.

A manifest this edit cannot make sound again is refused rather than rewritten, with the reason and the remedy: a binding whose value something else aliases, repeated `workloadId` keys that disagree, a file whose only recognized key is the binding, and a file carrying more than one YAML document.

A workload that was already gone before the command ran is reported, and the binding is left alone: a 404 says the workload is not on this instance, which is not the same as saying it no longer exists. That case, and deletion from the UI or by a teammate, is handled at deploy time instead: `dr workload up` treats a binding that resolves to nothing as drift, creates a new workload, and names the id it could not find in both the plan and the JSON envelope.

### `start` / `stop`

Start a stopped workload, or stop a running one. Both are asynchronous: the server acknowledges the request and the workload transitions in the background. Each is a no-op if the workload is already in the target state.

```bash
dr workload start [<workload-id>] [--dir <path>] [--yes] [--output-format text|json]
dr workload stop  [<workload-id>] [--dir <path>] [--yes] [--output-format text|json]
```

A workload whose id is specified in the manifest rather than on the command line is confirmed first; `--yes` (or `DATAROBOT_CLI_NON_INTERACTIVE=1`) skips the question. A typed id is never questioned.

### `status`

Print a workload's current status as a bare value (for example `running`), so it drops straight into scripts. An `errored` status is a valid answer, so the command still exits `0`. Use `dr workload get` for the full document.

```bash
dr workload status [<workload-id>] [--dir <path>] [--output-format text|json]
```

### `endpoint`

Print only the workload's endpoint URL and nothing else, so it composes directly in scripts. The URL ends with a trailing slash, so append sub-paths without a leading slash of their own:

```bash
curl "$(dr workload endpoint <workload-id>)health"
```

The command fails when the workload has no endpoint URL yet.

```bash
dr workload endpoint [<workload-id>] [--dir <path>]
```

### `logs`

Show the application logs from a workload's containers. By default it prints the most recent `--limit` lines oldest-first, like `kubectl logs --tail`. Use `--level` to drop everything below a severity, and `--follow` (`-f`) to keep streaming new lines as they arrive (Ctrl-C to stop).

```bash
dr workload logs [<workload-id>] [--dir <path>] [--limit N] [--level <level>] [--follow] [--output-format text|json]
```

**Flags:**

- `--limit <N>`: number of recent lines to fetch. Defaults to `100`.
- `--level <level>`: minimum level to show (`debug`, `info`, `warn`, `warning`, `error`, `critical`). Empty keeps every line.
- `--follow`, `-f`: stream new lines as they arrive.
- `--output-format <text|json>`: output format. Defaults to `text`. With `--follow`, JSON is emitted as one object per line (JSON Lines).

## Working in a project directory

Every command that takes a `<workload-id>` can leave it out inside a project that `dr workload up` has deployed. The id is read from the `workloadId` in the nearest `.datarobot.yaml`, searched upward from `--dir` (the current directory by default), and the command says on stderr which workload it picked:

```bash
cd my-app
dr workload status          # instead of dr workload status 68b0c1d2e3f4a5b6c7d8e9f0
dr workload logs --follow
```

A typed id always wins over the manifest. `--dir` is what reaches a project the current directory cannot see, because the search only walks upward:

```bash
dr workload logs --dir site   # the project deployed with 'dr workload up --dir site'
```

Because `stop`, `start` and `delete` change something, they ask for confirmation when the id is specified in a manifest rather than on the command line. Pass `--yes` to skip the prompt in a script; a typed id is never prompted.

`stop` and `start` also accept `DATAROBOT_CLI_NON_INTERACTIVE=1` in place of `--yes`. `delete` does not, when the id is specified in a manifest: that variable is usually set once across a whole CI pipeline, and deleting something nobody named is not what it was set for. Pass `--yes` explicitly there.

## Shared flags

### `--output-format`

Every subcommand accepts `--output-format json` for machine-parseable output. The default, `text`, is human-readable. `status` and `endpoint` print a single bare value by default so they slot straight into scripts.

### `--yes`

`stop`, `start` and `delete` prompt for confirmation when the id is specified in the manifest rather than on the command line; `delete` prompts either way. `--yes` / `-y` answers the question in advance. `DATAROBOT_CLI_NON_INTERACTIVE=1` stands in for the flag on `stop` and `start`, and on a `delete` you passed the id to, but not on a `delete` whose id came from the manifest. With no terminal to ask on, the command fails with `confirmation required: pass --yes` rather than assuming an answer, so a script that needs to run unattended passes the flag.

### Global options

All [global flags](README.md#global-flags) are available, notably `--debug` for protocol-level tracing.

## Examples

### Deploy, watch, and call a workload

```bash
dr workload create --spec-file workload.yaml   # prints the new workload id
dr workload status <workload-id>               # repeat until "running"
curl "$(dr workload endpoint <workload-id>)health"
```

### Operate a workload

```bash
dr workload list --status running
dr workload logs   <workload-id> --follow
dr workload stop   <workload-id>
dr workload start  <workload-id>
dr workload delete <workload-id>
```

## Error handling

| Status | Cause                                                                                                       |
| ------ | ----------------------------------------------------------------------------------------------------------- |
| `403`  | Starting the workload would exceed your concurrent workload limits.                                         |
| `404`  | The workload does not exist.                                                                                |
| `409`  | The workload must finish its current transition first (for example a `start` while it is still `stopping`). |
| `422`  | The spec failed server validation; the response names the offending JSON path (for example `maxReplicaCount` below 1). |

## See also

- [`dr artifact`](artifact.md): build and lock the artifact a workload runs.
- [Authentication](auth.md): how `dr auth login` and `--skip-auth` interact.
- [Configuration](../user-guide/configuration.md): config file and environment-variable precedence.
- [Feature gates](../development/feature-gates.md): turning `DATAROBOT_CLI_FEATURE_WORKLOAD` on and off.
