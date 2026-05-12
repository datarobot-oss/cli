# Validation Patterns

Applies to: Validation code (`internal/**/validation.go`, schema checking, YAML validation)

## Inverted Validation Logic

**Rule**: Validation functions must return error only when validation fails. Flag code that returns error when validation passes.

**Scope**: Functions with return statements in `if len(errors) == 0` blocks

**What to flag**:
- `if len(errs) == 0 { return fmt.Errorf(...) }`
- Logic returns error for valid input
- Test expectations show valid configs being rejected

**Risk**: Valid configs are rejected, CLI fails to start, users can't use the tool.

**Fix**: Invert the logic
```go
// Bad - returns error when valid
if len(errs) == 0 {
    return fmt.Errorf("validation passed...")
}

// Good - returns error only when invalid
if len(errs) > 0 {
    return fmt.Errorf("validation failed: %v", errs)
}
return nil
```

---

## Unused Error Result (Dead Code)

**Rule**: Never create errors that aren't used. `fmt.Errorf()` results must be assigned, returned, or logged.

**Scope**: All function calls returning errors

**What to flag**:
- `fmt.Errorf(...)` on a line by itself (result discarded)
- Error creation that doesn't contribute to return value or logging
- Linting failure: "\_\_name evaluated but not used"

**Risk**: Blocks `task lint` CI check, prevents PR merge.

**Fix**: Use or remove the error
```go
// Bad - error created but discarded
fmt.Errorf("invalid field: %s", fieldName)

// Good - error returned
return fmt.Errorf("invalid field: %s", fieldName)

// Good - error logged
if err := someFunc(); err != nil {
    log.Errorf("operation failed: %v", err)
}
```

---

## Debug Logging in Production Hot Paths

**Rule**: Avoid `log.Warnf()` or `log.Debugf()` in validation success cases. These are "happy path" logs that add noise.

**Scope**: Log calls in validation success blocks

**What to flag**:
- `log.Warnf()` or `log.Debugf()` in `if len(errs) == 0` block
- Logging entire struct with `%+v` in validation code
- Verbose logging of successful validation results

**Fix**: Remove or restrict to actual errors
```go
// Bad - logs schema on every successful validation
if len(errs) == 0 {
    log.Warnf("schema validation passed: %+v", schema)
}

// Good - only log on errors
if len(errs) > 0 {
    log.Errorf("validation failed with %d errors", len(errs))
    return fmt.Errorf("invalid config: %w", errs[0])
}
```

---

## Commented-Out Return Statements

**Rule**: When correct logic is commented out, document why with a JIRA ticket or clear explanation.

**Scope**: Functions with `// return nil` or similar comments near error returns

**What to flag**:
- `// return nil` after `return fmt.Errorf(...)`
- Commented-out logic without explanation
- Suggests incorrect implementation is active

**Fix**: Document intent or fix the bug
```go
// Bad - no explanation
if len(errs) == 0 {
    return fmt.Errorf("...")
}
// return nil

// Good - documented with intent
if len(errs) > 0 {
    return fmt.Errorf("validation failed: %v", errs)
}
return nil  // All validations passed
```

---

## Custom Schema Validation vs. Libraries

**Rule**: Don't build custom validation logic for basic struct field validation. Use `go-playground/validator` or struct tags instead.

**Scope**: Custom validation packages

**What to flag**:
- 150+ lines of custom validation code
- Field-level validation rules implemented manually
- Multiple packages implementing similar validation patterns
- Required field checking, type validation, string constraints (length, patterns)

**When custom validation is justified**:
- Domain-specific rules (e.g., version compatibility, artifact state transitions)
- Complex multi-field dependencies
- Validation cannot be expressed as struct tags

**Fix**: Use struct tags for simple validation
```go
// Bad - custom validation code for basic checks
func Validate(v *Version) error {
    if v.Name == "" {
        return errors.New("name required")
    }
    if len(v.Name) > 100 {
        return errors.New("name too long")
    }
    // ... more custom code
}

// Good - struct tags + validator library
import "github.com/go-playground/validator/v10"

type Version struct {
    Name string `validate:"required,max=100"`
    // ...
}

validate := validator.New()
err := validate.Struct(v)
```

---

## Inconsistent Validation Approaches

**Rule**: When the same type of validation is done multiple ways across packages, consolidate the pattern.

**Scope**: Cross-package validation code

**What to flag**:
- Multiple packages implementing validation differently
- Some use custom schemas, others use struct tags, others use go-cmp
- No documented reason for different approaches

**Example anti-pattern**:
```
tools/validation.go    → Custom declarative schema
plugin/validation.go   → go-cmp structural comparison
envbuilder/validator.go → Ad-hoc custom error aggregation
```

**Fix**: Document the chosen approach and apply consistently
```
// VALIDATION STRATEGY (document in README)
// - Simple field validation: struct tags + go-playground/validator
// - Complex rules: custom validation in <package>/validation.go
// - Structural comparison: go-cmp for specific manifest types
```

---

## Struct Field Validation Without Tags

**Rule**: Structs with required or constrained fields should have validator tags documenting the rules.

**Scope**: Struct definitions with validation logic

**What to flag**:
- `type Config struct { ... }` with custom validation but no tags
- Required fields without `validate:"required"`
- String length limits not documented in tags

**Fix**: Add validator tags and document intent
```go
// Before - validation code scattered
type Config struct {
    Name string
    Age  int
}

// After - intent clear in struct definition
type Config struct {
    Name string `validate:"required,max=100" json:"name"`
    Age  int    `validate:"min=0,max=150" json:"age"`
}

// Document why certain constraints exist:
// Name: max=100 for database column limit, required for user feedback
// Age: min/max from business requirements for eligibility
```

---

## Unnecessary Struct Logging

**Rule**: Don't log entire complex structs to production logs. Log specific fields with context instead.

**Scope**: Log calls in validation and error handling

**What to flag**:
- `log.Infof("result: %+v", largeStruct)` in production code
- Full struct dumps in hot paths
- No user-facing value from struct logging

**Fix**: Log specific fields
```go
// Bad - logs entire schema struct
log.Warnf("schema loaded: %+v", schema)

// Good - logs relevant details
log.Infof("schema loaded for workspace: %s (v%s)", schema.WorkspaceID, schema.Version)
```

---

## Validation Test Coverage

**Rule**: Validation tests must cover both success AND failure paths. Don't just test valid inputs.

**Scope**: Test files for validation code (`*_test.go`)

**What to flag**:
- Tests only for valid inputs
- No error case tests
- Test names like `TestValidation_HappyPath` without unhappy path tests

**Fix**: Test both success and failure
```go
func TestVersionsValidation(t *testing.T) {
    // Valid case
    valid := &Config{Name: "my-app", Version: "1.0.0"}
    err := Validate(valid)
    assert.NoError(t, err)
}

func TestVersionsValidation_Invalid(t *testing.T) {
    // Missing required field
    invalid := &Config{Version: "1.0.0"}  // Name missing
    err := Validate(invalid)
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "name required")
}

func TestVersionsValidation_FieldConstraints(t *testing.T) {
    // Field too long
    invalid := &Config{Name: strings.Repeat("a", 101), Version: "1.0.0"}
    err := Validate(invalid)
    assert.Error(t, err)
}
```

---

## Validation Error Messages

**Rule**: Validation errors must be specific and actionable. Tell users which field failed and why.

**Scope**: Error messages in validation code

**What to flag**:
- Generic "validation failed" without field name
- No guidance on what's wrong or how to fix
- User can't determine which field caused the error

**Fix**: Provide context
```go
// Bad - user doesn't know what's wrong
return fmt.Errorf("invalid config")

// Good - specific and actionable
return fmt.Errorf("invalid config: field 'name' required (got empty string)")

// Better - provides context
return fmt.Errorf("invalid config: field 'name' (line 5) required for artifact registration")
```
