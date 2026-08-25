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

## One File State, One Message

A file state both a read path and a write path can reject must produce one message from
one shared sentinel — not two phrasings that differ by which path reached the file first.

## Classification Errors Are Not User Messages

An error whose only consumer is a boolean — file-type predicates, "is this ours" checks — must
not be phrased as user guidance. Someone will later "align" it with a real message and put
a remedy into a string nothing prints.
