# Flag development guide

This guide covers best practices for defining and managing **command-line flags** in the DataRobot CLI.

> **Note:** This guide is about command-line flags (e.g., `--verbose`, `--output file.txt`), not feature gates. For feature gates, see [Feature gates](feature-gates.md).

## Table of contents

- [Define flags clearly](#define-flags-clearly)
- [Global output-format pattern](#global-output-format-pattern)
- [Universal flags (forwarded to plugins)](#universal-flags-forwarded-to-plugins)
- [Mark flag groups](#mark-flag-groups)
- [Flag naming conventions](#flag-naming-conventions)
- [Examples from the codebase](#examples-from-the-codebase)

## Define flags clearly

Define flags at the beginning of your command function, using clear variable names:

```go
// cmd/mycommand/cmd.go
var (
    myFlag bool
    count int
)

func Cmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "mycommand",
        Short: "Description",
        RunE: func(cmd *cobra.Command, args []string) error {
            // Implementation
            return nil
        },
    }

    // Define flags (use singular flag names)
    cmd.Flags().BoolVar(&myFlag, "my-flag", false, "Description")
    cmd.Flags().IntVar(&count, "count", 0, "Description")

    return cmd
}
```

## Global output-format pattern

`--output-format` is implemented as a shared flag strategy:

- `cmd/root.go` registers it once as a persistent root flag via `outputformat.AddPersistentFlag(...)`
- The root flag is inherited by all subcommands, so no per-command registration is needed
- Commands that render text/JSON read the effective value via `outputformat.GetFormat(cmd)` to resolve format from CLI flags or environment variables

Use this pattern for output-aware commands:

```go
import "github.com/datarobot/cli/internal/outputformat"

func Cmd() *cobra.Command {
    cmd := &cobra.Command{
        Use: "list",
        RunE: func(cmd *cobra.Command, _ []string) error {
            // --output-format is a global persistent flag; no local AddFlag needed.
            format := outputformat.GetFormat(cmd)
            return render(format)
        },
    }

    return cmd
}
```

Why `GetFormat(cmd)` instead of reading the variable directly:

- It correctly handles local flag usage (`dr <cmd> --output-format json`)
- It correctly handles inherited root usage (`dr --output-format json <cmd>`)
- It respects environment variable overrides (`DATAROBOT_CLI_OUTPUT_FORMAT`)

### PrintJSONEnvelope

When outputting JSON, use `outputformat.PrintJSONEnvelope` to wrap data in a consistent envelope:

```go
if format == outputformat.OutputFormatJSON {
    outputs := make([]MyOutput, len(items))
    for i, item := range items {
        outputs[i] = MyOutput{/* ... */}
    }
    return outputformat.PrintJSONEnvelope(os.Stdout, "items", outputs)
}
```

The envelope format is `{"<key>": <data>}`, which ensures output is always a JSON object (never a bare array). This makes the output forward-compatible for adding metadata, pagination, or warnings without breaking callers.

## Universal flags (forwarded to plugins)

Some root flags must be forwarded to plugin subprocesses as `DATAROBOT_CLI_*` environment variables so plugins can honour them (e.g. `--debug` → `DATAROBOT_CLI_DEBUG=1`). These are called **universal flags**.

### How it works

Separation of concerns is strict:

| Layer | Responsibility |
|---|---|
| `cmd/root.go` | Declares which flags are universal and what env var suffix they map to |
| `internal/plugin` | Reads the annotations and injects env vars when launching a subprocess |

The two sides share only one thing: the annotation key constant `plugin.UniversalAnnotationKey = "plugin-universal"`.

### Adding a universal flag

Call `bindUniversal` in the universal flags block in `cmd/root.go` — that is the **only** change required:

```go
// cmd/root.go — universal flags block
bindUniversal("debug")
bindUniversal("disable-telemetry")
bindUniversal("my-new-flag")  // ← one line, done
```

`bindUniversal` does three things at once:

1. Looks up the already-registered `*pflag.Flag`
2. Binds it to viper (same as a plain `viperx.BindPFlag` call)
3. Sets `flag.Annotations["plugin-universal"] = []string{"MY_NEW_FLAG"}` — the suffix is derived automatically from the flag name (uppercased, hyphens → underscores)

`internal/plugin` discovers annotated flags automatically when it builds the subprocess environment — no edits needed there.

### How `internal/plugin` discovers them

`cmd/plugin.RegisterPluginCommands` passes the root command's persistent flagset to `internal/plugin.SetRootFlags`. From that point `universalFlagEnv()` (called inside `buildPluginEnv`) walks the flagset, finds any flag carrying a `plugin-universal` annotation, and emits `DATAROBOT_CLI_<SUFFIX>=<value>` into the subprocess environment.

### What you must NOT do

- Do **not** add a new flag name to `universalFlagEnv` in `exec.go` — the annotation on the flag drives discovery automatically.
- Do **not** call `internalPlugin.SetRootFlags` from `cmd/root.go` — that call lives inside `RegisterPluginCommands`.

## Mark flag groups

Use Cobra's flag group markers to enforce constraints on flag combinations. This provides better UX and clearer error messages.

### Mutually exclusive flags

Prevent users from using incompatible flags together:

```go
// Only one of these flags can be used
cmd.MarkFlagsMutuallyExclusive("list", "versions", "version")
```

When multiple flags are used together, users get a clear error:

```
Error: if any flags in the group [list versions version] are set none of the others can be; list version were all set
```

**Use cases:**

- Different operation modes (e.g., `--list` all vs `--versions` for specific)
- Conflicting output levels (e.g., `--silent` vs `--verbose`)
- Incompatible actions (e.g., `--parallel` vs `--watch`)

### Required together

If any flag in a group is used, all must be used:

```go
// If --name is used, --version, --url, --sha256, and --release-date must also be used
cmd.MarkFlagsRequiredTogether("name", "version", "url", "sha256", "release-date")
```

**Use cases:**

- Flags that form a complete set of parameters (e.g., all fields required for a record)
- Flags that depend on each other for validity

### One required

At least one flag from a group must be provided:

```go
// User must provide either --output or --stdout
cmd.MarkFlagsOneRequired("output", "stdout")
```

**Use cases:**

- Required operation modes where user must choose one
- Output destination selection

### Combining constraints

You can combine multiple constraints on the same flags:

```go
// User must pick ONE of these approaches
cmd.MarkFlagsRequiredTogether("name", "version", "url", "sha256", "release-date")
cmd.MarkFlagsMutuallyExclusive("from-file", "name")
cmd.MarkFlagsMutuallyExclusive("from-file", "version")
cmd.MarkFlagsMutuallyExclusive("from-file", "url")
cmd.MarkFlagsMutuallyExclusive("from-file", "sha256")
cmd.MarkFlagsMutuallyExclusive("from-file", "release-date")
```

This pattern means:

- Either use `--from-file` alone
- Or use all five manual flags together
- But never mix them

See the [Cobra Command documentation](https://pkg.go.dev/github.com/spf13/cobra#Command) for complete API reference.

## Flag naming conventions

- **Use singular names** — `template`, `dependency`, `plugin` (not `templates`, `dependencies`, `plugins`)
  - Plural aliases are acceptable for backward compatibility
- **Use lowercase with hyphens** — `--my-flag` (not `--myFlag` or `--my_flag`)
- **Provide both short and long forms** when appropriate:

  ```go
  cmd.Flags().BoolVarP(&force, "force", "f", false, "Force operation")
  ```

- **Be descriptive** — Flag descriptions should explain the purpose and any side effects
- **Document defaults** — If a flag has a non-obvious default value, mention it in the description

## Examples from the codebase

### Task run command (cmd/task/run/cmd.go)

```go
// Incompatible operations
cmd.MarkFlagsMutuallyExclusive("parallel", "watch")
```

**Rationale:** Cannot run multiple tasks in parallel while watching files for changes.

### Plugin install command (cmd/plugin/install/cmd.go)

```go
// Different operation modes
cmd.MarkFlagsMutuallyExclusive("list", "versions", "version")
```

**Rationale:**

- `--list` shows all available plugins
- `--versions` shows available versions for one plugin
- `--version` specifies an exact version to install

These are fundamentally different operations that can't coexist.

### Self plugin add command (cmd/self/plugin/add/cmd.go)

```go
// Manual flags must be used together
cmd.MarkFlagsRequiredTogether("name", "version", "url", "sha256", "release-date")

// But mutually exclusive with file-based approach
cmd.MarkFlagsMutuallyExclusive("from-file", "name")
cmd.MarkFlagsMutuallyExclusive("from-file", "version")
cmd.MarkFlagsMutuallyExclusive("from-file", "url")
cmd.MarkFlagsMutuallyExclusive("from-file", "sha256")
cmd.MarkFlagsMutuallyExclusive("from-file", "release-date")
```

**Rationale:** Users can either:

1. Load all plugin metadata from a JSON file (`--from-file`)
2. Specify all fields manually (requires all five flags together)

## Best practices

1. **Add constraints at flag definition time** — Mark flag groups before returning the command:

   ```go
   func Cmd() *cobra.Command {
       cmd := &cobra.Command{ /* ... */ }

       // Define flags
       cmd.Flags().BoolVar(...)
       cmd.Flags().StringVar(...)

       // Mark constraints AFTER all flags are defined
       cmd.MarkFlagsMutuallyExclusive("flag1", "flag2")

       return cmd
   }
   ```

2. **Consider shell completion** — Cobra automatically hides mutually exclusive flags from completion once one is selected, improving UX.

3. **Write clear descriptions** — Help users understand why flags are incompatible:

   ```go
   cmd.Flags().BoolVar(&parallel, "parallel", false, "Run tasks in parallel (cannot be used with --watch)")
   cmd.Flags().BoolVar(&watch, "watch", false, "Watch files and re-run (cannot be used with --parallel)")
   ```

4. **Test flag combinations** — Verify that your constraints work as expected:

   ```bash
   # Should error: incompatible flags
   dr mycommand --flag1 --flag2

   # Should work: one flag only
   dr mycommand --flag1
   ```

## Viper binding rules

The CLI deliberately limits which flags are bound to viper. Subcommand
flags (such as `--yes`, `--all`, `--if-needed`) **must not** be bound via
`viperx.BindPFlag`, and `cmd/root.go` does not bulk-bind subcommand flags
either. Doing so would slurp those flag values into `viper.AllSettings()`
and risk persisting them to `drconfig.yaml` on the next config write.

Outside `internal/config/...`, all viper interaction goes through the
`internal/config/viperx` wrapper, which omits `WriteConfig`,
`SafeWriteConfig`, and `BindPFlags` by design. Direct
`github.com/spf13/viper` imports are blocked by `depguard`.

Quick rules for new flags:

- **Transient flags** (per-invocation): read directly via
  `cmd.Flags().GetBool(...)`. Do not bind to viper.
- **Env-var override needed?** Register only the env var with
  `viperx.BindEnv(key, "DATAROBOT_CLI_…")` and OR the two sources in your
  handler:

  ```go
  yesFlag, _ := cmd.Flags().GetBool("yes")
  yes := yesFlag || viperx.GetBool("yes")
  ```

- **Sticky CLI preferences** (rare): bind via `viperx.BindPFlag` *and*
  add the key to `config.PersistableKeys` in `internal/config/write.go`.

For full details and test patterns, see the
[Configuration guide](configuration.md).

## See also

- [Cobra documentation](https://cobra.dev/)
- [Building guide](building.md) — General development setup and standards
- [Configuration guide](configuration.md) — viper, drconfig.yaml, viperx, persisted keys
