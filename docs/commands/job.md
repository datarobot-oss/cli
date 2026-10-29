# `dr job` - Run-to-completion jobs

Run code that starts, does its work, and ends, such as model training or batch scoring. A job stores what its runs execute and where; each run executes it once and reports how it ended. The `dr job` group wraps the Workload API's `/jobs` routes, with one subcommand per operation. It is also available under the alias `jobs`.

> [!IMPORTANT]
> The `job` command is behind a feature gate. Export `DATAROBOT_CLI_FEATURE_JOB=true` before running any `dr job` subcommand. The cluster also needs the Workload API's jobs switch (`WORKLOAD_JOBS_ENABLED`). See [Feature gates](../development/feature-gates.md) for details.

## Synopsis

The group takes a subcommand and its flags.

```bash
dr job <command> [flags]
```

## Description

A **job** holds two things: the job artifact its runs execute and the runtime they start with (the resource bundle and, through a Use Case, Enclave placement). A job has no status of its own. Job names are not unique, so look a job up by its ID.

A **run** is one execution of a job. Each run starts from what the job stores at that moment, so changing the job with `dr job update` affects later runs only. A run moves through these statuses:

```text
submitted → provisioning → launching → running → succeeded | failed | errored
                                   any of these → stopping → stopped
```

- **succeeded** — the code exited with code `0`.
- **failed** — the code exited with a non-zero code. The run keeps the exit code and the reason, for example `Error` or `OOMKilled`.
- **errored** — the platform could not run the code, for example because the image could not be pulled.
- **stopped** — the run was cancelled before its code ended.

A run is never restarted. Its record and its logs remain after it ends; its pod is removed a short time later.

## Quick start

Build a job artifact, create a job from it, and run it.

```bash
export DATAROBOT_CLI_FEATURE_JOB=true

# Create a job artifact. The spec sets type: job.
dr artifact create --spec-file artifact.yaml

# Create the job. This does not start a run.
dr job create --name train --artifact-id <artifact-id> --runtime-file runtime.yaml

# Start a run and wait for it to end. The exit status is non-zero unless the run succeeds.
dr job run <job-id> --wait

# Read what the run wrote.
dr job logs <job-id> <run-id>
```

The artifact spec names the image and the command that runs to completion:

```yaml
name: train
type: job
spec:
  containerGroups:
    - containers:
        - imageUri: python:3.12-slim
          entrypoint: ["python", "-c", "print('training', flush=True)"]
```

The runtime file sets the resource bundle:

```yaml
containerGroups:
  - name: default
    resourceBundles: [cpu.small]
```

## Commands

Each subcommand maps to one Workload API operation. Every subcommand accepts `--output-format json` for machine-readable output.

### `create`

Create a job from flags, a spec file, or both. A flag fills a field the spec file leaves out but cannot contradict a field the file sets.

```bash
dr job create --name train --artifact-id <artifact-id>
dr job create --name train --artifact-id <artifact-id> --runtime-file runtime.yaml --use-case-id <use-case-id>
dr job create --spec-file job.yaml
```

| Flag | Description |
|------|-------------|
| `--name` | Job name. Names are not unique. |
| `--description` | Job description. |
| `--artifact-id` | The job artifact every run executes. The artifact must have `type: job`. |
| `--runtime-file` | Path to a JSON or YAML runtime block. |
| `--use-case-id` | Use Case to link the job to. Enclave placement requires it. |
| `--spec-file` | Path to a JSON or YAML `POST /jobs` body. Fields the CLI does not know are sent as given. |

A run is never scaled, so the server refuses `replicaCount`, `autoscaling`, and scale-to-zero settings in the runtime. To place runs on an Enclave, link a Use Case and set `enclaveSelectionPolicy` in the runtime: `availability` picks one of the Use Case's Enclaves, and `manual` with one name in `enclaves` pins that Enclave.

### `list`

List the jobs visible to the caller, newest first.

```bash
dr job list
dr job list --name train
```

`--name` is an exact, case-sensitive match and can return more than one job. `--limit` and `--offset` page through the results.

### `get`

Display one job: its name, artifact, and timestamps.

```bash
dr job get <job-id>
```

### `update`

Change what later runs use. Runs already started keep what they started with.

```bash
dr job update <job-id> --artifact-id <artifact-id>
dr job update <job-id> --runtime-file runtime.yaml
```

`--runtime-file` replaces the stored runtime as a whole. When the file leaves out `enclaveSelectionPolicy` and `enclaves`, the job keeps its Enclave placement.

### `delete`

Delete a job after stopping its active runs. The artifact is not deleted.

```bash
dr job delete <job-id>
dr job delete <job-id> --yes
```

The command asks for confirmation first; `--yes` skips it.

### `run`

Start a run of a job with the artifact and runtime the job stores.

```bash
dr job run <job-id>
dr job run <job-id> --wait
RUN_ID=$(dr job run <job-id> --output-format json | jq -r .id)
```

Without `--wait`, the command returns once the server accepts the run. With `--wait`, it follows the run until the run ends, prints each status change to stderr, and prints the run with its exit code and reason to stdout. A run that does not succeed makes the command exit non-zero, so a CI/CD step can depend on it.

| Flag | Default | Description |
|------|---------|-------------|
| `--wait` | `false` | Follow the run until it ends. |
| `--poll-interval` | `5s` | Time between status checks. |
| `--poll-timeout` | `2h` | Maximum time to wait. Reaching it stops the wait, not the run. |

The server limits how many runs a user can have active at once and refuses a run beyond that limit.

### `runs`

List a job's runs, newest first, or display one run.

```bash
dr job runs <job-id>
dr job runs <job-id> --status failed,errored
dr job runs <job-id> <run-id>
```

`--status` keeps runs in the given statuses; repeat it or separate the values with commas. `--limit` and `--offset` page through the list. These flags apply to the list only and are refused when a run ID is given.

### `cancel`

Cancel a run that has not ended.

```bash
dr job cancel <job-id> <run-id>
dr job cancel <job-id> <run-id> --yes
```

The run moves to `stopping`, and its container receives `SIGTERM` followed, after a grace period, by `SIGKILL`. The run then ends with the status of how its code exited: `stopped`, or `failed` when the code exited with an error while stopping. A process that ignores `SIGTERM` and finishes inside the grace period ends as `succeeded`. Cancelling a run that already ended returns an HTTP `409 Conflict` status code.

The command asks for confirmation first; `--yes` skips it.

### `logs`

Show what one run wrote to stdout and stderr.

```bash
dr job logs <job-id> <run-id>
dr job logs <job-id> <run-id> --follow
dr job logs <job-id> <run-id> --level error --grep timeout
```

The most recent `--limit` lines print oldest first. Log lines reach the log store a few seconds after the code writes them, so the last lines of a run that just ended can take a moment to appear. An empty result can also mean the cluster does not collect logs from the nodes the run used.

| Flag | Description |
|------|-------------|
| `--limit` | Maximum number of recent lines to return. Default `100`. |
| `--level` | Minimum log level: `debug`, `info`, `warn`, `warning`, `error`, or `critical`. |
| `--grep` | Keep lines containing the text, case-insensitive. Repeat to require every term. |
| `--exclude` | Drop lines containing the text, case-insensitive. Repeatable. |
| `--since` | Start time: RFC 3339, a date, or a duration back from the present (`15m`, `2h`, `1d`). |
| `--follow`, `-f` | Stream new lines as they arrive until Ctrl-C. |
| `--poll-interval` | Time between polls with `--follow`. Default `2s`. |

## See also

These pages cover the pieces a job builds on.

- [`dr artifact`](artifact.md) — build the job artifact a job runs.
- [`dr workload`](workload.md) — deploy long-running services.
- [Feature gates](../development/feature-gates.md) — enable `DATAROBOT_CLI_FEATURE_JOB`.
