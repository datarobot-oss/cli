package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestNewLogoAnimationModel(t *testing.T) {
	m := NewLogoAnimationModel()

	assert.Len(t, m.bars, len(pictogramLines), "one spring per pictogram line")
	assert.Equal(t, 0, m.phase)
	assert.False(t, m.Done)
	assert.False(t, m.bars[0].fromRight)
	assert.True(t, m.bars[1].fromRight)
}

func TestLogoAnimationSkipOnKeyPress(t *testing.T) {
	m := NewLogoAnimationModel()

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})

	result := updated.(LogoAnimationModel)

	assert.True(t, result.Done)
	assert.Equal(t, 3, result.phase)
	assert.NotNil(t, cmd)

	for _, bar := range result.bars {
		assert.InDelta(t, 0.0, bar.pos, 0.001)
		assert.InDelta(t, 0.0, bar.vel, 0.001)
	}

	assert.InDelta(t, 1.0, result.textOpacity, 0.001)
	assert.InDelta(t, 0.0, result.welcomePos, 0.001)
}

func TestLogoAnimationBarsConverge(t *testing.T) {
	m := NewLogoAnimationModel()

	for i := 0; i < 500; i++ {
		updated, _ := m.Update(logoTickMsg{})

		m = updated.(LogoAnimationModel)

		if m.phase > 0 {
			break
		}
	}

	assert.GreaterOrEqual(t, m.phase, 1)

	for _, bar := range m.bars {
		assert.InDelta(t, 0.0, bar.pos, 0.001)
	}
}

func TestLogoAnimationTextFadePhase(t *testing.T) {
	m := NewLogoAnimationModel()
	m.phase = 1
	m.frame = 200

	for i := range m.bars {
		m.bars[i].pos = 0
		m.bars[i].vel = 0
	}

	for i := 0; i < 30; i++ {
		updated, _ := m.Update(logoTickMsg{})

		m = updated.(LogoAnimationModel)

		if m.phase >= 2 {
			break
		}
	}

	assert.Equal(t, 2, m.phase)
	assert.InDelta(t, 1.0, m.textOpacity, 0.001)
}

func TestLogoAnimationDonePhase(t *testing.T) {
	m := NewLogoAnimationModel()
	m.phase = 3

	updated, cmd := m.Update(logoTickMsg{})

	result := updated.(LogoAnimationModel)

	assert.True(t, result.Done)
	assert.NotNil(t, cmd)
}

func TestLogoAnimationViewShowsPictogram(t *testing.T) {
	m := NewLogoAnimationModel()

	for i := range m.bars {
		m.bars[i].pos = 0
		m.bars[i].vel = 0
	}

	view := m.View()

	assert.Contains(t, view, "██████")
	assert.Contains(t, view, "Press any key to skip")
}

func TestLogoAnimationViewShowsTextAndWelcome(t *testing.T) {
	m := NewLogoAnimationModel()

	for i := range m.bars {
		m.bars[i].pos = 0
		m.bars[i].vel = 0
	}

	m.phase = 2
	m.textOpacity = 1.0
	m.welcomePos = 0

	view := m.View()

	assert.Contains(t, view, "DataRobot")
	assert.Contains(t, view, "Welcome to DataRobot CLI")
	assert.Contains(t, view, "Build AI Applications Faster")
}

func TestLogoAnimationViewNoSkipWhenDone(t *testing.T) {
	m := NewLogoAnimationModel()
	m.phase = 3

	view := m.View()

	assert.NotContains(t, view, "Press any key to skip")
}

func TestLerpColor(t *testing.T) {
	result := lerpColor("#ff0000", "#00ff00", 0.0)
	assert.Equal(t, "#ff0000", string(result))

	result = lerpColor("#ff0000", "#00ff00", 1.0)
	assert.Equal(t, "#00ff00", string(result))

	result = lerpColor("#ff0000", "#00ff00", 0.5)
	assert.Equal(t, "#808000", string(result))
}

func TestParseHex(t *testing.T) {
	r, g, b := parseHex("#7770F9")

	assert.Equal(t, uint8(0x77), r)
	assert.Equal(t, uint8(0x70), g)
	assert.Equal(t, uint8(0xF9), b)
}

func TestClamp01(t *testing.T) {
	assert.InDelta(t, 0.0, clamp01(-0.5), 0.001)
	assert.InDelta(t, 0.5, clamp01(0.5), 0.001)
	assert.InDelta(t, 1.0, clamp01(1.5), 0.001)
}
