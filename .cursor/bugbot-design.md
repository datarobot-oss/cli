# Architecture & Code Design

Applies to: Package design, foundational PRs, internal code organization, dependency management

## Separation of Concerns: Layers and Dependencies

**Rule**: Domain-specific logic (FilesAPI workflows, path traversal, pattern matching) must be kept separate from generic utilities (HTTP verbs, error handling, utility functions). All layers should depend inward only.

**Scope**: Package-level design, API design, cross-package boundaries

**What to flag**:
- Domain-specific logic creeping into generic layers
- HTTP verbs (GET, PUT, POST) mixed with business logic
- Generic utilities tightly coupled to specific domains
- Circular dependencies or sideways dependencies between packages

**Correct layering**:
```
Layer 1 (Generic): HTTP utilities, error handling, I/O, filesystem ops
  └─ drapi/errors.go — Convert HTTP status to error (domain-agnostic)
  └─ fileops/walk.go — Traverse filesystem (domain-agnostic)

Layer 2 (Domain): API clients, business logic
  └─ filesapi/client.go — FilesAPI workflows (uses generic layer)
  └─ artifact/client.go — Artifact workflows (uses generic layer)

Layer 3 (CLI): Commands, user interaction
  └─ cmd/workload/sync.go — Sync command (uses domain layer)
```

**Bad example**: FilesAPI package with domain logic in generic layer
```go
// fileops/errors.go (WRONG: generic package knows about FilesAPI)
func FilesAPIError(statusCode int, action string) error {
    // This couples fileops to a specific domain
}
```

---

## Single Responsibility Per Function

**Rule**: Each function should have one clear job. If doing setup, execution, cleanup, logging, and retries, split it.

**Scope**: All functions in `internal/**`

**What to flag**:
- Phase function doing 5+ distinct things
- Long function (>50 lines) with multiple concerns
- Nested conditionals (>3 levels) suggesting mixed concerns

**Fix**: Split into focused functions
```go
// Bad - does everything
func SyncPhase(ctx context.Context) error {
    lock.Lock()
    defer lock.Unlock()
    
    state, _ := loadState()
    if state.exists {
        // cleanup
    }
    // do sync, log, handle errors...
    return nil
}

// Good - clear separation
func SyncPhase(ctx context.Context) error {
    if err := cleanup(ctx); err != nil {
        return fmt.Errorf("cleanup failed: %w", err)
    }
    if err := doSync(ctx); err != nil {
        log.Errorf("sync failed: %v", err)
        return err
    }
    return nil
}
```

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

// I/O only
func Ask(prompt PromptFunc, dir string) (string, error) {
    return prompt("Enter directory:", dir)
}

// Logic only
func ResolveDir(ctx context.Context, response string) (string, error) {
    safe, err := SafeRelPath(response)
    if err != nil {
        return "", fmt.Errorf("invalid directory: %w", err)
    }
    return NormalizePath(safe)
}

// Orchestration
func Interactive(ctx context.Context, prompt PromptFunc) (string, error) {
    response, err := Ask(prompt, "/default")
    if err != nil {
        return "", err
    }
    return ResolveDir(ctx, response)
}
```

---

## Type-Driven Design: Function Signatures as Contracts

**Rule**: Define function type aliases to clarify intent and make signatures readable. Use interfaces to enable testing.

**Scope**: Package APIs with callbacks or dependency functions

**What to flag**:
- Long function signatures without type aliases
- `func(string, string) (string, error)` repeated in multiple places
- Functions taking concrete types instead of interfaces (tight coupling)

**Fix**: Use type aliases and interfaces
```go
// Before - unclear intent, tight coupling
func ResolveDir(ctx context.Context, dir string, 
    promptFunc func(question, defaultVal string) (string, error)) (string, error)

// After - intent is clear, testable
type PromptFunc func(question, defaultVal string) (string, error)
type Resolver interface {
    Sync(ctx context.Context) error
}

func ResolveDir(ctx context.Context, dir string, prompt PromptFunc) (string, error)
func Process(ctx context.Context, r Resolver) error
```

---

## Dependency Injection and Testability

**Rule**: Constructor injection with 2-4 dependencies is acceptable. Use interfaces for dependencies to enable mocking. Flag if dependencies grow uncontrollably or concrete types are used.

**Scope**: Type constructors and testable APIs in `internal/**`

**What to flag**:
- Constructor with 7+ parameters
- Tight coupling to concrete types (e.g., `*Engine` instead of interface)
- Test setup extremely complex or slow
- No interface definitions for mocked dependencies

**Good pattern**:
```go
// Small dependency set with interfaces: OK
type Processor interface {
    Sync(ctx context.Context) error
}

type Syncer struct {
    logger Logger
    processor Processor
}

func NewSyncer(logger Logger, proc Processor) *Syncer {
    return &Syncer{logger, proc}
}

// Growing dependency set: group related dependencies
type Config struct {
    Logger Logger
    Client Client
    Cache Cache
}
type Engine struct {
    config Config
}
```

**Bad pattern**: 
```go
// Tight coupling to concrete type
func Process(engine *Engine) error {
    return engine.Sync()
}

// Fix: Use interface
type Processor interface {
    Sync(ctx context.Context) error
}
func Process(ctx context.Context, p Processor) error {
    return p.Sync(ctx)
}
```

---

## Function Signature Consistency Across Implementations

**Rule**: All implementations of the same functionality must have identical signatures. This is critical for platform-specific code and interface implementations.

**Scope**: Platform-specific implementations (`*_unix.go`, `*_windows.go`), interface implementations

**What to flag**:
- Different parameter order across implementations
- Different return types
- Optional parameters in one version but not another
- Parameter count mismatch

**Fix**: Define interface once, all implementations match
```go
type DiskOps interface {
    GetAvailable(ctx context.Context, path string) (int64, error)
    Cleanup(ctx context.Context, paths []string) error
}

// unix.go and windows.go must have identical signatures
func (u *unixImpl) GetAvailable(ctx context.Context, path string) (int64, error) { ... }
func (w *windowsImpl) GetAvailable(ctx context.Context, path string) (int64, error) { ... }
```

---

## Code Organization Within Packages

**Rule**: File organization should be logical and clear. Related code belongs in the same file or subpackage.

**Scope**: Package structure in `internal/**`

**What to flag**:
- Unclear file naming or purpose
- Related logic split across many files (>5 for small package)
- Platform-specific code not clearly separated

**Good file structure**:
```
internal/diskspace/
  ├── diskspace.go      (interface and generic logic)
  ├── unix.go           (//go:build unix)
  └── windows.go        (//go:build windows)

internal/fileops/
  ├── fileops.go        (main types)
  ├── walk.go           (walking logic)
  └── synclock/
      ├── synclock.go
      ├── unix.go       (//go:build unix)
      └── windows.go    (//go:build windows)
```

---

## Phase Orchestration Clarity

**Rule**: Phase execution order must be explicit and documented. Make the ordering and rationale clear in code.

**Scope**: Phase definitions and orchestration

**What to flag**:
- Phases as `...phases` varargs without documentation
- Phase ordering assumptions implicit in code
- No comments explaining why phases are ordered this way

**Fix**: Make ordering explicit with comments
```go
func (e *Engine) Run(ctx context.Context) error {
    // Phase 1: Validate state early
    if err := e.validateState(ctx); err != nil {
        return fmt.Errorf("validation failed: %w", err)
    }
    
    // Phase 2: Lock resources
    if err := e.acquireLocks(ctx); err != nil {
        return fmt.Errorf("lock failed: %w", err)
    }
    
    // Phase 3: Do the work
    if err := e.execute(ctx); err != nil {
        log.Errorf("execution failed: %v", err)
        return err
    }
    
    return nil
}
```

---

## Library Choices Must Be Justified

**Rule**: New dependencies should be maintained, widely used, and necessary. Don't add them speculatively. Justify in PR description.

**Scope**: New imports in `go.mod`

**What to flag**:
- New dependencies without explanation
- Libraries that solve a small problem or have trivial implementations
- Unmaintained or rarely-used libraries

**Good justifications**:
- `go-gitignore` — Standard-compliant ignore pattern matching; reinventing this is 300+ lines and error-prone
- `golang.org/x/text` — Unicode normalization; incorrect NFC/NFD handling causes cross-platform bugs
- `drapi` (internal) — Shared HTTP utilities; reduces duplication across API clients

**Bad justifications**:
- "Might be useful later"
- "Easier than writing it ourselves" (if it's 20 lines)
- Unmaintained libraries

---

## Duplication Patterns: Extract Only After Validation

**Rule**: Extract utility functions only after the same code appears in 2+ independent places. Avoid premature abstraction.

**Scope**: Helper functions, utility packages

**Pattern for extraction**:
- **1 place**: Leave in original package
- **2 places**: Consider extracting to shared package
- **3+ places**: Definitely extract

**Example**:
```go
// In filesapi/client.go: assertNextOnSameHost()
// Only used in filesapi pagination
// DEFER extraction until artifact.go, templates.go, llms.go all implement pagination

// In validation.go: validateField()
// Only used for versions.yaml
// DEFER extraction until 2nd package needs field validation

// In fileops.go: NormalizePath()
// Used by filesapi, walk, and likely future packages
// EXTRACT immediately; pattern is clear
```

**Exception**: Extract immediately if code is complex (>50 lines) or architectural (all HTTP clients use drapi/errors).

---

## Scope Discipline: Foundational PRs

**Rule**: Foundational PRs establish architecture. Do architectural work now, but don't bundle unrelated refactorings (scope creep).

**Scope**: Foundational package design, PR scope decisions

**What to flag**:
- Obvious refactoring deferred to "future PRs" without justification
- Scope creep combining unrelated features
- Architectural decisions made without full context

**Good example**: Defer non-foundational work
```go
// TODO(PR-###): Extract endpointURL to drapi when 2nd pagination client uses it
// REASON: Premature abstraction; usage patterns still unclear

// vs

// Must be in PR: Foundational streaming strategy for uploads
// Deferred: Error aggregation refactoring for validation package
// REASON: validation is separate epic; don't couple here
```

**Bad example**: Bundling unrelated changes
```
PR #470 description: "Workload Code Sync Foundation"
But also includes: "Refactor dirprompt.Ask to support --yes in init command"
// Wrong: This is SEPARATE from the sync foundation
// Fix: Create separate PR for init command enhancement
```
