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

Intentional limitations must be tracked with JIRA tickets.

## README for Complex Packages

Foundational packages must have a README documenting design decisions, usage examples, and common pitfalls.
