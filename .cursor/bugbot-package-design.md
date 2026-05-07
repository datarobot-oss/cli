# Package Documentation & Public API Design

Applies to: Package documentation, public APIs, foundational packages, inter-package contracts

## doc.go for Package Intent

Each new package should have a `doc.go` explaining:
- What the package does
- Boundaries (what it doesn't do)
- Security considerations (if any)
- Common usage pattern

**Example**:
```go
// Package ignore implements .wapiignore pattern matching for the workload sync engine.
//
// The effective ignore set is the union of:
//   1. System excludes (hardcoded, non-overridable): .wapi, .git, __pycache__
//   2. User patterns from .wapiignore files (override-able)
//
// System excludes cannot be bypassed by user patterns. This prevents syncing
// authentication tokens or cache directories.
//
// Usage:
//	m := ignore.NewMatcher(wapiIgnoreContent)
//	if m.Match("some/path") {
//	    // Path should be ignored
//	}
package ignore
```

**What to flag**: No `doc.go`, empty documentation, missing boundaries or security notes.

---

## Contracts Between Packages

**Rule**: Document the contract when one package calls another. Contracts must be verified in integration tests.

**Contract documentation includes**:
- Input format requirements (types, ranges, formats)
- Return value semantics
- When errors occur
- Performance characteristics

**Example**:
```go
// Match returns true if path should be ignored.
//
// Contract (required input format):
//   - path must be relative (no leading /)
//   - path must use forward slashes (not \)
//   - path must be Unicode-normalized (NFC)
//
// If paths don't meet this contract, call NormalizePath() first.
//
// Returns:
//   - true if path is excluded
//   - false if path should be included
//
// Errors: Never returns error (patterns are pre-validated)
func (m *Matcher) Match(path string) bool
```

**Integration test example**:
```go
func TestIntegration_WalkCallsIgnore(t *testing.T) {
    ignoreCalls := []string{}
    walker := NewWalker(fileops.WithIgnore(func(p string) bool {
        ignoreCalls = append(ignoreCalls, p)
        return false
    }))
    walker.Walk("test_dir")
    
    // Verify all passed paths were normalized
    for _, path := range ignoreCalls {
        assert.True(t, isNormalized(path), "Walk passed non-normalized path: %q", path)
    }
}
```

**What to flag**: No contract documentation, implicit assumptions, missing integration tests.

---

## Nil/Empty Returns

Document when a function returns nil/empty as a valid state (vs error).

**Pattern**:
```go
// LoadPatterns reads .wapiignore and returns ignore patterns.
//
// Returns:
//   - patterns and no error if .wapiignore exists
//   - nil patterns and no error if .wapiignore does NOT exist (valid state)
//   - nil patterns and error if .wapiignore exists but cannot be read (I/O error)
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

// Caller must distinguish:
patterns, err := LoadPatterns(path)
if err != nil {
    return err  // I/O error
}
if patterns == nil {
    patterns = DefaultPatterns()  // File doesn't exist
}
```

**What to flag**: Nil returns with no explanation, tests being only documentation.

---

## Failure Modes

Document how functions fail, especially streaming, long-running, and resource-intensive operations.

**Failure modes to document**:
- What errors can be returned and why
- When the function might block or hang
- Timeout requirements
- Resource consumption (memory, disk)
- Partial failure modes

**Example**:
```go
// HashReader computes SHA256 hash while streaming data from body.
//
// Failure modes:
//   - If body exceeds available memory: blocks indefinitely (no size check)
//   - If body stream is interrupted: hash is incomplete, caller must retry
//   - If context is canceled: returns context.Err()
//
// BLOCKS until the entire body is read. Set a timeout on the context:
//   ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
//   defer cancel()
func HashReader(ctx context.Context, body io.Reader) (string, error)
```

---

## Limitations and Future Work

Document intentional limitations with JIRA tickets for tracking.

**Pattern**:
```go
// LoadPatterns loads patterns from a .wapiignore file.
//
// Limitations (see DATAROBOT ticket for status):
//   - Only reads from a single .wapiignore file (no traversal up tree) [DATAROBOT-12345]
//   - No negation priority rules like gitignore [DATAROBOT-12346]
//   - No include patterns (!) yet [DATAROBOT-12347]
func LoadPatterns(path string) ([]string, error)
```

**What to flag**: Limitations without JIRA tickets, "TODO"/"FIXME" without tracking.

---

## README for Complex Packages

For foundational packages, create `internal/<package>/README.md` with:
- What the package does
- Design decisions and rationale
- Usage examples
- Common pitfalls
- Links to test files for more examples

**Example structure**:
```markdown
# filesapi Package

## Overview
HTTP API client for DataRobot Workload FilesAPI.

## Design
- **Streaming uploads**: Uses multipart/form-data with io.Pipe (no buffering)
- **Error handling**: Wraps HTTP errors with operation context
- **Pagination**: Provides SafePaginate helper (prevents host boundary crossing)

## Usage
```go
client := New(baseURL, token)
reader := os.Open("myfile.txt")
resp, err := client.Upload(ctx, "workspaceID", reader)
```

## Limitations
- No retries; caller must implement retry logic
- Streaming uploads can't be retried mid-stream

## See Also
- fileops.Walk: Traverses local filesystem
- ignore.Matcher: Pattern matching for sync exclusions
```
