# Command Structure & Output

## Table Rendering

Use `lipgloss/table` with `tui.TableBorderStyle` for non-interactive lists — not `text/tabwriter`.

## Interactive vs Display-Only

Use bubbletea models for interactive UI; use lipgloss directly for display-only output.

## File Organization

Follow the standard file naming pattern: `cmd.go` for command logic, `model.go` for interactive models, and `render.go` for display rendering when splitting is needed.

## Output Consistency

Text and JSON output must contain identical data with consistent field naming (camelCase for JSON).

## Pagination Safety

Validate that pagination never crosses host boundaries.
