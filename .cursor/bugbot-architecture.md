# Architecture & Design Patterns

Applies to: Foundational PRs, cross-package architecture, design decisions

## Separation of Concerns: Domain Logic and Generic Utilities

**Rule**: Domain-specific logic (FilesAPI workflows, path traversal, pattern matching) must be kept separate from generic utilities (HTTP verbs, error handling, utility functions).

**Scope**: Package-level design, API design

**What to flag**:
- Domain-specific logic creeping into generic layers
- HTTP verbs (GET, PUT, POST) mixed with business logic
- Generic utilities tightly coupled to specific domains

**Example**: FilesAPI package
```go
// Good - separate concerns
// fileops/errors.go (generic error handling)
func errFromResp(resp *http.Response) error {
    // Generic: converts HTTP status to error
    // Usable by any API, not specific to FilesAPI
}

// filesapi/client.go (domain logic)
type Client struct { ... }
func (c *Client) Upload(ctx context.Context, path string) error {
    // Domain-specific: handles FilesAPI workflow
    // Uses generic errFromResp but doesn't expose it
}

// Bad - domain creeping into generic
// fileops/errors.go
func FilesAPIError(statusCode int, action string) error {
    // Wrong: generic package knows about FilesAPI
    // This couples fileops to a specific domain
}
```

**Fix**: Keep layers separate
```
Layer 1 (Generic): HTTP utilities, error handling, I/O
  └─ drapi/errors.go — Convert HTTP status to error (domain-agnostic)
  └─ fileops/walk.go — Traverse filesystem (domain-agnostic)

Layer 2 (Domain): API clients, business logic
  └─ filesapi/client.go — FilesAPI workflows (uses generic layer)
  └─ artifact/client.go — Artifact workflows (uses generic layer)

Layer 3 (CLI): Commands, user interaction
  └─ cmd/workload/sync.go — Sync command (uses domain layer)
```

---

## Scope Boundary Setting in Foundational PRs

**Rule**: Foundational PRs establish architecture. Don't defer architectural refactoring; do it now. BUT don't bundle unrelated refactorings (scope creep).

**Scope**: Foundational package design, PR scope decisions

**What to flag**:
- Obvious refactoring deferred to "future PRs" without justification
- Scope creep combining unrelated features
- Architectural decisions made without full context

**Defer extraction only when**:
- Duplication exists in 2+ independent places (not "could" be duplicated)
- The code is stable and usage patterns are clear
- Extracting now would require speculating about future needs

**Example**: PR #470 defers some refactorings
```go
// In filesapi/client.go
func endpointURL(base, path string) string {
    // Could be extracted to drapi/endpoints.go
    // But DEFER because:
    // - Only used in filesapi (no duplication yet)
    // - artifact.go, templates.go, llms.go don't paginate yet
    // - Extract when the third pagination client appears
}

// Don't defer THIS
func (c *Client) Upload(content io.Reader) error {
    // This is foundational; must be right NOW
    // Streaming strategy, error handling, retry logic
    // Cannot be deferred
}
```

**Fix**: Be explicit about deferral
```go
// Document deferral decisions
// TODO(PR-###): Extract endpointURL to drapi when 2nd pagination client uses it
// REASON: Premature abstraction; usage patterns still unclear

// vs

// Must be in PR: Foundational streaming strategy
// Deferred: Error aggregation refactoring for validation package
// REASON: validation is separate epic; don't couple here
```

---

## Scope Discipline: Don't Bundle Unrelated Refactorings

**Rule**: Foundational PRs should focus on the new feature/package. Resist the urge to fix unrelated bugs or refactor tangential code.

**Scope**: PR scope decisions, refactoring opportunities

**What to flag**:
- Architectural shifts bundled with feature work
- "While I'm here" cleanups that aren't essential
- Changes that could be separate PRs (--yes flag behavior, error consolidation, etc.)

**Example anti-pattern**: PR #470 could have bundled dirprompt's --yes flag changes into init/cmd.go
```go
// BAD: Bundling scope
// PR #470 description: "Workload Code Sync Foundation"
// But also includes: "Refactor dirprompt.Ask to support --yes in init command"
// This is SEPARATE from the sync foundation

// GOOD: Keep scopes separate
// PR #470: "Workload Code Sync Foundation: filesapi, fileops, ignore packages"
// PR #471: "Support --yes flag in init command" (separate, can be reviewed separately)
```

**Fix**: Create separate PRs for separate concerns
```
PR #470 (Sync Foundation)
  ├─ filesapi/client.go — New API client
  ├─ fileops/walk.go — New filesystem walker
  ├─ ignore/matcher.go — New pattern matcher
  └─ dirprompt extraction (preparation for separate PR)

PR #471 (Init Command Enhancement)
  ├─ cmd/workload/init.go — Support --yes flag
  └─ dirprompt.Ask(PromptFunc) — Enable mock prompts
```

---

## Library Choices Must Be Justified

**Rule**: New dependencies should be maintained, widely used, and necessary. Don't add them speculatively.

**Scope**: New imports in `go.mod`

**What to flag**:
- New dependencies without explanation
- Libraries that solve a small problem or have trivial implementations
- Unmaintained or rarely-used libraries

**Good justifications**:
- `go-gitignore` — Standard-compliant ignore pattern matching; using a well-maintained library prevents bugs
- `golang.org/x/text` — Unicode normalization; incorrect NFC/NFD handling causes cross-platform bugs; use standard library
- `drapi` (internal) — Shared HTTP utilities; reduces duplication across API clients

**Bad justifications**:
- "Might be useful later"
- "Easier than writing it ourselves" (if it's 20 lines)
- Unmaintained or single-version libraries

**Fix**: Document justification in PR description
```
## Dependencies Added

### go-gitignore
**Why**: Standard-compliant gitignore pattern matching. Our custom implementation would be 300+ lines and prone to edge cases. This library is maintained and widely used.

### golang.org/x/text
**Why**: Unicode NFC/NFD normalization for macOS compatibility. Critical for consistent cross-platform path deduplication. Reinventing this would be error-prone.

### (NOT ADDED) pflag
**Why considered**: Command-line flag parsing already solved by cobra. Adding pflag would introduce two competing flag libraries. Deferred.
```

---

## Integration Point Validation

**Rule**: When packages call each other, verify the contract is enforced at the boundary. Don't rely on implicit assumptions.

**Scope**: Public APIs between internal packages

**What to flag**:
- Integration between packages without tests
- Implicit assumptions about data format or validation
- Missing error handling at boundaries

**Example**: fileops.Walk calling ignore.Matcher
```go
// Contract: Walk passes normalized paths to Matcher
// Verify in tests
func TestIntegration_WalkCallsIgnore(t *testing.T) {
    ignoreCalls := []string{}
    
    mockIgnore := func(path string) bool {
        ignoreCalls = append(ignoreCalls, path)
        return false
    }
    
    walker := NewWalker(fileops.WithIgnore(mockIgnore))
    walker.Walk("test_dir")
    
    // Verify all passed paths were normalized
    for _, path := range ignoreCalls {
        if !isNormalized(path) {
            t.Errorf("Walk passed non-normalized path to Ignore: %q", path)
        }
    }
}
```

---

## Type-Driven Design: Function Signatures as Contracts

**Rule**: Define function type aliases to clarify intent and make signatures readable.

**Scope**: Package APIs with callback or dependency functions

**What to flag**:
- Long function signatures without type aliases
- `func(string, string) (string, error)` repeated in multiple places
- Unclear what each parameter means

**Fix**: Use type aliases
```go
// Before - unclear intent
func ResolveDir(ctx context.Context, dir string, 
    promptFunc func(question, defaultVal string) (string, error)) (string, error)

// After - intent is clear
type PromptFunc func(question, defaultVal string) (string, error)

func ResolveDir(ctx context.Context, dir string, prompt PromptFunc) (string, error)
```

**Benefits**:
- Documentation: reader immediately knows what PromptFunc does
- Testability: easy to inject mock `PromptFunc` in tests
- Consistency: same function signature used everywhere

---

## Separate I/O from Logic

**Rule**: I/O functions (reading user input, filesystem operations) should be separate from orchestration logic (validation, decision-making).

**Scope**: Functions mixing I/O and business logic

**What to flag**:
- Functions that ask user questions AND validate domain constraints
- Error handling that validates input format
- User interaction mixed with business logic

**Example**: dirprompt
```go
// Good - separate concerns
type PromptFunc func(question string, defaultVal string) (string, error)

// I/O only: Ask user
func Ask(prompt PromptFunc, dir string) (string, error) {
    return prompt("Enter directory:", dir)
}

// Logic only: Validate result
func ResolveDir(ctx context.Context, response string) (string, error) {
    safe, err := SafeRelPath(response)
    if err != nil {
        return "", fmt.Errorf("invalid directory: %w", err)
    }
    return NormalizePath(safe)
}

// Orchestration: Combine them
func Interactive(ctx context.Context, prompt PromptFunc) (string, error) {
    response, err := Ask(prompt, "/default")
    if err != nil {
        return "", err
    }
    return ResolveDir(ctx, response)
}

// Bad - mixes concerns
func AskAndValidate(prompt PromptFunc, dir string) (string, error) {
    response, _ := prompt("Enter directory:", dir)
    if response == "" {
        return "", errors.New("required")
    }
    // Now mixing I/O and validation
    if err := os.Chdir(response); err != nil {
        return "", err
    }
    return response, nil
}
```

---

## Defer Generic Utilities Until Duplication Exists

**Rule**: Extract utility functions only after the same code appears in 2+ independent places. Avoid premature abstraction.

**Scope**: Helper functions, utility packages

**Duplication pattern for extraction**:
- **1 place**: Leave in original package
- **2 places**: Consider extracting to shared package
- **3+ places**: Definitely extract; likely already duplicated elsewhere

**Example**: 
```go
// In filesapi/client.go: assertNextOnSameHost()
// Only used in filesapi pagination
// DEFER extraction until artifact.go, templates.go, llms.go all implement pagination

// In validation.go: validateField()
// Only used for versions.yaml validation
// DEFER extraction until 2nd package needs field validation

// In fileops.go: NormalizePath()
// Used by filesapi, walk, and likely future packages
// EXTRACT immediately; pattern is clear
```

**Exception**: Extract immediately if:
- Code is complex (>50 lines) and duplication would magnify the problem
- Architectural reason to centralize (all HTTP clients use drapi/errors)
- Package is foundational and needs to establish a pattern
