# Lock & Resource Lifecycle

## Lock Acquisition Must Be Traced

Locks must be acquired and released with clear, traceable paths — always use `defer lock.Unlock()`.

## Lock Error Handling

Lock acquisition and release errors must never be silently ignored.

## Timeouts on Long-Running Locks

Phases that hold locks for extended periods must have context timeouts.

## Double-Release Safety

If a lock can be released multiple times, the operation must be idempotent or documented as non-idempotent.

## Disk Space Checks Before Writes

Validate available disk space before any writes.

## Rollback Idempotency

Rollback and restore operations must be idempotent.

## File Permissions & Attributes Preserved

File modes, symlinks, and extended attributes must be preserved through backup and restore cycles.
