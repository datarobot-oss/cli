# Telemetry

The CLI collects anonymous usage analytics via [Amplitude](https://amplitude.com/) to help the DataRobot team understand how the tool is used. Telemetry is implemented in `internal/telemetry/`. On each CLI invocation a `Client` is created with a set of `CommonProperties`, events are queued via `Client.Track()`, and the queue is flushed at process exit via `Client.Flush()`.

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

## User ID

When the user is authenticated, the CLI sends a real DataRobot `uid` as the top-level Amplitude `user_id` field. If the user is unauthenticated (no API token, invalid token, or network failure with no valid cache), the field is left empty and Amplitude falls back to `device_id`-only anonymous tracking.

The `uid` is fetched from `GET /api/v2/account/info/`, which returns an `AccountInfo` response containing the user's unique identifier. The `uid` is stable per DataRobot instance and is not PII (email is deliberately excluded from telemetry to avoid transmitting personally identifiable information).

### Caching

To avoid an API call on every CLI invocation, the `uid` is cached to disk alongside `device_id` and `drconfig.yaml`:

- **Cache file**: `$CONFIG_DIR/datarobot/user_id` (respects `$XDG_CONFIG_HOME`)
- **File permissions**: `0600` (owner read/write only), consistent with `device_id` and `drconfig.yaml`
- **Cache format** (JSON):

  ```json
  {"uid":"...","endpoint":"https://app.datarobot.com","token_fingerprint":"sha256hex"}
  ```

  - `uid` — the DataRobot user identifier
  - `endpoint` — the scheme+host of the DataRobot instance (e.g., `https://app.datarobot.com`)
  - `token_fingerprint` — SHA-256 hex digest of the current API token

### Cache validation and invalidation

On subsequent invocations, when no fresh API `uid` is available, the cache is validated against both the current endpoint and the current token fingerprint:

- **Endpoint match**: the cached `endpoint` must equal the current `viperx.GetString(config.DataRobotURL)` (scheme+host only)
- **Token fingerprint match**: the cached `token_fingerprint` must equal the SHA-256 hex of the current API token

If either check fails, the cache is treated as stale and the `user_id` is left empty (anonymous tracking). This ensures correct behavior in shared environments (e.g., Codespaces) where two users may authenticate sequentially with different tokens — the token fingerprint prevents incorrectly attributing User B's activity to User A's cached `uid`.

### Behavior summary

| Scenario | `user_id` behavior |
|---|---|
| Authenticated, API succeeds | `uid` from API, cached to disk |
| Authenticated, cache hit (same endpoint + token) | Cached `uid` (no API call) |
| Endpoint changed | Re-fetch from API, update cache |
| Token changed (rotation / new user) | Re-fetch from API, update cache |
| No API token / invalid token | Empty `user_id`, anonymous tracking |
| Network error, same endpoint + token | Return cached `uid` |
| Network error, endpoint/token changed | Empty `user_id`, anonymous tracking |

## Common Properties

The following are attached to every event:

### Top-level event fields

These map to Amplitude's built-in fields and power native segmentation (version filters, OS breakdowns, language charts, etc.).

| Field | Source |
|---|---|
| `user_id` | DataRobot `uid` from `GET /api/v2/account/info/`, cached to disk with endpoint + token fingerprint validation; empty (anonymous) if unauthenticated or cache miss — see [User ID](#user-id) |
| `device_id` | OS machine ID (hashed) or persisted UUID — see [Device ID](#device-id) above |
| `session_id` | Unix millisecond timestamp generated once per process invocation — Amplitude uses this as the built-in Session ID for session-based analysis |
| `app_version` | CLI version set at build time via ldflags |
| `platform` | Always `"CLI"` |
| `os_name` | OS name (e.g. `"macOS"`) |
| `os_version` | OS version (e.g. `"15.7.5"`) |
| `language` | User locale tag (e.g. `"en-US"`), via `go-locale`; Amplitude maps to a display language name |
| `ip` | Always `"$remote"` — Amplitude resolves location server-side |

### Event properties

| Property | Source |
| --- | --- |
| `install_method` | Set at build time via ldflags (`release`, `source`, etc.) |
| `os_arch` | CPU architecture from `runtime.GOARCH` |
| `go_version` | Go runtime version (e.g. `go1.26.3`) from `runtime.Version()` |
| `environment` | `US`, `EU`, `JP`, or `custom` — derived from endpoint URL |
| `datarobot_instance` | Base URL of the configured DataRobot instance |
| `template_name` | Best-effort from `.datarobot/answers/` in the current repo |
| `command_kind` | `"core"` or `"plugin"` — automatically set by the root command dispatcher |

## Event Wiring

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

### Execution flow

```
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

## How to add telemetry to a new command

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

The event type is `cmd.CommandPath()` — for example `dr assist`. There is no longer a synthetic `"dr plugin execute"` event.

## Dev builds

`AmplitudeAPIKey` is empty in dev builds (it is injected via ldflags in release builds only). When the key is empty, `IsEnabled()` returns `false` and all `Track` calls log to the debug logger.

## SDK log routing

The Amplitude SDK emits its own internal logs (HTTP responses, client lifecycle, etc.) via a custom logger adapter in `amplitudeLogger`. All Amplitude SDK log entries are prefixed with `[amplitude]` for traceability in debug log files.

The adapter demotes Amplitude's INFO-level logs (e.g. `HTTP response code`, `HTTP response body`) to DEBUG when the app's log level is above INFO. This keeps them off stderr by default while still capturing them in the debug log file (see [Logging](../../user-guide/configuration.md#logging)).

| CLI flags | Amplitude INFO appears as | Visible on stderr? |
|---|---|---|
| *(default)* | DEBUG | No |
| `--verbose` | INFO | Yes |
| `--debug` | INFO | Yes |

WARN and ERROR messages from the SDK always pass through at their original level.

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

## Maintenance checklist

- **Renaming a command?** The event type follows `cmd.CommandPath()` automatically, but you must update `expectedTrackedCommands` in `cmd/telemetry_wiring_test.go`.
- **Removing a command?** Remove its `expectedTrackedCommands` entry.
- **Changing event properties?** Update the closure passed to `TrackWith`.
