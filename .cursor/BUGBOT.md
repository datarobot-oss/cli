# Bugbot Rules for DataRobot CLI

Organized by risk level: **High-Risk** (detailed, catch silent failures) → **Quality** (concise, enforce consistency).

## High-Risk Rules (Detailed)

Catch silent failures, resource leaks, data corruption, poor error handling, platform bugs:
- **bugbot-concurrency.md** (147 lines) — Goroutine safety, WaitGroup patterns, channel closure
- **bugbot-errors.md** (163 lines) — Error wrapping, user-facing messages, error handling patterns
- **bugbot-security.md** (306 lines) — Security boundaries, validation, threat models
- **bugbot-validation.md** (303 lines) — Input validation, error messages, test coverage

## Resource & Operations Rules (Medium Detail)

Prevent hangs, leaks, platform-specific bugs:
- **bugbot-resources.md** (173 lines) — Lock/resource lifecycle, timeouts, cleanup
- **bugbot-paths.md** (271 lines) — Path validation, normalization, Unicode, symlinks
- **bugbot-cross-platform.md** (298 lines) — Build tags, case sensitivity, line endings

## Design Rules (Medium Detail)

Prevent tight coupling, premature abstraction, architectural debt:
- **bugbot-design.md** (383 lines) — Code organization, separation of concerns, dependency injection
- **bugbot-package-design.md** (205 lines) — Contracts, API documentation, limitations

## Quality Rules (Concise)

Ensure consistency and maintainability:
- **bugbot-testing.md** (176 lines) — Race detector, error paths, test seams, mocking
- **bugbot-cmd.md** (51 lines) — Table rendering, file organization, output consistency

---

## How to Use

During PR review, reference rules by name:
- "This violates `bugbot-concurrency.md: Loop Variable Capture`"
- "Add test per `bugbot-testing.md: Error Path Coverage`"
- "Document contract per `bugbot-package-design.md`"

**High-risk rules** have detailed examples and "What to flag" sections. **Quality rules** are concise and reference existing patterns in the codebase.

---

## Quick Reference: What Gets Caught

| Category | Rules | Catches |
|----------|-------|---------|
| **High-Risk** | Concurrency, Errors, Security, Validation | Silent failures, data corruption, cryptic errors, tight security boundaries |
| **Resource/Ops** | Resources, Paths, Cross-Platform | Hangs, leaks, symlink loops, case collisions |
| **Design** | Design, Package Design | Tight coupling, premature abstractions, poor API contracts |
| **Quality** | Testing, Commands | Missing error paths, inconsistent output, brittle tests |

**Lines of code**: 2,476 total (original: 2,998). High-risk rules stay detailed; lower-risk rules simplified for quick reference.
