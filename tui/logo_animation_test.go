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
	"io"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLogoAnimationModel(t *testing.T) {
	m := NewLogoAnimationModel()

	assert.Len(t, m.bars, len(pictogramLines), "one spring per pictogram line")
	assert.Equal(t, phaseIntro, m.phase)
	assert.False(t, m.Done)
	assert.False(t, m.bars[0].started)
	assert.False(t, m.welcomeStarted)
	assert.InDelta(t, 0.0, m.textOpacity(), 0.001)
}

func TestLogoAnimationSkipOnKeyPress(t *testing.T) {
	m := NewLogoAnimationModel()

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})

	result := updated.(LogoAnimationModel)

	assert.True(t, result.Done)
	assert.Equal(t, phaseDone, result.phase)
	require.NotNil(t, cmd)

	msg := cmd()
	_, isQuit := msg.(tea.QuitMsg)
	assert.True(t, isQuit, "skip should issue a real tea.Quit command")

	for _, bar := range result.bars {
		assert.InDelta(t, 0.0, bar.pos, 0.001)
		assert.InDelta(t, 0.0, bar.vel, 0.001)
	}

	assert.InDelta(t, 1.0, result.textOpacity(), 0.001)

	view := result.View()
	assert.NotContains(t, view, "Press any key to skip")
	assert.Contains(t, view, "DataRobot")
	assert.Contains(t, view, "Welcome to DataRobot CLI")
	assert.Contains(t, view, "Build AI Applications Faster")
}

func TestLogoAnimationBarsCascadeAndSettle(t *testing.T) {
	m := NewLogoAnimationModel()

	terminated := false

	for i := 0; i < 300; i++ {
		updated, _ := m.Update(logoTickMsg{})
		m = updated.(LogoAnimationModel)

		if m.phase != phaseIntro {
			terminated = true

			break
		}
	}

	require.True(t, terminated, "bar cascade should settle within a bounded number of ticks")

	for _, bar := range m.bars {
		assert.InDelta(t, 0.0, bar.pos, barsSettledThresh)
	}
}

func TestLogoAnimationWelcomeOverlapsBars(t *testing.T) {
	m := NewLogoAnimationModel()

	var progressAtStart float64

	triggered := false

	for i := 0; i < 300; i++ {
		wasStarted := m.welcomeStarted

		updated, _ := m.Update(logoTickMsg{})
		m = updated.(LogoAnimationModel)

		if !wasStarted && m.welcomeStarted {
			progressAtStart = m.barsSettleProgress()
			triggered = true

			break
		}

		if m.phase != phaseIntro {
			break
		}
	}

	require.True(t, triggered, "welcome line should start during the intro phase")
	assert.GreaterOrEqual(t, progressAtStart, barsOverlapProgress)
	assert.Less(t, progressAtStart, 1.0, "welcome line should overlap the bar cascade, not wait for it to finish")
}

func TestLogoAnimationTextOpacitySpringDriven(t *testing.T) {
	m := NewLogoAnimationModel()

	assert.InDelta(t, 0.0, m.textOpacity(), 0.001)

	for i := 0; i < 10; i++ {
		updated, _ := m.Update(logoTickMsg{})
		m = updated.(LogoAnimationModel)
	}

	assert.Greater(t, m.textOpacity(), 0.0, "opacity should be moving early in the intro")

	for i := 0; i < 300 && m.phase == phaseIntro; i++ {
		updated, _ := m.Update(logoTickMsg{})
		m = updated.(LogoAnimationModel)
	}

	assert.GreaterOrEqual(t, m.textOpacity(), 1.0-opacitySettledThresh, "opacity should have settled near 1 by the time the intro ends")
}

func TestLogoAnimationSettleGraceThenDone(t *testing.T) {
	m := NewLogoAnimationModel()
	m.phase = phaseSettled

	ticks := 0
	done := false

	for i := 0; i < graceFrames+5; i++ {
		updated, _ := m.Update(logoTickMsg{})
		m = updated.(LogoAnimationModel)
		ticks++

		if m.Done {
			done = true

			break
		}
	}

	require.True(t, done)
	assert.Equal(t, graceFrames+1, ticks, "should take graceFrames ticks to settle plus one to process phaseDone")
}

func TestLogoAnimationFullRunTerminates(t *testing.T) {
	m := NewLogoAnimationModel()

	done := false

	for i := 0; i < 400; i++ {
		updated, _ := m.Update(logoTickMsg{})
		m = updated.(LogoAnimationModel)

		if m.Done {
			done = true

			break
		}
	}

	require.True(t, done, "animation should reach Done within a bounded number of ticks")
	assert.NotContains(t, m.View(), "Press any key to skip")
}

func TestLogoAnimationViewShowsPictogram(t *testing.T) {
	m := NewLogoAnimationModel()

	for i := range m.bars {
		m.bars[i].started = true
		m.bars[i].pos = 0
		m.bars[i].vel = 0
	}

	view := m.View()

	assert.Contains(t, view, "██████")
	assert.Contains(t, view, "Press any key to skip")
}

func TestLogoAnimationViewIsLeftAligned(t *testing.T) {
	m := NewLogoAnimationModel()

	for i := range m.bars {
		m.bars[i].started = true
		m.bars[i].pos = 0
		m.bars[i].vel = 0
	}

	for _, line := range strings.Split(m.View(), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}

		assert.True(t, strings.HasPrefix(line, leftMargin), "line should start with the shared left margin: %q", line)
	}
}

func TestLogoAnimationIntegrationSkipViaKeypress(t *testing.T) {
	tm := teatest.NewTestModel(t, NewLogoAnimationModel(), teatest.WithInitialTermSize(80, 24))

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})

	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))

	fm := tm.FinalModel(t)
	result, ok := fm.(LogoAnimationModel)
	require.True(t, ok, "final model is not LogoAnimationModel")
	assert.True(t, result.Done)

	out, err := io.ReadAll(tm.FinalOutput(t))
	require.NoError(t, err)
	assert.Contains(t, string(out), "DataRobot")
	assert.NotContains(t, string(out), "Press any key to skip")
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
