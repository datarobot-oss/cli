# Security Boundaries & Constraints

## Security Boundaries Must Be Non-Overridable

Critical security constraints cannot be bypassed by user input and must be enforced before user pattern checks.

## Test Non-Overridable Constraints Explicitly

Write tests that attempt to override security constraints and verify they fail.

## Avoid Trust Assumptions Between Packages

Validate input at every package boundary — don't assume upstream packages provide safe data.

Worked example: `docs/development/drapi-client.md` — before attaching a bearer token to
a server-supplied URL (pagination cursor, live endpoint), verify it with
`drapi.URLMatchesConfiguredBase()` first.

## Local Callback Listeners Must Validate Request Provenance

A listener bound to localhost for an OAuth-style handoff is reachable by any open web page —
localhost has no same-origin restriction. It must check something a page can't forge
(`Sec-Fetch-Dest`, a nonce) before treating a request as authentic, not just accept whatever
hits the port.

## Security Mitigation Claims Must Name the Residual Bypass

A doc or comment describing a security fix must scope its claim to the exact vectors it blocks
and name what still gets through — not claim a blanket guarantee the fix doesn't provide.

## Validate Integration Points Explicitly

Verify inter-package contracts with integration tests, not just assumptions.

## Minimize Package Coupling

Internal packages must be independent with no circular dependencies.

## Streaming Operations Must Document Timeout Behavior

Streaming operations with no timeout must document this explicitly and require callers to set a context deadline.
