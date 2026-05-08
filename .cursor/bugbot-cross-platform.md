# Cross-Platform Code & Behavior

## Build Tags Required

Platform-specific files must have `//go:build` comments.

## Identical Function Signatures Across Platforms

All platform implementations must have identical function signatures.

## Document Cross-Platform Behavior Differences

If behavior differs across platforms (symlinks, paths, line endings), document it explicitly.

## Symlink Handling Across Platforms

Symlink handling must be consistent and documented across all platforms.

## Case Sensitivity and Path Collisions

Enforce consistent case-collision validation on all platforms.

## Line Ending Handling

Document and enforce a consistent line-ending strategy across platforms.

## Syscall Portability

Use `golang.org/x/sys/unix` instead of the raw `syscall` package.

## Tracked Stubs for Incomplete Platforms

Incomplete platform implementations must be tracked with JIRA tickets and documented in CLI help without exposing ticket numbers to users.

## Platform Assumptions Must Be Explicit

Platform-specific behavior or assumptions must be explicit and tested.

## Cross-Platform Testing

Platform-specific code must be tested on all target platforms.
