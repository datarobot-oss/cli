# Cross-Platform Code & Behavior

Applies to: Platform-specific code, path handling, symlinks, case sensitivity, line endings, filesystem behavior

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

**Rule**: All platform implementations must have identical function signatures. Differences in signatures are a source of subtle bugs and platform-dependent failures.

**Scope**: Platform-specific implementations (`*_unix.go`, `*_windows.go`)

**What to flag**:
- Different parameter counts across implementations
- Different return types
- Parameter order changes
- Type mismatches

**Fix**: Verify signatures match via interfaces
```go
type DiskSpace interface {
    GetAvailable(ctx context.Context, path string) (int64, error)
    Cleanup(ctx context.Context, paths []string) error
}

// Both unix.go and windows.go must implement identically
```

---

## Document Cross-Platform Behavior Differences

**Rule**: If behavior differs across platforms (symlinks, paths, line endings), document it explicitly in code comments and tests.

**Scope**: Code with platform-specific behavior

**What to flag**:
- No documentation of platform differences
- Tests that assume Unix-only behavior
- Symlink or path handling without notes
- Undocumented behavior changes across platforms

**Fix**: Document platform differences clearly
```go
// Walk traverses the directory tree under root.
//
// Symlinks are NOT followed (treated as files):
//   - Unix/macOS: Symlinks are reported with type File; contents not traversed
//   - Windows: Symlinks require admin mode; most systems don't have them
//   - Rationale: Prevents infinite loops and matches git/rsync behavior
//
// Path separators:
//   - Input: root must use forward slashes (/)
//   - Output: paths use forward slashes (/) on all platforms
//   - Example: On Windows, "dir\file.txt" is converted to "dir/file.txt"
//
// Case sensitivity:
//   - Input paths are validated for case collisions (File.txt vs file.txt)
//   - This is enforced on all platforms to prevent sync errors when moving to case-insensitive systems
//
// See TestWalk_Symlinks for platform-specific tests.
func Walk(root string, fn WalkFunc) error
```

---

## Symlink Handling Across Platforms

**Rule**: Symlinks must be handled consistently across platforms. Document whether they are followed, ignored, or cause errors.

**Scope**: Filesystem traversal code, file operations

**What to flag**:
- Symlinks followed on Unix but errors on Windows
- No documentation of symlink behavior
- Infinite symlink loops possible in traversal

**Fix**: Make symlink behavior explicit
```go
// Does NOT follow symlinks (consistent across platforms)
// - Unix: Symlinks are reported as files
// - Windows: Symlinks are reported as files (most systems lack admin)
// - All: Prevents loops and matches git/rsync behavior

func Walk(root string, fn WalkFunc) error {
    // Use filepath.Walk or custom code that does NOT follow symlinks
}
```

---

## Case Sensitivity and Path Collisions

**Rule**: Windows paths are case-insensitive but case-preserving; Unix paths are case-sensitive. Enforce consistent behavior on all platforms to prevent sync errors.

**Scope**: Path validation, file operations, especially in sync scenarios

**What to flag**:
- No validation for case collisions (File.txt vs file.txt)
- Sync code that works on case-sensitive systems but fails on case-insensitive
- Paths normalized in tests but not in production

**Fix**: Detect and reject case collisions on all platforms
```go
// Validates that no two paths in the tree differ only by case.
// This prevents sync errors when moving code from Unix (case-sensitive)
// to Windows (case-insensitive), where two "different" files become one.
func ValidateCaseCollisions(paths []string) error {
    seen := make(map[string]string) // lowercase -> original
    for _, p := range paths {
        lower := strings.ToLower(p)
        if existing, ok := seen[lower]; ok && existing != p {
            return fmt.Errorf("case collision: %q and %q differ only by case", existing, p)
        }
        seen[lower] = p
    }
    return nil
}
```

---

## Line Ending Handling

**Rule**: Line endings (CRLF vs LF) must be handled consistently. Document whether line endings are preserved, normalized, or cause errors.

**Scope**: File read/write operations, especially for text files and configuration

**What to flag**:
- No handling of different line endings across platforms
- Text file operations that differ between Unix and Windows
- No documentation of line ending behavior

**Fix**: Be explicit about line ending strategy
```go
// Behavior:
// - Git: Reads file as-is (preserves CRLF or LF)
// - Most text processing: Normalizes to \n (LF) on all platforms
// - Generated files: Always use \n (LF) for consistency

// Option 1: Preserve line endings
data, _ := os.ReadFile(path)  // Keep CRLF on Windows, LF on Unix

// Option 2: Normalize to LF
data, _ := os.ReadFile(path)
content := strings.ReplaceAll(string(data), "\r\n", "\n")
```

---

## Syscall Portability

**Rule**: Use `golang.org/x/sys/unix` instead of raw `syscall` package for better portability and maintenance.

**Scope**: Code using `syscall` package directly

**What to flag**:
- Direct `syscall` package imports in new code
- `syscall.SYS_*` constants instead of wrappers
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

**Rule**: Incomplete platform implementations must have JIRA tickets and be documented in CLI help. Don't silently degrade functionality.

**Scope**: Stub functions that return "not implemented" errors

**What to flag**:
- `return errors.New("not implemented on Windows")` without JIRA ticket
- No documentation in CLI help text
- Silent feature degradation without user visibility

**Fix**: Link to JIRA ticket and document clearly
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

**Rule**: If code has platform-specific behavior or assumptions, they must be obvious and tested. "Should work on darwin" without explicit testing is not acceptable.

**Scope**: Platform-specific logic or assumptions

**What to flag**:
- Comment like "this should also work on darwin" without explicit testing
- Platform assumptions in non-platform-specific code
- Silent differences in behavior across platforms
- Code tested only on primary development platform

**Fix**: Test on all platforms or document clearly
```go
// Option 1: Add to CI/testing
// "Run tests on linux, darwin, windows"

// Option 2: Document limitation with JIRA ticket
// Only tested on Linux; darwin support pending (DATAROBOT-12345)

// Option 3: Detect platform and handle explicitly
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

**Scope**: All platform-specific files (`*_unix.go`, `*_windows.go`)

**What to flag**:
- `*_unix.go` without explicit testing on Linux and macOS
- No platform-specific test files
- Tests only on primary development platform

**Fix**: Ensure test coverage on all platforms
```go
// In test file or CI configuration:
// Test on: linux, darwin, windows

// Use platform-conditional tests:
//go:build unix

func TestUnixFeatures(t *testing.T) {
    // unix-specific tests
}

// Document platform-specific behavior in table-driven tests:
func TestCrossPlatform(t *testing.T) {
    tests := []struct {
        name string
        platforms []string // ["linux", "darwin", "windows"]
        expected string
    }{
        {"symlink handling", []string{"linux", "darwin"}, "symlink"},
        {"path separator", []string{"linux", "darwin"}, "/"},
    }
}
```
