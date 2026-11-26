---
name: qa
description: >
  Run diff-targeted functional QA for the DataRobot CLI through a tuistory
  terminal driver (Droid Control in Droid, the tuistory CLI in Claude Code).
  Test branch-built CLI behavior with isolated configuration, select flows
  from the FLOWS.md menu, capture terminal evidence, and report results for
  PRs, releases, or explicit smoke checks.
---

# DataRobot CLI QA

**Scope:** Functional QA only. Do not run or report lint, type checks, unit tests,
static analysis, `task test`, or any existing automated smoke/acceptance suite.
Build preparation is allowed. This skill never commits, pushes, or posts reports.
When installed, a separately approved CI workflow may post its report.

`/qa` is the single entry point for CLI functional QA. Select test flows from
[`FLOWS.md`](FLOWS.md) in this skill's directory. It is a **menu**, not a
checklist: choose flows using the behavioral diff, and an explicit smoke request
may select a broader subset. FLOWS.md also lists Known Failure Modes; consult
them before diagnosing FAIL or BLOCKED results.

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
"No app code changed; QA not applicable for this diff." Do not build or run
pre-flight checks. An explicit user-requested full smoke run can select the CLI
without a diff; label it as an explicit smoke run.

For the affected CLI, select a relevant subset of the FLOWS.md menu plus adjacent
integration checks. At least half the tests must directly exercise the changed
behavior. Add an ad-hoc functional test if no menu item covers it. Include a
related negative or boundary test. If you cannot explain the changed behavior,
report INCONCLUSIVE rather than PASS.

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
  PATH; if missing, report BLOCKED with the install hint `npm i -g tuistory`.
  Compose is Droid-only: never load it here.

Then follow the build, session, and isolation mechanics in section 4.

Read `video_evidence` and the active driver's `compose` flag at runtime:

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

## 4. Testing mechanics

### Build and session setup

Build the checked-out branch using `task build` from the repository root.
Resolve `dist/dr` to an absolute path before switching to scratch directories.
Go must satisfy `go.mod`; Task and Git must be available. Do not substitute a
released binary if the build fails.

Use a unique run-prefixed session and 110 columns by 36 rows for every flow.
Launch a shell through the driver, then type invocations of the absolute `dr`
binary. Wait for the shell prompt between commands. Assert exit status
immediately after each invocation using the sentinel technique in
"Waits, sentinels, and exit-status assertions" below.
`dr` is a command-oriented CLI with per-command TUIs, not a REPL: type
`dr --help`, never `/help`. Exercise real keystrokes for a relevant TUI flow
when the changed path offers one; do not substitute unit tests or a
non-interactive agent transcript (`droid exec`, `claude -p`) for functional proof.

### Waits, sentinels, and exit-status assertions

- **Sentinels must not match the typed command line.** `tctl wait` (and
  `tuistory wait`) scans the whole scrollback, so `wait "exit-B1="` matches the
  typed `echo "exit-B1=$?"` before the command executes and returns early.
  Echo a unique digit-bearing marker and wait for the regex that requires a
  digit: type `echo "EXIT-<TAG>=$?"`, then `wait "/EXIT-<TAG>=[0-9]/"`.
- **Escape `$?` from wrapper expansion.** In a double-quoted `type` payload the
  agent's own shell expands `$?` before the bytes reach the PTY, capturing the
  driver's exit status (usually 0) and making every assertion pass vacuously.
  Single-quote the payload, or escape it (`echo EXIT=\\\$?`), so the child
  shell expands it. Verify the escape once per run: after `false`, the marker
  must read 1 before any exit assertion is trusted.
- **Type only at a confirmed idle prompt.** Keystrokes sent while the first-run
  welcome animation plays (skip-on-keypress) or while a command redraws the
  screen are dropped or consumed; queued echo markers vanish and the wait times
  out. Wait for an output marker first; if the marker never appears, re-send
  the echo at the idle prompt instead of assuming the command's exit status.
- **Do not trust `wait-idle` in fresh environments.** A `dr` invocation can
  stay silent for 60–90 seconds (plugin and update checks) before its first
  output, which `wait-idle` treats as stable. Use long-timeout regex waits,
  then snapshot.

Driver-specific session mechanics:

- **Droid:** route every terminal interaction through `droid-control`; never
  launch raw tuistory. Recording additionally needs asciinema, and terminal
  Compose needs agg and the Compose prerequisites.
- **Claude Code:** call the tuistory CLI directly from Bash. `--env` only adds
  to the inherited environment (and `env -i` discards it), so start the child
  from an empty environment and pass every value inside the `env -i` list:
  `tuistory launch "env -i PATH=$PATH HOME=<scratch>/home TERM=xterm-256color
  DATAROBOT_CLI_DISABLE_TELEMETRY=true bash --norc --noprofile"
  -s <RUN_ID>-<flow> --cols 110 --rows 36 --cwd <scratch>/project --background`,
  adding the remaining isolation variables the same way. Then use `type`,
  `press` (e.g. `enter`, `ctrl c`), `wait <pattern>`, `wait-idle`, and
  `snapshot --trim` for evidence. Run `close -s <session>` after each flow and
  confirm `tuistory sessions` lists nothing from this run before reporting.
  Never change Claude Code's own environment to isolate the child.

### Isolation

Create a new scratch directory with `mktemp` and record its path for cleanup.
Use separate scratch home, config, state, cache, and project directories. For the
**child application only**, set `HOME`, `XDG_CONFIG_HOME`, `XDG_STATE_HOME`,
`XDG_CACHE_HOME`, and `DATAROBOT_CLI_CONFIG` to run-owned paths. Never change the
parent agent's (Droid's or Claude Code's) home, authentication, or plugin
directories.

Start a clean child environment. Remove all inherited `DATAROBOT_*` and
`DR_API_TOKEN` values, including endpoint, token, profile, config, certificate,
feature, and skip-auth overrides. Add back only approved flow-specific settings.
Keep the toolchain on PATH, but isolate plugin discovery from user-installed
plugins. Clear `SSH_AUTH_SOCK` and do not inherit private service credentials.
Disable application telemetry with `DATAROBOT_CLI_DISABLE_TELEMETRY=true`.
Use the absolute binary and operate from the scratch project, not this repo.

In CI under Droid, the generic terminal launch prefix is
`env -u CI FACTORY_DISABLE_KEYRING=true`. `dr` uses Bubble Tea, not Ink or Factory
keyring authentication; these generic controls do not replace the isolation
above. Keep `DATAROBOT_CLI_NON_INTERACTIVE` unset for interactive flows. Set it
only for a selected non-interactive behavior test, never as deletion consent.

Do not suppress the first-run animation when first-run behavior is the change.
Otherwise allow it to finish before asserting terminal state. Capture only
non-secret dummy values and sanitized output in `qa-results/<RUN_ID>/`.

### Authentication and staging

The initial persona is `local_unauthenticated`. No signup, stored credentials,
or live API token is needed. Local tests must not call DataRobot APIs.

Staging gating (enabled flag, dedicated persona, per-run approval) is defined in
section 1. When a future approved staging run executes, consume
`DATAROBOT_ENDPOINT` and `DATAROBOT_API_TOKEN` from the configured dedicated
source; the complete pair takes priority over stored profiles. CI must pass any
future dedicated secret under the app's documented variable name; no interactive
login is needed. Any local-only QA workflow must supply no DataRobot credential.
Do not reuse the existing smoke-suite `DR_API_TOKEN` secret. Missing credentials
or per-run permission makes a selected staging flow BLOCKED.

Do not use `--skip-auth` to make live requests. For strictly local config/wizard
paths that have an authentication pre-hook, it may bypass that hook **only after
inspecting the selected RunE path to establish it makes no API requests**.
This does not test authentication or prove authorization. Help parsing and local
validation sometimes occur after auth, so never assume `--dry-run` is offline.

## 5. Execute and collect evidence

Run only selected flows with the configured persona. Drive actual terminal
keystrokes, observe transitions, and verify the app's output and exit status.
The CLI is not an agent: it has subcommands, not `/help` or a chat prompt.

Generate a unique RUN_ID and save sanitized evidence under
`qa-results/<RUN_ID>/`. Capture a distinct, labeled terminal text snapshot after
each meaningful step. Tuistory does not provide compositor PNG screenshots; use
its text snapshots. Real rendered-image proof requires the plugin's appropriate
terminal route and prerequisites (Droid only). Do not dump environment variables,
credentials, `.env` secrets, auth exports, personal paths, or secret-bearing
config/debug logs.

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

Before reporting, close run-owned sessions (even after a flow failed) and perform
configured cleanup, preserving sanitized evidence before removing this run's
scratch directory. Never delete pre-existing resources or sweep remote resources
by name prefix. Report leftover run-owned resources and cleanup errors as action
items.

## 6. Handle failures and report

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

## 7. Failure learning

Read `failure_learning` at runtime. For new testing-environment insights from
FAIL/BLOCKED that are not already covered by Known Failure Modes (see FLOWS.md),
append:

`## Suggested Skill Updates (N issues found)`

Use a table with number, severity (Breaking/Degraded/Info), target file, issue, and
a collapsible self-contained fix prompt. Include the exact Markdown to insert
and the heading/location in a collapsed copy block. Omit this section if none
qualify. Do not catalog expected PR changes, bad selectors, or skill typos.

For `suggest_in_report`, write suggestions only, never edit skills or create
`skill-updates.json`. Other modes require a separately configured apply workflow;
this installation does not grant repository write or PR-creation permission.
