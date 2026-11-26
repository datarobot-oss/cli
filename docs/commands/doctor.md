# `dr doctor`: check whether an install accepts your deploys

Check what a DataRobot install says about itself against what a deploy needs, and get one remedy for each problem.

## Synopsis

```bash
dr doctor [flags]
```

## Description

`dr doctor` reads the report that [`dr info`](info.md) prints, with the caller's own credentials, and judges it. Each check is `OK`, `WARN`, `FAIL`, or `SKIP`. A finding that needs action carries one remedy that names who acts: the caller or an install administrator. The command is read-only.

The command is behind a feature gate. Set `DATAROBOT_CLI_FEATURE_PLATFORM_DOCTOR=1` to use it.

Authentication follows the other commands: `DATAROBOT_ENDPOINT` and `DATAROBOT_API_TOKEN` when both are set, otherwise the active profile. The command never starts a login prompt. An install that cannot be read is a `FAIL` of the first check.

### Checks

Check IDs are stable and their order is part of the output.

| ID                         | What it judges                                          |
| -------------------------- | ------------------------------------------------------- |
| `platform.install.read`    | The install answered. A failure skips the later checks. |
| `platform.workload.deploy` | The Workload API accepts the caller.                    |

The `platform.workload.deploy` check reads the `workloadApi` section of the report, which is the live answer. For a 403, the refusal message decides the cause. The `seats` and `entitlements` sections decide only when the message does not name one.

- `OK`: the Workload API answers the caller.
- `WARN`, HTTP 404: the install does not serve the Workload API. A Custom Application still deploys.
- `FAIL`, HTTP 401: the Workload API rejected the credentials. Check them.
- `FAIL`, HTTP 502: the Workload API proxy is switched off, or a gateway in front of it failed. An install administrator checks both.
- `FAIL`, HTTP 403 naming a seat license, or `AGENTIC_PREDICTIVE_GOVERNANCE_BUILDER` set to `false` in `seats`: the caller lacks the builder seat. An install administrator grants it.
- `FAIL`, HTTP 403 naming a feature flag, or `ENABLE_WORKLOAD_API_CONTAINERS` set to `false` in `entitlements`: the feature flag is off for the caller.
- `FAIL`, any other refusal: the details carry the HTTP status and the install's message.
- `WARN`, HTTP 429: the install is throttling the probe. Run the check again.
- `WARN`, no HTTP status: the route could not be read, for example after a timeout. Run the check again, then read the message and check the network path to the install. A message that starts with `internal error` is a failure of `dr`: report it if it repeats.
- `SKIP`: the report has no `workloadApi` section, or `platform.install.read` failed.

A seat that is absent from `seats` is not enforced on that install. The `NON_BUILDER_USER` seat restricts which routes the caller may use, and the Workload API does not read it, so it is never the reported cause.

### Exit status

The command exits `0` when no check fails (warnings are allowed) and `1` otherwise.

## Flags

- `--output-format <text|json>`: `json` prints one object with `title`, `status`, `checks`, and `summary`. It has the shape of the [`dr artifact code doctor`](artifact.md) report without `projectDir` and `artifactId`.

## Examples

```bash
export DATAROBOT_CLI_FEATURE_PLATFORM_DOCTOR=1

dr doctor
dr doctor --output-format json | jq '.checks[] | select(.status == "FAIL")'
```
