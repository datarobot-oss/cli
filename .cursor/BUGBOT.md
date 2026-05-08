# Bugbot Rules for DataRobot CLI

## High-Risk Rules

Catch silent failures, resource leaks, data corruption, poor error handling, platform bugs:

- **bugbot-concurrency.md** — Goroutine safety, WaitGroup patterns, channel closure
- **bugbot-errors.md** — Error wrapping, user-facing messages, error handling patterns
- **bugbot-security.md** — Security boundaries, validation, threat models
- **bugbot-validation.md** — Input validation, error messages, test coverage

## Resource & Operations Rules

Prevent hangs, leaks, platform-specific bugs:

- **bugbot-resources.md** — Lock/resource lifecycle, timeouts, cleanup
- **bugbot-paths.md** — Path validation, normalization, Unicode, symlinks
- **bugbot-cross-platform.md** — Build tags, case sensitivity, line endings

## Design Rules

Prevent tight coupling, premature abstraction, architectural debt:

- **bugbot-design.md** — Code organization, separation of concerns, dependency injection
- **bugbot-package-design.md** — Contracts, API documentation, limitations

## Quality Rules

Ensure consistency and maintainability:

- **bugbot-testing.md** — Race detector, error paths, test seams, mocking
- **bugbot-cmd.md** — Table rendering, file organization, output consistency
