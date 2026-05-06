# Documentation & Contract Clarity

Applies to: Package documentation, public APIs, foundational packages

## doc.go for Package-Level Intent

**Rule**: Foundational packages should have clear `doc.go` explaining purpose, boundaries, and typical usage patterns.

**Scope**: New packages in `internal/`, especially foundational packages

**What to flag**:
- No `doc.go` in new packages
- `doc.go` exists but doesn't explain purpose or boundaries
- No clear description of what the package does

**Fix**: Create `doc.go` with clear intent
```go
// Package ignore implements .wapiignore pattern matching for the workload sync engine.
//
// The effective ignore set is the union of:
//   1. System excludes (hardcoded, non-overridable): .wapi, .git, __pycache__, etc.
//   2. User patterns from .wapiignore files (override-able)
//
// System excludes cannot be bypassed by user patterns. This prevents syncing
// authentication tokens (.wapi/), private repository metadata (.git/), or cache
// directories (__pycache__/) that would compromise security or break the remote system.
//
// Usage:
//
//	m := ignore.NewMatcher(wapiIgnoreContent)
//	if m.Match("some/path") {
//	    // Path should be ignored during sync
//	}
//
// See the individual type and function documentation for details.
package ignore
```

**Include in doc.go**:
- What the package does
- What it doesn't do (boundaries)
- Security considerations (if any)
- Common usage pattern
- Link to relevant files (filesapi contract, etc.)

---

## Clarify Empty vs Nil Returns

**Rule**: When a function returns nil/empty as a valid state (no .wapiignore file = nil user patterns), document this explicitly in code comments, not just in tests.

**Scope**: Public functions with nullable/empty results

**What to flag**:
- Function returns `nil` or empty with no explanation
- Tests are only documentation for when `nil` is valid
- Comment missing: "Returns nil if file not found (not an error)"

**Fix**: Document with code comments
```go
// LoadPatterns reads .wapiignore and returns ignore patterns.
//
// Returns:
//   - patterns and no error if .wapiignore exists
//   - nil patterns and no error if .wapiignore does NOT exist (valid state)
//   - nil patterns and error if .wapiignore exists but cannot be read (I/O error)
//
// Callers should distinguish:
//   patterns, err := LoadPatterns(path)
//   if err != nil {
//       return err  // I/O error, should be reported
//   }
//   if patterns == nil {
//       // .wapiignore doesn't exist, use defaults
//       patterns = DefaultPatterns()
//   }
func LoadPatterns(path string) ([]string, error) {
    data, err := os.ReadFile(path)
    if os.IsNotExist(err) {
        return nil, nil  // Not an error; file just doesn't exist
    }
    if err != nil {
        return nil, fmt.Errorf("read %s: %w", path, err)
    }
    return parsePatterns(string(data))
}
```

---

## Document Failure Modes

**Rule**: Document how a function fails, especially for streaming operations, long-running processes, and resource-intensive operations.

**Scope**: Public APIs with non-obvious failure modes

**What to flag**:
- Streaming operations with no failure documentation
- Long-running processes that could hang or timeout
- Resource limits not documented
- Silent degradation or partial failure modes

**Fix**: Document failure modes in comments
```go
// HashReader computes SHA256 hash while streaming data from body.
//
// Failure modes:
//   - If body is not valid UTF-8: returns error (hash is incomplete)
//   - If body exceeds available memory: blocks indefinitely (no size check)
//   - If body stream is interrupted: hash is incomplete, caller must retry entire upload
//   - If context is canceled: returns context.Err(), hash is discarded
//
// Important: This function BLOCKS until the entire body is read.
// Set a timeout on the context to prevent indefinite hangs:
//
//   ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
//   defer cancel()
//
// See also: CopyBuffer for streaming without hashing.
func HashReader(ctx context.Context, body io.Reader) (string, error)
```

**Include**:
- What errors can be returned and why
- When the function might block or hang
- Timeout requirements
- Resource consumption (memory, disk)
- Partial failure modes (some data hashed, some not)

---

## Explain Non-Obvious Security Choices

**Rule**: When making unusual security decisions (rejecting backslashes, not following symlinks), document the WHY in code comments.

**Scope**: Security-critical validation, access control, path handling

**What to flag**:
- Unusual validation rules without explanation
- Security checks that seem arbitrary
- Comments like "reject backslashes" without reasoning

**Fix**: Document security decisions
```go
// ValidatePath checks if a path is safe for operations.
//
// Validates:
//   1. Path is relative (no absolute paths like /etc/passwd)
//   2. Path doesn't escape root (no .. traversal)
//   3. Path contains no backslashes
//
// The backslash check is critical:
//   - On Windows: \ is a path separator, but filepath.Join handles it
//   - On Unix: \ is a LEGAL FILENAME BYTE (not a separator)
//   - Example: path `..\..\etc\passwd` on Unix is literally:
//     - `..` (directory, literal bytes)
//     - `\` (filename character, not separator)
//     - `.` (filename character)
//     - `.` (filename character)
//     - etc.
//   - Result: Attacker escapes with path separators that look like separators to humans
//
// Solution: Reject backslashes entirely; force POSIX paths (forward slashes only)
//
// See: https://cwe.mitre.org/data/definitions/22.html (Path Traversal)
func ValidatePath(p string) error {
    if filepath.IsAbs(p) {
        return fmt.Errorf("path must be relative: %s", p)
    }
    if strings.Contains(p, "\\") {
        return fmt.Errorf("backslashes not allowed (use /): %s", p)
    }
    // ... check for ..
}
```

**Include**:
- Why the security measure exists (threat model)
- Why obvious solutions don't work
- Links to vulnerability databases or relevant RFCs
- Platform-specific reasoning

---

## Document Cross-Platform Behavior

**Rule**: If behavior differs across platforms (symlinks, paths, line endings), document it explicitly.

**Scope**: Code with platform-specific behavior

**What to flag**:
- No documentation of platform differences
- Tests that assume Unix-only behavior
- Symlink or path handling without notes

**Fix**: Document platform differences
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

## Contracts Between Packages

**Rule**: Document the contract (expected input format, assumptions, error behavior) when one package calls another.

**Scope**: Public APIs between internal packages

**What to flag**:
- No contract documentation for public functions
- Implicit assumptions about caller behavior
- Missing error documentation

**Fix**: Document the contract
```go
// Match returns true if path should be ignored per the ignore rules.
//
// Contract (required input format):
//   - path must be relative (no leading /)
//   - path must use forward slashes (not \)
//   - path must be Unicode-normalized (NFC)
//
// If the caller's paths don't meet this contract, call NormalizePath() first.
// Example:
//
//   userPath := somethingFromWeb
//   normalized, err := NormalizePath(userPath)
//   if err != nil { ... }
//   if m.Match(normalized) { ... }
//
// Returns:
//   - true if path is excluded (matches ignore patterns or is system exclude)
//   - false if path should be included
//
// Errors: Never returns error (patterns are pre-validated)
func (m *Matcher) Match(path string) bool
```

**Include**:
- Input format requirements
- What to do if input doesn't meet requirements
- Return value semantics
- When errors occur (if any)
- Performance characteristics (O(n) vs O(log n) etc.)

---

## Document Limitations and Future Work

**Rule**: If a feature has intentional limitations or known gaps, document them with JIRA tickets for tracking.

**Scope**: Public APIs, especially foundational packages

**What to flag**:
- Features with documented limitations but no tracking
- Stubs or incomplete implementations without tickets
- Comments like "TODO" or "FIXME" without ticket references

**Fix**: Document with tracking
```go
// LoadPatterns loads patterns from a .wapiignore file.
//
// Limitations:
//   - Only reads from a single .wapiignore file (no traversal up directory tree)
//     See DATAROBOT-12345 for supporting nested .wapiignore files
//   - Pattern syntax matches gitignore, not including negation priorities
//     See DATAROBOT-12346 for implementing gitignore negation priority rules
//   - No support for include patterns (!) yet
//     See DATAROBOT-12347 for feature request
//
// These limitations are intentional for the MVP. Please file an issue
// if your use case requires any of these features.
func LoadPatterns(path string) ([]string, error)
```

---

## README for Complex Packages

**Rule**: For complex foundational packages, provide a `README.md` explaining architecture, design decisions, and examples.

**Scope**: New foundational packages (fileops, filesapi, ignore)

**File location**: `internal/<package>/README.md`

**Include**:
- What the package does
- Why design decisions were made
- Example usage patterns
- Links to test files for more examples
- Common pitfalls or gotchas

**Example structure**:
```markdown
# filesapi Package

## Overview
filesapi provides the HTTP API client for DataRobot Workload FilesAPI.

## Design
- **Streaming uploads**: Uses multipart/form-data with io.Pipe to avoid buffering
- **Error handling**: Wraps HTTP errors with operation context (upload, download, list)
- **Pagination**: Provides SafePaginate helper to prevent host boundary crossing

## Usage

### Upload a file
```go
client := New(baseURL, token)
reader := os.Open("myfile.txt")
resp, err := client.Upload(ctx, "workspaceID", reader)
```

## Limitations
- No retries; caller must implement retry logic if desired
- Streaming uploads can't be retried mid-stream (incomplete hash)

## See Also
- fileops.Walk: Traverses local filesystem
- ignore.Matcher: Pattern matching for sync exclusions
```
