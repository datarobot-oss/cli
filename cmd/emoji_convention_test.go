// Copyright 2026 DataRobot, Inc. and its affiliates.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

// emoji_convention_test.go enforces the Terminal Emoji convention from
// .cursor/bugbot-cmd.md ("Emoji in Terminal Output") on cobra help output:
// only emoji whose display width every layer (width library, terminal, font)
// agrees on may appear in Short/Long/Example strings — single codepoint,
// East_Asian_Width=Wide, no presentation selector.
//
// Why: presentation-selector emoji (U+FE0F/U+FE0E, e.g. ⚙️ 🛠️ ✏️) are counted
// as 2 cells by width math, but terminal fonts often substitute a narrow
// fallback glyph, collapsing the space after the emoji in help output.
// Display width in terminals is unspecified by POSIX and decided independently
// by stale width tables and font fallbacks:
//
//   - UAX #11 (East Asian Width): https://www.unicode.org/reports/tr11/
//   - UTS #51 (emoji presentation sequences): https://www.unicode.org/reports/tr51/
//   - Markus Kuhn's wcwidth notes: https://www.cl.cam.ac.uk/~mgk25/ucs/wcwidth.c
//   - Jeff Quast's terminal survey: https://www.jeffquast.com/post/ucs-detect-test-results/
//   - jquast/wcwidth#19: https://github.com/jquast/wcwidth/issues/19
//
// The test walks the live command tree built by NewIsolatedRootFactory so any
// newly registered command is checked automatically. Runtime (non-help) strings
// — TUI models, task-list category headers, Taskfile templates — are out of
// scope for now. Known remaining variation-selector usage to handle separately:
//
//   - cmd/templates/setup/model.go, cmd/templates/clone/model.go (wizard UI)
//   - cmd/dotenv/helpers.go, internal/tools/prerequisites.go (⚠️ warnings)
//   - cmd/task/list/cmd.go (category headers, e.g. 🏗️)
//   - internal/task/Taskfile.tmpl.yaml, cmd/task/run/testdata/recipe/Taskfile.yml

import (
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"golang.org/x/text/width"
)

// emojiBlocks lists the Unicode blocks that contain pictographic emoji. The
// ranges are intentionally conservative: they flag glyphs a CLI would use as
// an icon, not every character a font might style.
var emojiBlocks = []struct {
	low  rune
	high rune
}{
	{0x2300, 0x23FF},   // Miscellaneous Technical (⌛, ⏱)
	{0x2600, 0x27BF},   // Miscellaneous Symbols + Dingbats (⚙, ✅, ➕)
	{0x2B00, 0x2BFF},   // Miscellaneous Symbols and Arrows (⬇)
	{0x1F000, 0x1FAFF}, // Supplemental Symbols and Pictographs (📦, 🚀, 🧰)
}

// isEmojiRune reports whether r falls in a pictographic emoji block.
func isEmojiRune(r rune) bool {
	for _, b := range emojiBlocks {
		if r >= b.low && r <= b.high {
			return true
		}
	}

	return false
}

// checkHelpString appends a violation for every emoji in s that terminals
// render inconsistently: presentation selectors, ZWJ sequences, and
// pictographs that are not East_Asian_Width=Wide.
func checkHelpString(path, field, s string, violations *[]string) {
	for _, r := range s {
		switch {
		case r == 0xFE0E || r == 0xFE0F:
			*violations = append(*violations,
				fmt.Sprintf("dr %s (%s): presentation selector U+%04X — pick a single-codepoint emoji instead", path, field, r))

		case r == 0x200D:
			*violations = append(*violations,
				fmt.Sprintf("dr %s (%s): ZWJ sequence U+200D — use a single-codepoint emoji instead", path, field))

		case isEmojiRune(r) && width.LookupRune(r).Kind() != width.EastAsianWide:
			*violations = append(*violations,
				fmt.Sprintf("dr %s (%s): emoji %c (U+%04X) is not East_Asian_Width=Wide", path, field, r, r))
		}
	}
}

// TestHelpStringsUseTerminalSafeEmoji fails with one aggregated message so a
// violation sweep can fix everything in a single pass.
func TestHelpStringsUseTerminalSafeEmoji(t *testing.T) {
	root := NewIsolatedRootFactory().Build().Command

	var violations []string

	// walk checks cmd and recurses into every subcommand, accumulating
	// violations instead of failing fast so the report lists all offenders.
	var walk func(cmd *cobra.Command)

	walk = func(cmd *cobra.Command) {
		path := strings.TrimPrefix(cmd.CommandPath(), root.Name())
		path = strings.TrimSpace(strings.TrimPrefix(path, " "))

		checkHelpString(path, "Short", cmd.Short, &violations)
		checkHelpString(path, "Long", cmd.Long, &violations)
		checkHelpString(path, "Example", cmd.Example, &violations)

		for _, sub := range cmd.Commands() {
			walk(sub)
		}
	}

	walk(root)

	if len(violations) > 0 {
		t.Errorf("help strings contain emoji that terminals render inconsistently:\n%s", strings.Join(violations, "\n"))
	}
}
