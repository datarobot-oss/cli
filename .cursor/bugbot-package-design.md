# Package Documentation & Public API Design

## doc.go for Package Intent

Each new package must have a `doc.go` explaining its purpose, boundaries, and security considerations.

## Contracts Between Packages

Document the contract when one package calls another, including input format requirements and error behavior.

## Nil/Empty Returns

Document when a function returns nil/empty as a valid state vs an error.

## Failure Modes

Document how functions fail, especially for streaming, long-running, and resource-intensive operations.

## Limitations and Future Work

Intentional limitations must be tracked with JIRA issues. However in package documentation or in the codebase,
do NOT list issues by number. Just describe the limitation and the intended future work to address it.

## A New API Wrapper Extends drapi, Never Re-Implements It

A package adding typed request/response shapes or a caller-specific error envelope over a
DataRobot API surface (see `internal/drapi/filesapi`, `internal/workload/apiclient`) must build
entirely on `drapi`'s verb functions, `EndpointURL`, and `HTTPError`. If the new package
constructs its own `http.Client`, re-derives a timeout default, or defines a second error type
shaped like `HTTPError`, that's a sign it's duplicating `drapi` rather than extending it.

## Non-DataRobot-API Calls Do Not Belong in drapi

A request that isn't to the authenticated DataRobot platform API — a local OAuth-redirect
handshake, a plugin download, a third-party registry fetch — must not be routed through
`drapi`'s verb functions or forced through `AuthorizeRequest`/`URLMatchesConfiguredBase`. Those
exist to protect a bearer token that has no business being attached to an unrelated origin; a
plain, purpose-built client for that call is correct, not a gap.
