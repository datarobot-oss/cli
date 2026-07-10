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

package start

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/datarobot/cli/tui"
	"github.com/stretchr/testify/assert"
)

// wrapperStub stands in for tui.Run()'s internal Konami sequenceOverlay
// wrapper (unexported, so it can't be constructed from this package) — any
// type implementing Unwrap() tea.Model exercises the same tui.Unwrap path.
type wrapperStub struct {
	inner tea.Model
}

func (w wrapperStub) Init() tea.Cmd                       { return nil }
func (w wrapperStub) Update(tea.Msg) (tea.Model, tea.Cmd) { return w, nil }
func (w wrapperStub) View() string                        { return "" }
func (w wrapperStub) Unwrap() tea.Model                   { return w.inner }

// TestGetInnerModel_UnwrapsThroughExtraWrapperLayers is a regression test
// for the bug where `dr start` in a repo-less directory would print
// "Launching template setup..." and then just exit without ever showing
// the wizard. tui.Run() wraps the model as
// InterruptibleModel{someWrapper{Model}}; getInnerModel used to only
// unwrap InterruptibleModel directly, so once tui.Run() started adding a
// second wrapper layer (the Konami sequenceOverlay), the type assertion to
// Model silently failed and getInnerModel returned (Model{}, false) —
// which cmd/start/cmd.go treats as "nothing to do", skipping the template
// setup launch entirely.
func TestGetInnerModel_UnwrapsThroughExtraWrapperLayers(t *testing.T) {
	original := Model{needTemplateSetup: true, done: true}

	wrapped := tui.NewInterruptibleModel(wrapperStub{inner: original})

	got, ok := getInnerModel(wrapped)

	assert.True(t, ok, "getInnerModel must unwrap through any number of wrapper layers")
	assert.True(t, got.needTemplateSetup)
	assert.True(t, got.done)
}

func TestGetInnerModel_FalseForUnrelatedModel(t *testing.T) {
	_, ok := getInnerModel(wrapperStub{inner: struct{ tea.Model }{}})

	assert.False(t, ok)
}
