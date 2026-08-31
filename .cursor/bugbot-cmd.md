# Command Structure & Output

## Table Rendering

Use `lipgloss/table` with `tui.TableBorderStyle` for non-interactive lists — not `text/tabwriter`.

## Interactive vs Display-Only

Use bubbletea models for interactive UI; use lipgloss directly for display-only output.

## File Organization

Follow the standard file naming pattern: `cmd.go` for command logic, `model.go` for interactive models, and `render.go` for display rendering when splitting is needed.

## Output Consistency

Text and JSON output must contain identical data with consistent field naming (camelCase for JSON).

### JSON output purity

When a command is invoked with `--output-format json` (or the deprecated `-o json` / `--format json`), **stdout must contain only valid JSON** — nothing else. This guarantees `dr <cmd> --output-format json | jq .` and `dr <cmd> --output-format json 2>&1 | jq .` both parse.

All non-JSON diagnostics must go to **stderr**, never stdout.

Prefer `outputformat.PrintJSONEnvelope` for structured output so the payload is always a single JSON object.

## Pagination Safety

Validate that pagination never crosses host boundaries.

## Count-Like Flags Validate at Parse Time

A numeric flag that carries a count (`--limit`, `--offset`, `--tail`, `--concurrency`) must
register through a `pflag.Value` (`internal/countflags`) so out-of-range input is rejected
during flag parsing, not with a runtime check inside `RunE` after the command has already
started.

## Zero-As-Default Flags Are a Documented Exception

A flag where `0` means "unset, use a default" (a port, a replica count) must not be forced
through `internal/countflags`' positive/non-negative validators — but the exception must be
called out at the flag definition, not left for a reviewer to guess.
