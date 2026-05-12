# Lock & Resource Lifecycle

Applies to: Internal packages with locks, file I/O, or long-running operations (`internal/**`)

## Lock Acquisition Must Be Traced

**Rule**: Locks must be acquired and released with clear, traceable paths. Verify no indefinite holds that could deadlock or hang the system.

**Scope**: Files with `sync.Mutex`, `sync.RWMutex`, or custom locking

**What to flag**:
- Lock acquired but release path unclear
- No `defer lock.Unlock()` protection
- Long-running phase holding lock without timeout
- Nested locks without clear ordering (deadlock risk)

**Fix**: Use defer and trace the critical section
```go
// Bad - unclear when lock is released
lock.Lock()
doWork()
lock.Unlock()

// Good - clear critical section
lock.Lock()
defer lock.Unlock()
doWork()
```

---

## Lock Error Handling

**Rule**: Lock acquisition/release errors must never be silently ignored. Errors indicate broken mutual exclusion or stuck locks.

**Scope**: Lock-heavy code paths

**What to flag**:
- `lock.Lock()` or `lock.Unlock()` return value ignored
- No error check on context-based lock acquisition
- Error in `defer lock.Unlock()` silently dropped

**Risk**: Silent lock failures lead to data races, corruption, and are nearly impossible to debug.

**Fix**: Check and log lock errors
```go
// Bad
lock.Unlock()

// Good
if err := lock.Unlock(); err != nil {
    log.Errorf("failed to release lock: %v", err)
}
```

---

## Timeouts on Long-Running Locks

**Rule**: Phases that hold locks for extended periods must have timeouts. Indefinite waits cause hangs.

**Scope**: Phase functions holding locks, long-running operations

**What to flag**:
- `lock.Lock()` in phase with no timeout context
- Phase can run indefinitely while holding lock
- No deadline on lock acquisition

**Fix**: Add context timeout
```go
ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
defer cancel()

if !lock.TryLockWithContext(ctx) {
    return fmt.Errorf("timeout acquiring lock after 30s")
}
defer lock.Unlock()
```

---

## Double-Release Safety

**Rule**: If a lock can be released multiple times, it must be idempotent or clearly documented as non-idempotent.

**Scope**: Code with multiple `Unlock()` calls or error paths releasing same lock

**What to flag**:
- Same lock released in multiple code paths (even with guards)
- No documentation of idempotency
- Panic possible if released twice

**Fix**: Ensure idempotency or prevent double-release
```go
// Option 1: Guard with flag
var released bool
if !released {
    lock.Unlock()
    released = true
}

// Option 2: Use defer (preferred)
lock.Lock()
defer lock.Unlock()
```

---

## Disk Space Checks Before Writes

**Rule**: Pre-flight disk space validation must happen before any writes. Partial writes on full disk cause data corruption.

**Scope**: Functions that write files (`internal/fileops/**`, etc.)

**What to flag**:
- First write without checking available disk space
- No disk space validation before phase execution
- Assuming disk has space available

**Fix**: Validate early
```go
if !hasEnoughSpace(path, requiredBytes) {
    return fmt.Errorf("insufficient disk space at %s: need %d bytes, have %d", 
        path, requiredBytes, availableBytes)
}
// Now safe to write
```

---

## Rollback Idempotency

**Rule**: Rollback/restore operations must be idempotent. It's safe to call them multiple times.

**Scope**: Backup/restore code, file rollback logic

**What to flag**:
- Restore that fails on second call
- No guard against double-restore
- Assumptions about file state during restore

**Fix**: Make restore idempotent
```go
// Bad - fails if called twice
os.Remove(backupPath)

// Good - idempotent
_ = os.Remove(backupPath)  // Safe to call multiple times
```

---

## File Permissions & Attributes Preserved

**Rule**: File modes, symlinks, and extended attributes must be preserved through backup/restore cycles.

**Scope**: File I/O code with backup/restore, sync operations

**What to flag**:
- `os.WriteFile()` without preserving original file mode
- Symlinks converted to regular files
- Extended file attributes dropped
- Owner/group information lost

**Fix**: Preserve metadata
```go
// Get original permissions
originalInfo, _ := os.Stat(originalPath)
originalMode := originalInfo.Mode()

// Write with original mode
os.WriteFile(backupPath, data, originalMode)
```
