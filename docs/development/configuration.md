# Configuration & viper integration

This guide covers how the CLI loads, reads, and writes its persistent
configuration (`drconfig.yaml`), and the rules contributors must follow when
adding new flags or persisted config keys.

## Table of contents

- [The viperx wrapper](#the-viperx-wrapper)
- [Where config lives](#where-config-lives)
- [How values reach viper](#how-values-reach-viper)
- [Writing back to drconfig.yaml](#writing-back-to-drconfigyaml)
- [Rules for new flags](#rules-for-new-flags)
- [Rules for new env vars](#rules-for-new-env-vars)
- [Rules for new persisted keys](#rules-for-new-persisted-keys)
- [Common pitfalls](#common-pitfalls)

## The viperx wrapper

Outside of `internal/config/...`, **the only viper entry point is
`internal/config/viperx`**. Direct imports of `github.com/spf13/viper`
are blocked by `depguard` everywhere else in the tree.

```go
// good
import "github.com/datarobot/cli/internal/config/viperx"

if viperx.GetBool("debug") { /* ... */ }

// blocked by golangci-lint outside internal/config/**
import "github.com/spf13/viper"
```

`viperx` re-exports only the safe subset of viper's API. The following
symbols are **deliberately not** re-exported:

- `viper.WriteConfig`, `viper.SafeWriteConfig` — they serialize the entire
  `viper.AllSettings()` map, leaking transient flag state into
  `drconfig.yaml`. Use `config.UpdateConfigFile(...)` instead, which only
  writes keys in the `config.PersistableKeys` allowlist.
- `viper.BindPFlags(cmd.Flags())` — bulk-binds every subcommand flag into
  viper, with the same leakage problem. Use `viperx.BindPFlag` for
  individual persistent flags; read subcommand flags directly via
  `cmd.Flags().GetX(...)`.

If you need a viper symbol that is not currently re-exported, add it to
`internal/config/viperx/viperx.go` consciously and document why. New
additions should be reviewed for whether they expand the leakage surface.

## Where config lives

The CLI stores user configuration in a single YAML file:

- Default location: `$XDG_CONFIG_HOME/datarobot/drconfig.yaml`
  (falls back to `~/.config/datarobot/drconfig.yaml`)
- Override with `--config <path>` or `DATAROBOT_CLI_CONFIG=<path>`

Today this file holds connection credentials and a small set of sticky CLI
preferences. It is **not** a dumping ground for transient flag state.

## How values reach viper

Viper resolves a key from these sources in priority order:

1. Explicit `viperx.Set(key, value)` call (e.g. after a successful login)
2. Flag bound via `viperx.BindPFlag(key, flag)` (only persistent root flags
   today &mdash; see below)
3. Environment variable bound via `viperx.BindEnv(key, "DATAROBOT_…")`
   or auto-mapped via `viperx.SetEnvPrefix("DATAROBOT_CLI")`
4. The active named profile's own keys, merged into this layer by
   `config.ReadConfigFile` via `viper.MergeConfigMap` (see
   [Named profiles](#named-profiles) below) &mdash; shadows the default
   profile's values from the same layer
5. Value loaded from `drconfig.yaml` (the default/top-level profile)
6. Default registered via `viperx.SetDefault`

### Persistent root flags bound to viper

Only the persistent root flags registered in `cmd/root_factory.go`'s
`registerFlags` are bound explicitly with `viperx.BindPFlag`, in
`bindViperFlags`. We do **not** bulk-bind subcommand flags (and `viperx`
does not even expose a `BindPFlags` function), because that would slurp
every subcommand flag (such as `--yes`, `--if-needed`) into
`viper.AllSettings()` and risk leaking transient flag state into
`drconfig.yaml`.

`--output-format` is one of these root-bound flags. It is global and supports
`DATAROBOT_CLI_OUTPUT_FORMAT`, but it is still treated as transient and is not
written to `drconfig.yaml` (it is not in `config.PersistableKeys`).

### Subcommand flags

Read subcommand flag values directly from cobra:

```go
yesFlag, _ := cmd.Flags().GetBool("yes")
```

If a subcommand flag also needs an environment variable override, register
the env var only with `viperx.BindEnv(...)` and merge the two sources
explicitly in your handler:

```go
// cmd/dotenv/cmd.go
_ = viperx.BindEnv("yes", "DATAROBOT_CLI_NON_INTERACTIVE")

// In RunE:
yesFlag, _ := cmd.Flags().GetBool("yes")
yes := yesFlag || viperx.GetBool("yes")
```

This keeps the explicit `--yes` flag value out of `viper.AllSettings()`
while preserving env-var support.

## Writing back to drconfig.yaml

Use the allowlisted writer in `internal/config`:

```go
// Write all allowlisted keys that are currently set in viper:
config.UpdateConfigFile()

// Or write only specific keys (recommended when the call site knows
// exactly what changed):
config.UpdateConfigFile(config.DataRobotURL)
config.UpdateConfigFile(config.DataRobotAPIKey, config.DataRobotURL)
```

`UpdateConfigFile` reads the existing YAML, overlays only the allowlisted
keys (`config.PersistableKeys`), and writes the result back. Any other
viper state &mdash; including transient flags such as `--yes`, `--verbose`,
`--debug` &mdash; is intentionally dropped.

The wrappers in the auth package (`auth.WriteConfigFileSilent`,
`auth.WriteConfigFile`) call this writer under the hood.

### Named profiles

`--profile <name>` / `DATAROBOT_CLI_PROFILE` select a section under
`profiles:` in `drconfig.yaml` (see the
[user-facing docs](../user-guide/configuration.md#named-profiles)). The
mechanism lives in `internal/config/profile.go`:

- `config.ReadConfigFile` calls `applyProfile(name)`, which
  `viper.MergeConfigMap`s the profile's own keys into viper's **config**
  layer &mdash; the same layer `viper.ReadInConfig` populates from the file.
  Deliberately **not** `viperx.Set`: `Set` writes the override layer, which
  outranks flags and env, so a profile's `endpoint` would beat an explicit
  `DATAROBOT_CLI_ENDPOINT` instead of losing to it.
- Only `config.ProfileScopedKeys` (`endpoint`, `token`, `ca-cert`) may live
  under a profile; every other persistable key stays global at the top level,
  shared by all profiles. `ssl_verify` is deliberately excluded: nothing in
  the CLI reads it (TLS is driven by `--ca-cert` and
  `--skip-certificate-check`), so it stays a global passthrough value that
  `UpdateConfigFile` preserves.
- `endpoint`/`token` merge atomically: if a profile defines either one, both
  are merged (substituting `""` for the one it omits), so a profile can
  never end up pairing its own endpoint with the default profile's token.

On the write side, `UpdateConfigFile`'s `PersistableKeys` allowlist is
unchanged &mdash; `profileDestPath(key)` in `internal/config/write.go` maps an
allowlisted key to `profiles.<name>.<key>` instead of `<key>` when a
profile is active. A **bare** sweep (`UpdateConfigFile()` with no explicit
keys) additionally restricts profile-scoped candidates to keys the
profile's own section already defines (`candidateKeys`) &mdash; otherwise it
would copy every value the profile currently inherits from the default
profile (including its endpoint/token) permanently into the profile's
section. Call sites that already pass explicit keys (e.g.
`auth.WriteConfigFileSilent` passing `endpoint`, `token`) are unaffected by
that restriction, which is what lets a brand-new profile's section be
created on its first write.

## Rules for new flags

When adding a new flag, decide which category it falls into:

| Category                                                  | Bind to viper?  | Persist to drconfig.yaml?            |
| --------------------------------------------------------- | --------------- | ------------------------------------ |
| Transient subcommand flag (e.g. `--yes`, `--all`)          | No              | No                                   |
| Global transient root flag (e.g. `--output-format`)        | Yes (root only) | No                                   |
| Sticky preference (e.g. `--ca-cert`)                        | Yes (root only) | Yes &mdash; add to `PersistableKeys` |
| Connection credential (e.g. `--token`)                     | Yes             | Yes                                  |

For transient subcommand flags:

- Define with `cmd.Flags().Bool(...)`
- Read with `cmd.Flags().GetBool(...)`
- Do **not** call `viperx.BindPFlag(...)`

For global transient root flags:

- Register on root with `PersistentFlags()`
- Bind only on root via `viperx.BindPFlag(...)`
- Keep the key out of `config.PersistableKeys`

## Rules for new env vars

`viperx.AutomaticEnv()` with prefix `DATAROBOT_CLI` is enabled in
`defaultConfigInitializer` (`cmd/root_factory.go`), so any key you
`viperx.Get` will already check `DATAROBOT_CLI_<KEY>` (with `-` replaced by
`_`).

For env vars that should map to a different name (e.g.
`DATAROBOT_CLI_NON_INTERACTIVE` → key `yes`), use `viperx.BindEnv` and
read the merged value as shown in [Subcommand flags](#subcommand-flags).

## Rules for new persisted keys

To make a key writable to `drconfig.yaml`:

1. Add the key to `PersistableKeys` in `internal/config/write.go`
2. Update its production write call sites to pass the key explicitly:
   `config.UpdateConfigFile("my-new-key")`
3. Decide whether the key is per-profile or global (see
   [Named profiles](#named-profiles) above). Per-profile-with-fallback is
   the default for anything that can legitimately differ between DataRobot
   installations (e.g. TLS settings); add it to `ProfileScopedKeys` in
   `internal/config/profile.go` if so. Leave it out (global) otherwise.
4. Add a regression test under `internal/auth/writeConfig_test.go` (or a
   dedicated test file) verifying the key round-trips correctly and that
   transient flags still do not leak.

Use viper dotted-path notation if the value is nested, e.g.
`"pulumi.config.passphrase"`. The writer handles nested map creation.

### Marking keys as sensitive

Sensitive keys (credentials, passphrases, tokens) must be redacted in debug output
to prevent accidental exposure of secrets in logs. To mark a key as sensitive:

1. Add the key to `sensitiveDebugKeys` in `internal/config/config.go`:

   ```go
   var sensitiveDebugKeys = map[string]struct{}{
       "token":                    {},
       "pulumi_config_passphrase": {},
   }
   ```

2. When `--debug` is enabled, the key will be redacted as `****` in console output
   from `DebugViperConfig()`.

Redaction is depth-recursive (`redactSettings`/`redactValue` in
`internal/config/config.go`): a key match is redacted no matter how deeply
nested, which is what keeps e.g. `profiles.eu-mtsaas.token` redacted the
same as top-level `token`.

## Common pitfalls

- **Don't import `github.com/spf13/viper` outside `internal/config/`.**
  `depguard` will reject it. Use `internal/config/viperx` instead.
- **Don't add `viper.WriteConfig` or `BindPFlags` to `viperx`.** Those
  omissions are deliberate. If you need to persist new state, extend
  `config.PersistableKeys` and call `config.UpdateConfigFile`.
- **Don't read transient flags through viper.** `viperx.GetBool("yes")`
  hides whether the value came from a flag, an env var, or a stale
  drconfig entry. Read flags directly from cobra and merge in env vars
  explicitly.
- **Don't write to `drconfig.yaml` from tests** without `viperx.Reset()`
  and a temp `XDG_CONFIG_HOME`. See `internal/auth/writeConfig_test.go`
  for the recommended test pattern.

## See also

- [Flag development guide](flags.md)
- [Authentication flow](authentication.md)
- [User configuration guide](../user-guide/configuration.md)
