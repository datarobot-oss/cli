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

package wizard

import (
	"strings"

	"github.com/datarobot/cli/tui"
)

// The wizard's own styles, all drawn from the shared palette so it looks like
// the rest of the CLI rather than like a second product.
var (
	// questionStyle is the one line that says what is being asked.
	questionStyle = tui.InfoStyle.MarginLeft(2)

	// labelStyle names a field without competing with its value.
	labelStyle = tui.HintStyle

	// selectedStyle marks whatever is under the cursor — a menu option, the
	// advanced-options row, a picker row — in one yellow, so a selection reads
	// the same wherever the wizard shows a list of things to choose between.
	// Bold yellow foreground rather than the ticket's black-on-yellow bar: the
	// filled bar was tried and read as heavy/ugly against the neutral rows, so
	// the lighter yellow cue was kept. The pickers show it only because their
	// table cells are left uncoloured (see tableStyles): bubbles/table colours
	// each cell before wrapping the row, and a coloured cell's reset codes would
	// swallow this foreground. The yellow adapts to the terminal background so
	// the cue stays legible in both light and dark themes.
	selectedStyle = tui.BaseTextStyle.
			Foreground(tui.GetAdaptiveColor(tui.DrYellow, tui.DrYellowDark)).
			Bold(true)

	// noteStyle is the sentence explaining the question. Italic so it reads as
	// an aside rather than as another thing to answer.
	noteStyle = tui.HintStyle.MarginLeft(2).Italic(true)

	// filterStyle marks an active filter. It is a mode, not a label, so it
	// gets a colour nothing else on the screen uses.
	filterStyle = tui.BaseTextStyle.Foreground(tui.GetAdaptiveColor(tui.DrYellow, tui.DrYellowDark)).Bold(true)

	// liveStyle marks a value that came from the running workload rather than
	// from this project or from a documented default.
	liveStyle = tui.BaseTextStyle.Foreground(tui.TitleColor)

	// keyStyle is a keystroke in the legend; keyLabelStyle is what it does.
	keyStyle      = tui.BaseTextStyle.Foreground(tui.BorderColor).Bold(true)
	keyLabelStyle = tui.HintStyle
)

// The width the wizard lays out to before the terminal has told it anything,
// and the narrowest it will squeeze to afterwards. Both leave room for the
// two-column margin every screen uses.
const (
	fallbackWidth = 78
	minWidth      = 32
	marginColumns = 4
)

// contentWidth is how wide a wrapped paragraph may be. Notes are the only
// long prose on any screen, and letting them run past the edge is how a hint
// turns into a line the user has to guess the end of.
func contentWidth(terminalWidth int) int {
	if terminalWidth <= 0 {
		return fallbackWidth
	}

	return max(terminalWidth-marginColumns, minWidth)
}

// keyBinding is one entry in a screen's legend.
type keyBinding struct {
	// Key is shown in brackets, e.g. "enter".
	Key string
	// Does is the short phrase after it.
	Does string
}

// legend renders a screen's keys, the keystrokes picked out from what they
// do so the line can be skimmed for the one you want.
func legend(bindings ...keyBinding) string {
	parts := make([]string, 0, len(bindings))

	for _, b := range bindings {
		parts = append(parts, keyStyle.Render("["+b.Key+"]")+" "+keyLabelStyle.Render(b.Does))
	}

	return strings.Join(parts, keyLabelStyle.Render("  ·  "))
}
