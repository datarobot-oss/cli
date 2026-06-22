# Tool Registry

The tool registry lives in `internal/dependencies/registry/` and drives two behaviours:

- **Install suggestions** — when a dependency install fails, the CLI picks an install command tailored to the package managers it detected in the user's environment.
- **Name normalisation** — display names like `"Node.js"` or `"Taskfile task runner"` are resolved back to registry keys like `"node"` or `"task"`.

## Architecture

| File | Purpose |
|------|---------|
| `registry_managers.go` | Defines every known package/version manager and how to detect it |
| `registry_types_and_methods.go` | Core types (`ToolInfo`, `ManagerStrategy`, `FallbackStrategy`) and `DetectEnvironment` / `SelectInstallStrategy` |
| `<tool>.go` (e.g. `python.go`, `node.go`) | Registers one tool into `ToolRegistry` via `init()` |

---

## How to add a new tool

Create a new file `internal/dependencies/registry/<toolname>.go` and register the tool in `init()`.

```go
package registry

func init() {
    ToolRegistry["mytool"] = ToolInfo{
        Name:    "My Tool",                          // display name shown in tips
        Aliases: []string{"mytool-cli", "my tool"},  // optional alternate names
        Strategies: []Strategy{
            // Manager strategies — evaluated in order; first detected manager wins.
            ManagerStrategy{
                Manager:        "brew",
                Commands:       []string{"brew install mytool"},
            },
            ManagerStrategy{
                Manager:        "winget",
                Commands:       []string{"winget install Vendor.MyTool"},
            },
            // FallbackStrategy — always last; shown when no manager is detected.
            FallbackStrategy{
                Commands:        []string{"curl -fsSL https://example.com/install.sh | sh"},
                CommandsWindows: []string{`powershell -c "irm https://example.com/install.ps1 | iex"`},
                URL:             "https://example.com/install",
            },
        },
    }
}
```

### Version placeholders

Use `{version}` and `{version_mm}` in commands when install commands accept a version:

| Placeholder | Example input | Substituted value |
|-------------|--------------|-------------------|
| `{version}` | `3.9.6` | `3.9.6` |
| `{version_mm}` | `3.9.6` | `3.9` (major.minor only) |

When a strategy uses a placeholder, set `DefaultVersion` so the CLI can suggest a command even when no minimum version is specified:

```go
ManagerStrategy{
    Manager:        "pyenv",
    DefaultVersion: "3.14",
    Commands:       []string{"pyenv install {version}", "pyenv global {version}"},
},
```

### Rules

- The registry key (e.g. `"mytool"`) must be lowercase.
- Always include a `FallbackStrategy` as the last entry — the test `TestAllToolsHaveAtLeastOneFallback` enforces this.
- `Manager` in `ManagerStrategy` must match a name in `knownManagers` (see `registry_managers.go`).

---

## How to add a new manager

All manager definitions live in `internal/dependencies/registry/registry_managers.go` in the `knownManagers` slice.

For most managers, detection is a simple PATH check:

```go
{
    Name:    "mymanager",
    present: func(ctx detectionCtx) bool { return ctx.hasCommand("mymanager") },
},
```

For managers that are only valid on certain platforms:

```go
// macOS/Linux only
{
    Name:    "mymanager",
    present: func(ctx detectionCtx) bool { return ctx.hasCommand("mymanager") && ctx.goos != "windows" },
},

// Windows only
{
    Name:    "mymanager",
    present: func(ctx detectionCtx) bool { return ctx.hasCommand("mymanager") && ctx.goos == "windows" },
},
```

For managers detected by directory rather than a binary (like `nvm`), override `present` with custom logic:

```go
{
    Name: "mymanager",
    present: func(ctx detectionCtx) bool {
        dir := ctx.getenv("MYMANAGER_DIR")
        if dir == "" {
            if home, err := os.UserHomeDir(); err == nil {
                dir = filepath.Join(home, ".mymanager")
            }
        }
        return ctx.dirExists(dir)
    },
},
```

The `detectionCtx` fields available inside `present`:

| Field | Type | Description |
|-------|------|-------------|
| `ctx.hasCommand(name)` | `bool` | Reports whether `name` is on `PATH` |
| `ctx.getenv(key)` | `string` | Reads an environment variable |
| `ctx.dirExists(path)` | `bool` | Reports whether `path` is an existing directory |
| `ctx.goos` | `string` | `runtime.GOOS` value (e.g. `"darwin"`, `"linux"`, `"windows"`) |

After adding a manager to `knownManagers`, it is automatically included in `KnownManagers` (the exported slice used by the installer to identify managers in failed command output) and in `DetectEnvironment()`. No further wiring is needed.

### Using the new manager in a tool

Reference the new manager's `Name` in any tool's `ManagerStrategy.Manager` field:

```go
ManagerStrategy{
    Manager:  "mymanager",
    Commands: []string{"mymanager install mytool"},
},
```
