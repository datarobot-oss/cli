# Root Command Factory

## Overview

The `RootFactory` (`cmd/root_factory.go`) assembles the root cobra command from
injectable dependencies. Each call to `Build()` returns a fresh, fully-wired
`*cli.CommandAdder` whose flags, sub-commands, and hooks are independent of
every other tree's — the key property that prevents cross-test leakage and
lets tests run in isolation. Some wiring still goes through process-global
state; see [What stays shared](#what-stays-shared) for the list and the
resulting rules.

Production code (via `main`) uses the singleton built in `init()`. Tests use
`NewRootFactory(...)` directly with stubbed dependencies, or
`NewIsolatedRootFactory()` for an all-no-op preset.

## Diagrams

### Factory structure

```mermaid
classDiagram
    class RootFactory {
        +deps Dependencies
        +telemetryClient *Client
        +Build() *CommandAdder
        +TelemetryClient() *Client
    }

    class Dependencies {
        +ConfigInitializer func
        +TLSSetup func
        +TelemetryProps func
        +TelemetryClient func
        +Animation func
        +PluginRegistrar func
        +ViperBinder func
    }

    class CommandAdder {
        +Command *cobra.Command
        +AddCommand(...)
        +ExecuteContext(ctx)
    }

    class Options {
        WithConfigInitializer()
        WithTLSSetup()
        WithTelemetryProps()
        WithTelemetryClient()
        WithAnimation()
        WithPluginRegistrar()
    }

    RootFactory --> Dependencies : owns
    RootFactory --> CommandAdder : Build() produces
    Options ..> RootFactory : configure
```

### Runtime sequence

How a command execution flows from `main` through the factory.

```mermaid
sequenceDiagram
    participant M as main
    participant E as ExecuteContext
    participant F as productionFactory
    participant P as persistentPreRun
    participant C as cobra cmd

    Note over F: built once at init()
    M->>E: ExecuteContext(ctx)
    E->>F: RootCmd.ExecuteContext(ctx)
    F->>P: cobra fires PersistentPreRunE
    P->>P: deps.ConfigInitializer()
    P->>P: deps.TLSSetup()
    P->>P: deps.TelemetryProps()
    P->>P: stamp CommandKind
    P->>P: deps.TelemetryClient(props)
    P->>P: deps.Animation()
    P->>C: cmd.RunE()
    C-->>M: error or nil
    Note over F: cobra.OnFinalize flushes telemetry
```

## Adding a new command

### 1. Register it

```go
// cmd/root_factory.go — addSubcommands()
func (f *RootFactory) addSubcommands(adder *cli.CommandAdder) {
    adder.AddCommand(
        // ...existing commands...
        myfeature.Cmd(),  // ← add it here
    )
}
```

### 2. Write the command

Commands have no knowledge of the factory. Same cobra pattern as always:

```go
// cmd/myfeature/cmd.go
package myfeature

func Cmd() *cobra.Command {
    c := &cobra.Command{
        Use:   "myfeature",
        Short: "Does the thing",
        RunE:  run,
    }

    telemetry.Track(c)  // wired once, fires automatically via cobra.OnFinalize

    return c
}

func run(cmd *cobra.Command, args []string) error {
    // config, TLS, and telemetry client are already set up by the factory's
    // PersistentPreRunE — nothing to initialize here
    return nil
}
```

### 3. Write a test

This is where the factory pays off. Previously you'd fight shared viperx state;
now you get a clean isolated tree:

```go
func TestMyFeatureCmd(t *testing.T) {
    root := cmd.NewIsolatedRootFactory().Build()

    var buf bytes.Buffer
    root.SetOut(&buf)
    root.SetArgs([]string{"myfeature"})

    require.NoError(t, root.Execute())
    assert.Contains(t, buf.String(), "expected output")
}
```

`NewIsolatedRootFactory` pre-applies no-op overrides for every dependency, so
the test touches no disk, no environment, no global viper state, and no plugin
binaries. Pass your own options after the defaults to override specific
dependencies per test.

**The short version:** commands are written identically to before — they return a
`*cobra.Command` from a `Cmd()` constructor, get registered in one place, and
know nothing about the factory. The factory only changes how tests and `main`
assemble the tree.

---

## Overriding a dependency in a test

The most common reason to override a dependency is preventing viperx state from
leaking between tests. Viper resolves a key in this precedence order:

    viperx.Set()  >  changed pflag  >  env var (live)  >  config file  >  defaults

Env vars are read live — `os.LookupEnv` on every `Get` — so `t.Setenv` cleanup
is visible immediately, and env vars are *not* the leak vector. The sticky
layers are the other three:

- **`viperx.Set(...)` overrides** persist until `viperx.Reset()` and, being
  highest-precedence, shadow live env reads for every later test in the
  process.
- **Config-file contents** parsed by `ReadInConfig` persist in the global
  instance until the next read or `viperx.Reset()`.
- **Changed-flag state on the shared `RootCmd` singleton**: once a test
  executes it with a flag (e.g. `--debug`), the pflag binding serves that
  value with priority over env and config for the rest of the process.

The fix is to give each test its own command tree built with a no-op
`ConfigInitializer` that never touches viper:

```go
func TestSomethingThatSetsEnvVars(t *testing.T) {
    t.Setenv("DATAROBOT_CLI_API_TOKEN", "test-token")

    // Without the factory: Set overrides and config-file contents from
    // earlier tests persist in the global viper instance and shadow this
    // test's env vars.
    //
    // With the factory: the no-op ConfigInitializer never calls
    // viperx.AutomaticEnv() or ReadConfigFile(), so this tree resolves
    // only what the test seeds explicitly.
    root := cmd.NewIsolatedRootFactory(
        cmd.WithConfigInitializer(func(_ *cobra.Command) error {
            // Seed the viper keys your command needs without calling
            // AutomaticEnv() or ReadConfigFile(). Note: viperx.Set writes
            // a highest-precedence override that persists until
            // viperx.Reset() (which itself wipes flag bindings, so it is
            // only safe when the next use is a fresh Build). Prefer
            // seeding keys only in isolated-tree tests like this one.
            viperx.Set("api-token", "test-token")
            return nil
        }),
    ).Build()

    root.SetArgs([]string{"mycommand"})
    require.NoError(t, root.Execute())
}
```

`NewIsolatedRootFactory` already no-ops every dependency, including the ones
with surprising side effects: the default `TelemetryProps` hits the network or
disk for device/user IDs, and the default `PluginRegistrar` executes every
`dr-*` binary found on PATH to fetch its manifest. Prefer the preset over
hand-assembling stubs:

```go
// Minimal no-op factory — safe for any unit test.
root := cmd.NewIsolatedRootFactory().Build()
```

> **Why not just use the global `RootCmd`?** The singleton is built once in
> `init()` with the production dependencies, and every `Execute()` on it runs
> `ReadInConfig` against the live environment and marks its bound flags
> Changed. Both effects stick: config contents stay in the global viper
> instance until the next read, and a changed flag keeps outranking env vars
> for the rest of the process. (There is no env-var caching involved — viper
> reads `os.LookupEnv` live on every `Get`.) A fresh factory tree sidesteps
> both, and `NewIsolatedRootFactory()` skips the config read entirely.

---

## What stays shared

`Build()` gives each test its own cobra tree, but some wiring still goes
through process-global state. Knowing exactly what remains shared tells you
which test topologies are safe.

| Shared global | Touched when | Effect |
| --- | --- | --- |
| viper flag bindings | every `Build()` (default `ViperBinder`) | bindings re-point at the newest tree; only the last-built tree's flags resolve via `viperx.Get*` |
| viper config contents | `Execute()` via `ReadInConfig`, read at `Build()` by plugin discovery | parsed file contents persist in the global instance until the next read or `viperx.Reset()` |
| `cobra.EnableCaseInsensitive` | every `Build()` | idempotent global write |
| `cobra.OnFinalize` list | every Execute | append-only and replayed on every Execute; the factory's generation guard neutralizes stale replays |
| `http.DefaultTransport` | Execute (TLS setup) | last-executed tree's TLS config wins |
| `log` package logger | Execute (`log.Start` / `log.Stop`) | single global logger |
| `config.SetAPIConsumerTrace` | Execute | last-executed tree's trace string wins |

The practical rules:

- **Serial build-then-execute is safe**, especially via
  `NewIsolatedRootFactory()`, which skips the viper binding and plugin
  discovery entirely.
- **Mixing `RootCmd` with factory trees is not**: the newest `Build()`
  hijacks the global viper bindings from any previously built tree, so only
  one tree's bound flags resolve at a time ("one live tree at a time").
- **`t.Parallel` across trees is not safe** with default dependencies —
  parallel trees race on every global listed above. True parallel isolation
  requires an injected viper instance, tracked as a backlog follow-up.

---

## Adding a new dependency to the factory

Follow these four steps whenever a new cross-cutting concern needs to be
injectable (for example, a new HTTP client, a feature-flag provider, or an
audit logger).

### Step 1 — Define the function type

Add a named function type to the top of `root_factory.go`. Named types make
option signatures self-documenting and prevent callers from accidentally passing
the wrong function:

```go
// AuditLoggerFunc writes an audit event for the current command invocation.
// Replace in tests with a no-op or a recorder.
type AuditLoggerFunc func(cmd *cobra.Command, args []string) error
```

### Step 2 — Add the field to `Dependencies`

```go
type Dependencies struct {
    ConfigInitializer ConfigInitializerFunc
    TLSSetup          TLSSetupFunc
    // ... existing fields ...

    // AuditLogger writes an audit event before each command runs.
    AuditLogger AuditLoggerFunc
}
```

### Step 3 — Set the production default and add a `With*` option

```go
func DefaultDependencies() Dependencies {
    return Dependencies{
        // ... existing defaults ...
        AuditLogger: defaultAuditLogger,
    }
}

// WithAuditLogger overrides the audit-logging step.
// Pass a no-op in tests that don't need audit coverage.
func WithAuditLogger(fn AuditLoggerFunc) Option {
    return func(f *RootFactory) {
        f.deps.AuditLogger = fn
    }
}
```

### Step 4 — Call it in the right factory method

Most new dependencies belong in `persistentPreRun` (runs before every command)
or `addSubcommands` (runs once at build time). For something like an audit
logger that needs to fire before each command:

```go
func (f *RootFactory) persistentPreRun(cmd *cobra.Command, args []string) error {
    // ... existing setup ...

    if err := f.deps.AuditLogger(cmd, args); err != nil {
        return err
    }

    return nil
}
```

That's it. Tests override it with `WithAuditLogger(func(...) error { return nil })`;
production gets `defaultAuditLogger` for free.

> **Planned next field:** an `IOStreams` dependency (stdin/stdout/stderr
> abstraction, à la GitHub CLI's `cmdutil.Factory`). The injection rails are in
> place, but it only pays off once command constructors consume injected
> streams instead of writing to `os.Stdout` directly — a cross-cutting
> migration tracked as a follow-up to this stack.

---

## Feature-gating a command

For gating mechanics — `features.SetGate`, env-var naming, and gating nested
subcommands — see [feature-gates.md](feature-gates.md).

The factory-specific fact: gates are evaluated by `cli.CommandAdder.AddCommand`
at **Build time**, so a built tree's gate state is frozen. The production
singleton's gate state is fixed at `init()`; a test that needs a different
gate state must set the env var first and then build a fresh tree:

```go
func TestMyFeatureHiddenByDefault(t *testing.T) {
    // Build a fresh tree with no feature gate enabled.
    root := cmd.NewIsolatedRootFactory().Build()

    for _, sub := range root.Commands() {
        assert.NotEqual(t, "myfeature", sub.Name(),
            "myfeature should not be present when gate is disabled")
    }
}

func TestMyFeatureVisibleWhenGateEnabled(t *testing.T) {
    t.Setenv("DATAROBOT_CLI_FEATURE_MYFEATURE", "true")

    root := cmd.NewIsolatedRootFactory().Build()

    var found bool
    for _, sub := range root.Commands() {
        if sub.Name() == "myfeature" {
            found = true
        }
    }

    assert.True(t, found, "myfeature should be present when gate is enabled")
}
```
