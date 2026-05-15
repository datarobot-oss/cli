# Telemetry Event Wiring

This document explains how telemetry events are wired to CLI commands and how to add telemetry for new commands.

## Overview

Telemetry events are wired declaratively at command-construction time using a small API exported by `internal/telemetry`:

| Helper                              | Use when…                                                                       |
| ----------------------------------- | ------------------------------------------------------------------------------- |
| `telemetry.Track(cmd)`              | The command needs no extra event properties beyond the common ones.             |
| `telemetry.TrackWith(cmd, extract)` | The command needs dynamic event properties from flags or args at firing time.   |
| `telemetry.TrackPlugin(cmd, ver)`   | The command comes from a plugin. Adds `plugin_version` and sets `command_kind`. |

Each helper sets a `"telemetry"` annotation on the cobra command. The root
command's `PersistentPreRunE` calls `telemetry.EventFor(cmd, args)` which
returns an Amplitude event with `EventType == cmd.CommandPath()` and any
properties the registered extractor produced.

This approach ensures:

- **Local**: Wiring lives next to the command it tracks, not in a central map.
- **Safe**: Events fire in `PersistentPreRunE` before commands that may call `os.Exit` directly.
- **Extensible**: Adding a new event requires one call where the command is built.
- **Self-documenting**: The cobra command itself carries its telemetry intent.

## Architecture

```text
User invokes command
    ↓
Cobra parses flags
    ↓
PersistentPreRunE (root.go)
    ├─ Initialize CommonProperties (session ID, user ID, env, ...)
    ├─ Stamp props.CommandKind = "core" or "plugin"
    │   based on telemetry.IsPluginCommand(cmd)
    ├─ Build telemetry.Client
    └─ telemetry.EventFor(cmd, args) → if tracked, client.Track(event)
    ↓
RunE / Run executes (may call os.Exit)
    ↓
PersistentPostRunE (root.go)
    └─ Flush telemetry (3-second timeout)
```

`Client.Track` merges the `CommonProperties` map (which now includes
`command_kind`) into every event before sending.

## Common Properties

Collected once per CLI invocation in `telemetry.CollectCommonProperties`:

| Property             | Source                                                      |
| -------------------- | ----------------------------------------------------------- |
| `session_id`         | UUID v4 generated per process                               |
| `user_id`            | `drapi.GetAccountInfo` → `UID`                              |
| `cli_version`        | `internal/version.Version` (ldflags)                        |
| `install_method`     | `telemetry.InstallMethod` (ldflags; defaults to `"source"`) |
| `os_info`            | `runtime.GOOS + "/" + runtime.GOARCH`                       |
| `environment`        | Derived from `endpoint` config (US / EU / JP / custom)      |
| `datarobot_instance` | Base URL of configured DataRobot instance                   |
| `command_kind`       | `"core"` or `"plugin"` — set by the root after dispatch     |

## How to Add Telemetry to a New Command

### 1. Decide what (if anything) to extract

Inspect the command's flags and args. Decide which (if any) should be
exposed as event properties.

### 2. Wire the command at construction

Find the function (or `init`) that builds the cobra command and add a
`telemetry.Track*` call before returning.

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

IMPORTANT: Edit `cmd/telemetry_wiring_test.go` and add the new `cmd.CommandPath()`
to `expectedTrackedCommands`. The test will fail loudly if anyone later removes
the wiring.

### 4. Test it

```bash
task test
task lint
```

Run the CLI with telemetry disabled (the dev default) and check the
debug log to see your event:

```bash
dr foo --debug
# .dr-tui-debug.log will include "Telemetry event (dry-run)" entries
```

## Plugin Commands

Plugin commands are discovered at runtime by
`cmd/plugin/discovery.go::createPluginCommand`, which calls
`telemetry.TrackPlugin(cmd, manifest.Version)`. This:

- Sets the `"telemetry"` annotation so `EventFor` will fire an event.
- Sets the `"telemetry:plugin"` annotation so `IsPluginCommand` returns
  true, which causes the root to stamp `command_kind = "plugin"` on the
  common properties.
- Registers an extractor that adds `plugin_version` to the event.

The event type is `cmd.CommandPath()` — for example `dr assist`. There
is no longer a synthetic `"dr plugin execute"` event.

## Testing

Run the telemetry test suite:

```bash
task test -- ./internal/telemetry/... ./cmd/...
```

Key tests:

- `internal/telemetry/wire_test.go` — exercises `Track`, `TrackWith`,
  `TrackPlugin`, `EventFor`, `IsPluginCommand`, `FirstArg`.
- `internal/telemetry/properties_test.go` — exercises common properties
  including `command_kind`.
- `cmd/telemetry_wiring_test.go` — verifies that every expected core
  command path is wired in the static command tree.

## Cleanup Checklist

- **Renaming a command?** The event type follows `cmd.CommandPath()`
  automatically, but you must update `expectedTrackedCommands` in
  `cmd/telemetry_wiring_test.go`.
- **Removing a command?** Remove its `expectedTrackedCommands` entry.
- **Changing event properties?** Update the closure passed to `TrackWith`.
