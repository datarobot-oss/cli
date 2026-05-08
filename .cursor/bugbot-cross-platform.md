# Cross-Platform Code & Behavior

## Build Tags Recommended

Platform-specific files may have `//go:build` comments to clarify intent, but are not required if the filename is unambiguous.

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

For cross-platform compatibility, prefer standard libraries instead of the `syscall` package. Prefer `syscall` over spawning subprocesses.

## Tracked Stubs for Incomplete Platforms

Incomplete platform implementations must be tracked with JIRA issues. JIRA issues may be referenced in code comments but should not be exposed to users.

## Platform Assumptions Must Be Explicit

Platform-specific behavior or assumptions must be explicit and tested.

## Cross-Platform Testing

Platform-specific code must be tested on all target platforms.
