# Validation Patterns

## Inverted Validation Logic

Validation functions must return error only when validation fails, not when it passes.

## Unused Error Result (Dead Code)

`fmt.Errorf()` results must be assigned, returned, or logged — never discarded.

## Debug Logging in Production Hot Paths

Do not log in validation success cases.

## Commented-Out Return Statements

Commented-out return statements must have a documented reason or a JIRA ticket.

## Inconsistent Validation Approaches

Consolidate validation patterns — don't implement the same validation multiple ways across packages.

## Struct Field Validation Without Tags

Structs with required or constrained fields must have validator tags documenting the rules.

## Unnecessary Struct Logging

Do not log entire complex structs in production code — log specific fields with context.

## Validation Test Coverage

Validation tests must cover both success and failure paths.

## Validation Error Messages

Validation errors must name the field that failed and explain why.
