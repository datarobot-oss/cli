# Spec: Forward universal CLI flags to plugins as `DATAROBOT_CLI_` env vars

Status: Implemented
Date: 2026-07-01
Component: DataRobot CLI (`dr`) — plugin execution + root flag traversal

---

## Original request (verbatim)

> We're having an issue where we aren't passing universal arguments into plugins.
> We want to follow the established pattern of consuming them if they happen before
> the plugin is called, and then converting them into environment variables for
> plugins to decide if they want to consume them. Specifically for now, that is just
> two items --debug and --disable-telemetry. We'll want to make these a struct and
> then pass environment variables to the plugin with the prefix `DATAROBOT_CLI_` so
> that when --debug is turned on we pass `DATAROBOT_CLI_DEBUG=1`. For the future, if
> that argument has a value like `--cacert <path>` we'd pass
> `DATAROBOT_CLI_CACERT=<path>` to the plugin. We need to clearly document this. Both
> for the plugin authors to consume, and for users of the CLI that they would need to
> pass `dr --debug <plugin> <plugin args>` to set debug for both the core
> functionality and the plugin (if it supports that flag).

### Follow-up clarifications (verbatim intent)

1. "Any arguments passed after the plugin should not even be seen or processed by the
   core machinery. i.e., the core should stay blind to the plugin args on purpose.
   I believe that is how it is wired now, and I absolutely do not want that to change.
   We are following the standards laid down by kubectl, helm, etc."
2. "Also add a regression test that this will not change with core subcommands, i.e.,
   `dr auth --set-url.. --debug` MUST properly set the debug flag from the universal set."

---

## Goal

`dr --debug <plugin> <args>` and `dr --disable-telemetry <plugin> <args>` must:

1. Be consumed by core (enable core debug / disable telemetry).
2. NOT be passed to the plugin as literal args.
3. Be forwarded to the plugin subprocess as env vars with the `DATAROBOT_CLI_` prefix:
   `DATAROBOT_CLI_DEBUG=1`, `DATAROBOT_CLI_DISABLE_TELEMETRY=1`.

Struct-driven and extensible: a future value flag `--cacert <path>` emits
`DATAROBOT_CLI_CACERT=<path>`.

## Hard invariant (non-negotiable)

Core stays BLIND to any args AFTER the plugin name (kubectl/helm model). Only flags
BEFORE the plugin name are consumed by core; everything after passes to the plugin
verbatim and is never parsed/validated by core.

---

## Design & key findings

- Plugin commands are registered in `cmd/plugin/discovery.go::createPluginCommand`
  with `DisableFlagParsing: true` (must stay — lets plugins receive their own flags
  verbatim).
- Root command (`cmd/root.go` `RootCmd`) previously had no `TraverseChildren`. With
  cobra's default `Find`, flags before the plugin name were swallowed into the plugin's
  raw args and never parsed by root. Both `dr --debug plug foo` and
  `dr plug --debug foo` collapsed to args `["--debug","foo"]` (position lost).
- **Fix:** Set `TraverseChildren: true` on `RootCmd`. Verified against cobra's
  `Traverse`: root parses persistent flags encountered before a command name, then
  descends. A plugin has no subcommands, so `findNext` returns nil and `Traverse`
  returns early WITHOUT calling `ParseFlags` on the remaining tokens; `execute()` then
  honours `DisableFlagParsing` and hands raw args to `Run`. Pre-plugin flags are parsed
  by core; post-plugin args stay invisible to core. Core subcommands (no
  `DisableFlagParsing`) still parse `--debug` appearing after the subcommand name.
- `buildPluginEnv` in `internal/plugin/exec.go` builds `cmd.Env`; universal flag env
  vars are appended after `os.Environ()` so they override any inherited values.
- Flags already bound to viper via `viperx.BindPFlag("debug")` and
  `viperx.BindPFlag("disable-telemetry")` in `cmd/root.go` init(). Read via
  `viperx.GetBool`.

---

## Implementation

### Files changed

| File | Change |
|---|---|
| `internal/plugin/universalenv.go` | **New.** Struct table `[]universalFlag{ViperKey, EnvSuffix, IsBool}` seeded with `debug→DEBUG` and `disable-telemetry→DISABLE_TELEMETRY`. `universalFlagEnv() []string` emits `DATAROBOT_CLI_<SUFFIX>=1` for true bools, `=<value>` for non-empty strings (future). |
| `internal/plugin/exec.go` | `buildPluginEnv` appends `universalFlagEnv()` results. |
| `cmd/root.go` | `TraverseChildren: true` added to `RootCmd` cobra.Command. |
| `internal/plugin/universalenv_test.go` | **New.** Unit tests for `universalFlagEnv` (all unset, debug only, telemetry only, both, false-omitted). Isolated cobra-tree tests for the core-blind invariant (pre-plugin flag consumed; post-plugin flag invisible). |
| `cmd/root_test.go` | `TestRootCmdTraverseChildrenEnabled` (guard). `TestUniversalFlagsParsedOnCoreSubcommand` (regression: `--debug` after a core subcommand's own flags is still parsed). |
| `docs/development/plugins.md` | New "### Environment variables" section documenting all vars + `DATAROBOT_CLI_*` forwarding convention + code examples. |
| `docs/commands/plugins.md` | New "## Passing global flags to plugins" section with ordering rule, table, and link to authoring reference. |

### Adding a new universal flag in the future

1. Ensure the flag is already defined as a persistent flag on `RootCmd` in
   `cmd/root.go` and bound to viper via `viperx.BindPFlag`.
2. Add one entry to `universalFlags` in `internal/plugin/universalenv.go`:
   - `IsBool: true` for boolean flags (emits `=1` / omitted).
   - `IsBool: false` for string flags (emits `=<value>` / omitted when empty).
3. Update the tables in `docs/development/plugins.md` and
   `docs/commands/plugins.md`.

---

## Decisions

- Boolean universal flags emit value `1` (per the `DATAROBOT_CLI_DEBUG=1` example).
  Struct is extensible so future value flags emit their actual value.
- Only flags **before** the plugin name are consumed by core; args after the plugin
  name are never seen by core (kubectl/helm model). Regression-tested.
- Mechanism: `TraverseChildren: true` on root — chosen over os.Args splitting.
- Scope: only `--debug` and `--disable-telemetry` for now.
