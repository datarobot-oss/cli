# Command Structure & Output

Quick reference for CLI command conventions. Lower-risk; enforce for consistency.

## Table Rendering

- **Non-interactive lists**: Use `lipgloss/table` with `tui.TableBorderStyle`, not `text/tabwriter`
- **Adaptive colors**: Always use `tui.GetAdaptiveColor()` for light/dark theme support
- **Available styles**: `tui.BaseTextStyle`, `tui.SubTitleStyle`, `tui.DimStyle` in `tui/styles.go`
- **References**: `cmd/plugin/list`, `cmd/task/list`, `cmd/templates/list/model.go`

```go
t := table.New().
    Border(tui.TableBorderStyle).
    StyleFunc(func(row, col int) lipgloss.Style {
        return tui.GetAdaptiveColor(lipgloss.Color("light"), lipgloss.Color("dark"))
    }).
    Rows(rows...)
fmt.Println(t.Render())
```

## Interactive vs Display-Only

| Need | Use | File | Example |
|------|-----|------|---------|
| User selection/navigation | bubbletea | `model.go` (implements `tea.Model`) | `cmd/templates/list/model.go` |
| Just showing data | lipgloss | `cmd.go` or `render.go` | `cmd/plugin/list` |

## File Organization

| File | Purpose |
|------|---------|
| `cmd.go` | Command definition, flags, main logic |
| `model.go` | `tea.Model` implementation (interactive UI only) |
| `<feature>Model.go` | Sub-models (`promptModel.go`, `hostModel.go`) |
| `render.go` | Non-interactive rendering (only if splitting needed) |

**Size limit**: If cmd + render exceeds ~350 lines total, consolidate or split more carefully.

## Output Consistency

- **Text**: Use tui formatting and styles
- **JSON**: camelCase keys, match structure across similar commands
- **Both modes must have identical data** (same fields, just different format)

## Pagination Safety

When paginating API results:
- Validate pagination consistency (no host boundary crossing)
- Example: `assertNextOnSameHost()` validates pagination doesn't jump hosts
- Flag any context switches mid-pagination
