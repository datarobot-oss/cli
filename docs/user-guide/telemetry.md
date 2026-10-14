# Telemetry

The CLI collects usage analytics linked to your DataRobot user ID via [Amplitude](https://amplitude.com/) to help the DataRobot team understand how the tool is used. Telemetry is an optional feature that can be turned off at any time (see [Configuring and disabling telemetry](#configuring-and-disabling-telemetry)). For how telemetry is implemented and how to add events, see the [contributor guide](../development/telemetry.md).

All telemetry data sent over the network is stored in the USA. When telemetry is disabled, every operation is a safe no-op — events are logged to the debug logger instead of being sent over the network.

## Configuring and disabling telemetry

Telemetry is enabled by default. Users can disable it at any time via any of the three methods below (listed in order of precedence — higher precedence wins):

| Method               | How                                          |
|----------------------|----------------------------------------------|
| Flag                 | `dr --disable-telemetry <command>`           |
| Environment variable | `DATAROBOT_CLI_DISABLE_TELEMETRY=true`       |
| Config file          | `disable-telemetry: true` in `drconfig.yaml` |

To disable telemetry permanently, add the key to your config file:

```yaml
# ~/.config/datarobot/drconfig.yaml
disable-telemetry: true
```

The `--disable-telemetry` flag is a [universal flag](../development/flags.md) — it is forwarded to plugin subprocesses as the `DATAROBOT_CLI_DISABLE_TELEMETRY` environment variable, so plugin commands respect the same setting.

When telemetry is disabled, events are logged to the debug logger (visible with `--debug`) instead of being sent over the network to Amplitude.

## Network endpoints

Telemetry makes outbound HTTPS requests to two services. In network-restricted environments (corporate proxies, firewalls, air-gapped CI), the following hosts must be allowlisted for telemetry to function:

| Host                                                       | Purpose                                                                                                       | Port |
|------------------------------------------------------------|---------------------------------------------------------------------------------------------------------------|------|
| `api2.amplitude.com`                                       | Amplitude HTTP API (US zone) — event ingestion                                                                | 443  |
| *configured DataRobot endpoint* (e.g. `app.datarobot.com`) | `GET /api/v2/account/info/` — fetches the `user_id`, `organization_id`, and `tenant_id` for event attribution | 443  |

The DataRobot endpoint call is only made when the user is authenticated and the cached account info is stale or absent (see [User ID](#user-id)). If that call fails due to network restrictions, telemetry falls back to `device_id`-only tracking — the CLI does not error.

No other hosts are contacted by the telemetry subsystem.

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

When the user is authenticated, the CLI sends a real DataRobot `uid` as the top-level Amplitude `user_id` field. If the user is unauthenticated (no API token, invalid token, or network failure with no valid cache), the field is left empty and Amplitude falls back to `device_id`-only tracking.

The `uid` is fetched from `GET /api/v2/account/info/`, which returns an `AccountInfo` response containing the user's unique identifier, organization ID, and tenant ID. The `uid` is stable per DataRobot instance and uniquely identifies your DataRobot user account. Email is deliberately excluded from telemetry payloads. The organization ID and tenant ID are also cached and sent as event properties (see [Common Properties](../development/telemetry.md#common-properties)).

### Caching

To avoid an API call on every CLI invocation, the account info (`uid`, `organization_id`, `tenant_id`) is cached to disk alongside `device_id` and `drconfig.yaml`:

- **Cache file**: `$CONFIG_DIR/datarobot/user_id` (respects `$XDG_CONFIG_HOME`)
- **File permissions**: `0600` (owner read/write only), consistent with `device_id` and `drconfig.yaml`
- **Cache format** (JSON):

  ```json
  {
    "uid": "...",
    "endpoint": "https://app.datarobot.com",
    "token_fingerprint": "sha256hex",
    "organization_id": "...",
    "tenant_id": "..."
  }
  ```

  - `uid` — the DataRobot user identifier
  - `endpoint` — the scheme+host of the DataRobot instance (e.g., `https://app.datarobot.com`)
  - `token_fingerprint` — SHA-256 hex digest of the current API token
  - `organization_id` — the DataRobot organization ID
  - `tenant_id` — the DataRobot tenant ID (may be empty for legacy/system accounts)

### Cache validation and invalidation

On subsequent invocations, when no fresh API response is available, the cache is validated against both the current endpoint and the current token fingerprint:

- **Endpoint match**: the cached `endpoint` must equal the current `config.GetBaseURL()` (scheme+host only)
- **Token fingerprint match**: the cached `token_fingerprint` must equal the SHA-256 hex of the current API token
- **Completeness check**: the cache must contain both a non-empty `uid` and `organization_id`. Cache files written by older CLI versions (before `organization_id`/`tenant_id` were added) are treated as partial and trigger a re-fetch. (`tenant_id` may legitimately be empty for legacy or system accounts, so it is not included in this check.)

If any check fails, the cache is treated as stale and a fresh API call is made. If the API call also fails (network error), the cached `uid` is still used as long as the endpoint and token fingerprint match — this preserves tracking in offline scenarios. If the endpoint or token changed and the API is unreachable, `user_id` is left empty (`device_id`-only tracking).

This ensures correct behavior in shared environments (e.g., Codespaces) where two users may authenticate sequentially with different tokens — the token fingerprint prevents incorrectly attributing User B's activity to User A's cached `uid`.

### Behavior summary

| Scenario                                                         | `user_id` / `organization_id` / `tenant_id` behavior  |
|------------------------------------------------------------------|-------------------------------------------------------|
| Authenticated, API succeeds                                      | `uid`, `org_id`, `tenant_id` from API, cached to disk |
| Authenticated, cache hit (same endpoint + token, complete cache) | Cached `uid`, `org_id`, `tenant_id` (no API call)     |
| Endpoint changed                                                 | Re-fetch from API, update cache                       |
| Token changed (rotation / new user)                              | Re-fetch from API, update cache                       |
| Partial cache (old CLI version, missing `org_id`)                | Re-fetch from API, update cache                       |
| No API token / invalid token                                     | Empty `user_id`, `device_id`-only tracking            |
| Network error, same endpoint + token, valid cache                | Return cached `uid`, `org_id`, `tenant_id`            |
| Network error, endpoint/token changed                            | Empty `user_id`, `device_id`-only tracking            |

