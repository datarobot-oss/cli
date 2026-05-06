# Code Organization & Internal Patterns

Applies to: Internal package code (`internal/**`)

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
        // cleanup old state
    }
    
    // do sync
    
    // log results
    
    // handle errors
    
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

## Platform-Specific Grouping

**Rule**: If 2+ platform implementations exist for same functionality, consider a subpackage. Keeps platform logic organized.

**Scope**: Internal packages with platform-specific code

**What to flag**:
- Multiple `*_unix.go`, `*_windows.go` files in same package
- No clear grouping of platform code
- Platform logic mixed with generic logic

**Good structure**:
```
internal/diskspace/
  ├── diskspace.go      (interface and generic logic)
  ├── unix.go           (//go:build unix)
  └── windows.go        (//go:build windows)

internal/fileops/
  ├── fileops.go
  └── synclock/
      ├── synclock.go
      ├── unix.go       (//go:build unix)
      └── windows.go    (//go:build windows)
```

---

## Dependency Injection Through Constructors

**Rule**: Constructor injection with setter methods is acceptable for small dependency sets (2-4 items). Flag if dependencies grow uncontrollably.

**Scope**: Type constructors and initialization in `internal/**`

**What to flag**:
- Constructor with 7+ parameters
- Growing dependency list without refactoring
- Tight coupling to concrete types instead of interfaces

**Good pattern**:
```go
// Small dependency set: OK
type Syncer struct {
    logger Logger
    client Client
}

func NewSyncer(logger Logger, client Client) *Syncer {
    return &Syncer{logger, client}
}

// Growing dependency set: consider refactoring
type Engine struct {
    logger Logger
    client Client
    cache Cache
    db Database
    metrics Metrics
    validator Validator
    transport Transport
}

// Better: group related dependencies
type Config struct {
    Logger Logger
    Client Client
    Cache Cache
    // ...
}
type Engine struct {
    config Config
}
```

---

## Test Seams for Testability

**Rule**: Use interfaces to allow test mocking. Concrete type dependencies make tests brittle and hard to maintain.

**Scope**: Public APIs and testable interfaces in `internal/**`

**What to flag**:
- Functions taking `*Engine` directly (tight coupling)
- No interface definitions for mocked dependencies
- Test setup extremely complex or slow

**Fix**: Define interfaces for dependencies
```go
// Bad - tight coupling
func Process(engine *Engine) error {
    return engine.Sync()
}

// Good - interface-based
type Processor interface {
    Sync(ctx context.Context) error
}

func Process(ctx context.Context, p Processor) error {
    return p.Sync(ctx)
}

// Easy to mock in tests
type mockProcessor struct{}
func (m *mockProcessor) Sync(ctx context.Context) error {
    return nil
}
```

---

## Function Signature Consistency

**Rule**: All implementations of the same functionality must have identical signatures. This is especially critical for platform-specific code.

**Scope**: Platform-specific implementations, interface implementations

**What to flag**:
- Different parameter order across implementations
- Different return types
- Optional parameters in one version but not another
- Parameter count mismatch

**Fix**: Verify against interface
```go
// Define interface once
type DiskOps interface {
    GetAvailable(ctx context.Context, path string) (int64, error)
    Cleanup(ctx context.Context, paths []string) error
}

// All implementations must match signature exactly
// unix.go
func (u *unixImpl) GetAvailable(ctx context.Context, path string) (int64, error) { ... }

// windows.go
func (w *windowsImpl) GetAvailable(ctx context.Context, path string) (int64, error) { ... }
```

---

## Explicit File Organization

**Rule**: File organization should be logical and documented. Don't scatter related code across many files.

**Scope**: Package structure in `internal/**`

**What to flag**:
- Unclear file naming or purpose
- Related logic split across many files (>5 for small package)
- No README explaining package structure

**Good practices**:
- `<packagename>.go` — Main types and interfaces
- `<feature>.go` — Feature-specific logic
- `*_unix.go`, `*_windows.go` — Platform-specific
- `*_test.go` — Tests (unless very large)
- README.md — Package purpose and structure (for complex packages)

---

## Phase Orchestration Clarity

**Rule**: Phase execution order must be explicit. Document why phases run in that order.

**Scope**: Phase definitions and orchestration

**What to flag**:
- Phases as `...phases` varargs without documentation
- Phase ordering assumptions implicit in code
- No comments explaining why phases are ordered this way

**Fix**: Make ordering explicit
```go
// Option 1: Named phase execution (clear)
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

// Option 2: Documented phase list
func (e *Engine) Run(ctx context.Context) error {
    phases := []Phase{
        // Order critical: validate before locking
        e.validateState,
        // Lock before executing to prevent concurrent access
        e.acquireLocks,
        // Do work with locks held
        e.execute,
    }
    
    for _, phase := range phases {
        if err := phase(ctx); err != nil {
            return err
        }
    }
    return nil
}
```
