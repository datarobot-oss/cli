---
name: qa-cli
description: >
  Functional QA for the Go DataRobot CLI: command discovery, isolated auth
  configuration, dotenv TUIs, task execution, local validation, and approved
  staging workload/artifact flows. Drives the terminal through tuistory (Droid
  Control in Droid, the tuistory CLI in Claude Code).
---

# DataRobot CLI functional QA

Read `.factory/skills/qa/config.yaml`, even when invoked independently.
This is a **menu**, not a checklist. Select flows using the behavioral diff;
an explicit smoke request may select a broader subset. Do not run test suites.

## Testing target and prerequisites

Build the checked-out branch using `task build` from the repository root.
Resolve `dist/dr` to an absolute path before switching to scratch directories.
Go must satisfy `go.mod` (currently 1.27.1); Task and Git must be available.
Do not substitute a released binary if the build fails.

Use a unique run-prefixed session and 110 columns by 36 rows for every flow.
Follow the driver route for the agent you are running in (see `qa` section 3):

- **Droid:** invoke `droid-control` for all terminal interactions. Route through
  its terminal tuistory backend and `tctl` wrapper. Use Capture and Verify; load
  Compose only when both `video_evidence` and `droid_control.compose` in config
  are enabled. Let the installed plugin own launch, keystroke, capture, and
  verification mechanics. Do not launch raw tuistory. Recording additionally
  needs asciinema, and terminal Compose needs agg and the Compose prerequisites.
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

Launch a shell through the driver, then type invocations of the absolute `dr`
binary. Wait for the shell prompt between commands. Assert exit status immediately
after each invocation (e.g. type `echo "exit=$?"` and wait for it). `dr` is a
command-oriented CLI with per-command TUIs, not a REPL: type `dr --help`, never
`/help`. Exercise real keystrokes for a relevant TUI flow when the changed path
offers one; do not substitute unit tests or a non-interactive agent transcript
(`droid exec`, `claude -p`) for functional proof.

### Isolation

Create a new scratch directory with `mktemp` and record its path for cleanup.
Use separate scratch home, config, state, cache, and project directories. For
the **child application only**, set `HOME`, `XDG_CONFIG_HOME`, `XDG_STATE_HOME`,
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

Staging is configured but disabled until dedicated accounts and secret references
are provided. Approved future staging runs consume `DATAROBOT_ENDPOINT` and
`DATAROBOT_API_TOKEN` from the configured dedicated source; the complete pair
takes priority over stored profiles. CI must pass any future dedicated secret
under the app's documented variable name; no interactive login is needed.
Any local-only QA workflow must supply no DataRobot credential.
Do not reuse the existing smoke-suite `DR_API_TOKEN` secret. Missing credentials
or per-run permission makes a selected staging flow BLOCKED.

Do not use `--skip-auth` to make live requests. For strictly local config/wizard
paths that have an authentication pre-hook, it may bypass that hook **only after
inspecting the selected RunE path to establish it makes no API requests**.
This does not test authentication or prove authorization. Help parsing and local
validation sometimes occur after auth, so never assume `--dry-run` is offline.

## Flow menu

### A. Discovery, aliases, output, and completions

Relevant paths: root command, command registration, output formatting, completion.

- Type `dr --help`, the changed command's `--help`, and applicable alias.
  Verify singular canonical command names, correct flags, and adjacent help.
- Toggle the relevant feature gate for this process only; verify gated command
  visibility in both states. A gate is not a platform entitlement.
- Exercise invalid subcommands/arguments/flags near the change and assert an
  actionable error and expected exit status. Never require a network request
  just to read help.
- For version changes, exercise `dr --version` and
  `dr self version --format=json`. For other commands, discover their supported
  JSON flag from help, commonly `--output-format json`; do not assume `--format`.
  Capture stdout/stderr separately and parse stdout as one JSON document.
- For completion changes, generate the changed shell's completion and source it
  only in a disposable shell. Press Tab with a partial command and observe the
  completed/suggested command. Never install completions in the user's shell.

### B. Auth configuration, profiles, and local failure paths

Relevant paths: `cmd/auth/**`, auth/config/profile packages, host-picker TUI.

- With empty isolated config, run `dr auth check`; verify missing endpoint/token
  advice and failure status without opening a browser.
- Exercise `dr auth profile list` and `dr auth profile show` with non-secret
  run-owned config. Test a missing profile and check that no other profile's
  credentials are inherited. Do not print stored tokens.
- `dr auth set-url` calls authentication after writing the endpoint. For local
  host-picker behavior, use `--skip-auth` only for this known config-only path:
  select a cloud/custom URL with actual keystrokes, verify the written canonical
  endpoint and sibling config/comment preservation, then test Ctrl-C separately.
  Do not proceed to browser login or mistake skipped auth for successful login.
- Test malformed endpoint/argument failures only if the selected code path
  rejects them before any HTTP call. Never send invalid dummy tokens to a real
  endpoint as an "offline" negative test.
- Auth verification, environment-vs-stored token precedence, and genuine permission
  denial need approved staging personas. Report them BLOCKED when selected without
  prerequisites. Do not call `auth export` with real secrets in captured sessions.

### C. Dotenv editor, wizard, and file preservation

Relevant paths: `cmd/dotenv/**`, envbuilder, dotenv TUI, YAML/config writers.

- Create a scratch `.env` containing comments and harmless sample variables
  such as `QA_LABEL=before`. Launch `dr dotenv edit`, edit a sample value using
  discovered bindings, save, and verify comments/unrelated keys survive.
- Reopen to verify the saved state. Exercise cancellation/Ctrl-C in a separate
  invocation and verify it does not partially write the file.
- Use actual documented `.datarobot/` prompt definitions for wizard tests,
  obtained from current repository examples. Sample local input files are allowed;
  do not mock backend responses or pretend they prove live behavior.
- `dotenv setup` has an auth pre-hook. Inspect its selected path before using
  `--skip-auth` for a local-only wizard; otherwise report the flow BLOCKED.
  Check interactive vs explicit non-interactive behavior separately.
- When the diff concerns round trips, include empty/multiline values and deletion
  of a key with its associated CLI marker. Never touch the repository's own `.env`.

### D. Templates, tasks, dependencies, and plugins

Relevant paths: templates/start, run/task, dependencies/tools, plugin/self.

- For task-runner changes, create a minimal scratch Taskfile with a harmless
  task that prints a sentinel. Use current command docs to establish any required
  repo/.env structure. Select/run the task through the CLI, verify the sentinel
  and failure exit status from a separate deliberately failing local task.
- Public template/plugin metadata downloads are allowed. Capture listing/filtering
  behavior where it needs no authenticated API request. If discovery/setup requires
  a DataRobot token, report the selected operation BLOCKED.
- Template clones must stay inside scratch. Do not automatically run downloaded
  Taskfiles, hooks, plugins, package installers, or application deployment scripts.
  Those require separate explicit approval; unattended CI cannot obtain it.
- For plugin discovery, use a harmless locally authored dummy executable only
  when it directly tests the changed plugin contract. It is not a substitute for
  a genuine downloaded-plugin test. Verify no personal plugin is discovered.
- Test dependency planning/listing and input validation where safe; installation
  and self-update require approval and an isolated install prefix. Never replace
  the user's `dr`, Homebrew installation, or global dependencies.

### E. Workload/artifact local configuration and validation

Relevant paths: workload/artifact commands, manifest/wapi/wizard/sync packages.

- Set `DATAROBOT_CLI_FEATURE_WORKLOAD_ALPHA=true` for config/up help or local wizard
  tests that need it. Check help and required flag/ID/spec validation.
- Use scratch Dockerfiles, `.datarobot.yaml`, `.env`, and project state built from
  current documented formats. Verify local schema rejection, file preservation,
  and clear remediation without making DataRobot requests.
- Config and up have authenticated paths. Inspect the exact selected flow before
  attempting an offline config wizard or preview. If it reads/binds a workload,
  resolves platform metadata, or creates credentials, it is not local-only.
- A sync preview can still generate/refresh `uv.lock` and execute `uv lock`.
  Do not invoke it when dependency work lacks approval, even with `--dry-run`.
- Verify JSON stdout purity, missing/corrupt local state errors, canceled wizards,
  and relevant comment/marker round trips where reachable without remote access.
  Mark remote-dependent assertions BLOCKED rather than mocking APIs.

### F. Approved staging lifecycle, sync, and permissions

Relevant paths: DataRobot client wrappers and resource/authorization behavior.

This menu entry is unavailable until staging is enabled, accounts are configured,
and a human approves the endpoint and exact mutations for this run. Do not run
it in the local-only PR workflow.

- Use the branch binary and configured staging credentials, not browser login.
  Give every created resource a run-specific name, record its returned ID/type in
  a private run-owned ledger immediately, and never alter an existing resource.
- Select only lifecycle operations needed by the diff: artifact create/get,
  code init/sync/version/checkout, workload create/status/endpoint/logs/events,
  settings/start/stop/delete. Read current help/docs for exact syntax and payloads.
- Verify asynchronous transitions with bounded polling (up to five minutes for
  normal startup). Do not run the 20–30 minute build acceptance scenario by
  default or launch existing acceptance scripts as a substitute for interaction.
- Treat a workload 403 as a possible missing platform entitlement, not proof of
  an invalid token. A negative permission test needs an actual configured
  restricted persona; never invent an administrator or read-only role.
- Test component/enclave/LLM gateway/pipeline behavior only when the diff touches
  it and the run has the corresponding approved access. Do not make paid LLM
  requests or change enclave placement without separate approval.
- Stop/delete only ledger-recorded resources created by this run. Report leftovers,
  including artifact/catalog/credential IDs requiring manual cleanup, without
  exposing their secrets.

## Evidence, cleanup, and reporting

Capture distinct text snapshots at meaningful transitions. Tuistory does not
provide compositor PNG screenshots; use its text snapshots, not an unsupported
screenshot command. Real rendered-image proof requires the plugin's appropriate
terminal route and prerequisites (Droid only). Under Claude Code, text snapshots
are the evidence; save each one as a labeled file under `qa-results/<RUN_ID>/`.

Close each session after its flow, even after failure. Preserve sanitized evidence
before removing this run's scratch directory. Never put tokens or secret-bearing
raw recordings into artifacts. Never silently skip a selected flow: report BLOCKED
with attempts and remediation, and continue other selected flows.

For standalone invocation, use `qa/REPORT-TEMPLATE.md` and write
`qa-results/report.md` plus `qa-results/status.json` using the orchestrator's
result contract. Report behavior assertions only, not setup as passing tests.

## Known Failure Modes

1. **Auth is not merely a pre-hook.** `auth set-url` starts auth after saving;
   some dry-run/config commands still call APIs. Inspect the actual command path
   before calling it local-only.
2. **Staging accounts are not configured.** Local unauthenticated tests cannot
   establish API correctness or permission boundaries. Selected live flows remain
   BLOCKED until dedicated accounts and explicit run approval exist.
3. **Feature gate versus entitlement.** A visible workload/enclave command can
   still return 403 because CLI gates do not grant platform permission.
4. **Config/environment fallback.** An invalid explicit endpoint/token pair must
   not silently use stored credentials. Quoted endpoint values are not stripped.
5. **Interactive startup.** A fresh TTY can show a welcome animation. Wait for
   the UI before typing; do not globally enable non-interactive mode.
6. **Wrong JSON flag.** `self version` uses `--format=json`; resource commands
   generally use `--output-format json`. Verify the selected command's help.
7. **Sync previews can modify lockfiles.** `--dry-run`/`--diff` can run `uv lock`;
   no remote write does not mean no installation or local file change.
8. **Unapproved downloaded code.** Public fetch permission is not permission to
   execute plugins, Taskfiles, or installers. Report selected execution BLOCKED.
