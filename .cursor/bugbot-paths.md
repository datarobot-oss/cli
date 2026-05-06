# Path Handling & Filesystem Operations

Applies to: Code handling file paths, external path inputs, filesystem operations (`internal/fileops/**`, `internal/filesapi/**`)

## Path Validation vs Normalization Are Distinct

**Rule**: Path handling requires two separate steps: validation (security) and normalization (canonicalization). Don't confuse them.

**Scope**: Functions receiving external paths (HTTP APIs, file uploads, config files)

**Validation**: Security-focused
- Reject absolute paths
- Reject traversal attempts (`..`, path separators escaping a base directory)
- Reject backslashes (on Unix, `\` is a legal filename byte)
- Should happen at system boundaries

**Normalization**: Canonicalization for consistent comparison
- Convert backslashes to forward slashes (POSIX)
- Normalize Unicode to NFC (especially for macOS compatibility)
- Resolve `.` and `..` (if validation already rejected them)
- Makes paths suitable for map keys, deduplication, comparisons

**What to flag**:
- Using `path.Clean` to validate traversal (insufficient on Unix)
- Assuming `filepath.Join` handles untrusted paths safely
- Missing backslash rejection on input boundaries
- Paths used as map keys without Unicode normalization

**Fix**: Implement both steps
```go
// Step 1: Validate at boundary (security)
func SafeRelPath(p string) (string, error) {
    if filepath.IsAbs(p) {
        return "", fmt.Errorf("path must be relative: %s", p)
    }
    if strings.Contains(p, "\\") {
        return "", fmt.Errorf("backslashes not allowed: %s", p)
    }
    if strings.Contains(p, "..") {
        return "", fmt.Errorf("traversal not allowed: %s", p)
    }
    return p, nil
}

// Step 2: Normalize for comparisons (canonicalization)
func NormalizePath(p string) (string, error) {
    if err := checkValid(p); err != nil {  // Validate first
        return "", err
    }
    // Convert backslashes to forward slashes
    p = strings.ReplaceAll(p, "\\", "/")
    // Normalize Unicode to NFC
    p = norm.NFC.String(p)
    return p, nil
}

// Use both at API boundary
func HandleUpload(ctx context.Context, path string) error {
    safe, err := SafeRelPath(path)  // Security
    if err != nil {
        return fmt.Errorf("invalid path: %w", err)
    }
    normalized, err := NormalizePath(safe)  // Canonicalization
    if err != nil {
        return fmt.Errorf("normalize path: %w", err)
    }
    // Now safe to use normalized path
}
```

---

## Backslash Rejection Is Cross-Platform Security

**Rule**: Explicitly reject backslashes at system boundaries. On Unix, `\` is a legal filename byte; `path.Clean` won't catch traversal via `..\..\escape`.

**Scope**: Path validation functions, API input validation

**What to flag**:
- Missing backslash checks in path validation
- Assuming `filepath.Join` or `path.Clean` prevents backslash traversal
- No platform-specific documentation for why backslashes matter

**Risk**: Path traversal vulnerability on Unix systems where `\` is a valid filename character.

**Example vulnerability**:
```go
// Bad - vulnerable on Unix
func ValidatePath(p string) error {
    // path.Clean doesn't catch this on Unix
    return nil
}
// Attacker passes: `..\..\etc\passwd`
// On Unix: `\` is literal; path becomes `../../../../etc/passwd`
// On Windows: `\` is separator; `..` escapes, but joined with base, may still escape

// Good - explicitly reject
func ValidatePath(p string) error {
    if strings.Contains(p, "\\") {
        return fmt.Errorf("backslashes not allowed")
    }
    // ... other checks
}
```

**Fix**: Always reject backslashes in validation
```go
if strings.Contains(path, "\\") {
    return fmt.Errorf("invalid path: backslashes not allowed (got %q)", path)
}
```

---

## Unicode Normalization for Cross-Platform Map Keys

**Rule**: Any path used as a map key (deduplication, manifest tracking) must be normalized to NFC. macOS uses NFD; Windows/Linux use NFC. Without normalization, the same file is two different keys.

**Scope**: Code using paths as map keys, deduplication logic, manifest files

**What to flag**:
- Paths used directly as map keys without normalization
- No handling of composed vs decomposed Unicode on macOS
- Manifest or deduplication logic without Unicode handling

**Impact**: Same file appears twice if accessed via different Unicode representations (macOS vs non-macOS systems).

**Fix**: Normalize before using as key
```go
import "golang.org/x/text/unicode/norm"

// When storing path as key
normalizedPath := norm.NFC.String(path)
fileMap[normalizedPath] = fileInfo

// When looking up
normalizedPath := norm.NFC.String(userPath)
info, exists := fileMap[normalizedPath]
```

---

## Document Path Format Contracts

**Rule**: Functions accepting paths should document the expected format. Do callers need to call NormalizePath() first? Are absolute paths allowed?

**Scope**: Public APIs that accept path parameters

**What to flag**:
- Path parameters without documented format expectations
- Inconsistent assumptions across related functions
- Unclear when NormalizePath() should be called

**Fix**: Document in function comments
```go
// Match returns true if path should be ignored per the ignore rules.
// Expects path to be relative, with forward slashes, and Unicode-normalized (NFC).
// Call NormalizePath() before passing untrusted paths.
func (m *Matcher) Match(path string) bool {
    // Implementation assumes normalized input
}

// IsSystemExcluded checks if path matches hardcoded system excludes.
// Path must be normalized; see Match().
func (m *Matcher) IsSystemExcluded(path string) bool {
}
```

---

## Streaming Without In-Memory Buffering

**Rule**: For file operations receiving external data (multipart uploads, streaming hashes), use io.Pipe and io.CopyBuffer instead of buffering to memory.

**Scope**: Large file operations, streaming endpoints

**What to flag**:
- Reading entire file into memory before processing
- `ioutil.ReadAll` on streamed upload data
- No buffering strategy documented for streaming operations

**Trade-off**: Streaming can't transparently retry if body read fails partway through. Document this explicitly.

**Fix**: Stream data instead of buffering
```go
// Bad - buffers entire file in memory
data, _ := ioutil.ReadAll(req.Body)
hash := sha256.Sum256(data)

// Good - streams and hashes without full buffering
hash := sha256.New()
io.CopyBuffer(hash, req.Body, make([]byte, 32*1024))

// Better - with error handling
hash := sha256.New()
if _, err := io.CopyBuffer(hash, req.Body, make([]byte, 32*1024)); err != nil {
    return fmt.Errorf("hash stream: %w", err)
}
```

**Document the trade-off**:
```go
// HashReader streams data through SHA256 without buffering.
// Callers must ensure the input stream doesn't exceed available disk/memory.
// If the stream is interrupted, the hash is incomplete; caller must retry the entire upload.
type HashReader struct { ... }
```

---

## Case Collision Detection

**Rule**: When paths are case-insensitive on some systems (Windows/macOS), detect collisions explicitly.

**Scope**: Path deduplication, file listing, manifest generation

**What to flag**:
- No handling of case-insensitive collisions
- Assumes case-sensitivity across all platforms
- Missing tests for `File.txt` vs `file.txt`

**Fix**: Detect collisions
```go
// Detect case-insensitive collisions
lowerMap := make(map[string]string)
for _, path := range paths {
    lower := strings.ToLower(path)
    if existing, exists := lowerMap[lower]; exists && existing != path {
        return fmt.Errorf("case collision: %q and %q", existing, path)
    }
    lowerMap[lower] = path
}
```

---

## Symlink Handling and Platform Differences

**Rule**: Symlink behavior differs across platforms. Document assumptions and test appropriately.

**Scope**: Filesystem walk, file operations

**What to flag**:
- No handling of symlinks
- Assumptions that symlinks work the same everywhere
- Missing platform-specific tests

**Platform differences**:
- Unix: symlinks are files; reading them returns link target path, not content
- Windows: symlinks require admin or developer mode; many systems don't have them
- macOS: symlinks are common in system directories

**Fix**: Document and test symlink behavior
```go
// Walk traverses the directory tree.
// Symlinks are NOT followed; they're reported as SymlinkType files.
// This prevents infinite loops and matches other tools' behavior (rsync, git).
func Walk(root string, fn WalkFunc) error {
    // Don't use filepath.Walk which follows symlinks
    // Use os.Lstat to detect symlinks without following
    return walkRecursive(root, fn)
}

// Test for symlink handling
//go:build unix

func TestWalk_SymlinksNotFollowed(t *testing.T) {
    // Create a symlink to a directory
    // Verify Walk reports it without traversing into it
}
```
