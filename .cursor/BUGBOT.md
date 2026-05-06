# Bugbot Rules for DataRobot CLI

This directory contains architecture and code review guidelines for the DataRobot CLI project. Rules are organized by scope and concern area.

## Rule Files

- **bugbot-concurrency.md** — Goroutine safety, error channels, WaitGroup patterns, channel closure
- **bugbot-errors.md** — Error wrapping, user-facing messages, logging, error handling patterns
- **bugbot-resources.md** — Lock/resource lifecycle, timeouts, cleanup, disk space checks
- **bugbot-platform.md** — Platform-specific code, build tags, syscalls, cross-platform testing
- **bugbot-cmd.md** — Command structure, table rendering, output formatting, TUI patterns
- **bugbot-internal.md** — Code organization, single responsibility, dependency injection, file I/O
- **bugbot-validation.md** — Validation logic, struct tags, error messages, test coverage
- **bugbot-testing.md** — Race detector, error paths, platform testing, test seams

## Overview

These rules reflect architectural patterns from two major PR reviews. They are ordered by impact: concurrency and error handling have the highest risk, while command structure and testing ensure consistency and reliability.

Run PR reviews against these standards to catch:
- Silent failures (goroutines, errors)
- Resource leaks (locks, goroutines, temp files)
- Cryptic user errors
- Platform-specific bugs
- Tight coupling in tests
- Inconsistent UI/output
