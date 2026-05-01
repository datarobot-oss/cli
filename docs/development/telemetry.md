# Telemetry

The CLI collects anonymous usage analytics via [Amplitude](https://amplitude.com/) to help the DataRobot team understand how the tool is used.

## How it works

Telemetry is implemented in `internal/telemetry/`. On each CLI invocation a `Client` is created with a set of `CommonProperties`, events are queued via `Client.Track()`, and the queue is flushed at process exit via `Client.Flush()`.

When telemetry is disabled or the Amplitude API key is absent (all dev builds), every operation is a safe no-op — events are logged to the debug logger instead of being sent over the network.

## Opting out

Users can disable telemetry in three ways, in order of precedence:

| Method | How |
|---|---|
| Flag | `dr --disable-telemetry <command>` |
| Environment variable | `DATAROBOT_CLI_DISABLE_TELEMETRY=true` |
| Config file | `disable-telemetry: true` in `drconfig.yaml` |

## Device ID

Amplitude requires a `device_id` or `user_id` on every event. The CLI uses a stable device identifier obtained in this order:

1. **OS-provided machine ID** — via [`github.com/denisbrodbeck/machineid`](https://github.com/denisbrodbeck/machineid), which reads:
   - `IOPlatformUUID` on macOS
   - `/etc/machine-id` on Linux
   - `HKLM\SOFTWARE\Microsoft\Cryptography\MachineGuid` on Windows

   The raw value is HMAC-SHA256'd with the app ID `"dr"` before use, so the actual system identifier is never sent to Amplitude.

2. **Persisted random UUID** — if the OS identifier is unavailable, a random UUID is generated and written to `~/.config/datarobot/device_id` (respects `$XDG_CONFIG_HOME`). The same value is reused on subsequent invocations.

3. **Session-scoped fallback** — if the config directory is also inaccessible, a fresh ID prefixed with `"fallback-"` is generated for that session only.

## Adding a new event

All event constructors live in `internal/telemetry/events.go`. Add a new function following the existing pattern:

```go
func NewMyCommandEvent(param string) types.Event {
    return types.Event{
        EventType: "dr my command",
        EventProperties: map[string]any{
            "param": param,
        },
    }
}
```

Then call it from the command's `RunE` or `PersistentPostRunE`:

```go
telemetryClient.Track(telemetry.NewMyCommandEvent(param))
```

Common properties (`session_id`, `device_id`, `cli_version`, `os_info`, etc.) are merged automatically by `Client.Track()` — do not add them to individual event constructors.

## Common properties

The following are attached to every event:

### Top-level event fields

| Field | Source |
|---|---|
| `user_id` | DataRobot user ID from the API (empty if unauthenticated) |
| `device_id` | OS machine ID (hashed) or persisted UUID — see [Device ID](#device-id) above |

### Event properties

| Property | Source |
|---|---|
| `session_id` | Random UUID generated once per process invocation |
| `user_id` | Same as the top-level `user_id` field |
| `cli_version` | Set at build time via ldflags |
| `install_method` | Set at build time via ldflags (`release`, `source`, etc.) |
| `os_info` | `runtime.GOOS/runtime.GOARCH` |
| `environment` | `US`, `EU`, `JP`, or `custom` — derived from endpoint URL |
| `datarobot_instance` | Base URL of the configured DataRobot instance |
| `template_name` | Best-effort from `.datarobot/answers/` in the current repo |

## Dev builds

`AmplitudeAPIKey` is empty in dev builds (it is injected via ldflags in release builds only). When the key is empty, `IsEnabled()` returns `false` and all `Track` calls log to the debug logger. Run with `--debug` to see telemetry events in `.dr-tui-debug.log`.
