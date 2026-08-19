# Feature Flags

Feature flags allow commands to be hidden from users until they're ready for release. This is useful for developing new features behind a feature gate and only enabling them when they're ready.

## Purpose

Feature flags in this CLI serve to:

- Hide unreleased or experimental commands from end users
- Allow development teams to work on new features without exposing them in help text
- Provide a clean migration path for commands (keep them hidden until GA, then remove the flag)

**Security Note:** Feature flags are **not** an access control mechanism. Any user with shell access can enable a disabled feature via environment variable. They exist to prevent casual discovery of unfinished features.

## How It Works

A command declares a feature gate by calling `features.SetGate`, which records the gate name in the command's Cobra `Annotations` map under `features.AnnotationKey`. During CLI initialization, any command whose gate is not enabled is filtered out before it is ever registered in the command tree.

```go
cmd := &cobra.Command{Use: "pipeline"}

features.SetGate(cmd, "pipeline")
```

Call `SetGate` rather than assigning `Annotations` yourself: it allocates the map when needed and preserves annotations written by other subsystems, such as the `telemetry` annotation set by `telemetry.Track`.

When a command is filtered out, it:
- Does not appear in `dr --help`
- Returns "unknown command" error if invoked directly
- Is implicitly unavailable for subcommands if its parent is removed

To list every gate currently live in the tree, run `grep -rn 'features.SetGate' cmd/`.

## Enabling a Feature

### Environment Variable (Primary Method)

Set the env var `DATAROBOT_CLI_FEATURE_<FEATURE_NAME>=true` or `=1`:

```bash
DATAROBOT_CLI_FEATURE_PIPELINE=true dr pipeline --help
```

Feature names are converted from lowercase with hyphens to uppercase with underscores:
- `pipeline` → `DATAROBOT_CLI_FEATURE_PIPELINE`
- `my-feature` → `DATAROBOT_CLI_FEATURE_MY_FEATURE`

### Config File (Future)

Config file support (e.g., `drconfig.yaml`) is not yet implemented. See the TODO in `internal/features/features.go`. Currently, only environment variables work because feature gating happens during command registration (`init()`), before Viper configuration is loaded.

## Adding a Feature-Gated Command

1. **Create the command package** (e.g., `cmd/pipeline/cmd.go`):

```go
package pipeline

import (
    "github.com/datarobot/cli/internal/features"
    "github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:     "pipeline",
        GroupID: "core",
        Short:   "Pipelines API management commands",
    }

    features.SetGate(cmd, "pipeline")

    cmd.AddCommand(
        // ... subcommands ...
    )

    return cmd
}
```

2. **Register in `cmd/root.go`** alongside other commands:

```go
RootCmd.AddCommand(
    // ... existing commands ...
    pipeline.Cmd(),
    // ...
)
```

3. **Assert the gate in the package's own test**, as `TestCmd_FeatureGate` does in `cmd/pipeline/cmd_test.go`. Dropping the gate later should then be a deliberate, reviewed change rather than an accident.

That's it. `RootCmd` is a `cli.CommandAdder`, so `AddCommand` automatically filters any gated command whose feature is disabled.

## Gating Subcommands

To gate a subcommand, the **parent** command must also use `cli.CommandAdder` so that filtering applies when its children are registered:

```go
func Cmd() *cobra.Command {
    parent := &cobra.Command{Use: "parent"}

    features.SetGate(gatedSubCmd, "my-feature")

    adder := &cli.CommandAdder{Command: parent}
    adder.AddCommand(
        ungatedSubCmd,
        gatedSubCmd, // filtered at registration time if feature is off
    )

    return parent
}
```

When the parent command itself is gated and disabled, child commands are implicitly unavailable because the parent is never added to the tree.

## Removing a Feature Gate (GA Release)

When a feature is ready for general availability:

1. Delete the `features.SetGate` call from every command carrying the gate. One gate name can cover several command trees, so run `grep -rn 'features.SetGate' cmd/` and confirm you have found them all before you stop.
2. Delete the now-unused `github.com/datarobot/cli/internal/features` import from each file you touched. Go does not compile with an unused import, so this is not optional cleanup.
3. Update the tests that encoded the gate: drop the package's gate assertion, and invert any root-level test asserting the command is absent by default so that it asserts presence instead.
4. Update the docs that tell people to export the env var: the note at the top of `docs/commands/<command>.md`, and the feature-gated entries in `docs/commands/README.md`.
5. Leave `internal/features` and `cli.CommandAdder` alone. They stay live for the remaining gates, and `DATAROBOT_CLI_FEATURE_<NAME>` simply becomes a no-op for the released feature, so scripts that still set it keep working.

```go
// Before (gated)
import (
    "github.com/datarobot/cli/cmd/pipeline/create"
    "github.com/datarobot/cli/internal/features"
    "github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:     "pipeline",
        GroupID: "core",
        Short:   "Pipelines API management commands",
    }

    features.SetGate(cmd, "pipeline")

    cmd.AddCommand(create.Cmd())

    return cmd
}
```

```go
// After (GA)
import (
    "github.com/datarobot/cli/cmd/pipeline/create"
    "github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:     "pipeline",
        GroupID: "core",
        Short:   "Pipelines API management commands",
    }

    cmd.AddCommand(create.Cmd())

    return cmd
}
```

## Testing

Feature flags are tested via:

- Unit tests in `internal/features/features_test.go` covering `Enabled`, `SetProvider`, and `SetGate`
- Unit tests in `internal/cli/command_test.go` covering `CommandAdder` filtering, including nested gated subcommands
- A per-command gate assertion in the gated package's own test, as `TestCmd_FeatureGate` does in `cmd/pipeline/cmd_test.go`
- Manual testing with env vars: `DATAROBOT_CLI_FEATURE_<NAME>=true dr <command>`

## Limitations

- **Config file support not yet implemented**: only env vars work (TODO in code)
- Feature flags are evaluated at CLI startup, not at runtime
- Enabling a feature via env var affects the entire CLI session
- Disabling a feature is not supported (only enabled or absent)
