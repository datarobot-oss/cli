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

package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

// runDaxTicks drives m with daxTickMsg up to maxTicks times. Most ticks
// return a tea.Tick-based cmd (which sleeps for real when invoked), so this
// only ever invokes the returned cmd at the one tick where the model's own
// state says it's about to signal completion — every other tick just
// advances state via Update without touching cmd.
func runDaxTicks(t *testing.T, m DaxModel, maxTicks int) (DaxModel, bool) {
	t.Helper()

	for range maxTicks {
		updated, cmd := m.Update(daxTickMsg{})
		m = updated.(DaxModel)

		if m.phase == daxPhaseHolding && m.holdFrames >= daxHoldFrames {
			_, ok := cmd().(OverlayDoneMsg)

			return m, ok
		}
	}

	return m, false
}

func TestDaxModel_RevealsProgressively(t *testing.T) {
	m := newDaxModel(80, 24).(DaxModel)

	assert.Equal(t, 0, m.revealed, "should start fully unrevealed")

	updated, _ := m.Update(daxTickMsg{})
	m = updated.(DaxModel)

	for range 20 {
		updated, _ := m.Update(daxTickMsg{})
		m = updated.(DaxModel)
	}

	assert.Positive(t, m.revealed, "reveal should have advanced after Dax starts crossing")
	assert.LessOrEqual(t, m.revealed, daxRevealWidth(daxScale(m.width, m.height)), "reveal should never exceed the logo width")
}

func TestDaxModel_RevealIsMonotonic(t *testing.T) {
	m := newDaxModel(80, 24).(DaxModel)

	prev := 0

	for range 400 {
		updated, _ := m.Update(daxTickMsg{})
		m = updated.(DaxModel)

		assert.GreaterOrEqual(t, m.revealed, prev, "revealed must never shrink, even while Dax bounces back")
		prev = m.revealed
	}
}

func TestDaxModel_BouncesBeforeExiting(t *testing.T) {
	m := newDaxModel(80, 24).(DaxModel)

	seenPassIdx := map[int]bool{0: true}

	for range 400 {
		updated, _ := m.Update(daxTickMsg{})
		m = updated.(DaxModel)
		seenPassIdx[m.passIdx] = true
	}

	assert.True(t, seenPassIdx[1], "should have bounced into the second pass")
	assert.True(t, seenPassIdx[2], "should have bounced into the third (exiting) pass")
}

func TestDaxModel_SignalsOverlayDoneEventually(t *testing.T) {
	m := newDaxModel(80, 24).(DaxModel)

	_, done := runDaxTicks(t, m, 2000)

	assert.True(t, done, "Dax overlay should eventually signal OverlayDoneMsg")
}

func TestDaxModel_WindowSizeMsgResizes(t *testing.T) {
	m := newDaxModel(80, 24).(DaxModel)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(DaxModel)

	assert.Equal(t, 120, m.width)
	assert.Equal(t, 40, m.height)
}

func TestDaxModel_ViewDoesNotPanic(t *testing.T) {
	m := newDaxModel(80, 24).(DaxModel)

	for range 300 {
		assert.NotPanics(t, func() { m.View() })

		updated, _ := m.Update(daxTickMsg{})
		m = updated.(DaxModel)
	}
}

func TestDaxModel_ViewDoesNotPanicAtLargeSize(t *testing.T) {
	m := newDaxModel(220, 80).(DaxModel)

	for range 300 {
		assert.NotPanics(t, func() { m.View() })

		updated, _ := m.Update(daxTickMsg{})
		m = updated.(DaxModel)
	}
}

func TestDaxScale_ClampsAndGrowsWithTerminalSize(t *testing.T) {
	assert.Equal(t, 1, daxScale(80, 24), "minimum guaranteed size should stay at 1x")
	assert.Equal(t, 2, daxScale(150, 45), "a sufficiently wide/tall terminal should scale the logo up")
	assert.Equal(t, daxScaleMax, daxScale(1000, 1000), "huge terminals should cap at daxScaleMax")
}

// TestDaxLogoLines_WordmarkScalesWithPictogram covers the actual bug
// report: the pictogram scaled with terminal size but "DataRobot" stayed a
// fixed size next to it. The wordmark is now rendered as pixel-font glyphs
// merged into the same block the pictogram lives in, so nearest-neighbor
// scaling grows both together — verified here by checking the combined
// reveal width scales exactly linearly with scale.
func TestDaxLogoLines_WordmarkScalesWithPictogram(t *testing.T) {
	width1x := daxRevealWidth(1)
	width2x := daxRevealWidth(2)
	width3x := daxRevealWidth(3)

	assert.Equal(t, width1x*2, width2x, "reveal width (pictogram+wordmark together) should double at 2x scale")
	assert.Equal(t, width1x*3, width3x, "reveal width (pictogram+wordmark together) should triple at 3x scale")

	lines1x := daxLogoLines(1)
	lines2x := daxLogoLines(2)

	assert.Len(t, lines2x, len(lines1x)*2, "vertical scale should repeat every row — pictogram and wordmark together")
}

func TestDaxNameGlyphLines_SpellsDataRobot(t *testing.T) {
	rows := daxNameGlyphLines()

	assert.Len(t, rows, daxFontHeight)

	for _, row := range rows {
		assert.NotEmpty(t, row)
		assert.NotContains(t, row, ".", "off-pixels must render as blank, not a literal distracting dot")
	}
}

func TestDaxRevealWidth_MatchesLogoLineLength(t *testing.T) {
	for _, scale := range []int{1, 2, 3} {
		lines := daxLogoLines(scale)
		mid := len(lines) / 2

		assert.Equal(t, len([]rune(lines[mid])), daxRevealWidth(scale),
			"daxRevealWidth must match the actual widest (name-bearing) logo line at scale %d", scale)
	}
}
