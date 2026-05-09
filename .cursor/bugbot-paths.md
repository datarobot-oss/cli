# Path Handling & Filesystem Operations

## Path Validation vs Normalization Are Distinct

Path handling requires two separate steps: validation (security) and normalization (canonicalization).

## Backslash Rejection Is Cross-Platform Security

Explicitly reject backslashes at system boundaries — `path.Clean` does not catch backslash-based traversal on Unix.

## Unicode Normalization for Cross-Platform Map Keys

Paths used as map keys must be Unicode-normalized to NFC before use. See golang.org/x/text/unicode/norm.

## Document Path Format Contracts

Functions accepting paths must document the expected format (relative, forward slashes, normalized).

## Streaming Without In-Memory Buffering

Use streaming I/O for large file operations instead of buffering to memory.

## Case Collision Detection

Detect and handle case-insensitive path collisions explicitly.

## Symlink Handling and Platform Differences

Document symlink behavior assumptions and test them on all target platforms.
