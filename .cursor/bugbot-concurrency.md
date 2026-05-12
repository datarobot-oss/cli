# Concurrency Patterns

## Goroutine Panic Recovery

Goroutines must have panic recovery to prevent silent failures.

## Error Channel Buffering

Error channel buffer size must match the number of goroutines writing to it.

## WaitGroup Cleanup

Every `wg.Add()` must have a corresponding `defer wg.Done()`, and `wg.Wait()` must be called after all goroutines are spawned.

## Channel Closure Safety

Channels must be closed only after all writers finish, and only by the sender.

## Multiple Errors from Goroutines

When multiple goroutines can fail, explicitly document the error handling strategy.
