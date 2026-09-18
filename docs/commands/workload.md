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
> **First time?** If you're new to the CLI, start with the [Quick start](https://github.com/datarobot-oss/cli/blob/main/README.md#quick-start) for step-by-step setup instructions.

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

Starting from source code rather than from a spec you already have? The [spec reference](workload-spec.md#deploy-a-service-from-source-end-to-end) walks the whole path, from an empty artifact to a URL that answers.

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

Deploy a workload from a JSON or YAML spec file. The spec needs a `name` and exactly one of `artifactId` (an existing artifact) or an inline `artifact` object. JSON is sent to the server byte-for-byte; YAML is converted to JSON first. Startup is asynchronous, and the response includes the stable endpoint URL. Every field the spec accepts is documented in the [spec reference](workload-spec.md#workload-spec), which also walks the whole path from source code to a URL that answers.

```bash
dr workload create --spec-file <path> [--enclave <name>] [--output-format text|json]
```

**Flags:**

- `--spec-file <path>`: path to the JSON or YAML spec (required).
- `--enclave <name>`: pin the workload to a named Enclave. It sets `runtime.enclaveSelectionPolicy` to `manual` and `runtime.enclaves` to that Enclave, which must be eligible and grant you deploy access. The flag refuses to override a spec that already sets either field, and using it means the spec is re-encoded rather than sent byte-for-byte. Without it, DataRobot picks the placement; confirm where a workload landed with `dr workload list --enclave <name>`.
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

Use **either** `replicaCount` (fixed scale) **or** `autoscaling.enabled: true` (dynamic scale) per container group, not both. Omit `replicaCount` when autoscaling is enabled.

`runtime` addresses the artifact's containers by name, so `containerGroups[].name` must match a group in the artifact and `containers[].name` a container inside it. `importance` (`low`, `moderate`, `high` or `critical`; `low` by default) is the other top-level field worth setting, and `resourceAllocation.memory` takes 1000-based units (`512MB`, `2GB`), never binary ones.

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
dr workload list [--status <status>] [--enclave <name>] [--limit N] [--offset N] [--output-format text|json]
```

**Flags:**

- `--status <status>`: filter by status. Repeatable, and also accepts comma-separated values (for example `--status running --status errored`).
- `--enclave <name>`: only list workloads running on the named Enclave.
- `--limit <N>`: maximum number to return. Defaults to `100`.
- `--offset <N>`: number of workloads to skip before returning results. Defaults to `0`.
- `--output-format <text|json>`: output format. Defaults to `text`.

### `delete`

Delete a workload by id. A running workload is stopped first and then removed. The artifact it was created from is not deleted. You are asked to confirm unless `--yes` is set.

```bash
dr workload delete [<workload-id>] [--dir <path>] [--yes]
```

**Flags:**

- `--yes`, `-y`: skip the confirmation prompt. `DATAROBOT_CLI_NON_INTERACTIVE=1` stands in for it only when you passed the workload id; a workload whose id is specified in the manifest takes the explicit flag.
- `--dir <path>`: project directory whose `.datarobot.yaml` names the workload, and holds the binding to clear, searched upward from there. Defaults to the current directory. Pass the same value you deployed with, since a manifest in a subdirectory is not visible from its parent.

If the manifest found from `--dir` is bound to the workload just deleted, the `workloadId` line the CLI wrote is removed with it, so the next deploy from this project creates a new workload instead of pointing at one that is gone. Only a manifest naming that exact id is touched. The artifact link under `.datarobot/` is left alone, because the artifact itself survives the deletion. The command names the artifact so the link is not left invisible, and names the state directory to remove if you want to unlink from it. These notes go to stderr, so stdout stays the command's result.

A manifest this edit cannot make sound again is refused rather than rewritten, with the reason and the remedy: a binding whose value something else aliases, repeated `workloadId` keys that disagree, a file whose only recognized key is the binding, and a file carrying more than one YAML document.

A workload that was already gone before the command ran is reported, and the binding is left alone: a 404 says the workload is not on this instance, which is not the same as saying it no longer exists. That case, and deletion from the UI or by a teammate, is handled at deploy time instead: a deploy treats a binding that resolves to nothing as drift, creates a new workload, and names the id it could not find in both the plan and the JSON envelope.

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

> [!NOTE]
> **Empty output is not always a bug.** Container stdout is gathered by a platform log collector that is rolled out per cluster. On an installation that does not run it, the logs endpoint answers `200` with an empty list however healthy the workload is, and raising `--limit` changes nothing. When a workload is failing and its logs are empty, the status is in `dr workload get`, and the per-replica detail lives on the platform at `GET /api/v2/workloads/{id}/protons/{proton-id}/statusDetails`, which names the reason (`ErrImagePull`, `CrashLoopBackOff`) and the exit code of the run that failed.

## Working in a project directory

Every command that takes a `<workload-id>` can leave it out inside a project whose `.datarobot.yaml` names a workload. The id is read from the `workloadId` in the nearest `.datarobot.yaml`, searched upward from `--dir` (the current directory by default), and the command says on stderr which workload it picked:

```bash
cd my-app
dr workload status          # instead of dr workload status 68b0c1d2e3f4a5b6c7d8e9f0
dr workload logs --follow
```

A one-line file is enough to get that. Take the id `dr workload create` printed and write it at the root of the project it belongs to:

```yaml
# .datarobot.yaml
workloadId: 68b0c1d2e3f4a5b6c7d8e9f0
```

A typed id always wins over the manifest. `--dir` is what reaches a project the current directory cannot see, because the search only walks upward:

```bash
dr workload logs --dir site   # the project whose .datarobot.yaml lives in site/
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
| `403`  | Starting the workload would exceed your concurrent workload limits, or the Workload API is not enabled for your account (see below). |
| `404`  | The workload does not exist.                                                                                |
| `409`  | The workload must finish its current transition first (for example a `start` while it is still `stopping`). |
| `422`  | The spec failed server validation; the response names the offending JSON path (for example `maxReplicaCount` below 1). |
| `502`  | The installation routes the Workload API but the backing service is switched off. This is an operator setting, not something a flag on your side changes. |

### Every command answers `403`

A `403` on the very first request, including a plain `dr workload list`, is usually about access rather than about the workload you asked for. The Workload API sits behind a platform entitlement, and the CLI's own feature gate is unrelated to it: `DATAROBOT_CLI_FEATURE_WORKLOAD` only decides which commands the binary registers, never what your account may call.

The response body names the check that refused you. The two common ones are a feature flag (a message naming `WORKLOAD_API_CONTAINERS`, or saying the Workload API is disabled in feature flags) and a seat license (a message saying the Agentic, Predictive and Governance seat has not been granted to this user). Both are grants your DataRobot administrator makes; neither can be worked around from the CLI. Entitlements are cached per user on the platform for a few minutes, so a newly granted one can keep failing briefly after it is switched on.

The CLI keeps the response body on writes but not on reads, so a refused `GET` prints only the status. Read the message directly:

```bash
curl -i -H "Authorization: Bearer $DATAROBOT_API_TOKEN" "$DATAROBOT_ENDPOINT/workloads/"
```

`dr auth export` puts both variables in your shell, and `eval "$(dr auth export)"` is the form that keeps the quotes out of the values.

## See also

- [`dr artifact`](artifact.md): build and lock the artifact a workload runs.
- [Spec reference](workload-spec.md): every field an artifact and workload spec accepts, and one end-to-end walkthrough.
- [Authentication](auth.md): how `dr auth login` and `--skip-auth` interact.
- [Configuration](../user-guide/configuration.md): config file and environment-variable precedence.
