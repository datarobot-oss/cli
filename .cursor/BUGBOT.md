# Bugbot Rules for DataRobot CLI

This directory contains architecture and code review guidelines for the DataRobot CLI project. Rules are organized by risk level and concern area.

## Rule Files (Organized by Risk & Impact)

**High-Risk Patterns** (catches silent failures, resource leaks, data corruption):
- **bugbot-concurrency.md** — Goroutine safety, error channels, WaitGroup patterns, channel closure
- **bugbot-errors.md** — Error wrapping, user-facing messages, logging, error handling patterns
- **bugbot-validation.md** — Validation logic, struct tags, error messages, test coverage
- **bugbot-security.md** — Security boundaries, non-overridable constraints, boundary testing

**Resource & Operations** (prevents hangs, leaks, and platform bugs):
- **bugbot-resources.md** — Lock/resource lifecycle, timeouts, cleanup, disk space checks
- **bugbot-paths.md** — Path validation, normalization, Unicode handling, symlinks
- **bugbot-cross-platform.md** — Cross-platform behavior, build tags, symlinks, case sensitivity, line endings

**Design & Architecture** (prevents tight coupling, premature abstraction, architectural debt):
- **bugbot-design.md** — Code organization, separation of concerns, dependency injection, phase orchestration, duplication patterns
- **bugbot-package-design.md** — Public API documentation, contracts between packages, doc.go, README, limitations

**Quality & Consistency** (ensures tests work, code is maintainable):
- **bugbot-testing.md** — Race detector, error paths, platform testing, test seams
- **bugbot-cmd.md** — Command structure, table rendering, output formatting, TUI patterns

## Quick Reference: What Gets Caught

These rules catch:
- Silent failures in goroutines and error handling
- Resource leaks (locks, goroutines, temp files)
- Data corruption from concurrent access
- Cryptic user errors and poor error handling
- Platform-specific bugs (symlinks, paths, case sensitivity)
- Tight coupling in tests and packages
- Inconsistent UI/output formatting
- Premature abstractions and scope creep
- Incomplete cross-platform implementations
- Poor API contracts between packages

## How to Use

During PR review, reference the relevant rule file by name:
- "This violates `bugbot-concurrency.md: Loop Variable Capture`"
- "Add error logging per `bugbot-errors.md: Never Silently Ignore Errors`"
- "Document the contract per `bugbot-package-design.md: Contracts Between Packages`"

These rules reflect patterns from DataRobot CLI development and are maintained as the architecture evolves.
