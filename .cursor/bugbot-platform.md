# Platform-Specific Code

Applies to: Platform-specific files (`internal/**/*_unix.go`, `internal/**/*_windows.go`, etc.)

## Build Tags Required

**Rule**: Platform-specific files must have `//go:build` comments at the top. This prevents accidental compilation on wrong platforms.

**Scope**: Any `*_unix.go`, `*_windows.go`, `*_darwin.go` file

**What to flag**:
- Platform-specific file without `//go:build` comment
- Wrong or missing platform tag
- Mismatched build tag and filename

**Fix**: Add at the very top of the file
```go
//go:build unix

package diskspace

// unix-specific implementation
```

**Common build tags**:
- `//go:build unix` — Linux, macOS, BSD (POSIX systems)
- `//go:build windows` — Windows only
- `//go:build linux` — Linux only
- `//go:build darwin` — macOS only

---

## Identical Function Signatures Across Platforms

**Rule**: All platform implementations must have identical function signatures. This is enforced at compile time if using interfaces, but must be verified for concrete implementations.

**Scope**: Platform-specific implementations of same functionality

**What to flag**:
- Different parameter counts: `func FooUnix(ctx context.Context, path string) error` vs `func FooWindows(path string) error`
- Different return types: one returns `(string, error)`, another returns `([]string, error)`
- Parameter order changes
- Type mismatches: `int64` vs `int`

**Fix**: Verify signatures match by creating a shared interface
```go
type DiskSpace interface {
    GetAvailable(ctx context.Context, path string) (int64, error)
    Cleanup(ctx context.Context, paths []string) error
}

// Both unix.go and windows.go must implement the same interface
```

---

## Consistent Syscall Usage

**Rule**: Use `golang.org/x/sys/unix` instead of raw syscalls for better portability and maintenance.

**Scope**: Code using `syscall` package directly

**What to flag**:
- Direct `syscall` package imports in new code
- `syscall.SYS_*` constants instead of `golang.org/x/sys` wrappers
- Unportable syscall usage (doesn't work consistently across unix-like systems)

**Fix**: Use stdlib portability wrappers
```go
// Bad
import "syscall"
fd, _ := syscall.Open(path, syscall.O_RDONLY, 0)

// Good
import "golang.org/x/sys/unix"
fd, _ := unix.Open(path, unix.O_RDONLY, 0)
```

---

## Tracked Stubs for Incomplete Platforms

**Rule**: Incomplete platform implementations must have JIRA tickets and be documented in CLI help.

**Scope**: Stub functions that return "not implemented" errors

**What to flag**:
- `return errors.New("not implemented on Windows")` without JIRA ticket
- No documentation in CLI help text
- Silent feature degradation without user visibility

**Fix**: Link to JIRA ticket and document
```go
// Windows: https://jira.internal/DATAROBOT-12345
// This feature requires Windows registry access; implementation pending
func GetSystemInfo(ctx context.Context) (Info, error) {
    return Info{}, fmt.Errorf("system info not yet implemented on Windows (see DATAROBOT-12345)")
}
```

Also add to CLI help:
```go
// In command struct:
Help: "Get system information (not available on Windows; see DATAROBOT-12345 for tracking)"
```

---

## Platform Assumptions Must Be Explicit

**Rule**: If code has platform-specific behavior or assumptions, they must be obvious to users. "Should work on darwin" without testing is not acceptable.

**Scope**: Platform-specific logic or assumptions

**What to flag**:
- Comment like "this should also work on darwin" without explicit testing
- Platform assumptions in non-platform-specific code
- Silent differences in behavior across platforms

**Fix**: Test on all platforms or document clearly
```go
// Option 1: Add to CI/testing
// "Run tests on linux, darwin, windows"

// Option 2: Document limitation with JIRA ticket
// Only tested on Linux; darwin support pending (DATAROBOT-12345)

// Option 3: Detect platform and handle
func GetPlatformFeatures() Features {
    switch runtime.GOOS {
    case "linux":
        return linuxFeatures()
    case "darwin":
        return darwinFeatures()
    case "windows":
        return windowsFeatures()
    }
}
```

---

## Cross-Platform Testing

**Rule**: Platform-specific code must be tested on all target platforms, not assumed to work.

**Scope**: All platform-specific files

**What to flag**:
- `*_unix.go` without explicit testing on Linux and macOS
- No platform-specific test files
- Tests only on primary development platform

**Fix**: Ensure test coverage on all platforms
```go
// In test file or CI configuration:
// Test on: linux, darwin, windows
// or document platform-specific limitations

// Test-specific code can be platform-conditional:
//go:build unix

func TestUnixFeatures(t *testing.T) {
    // unix-specific tests
}
```
