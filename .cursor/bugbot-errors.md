# Error Handling Patterns

## Wrap User-Facing Errors with Context

User-facing errors must provide actionable context — not raw system errors.

## Specialize Error Messages by Type

Different error types (404, 403, 500) must produce different, specific user messages.

## Log Errors Before Returning in Orchestrators

Orchestrators must log errors before returning them to preserve context.

## Multiple Error Scenarios Must Be Explicit

When multiple errors can occur, document which error is returned and why.

## Avoid Cryptic Error Messages

Error messages must explain what operation failed and why, not just return the system error.
