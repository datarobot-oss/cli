# Testing & Verification

## Race Detector Must Pass

All concurrent code must pass tests with the `-race` flag.

## Error Path Coverage

Test unhappy paths (timeout, permissions, network errors, missing resources), not just the happy path.

## Test Seams and Mocking

Depend on interfaces, not concrete types, so dependencies can be mocked in tests.

## Platform-Specific Testing

Platform-specific implementations must be tested on target platforms.

## Pagination Testing

Test pagination for single page, multiple pages, edge cases, and host boundary validation.

## Output Format Testing

Output tests must verify actual formatting and structure, not just keyword presence.
