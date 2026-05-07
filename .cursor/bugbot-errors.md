# Error Handling Patterns

Applies to: All Go code with error handling

## Wrap User-Facing Errors with Context

**Rule**: User-facing errors must provide actionable context. System errors like "context deadline exceeded" are cryptic and unhelpful.

**Scope**: All functions returning errors visible to users

**What to flag**:
- Returning system error directly: `return ctx.Err()`
- Generic error messages: `return fmt.Errorf("API error: %v", err)`
- Missing operation context: "timeout" instead of "timeout waiting for X"
- Cryptic library errors exposed to users

**Fix**: Wrap with context
```go
// Bad
return ctx.Err()

// Good
if ctx.Err() != nil {
    return fmt.Errorf("timeout after %ds waiting for artifact sync: %w", timeoutSecs, ctx.Err())
}
```

---

## Specialize Error Messages by Type

**Rule**: Different error types need different user messages. A 404 is not the same as a 500; distinguish them clearly.

**Scope**: API call error handling, HTTP status code checks

**What to flag**:
- Generic "API error" for all status codes
- All HTTP errors treated the same
- No user-friendly message for specific errors (404, 403, timeout)

**Fix**: Check status and provide specific message
```go
// Bad
return fmt.Errorf("API request failed: %v", err)

// Good
if resp.StatusCode == 404 {
    return fmt.Errorf("artifact %q not found (check name and workspace)", artifactID)
}
if resp.StatusCode == 403 {
    return fmt.Errorf("permission denied: you don't have access to this artifact")
}
if resp.StatusCode == 500 {
    return fmt.Errorf("server error: DataRobot API is experiencing issues (try again in a few minutes)")
}
return fmt.Errorf("API request failed: %v", err)
```

---

## Never Silently Ignore Errors

**Rule**: Never use `_ = function()` without logging. Silent error ignores hide bugs.

**Scope**: All function calls that return errors

**What to flag**:
- `_ = someFunc()` with no log or surrounding comment
- `err := doSomething()` without checking or logging
- Error result ignored in defer or cleanup

**Exception**: Only ignore if you can explain why in a comment:
```go
// Safe to ignore: we're cleaning up anyway
_ = tempFile.Close()

// Bad: no explanation
_ = db.Close()
```

**Fix**: Log or return the error
```go
if err := cleanup(); err != nil {
    log.Warnf("cleanup failed (non-fatal): %v", err)
}
```

---

## Log Errors Before Returning in Orchestrators

**Rule**: Phase orchestrators should log errors before returning them. Logging only at the top level loses context about which phase failed.

**Scope**: Orchestrator functions, phase executors, main flow

**What to flag**:
- Phase function returns error without logging
- Error returned but context lost about which phase failed
- No logging between phase execution and error return

**Fix**: Log at the point of failure
```go
// Bad - context lost
if err := phase.Run(ctx); err != nil {
    return err
}

// Good - context preserved
if err := phase.Run(ctx); err != nil {
    log.Errorf("phase %s failed: %v", phase.Name(), err)
    return err
}
```

---

## Multiple Error Scenarios Must Be Explicit

**Rule**: When multiple errors can occur, document the strategy: return first error, collect all, or fail-fast? The choice must be explicit and documented.

**Scope**: Functions with multiple error sources (goroutines, retries, validations)

**What to flag**:
- Multiple error paths without documentation
- Unclear which error is returned
- No comment explaining error handling strategy

**Fix**: Document the strategy in comments
```go
// Returns the first validation error encountered.
// TODO: Consider collecting all errors for comprehensive user feedback.
func Validate(items []Item) error {
    for i, item := range items {
        if err := validateItem(item); err != nil {
            return fmt.Errorf("item %d: %w", i, err)
        }
    }
    return nil
}
```

---

## Avoid Cryptic Error Messages

**Rule**: Error messages should explain what operation failed and why, not just return the system error.

**Scope**: All user-facing error returns

**What to flag**:
- "context deadline exceeded" (what operation? how long?)
- "connection refused" (to what service?)
- "permission denied" (which permission? on what resource?)
- "invalid argument" (which argument? what's invalid about it?)

**Fix**: Add context
```go
// Bad
return err

// Good
return fmt.Errorf("failed to create artifact in workspace %q: %w", workspaceID, err)
```
