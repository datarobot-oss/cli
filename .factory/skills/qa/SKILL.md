---
name: qa
description: >
  Run diff-targeted functional QA for the DataRobot CLI through a tuistory
  terminal driver (Droid Control in Droid, the tuistory CLI in Claude Code).
  Test branch-built CLI behavior with isolated configuration, capture terminal
  evidence, and report results for PRs, releases, or explicit smoke checks.
---

# DataRobot CLI QA

**Scope:** Functional QA only. Do not run or report lint, type checks, unit tests,
static analysis, `task test`, or any existing automated smoke/acceptance suite.
Build preparation is allowed. This skill never commits, pushes, or posts reports.
When installed, a separately approved CI workflow may post its report.

## 1. Load configuration and select target

Read `.factory/skills/qa/config.yaml` on every run. It is the single source of
truth for targets, personas, permissions, and evidence options.

Use `default_target` unless the user explicitly selects another configured
target. Unattended CI uses `ci.target` and must remain local-only. Never treat
credentials, a feature gate, or access to an endpoint as authorization.

Staging requires `enabled`, a dedicated configured persona, credential references,
and explicit per-run approval of the endpoint and remote actions. Until accounts
are configured, run only unauthenticated local flows. If a selected test requires
staging, report it BLOCKED instead of substituting another test or production.
Do not create accounts. Never read or copy the user's normal `drconfig.yaml`.

Public downloads are allowed. Downloaded code execution and dependency installation
by `dr` need separate action-specific approval. Installing the QA toolchain named
in the approved workflow is distinct from granting the application those powers.
No human is available in CI: never ask questions or wait for confirmation there.

## 2. Determine diff scope

Honor the explicit `QA_BASE_SHA` supplied by CI and compare its merge base with
HEAD: `git diff "$QA_BASE_SHA"...HEAD`. Also inspect local staged/unstaged changes
when running locally. Without an explicit base, resolve the main branch ref
(`upstream/main`, then `origin/main`, then `main`) and include local changes.
Do not silently compare with HEAD's parent. If no usable base exists, report
INCONCLUSIVE and explain how to provide one.

Map changed paths using `apps.*.path_patterns`. Inspect the behavioral hunks, not
just filenames. Docs, CI, QA skill files, and other unmatched paths do not select
the CLI. If no app matches, report INCONCLUSIVE:
"No app code changed; QA not applicable for this diff." Do not build, load an app
sub-skill, or run app pre-flight checks. An explicit user-requested full smoke run
can select the CLI without a diff; label it as an explicit smoke run.

For affected apps, load only their configured sub-skills. Select a relevant subset
of their flow menu plus adjacent integration checks. At least half the tests must
directly exercise the changed behavior. Add an ad-hoc functional test if no menu
item covers it. Include a related negative or boundary test. If you cannot
explain the changed behavior, report INCONCLUSIVE rather than PASS.

## 3. Pre-flight and interaction routing

Both agents read the whole config, but each acts only on its own driver block:
`droid_control` in Droid, `claude_code` in Claude Code. Shared keys (targets,
personas, `video_evidence`, cleanup) apply to both. Pick the route for the agent
you are running in:

- **Droid:** for the selected CLI, invoke `droid-control` before terminal
  interaction. Follow its terminal/TUI route through the tuistory backend and
  `tctl` wrapper. Let the installed skill resolve its own plugin paths and driver
  mechanics. Verify `droid-control@factory-plugins` is effectively active and the
  project settings declare its dependency. An active inherited user-scope
  installation is valid; do not uninstall it to force a duplicate project-scope
  install. An inactive or policy-blocked plugin is BLOCKED.
- **Claude Code:** there is no Droid Control plugin. Drive the same tuistory
  backend directly through its CLI (`claude_code.tool`). Verify `tuistory` is on
  PATH; if missing, report BLOCKED with the install hint `npm i -g tuistory`. Use
  `launch --background`, `type`, `press`, `wait`, `wait-idle`, `snapshot --trim`,
  and `close` as described in `qa-cli`. Compose is Droid-only: never load it here.

Build with the configured build command only when the CLI is selected. Use the
current checkout's binary, never a system-installed or released `dr`. Establish
run-scoped sessions, fresh app configuration, and scratch directories as
described in `qa-cli`.

Capture and verify every flow (Droid: Capture and Verify; Claude Code: labeled
`tuistory snapshot --trim` output plus exit-status checks). Read `video_evidence`
and the active driver's `compose` flag at runtime:

- Text snapshots remain primary evidence in every branch.
- When `video_evidence` is enabled, record one raw cast per interactive flow.
  Under Claude Code, record with `asciinema` if installed; otherwise rely on text
  snapshots and note the missing recorder as an action item.
- Invoke Compose only in Droid and only when both flags are enabled. Otherwise
  do not load Compose or install its dependencies.
- Compose owns prerequisite checks and plugin-root resolution. Install its locked
  Remotion dependencies only when needed. Render a single-layout MP4 with literal
  `"preset": "factory"`, trim dead time, and keep it at most 60 seconds.
- Ignore any manually added preset config. Branding is not user-configurable.

A failed pre-flight blocks the selected app, not unrelated apps. Explain the exact
error and remediation. Never invent mocked DataRobot responses to bypass a block.

## 4. Execute and collect evidence

Run only selected flows with the configured persona. Drive actual terminal
keystrokes, observe transitions, and verify the app's output and exit status.
The CLI is not an agent: it has subcommands, not `/help` or a chat prompt.

Generate a unique RUN_ID and save sanitized evidence under
`qa-results/<RUN_ID>/`. Capture a distinct, labeled terminal text snapshot after
each meaningful step. Do not dump environment variables, credentials, `.env`
secrets, auth exports, personal paths, or secret-bearing config/debug logs.

Raw casts are downloadable artifacts, not inline videos. Never upload a cast to
an image/video endpoint. If recording or Compose fails, retry the stage once,
then retain safe raw evidence and use text snapshots; do not fail app behavior
solely because optional media could not be produced.

For GitHub, with video evidence enabled and explicit upload authorization
(the approved CI workflow or a separate local request), upload only verified,
non-empty playable media to the user-attachments endpoint using
`QA_EVIDENCE_TOKEN`, a classic PAT with `repo` scope and any required org SSO.
Neither `GITHUB_TOKEN` nor a fine-grained PAT works for that endpoint. Never print
the token. Verify the live response; place a returned video URL alone on its own
line, never inside image Markdown. Screenshots may use image Markdown only with
a verified uploaded asset URL. Never upload media without authorization.
When evidence upload is disabled, unauthorized, or fails, reference filenames in
downloadable artifacts and use text, never broken repository/artifact image URLs.

Before reporting, close run-owned sessions and perform configured cleanup.
Never delete pre-existing resources or sweep remote resources by name prefix.
Report leftover run-owned resources and cleanup errors as action items.

## 5. Handle failures and report

**Never silently skip a flow. If a flow cannot complete, report it as BLOCKED
with what was tried and how the user can fix it.** Continue other selected flows.
Distinguish product FAIL from missing prerequisites/permissions BLOCKED.
Do not turn a missing account into PASS by testing only help.

Write `qa-results/report.md` using `.factory/skills/qa/REPORT-TEMPLATE.md`.
Start with `## QA Report` and the results table. Use the template's result markers.
Include only behavior tests, not builds or driver installation as rows.
Keep action items brief and put all evidence in one collapsed details block.
For a no-app diff, include one INCONCLUSIVE scope row and no app-flow rows.

Also write `qa-results/status.json` with this machine-readable contract:

```json
{"results": ["PASS", "BLOCKED"], "summary": "BLOCKED"}
```

List one uppercase result per report row. Aggregate with precedence:
FAIL, BLOCKED, FLAKY, INCONCLUSIVE, PASS. Include INCONCLUSIVE for a no-app diff.
Never claim PASS with zero tests. CI validates this file independently of the
agent's exit code.

## 6. Failure learning

Read `failure_learning` at runtime. For new testing-environment insights from
FAIL/BLOCKED that are not already covered by Known Failure Modes, append:

`## Suggested Skill Updates (N issues found)`

Use a table with number, severity (Breaking/Degraded/Info), target file, issue, and
a collapsible self-contained fix prompt. Include the exact Markdown to insert
and the heading/location in a collapsed copy block. Omit this section if none
qualify. Do not catalog expected PR changes, bad selectors, or skill typos.

For `suggest_in_report`, write suggestions only, never edit skills or create
`skill-updates.json`. Other modes require a separately configured apply workflow;
this installation does not grant repository write or PR-creation permission.
