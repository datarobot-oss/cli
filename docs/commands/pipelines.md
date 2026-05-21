# `dr pipelines` - Pipelines API management

Manage AI/ML pipelines orchestrated by Covalent through the DataRobot
pipelines service. The `dr pipelines` group is a thin CLI wrapper over
the pipelines REST API: every subcommand maps directly to a single
endpoint.

## Synopsis

```bash
dr pipelines <command> [subcommand] [flags]
```

## Description

A **pipeline** is a versioned bundle of Python source defining a DataRobot pipeline (one or more tasks). Each
top-level `dr pipelines` subcommand operates on one of four resources:

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
> The `pipelines` command is currently behind a feature gate. Enable it
> by exporting `DATAROBOT_CLI_FEATURE_PIPELINE=true` before running any
> `dr pipelines` subcommand. See
> [Feature gates](../development/feature-gates.md) for details.

> [!NOTE]
> **First time?** If you're new to the CLI, start with the
> [Quick start](../../README.md#quick-start) for step-by-step setup
> instructions.

## See also

- [Authentication](auth.md) — how `dr auth login` and `--skip-auth`
  interact.
- [Configuration](../user-guide/configuration.md) — config file and
  environment-variable precedence.
- [Feature gates](../development/feature-gates.md) — flipping
  `DATAROBOT_CLI_FEATURE_PIPELINE` on and off.
