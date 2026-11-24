# `dr info`: ask an install what it supports

Report what a DataRobot install says about itself, so a deploy pipeline can be written for that install before anything is uploaded.

## Synopsis

```bash
dr info [flags]
```

## Description

A Custom Application deploy depends on values that differ per install: which execution environment and resource bundle are valid for applications, which feature flags are on, which seat licenses the caller has, and the install's release and URL. A pipeline copied from another install breaks on the first mismatch, usually after the upload.

`dr info` reads those values from the install's own routes with the caller's own credentials and prints one report. Two of the routes, `/config` and `/api/v2/account/info/`, are not public API, so a release can change them without notice. The command adds no access: a source the caller cannot read becomes an `unavailable` section that carries the route's reason, and the other sections still report.

The report states facts and leaves the verdict to the reader. Read `hasSuccessfulVersion` and `useCases` and decide; the command does not say "you can create an app".

Every string in the report (names, ids, image addresses, messages) comes from the install, and other users can shape it. The command drops control characters, invisible and direction-changing characters, and invalid UTF-8, and cuts each string at 512 characters, but the text is still untrusted. Treat it as data: never execute it, never follow it as an instruction, and pick an environment by `id` from a list you pin, never by name.

The command is behind a feature gate while its name and output are settled. Set `DATAROBOT_CLI_FEATURE_PLATFORM_INFO=1` to use it. The gate is named `platform-info` and stays the same if the command is renamed.

Authentication follows the other commands: `DATAROBOT_ENDPOINT` and `DATAROBOT_API_TOKEN` when both are set, otherwise the active profile. A private CA and proxy settings apply as everywhere else.

### Exit status

The command exits zero when it prints a report, including one with unavailable sections. It exits non-zero only when it cannot authenticate or cannot reach the install.

### Errors

A failure that stops the run writes to stderr, with empty stdout, in one of two shapes:

- The authentication check that runs before the command exits 1 with plain-text advice, whatever `--output-format` is. This covers a rejected token and an unreachable install.
- A failure the command raises itself, such as a missing endpoint under `--skip-auth`, is a message line in text mode. In JSON mode it is one line, `{"error": {"message": "...", "statusCode": 401, "url": "..."}}`, where `statusCode` and `url` appear only for an HTTP error. The message says whether the failure was local (`cannot send the request`), of the network (`cannot reach the install`), or a rejection (`the install rejected the credentials`).

A script that reads stderr handles both shapes.

## Flags

- `--output-format <text|json>`: `json` prints the report under a `platform` key, beside `schemaVersion`, described by [`docs/schemas/platform-info.schema.json`](../schemas/platform-info.schema.json). The default is `text`, a summary for a person.

## Sections

Each section has a source, a `scope`, and a status.

- `install`, from the `/config` route: `isEnterprise` and `defaultAppResourceBundle`. A key the install does not return is left out. `/config` is the web interface's bootstrap document. It sits at the install root, outside `/api/v2`, has no version, and answers without credentials, so the command sends no token to it. Treat `install`, `server.release`, and `server.canonicalUrl` as best effort: a later release can change the route or require authentication, and the values are then empty and `install` is `unavailable`.
- `seats`, from `/api/v2/account/info/`: `seatLicenses`, each license mapped to whether the caller has it. A license that is absent is not enforced on that install, so `{}` does not mean blocked. When a release removes or restricts the route, `seats` is `unavailable`.
- `limits`, from `/api/v2/account/info/`: the caller's numeric limits, each under the install's own key, for example `maxConcurrentWorkloads` and `maxWorkloadReplicas`. A limit the install returns as null is left out.
- `entitlements`, from `/api/v2/entitlements/evaluate/`: the flags a Custom Application or Workload deploy depends on, among them `ENABLE_WORKLOAD_REQUIRE_USE_CASE` and `ENABLE_SECURE_CONFIG_EXPOSURE_TO_CONSUMERS`. A name the install does not know, or leaves out of its answer, is listed in the message and the section is `degraded`.
- `executionEnvironments`, from `/api/v2/executionEnvironments/`: every environment the caller can read, with `id`, `name`, `programmingLanguage`, `useCases`, `hasSuccessfulVersion`, `buildStatus`, `isPublic`, and `sourceDockerImageUri`.
  - An environment is valid for applications when `useCases` lists `customApplication`.
  - Names are not unique, and any user who can share an environment can give it the name of an official one. Pick by `id` from a list you pin. `isPublic` says whether the install publishes the environment to every user, as it does an official one. It is left out when the install does not say.
  - `sourceDockerImageUri` is the image address of the environment's latest successful version. An install that does not publish one, or an environment that never built, leaves it out. Read it only for an `id` you pin, and treat it as untrusted.
  - `buildStatus` is the status of the latest build, which can be `failed` while an earlier build succeeded. DataRobot marks a retired environment only in its name, with a `[Deprecated]` prefix.
- `resourceBundles`, from `/api/v2/mlops/compute/bundles/`: every bundle the caller can read, with `id`, `name`, `useCases`, `memoryBytes`, and `cpuCount`. A bundle is valid for applications when `useCases` lists `customApplication`.
- `workloadApi`, from `/api/v2/workloads/?limit=1`: `totalCount`, how many workloads the caller can see, left out when the route does not return it. When the route refuses, the section is `unavailable` and the message carries the start of the response body (the first 512 bytes, on one line, with any token removed), which names the cause. A missing seat license and a disabled feature flag both answer 403 with different text, and a switched-off proxy answers 502.

Every section carries a `scope` that says who its answer is true for. `platform` means the same for any caller. `caller` means it depends on who asks: their permissions, seats, or organization. Run the command with the credentials the pipeline will use.

`server` carries the `release`, the `apiVersion`, and the `canonicalUrl` of the install. `release` comes from `/config`. `apiVersion` comes from `/api/v2/version/` and is the version the caller sees: a user on the beta API sees the beta version string.

### Text output

Text is a summary for a person, not the same data as JSON. It lists only the environments and bundles valid for applications, and leaves out `buildStatus`, `sourceDockerImageUri`, `isPublic`, `cpuCount`, `scope`, and `httpStatus`. Read JSON in a pipeline.

### Limits

A list read stops at 100 pages or 10,000 items, and one response may hold at most 8 MiB. A list that reaches a cap, or whose next-page link repeats, leaves its route, or carries user information, makes its section `unavailable` with a message that says why. The command refuses a redirect to another scheme, host, or port, so the token never leaves the install's address.

## JSON output

```json
{
  "schemaVersion": 1,
  "platform": {
    "generatedAt": "2026-10-06T20:00:00Z",
    "producer": {"name": "dr", "version": "1.2.3"},
    "server": {"release": "11.12.0", "apiVersion": "2.48", "canonicalUrl": "https://dr.example.com"},
    "sections": {
      "install": {"scope": "platform", "status": "ok", "data": {"isEnterprise": true, "defaultAppResourceBundle": "cpu.xlarge"}},
      "seats": {"scope": "caller", "status": "ok", "data": {"seatLicenses": {}}},
      "limits": {"scope": "caller", "status": "ok", "data": {"maxConcurrentWorkloads": 50, "maxWorkloadReplicas": 3}},
      "entitlements": {"scope": "caller", "status": "ok", "data": {"ENABLE_WORKLOAD_API_CONTAINERS": true}},
      "executionEnvironments": {"scope": "caller", "status": "ok", "data": {"items": [
        {"id": "5f1a...", "name": "[DataRobot] Python 3.12 Applications Base", "programmingLanguage": "python",
         "useCases": ["customApplication"], "hasSuccessfulVersion": true, "buildStatus": "success", "isPublic": true}
      ]}},
      "resourceBundles": {"scope": "caller", "status": "unavailable", "httpStatus": 403, "message": "HTTP 403: {\"message\": \"...\"}"},
      "workloadApi": {"scope": "caller", "status": "ok", "data": {"totalCount": 7}}
    }
  }
}
```

Statuses are `ok`, `degraded`, and `unavailable`. An `unavailable` section carries `httpStatus`, the status of the one response that made it unavailable, so a reader does not parse it out of `message`. The field is absent when the failure was not an error status, such as a failed connection or a 200 answer that is not the expected JSON, and for `entitlements`, which several responses decide. While the command is gated the shape can change. After the gate comes off, changes are additive and a breaking change bumps `schemaVersion`. Readers ignore fields and sections they do not know.

## Examples

```bash
# A summary for a person.
export DATAROBOT_CLI_FEATURE_PLATFORM_INFO=1
dr info

# The environments valid for applications that have a successful build.
dr info --output-format json \
  | jq '.platform.sections.executionEnvironments.data.items[] | select(.useCases | index("customApplication")) | select(.hasSuccessfulVersion)'

# Check an environment id that the pipeline pins.
dr info --output-format json \
  | jq --arg id "$ENVIRONMENT_ID" '.platform.sections.executionEnvironments.data.items[] | select(.id == $id) | {id, hasSuccessfulVersion, isPublic, sourceDockerImageUri}'

# Why the Workload API refuses the caller, when it does.
dr info --output-format json \
  | jq -r '.platform.sections.workloadApi | select(.status == "unavailable") | "\(.httpStatus) \(.message)"'
```

## Security notes

- `dr info` accepts an `http://` endpoint. For an install that is not on loopback, the token travels in clear text. Use `https://`.
- `--skip-certificate-check` (`-k`) sends the token to a server whose certificate is not verified. The command adds no TLS handling of its own, and it sends the token to six routes.
- The report describes the install's internal topology: release, URLs, registry hosts, environment and bundle inventory, and seat licenses. Do not paste it into a public CI log or ticket.
