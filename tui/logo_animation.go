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
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/harmonica"
	"github.com/charmbracelet/lipgloss"
)

type logoTickMsg struct{}

// animPhase models the animation as a single continuous "intro" driven by one
// shared spring, followed by a brief settle grace period, then done.
type animPhase int

const (
	phaseIntro   animPhase = iota // bars cascading, text fading, and welcome line overlapping in — all concurrent
	phaseSettled                  // everything at rest; graceFrames countdown running before quit
	phaseDone                     // Done=true, quitting
)

const (
	animFPS        = 60
	animFrameDelay = time.Second / animFPS

	springFreq    = 5.5
	springDamping = 0.35

	staggerFrames = 4
	slideDistance = 30.0

	welcomeStartOffset = 3.0

	// Settle thresholds are scaled per quantity: bars/welcome move in column
	// units (~1-30), opacity lives in 0..1 — a single shared threshold would
	// either never trigger for opacity or trigger too early for position.
	barsSettledThresh    = 0.3
	welcomeSettledThresh = 0.05
	opacitySettledThresh = 0.02

	// barsOverlapProgress releases the welcome line once the bar cascade is
	// mostly (not fully) settled, so the two motions overlap instead of the
	// welcome line waiting for a hard "100% done" gate.
	barsOverlapProgress = 0.55

	// graceFrames is a short hold (~200ms at 60fps) after everything settles,
	// so the final frame doesn't feel clipped the instant physics crosses
	// its threshold.
	graceFrames = 12
)

// fadeStartDark/fadeStartLight approximate the muted gray used elsewhere for
// dim text (DrGray/DrGrayDark, ANSI 256 indices "252"/"240") but in hex form,
// since lerpColor interpolates hex RGB rather than ANSI palette indices.
const (
	fadeStartDark  = lipgloss.Color("#5a5a5a")
	fadeStartLight = lipgloss.Color("#c8c8c8")
)

// Pictogram — the 9 bars matching the DataRobot brand icon.
// Pattern: symmetric, alternating left/indented bars.
var pictogramLines = []string{
	"█████████",
	"         █████",
	"█████████",
	"              █████",
	"██████████████",
	"              █████",
	"█████████",
	"         █████",
	"█████████",
}

// pictoBar tracks per-line spring animation state.
type pictoBar struct {
	started bool
	pos     float64
	vel     float64
}

// LogoAnimationModel animates a compact DataRobot logo using spring physics.
// A single shared spring drives every animated quantity (bar offsets, text
// opacity, welcome-line offset) so all motion shares one physical feel.
type LogoAnimationModel struct {
	bars   []pictoBar
	spring harmonica.Spring

	frame         int
	phase         animPhase
	settledFrames int

	textOpacityPos float64
	textOpacityVel float64

	welcomeStarted bool
	welcomePos     float64
	welcomeVel     float64

	Done bool
}

// NewLogoAnimationModel creates a new compact logo animation model.
func NewLogoAnimationModel() LogoAnimationModel {
	bars := make([]pictoBar, len(pictogramLines))

	for i := range bars {
		bars[i] = pictoBar{started: false, pos: slideDistance}
	}

	return LogoAnimationModel{
		bars:       bars,
		spring:     harmonica.NewSpring(harmonica.FPS(animFPS), springFreq, springDamping),
		welcomePos: welcomeStartOffset,
	}
}

func (m LogoAnimationModel) Init() tea.Cmd {
	return tea.Tick(animFrameDelay, func(time.Time) tea.Msg {
		return logoTickMsg{}
	})
}

func (m LogoAnimationModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		_ = msg

		m.skipToEnd()

		return m, tea.Quit

	case logoTickMsg:
		return m.handleTick()
	}

	return m, nil
}

func (m *LogoAnimationModel) skipToEnd() {
	m.Done = true
	m.phase = phaseDone

	for i := range m.bars {
		m.bars[i].started = true
		m.bars[i].pos = 0
		m.bars[i].vel = 0
	}

	m.textOpacityPos = 1.0
	m.textOpacityVel = 0

	m.welcomeStarted = true
	m.welcomePos = 0
	m.welcomeVel = 0
}

func (m LogoAnimationModel) handleTick() (tea.Model, tea.Cmd) {
	m.frame++

	nextTick := tea.Tick(animFrameDelay, func(time.Time) tea.Msg {
		return logoTickMsg{}
	})

	switch m.phase {
	case phaseIntro:
		m.updateIntro()

		return m, nextTick

	case phaseSettled:
		m.settledFrames++

		if m.settledFrames >= graceFrames {
			m.phase = phaseDone
		}

		return m, nextTick

	case phaseDone:
		m.Done = true

		return m, tea.Quit
	}

	return m, nil
}

// updateIntro advances the bar cascade, text opacity, and welcome line for
// one tick. All three share m.spring so they decelerate/settle with the same
// physical feel; the welcome line is released early (see barsOverlapProgress)
// so it overlaps the tail of the bar cascade instead of waiting for it.
func (m *LogoAnimationModel) updateIntro() {
	barsSettled := m.updateBars()
	opacitySettled := m.updateOpacity()
	welcomeSettled := m.updateWelcome()

	if barsSettled && opacitySettled && welcomeSettled {
		m.phase = phaseSettled
		m.settledFrames = 0
	}
}

func (m *LogoAnimationModel) updateBars() bool {
	settled := true

	for i := range m.bars {
		startFrame := i * staggerFrames

		if m.frame < startFrame {
			settled = false

			continue
		}

		m.bars[i].started = true
		m.bars[i].pos, m.bars[i].vel = m.spring.Update(m.bars[i].pos, m.bars[i].vel, 0)

		if math.Abs(m.bars[i].pos) > barsSettledThresh || math.Abs(m.bars[i].vel) > barsSettledThresh {
			settled = false
		}
	}

	return settled
}

func (m *LogoAnimationModel) updateOpacity() bool {
	m.textOpacityPos, m.textOpacityVel = m.spring.Update(m.textOpacityPos, m.textOpacityVel, 1.0)

	return math.Abs(m.textOpacityPos-1.0) < opacitySettledThresh &&
		math.Abs(m.textOpacityVel) < opacitySettledThresh
}

// updateWelcome releases the welcome line once the bar cascade crosses
// barsOverlapProgress, then springs it toward 0 like any other track.
func (m *LogoAnimationModel) updateWelcome() bool {
	if !m.welcomeStarted && m.barsSettleProgress() >= barsOverlapProgress {
		m.welcomeStarted = true
	}

	if !m.welcomeStarted {
		return false
	}

	m.welcomePos, m.welcomeVel = m.spring.Update(m.welcomePos, m.welcomeVel, 0)

	return math.Abs(m.welcomePos) < welcomeSettledThresh && math.Abs(m.welcomeVel) < welcomeSettledThresh
}

// barsSettleProgress reports how far along (0..1) the bar cascade is,
// averaged across all bars. A not-yet-started bar contributes 0. This gates
// when the welcome line overlaps in (see barsOverlapProgress) instead of
// requiring every bar to be 100% settled first.
func (m LogoAnimationModel) barsSettleProgress() float64 {
	if len(m.bars) == 0 {
		return 1
	}

	var sum float64

	for _, b := range m.bars {
		if !b.started {
			continue
		}

		sum += clamp01(1 - math.Abs(b.pos)/slideDistance)
	}

	return sum / float64(len(m.bars))
}

// textOpacity returns the "DataRobot" text's current fade-in progress, 0..1.
func (m LogoAnimationModel) textOpacity() float64 {
	return clamp01(m.textOpacityPos)
}

// leftMargin is the indent applied to the whole animation block, matching
// the 2-space indent other inline screens (e.g. dr start's step list) use
// for their content instead of centering it in the terminal.
const leftMargin = "  "

func (m LogoAnimationModel) View() string {
	var sb strings.Builder

	sb.WriteString("\n")
	m.renderLogo(&sb)

	// The hint line's space is always reserved (blank when not shown) so the
	// rendered block's height never changes mid-animation — inline bubbletea
	// redraws in place based on the previous frame's line count, and a
	// height change there is what causes a visible jump.
	sb.WriteString("\n")

	if m.phase == phaseIntro {
		sb.WriteString(DimStyle.Render("Press any key to skip"))
	}

	sb.WriteString("\n")

	return indentBlock(sb.String(), leftMargin)
}

// indentBlock prefixes every line of content with margin.
func indentBlock(content, margin string) string {
	lines := strings.Split(content, "\n")

	for i, line := range lines {
		if line == "" {
			continue
		}

		lines[i] = margin + line
	}

	return strings.Join(lines, "\n")
}

func (m LogoAnimationModel) renderLogo(sb *strings.Builder) {
	pictoBlock := m.buildPictogramBlock()
	textBlock := m.buildTextBlock()
	joined := lipgloss.JoinHorizontal(lipgloss.Center, pictoBlock, "   ", textBlock)

	sb.WriteString(joined)
	sb.WriteString("\n")
}

func (m LogoAnimationModel) buildPictogramBlock() string {
	total := len(pictogramLines)
	lines := make([]string, 0, total)

	from := GetAdaptiveColor(DrPurple, DrPurpleDark)
	to := GetAdaptiveColor(DrGreen, DrGreenDark)

	for i, bar := range m.bars {
		progress := 0.0
		if total > 1 {
			progress = float64(i) / float64(total-1)
		}

		color := lerpColor(from, to, progress)
		style := lipgloss.NewStyle().Foreground(color)

		if !bar.started {
			lines = append(lines, "")

			continue
		}

		offset := int(math.Round(bar.pos))

		line := pictogramLines[i]

		if offset > 0 {
			line = strings.Repeat(" ", offset) + line
		} else if offset < 0 {
			runes := []rune(line)
			trim := min(-offset, len(runes))

			line = string(runes[trim:])
		}

		lines = append(lines, style.Render(line))
	}

	return strings.Join(lines, "\n")
}

func (m LogoAnimationModel) buildTextBlock() string {
	height := len(pictogramLines)
	lines := make([]string, height)

	// Fade "DataRobot" from the existing dim-gray token to the same purple
	// BaseTextStyle/WelcomeStyle resolve to, so full opacity matches the
	// welcome line's color exactly.
	fadeFrom := GetAdaptiveColor(fadeStartDark, fadeStartLight)
	fadeTo := GetAdaptiveColor(DrPurple, DrPurpleDark)
	nameStyle := lipgloss.NewStyle().Bold(true).Foreground(lerpColor(fadeFrom, fadeTo, m.textOpacity()))

	// Layout: place text lines in the vertical center of the pictogram
	// Line arrangement (for 9 lines, mid=4):
	//   mid-1 = "DataRobot"
	//   mid   = (empty spacer)
	//   mid+1 = welcome (once welcomeStarted)
	//   mid+2 = subtitle (once welcomeStarted)
	mid := height / 2

	lines[mid-1] = nameStyle.Render("DataRobot")

	if m.welcomeStarted {
		offset := max(int(math.Round(m.welcomePos)), 0)

		welcomeLine := "✨ Welcome to DataRobot CLI"
		subtitleLine := "Build AI Applications Faster"

		if offset > 0 {
			welcomeLine = strings.Repeat(" ", offset) + welcomeLine
			subtitleLine = strings.Repeat(" ", offset) + subtitleLine
		}

		lines[mid+1] = WelcomeStyle.Render(welcomeLine)
		lines[mid+2] = DimStyle.Render(subtitleLine)
	}

	return strings.Join(lines, "\n")
}

// clamp01 clamps a value between 0 and 1.
func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}

	if v > 1 {
		return 1
	}

	return v
}

// lerpColor linearly interpolates between two hex lipgloss colors.
func lerpColor(from, to lipgloss.Color, t float64) lipgloss.Color {
	fr, fg, fb := parseHex(string(from))
	tr, tg, tb := parseHex(string(to))

	r := uint8(math.Round(float64(fr) + float64(int(tr)-int(fr))*t))
	g := uint8(math.Round(float64(fg) + float64(int(tg)-int(fg))*t))
	b := uint8(math.Round(float64(fb) + float64(int(tb)-int(fb))*t))

	return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", r, g, b))
}

// parseHex extracts RGB values from a hex color string like "#AABBCC".
func parseHex(hex string) (uint8, uint8, uint8) {
	hex = strings.TrimPrefix(hex, "#")

	r, _ := strconv.ParseUint(hex[0:2], 16, 8)
	g, _ := strconv.ParseUint(hex[2:4], 16, 8)
	b, _ := strconv.ParseUint(hex[4:6], 16, 8)

	return uint8(r), uint8(g), uint8(b)
}
