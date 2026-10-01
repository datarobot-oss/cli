# Telemetry (contributor guide)

This page covers how telemetry is implemented and how to add it to a command. For what the CLI collects, where it is sent, and how to opt out, see [Telemetry](../user-guide/telemetry.md) in the user guide.

Telemetry is implemented in `internal/telemetry/`.

## Common Properties

The following are attached to every event:

### Top-level event fields

These map to Amplitude's built-in fields and power native segmentation (version filters, OS breakdowns, language charts, etc.).

| Field         | Source                                                                                                                                                                          |
|---------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `user_id`     | DataRobot `uid` from `GET /api/v2/account/info/`, cached to disk with endpoint + token fingerprint validation; empty if unauthenticated or cache miss — see [User ID](../user-guide/telemetry.md#user-id) |
| `device_id`   | OS machine ID (hashed) or persisted UUID — see [Device ID](../user-guide/telemetry.md#device-id)                                                                                                  |
| `session_id`  | Unix millisecond timestamp generated once per process invocation — Amplitude uses this as the built-in Session ID for session-based analysis                                    |
| `app_version` | CLI version set at build time via ldflags                                                                                                                                       |
| `platform`    | Always `"CLI"`                                                                                                                                                                  |
| `os_name`     | OS name (e.g. `"macOS"`)                                                                                                                                                        |
| `os_version`  | OS version (e.g. `"15.7.5"`)                                                                                                                                                    |
| `language`    | User locale tag (e.g. `"en-US"`), via `go-locale`; Amplitude maps to a display language name                                                                                    |
| `ip`          | Always `"$remote"` — Amplitude resolves location server-side                                                                                                                    |

### Event properties

| Property             | Source                                                                                                        |
|----------------------|---------------------------------------------------------------------------------------------------------------|
| `install_method`     | Set at build time via ldflags (`release`, `source`, etc.)                                                     |
| `os_arch`            | CPU architecture from `runtime.GOARCH`                                                                        |
| `go_version`         | Go runtime version (e.g. `go1.26.4`) from `runtime.Version()`                                                 |
| `shell`              | Detected shell name (e.g. `zsh`, `bash`, `powershell`) from parent process / `$SHELL`                         |
| `datarobot_instance` | Base URL of the configured DataRobot instance                                                                 |
| `command_kind`       | `"core"` or `"plugin"` — automatically set by the root command dispatcher                                     |
| `organization_id`    | DataRobot org ID from `GET /api/v2/account/info/`, cached to disk; absent if unauthenticated                  |
| `tenant_id`          | DataRobot tenant ID from `GET /api/v2/account/info/`, cached to disk; may be empty for legacy/system accounts |
| `template_name`      | Best-effort from `.datarobot/answers/` in the current repo                                                    |
| `template_id`        | Stable ID of the DataRobot template used to create this project; absent when not inside a template project    |

## Event Wiring

Telemetry events are wired declaratively at command-construction time using a small API exported by `internal/telemetry`:

| Helper                              | Use when…                                                                       |
|-------------------------------------|---------------------------------------------------------------------------------|
| `telemetry.Track(cmd)`              | The command needs no extra event properties beyond the common ones.             |
| `telemetry.TrackWith(cmd, extract)` | The command needs dynamic event properties from flags or args at firing time.   |
| `telemetry.TrackPlugin(cmd, ver)`   | The command comes from a plugin. Adds `plugin_version` and sets `command_kind`. |

Each helper sets a `"telemetry"` annotation on the cobra command. After RunE completes, [`cobra.OnFinalize`](https://pkg.go.dev/github.com/spf13/cobra#OnFinalize) calls `telemetry.EventFor(cmd, args)`, which returns an Amplitude event with `EventType == cmd.CommandPath()` and any properties the registered extractor produced.

This approach ensures:

- **Local**: Wiring lives next to the command it tracks, not in a central map.
- **Late-bound**: Events fire after RunE, so PropExtractors can read results computed during command execution (see [Reading RunE results in a PropExtractor](#reading-rune-results-in-a-propextractor)).
- **Extensible**: Adding a new event requires one call where the command is built.
- **Self-documenting**: The cobra command itself carries its telemetry intent.

## Process exit and telemetry flush

Amplitude events are queued in-process and flushed asynchronously. If a command calls `os.Exit` directly (plugins, task runner exit-code propagation) or if `RunE` returns an error that causes cobra to skip `PersistentPostRunE`, the queue would be silently dropped. Two mechanisms handle this:

### `cmd.Exit(code int)` — for `main.go`'s error path

`cmd.Exit` lives in `cmd/exit.go` alongside the `telemetryClient` package-level variable that `PersistentPreRunE` sets. Use it only from `main.go` when `ExecuteContext` returns an error:

```go
if err := cmd.ExecuteContext(ctx); err != nil {
    log.Stop()
    cmd.Exit(1) // flushes telemetry then calls os.Exit(1)
}
```

`cmd.Exit` is nil-safe: if `PersistentPreRunE` never ran (e.g. flag parse failure before any command executes) there are no queued events and it falls straight through to `os.Exit`.

### `telemetry.ExitWithContext(ctx context.Context, code int)` — for cobra sub-commands

Commands that must propagate a subprocess exit code (plugin dispatch, `task run --exit-code`) cannot use `return err` because Go errors carry no integer code. They call `telemetry.ExitWithContext` with the command's cobra context — the client stored there by `PersistentPreRunE` is flushed before `os.Exit`:

```go
RunE: func(cmd *cobra.Command, args []string) error {
    exitCode := runSubprocess(...)
    telemetry.ExitWithContext(cmd.Context(), exitCode) // never returns
    return nil // unreachable
},
```

### State ownership

`internal/telemetry` is **stateless** — it defines `Client`, helpers, and `ClientContextKey` but holds no global variables. The two places that own state are:

1. The `cmd` package (`root.go` + `exit.go`) with the `telemetryClient` variable.
2. The cobra command context, which stores a `*telemetry.Client` under `telemetry.ClientContextKey{}`.

Both are set in `PersistentPreRunE`; the context value is consumed by `PersistentPostRunE` (normal path) or `telemetry.ExitWithContext` (exit-code path).

## Silent errors and `SilenceErrors`

`cli.ErrSilent` (`internal/cli/command.go`) is a sentinel returned by `RunE` implementations that have **already printed** their own user-facing error. Returning it tells cobra "don't add a second `Error: …` line", but only if the command (or an ancestor) has `SilenceErrors: true` set.

**Rules:**

1. Any command whose `RunE` can return `cli.ErrSilent` — either directly or by calling a sub-command's `RunE` — must set `SilenceErrors: true` on its own `cobra.Command`.
2. When composing commands (e.g. calling `compose.Cmd().RunE(nil, nil)` inside another command's `RunE`), the *caller* must also carry `SilenceErrors: true`, because the sentinel propagates up the call stack.
3. `main.go` always calls `cmd.Exit(1)` for any non-nil error, so telemetry is recorded even for silent failures.

**Example:**

```go
func Cmd() *cobra.Command {
    return &cobra.Command{
        Use:           "add [...]",
        RunE:          RunE,
        SilenceErrors: true, // required: RunE can return cli.ErrSilent via compose.Cmd().RunE
    }
}

func RunE(_ *cobra.Command, args []string) error {
    if err := addComponents(args); err != nil {
        return err
    }

    // compose prints its own error and returns cli.ErrSilent on failure;
    // SilenceErrors: true above prevents cobra from echoing "Error: silent error".
    return compose.Cmd().RunE(nil, nil)
}
```

### Execution flow

```text
User invokes command
    ↓
Cobra parses flags
    ↓
PersistentPreRunE (root.go)
    ├─ Initialize CommonProperties (session ID, user ID, env, ...)
    ├─ Stamp props.CommandKind = "core" or "plugin"
    │   based on telemetry.IsPluginCommand(cmd)
    ├─ Stamp props.NonInteractive via telemetry.StampInteractionMode
    │   so every event shares the same automation signal
    ├─ Build telemetry.Client
    └─ Register cobra.OnFinalize (closes over cmd, args, client)
    ↓
RunE / Run executes
    ↓
cobra.OnFinalize (via cobra's deferred postRun, fires on success and error paths — unlike PersistentPostRunE, see [gotcha](https://www.jvt.me/posts/2024/11/29/gotcha-cobra-persistentpostrune/))
    ├─ telemetry.EventFor(cmd, args) → if tracked, client.Track(event)
    ├─ Flush telemetry (3-second timeout)
    └─ log.Stop()
```

### Stamping universal interaction flags

`telemetry.StampInteractionMode` centralizes everything that decides whether the
current invocation is interactive. The helper runs exactly once in
`root.PersistentPreRunE`, so every event in the session inherits the same
`non_interactive` property without each command needing to parse flags on its own.

- `DATAROBOT_CLI_NON_INTERACTIVE=true` forces `non_interactive=true`
- Any parsed `--yes` flag (including short `-y`) flips the flag for that session
- `--force-interactive` is not an interaction signal: it only forces the setup wizard
  to re-run (via `state.HasCompletedDotenvSetup`) and does not override `--yes` or the
  env var for confirmation prompts
- Commands can reuse the same logic directly via `cli.IsNonInteractive(cmd)`

To expose another universal flag or environment override, extend
`telemetry.StampInteractionMode` (and update its tests) rather than touching
individual commands. **TODO:** when the next global automation flag is added,
document its behavior here and wire it through the helper alongside `--yes`.

### Reading RunE results in a PropExtractor

Because [`cobra.OnFinalize`](https://pkg.go.dev/github.com/spf13/cobra#OnFinalize) fires after RunE on both success and error paths (unlike [`PersistentPostRunE`](https://www.jvt.me/posts/2024/11/29/gotcha-cobra-persistentpostrune/)), a PropExtractor can read data that RunE computed. The recommended pattern is to declare closure variables in `Cmd()` that RunE writes and the PropExtractor reads:

```go
func Cmd() *cobra.Command {
    var (
        checkResult    tools.CheckResult
        installSuccess []string
        installError   string
    )

    cmd := &cobra.Command{
        RunE: func(cmd *cobra.Command, _ []string) error {
            checkResult = tools.CheckPrerequisites()       // written by RunE
            installSuccess, err = dependencies.Install(…)
            if err != nil {
                installError = err.Error()
                return err
            }
            return nil
        },
    }

    telemetry.TrackWith(cmd, func(_ *cobra.Command, _ []string) map[string]any {
        return map[string]any{                             // read by PropExtractor
            "validation_violations": checkResult.ValidationViolations,
            "install_success":       installSuccess,
            "install_error":         installError,
        }
    })

    return cmd
}
```

Both RunE and the PropExtractor close over the same local variables. No context keys or package-level variables are needed.

## How to add telemetry to a new command

### 1. Decide what (if anything) to extract

Inspect the command's flags and args. Decide which (if any) should be exposed as event properties.

### 2. Wire the command at construction

Find the function (or `init`) that builds the cobra command and add a `telemetry.Track*` call before returning.

**Simple command, no extra properties:**

```go
import "github.com/datarobot/cli/internal/telemetry"

func Cmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "foo",
        Short: "Do foo",
        // ...
    }

    telemetry.Track(cmd)

    return cmd
}
```

**Command that contributes properties from positional args:**

```go
telemetry.TrackWith(cmd, func(_ *cobra.Command, args []string) map[string]any {
    return map[string]any{
        "component_name": telemetry.FirstArg(args),
    }
})
```

**Command that contributes a property from a flag:**

```go
telemetry.TrackWith(cmd, func(c *cobra.Command, args []string) map[string]any {
    ver, _ := c.Flags().GetString("version")

    return map[string]any{
        "plugin_name":    telemetry.FirstArg(args),
        "plugin_version": ver,
    }
})
```

### 3. Add the command's path to the wiring test

IMPORTANT: Edit `cmd/telemetry_wiring_test.go` and add the new `cmd.CommandPath()` to `expectedTrackedCommands`. The test will fail loudly if anyone later removes the wiring.

### 4. Test it

```bash
task test
task lint
```

Run the CLI with telemetry disabled and check the debug log to see your
event:

```bash
dr foo --debug --disable-telemetry
# .dr-tui-debug.log will include "Telemetry event (dry-run)" entries
```

## Plugin Commands

Plugin commands are discovered at runtime by `cmd/plugin/discovery.go::createPluginCommand`, which calls `telemetry.TrackPlugin(cmd, manifest.Version)`. This:

- Sets the `"telemetry"` annotation so `EventFor` will fire an event.
- Sets the `"telemetry:plugin"` annotation so `IsPluginCommand` returns true, which causes the root to stamp `command_kind = "plugin"` on the common properties.
- Registers an extractor that adds `plugin_version` to the event.

The event type is `cmd.CommandPath()` — for example `dr assist`. There is no longer a synthetic `"dr plugin execute"` event.

## SDK log routing

The Amplitude SDK emits its own internal logs (HTTP responses, client lifecycle, etc.) via a custom logger adapter in `amplitudeLogger`. All Amplitude SDK log entries are prefixed with `[amplitude]` for traceability in debug log files.

The adapter demotes Amplitude's INFO-level logs (e.g. `HTTP response code`, `HTTP response body`) to DEBUG when the app's log level is above INFO. This keeps them off stderr by default while still capturing them in the debug log file (see [Logging](../user-guide/configuration.md#logging)).

| CLI flags   | Amplitude INFO appears as | Visible on stderr? |
|-------------|---------------------------|--------------------|
| *(default)* | DEBUG                     | No                 |
| `--verbose` | INFO                      | Yes                |
| `--debug`   | INFO                      | Yes                |

WARN and ERROR messages from the SDK always pass through at their original level.

## Testing

Run the telemetry test suite:

```bash
task test -- ./internal/telemetry/... ./cmd/...
```

Key tests:

- `internal/telemetry/wire_test.go` — exercises `Track`, `TrackWith`, `TrackPlugin`, `EventFor`, `IsPluginCommand`, `FirstArg`.
- `internal/telemetry/properties_test.go` — exercises common properties including `command_kind`.
- `cmd/telemetry_wiring_test.go` — verifies that every expected core command path is wired in the static command tree, and separately walks the `dr workload`, `dr artifact` and `dr pipeline` groups asserting every leaf under them is wired, listed or not.

## Maintenance checklist

- **Renaming a command?** The event type follows `cmd.CommandPath()` automatically, but you must update `expectedTrackedCommands` in `cmd/telemetry_wiring_test.go`.
- **Removing a command?** Remove its `expectedTrackedCommands` entry.
- **Adding a leaf under `dr workload`, `dr artifact` or `dr pipeline`?** Wire it with `Track` / `TrackWith` and add it to `expectedTrackedCommands`. The subtree walk fails on an unwired leaf whether or not the list mentions it, which is what a list alone cannot check.
- **Changing event properties?** Update the closure passed to `TrackWith`.
