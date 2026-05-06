# Testing & Verification

Applies to: Test files and test-related code

## Race Detector Must Pass

**Rule**: All concurrent code must pass tests with the `-race` flag. This is mandatory and automatically runs via `task test`.

**Scope**: All `*_test.go` files with goroutines or channels

**What to flag**:
- Concurrent code without tests
- Tests not covering concurrent paths
- Race detector warnings (data races detected)
- Skipping race detector

**Fix**: Ensure concurrent tests pass with race detector
```go
// Run via: task test (includes -race)
// or: go test -race ./...

// Good test: exercises concurrent access
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

**Rule**: Unhappy paths must be tested, not just the happy path. Cover timeout, permissions, network errors, missing resources, etc.

**Scope**: All test files

**What to flag**:
- Only happy path tested
- No error cases in tests
- 100% coverage but only happy path exercised
- Comments like "TODO: test error case"

**Fix**: Test both success and failure
```go
func TestProcess(t *testing.T) {
    // Happy path
    err := Process(context.Background())
    assert.NoError(t, err)
}

// Add unhappy paths
func TestProcessTimeout(t *testing.T) {
    ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
    defer cancel()
    
    err := Process(ctx)
    assert.Error(t, err)
    assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestProcessPermissionDenied(t *testing.T) {
    // Mock client that returns 403
    client := &mockClient{statusCode: 403}
    err := ProcessWithClient(context.Background(), client)
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "permission denied")
}

func TestProcessMissingResource(t *testing.T) {
    // Mock client that returns 404
    client := &mockClient{statusCode: 404}
    err := ProcessWithClient(context.Background(), client)
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "not found")
}
```

**Error scenarios to test**:
- Timeout/context deadline
- Permission denied (403)
- Not found (404)
- Server error (500)
- Network error
- Invalid input
- Resource exhaustion (disk full, memory)

---

## Platform-Specific Testing

**Rule**: Platform-specific code must have platform-specific tests or documented assumptions.

**Scope**: `*_unix.go`, `*_windows.go` files and their tests

**What to flag**:
- Platform implementation without platform tests
- Tests only on primary development platform
- "Assume it works on X" without testing

**Fix**: Test on all target platforms
```go
// Option 1: Platform-specific test files
// unix_test.go
//go:build unix

func TestUnixGetDiskSpace(t *testing.T) {
    size, err := GetDiskSpace("/tmp")
    assert.NoError(t, err)
    assert.Greater(t, size, int64(0))
}

// windows_test.go
//go:build windows

func TestWindowsGetDiskSpace(t *testing.T) {
    size, err := GetDiskSpace("C:\\")
    assert.NoError(t, err)
    assert.Greater(t, size, int64(0))
}

// Option 2: Conditional tests
func TestGetDiskSpace(t *testing.T) {
    var path string
    switch runtime.GOOS {
    case "linux", "darwin":
        path = "/tmp"
    case "windows":
        path = "C:\\"
    default:
        t.Skip("platform not supported")
    }
    
    size, err := GetDiskSpace(path)
    assert.NoError(t, err)
    assert.Greater(t, size, int64(0))
}

// Option 3: Document limitation
// diskspace_windows.go
// TODO (DATAROBOT-12345): Add Windows disk space tests
```

---

## Test Seams and Mocking

**Rule**: Use interfaces and dependency injection to enable easy mocking. Tests should not require complex setup or hit real services.

**Scope**: All test files

**What to flag**:
- Tests making real API calls (should use mocks)
- Complex setup required to test a function
- Concrete type dependencies hard to mock
- Global state or singletons

**Fix**: Create test seams
```go
// Good test seam: interface-based dependency
type Client interface {
    Get(ctx context.Context, path string) ([]byte, error)
}

func Fetch(ctx context.Context, client Client, path string) (Data, error) {
    data, err := client.Get(ctx, path)
    // process data
    return data, nil
}

// Easy to mock
type mockClient struct {
    data []byte
    err error
}

func (m *mockClient) Get(ctx context.Context, path string) ([]byte, error) {
    return m.data, m.err
}

// Simple test
func TestFetch(t *testing.T) {
    client := &mockClient{data: []byte("test")}
    data, err := Fetch(context.Background(), client, "/path")
    assert.NoError(t, err)
    assert.Equal(t, "test", string(data))
}
```

---

## Pagination Test Coverage

**Rule**: Pagination logic must be tested, including edge cases and safety checks.

**Scope**: Tests for paginated list commands

**What to flag**:
- No pagination tests
- Only happy path tested (single page)
- No cross-host validation tests
- Missing edge case tests

**Fix**: Test pagination thoroughly
```go
func TestPaginationSinglePage(t *testing.T) {
    client := &mockClient{pages: []Page{{Items: items1}}}
    result, err := Paginate(context.Background(), client)
    assert.NoError(t, err)
    assert.Equal(t, len(items1), len(result))
}

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
    // Ensure pagination doesn't jump between hosts
    client := &mockClient{
        pages: []Page{
            {Items: items1, Host: "api1.example.com", NextToken: "page2"},
            {Items: items2, Host: "api2.example.com"}, // Different host!
        },
    }
    result, err := Paginate(context.Background(), client)
    assert.Error(t, err) // Should fail or detect
    assert.Contains(t, err.Error(), "host boundary")
}

func TestPaginationAPIError(t *testing.T) {
    client := &mockClient{err: errors.New("API error")}
    _, err := Paginate(context.Background(), client)
    assert.Error(t, err)
}
```

---

## Output Format Testing

**Rule**: Output format tests must verify actual formatting (spacing, colors, alignment), not just content presence.

**Scope**: Tests for table rendering and output formatting

**What to flag**:
- Tests only checking `assert.Contains(output, "keyword")`
- No tests for alignment, spacing, or colors
- JSON output tests only checking field names

**Fix**: Test actual formatting
```go
// Bad - only checks content
func TestTableRendering(t *testing.T) {
    output := renderTable(data)
    assert.Contains(t, output, "Column1")
    assert.Contains(t, output, "Value1")
}

// Good - checks formatting
func TestTableRendering(t *testing.T) {
    output := renderTable(data)
    
    // Verify structure
    lines := strings.Split(output, "\n")
    assert.GreaterOrEqual(t, len(lines), 3) // header, separator, data
    
    // Verify content
    assert.Contains(t, output, "Column1")
    
    // Verify alignment (simplified)
    assert.Contains(t, output, "Column1")
    assert.Contains(t, output, "Value1")
    
    // If JSON
    var result map[string]interface{}
    err := json.Unmarshal([]byte(output), &result)
    assert.NoError(t, err)
    assert.Equal(t, "Value1", result["column1"])
}

// Test JSON camelCase keys
func TestJSONOutput(t *testing.T) {
    output := renderJSON(data)
    var result map[string]interface{}
    json.Unmarshal([]byte(output), &result)
    
    // Keys should be camelCase, not snake_case
    assert.Contains(t, result, "artifactId")
    assert.NotContains(t, result, "artifact_id")
}
```
