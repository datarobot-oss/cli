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

const (
	animFPS        = 60
	animFrameDelay = time.Second / animFPS
	springFreq     = 5.5
	springDamping  = 0.35
	staggerFrames  = 4
	slideDistance  = 30.0
	settledThresh  = 0.3
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
type LogoAnimationModel struct {
	bars   []pictoBar
	spring harmonica.Spring

	width  int
	height int

	frame        int
	phase        int     // 0=bars-slide+text-fade, 2=welcome, 3=slide-out, 4=done
	textOpacity  float64 // 0..1 for "DataRobot" text
	welcomePos   float64
	welcomeVel   float64
	slideOutVPos float64 // 0.5=centre→0.0=top, animated during phase 3
	slideOutStep float64 // per-frame step, increases each frame (acceleration)
	Done         bool
}

// NewLogoAnimationModel creates a new compact logo animation model.
func NewLogoAnimationModel() LogoAnimationModel {
	bars := make([]pictoBar, len(pictogramLines))

	for i := range bars {
		bars[i] = pictoBar{started: false, pos: slideDistance}
	}

	return LogoAnimationModel{
		bars:         bars,
		spring:       harmonica.NewSpring(harmonica.FPS(animFPS), springFreq, springDamping),
		width:        80,
		height:       24,
		welcomePos:   3.0,
		slideOutVPos: 0.5,
		slideOutStep: 0.008,
	}
}

func (m LogoAnimationModel) Init() tea.Cmd {
	return tea.Tick(animFrameDelay, func(time.Time) tea.Msg {
		return logoTickMsg{}
	})
}

func (m LogoAnimationModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		return m, nil

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
	m.phase = 4

	for i := range m.bars {
		m.bars[i].started = true
		m.bars[i].pos = 0
		m.bars[i].vel = 0
	}

	m.textOpacity = 1.0
	m.welcomePos = 0
}

func (m LogoAnimationModel) handleTick() (tea.Model, tea.Cmd) {
	m.frame++

	nextTick := tea.Tick(animFrameDelay, func(time.Time) tea.Msg {
		return logoTickMsg{}
	})

	switch m.phase {
	case 0:
		m.updateBarsSlide()

		return m, nextTick

	case 2:
		return m.updateWelcome(nextTick)

	case 3:
		return m.updateSlideOut(nextTick)

	case 4:
		m.Done = true

		return m, tea.Quit
	}

	return m, nil
}

func (m *LogoAnimationModel) updateBarsSlide() {
	allSettled := true

	for i := range m.bars {
		startFrame := i * staggerFrames

		if m.frame < startFrame {
			allSettled = false

			continue
		}

		m.bars[i].started = true
		m.bars[i].pos, m.bars[i].vel = m.spring.Update(m.bars[i].pos, m.bars[i].vel, 0)

		if math.Abs(m.bars[i].pos) > settledThresh || math.Abs(m.bars[i].vel) > settledThresh {
			allSettled = false
		}
	}

	// Fade text in simultaneously with bars sliding in.
	m.textOpacity = clamp01(m.textOpacity + 0.04)

	if allSettled && m.textOpacity >= 1.0 {
		for i := range m.bars {
			m.bars[i].pos = 0
			m.bars[i].vel = 0
		}

		m.phase = 2
	}
}

func (m LogoAnimationModel) updateSlideOut(nextTick tea.Cmd) (tea.Model, tea.Cmd) {
	m.slideOutStep += 0.002
	m.slideOutVPos -= m.slideOutStep

	if m.slideOutVPos <= 0 {
		m.slideOutVPos = 0
		m.phase = 4

		return m, tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg {
			return logoTickMsg{}
		})
	}

	return m, nextTick
}

func (m LogoAnimationModel) updateWelcome(nextTick tea.Cmd) (tea.Model, tea.Cmd) {
	m.welcomePos, m.welcomeVel = m.spring.Update(m.welcomePos, m.welcomeVel, 0)

	if math.Abs(m.welcomePos) < 0.05 {
		m.phase = 3

		return m, tea.Tick(1200*time.Millisecond, func(time.Time) tea.Msg {
			return logoTickMsg{}
		})
	}

	return m, nextTick
}

func (m LogoAnimationModel) View() string {
	var sb strings.Builder

	sb.WriteString("\n")
	m.renderLogo(&sb)
	m.renderWelcome(&sb)

	if m.phase < 3 {
		sb.WriteString("\n")
		sb.WriteString(DimStyle.Render("  Press any key to skip"))
	}

	sb.WriteString("\n")

	content := sb.String()

	if m.width > 0 && m.height > 0 {
		return lipgloss.Place(
			m.width,
			m.height,
			lipgloss.Center,
			lipgloss.Position(m.slideOutVPos),
			content,
		)
	}

	return content
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

	for i, bar := range m.bars {
		progress := 0.0
		if total > 1 {
			progress = float64(i) / float64(total-1)
		}

		color := lerpColor(DrPurple, DrGreen, progress)
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

	// Fade effect for "DataRobot"
	fadedColor := lerpColor(lipgloss.Color("#1a1a2e"), DrPurple, m.textOpacity)
	nameStyle := lipgloss.NewStyle().Bold(true).Foreground(fadedColor)

	// Layout: place text lines in the vertical center of the pictogram
	// Line arrangement (for 9 lines, mid=4):
	//   mid-1 = "DataRobot"
	//   mid   = (empty spacer)
	//   mid+1 = welcome (phase 2+)
	//   mid+2 = subtitle (phase 2+)
	mid := height / 2

	lines[mid-1] = nameStyle.Render("DataRobot")

	if m.phase >= 2 {
		welcomeStyle := lipgloss.NewStyle().Bold(true).Foreground(DrPurple)

		welcomeOffset := max(int(math.Round(m.welcomePos)), 0)

		if welcomeOffset == 0 {
			lines[mid+1] = welcomeStyle.Render("✨ Welcome to DataRobot CLI")
			lines[mid+2] = DimStyle.Render("Build AI Applications Faster")
		}
	}

	return strings.Join(lines, "\n")
}

// renderWelcome is a no-op — welcome text is now inside buildTextBlock.
func (m LogoAnimationModel) renderWelcome(_ *strings.Builder) {}

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
