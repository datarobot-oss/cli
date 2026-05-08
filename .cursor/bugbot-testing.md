# Testing & Verification

Applies to: Test files and test-related code

## Race Detector Must Pass

**Rule**: All concurrent code must pass tests with the `-race` flag. This is mandatory and automatically runs via `task test`.

**What to flag**:
- Concurrent code without tests
- Tests not covering concurrent paths
- Race detector warnings (data races detected)

**Fix**: Exercise concurrent access in tests
```go
// Run via: task test (includes -race)
// or: go test -race ./...

func TestConcurrentSync(t *testing.T) {
    var wg sync.WaitGroup
    results := make([]error, 10)
    
    for i := 0; i < 10; i++ {
        wg.Add(1)
        go func(idx int) {
            defer wg.Done()
            results[idx] = sync.Run(context.Background())
        }(i)
    }
    
    wg.Wait()
    for _, err := range results {
        assert.NoError(t, err)
    }
}
```

---

## Error Path Coverage

**Rule**: Test unhappy paths, not just the happy path. Cover timeout, permissions, network errors, missing resources.

**Error scenarios to test**:
- Timeout/context deadline
- Permission denied (403)
- Not found (404)
- Server error (500)
- Network error
- Invalid input
- Resource exhaustion

**Fix**: Add error path tests
```go
func TestProcessTimeout(t *testing.T) {
    ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
    defer cancel()
    err := Process(ctx)
    assert.Error(t, err)
    assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestProcessPermissionDenied(t *testing.T) {
    client := &mockClient{statusCode: 403}
    err := ProcessWithClient(context.Background(), client)
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "permission denied")
}

func TestProcessMissingResource(t *testing.T) {
    client := &mockClient{statusCode: 404}
    err := ProcessWithClient(context.Background(), client)
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "not found")
}
```

---

## Test Seams and Mocking

Design code using **dependency inversion**: depend on abstractions (interfaces), not concrete types. This makes tests simple and code decoupled.

**Anti-pattern** (tight coupling to concrete types):
```go
// Bad: Function tightly coupled to concrete *http.Client
func Fetch(ctx context.Context, path string) ([]byte, error) {
    // Creates HTTP client internally; cannot mock in tests
    client := &http.Client{}
    resp, err := client.Get(ctx, path)
    // ...
}

// Test must make real HTTP calls
func TestFetch(t *testing.T) {
    data, err := Fetch(context.Background(), "/path")  // Real HTTP!
    assert.NoError(t, err)
}
```

**Correct pattern** (dependency inversion via interface):
```go
// Good: Depend on abstraction, not concrete type
type HTTPClient interface {
    Get(ctx context.Context, path string) ([]byte, error)
}

func Fetch(ctx context.Context, client HTTPClient, path string) ([]byte, error) {
    data, err := client.Get(ctx, path)
    // ...
}

// Test uses mock; no real HTTP calls
type mockClient struct { data []byte; err error }
func (m *mockClient) Get(ctx context.Context, path string) ([]byte, error) {
    return m.data, m.err
}

func TestFetch(t *testing.T) {
    client := &mockClient{data: []byte("test")}
    data, err := Fetch(context.Background(), client, "/path")
    assert.NoError(t, err)
}
```

**What to flag**: Real API calls in tests, complex setup, functions depending on concrete types (not interfaces), global state, creating dependencies internally instead of accepting them.

---

## Platform-Specific Testing

Platform implementations must be tested on target platforms, not assumed to work.

**Options**:
1. **Platform-specific test files**: `unix_test.go` (//go:build unix), `windows_test.go`
2. **Conditional tests**: Use `runtime.GOOS` to select test path, `t.Skip()` if not applicable
3. **Document**: If untested, note with JIRA ticket (e.g., "TODO (DATAROBOT-12345): Add Windows tests")

**What to flag**: Platform implementations without corresponding tests, "assume it works on X".

---

## Pagination Testing

Test single page, multiple pages, edge cases, and safety checks (host boundary validation).

```go
func TestPaginationMultiplePages(t *testing.T) {
    client := &mockClient{
        pages: []Page{
            {Items: items1, NextToken: "page2"},
            {Items: items2, NextToken: "page3"},
            {Items: items3},
        },
    }
    result, err := Paginate(context.Background(), client)
    assert.NoError(t, err)
    assert.Equal(t, len(items1)+len(items2)+len(items3), len(result))
}

func TestPaginationHostBoundary(t *testing.T) {
    client := &mockClient{
        pages: []Page{
            {Items: items1, Host: "api1.example.com", NextToken: "page2"},
            {Items: items2, Host: "api2.example.com"}, // Different host!
        },
    }
    _, err := Paginate(context.Background(), client)
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "host boundary")
}
```

**What to flag**: No pagination tests, only single-page tests, no cross-host validation tests.

---

## Output Format Testing

Text and JSON output tests must verify actual formatting, not just content presence.

**What to flag**: Tests only checking `assert.Contains(output, "keyword")`, no alignment/spacing tests, JSON tests only checking field names.

**Pattern**:
```go
// Verify structure and content
lines := strings.Split(output, "\n")
assert.GreaterOrEqual(t, len(lines), 3) // header, separator, data
assert.Contains(t, output, "Column1")

// Verify JSON camelCase keys
var result map[string]interface{}
json.Unmarshal([]byte(output), &result)
assert.Contains(t, result, "artifactId") // camelCase
assert.NotContains(t, result, "artifact_id") // not snake_case
```
