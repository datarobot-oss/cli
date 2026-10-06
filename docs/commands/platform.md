# `dr platform`: ask an install what it supports

Report what a DataRobot install says about itself, so a deploy pipeline can be written for that install before anything is uploaded.

## Synopsis

```bash
dr platform describe [flags]
```

## Description

A Custom Application deploy depends on values that differ per install: which execution environment and resource bundle are valid for applications, which feature flags are on, which seat licenses the caller has, and the install's release and URL. A pipeline copied from another install breaks on the first mismatch, usually after the upload.

`dr platform describe` reads those values from public routes with the caller's own credentials and prints one report. It adds no access: a source the caller cannot read becomes an `unavailable` section that carries the route's reason, and the other sections still report.

The report states facts and leaves the verdict to the reader. Read `hasSuccessfulVersion` and `useCases` and decide; the command does not say "you can create an app".

Authentication follows the other commands: `DATAROBOT_ENDPOINT` and `DATAROBOT_API_TOKEN` when both are set, otherwise the active profile. A private CA and proxy settings apply as everywhere else.

### Exit status

The command exits zero when it prints a report, including one with unavailable sections. It exits non-zero only when it cannot authenticate or cannot reach the install.

## Flags

- `--output-format <text|json>`: `json` prints the report itself, described by [`docs/schemas/platform-describe.schema.json`](../schemas/platform-describe.schema.json). The default is `text`, a summary for a person.

## Sections

| Section | Source | What it holds |
|---|---|---|
| `install` | public `/config` | `isEnterprise`, `defaultAppResourceBundle`. A key the install does not return is left out |
| `seats` | `/api/v2/account/info/` | `seatLicenses`: each license mapped to whether the caller has it. A license that is absent is not enforced on that install, so `{}` does not mean blocked |
| `entitlements` | `/api/v2/entitlements/evaluate/` | The flags a Custom Application deploy depends on. Names the install does not know are listed in the message and the section is `degraded` |
| `executionEnvironments` | `/api/v2/executionEnvironments/` | Every environment the caller can read: `id`, `name`, `programmingLanguage`, `useCases`, `hasSuccessfulVersion`, `buildStatus`. Names are not unique, so pick by `id` and `hasSuccessfulVersion`. DataRobot marks a retired environment only in its name, with a `[Deprecated]` prefix. An environment is valid for applications when `useCases` lists `customApplication` |
| `resourceBundles` | `/api/v2/mlops/compute/bundles/` | Every bundle the caller can read: `id`, `name`, `useCases`, `memoryBytes`, `cpuCount`. A bundle is valid for applications when `useCases` lists `customApplication` |

`server` carries the `release`, the `apiVersion`, and the `canonicalUrl` of the install.

## JSON output

```json
{
  "schemaVersion": 1,
  "generatedAt": "2026-10-06T20:00:00Z",
  "producer": {"name": "dr", "version": "1.2.3"},
  "server": {"release": "11.12.0", "apiVersion": "2.48", "canonicalUrl": "https://dr.example.com"},
  "sections": {
    "install": {"status": "ok", "data": {"isEnterprise": true, "defaultAppResourceBundle": "cpu.xlarge"}},
    "seats": {"status": "ok", "data": {"seatLicenses": {}}},
    "entitlements": {"status": "ok", "data": {"ENABLE_WORKLOAD_API_CONTAINERS": true}},
    "executionEnvironments": {"status": "ok", "data": {"items": [
      {"id": "5f1a...", "name": "[DataRobot] Python 3.12 Applications Base", "programmingLanguage": "python",
       "useCases": ["customApplication"], "hasSuccessfulVersion": true, "buildStatus": "success"}
    ]}},
    "resourceBundles": {"status": "unavailable", "message": "HTTP 403: {\"message\": \"...\"}"}
  }
}
```

Statuses are `ok`, `degraded`, and `unavailable`. Changes are additive: a breaking change bumps `schemaVersion`. Readers ignore fields and sections they do not know.

## Examples

```bash
# A summary for a person.
dr platform describe

# The environments a pipeline can build an application on.
dr platform describe --output-format json \
  | jq '.sections.executionEnvironments.data.items[] | select(.useCases | index("customApplication")) | select(.hasSuccessfulVersion)'
```
