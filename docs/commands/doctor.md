# `dr doctor`: check whether an install accepts your deploys

Check what a DataRobot install says about itself against what a deploy needs, and get one remedy for each problem.

## Synopsis

```bash
dr doctor [flags]
```

## Description

`dr doctor` reads the report that [`dr info`](info.md) prints, with the caller's own credentials, and judges it. Each check is `OK`, `WARN`, `FAIL`, or `SKIP`. A finding that needs action carries one remedy that names who acts: the caller or an install administrator. The command is read-only. It writes nothing to the install or to disk.

The command is behind a feature gate while its name and output are settled. Set `DATAROBOT_CLI_FEATURE_PLATFORM_DOCTOR=1` to use it. The gate is named `platform-doctor` and stays the same if the command is renamed.

Authentication follows the other commands: `DATAROBOT_ENDPOINT` and `DATAROBOT_API_TOKEN` when both are set, otherwise the active profile. The command never starts a login prompt. An install that cannot be read is a `FAIL` of the first check.

### Checks

Check IDs are stable and their order is part of the output.

| ID                         | What it judges                                                                                                     |
| -------------------------- | ------------------------------------------------------------------------------------------------------------------ |
| `platform.install.read`    | The install answered. It fails when the install is unreachable or rejects the credentials. Later checks skip then. |
| `platform.workload.deploy` | The Workload API accepts the caller. The outcomes are listed below.                                                |

The `platform.workload.deploy` check reads the `workloadApi` section of the report, which is the live answer, and uses the `seats` and `entitlements` sections only to name the cause of a refusal.

- `OK`: the Workload API answers the caller.
- `WARN`, HTTP 404: the release has no Workload API. A Custom Application still deploys.
- `FAIL`, HTTP 502: the Workload API proxy is switched off. An install administrator switches it on.
- `FAIL`, HTTP 403 with `AGENTIC_PREDICTIVE_GOVERNANCE_BUILDER` set to `false` in `seats`: the caller lacks the builder seat. An install administrator grants it.
- `FAIL`, HTTP 403 with `ENABLE_WORKLOAD_API_CONTAINERS` set to `false` in `entitlements`: the feature flag is off for the caller.
- `FAIL`, any other refusal: the details carry the HTTP status and the install's message.
- `WARN`, no HTTP status: the route could not be read, for example after a timeout. Run the check again.
- `SKIP`: the report has no `workloadApi` section, or `platform.install.read` failed.

A seat that is absent from `seats` is not enforced on that install. The `NON_BUILDER_USER` seat restricts which routes the caller may use, and the Workload API does not read it, so it is never the reported cause.

### Exit status

The command exits `0` when no check fails, warnings included, and `1` when at least one check fails. The report prints in both cases.

## Flags

- `--output-format <text|json>`: `json` prints one object with `title`, `status`, `checks`, and `summary`. It has the shape of the [`dr artifact code doctor`](artifact.md) report without `projectDir` and `artifactId`.

## Examples

```bash
export DATAROBOT_CLI_FEATURE_PLATFORM_DOCTOR=1

dr doctor
dr doctor --output-format json | jq '.checks[] | select(.status == "FAIL")'
```
