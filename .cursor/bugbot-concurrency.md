# Concurrency Patterns

Applies to: All Go code with goroutines or channels

## Goroutine Panic Recovery

**Rule**: Goroutines must have panic recovery. Silent panics from concurrent code cause data corruption and are nearly impossible to debug.

**Scope**: Any file with `go func()` or goroutine spawning

**What to flag**:
- Goroutine without `recover()` block
- Panic in goroutine caught but not logged
- Missing panic handling in long-running goroutines

**Example fix**:
```go
go func() {
    defer func() {
        if r := recover(); r != nil {
            log.Errorf("goroutine panic: %v", r)
        }
    }()
    // goroutine work here
}()
```

---

## Loop Variable Capture in Goroutines

**Rule**: Loop variables passed to goroutines must be captured with `fa := fa` pattern. Direct capture causes all goroutines to reference the last iteration value.

**Scope**: Loop with goroutine spawning inside

**What to flag**:
- `for _, fa := range slice { go func() { use(fa) }() }`
- No variable capture before goroutine spawn

**Correct pattern**:
```go
for _, fa := range slice {
    fa := fa  // Capture loop variable
    go func() { use(fa) }()
}
```

---

## Error Channels Buffering

**Rule**: Error channel buffer size must match the expected number of goroutines writing to it. Over-buffering wastes memory; under-buffering deadlocks.

**Scope**: Files with `make(chan error, ...)`

**What to flag**:
- `errChan := make(chan error, 10)` when only 2 goroutines write to it
- `errChan := make(chan error)` when 5+ goroutines write to it
- Channel capacity doesn't match documented error producers

**Fix**: Make capacity explicit and match goroutine count
```go
// 3 goroutines produce errors
errChan := make(chan error, 3)
wg.Add(3)
for i := 0; i < 3; i++ {
    go func() { defer wg.Done(); errChan <- work() }()
}
wg.Wait()
close(errChan)
```

---

## WaitGroup Cleanup

**Rule**: Every `wg.Add()` must have a corresponding `defer wg.Done()`. Verify `wg.Wait()` is called after all goroutines are spawned.

**Scope**: Files with `sync.WaitGroup`

**What to flag**:
- `wg.Add()` without corresponding `wg.Done()` in spawned goroutine
- `wg.Done()` not deferred (can leak if goroutine panics)
- `wg.Wait()` before all goroutines added
- Mismatched Add/Done counts

**Fix**:
```go
wg := sync.WaitGroup{}
wg.Add(1)
go func() {
    defer wg.Done()
    // work
}()
wg.Wait()  // After all goroutines spawned
```

---

## Channel Closure Safety

**Rule**: Channels must be closed only after all writers finish. Only the sender should close channels. Never close a channel that receivers are reading from.

**Scope**: Files with `close(chan)` or goroutines writing to channels

**What to flag**:
- Closing channel before `wg.Wait()` completes
- Receiver closing a channel
- Multiple goroutines closing the same channel
- Closing channel while goroutines still writing

**Fix**:
```go
wg.Wait()       // Ensure all writers done
close(errChan)  // Then close
for err := range errChan {
    // safe to read
}
```

---

## Multiple Errors from Goroutines

**Rule**: When multiple goroutines can fail, be explicit about error handling: return first error, collect all, or fail-fast? Don't silently hide concurrent failures.

**Scope**: Functions spawning multiple goroutines with error handling

**What to flag**:
- Only first error returned when multiple can occur
- Errors silently dropped in goroutine
- No documentation of error handling strategy

**Fix**: Document strategy in comments
```go
// Returns first error encountered; other failures are logged
// TODO: Consider collecting all errors for better debugging
var firstErr error
for err := range errChan {
    if firstErr == nil {
        firstErr = err
    } else {
        log.Warnf("additional error: %v", err)
    }
}
return firstErr
```
