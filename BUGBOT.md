# BUGBOT.md

Architectural review checklists to prevent common bugs and patterns. Use these during PR reviews and when implementing new features.

## Concurrency Patterns

- [ ] **Goroutine panics** — If goroutines can panic, add `recover()` to avoid silent failures
- [ ] **Error channels** — Verify the buffering matches actual error reporting (over-buffered channels waste memory)
- [ ] **"First error closes done" pattern** — If only returning one error, ensure early cancellation doesn't hide other failures
- [ ] **Loop variable capture** — Confirm `fa := fa` pattern used in goroutine loops (not `for _, fa := range ...` directly)
- [ ] **WaitGroup cleanup** — Verify no goroutine leaks; check `defer wg.Done()` and `wg.Wait()` pairing
- [ ] **Channel closure** — Ensure channels are closed only after all writers finish (`wg.Wait()` before `close()`)

## Error Handling

- [ ] **Silent error ignoring** — Watch for `_ = someFunc()` patterns; at minimum log errors
- [ ] **Cryptic error messages** — User-facing errors should be wrapped with context ("timeout after 300s" not "context deadline exceeded")
- [ ] **Multiple error scenarios** — If multiple errors can occur, clarify: return first, collect all, or fail-fast?
- [ ] **Error logging in orchestrators** — Phase orchestrators should log errors before returning them
- [ ] **Specialized error messages** — Distinguish error types (404 → "artifact X not found" vs generic API error)

## Lock & Resource Lifecycle

- [ ] **Lock acquisition tracking** — Trace when lock is acquired and released; verify no indefinite holds
- [ ] **Double-release safety** — If releasing multiple times, confirm it's idempotent or documented
- [ ] **Lock error handling** — Don't silently ignore lock acquisition/release failures
- [ ] **Timeout on locks** — Long-running phases holding locks should have timeouts
- [ ] **Lock cleanup** — Verify locks are properly released even in error paths (use defer for safety)

## Platform-Specific Code

- [ ] **Build tags present** — Files like `*_unix.go`, `*_windows.go` must have `//go:build` comments
- [ ] **Test on all platforms** — Don't assume `_unix.go` works on darwin; test explicitly
- [ ] **Consistent syscall usage** — Use `golang.org/x/sys/unix` instead of raw syscall for portability
- [ ] **Stub implementations** — Windows stubs should be tracked (JIRA ticket) and documented in CLI help
- [ ] **Identical signatures** — All platform-specific implementations must have matching function signatures
- [ ] **Platform assumptions documented** — If code has platform-specific behavior, it must be obvious to users (not silent)

## Dependency Injection & Architecture

- [ ] **Test seams** — Constructor injection + setter methods are acceptable for small dependency sets; flag if it grows
- [ ] **Tight coupling** — Passing `*Engine` as a parameter couples to concrete type; consider interfaces if testability is a concern
- [ ] **Function signature consistency** — Verify all platform implementations match expected signatures

## Code Organization

- [ ] **Platform-specific grouping** — If 2+ platform implementations exist, consider subpackage (e.g., `diskspace/`, `synclock/`)
- [ ] **Single-responsibility** — Each phase function should have one clear job
- [ ] **File naming for interactive commands** — Use `model.go` for `tea.Model` implementations, `<specific>Model.go` for sub-models
- [ ] **Render logic placement** — Non-interactive render logic stays in `cmd.go` or splits to `render.go` (don't introduce new conventions like `view.go`)
- [ ] **File size check** — If total cmd + render logic > 350 lines, consolidate or rename thoughtfully
- [ ] **Don't invent conventions** — Review existing patterns first (`cmd/plugin/list`, `cmd/task/list`, `cmd/templates/list/model.go`)

## Phase Orchestration (State Machines)

- [ ] **Phase order clarity** — Phase ordering should be explicit, not implicit in varargs (or well-documented)
- [ ] **Early exit safety** — If phases can return early, verify cleanup (locks, goroutines, temp files) still happens
- [ ] **Crash recovery** — Stale state detection should be obvious to users (not silent)
- [ ] **Three-way merge conflicts** — When comparing BASE vs LOCAL vs REMOTE, classify each scenario and handle appropriately

## File I/O & Rollback

- [ ] **Disk space checks** — Pre-flight validation before any writes (prevents partial failures)
- [ ] **Rollback mechanism** — If backup-on-write used, verify restore is idempotent and tested
- [ ] **File permissions** — Are file modes, symlinks, attributes preserved through backup/restore?
- [ ] **Idempotent restore** — Rollback should be safe to call multiple times

## Table Rendering

- [ ] **Use lipgloss table** — Non-interactive list commands use `charmbracelet/lipgloss/table`, not `text/tabwriter`
- [ ] **Add borders** — Tables should have borders (`.Border(lipgloss.RoundedBorder())`) and use `tui.TableBorderStyle`
- [ ] **Style with adaptive colors** — Use `StyleFunc` with colors from `tui/styles.go` (supports light/dark themes)
- [ ] **Reference existing patterns** — Check `cmd/plugin/list`, `cmd/task/list`, `cmd/templates/list/model.go` for examples
- [ ] **Verify output modes** — Check text vs JSON output follows existing patterns

## Command Structure for List Commands

- [ ] **Review similar commands first** — Study plugin, task, templates, component list commands
- [ ] **Interactive vs display** — Determine if table needs interactive selection (→ bubbletea) or just display (→ lipgloss)
- [ ] **Pagination safety** — If paginating across multiple API calls, add cross-host or context safety checks

## Output Formatting & TUI Design System

- [ ] **Use tui design system** — Apply `tui.SubTitleStyle`, `tui.BaseTextStyle`, `tui.DimStyle`, etc.
- [ ] **Adaptive colors** — Use `tui.GetAdaptiveColor(light, dark)` for dark mode support
- [ ] **Text output tests** — Verify **actual formatting**, not just string presence
- [ ] **Consistent styling** — All UI elements should use styles from `tui/styles.go`

## Testing & Verification

- [ ] **Race detector** — Tests should run with `-race` flag (Go's race detector catches many concurrency bugs)
- [ ] **Error path coverage** — Verify unhappy paths are tested, not just happy path
- [ ] **Platform testing** — Platform-specific code needs platform-specific tests or documented assumptions
- [ ] **Pagination tests** — Include cross-host safety checks in pagination tests
- [ ] **Comprehensive coverage** — Verify all output modes (text, JSON) are tested

## Red Flags — Don't Miss These

- **Goroutines + panic** → Silent success (data corruption risk)
- **Lock errors ignored** → Stuck locks in troubleshooting scenarios
- **"First error only" with no panic handling** → Multiple failure modes hidden
- **Platform assumptions** → "Should work on darwin" isn't good enough; test or document
- **Cryptic user errors** → "context deadline exceeded" needs wrapping for UX
- **New naming conventions** → `view.go` doesn't exist; use established patterns
- **Untracked stubs** → Windows stubs without JIRA tickets or documentation
- **Tight coupling in tests** — Using concrete types everywhere makes tests brittle
