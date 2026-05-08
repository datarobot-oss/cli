# Security Boundaries & Constraints

Applies to: Code with security constraints, filesystem access, ignore/filter logic (`internal/ignore/**`, `internal/fileops/**`)

## Security Boundaries Must Be Non-Overridable

**Rule**: Critical security constraints cannot be bypassed by user input. Document why they exist and verify they're enforced before error handling.

**Scope**: Ignore patterns, access control, traversal prevention

**What to flag**:
- Security rules that can be overridden (system excludes bypassed by patterns, access controls that respect user preferences over security)
- No documentation for why a boundary exists
- Security checks mixed into business logic (hard to verify)

**Example**: System excludes (`.wapi`, `.git`) in ignore.Matcher
```go
// These CANNOT be overridden by user patterns
// Reason: .wapi and .git contain authentication tokens and private metadata
// Even if user writes "!.wapi" in .wapiignore, .wapi must still be excluded

var systemExcludes = []string{".wapi", ".git"}

// Correct: system excludes checked FIRST, before user patterns
func (m *Matcher) Match(path string) bool {
    // Always exclude system dirs, regardless of user patterns
    if m.IsSystemExcluded(path) {
        return true  // excluded
    }
    // Then check user patterns
    return m.matchUserPatterns(path)
}

// Wrong: user patterns can override
func (m *Matcher) Match(path string) bool {
    // Bad: user pattern "!.wapi" could bypass system exclude
    return m.matchUserPatterns(path)
}
```

**Fix**: Enforce at entry point
```go
// Document why the boundary exists
const systemExcludeReason = `
System excludes prevent syncing:
- .wapi: authentication tokens, API keys
- .git: private repository metadata
- __pycache__: rebuilds from source

These are non-negotiable and cannot be overridden.
`

func (m *Matcher) Match(path string) bool {
    // Check system excludes FIRST
    for _, exclude := range systemExcludes {
        if path == exclude || strings.HasPrefix(path, exclude+"/") {
            return true
        }
    }
    // THEN user patterns
    return m.matchUserPatterns(path)
}
```

---

## Test Non-Overridable Constraints Explicitly

**Rule**: If a rule "cannot be overridden," write a test that attempts the override and verifies it fails.

**Scope**: Test files for security-critical code

**What to flag**:
- No tests attempting to override system constraints
- Tests only for normal cases, not attack vectors
- Constraints documented but not tested

**Fix**: Write explicit override tests
```go
func TestSystemExcludes_NotOverridable(t *testing.T) {
    m := NewMatcher(`.wapiignore contains "!.wapi"`)
    
    // Even if user tries to un-exclude .wapi, it stays excluded
    tests := []struct {
        path     string
        expected bool  // expected to be excluded
    }{
        {".wapi", true},                  // system exclude
        {".wapi/config.json", true},      // inside system exclude
        {".git", true},                   // system exclude
        {".git/HEAD", true},              // inside system exclude
        {"normal.txt", false},            // user file, not excluded
    }
    
    for _, tt := range tests {
        result := m.Match(tt.path)
        if result != tt.expected {
            t.Errorf("Match(%q) = %v, want %v", tt.path, result, tt.expected)
        }
    }
}

// Test that negation patterns DON'T bypass system excludes
func TestSystemExcludes_NegationPatternsFail(t *testing.T) {
    patterns := []string{
        "!.wapi",           // try to un-exclude
        "!.git",            // try to un-exclude
        "!.wapi/**",        // try to un-exclude contents
    }
    
    for _, pattern := range patterns {
        m := NewMatcher(pattern)
        // Negation pattern should be ignored; system exclude still applies
        if !m.Match(".wapi/secret.key") {
            t.Errorf("negation pattern %q bypassed system exclude", pattern)
        }
    }
}
```

---

## Avoid Trust Assumptions Between Packages

**Rule**: Even internal packages should validate input. Don't assume upstream packages provide safe data.

**Scope**: Public APIs between internal packages

**What to flag**:
- Functions that assume input is already validated
- No input validation in internal packages
- Comments like "assume caller validated this"

**Risk**: If upstream validation is bypassed, downstream code fails unexpectedly. Validate once at each boundary.

**Fix**: Validate at package boundaries
```go
// Bad - assumes fileops caller validated paths
func (w *Walker) Walk(root string) error {
    // No validation; assumes root is safe
    return w.walkRecursive(root)
}

// Good - validate at boundary
func (w *Walker) Walk(root string) error {
    // Validate even though internal package
    safe, err := SafeRelPath(root)
    if err != nil {
        return fmt.Errorf("invalid root: %w", err)
    }
    normalized, err := NormalizePath(safe)
    if err != nil {
        return fmt.Errorf("normalize root: %w", err)
    }
    return w.walkRecursive(normalized)
}

// Internal helper can then rely on normalized input
func (w *Walker) walkRecursive(normalizedPath string) error {
    // Safe to use normalizedPath as key, for comparisons, etc.
    // No re-validation needed
}
```

---

## Validate Integration Points Explicitly

**Rule**: When one package calls another, verify the contract is enforced. Don't rely on "should work correctly" — test it.

**Scope**: Integration between packages (fileops → ignore, filesapi → fileops)

**What to flag**:
- No tests verifying integration contracts
- Assumptions about data format between packages
- Missing validation at integration points

**Example contract**: fileops.Walk normalizes paths before calling ignore.Matcher.Match()
```go
// In fileops/walk.go
func (w *Walker) Walk(root string, onFile func(string) error) error {
    normalized, _ := normalizePath(relPath)
    
    // Contract: pass NORMALIZED path to ignore matcher
    if w.ignore.Match(normalized) {
        continue
    }
}

// Test the contract
func TestIntegration_WalkNormalizesBeforeIgnore(t *testing.T) {
    // Create file with decomposed Unicode (NFD)
    testPath := "café.txt"  // could be NFC or NFD on macOS
    
    // Walker should normalize before passing to ignore
    walked := []string{}
    w.Walk(".", func(path string) error {
        walked = append(walked, path)
        return nil
    })
    
    // Verify all walked paths were normalized
    for _, p := range walked {
        normalized := norm.NFC.String(p)
        if p != normalized {
            t.Errorf("Walk returned non-normalized path: %q vs %q", p, normalized)
        }
    }
}
```

---

## Minimize Package Coupling

**Rule**: Internal packages should be independent. New foundational packages shouldn't introduce tight coupling between existing modules.

**Scope**: Package imports and dependencies

**What to flag**:
- New package imported by many unrelated packages
- Circular dependencies
- Creating a common dependency that wasn't needed before

**Good structure**:
```
fileops (independent)
  └─ ignore (independent; used by fileops)
       └─ go-gitignore library

dirprompt (isolated to cmd/workload)
  └─ (minimal dependencies)

filesapi (independent API layer)
  └─ (no dependency on fileops)
```

**Bad structure**:
```
fileops (depends on ignore)
  └─ ignore (depends on fileops)  // circular!

filesapi (depends on fileops)
  └─ fileops (now depends on filesapi)  // coupling
```

**Fix**: Keep packages independent
```go
// fileops.go - can exist without filesapi
type Walker struct {
    ignoreFunc func(string) bool  // Interface, not concrete type
}

// filesapi.go - can exist without fileops
type Client struct {
    // No imports from fileops
}

// cmd/workload - brings them together
client := filesapi.NewClient()
walker := fileops.NewWalker(client.IgnorePath)
```

---

## Streaming Operations Must Document Timeout Behavior

**Rule**: If a streaming operation can hang indefinitely (no timeout), document this explicitly.

**Scope**: Streaming file operations, multipart uploads, long-running I/O

**What to flag**:
- Streaming operations with no timeout
- No documentation of timeout behavior
- No context-based cancellation

**Fix**: Document timeout behavior
```go
// HashReader streams data through SHA256 hashing.
//
// WARNING: This function will read from body until EOF or error.
// There is NO timeout. If the client stops sending data but doesn't close the connection,
// HashReader will block indefinitely.
//
// Callers MUST set a timeout on the context:
//
//   ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
//   defer cancel()
//   hash, err := HashReader(ctx, body)
//
func HashReader(ctx context.Context, body io.Reader) (string, error) {
    // Check context timeout
    select {
    case <-ctx.Done():
        return "", ctx.Err()
    default:
    }
    
    // Read with context awareness
    hash := sha256.New()
    if _, err := io.CopyBuffer(hash, body, make([]byte, 32*1024)); err != nil {
        return "", fmt.Errorf("hash stream: %w", err)
    }
    return fmt.Sprintf("%x", hash.Sum(nil)), nil
}
```
