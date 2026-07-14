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

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

type noopModel struct{}

func (noopModel) Init() tea.Cmd                       { return nil }
func (noopModel) Update(tea.Msg) (tea.Model, tea.Cmd) { return noopModel{}, nil }
func (noopModel) View() string                        { return "" }

// TestWrapWithKonamiOverlay_TriggersDaxModel drives the real production
// wiring (wrapWithKonamiOverlay -> sequenceOverlay -> konamiDetector) with
// the exact tea.KeyMsg shapes bubbletea produces for real arrow-key and
// letter-key presses, confirming the full Konami sequence activates
// DaxModel rather than just testing konamiDetector in isolation.
func TestWrapWithKonamiOverlay_TriggersDaxModel(t *testing.T) {
	model := wrapWithKonamiOverlay(noopModel{})

	sequence := []tea.KeyMsg{
		arrowKey(tea.KeyUp),
		arrowKey(tea.KeyUp),
		arrowKey(tea.KeyDown),
		arrowKey(tea.KeyDown),
		arrowKey(tea.KeyLeft),
		arrowKey(tea.KeyRight),
		arrowKey(tea.KeyLeft),
		arrowKey(tea.KeyRight),
		key("b"),
		key("a"),
	}

	for _, k := range sequence {
		model, _ = model.Update(k)
	}

	so, ok := model.(*sequenceOverlay)
	if !assert.True(t, ok, "wrapWithKonamiOverlay should return a *sequenceOverlay") {
		return
	}

	assert.IsType(t, DaxModel{}, so.active, "full Konami sequence should activate the Dax overlay")
}

// TestDaxOverlay_DismissesOnAnyKey locks in the "runs until any key" design:
// once Dax is bouncing he never stops on his own (no bounce/time limit), so
// the only exit is a keypress, which the overlay consumes to restore the
// inner model.
func TestDaxOverlay_DismissesOnAnyKey(t *testing.T) {
	t.Setenv(daxLoveEnvVar, "1")

	model := wrapWithKonamiOverlay(noopModel{})

	so, ok := model.(*sequenceOverlay)
	if !assert.True(t, ok, "wrapWithKonamiOverlay should return a *sequenceOverlay") {
		return
	}

	assert.IsType(t, DaxModel{}, so.active, "Dax should be active to start")

	// Many ticks pass with no key — Dax keeps bouncing, never dismisses.
	for range 1000 {
		model, _ = model.Update(daxTickMsg{})
	}

	so = model.(*sequenceOverlay)
	assert.NotNil(t, so.active, "Dax must keep running until a key is pressed")

	// Any key dismisses him.
	model, _ = model.Update(key("x"))

	so = model.(*sequenceOverlay)
	assert.Nil(t, so.active, "pressing any key must dismiss the Dax overlay")
}

// TestWrapWithKonamiOverlay_PartialSequenceDoesNotTrigger guards against a
// regression where any key (not just the exact Konami sequence) would open
// the overlay.
func TestWrapWithKonamiOverlay_PartialSequenceDoesNotTrigger(t *testing.T) {
	model := wrapWithKonamiOverlay(noopModel{})

	sequence := []tea.KeyMsg{
		arrowKey(tea.KeyUp),
		arrowKey(tea.KeyUp),
		arrowKey(tea.KeyDown),
	}

	for _, k := range sequence {
		model, _ = model.Update(k)
	}

	so, ok := model.(*sequenceOverlay)
	if !assert.True(t, ok, "wrapWithKonamiOverlay should return a *sequenceOverlay") {
		return
	}

	assert.Nil(t, so.active, "a partial sequence must not activate the overlay")
}

// listMenuModel wraps a real bubbles/list.Model (the same component
// cmd/templates/setup uses for its template picker) so its Update fully
// consumes arrow keys for its own navigation, just like a real menu screen.
type listMenuModel struct {
	list list.Model
}

func newListMenuModel() listMenuModel {
	items := []list.Item{
		listItem("first template"),
		listItem("second template"),
		listItem("third template"),
	}

	return listMenuModel{list: list.New(items, list.NewDefaultDelegate(), 40, 20)}
}

type listItem string

func (i listItem) FilterValue() string { return string(i) }

func (m listMenuModel) Init() tea.Cmd { return nil }

func (m listMenuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	m.list, cmd = m.list.Update(msg)

	return m, cmd
}

func (m listMenuModel) View() string { return m.list.View() }

// TestWrapWithKonamiOverlay_TriggersDespiteArrowMenu guards against the
// (structurally impossible, but worth proving) concern that a screen with
// its own arrow-key-driven menu — like the template picker in
// `dr templates setup` — could "consume" the arrow keys before the Konami
// detector sees them. sequenceOverlay sits outside the inner model and
// checks every key first, so the inner model reacting to the same keys
// (list navigation, filtering, whatever) has no bearing on detection.
func TestWrapWithKonamiOverlay_TriggersDespiteArrowMenu(t *testing.T) {
	model := wrapWithKonamiOverlay(newListMenuModel())

	sequence := []tea.KeyMsg{
		arrowKey(tea.KeyUp),
		arrowKey(tea.KeyUp),
		arrowKey(tea.KeyDown),
		arrowKey(tea.KeyDown),
		arrowKey(tea.KeyLeft),
		arrowKey(tea.KeyRight),
		arrowKey(tea.KeyLeft),
		arrowKey(tea.KeyRight),
		key("b"),
		key("a"),
	}

	for _, k := range sequence {
		model, _ = model.Update(k)
	}

	so, ok := model.(*sequenceOverlay)
	if !assert.True(t, ok, "wrapWithKonamiOverlay should return a *sequenceOverlay") {
		return
	}

	assert.IsType(t, DaxModel{}, so.active,
		"Konami sequence should still activate Dax even though the inner model has its own arrow-key menu")
}

// TestDaxLoveEnvVar_ActivatesImmediately covers the I_LOVE_DAX escape hatch:
// setting it should show Dax without needing the Konami sequence at all.
func TestDaxLoveEnvVar_ActivatesImmediately(t *testing.T) {
	t.Setenv(daxLoveEnvVar, "1")

	model := wrapWithKonamiOverlay(noopModel{})

	so, ok := model.(*sequenceOverlay)
	if !assert.True(t, ok, "wrapWithKonamiOverlay should return a *sequenceOverlay") {
		return
	}

	assert.IsType(t, DaxModel{}, so.active, "I_LOVE_DAX should activate the Dax overlay immediately")
}
