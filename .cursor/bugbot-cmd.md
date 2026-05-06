# Command Structure & Output

Applies to: Command implementations (`cmd/**`)

## Use Lipgloss for Table Rendering

**Rule**: Non-interactive list commands must use `charmbracelet/lipgloss/table`, not `text/tabwriter`. Lipgloss provides modern styling and consistent formatting.

**Scope**: List commands, table rendering in `cmd/**`

**What to flag**:
- `text/tabwriter` used in new list commands
- Raw string formatting for tables
- Hardcoded spacing without proper table library

**Fix**: Use lipgloss table
```go
import "github.com/charmbracelet/lipgloss/table"

t := table.New().
    Border(lipgloss.RoundedBorder()).
    StyleFunc(func(row, col int) lipgloss.Style { /* ... */ }).
    Rows(rows...)
fmt.Println(t.Render())
```

**Reference patterns**: 
- `cmd/plugin/list`
- `cmd/task/list`
- `cmd/templates/list/model.go`

---

## Table Styling with Adaptive Colors

**Rule**: Tables must use `tui.TableBorderStyle` with adaptive colors from `tui/styles.go`. Colors must respect light/dark terminal themes.

**Scope**: All table rendering

**What to flag**:
- Hardcoded colors (e.g., `lipgloss.Color("#FF0000")`)
- No `.Border()` call
- Not using `tui` design system styles
- Colors that don't work in both light and dark themes

**Fix**: Use adaptive colors
```go
import "github.com/datarobot/cli/internal/tui"

t := table.New().
    Border(tui.TableBorderStyle).
    StyleFunc(func(row, col int) lipgloss.Style {
        return tui.GetAdaptiveColor(lipgloss.Color("lightGray"), lipgloss.Color("darkGray"))
    })
```

**Available styles**: `tui.BaseTextStyle`, `tui.SubTitleStyle`, `tui.DimStyle`, etc. in `tui/styles.go`

---

## Interactive vs Display-Only Tables

**Rule**: Determine if table needs user interaction. Interactive selection uses bubbletea; display-only uses lipgloss.

**Scope**: New list/table commands

**What to flag**:
- Using bubbletea when display-only would suffice
- Using lipgloss for interactive selection
- Unclear whether selection is needed

**Interactive (bubbletea)**:
- File: `model.go` implementing `tea.Model`
- Use when: user selects, navigates, filters table
- Example: `cmd/templates/list/model.go`

**Display-only (lipgloss)**:
- File: stays in `cmd.go` or `render.go`
- Use when: just showing data
- Example: `cmd/plugin/list`

---

## Pagination Safety Checks

**Rule**: When paginating across multiple API calls, add safety checks. Ensure pagination doesn't jump between hosts or lose context.

**Scope**: Paginated list commands

**What to flag**:
- Pagination without context checks
- No validation between pages
- Possible host switching mid-pagination

**Fix**: Add cross-host validation
```go
// Example: assertNextOnSameHost validates pagination consistency
func paginate(ctx context.Context, client Client) ([]Item, error) {
    var items []Item
    nextToken := ""
    currentHost := ""
    
    for {
        result, err := client.List(ctx, nextToken)
        if err != nil {
            return nil, err
        }
        
        // Validate pagination consistency
        if currentHost == "" {
            currentHost = result.Host
        } else if result.Host != currentHost {
            return nil, fmt.Errorf("pagination crossed host boundary: %s -> %s", 
                currentHost, result.Host)
        }
        
        items = append(items, result.Items...)
        if result.NextToken == "" {
            break
        }
        nextToken = result.NextToken
    }
    
    return items, nil
}
```

---

## Output Mode Consistency

**Rule**: Text and JSON output modes must follow existing patterns. Verify output structure matches similar commands.

**Scope**: Commands with multiple output formats

**What to flag**:
- Custom JSON structure not matching existing patterns
- Missing `--format json` support when expected
- Text and JSON output inconsistent (different fields)

**Fix**: Match existing patterns
```go
// Reference: cmd/plugin/list for output mode examples

// Text output: use tui formatting
fmt.Println(prettyFormat(items))

// JSON output: camelCase keys, consistent structure
jsonBytes, _ := json.Marshal(items)
fmt.Println(string(jsonBytes))
```

---

## File Organization for Interactive Commands

**Rule**: Interactive commands (bubbletea) use specific file names; non-interactive render logic follows established conventions.

**Scope**: Command structure in `cmd/**`

**What to flag**:
- New naming conventions like `view.go`, `ui.go`, `render.go` without precedent
- Interactive logic mixed with command definition
- Inconsistent file naming across similar commands

**Correct naming**:
- `cmd.go` — Command definition and main logic
- `model.go` — `tea.Model` implementation for interactive UI
- `<specific>Model.go` — Sub-models (e.g., `promptModel.go`, `hostModel.go`)
- `render.go` — Only if splitting non-interactive render from `cmd.go` (must be consistent with codebase)

**Exception**: Don't invent `view.go` or `ui.go` unless they're already used elsewhere in the codebase.

---

## File Size Limits

**Rule**: If cmd + render logic exceeds ~350 lines total, consolidate or split more carefully.

**Scope**: Command files in `cmd/**`

**What to flag**:
- Single file > 350 lines combining command and rendering
- Unclear split justification

**Fix**: Either consolidate or thoughtfully split
```go
// Option 1: Consolidate into cmd.go if simple
// Option 2: Split if clear separation:
//   - cmd.go: command definition, flags, main logic
//   - model.go: interactive bubbletea Model
//   - render.go: explicit rendering function (if needed)
```

---

## Style System Consistency

**Rule**: All styling must use `tui` design system. No hardcoded colors or custom style definitions.

**Scope**: Output formatting in commands

**What to flag**:
- Creating custom `lipgloss.Style` variables
- Hardcoded `lipgloss.Color("#...")`
- Not using `tui.SubTitleStyle`, `tui.DimStyle`, etc.

**Fix**: Use tui styles
```go
import "github.com/datarobot/cli/internal/tui"

// Use provided styles
fmt.Println(tui.SubTitleStyle.Render("Header"))
fmt.Println(tui.DimStyle.Render("Dimmed text"))

// For custom styling, use adaptive colors
customStyle := lipgloss.NewStyle().
    Foreground(tui.GetAdaptiveColor(lightColor, darkColor))
```
