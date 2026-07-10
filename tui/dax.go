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
	"math"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// daxGridBase is Dax's pixel-art sprite, reverse-engineered from the rect
// coordinates in dax-assets/datarobot-DAX-yellow.svg (a 12x13 grid of 17px
// square cells). '#' is a full-opacity fill rect, '.' is one of the
// half-opacity shading rects the source art uses for the belt and feet,
// ' ' is empty. Terminal character cells are roughly twice as tall as they
// are wide, so rendering one character per source cell squashes Dax
// horizontally — daxGrid (below) doubles each column to compensate.
var daxGridBase = []string{
	"  .######.   ",
	"  #      #   ",
	" #  #  #  #  ",
	" #  #  #  #  ",
	"  #      #   ",
	"  .######.   ",
	"             ",
	"  #  ##  #   ",
	"  #      #   ",
	"  .      .   ",
	"    .  .     ",
	"  ###  ###   ",
}

// daxGrid doubles every column of daxGridBase so Dax's proportions read
// correctly against a monospace font's ~1:2 width:height cell ratio.
var daxGrid = doubleColumns(daxGridBase)

func doubleColumns(rows []string) []string {
	doubled := make([]string, len(rows))

	for i, row := range rows {
		var sb strings.Builder

		for _, ch := range row {
			sb.WriteRune(ch)
			sb.WriteRune(ch)
		}

		doubled[i] = sb.String()
	}

	return doubled
}

const (
	daxSpriteWidth  = len("  .######.   ") * 2
	daxSpriteHeight = 12

	daxPictogramWidth = 19 // widest bar in Banner's pictogram portion
	daxNameGap        = 3
	daxName           = "DataRobot"
	daxGlyphGap       = 1 // columns between letters in the pixel font

	daxLeftMargin  = "  "
	daxRowGapBelow = 1

	// Baseline the logo is designed at 1x. Dax's bounce area always fills
	// the whole terminal, so on anything bigger than a minimal window a
	// 1x logo reads as tiny and gets swept past almost instantly.
	daxScaleBaseHeight  = 15
	daxScaleMax         = 3
	daxWidthFitFraction = 0.9 // reveal content should use at most this much of the terminal width
)

// daxFont is a tiny 5-row pixel font covering just the letters "DataRobot"
// needs. A plain text name can't be nearest-neighbor scaled like the
// pictogram (repeating characters would spell it wrong), so the wordmark is
// rendered as pixel glyphs instead — that lets it scale in lockstep with
// the pictogram using the exact same technique.
var daxFont = map[rune][]string{
	'D': {
		"### ",
		"#  #",
		"#  #",
		"#  #",
		"### ",
	},
	'a': {
		"    ",
		" ## ",
		"#  #",
		"#  #",
		" ###",
	},
	't': {
		" #  ",
		"### ",
		" #  ",
		" #  ",
		"  ##",
	},
	'R': {
		"### ",
		"#  #",
		"### ",
		"# # ",
		"#  #",
	},
	'o': {
		"    ",
		" ## ",
		"#  #",
		"#  #",
		" ## ",
	},
	'b': {
		"#   ",
		"#   ",
		"### ",
		"#  #",
		"### ",
	},
}

const daxFontHeight = 5

// daxNameGlyphLines renders daxName as daxFontHeight rows of pixel-font
// glyphs, so it can be merged into the pictogram block and scaled the same
// way — one shared nearest-neighbor scale for the whole reveal target.
func daxNameGlyphLines() []string {
	rows := make([]string, daxFontHeight)

	for i, ch := range daxName {
		glyph := daxFont[ch]

		for r := range daxFontHeight {
			rows[r] += glyph[r]

			if i < len(daxName)-1 {
				rows[r] += strings.Repeat(" ", daxGlyphGap)
			}
		}
	}

	return rows
}

// daxScale picks how many terminal cells make up one logo "pixel", so the
// reveal target grows along with Dax's much larger bounce area instead of
// staying a fixed tiny block regardless of terminal size — capped so the
// scaled block never exceeds daxWidthFitFraction of the terminal width.
func daxScale(width, height int) int {
	contentWidth := daxRevealWidth(1)

	maxByWidth := int(float64(width)*daxWidthFitFraction) / max(contentWidth, 1)
	maxByHeight := height / daxScaleBaseHeight

	s := min(maxByWidth, maxByHeight)

	return clampInt(s, 1, daxScaleMax)
}

// daxPass describes one leg of Dax's bounce: a single brand color — brand
// guidelines require Dax render in exactly one color at a time, never a
// blend — and whether this leg ends by flying off-screen instead of
// bouncing again.
type daxPass struct {
	color lipgloss.Color
	exits bool
}

// daxPassSequence is the fixed "enter, bounce twice, exit" run: Dax enters
// from off-screen left revealing the logo, bounces around like a DVD-logo
// screensaver off whichever edge he hits, then on the third leg flies off
// in whatever diagonal direction he's going and disappears. Colors come
// from the existing DataRobot design-system palette in colors.go.
func daxPassSequence() []daxPass {
	return []daxPass{
		{color: GetAdaptiveColor(DrYellow, DrYellowDark)},
		{color: GetAdaptiveColor(DrGreen, DrGreenDark)},
		{color: GetAdaptiveColor(DrIndigo, DrIndigoDark), exits: true},
	}
}

const (
	daxFPS        = 60
	daxFrameDelay = time.Second / daxFPS

	daxSpeedX = 1.6 // columns per frame
	daxSpeedY = 0.8 // rows per frame — half of X so the diagonal reads naturally against tall character cells

	daxWobbleAmplitude = 1.6
	daxWobbleDecay     = 0.35
	daxWobbleFreq      = 1.4
	daxWobbleFrames    = 18

	daxExitMargin = 4
	daxHoldFrames = 60 // ~1s hold on the fully-revealed logo before dismissing
)

type daxTickMsg struct{}

type daxPhase int

const (
	daxPhaseRunning daxPhase = iota
	daxPhaseHolding
)

// DaxModel animates Dax bouncing diagonally across the screen like a
// DVD-logo screensaver, sweeping open a reveal of the real DataRobot
// pictogram and name (sliced from the Banner constant in banner.go) as he
// crosses it, before flying off and disappearing.
type DaxModel struct {
	width  int
	height int

	x, y        float64
	vx, vy      float64
	passIdx     int
	passes      []daxPass
	revealed    int
	bounceFrame int
	bounceVX    float64
	bounceVY    float64

	frame      int
	holdFrames int
	phase      daxPhase
}

// newDaxModel creates a new Dax bounce/reveal overlay sized to the current
// terminal. Matches the overlayFactory signature so it drops in for the
// Konami trigger in konami_overlay.go.
func newDaxModel(width, height int) tea.Model {
	startY := float64(height-daxSpriteHeight) / 2

	return DaxModel{
		width:  width,
		height: height,
		x:      -float64(daxSpriteWidth),
		y:      startY,
		vx:     daxSpeedX,
		vy:     daxSpeedY,
		passes: daxPassSequence(),
	}
}

// daxLogoLines builds the reveal target: the pictogram sliced straight out
// of the shared Banner constant (not the huge multi-hundred-column
// wordmark art, which would overflow most terminals) with the "DataRobot"
// pixel-font wordmark merged in, vertically centered against it, then the
// whole combined block nearest-neighbor upscaled by scale as one unit — so
// the name grows in lockstep with the pictogram instead of staying tiny.
func daxLogoLines(scale int) []string {
	var pictogram []string

	for line := range strings.SplitSeq(Banner, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}

		runes := []rune(line)
		n := min(daxPictogramWidth, len(runes))
		padded := string(runes[:n]) + strings.Repeat(" ", daxPictogramWidth-n)
		pictogram = append(pictogram, padded)
	}

	nameRows := daxNameGlyphLines()
	nameStart := (len(pictogram) - len(nameRows)) / 2

	combined := make([]string, len(pictogram))

	for i, row := range pictogram {
		if nameIdx := i - nameStart; nameIdx >= 0 && nameIdx < len(nameRows) {
			row += strings.Repeat(" ", daxNameGap) + nameRows[nameIdx]
		}

		combined[i] = row
	}

	return scaleLines(combined, scale)
}

// scaleLines nearest-neighbor upscales a block of equal-width lines,
// repeating each character scale times horizontally and each resulting
// line scale times vertically.
func scaleLines(lines []string, scale int) []string {
	if scale <= 1 {
		return append([]string(nil), lines...)
	}

	out := make([]string, 0, len(lines)*scale)

	for _, line := range lines {
		var sb strings.Builder

		for _, ch := range line {
			sb.WriteString(strings.Repeat(string(ch), scale))
		}

		scaledLine := sb.String()
		for range scale {
			out = append(out, scaledLine)
		}
	}

	return out
}

func (m DaxModel) Init() tea.Cmd {
	return daxTick()
}

func daxTick() tea.Cmd {
	return tea.Tick(daxFrameDelay, func(time.Time) tea.Msg { return daxTickMsg{} })
}

func (m DaxModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		return m, nil

	case daxTickMsg:
		return m.handleTick()
	}

	return m, nil
}

func (m DaxModel) handleTick() (tea.Model, tea.Cmd) {
	m.frame++

	switch m.phase {
	case daxPhaseRunning:
		m.updateRunning()

		return m, daxTick()

	case daxPhaseHolding:
		m.holdFrames++

		if m.holdFrames >= daxHoldFrames {
			return m, func() tea.Msg { return OverlayDoneMsg{} }
		}

		return m, daxTick()
	}

	return m, nil
}

func (m *DaxModel) updateRunning() {
	m.x += m.vx
	m.y += m.vy

	pass := m.passes[m.passIdx]
	if pass.exits {
		m.updateExiting()

		return
	}

	m.bounceOffEdges()

	revealTo := clampInt(int(math.Round(m.x))+daxSpriteWidth, 0, daxRevealWidth(daxScale(m.width, m.height)))
	m.revealed = max(m.revealed, revealTo)
}

// bounceOffEdges reflects vx/vy off whichever edge (or edges, in a corner
// hit) Dax has reached, DVD-logo style, and advances to the next pass.
func (m *DaxModel) bounceOffEdges() {
	rightBound := float64(m.width - daxSpriteWidth)
	bottomBound := float64(m.height - daxSpriteHeight)

	bounced := false

	switch {
	case m.vx > 0 && m.x >= rightBound:
		m.x = rightBound
		m.vx = -m.vx
		bounced = true
	case m.vx < 0 && m.x <= 0:
		m.x = 0
		m.vx = -m.vx
		bounced = true
	}

	switch {
	case m.vy > 0 && m.y >= bottomBound:
		m.y = bottomBound
		m.vy = -m.vy
		bounced = true
	case m.vy < 0 && m.y <= 0:
		m.y = 0
		m.vy = -m.vy
		bounced = true
	}

	if bounced {
		m.bounce()
	}
}

// updateExiting lets Dax fly straight off in whatever direction he's
// already going (no more reflecting) until he's fully outside the overlay,
// then settles into the hold phase.
func (m *DaxModel) updateExiting() {
	revealTo := clampInt(int(math.Round(m.x))+daxSpriteWidth, 0, daxRevealWidth(daxScale(m.width, m.height)))
	m.revealed = max(m.revealed, revealTo)

	margin := float64(daxExitMargin)

	offX := m.x < -float64(daxSpriteWidth)-margin || m.x > float64(m.width)+margin
	offY := m.y < -float64(daxSpriteHeight)-margin || m.y > float64(m.height)+margin

	if offX || offY {
		m.phase = daxPhaseHolding
		m.holdFrames = 0
	}
}

func (m *DaxModel) bounce() {
	m.bounceFrame = m.frame
	m.bounceVX = m.vx
	m.bounceVY = m.vy

	if m.passIdx < len(m.passes)-1 {
		m.passIdx++
	}
}

// wobble returns a decaying oscillation applied to Dax's position for a few
// frames right after each bounce, so hitting a wall reads as a little
// "boing" instead of an instant direction flip.
func (m DaxModel) wobble() (float64, float64) {
	elapsed := m.frame - m.bounceFrame
	if elapsed < 0 || elapsed >= daxWobbleFrames {
		return 0, 0
	}

	t := float64(elapsed)
	decay := math.Exp(-daxWobbleDecay*t) * math.Cos(daxWobbleFreq*t)

	return m.bounceVX * daxWobbleAmplitude * decay / daxSpeedX, m.bounceVY * daxWobbleAmplitude * decay / daxSpeedY
}

// daxRevealWidth is the width of the widest line in the scaled reveal
// target, measured directly from daxLogoLines rather than a formula so it
// can never drift out of sync with what's actually rendered.
func daxRevealWidth(scale int) int {
	width := 0

	for _, line := range daxLogoLines(scale) {
		width = max(width, len([]rune(line)))
	}

	return width
}

func (m DaxModel) View() string {
	lines := make([]string, max(m.height, 1))

	logoLines := daxLogoLines(daxScale(m.width, m.height))

	contentHeight := len(logoLines) + daxRowGapBelow + daxSpriteHeight
	top := max((m.height-contentHeight)/2, 0)

	logoStyle := lipgloss.NewStyle().Bold(true).Foreground(GetAdaptiveColor(DrPurple, DrPurpleDark))

	for i, line := range logoLines {
		row := top + i
		if row < 0 || row >= len(lines) {
			continue
		}

		runes := []rune(line)
		n := min(m.revealed, len(runes))
		lines[row] = daxLeftMargin + logoStyle.Render(string(runes[:n]))
	}

	m.renderDax(lines)

	return strings.Join(lines, "\n")
}

func (m DaxModel) renderDax(lines []string) {
	if m.phase != daxPhaseRunning {
		return
	}

	wobbleX, wobbleY := m.wobble()

	offsetX := int(math.Round(m.x + wobbleX))
	offsetY := int(math.Round(m.y + wobbleY))

	if offsetX <= -daxSpriteWidth || offsetX >= m.width+daxExitMargin {
		return
	}

	color := m.passes[m.passIdx].color
	full := lipgloss.NewStyle().Foreground(color)
	shade := lipgloss.NewStyle().Foreground(dimColor(color, 0.5))

	for i, row := range daxGrid {
		r := offsetY + i
		if r < 0 || r >= len(lines) {
			continue
		}

		lines[r] = daxLeftMargin + shiftLine(renderDaxRow(row, full, shade), offsetX)
	}
}

func renderDaxRow(row string, full, shade lipgloss.Style) string {
	var sb strings.Builder

	for _, ch := range row {
		switch ch {
		case '#':
			sb.WriteString(full.Render("█"))
		case '.':
			sb.WriteString(shade.Render("█"))
		default:
			sb.WriteString(" ")
		}
	}

	return sb.String()
}

// shiftLine prepends spaces (positive offset) or trims leading runes
// (negative offset) so line appears to move horizontally.
func shiftLine(line string, offset int) string {
	if offset > 0 {
		return strings.Repeat(" ", offset) + line
	}

	if offset < 0 {
		runes := []rune(line)
		trim := min(-offset, len(runes))

		return string(runes[trim:])
	}

	return line
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}

	if v > hi {
		return hi
	}

	return v
}

// dimColor scales an RGB color's channels toward black — used for Dax's
// half-opacity shading cells, which are a shade of the current pass's
// single brand color, not a second hue.
func dimColor(c lipgloss.Color, amount float64) lipgloss.Color {
	hex := strings.TrimPrefix(string(c), "#")

	r, _ := strconv.ParseUint(hex[0:2], 16, 8)
	g, _ := strconv.ParseUint(hex[2:4], 16, 8)
	b, _ := strconv.ParseUint(hex[4:6], 16, 8)

	const hexDigits = "0123456789abcdef"

	buf := []byte{'#', 0, 0, 0, 0, 0, 0}
	for i, v := range []uint8{uint8(float64(r) * amount), uint8(float64(g) * amount), uint8(float64(b) * amount)} {
		buf[1+i*2] = hexDigits[v>>4]
		buf[2+i*2] = hexDigits[v&0x0f]
	}

	return lipgloss.Color(buf)
}
