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

## Emoji in Terminal Output

Prefer emoji whose display width every layer (width library, terminal, font) agrees on:

- **Safe**: single codepoint, `East_Asian_Width=Wide`, no variation selector — e.g. `📦 🚀 🔀 🧰 🔧 🔐 📚 🧩 🔌`.
- **Avoid**: presentation-selector emoji (`U+FE0F` / `U+FE0E`) such as `⚙️ 🛠️ ✏️ 🗑️ 🏷️`. Width math counts them as 2 cells, but terminal fonts often substitute a narrow fallback glyph, collapsing the space after the emoji in help output.

Never rely on a variation selector to make a character "wide" — the selector picks the glyph, not its width. See UAX #11 (East Asian Width), UTS #51 (emoji presentation sequences), and Markus Kuhn's wcwidth notes for why display width is unspecified across terminals.
