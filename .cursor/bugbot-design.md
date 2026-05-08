# Architecture & Code Design

## Separation of Concerns: Layers and Dependencies

Domain-specific logic must be kept separate from generic utilities, with all layers depending inward only.

## Single Responsibility Per Function

Each function should have one clear job.

## Separate I/O from Logic

I/O functions must be separate from orchestration and business logic.

## Type-Driven Design: Function Signatures as Contracts

Use type aliases and interfaces to clarify intent and enable testing.

## Dependency Injection and Testability

Use interfaces for dependencies so they can be mocked; flag constructors with 7+ parameters.

## Function Signature Consistency Across Implementations

All implementations of the same functionality must have identical function signatures.

## Code Organization Within Packages

File organization must be logical, with related code in the same file or subpackage.

## Phase Orchestration Clarity

Phase execution order must be explicit and documented with comments explaining why.

## Library Choices Must Be Justified

New dependencies must be maintained, widely used, and necessary — justify in the PR.

## Duplication Patterns: Extract Only After Validation

Extract utility functions only after the same code appears in 2+ independent places.

## Scope Discipline: Foundational PRs

Foundational PRs should do architectural work without bundling unrelated refactorings.
