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
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestDaxModel_RunsIndefinitely(t *testing.T) {
	// Dax must never dismiss himself — the overlay runs until a keypress
	// (handled by sequenceOverlay), so every tick keeps ticking.
	m := newDaxModel(80, 24).(DaxModel)

	for range 5000 {
		_, cmd := m.Update(daxTickMsg{})

		updated, _ := m.Update(daxTickMsg{})
		m = updated.(DaxModel)

		assert.NotNil(t, cmd, "Dax should keep scheduling ticks and never stop on its own")
	}
}

func TestDaxModel_StaysWithinBounds(t *testing.T) {
	const w, h = 80, 24

	m := newDaxModel(w, h).(DaxModel)

	rightBound := float64(w - daxSpriteWidth)
	// The bottom row is reserved for the hint, so Dax stops one row higher.
	bottomBound := float64(h - daxSpriteHeight - daxHintReserve)

	for range 5000 {
		updated, _ := m.Update(daxTickMsg{})
		m = updated.(DaxModel)

		assert.GreaterOrEqual(t, m.x, 0.0, "Dax must not leave the left wall")
		assert.LessOrEqual(t, m.x, rightBound, "Dax must not leave the right wall")
		assert.GreaterOrEqual(t, m.y, 0.0, "Dax must not leave the top wall")
		assert.LessOrEqual(t, m.y, bottomBound, "Dax must not overlap the bottom hint row")
	}
}

func TestDaxModel_BouncesAndRecolors(t *testing.T) {
	// A cramped terminal forces frequent wall hits quickly.
	m := newDaxModel(34, 16).(DaxModel)

	startColor := m.colorIdx
	sawDifferentColor := false

	for range 2000 {
		updated, _ := m.Update(daxTickMsg{})
		m = updated.(DaxModel)

		if m.colorIdx != startColor {
			sawDifferentColor = true
		}
	}

	assert.Positive(t, m.bounces, "Dax should bounce off the walls")
	assert.True(t, sawDifferentColor, "Dax should switch to a different brand color when he bounces")
}

func TestDaxModel_ViewShowsHint(t *testing.T) {
	m := newDaxModel(80, 24).(DaxModel)

	assert.Contains(t, m.View(), daxHint, "the view should always show the press-any-key hint")
}

func TestRandomVelocity_IsLivelyDiagonal(t *testing.T) {
	// Every generated vector must have a meaningful component on both axes
	// so the path is a real diagonal, never purely horizontal or vertical.
	for range 200 {
		vx, vy := randomVelocity()

		assert.GreaterOrEqual(t, math.Abs(vx), daxBaseSpeedX-daxSpeedJitter)
		assert.LessOrEqual(t, math.Abs(vx), daxBaseSpeedX+daxSpeedJitter)
		assert.GreaterOrEqual(t, math.Abs(vy), daxBaseSpeedY-daxSpeedJitter)
		assert.LessOrEqual(t, math.Abs(vy), daxBaseSpeedY+daxSpeedJitter)
	}
}

func TestOnBounce_NeverRepeatsColor(t *testing.T) {
	m := newDaxModel(80, 24).(DaxModel)

	for range 100 {
		prev := m.colorIdx
		m.onBounce()

		assert.NotEqual(t, prev, m.colorIdx, "a bounce must always change to a different color")
	}
}

func TestDaxModel_WindowSizeMsgResizes(t *testing.T) {
	m := newDaxModel(80, 24).(DaxModel)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(DaxModel)

	assert.Equal(t, 120, m.width)
	assert.Equal(t, 40, m.height)
}

func TestDaxModel_ViewDoesNotPanic(t *testing.T) {
	for _, sz := range [][2]int{{80, 24}, {220, 80}, {20, 8}} {
		m := newDaxModel(sz[0], sz[1]).(DaxModel)

		for range 300 {
			assert.NotPanics(t, func() { m.View() })

			updated, _ := m.Update(daxTickMsg{})
			m = updated.(DaxModel)
		}
	}
}
