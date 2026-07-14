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
	"math/rand/v2"
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
)

// daxColors is the set of solid brand colors Dax cycles through, one per
// bounce. Brand guidelines require Dax render in exactly one color at a
// time (never a blend), so each is a single flat color drawn from the
// existing DataRobot design-system palette in colors.go.
func daxColors() []lipgloss.Color {
	return []lipgloss.Color{
		GetAdaptiveColor(DrYellow, DrYellowDark),
		GetAdaptiveColor(DrGreen, DrGreenDark),
		GetAdaptiveColor(DrPurple, DrPurpleDark),
		GetAdaptiveColor(DrIndigo, DrIndigoDark),
		GetAdaptiveColor(DrPurpleLight, DrPurpleDarkLight),
	}
}

const (
	daxFPS        = 60
	daxFrameDelay = time.Second / daxFPS

	// Base per-frame speed. Y is roughly half X because terminal cells are
	// about twice as tall as wide, so equal cell-counts per axis would make
	// vertical motion look twice as fast as horizontal.
	daxBaseSpeedX = 1.0
	daxBaseSpeedY = 0.5

	// Each axis's speed is randomized within ±(daxSpeedJitter) of its base
	// so every run starts on a different vector, DVD-screensaver style.
	daxSpeedJitter = 0.25

	// daxHintReserve keeps the bottom terminal row free for the "press any
	// key" hint so Dax never bounces over it.
	daxHintReserve = 1
)

// daxHint is the dismissal prompt shown along the bottom. The overlay runs
// until any key is pressed (handled by sequenceOverlay), so unlike the old
// build there is no bounce/time limit — this hint is the only exit cue.
const daxHint = "Press any key to exit"

type daxTickMsg struct{}

// DaxModel bounces Dax around the terminal like the classic DVD-player
// screensaver: a randomized starting position and velocity, reflecting off
// all four walls, switching to a random solid brand color on every bounce.
type DaxModel struct {
	width  int
	height int

	x, y     float64
	vx, vy   float64
	colorIdx int
	bounces  int
}

// newDaxModel creates a Dax bounce overlay sized to the current terminal,
// with a random start position, velocity, and color. Matches the
// overlayFactory signature so it drops in for the Konami trigger in
// konami_overlay.go.
func newDaxModel(width, height int) tea.Model {
	m := DaxModel{
		width:    width,
		height:   height,
		colorIdx: rand.IntN(len(daxColors())), //nolint:gosec // cosmetic, not security-sensitive
	}

	m.x, m.y = m.randomStart()
	m.vx, m.vy = randomVelocity()

	return m
}

// bounds returns the maximum x and y the sprite's top-left corner may reach
// while staying fully on-screen, with the bottom row reserved for the hint.
func (m DaxModel) bounds() (float64, float64) {
	right := max(m.width-daxSpriteWidth, 0)
	bottom := max(m.height-daxSpriteHeight-daxHintReserve, 0)

	return float64(right), float64(bottom)
}

// randomStart returns a random on-screen position for Dax, clamped so the
// whole sprite fits when the terminal is large enough (and pinned to the
// origin when it isn't).
func (m DaxModel) randomStart() (float64, float64) {
	right, bottom := m.bounds()

	return float64(rand.IntN(int(right) + 1)), float64(rand.IntN(int(bottom) + 1)) //nolint:gosec // cosmetic
}

// randomVelocity returns a random diagonal velocity: each axis keeps its
// base speed jittered within ±daxSpeedJitter and a random sign, so the path
// is always a lively diagonal and never degenerate (purely horizontal or
// vertical).
func randomVelocity() (float64, float64) {
	return jitteredSpeed(daxBaseSpeedX) * randomSign(), jitteredSpeed(daxBaseSpeedY) * randomSign()
}

func jitteredSpeed(base float64) float64 {
	return base + (rand.Float64()*2-1)*daxSpeedJitter //nolint:gosec // cosmetic
}

func randomSign() float64 {
	if rand.IntN(2) == 0 { //nolint:gosec // cosmetic
		return -1
	}

	return 1
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
	m.advance()

	return m, daxTick()
}

// advance moves Dax one frame and reflects off any wall he has reached,
// recoloring on each bounce.
func (m *DaxModel) advance() {
	m.x += m.vx
	m.y += m.vy

	rightBound, bottomBound := m.bounds()

	bounced := false

	if m.x <= 0 && m.vx < 0 || m.x >= rightBound && m.vx > 0 {
		m.x = clampFloat(m.x, 0, rightBound)
		m.vx = -m.vx
		bounced = true
	}

	if m.y <= 0 && m.vy < 0 || m.y >= bottomBound && m.vy > 0 {
		m.y = clampFloat(m.y, 0, bottomBound)
		m.vy = -m.vy
		bounced = true
	}

	if bounced {
		m.onBounce()
	}
}

// onBounce advances the bounce counter and switches Dax to a random
// different brand color, so consecutive bounces never repeat a color.
func (m *DaxModel) onBounce() {
	m.bounces++

	colors := daxColors()
	if len(colors) < 2 {
		return
	}

	next := rand.IntN(len(colors) - 1) //nolint:gosec // cosmetic
	if next >= m.colorIdx {
		next++
	}

	m.colorIdx = next
}

func (m DaxModel) View() string {
	lines := make([]string, max(m.height, 1))

	color := daxColors()[m.colorIdx]
	full := lipgloss.NewStyle().Foreground(color)
	shade := lipgloss.NewStyle().Foreground(dimColor(color, 0.5))

	offsetX := int(math.Round(m.x))
	offsetY := int(math.Round(m.y))

	for i, row := range daxGrid {
		r := offsetY + i
		if r < 0 || r >= len(lines) {
			continue
		}

		lines[r] = shiftLine(renderDaxRow(row, full, shade), offsetX)
	}

	m.renderHint(lines)

	return strings.Join(lines, "\n")
}

// renderHint centers the "press any key" prompt on the bottom row (kept
// clear of Dax by daxHintReserve).
func (m DaxModel) renderHint(lines []string) {
	if m.height < 1 {
		return
	}

	hint := HintStyle.Render(daxHint)
	pad := max((m.width-lipgloss.Width(hint))/2, 0)

	lines[m.height-1] = strings.Repeat(" ", pad) + hint
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

func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}

	if v > hi {
		return hi
	}

	return v
}

// dimColor scales an RGB color's channels toward black — used for Dax's
// half-opacity shading cells, which are a shade of the current bounce's
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
