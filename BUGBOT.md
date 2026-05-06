# BUGBOT.md

Reference guide for DataRobot CLI code review patterns. Cursor's bugbot uses this to provide more useful PR feedback.

## Concurrency Patterns

**Goroutine panics** — All goroutines must have `recover()` blocks. Silent panics from goroutines cause data corruption and are hard to debug.
- Flag: goroutine spawned without panic recovery
- Example fix: `go func() { defer func() { if r := recover(); r != nil { log.Errorf(...) } }(); ... }()`

**Error channels** — Error channel buffer size must match the expected number of errors. Over-buffered channels waste memory and hide goroutine leaks.
- Flag: `errChan := make(chan error, <large-number>)` where small number of goroutines write to it
- Review: Verify `cap(errChan)` matches actual error producers

**Loop variable capture in goroutines** — Use `fa := fa` before passing loop variables to goroutines. Direct loop variable capture causes all goroutines to see the last value.
- Flag: `for _, fa := range ... { go func() { use(fa) }() }`
- Correct: `for _, fa := range ... { fa := fa; go func() { use(fa) }() }`

**WaitGroup cleanup** — Every `wg.Add()` must have a corresponding `defer wg.Done()`. Verify `wg.Wait()` is called after all goroutines are spawned.
- Flag: Missing `defer wg.Done()` in spawned goroutine
- Flag: `wg.Wait()` called before all goroutines spawned
- Risk: Goroutine leaks, deadlocks

**Channel closure** — Channels must be closed only by the sender after all writers finish. Only the sender should close; receivers must not close.
- Flag: Receiver closing a channel
- Flag: Closing channel before `wg.Wait()` completes
- Verify: `wg.Wait()` comes before `close(errChan)`

## Error Handling

**Silent error ignoring** — Never use `_ = someFunc()` without logging. Errors must be visible for debugging.
- Flag: `_ = function()` with no surrounding error check or log
- Fix: Either return the error, log it, or add a comment explaining why it's safe to ignore

**Cryptic error messages** — User-facing errors must wrap system errors with context. "context deadline exceeded" is not helpful; "timeout after 300s waiting for API" is.
- Flag: Returning wrapped errors directly without additional context
- Flag: User-facing error messages that don't explain what operation timed out
- Fix: `fmt.Errorf("timeout after %ds waiting for %s: %w", timeoutSecs, operation, err)`

**Specialized error messages** — Distinguish error types with specific user messaging, not generic "API error".
- Flag: Generic error handling (all API errors treated the same)
- Good: `if statusCode == 404 { return fmt.Errorf("artifact %s not found", id) }`
- Bad: `return fmt.Errorf("API error: %v", err)`

**Error logging in orchestrators** — Phase orchestrators must log errors before returning them. Logging only at the top level loses context about which phase failed.
- Flag: Errors returned from phase functions without logging
- Fix: `if err != nil { log.Errorf("phase %s failed: %v", phaseName, err); return err }`

**Multiple error scenarios** — When multiple errors can occur, the code must explicitly handle them: return first error, collect all, or fail-fast. The choice must be documented.
- Flag: Multiple error paths without clear documentation of which is returned
- Review: Verify behavior matches comments/docs

## Lock & Resource Lifecycle

**Lock acquisition tracking** — Trace when locks are acquired and released. Verify no indefinite holds that could deadlock or stall the system.
- Flag: Long-running phase holding a lock without clear release condition
- Flag: No `defer lock.Unlock()` or equivalent cleanup in error paths
- Fix: Use `defer` for all lock releases; add timeout context for long operations

**Lock error handling** — Never silently ignore lock acquisition/release failures.
- Flag: `lock.Lock()` or `lock.Unlock()` errors ignored (no error check)
- Risk: Stuck locks, broken mutual exclusion, silent data races

**Timeout on locks** — Long-running phases holding locks must have timeouts to prevent indefinite waits.
- Flag: `lock.Lock()` called in phase that can run indefinitely without timeout
- Fix: Use context with timeout: `if !lock.TryLockWithContext(ctx) { return errors.New("timeout acquiring lock") }`

**Double-release safety** — If a lock can be released multiple times, it must be idempotent or clearly documented.
- Flag: `lock.Unlock()` called in multiple code paths without guard
- Verify: Idempotent behavior or document that double-release is prevented by higher-level logic

## Platform-Specific Code

**Build tags required** — Files with platform-specific code (`*_unix.go`, `*_windows.go`) must have `//go:build` comments at the top.
- Flag: `_unix.go` or `_windows.go` file without `//go:build` comment
- Fix: Add `//go:build unix` or `//go:build windows` at the very top of the file

**Identical function signatures** — All platform implementations must have identical function signatures. Polymorphism happens through file names, not type assertions.
- Flag: `func FooUnix() error` vs `func FooWindows(ctx context.Context) error` (different signatures)
- Fix: Ensure all platforms implement the same interface

**Consistent syscall usage** — Use `golang.org/x/sys/unix` instead of raw syscalls for better portability and maintenance.
- Flag: Direct `syscall` package usage in platform code; should use `golang.org/x/sys`
- Review: Ensure portability across unix-like systems (linux, darwin, etc.)

**Stub implementations tracked** — Windows stubs without full implementations must have JIRA tickets and be documented in CLI help.
- Flag: Windows stub returning error ("not implemented on Windows") without JIRA reference
- Fix: Add comment linking to JIRA ticket and document limitation in CLI help

**Platform assumptions documented** — If code has platform-specific behavior (even if it "should" work), it must be tested and documented.
- Flag: Comment like "this should work on darwin too" without explicit testing
- Fix: Test on all platforms or document the assumption with JIRA ticket for future testing

## Dependency Injection & Architecture

**Test seams** — Constructor injection and setter methods are acceptable for small dependency sets. Flag if dependencies grow beyond 3-4 items.
- Good: `NewPhase(logger, client)` with 2 dependencies
- Flag: `NewPhase(logger, client, db, cache, transport, validator, ...)` growing uncontrollably
- Fix: If growing, consider grouping related dependencies into a config struct or interface

**Tight coupling** — Passing `*Engine` directly couples implementation to a concrete type, making tests brittle. Consider interfaces for better testability.
- Flag: Multiple functions taking `*Engine` as a parameter
- Consider: Interface `type EngineClient interface { Start() error; Stop() error }` for better test seams

## Code Organization

**File naming conventions** — Follow established patterns; don't invent new ones.
- Interactive commands (bubbletea): `model.go` (implements `tea.Model`)
- Sub-models: `<specific>Model.go` (e.g., `promptModel.go`, `hostModel.go`)
- Non-interactive render: stays in `cmd.go` or explicit `render.go` if splitting
- **Flag**: New conventions like `view.go` or `ui.go` without precedent in codebase

**File size limits** — If cmd + render logic exceeds ~350 lines, consolidate or split more carefully.
- Flag: Single file > 350 lines combining command logic and rendering
- Review: Check if split is justified or if consolidation is better

**Single responsibility** — Each phase function should have one clear job. If doing multiple things, split it.
- Flag: Phase function doing setup, execution, cleanup, logging, and retry logic
- Fix: Split into focused functions or use composition

**Platform-specific grouping** — If 2+ platform implementations exist, consider a subpackage.
- Good: `diskspace/unix.go`, `diskspace/windows.go`
- Better: `diskspace/` subpackage with `unix.go`, `windows.go`, `interface.go`

## Phase Orchestration (State Machines)

**Phase order clarity** — Phase execution order must be explicit and documented, not implicit.
- Flag: Phases passed as `...phases` varargs with implicit ordering assumptions
- Fix: Document phase ordering in comments or use named phase execution (`p1, p2, p3` in order)
- Review: Verify early exit from one phase doesn't hide cleanup needs in later phases

**Early exit safety** — If a phase can return early, verify all cleanup (locks, goroutines, temp files, deferred functions) still happens.
- Flag: Early return in phase without defer-based cleanup
- Risk: Orphaned goroutines, held locks, stale temp files, resource leaks

**Crash recovery** — Stale state detection must be obvious to users, not silent recovery.
- Flag: Silent recovery from incomplete phase (e.g., silently retrying a partial sync)
- Fix: Detect and report stale state; let user decide retry/recovery
- Good: "Previous sync incomplete. Run with --force to retry or --reset to start fresh."

**Three-way merge conflicts** — When comparing BASE vs LOCAL vs REMOTE, all scenarios must be explicitly handled.
- Flag: Missing cases in three-way comparison (BASE=X, LOCAL=Y, REMOTE=Z combinations)
- Review: Ensure all 8 combinations (modified/unmodified for each) are classified and handled

## File I/O & Rollback

**Disk space checks** — Pre-flight disk space validation must happen before any writes to prevent partial failures.
- Flag: First write without checking available disk space
- Fix: Validate disk space at phase start; fail fast if insufficient
- Good: `if !hasEnoughSpace(path, requiredBytes) { return fmt.Errorf("insufficient disk space") }`

**Rollback mechanism** — If backup-on-write is used, restore must be idempotent and tested.
- Flag: Restore logic that can fail on second call (not idempotent)
- Fix: Restore should be safe to call multiple times; test rollback scenarios

**File permissions & attributes** — File modes, symlinks, and attributes must be preserved through backup/restore cycles.
- Review: Check `os.Chmod`, symlink handling, extended attributes if applicable
- Test: Verify backup/restore preserves all file metadata

## Table Rendering

**Use lipgloss for tables** — Non-interactive list commands must use `charmbracelet/lipgloss/table`, not `text/tabwriter`.
- Flag: `text/tabwriter` used in new list commands
- Fix: Use `lipgloss.NewTable()` with `.Border(lipgloss.RoundedBorder())`

**Table styling** — Apply borders and use `tui.TableBorderStyle` with adaptive colors.
- Flag: Tables without borders or with hardcoded colors
- Fix: Use `.Border(lipgloss.RoundedBorder())` and `StyleFunc` with colors from `tui/styles.go`

**Reference existing patterns** — Check similar list commands before implementing tables.
- Review: `cmd/plugin/list`, `cmd/task/list`, `cmd/templates/list/model.go` for established patterns

**Adaptive colors** — Use `tui.GetAdaptiveColor(light, dark)` for dark mode support.
- Flag: Hardcoded colors that don't adapt to terminal theme
- Fix: All colors should respect light/dark theme preference

## Command Structure for List Commands

**Interactive vs display** — Determine if table needs user interaction (bubbletea) or just display (lipgloss).
- Interactive (selection, navigation): use bubbletea `tea.Model` in `model.go`
- Display-only: use lipgloss table rendering in `cmd.go`

**Pagination safety** — If paginating across multiple API calls, add safety checks (cross-host validation, context checks).
- Flag: Pagination without verifying consistency across pages
- Good: `assertNextOnSameHost()` to prevent pagination from jumping between different hosts

**Output modes** — Verify text and JSON output modes follow existing patterns.
- Review: Check how `cmd/plugin/list` handles `--format json` vs default text output

## Output Formatting & TUI Design System

**Use tui design system** — All styling must use `tui.SubTitleStyle`, `tui.BaseTextStyle`, `tui.DimStyle`, etc.
- Flag: Creating custom styles or hardcoding colors
- Fix: Reference `tui/styles.go` for all styling

**Text output tests** — Tests must verify **actual formatting** (spacing, colors, alignment), not just string presence.
- Flag: Tests only checking if output contains a string, not formatting
- Fix: Test rendered output, not just content presence

## Testing & Verification

**Race detector** — All concurrent code must pass tests with `-race` flag.
- This is mandatory via `task test`
- Flag: Tests not covering concurrent paths; race conditions won't be detected

**Error path coverage** — Unhappy paths must be tested, not just the happy path.
- Flag: No tests for error cases (timeouts, API failures, permissions, etc.)
- Fix: Test at least: missing resource (404), permission denied (403), timeout, network error

**Platform testing** — Platform-specific code must have platform-specific tests or documented assumptions.
- Flag: `_unix.go` without explicit darwin/linux testing
- Fix: Test on target platforms or document assumption with JIRA ticket

**Pagination tests** — Include cross-host safety checks in pagination tests.
- Verify: Pagination doesn't jump between hosts or contexts unexpectedly

## Red Flags — Critical Issues

- **Goroutines without panic recovery** → Silent panics cause data corruption
- **Lock errors ignored** → Stuck locks, broken mutual exclusion, hard-to-debug failures
- **"First error only" with hidden failures** → Multiple failure modes silenced; users can't diagnose
- **Platform assumptions without testing** → "Should work on darwin" breaks in production
- **Cryptic user errors** → "context deadline exceeded" is not actionable; wrap with context
- **Invented naming conventions** → `view.go`, `ui.go` don't exist in codebase; breaks consistency
- **Untracked stubs** → Windows stubs without JIRA tickets leave debt unmanaged
- **Tight coupling in tests** → Concrete types everywhere make tests brittle and hard to maintain
