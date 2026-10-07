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
| `dr workload settings` | `GET/PATCH /api/v2/workloads/{id}/settings/` | Show or change replicas, autoscaling and resources. |
| `dr workload status`   | `GET    /api/v2/workloads/{id}/`          | Print the bare status value.                   |
| `dr workload diagnose` | `GET    /api/v2/workloads/{id}/protons/…` | Explain why a workload is in its state.        |
| `dr workload endpoint` | `GET    /api/v2/workloads/{id}/`          | Print the endpoint URL.                        |
| `dr workload logs`     | `GET    /api/v2/otel/workload/{id}/logs/` | Show a workload's container logs.              |
| `dr workload events`   | `GET    /api/v2/workloads/{id}/events/`   | Show a workload's lifecycle events.            |

## Subcommands

### `create`

Deploy a workload from a JSON or YAML spec file. The spec needs a `name` and exactly one of `artifactId` (an existing artifact) or an inline `artifact` object. JSON is sent to the server byte-for-byte; YAML is converted to JSON first. Startup is asynchronous, and the response includes the stable endpoint URL. Every field the spec accepts is documented in the [spec reference](workload-spec.md#workload-spec), which also walks the whole path from source code to a URL that answers.

```bash
dr workload create --spec-file <path> [--use-case-id <id>] [--enclave <name>] [--output-format text|json]
```

**Flags:**

- `--spec-file <path>`: path to the JSON or YAML spec (required).
- `--use-case-id <id>`: link the workload to a Use Case at create time, the same as a top-level `useCaseId` in the spec. When the Use Case has no Enclaves this is an organizational link and places nothing: the workload runs outside any Enclave, like any other asset in the Use Case. When the Use Case has Enclaves, the flag alone is refused with `ENCLAVE_TARGETING_REQUIRED`; a selection policy or `--enclave` is needed too. The id must be a 24-character hex id, checked locally. See [Use Case and Enclave placement](#use-case-and-enclave-placement).
- `--enclave <name>`: pin the workload to a named Enclave. It sets `runtime.enclaveSelectionPolicy` to `manual` and `runtime.enclaves` to that Enclave, which must be granted to the workload's Use Case, and pinning needs the `CAN_OVERRIDE_WORKLOAD_PLACEMENT` permission on workloads. It requires a Use Case, from `--use-case-id` or `useCaseId` in the spec. Confirm where a workload landed with `dr workload list --enclave <name>`.
- `--output-format <text|json>`: output format. Defaults to `text`.

Both placement flags refuse to override a spec that already sets the field they write (`useCaseId`, or `runtime.enclaveSelectionPolicy` and `runtime.enclaves`), and using either means the spec is re-encoded rather than sent byte-for-byte.

#### Use Case and Enclave placement

Enclave placement is opt-in per workload, and it is governed by a Use Case: an administrator grants Enclaves to a Use Case, and a workload linked to that Use Case can be placed on them. Three shapes cover it:

| You want | Spec or flags |
|---|---|
| A workload linked to a Use Case that has no Enclaves | `--use-case-id <id>` alone, or `useCaseId` in the spec. If the Use Case has Enclaves, this is refused with `ENCLAVE_TARGETING_REQUIRED`; use one of the rows below |
| DataRobot to pick among the Use Case's Enclaves | `useCaseId` plus `runtime.enclaveSelectionPolicy: "availability"` in the spec |
| One specific Enclave | `--use-case-id <id> --enclave <name>`, which sets the policy to `manual` |

```bash
dr workload create --spec-file workload.yaml --use-case-id 68b0aa11bb22cc33dd44ee55
dr workload create --spec-file workload.yaml --use-case-id 68b0aa11bb22cc33dd44ee55 --enclave prod-east
```

Placement rejections are typed on the server, and the CLI prints the server's sentence with the machine code in parentheses rather than a raw JSON dump:

- `MISSING_USE_CASE`: the spec asks for Enclave placement but names no Use Case.
- `ENCLAVE_TARGETING_REQUIRED`: the Use Case has Enclaves and the spec sets no selection policy. Set `runtime.enclaveSelectionPolicy` to `"availability"`, or pin one with `--enclave`.

> [!IMPORTANT]
> **Breaking change in v0.10.0.** `--enclave` on its own is refused locally with `--enclave requires --use-case-id (or useCaseId in the spec)`, where earlier releases sent the request and let the server reject it. A script that pinned an Enclave without naming a Use Case needs `--use-case-id` added.

The spec fields themselves, `useCaseId` and `runtime.enclaveSelectionPolicy`, are documented in the [spec reference](workload-spec.md#placement).

#### Examples

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

Delete a workload by id. A running workload is stopped first and then removed. The artifact it was created from is not deleted unless you pass `--purge`. You are asked to confirm unless `--yes` is set.

```bash
dr workload delete [<workload-id>] [--dir <path>] [--purge] [--yes]
```

**Flags:**

- `--yes`, `-y`: skip the confirmation prompt. `DATAROBOT_CLI_NON_INTERACTIVE=1` stands in for it only when you passed the workload id; a workload whose id is specified in the manifest takes the explicit flag.
- `--dir <path>`: project directory whose `.datarobot.yaml` names the workload, and holds the binding to clear, searched upward from there. Defaults to the current directory. Pass the same value you deployed with, since a manifest in a subdirectory is not visible from its parent.
- `--purge`: also remove what the deploy created beside the workload, so the next `up` starts from scratch: the credentials this project minted (named `<workload>/<VARIABLE>` and referenced from the manifest), the artifact the workload ran when it is a draft no other workload references, and the `.datarobot/workload/` state directory. A locked artifact cannot be deleted and is named, and whenever the artifact survives the credentials stay with it, since whatever runs it may read them. A credential referenced by id but not minted by this project may be shared, so it is left and named too. The one case the name cannot tell apart is a second project set up under the same workload name and pointed at the same credential: it carries the minted name, so a purge of either project removes it. The manifest keeps its environment variables; each reference to a credential the purge removed is reset to the placeholder, which the next `--sync-env` fills with a fresh credential. The purge set is read while the workload still exists, so a binding the command cannot clear afterwards does not stop it. The confirmation names everything a purge removes; `--yes` alone never widens what a delete does. Without a manifest naming the workload, only the workload is deleted and the command says so.

If the manifest found from `--dir` is bound to the workload just deleted, the `workloadId` line the CLI wrote is removed with it, so the next deploy from this project creates a new workload instead of pointing at one that is gone. Only a manifest naming that exact id is touched. Without `--purge`, the artifact link under `.datarobot/` is left alone, because the artifact itself survives the deletion. The command names the artifact so the link is not left invisible, and names the state directory to remove if you want to unlink from it. These notes go to stderr, so stdout stays the command's result.

A manifest this edit cannot make sound again is refused rather than rewritten, with the reason and the remedy: a binding whose value something else aliases, repeated `workloadId` keys that disagree, a file whose only recognized key is the binding, and a file carrying more than one YAML document.

A workload that was already gone before the command ran is reported, and the binding is left alone: a 404 says the workload is not on this instance, which is not the same as saying it no longer exists. That case, and deletion from the UI or by a teammate, is handled at deploy time instead: a deploy treats a binding that resolves to nothing as drift, creates a new workload, and names the id it could not find in both the plan and the JSON envelope.

### `start` / `stop`

Start a stopped workload, or stop a running one. Both are asynchronous: the server acknowledges the request and the workload transitions in the background. Each is a no-op if the workload is already in the target state.

```bash
dr workload start [<workload-id>] [--dir <path>] [--yes] [--output-format text|json]
dr workload stop  [<workload-id>] [--dir <path>] [--yes] [--output-format text|json]
```

A workload whose id is specified in the manifest rather than on the command line is confirmed first; `--yes` (or `DATAROBOT_CLI_NON_INTERACTIVE=1`) skips the question. A typed id is never questioned.

### `settings`

Show or change how much a workload runs with: replicas, autoscaling, resource bundles, and the CPU and memory of each container. This is the same runtime that `dr workload up` reconciles from `.datarobot.yaml`, so it is the place to look at, or resize, a workload that no manifest describes.

```bash
dr workload settings [<workload-id>] [--dir <path>] [--output-format text|json]
dr workload settings [<workload-id>] --replicas <N> [--group <name>] [--yes] [--wait]
dr workload settings [<workload-id>] --spec-file <path> [--yes] [--wait]
```

With no change flags the current settings are printed, one row per container, and the replacement in flight when a change is still being rolled out:

```
╭─────────┬──────────┬──────────────────────────────────┬────────────┬───────────┬─────┬────────────┬─────╮
│ GROUP   │ REPLICAS │ AUTOSCALING                      │ BUNDLE     │ CONTAINER │ CPU │ MEMORY     │ GPU │
├─────────┼──────────┼──────────────────────────────────┼────────────┼───────────┼─────┼────────────┼─────┤
│ default │ auto     │ 0-3 on httpRequestsConcurrency=2 │ cpu.xlarge │ primary   │ 1   │ 2147483648 │ -   │
╰─────────┴──────────┴──────────────────────────────────┴────────────┴───────────┴─────┴────────────┴─────╯
```

Memory is spelled the way a manifest spells it: the largest 1000-based unit that divides it exactly, or the bare byte count when none does (2 GiB above), since a binary size is never rounded down to a decimal one.

**Flags:**

- `--replicas <N>`: scale one container group to `N` replicas. A workload with one group needs no `--group`; with several, name it. A group that autoscales is refused, because its count belongs to the autoscaler: change the policy with `--spec-file` instead.
- `--spec-file <path>`: apply a whole settings body, JSON or YAML: the document `--output-format json` prints, `{"runtime": ...}`, or the runtime block itself, which is what a manifest carries under `runtime`. Save the JSON, edit it, send it back. The body replaces the whole runtime, so every group needs its name, its containers, and either `resourceBundles` or a `resourceAllocation` on each container. A body missing those is refused before it is sent: the platform accepts it and then fails the rollout, which without `--wait` nobody sees.
- `--yes`, `-y`: skip the confirmation. The rolling restart is still announced on stderr. `DATAROBOT_CLI_NON_INTERACTIVE=1` also skips it.
- `--wait`: follow the replacement to its end, wait for the workload to be running on the new settings, read them back and print them. A replica change is `applied` when the count read back is the one asked for, and an error otherwise. A settings body is `applied` when the replacement ended `completed`, and `unconfirmed` when the platform gave no final status for it, in which case the printed settings are what the workload runs on now and worth comparing with what was sent. A replacement that ends `failed` leaves the workload on the settings it had, and the command says so and exits non-zero. Without a change there is nothing to wait for, and the flag is refused.
- `--poll-timeout <duration>`: how long `--wait` may take, 30 minutes by default. Giving up ends the wait, not the rollout.
- `--output-format <text|json>`: output format. Defaults to `text`. JSON is one `{"settings": …}` document with the runtime in the platform's own field names, the replacement, and for a change the status `requested`, `applied` or `unconfirmed`.

A change is a rolling replacement: the platform brings up containers with the new settings and retires the old ones, and the endpoint keeps answering throughout. A change is refused while another replacement is in flight. Applied to a stopped workload, the platform starts it. Without `--wait` the command returns once the change is accepted and names the replacement; follow it with `dr workload settings <id>`, which prints the replacement in flight. `dr workload status` stays `running` throughout.

### `status`

Print a workload's current status as a bare value (for example `running`), so it drops straight into scripts. An `errored` status is a valid answer, so the command still exits `0`. Use `dr workload get` for the full document.

```bash
dr workload status [<workload-id>] [--dir <path>] [--output-format text|json]
```

### `diagnose`

Explain why a workload is in its current state. `status` says `errored` and stops there, and `logs` can be empty for a container that never started; this command reads the platform's per-replica status details for every container generation of the workload and prints the overall verdict, then a table of each replica and container with its state, reason, restart count, readiness and how its last run ended. Anything that stands out is listed as a finding: `CrashLoopBackOff`, `ImagePullBackOff`, `ErrImagePull`, `OOMKilled`, a non-zero exit code, restarts. The generation answering the endpoint comes first; during a rolling replacement the one on its way out follows it.

```bash
dr workload diagnose [<workload-id>] [--dir <path>] [--output-format text|json]
```

```
Workload 6abd2a3ec4b5e3476a311778 (review-crashloop) is errored

Generation 6abd2a3ec4b5e3476a311779 · errored · active  artifact 6abd2a3ec4b5e3476a311777
  Workload is in state errored based on pod states
  ╭─────────────────────────────┬───────────┬─────────┬──────────────────┬──────────┬───────┬────────────────────╮
  │ REPLICA                     │ CONTAINER │ STATE   │ REASON           │ RESTARTS │ READY │ LAST EXIT          │
  ├─────────────────────────────┼───────────┼─────────┼──────────────────┼──────────┼───────┼────────────────────┤
  │ 6945b6ddd7-6mhh8 (running)  │ primary   │ waiting │ CrashLoopBackOff │ 3        │ no    │ exit 0 (Completed) │
  ╰─────────────────────────────┴───────────┴─────────┴──────────────────┴──────────┴───────┴────────────────────╯
  Findings:
    ⚠ primary: CrashLoopBackOff; last run exited 0 (Completed)
```

An `errored` workload is the answer, not a command failure, so the command exits `0`, like `status`. A generation the platform's monitor has not reported for yet is said to have no snapshot. With `--output-format json`, stdout is one `{"diagnosis": …}` document carrying the platform's own field names, with `findings` as a list on every generation, empty when nothing stands out.

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

Show the application logs from a workload's containers. By default it prints the most recent `--limit` lines oldest-first, like `kubectl logs --tail`. Use `--level` to drop everything below a severity, the filters below to narrow further, and `--follow` (`-f`) to keep streaming new lines as they arrive (Ctrl-C to stop).

```bash
dr workload logs [<workload-id>] [--dir <path>] [--limit N] [--level <level>] [--grep <text>]... [--exclude <text>]... [--trace-id <id>] [--span-id <id>] [--since <time>] [--until <time>] [--follow] [--output-format text|json]
```

**Flags:**

- `--limit <N>`: number of recent lines to fetch. Defaults to `100`. It bounds what is fetched, before `--exclude` and any second `--grep` term are applied, so a filtered result can be shorter.
- `--level <level>`: minimum level to show (`debug`, `info`, `warn`, `warning`, `error`, `critical`). Empty keeps every line.
- `--grep <text>`: only lines containing the text, case-insensitive. Repeat it to require every term.
- `--exclude <text>`: drop lines containing the text, case-insensitive. Repeatable.
- `--trace-id <id>`, `--span-id <id>`: only the lines of one trace or span.
- `--since <time>`, `--until <time>`: a time window. A time is RFC 3339 (`2026-06-11T14:04:15Z`), a date (`2026-06-11`), or a duration back from now (`15m`, `2h30m`, `1d`, `1w`). A timestamp as the command prints it can be pasted back as it is; a time without a zone is UTC. A date given to `--until` covers the whole of that day, so `--since 2026-06-11 --until 2026-06-11` is everything from the 11th. With `--follow`, `--since` narrows the first batch and `--until` is refused, since a stream has no end. When a filter leaves nothing, the command says `No logs matched the filters.` rather than `No logs found.`; if the same command without the filter is empty too, the note below about empty output is the reason.
- `--follow`, `-f`: stream new lines as they arrive.
- `--output-format <text|json>`: output format. Defaults to `text`. With `--follow`, JSON is emitted as one object per line (JSON Lines).

```bash
dr workload logs --grep "connection refused" --since 1h
dr workload logs --level error --exclude healthz --follow
```

> [!NOTE]
> **Empty output is not always a bug.** Container stdout is gathered by a platform log collector that is rolled out per cluster. On an installation that does not run it, the logs endpoint answers `200` with an empty list however healthy the workload is, and raising `--limit` changes nothing. A container that never started wrote nothing either. When the result is empty, the command says so on stderr and points at [`dr workload diagnose`](#diagnose), which reads the per-replica status details and names the reason (`ErrImagePull`, `CrashLoopBackOff`) and the exit code of the run that failed; stdout stays log lines only, so a pipe is unaffected.

### `events`

Show the lifecycle events the platform recorded for a workload: what changed, when, and who did it. A settings change that rolled out, a rollout that failed and why, a start or a stop. Where `logs` is what the container said, `events` is what the platform did to the workload.

The most recent `--limit` events are printed oldest-first. The table carries the platform's own message for each event; `--output-format json` gives the complete records, details included.

```bash
dr workload events [<workload-id>] [--dir <path>] [--limit N] [--type <text>]... [--since <time>] [--until <time>] [--proton-id <id>] [--output-format text|json]
```

**Flags:**

- `--limit <N>`: number of recent events to show. Defaults to `100`.
- `--type <text>`: only events whose type contains this text, in any case. Repeatable; any of them matches, so `--type errored --type completed` shows both outcomes of a rollout.
- `--since <time>`, `--until <time>`: bound the window. Each takes an RFC 3339 time (`2026-09-30T18:00:00Z`), a date (`2026-09-30`), or a duration back from now (`15m`, `2h`, `1d`, `1w`). A date or a time without a zone is read as UTC, which is what the table prints, so west of UTC `--until 2026-09-30` ends before the local evening. A date given to `--until` covers the whole of that UTC day, so `--since 2026-09-30 --until 2026-09-30` is everything from the 30th.
- `--proton-id <id>`: only events that name the given generation (proton) in their details, such as the rollout that promoted it. A rollout that failed before launching a generation names none.
- `--output-format <text|json>`: output format. Defaults to `text`. JSON is one `{"events": [...]}` document, with `[]` when nothing matched.

The event route takes no filters of its own, so the trail is read whole and narrowed on your side. A workload's trail is short, a page or two at most, so this costs nothing you would notice.

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

## Deploying from a project: `config` and `up`

> These two are still behind a feature gate. Set `DATAROBOT_CLI_FEATURE_WORKLOAD=true` to see them in `--help`.

`dr workload config` writes the `.datarobot.yaml` that describes your project, and `dr workload up` deploys the difference between that file and what is running. Together they are the deploy loop for a repository, where `create` deploys an artifact you already have.

```bash
dr workload config      # write .datarobot.yaml (a wizard, on a terminal)
dr workload up          # plan, then apply
dr workload up --dry-run  # plan and stop
```

### CI, scripts, and agents

**`dr workload up --yes` asks nothing, including on a project that has never deployed.** With no manifest it answers the setup from the project, writes `.datarobot.yaml`, prints the path and the contents, and deploys. The file is written before anything is deployed, so what was inferred is on disk to read and to commit; nothing is deployed from a guess that leaves no trace.

**Commit the file.** It records the id of the workload this run created, and every later deploy reads it to find that workload. A CI job that runs `up --yes` on a checkout with no manifest and never commits the one it wrote would create a new workload on every run.

The answers come from the project, exactly as `dr workload config --yes` would take them:

- the `Dockerfile` and its `EXPOSE` decide the image source and the port;
- `.env`, when present, decides the variables. A value that looks secret is stored as a credential on the tenant and referenced from the file, a name that looks like a local convenience is left out, and the rest are written into the file in the clear, which the run warns about by name. Nothing is read from `.env` at deploy time; the file is the interface.

```bash
dr workload up --yes                    # a fresh project, in one command
dr workload up --yes --dry-run          # the file it would write, and the plan, without writing
```

`--yes` is the supported way to run `up` unattended. It answers every prompt, including the confirmation for rolling a workload whose **live version is locked**. In a pipeline, `DATAROBOT_CLI_NON_INTERACTIVE=true` set once does the same for this command.

Without a terminal on stdin, `up` asks nothing either. `--output-format json` skips the setup wizard but not the locked-roll confirmation.

`up` waits for the deploy to serve, which can take minutes. `--poll-timeout` bounds each wait the deploy does, including the build and the rollout (30 minutes by default), and `--detach` returns as soon as the deploy is requested. Neither stops the deploy itself: `dr workload status` says where it got to.

A project it cannot read is still refused rather than guessed at. With no `Dockerfile` there is no image source to infer, and the error names the flags that settle it — pass them to `dr workload config`, which is where they live:

```bash
dr workload config --yes --build-mode image --image registry.example.com/app:v1
dr workload up --yes
```

`--dry-run` on a project with no manifest writes nothing. It prints the `.datarobot.yaml` a real run would write, then the plan that run would carry out, so looking before deploying is one command on a fresh project too. A project the setup cannot read is refused the same way with or without `--dry-run`.

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
- [Manifest reference](workload-manifest.md): the `.datarobot.yaml` file `config` writes and `up` deploys from, what `up` manages, and what it does on each run.
- [Authentication](auth.md): how `dr auth login` and `--skip-auth` interact.
- [Configuration](../user-guide/configuration.md): config file and environment-variable precedence.
