# Feature Flags

Feature flags allow commands to be hidden from users until they're ready for release. This is useful for developing new features behind a feature gate and only enabling them when they're ready.

## Purpose

Feature flags in this CLI serve to:

- Hide unreleased or experimental commands from end users
- Allow development teams to work on new features without exposing them in help text
- Provide a clean migration path for commands (keep them hidden until GA, then remove the flag)

**Security Note:** Feature flags are **not** an access control mechanism. Any user with shell access can enable a disabled feature via environment variable. They exist to prevent casual discovery of unfinished features.

## How It Works

Commands can declare a feature gate using Cobra's built-in `Annotations` map. During CLI initialization, any command with a disabled feature annotation is removed from the command tree.

```go
Annotations: map[string]string{
    features.AnnotationKey: "feature-name",
}
```

When a command is removed, it:
- Does not appear in `dr --help`
- Returns "unknown command" error if invoked directly
- Is implicitly unavailable for subcommands if its parent is removed

## Enabling a Feature

### Environment Variable (Primary Method)

Set the env var `DATAROBOT_CLI_FEATURE_<FEATURE_NAME>=true` or `=1`:

```bash
DATAROBOT_CLI_FEATURE_WORKLOAD=true dr workload --help
```

Feature names are converted from lowercase with hyphens to uppercase with underscores:
- `workload` → `DATAROBOT_CLI_FEATURE_WORKLOAD`
- `my-feature` → `DATAROBOT_CLI_FEATURE_MY_FEATURE`

### Config File (Future)

Config file support (e.g., `drconfig.yaml`) is not yet implemented. See the TODO in `internal/features/features.go`. Currently, only environment variables work because feature gating happens during command registration (`init()`), before Viper configuration is loaded.

## Adding a Feature-Gated Command

1. **Create the command package** (e.g., `cmd/workload/cmd.go`):

```go
package workload

import (
    "github.com/datarobot/cli/internal/features"
    "github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
    return &cobra.Command{
        Use:     "workload",
        GroupID: "core",
        Short:   "Workload management commands",
        Annotations: map[string]string{
            features.AnnotationKey: "workload",
        },
    }
}
```

2. **Register in `cmd/root.go`** alongside other commands:

```go
RootCmd.AddCommand(
    // ... existing commands ...
    workload.Cmd(),
    // ...
)
```

That's it. `RootCmd` is a `cli.CommandAdder`, so `AddCommand` automatically filters any gated command whose feature is disabled.

## Gating Subcommands

To gate a subcommand, the **parent** command must also use `cli.CommandAdder` so that filtering applies when its children are registered:

```go
func Cmd() *cobra.Command {
    parent := &cobra.Command{Use: "parent"}

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

1. Delete the `Annotations` map from the command
2. No other changes needed; feature is now permanently available

```go
// Before (gated)
&cobra.Command{
    Use:     "workload",
    Annotations: map[string]string{
        features.AnnotationKey: "workload",
    },
}

// After (GA)
&cobra.Command{
    Use:     "workload",
    // No annotations
}
```

## Testing

Feature flags are tested via:

- Unit tests in `internal/features/features_test.go` covering `Enabled()`
- Unit tests in `internal/cli/command_test.go` covering `CommandAdder`
- Integration tests in `cmd/root_test.go` verifying runtime behavior
- Manual testing with env vars: `DATAROBOT_CLI_FEATURE_<NAME>=true dr <command>`

## Limitations

- **Config file support not yet implemented** — only env vars work (TODO in code)
- Feature flags are evaluated at CLI startup, not at runtime
- Enabling a feature via env var affects the entire CLI session
- Disabling a feature is not supported (only enabled or absent)
