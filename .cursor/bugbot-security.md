# Security Boundaries & Constraints

## Security Boundaries Must Be Non-Overridable

Critical security constraints cannot be bypassed by user input and must be enforced before user pattern checks.

## Test Non-Overridable Constraints Explicitly

Write tests that attempt to override security constraints and verify they fail.

## Avoid Trust Assumptions Between Packages

Validate input at every package boundary — don't assume upstream packages provide safe data.

## Validate Integration Points Explicitly

Verify inter-package contracts with integration tests, not just assumptions.

## Minimize Package Coupling

Internal packages must be independent with no circular dependencies.

## Streaming Operations Must Document Timeout Behavior

Streaming operations with no timeout must document this explicitly and require callers to set a context deadline.
