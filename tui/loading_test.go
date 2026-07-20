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

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/stretchr/testify/assert"
)

func TestNewLoading_UsesStandardGlyphAndStyle(t *testing.T) {
	l := NewLoading()

	assert.NotEmpty(t, l.View())
}

func TestLoading_InitReturnsTickCmd(t *testing.T) {
	l := NewLoading()

	cmd := l.Init()

	assert.NotNil(t, cmd)
}

func TestLoading_UpdateAdvancesOnOwnTick(t *testing.T) {
	l := NewLoading()

	updated, cmd := l.Update(spinner.TickMsg{ID: l.Spinner.ID()})

	assert.NotNil(t, cmd)
	assert.Equal(t, l.Spinner.ID(), updated.Spinner.ID())
}

func TestLoading_UpdateNoOpsOnMismatchedID(t *testing.T) {
	l := NewLoading()

	// A tick carrying an ID that isn't this spinner's own must not schedule
	// a follow-up tick - this is the exact scenario that starves a nested
	// child's spinner if a parent forwards its own stale/foreign ticks.
	foreignTick := spinner.TickMsg{ID: l.Spinner.ID() + 1}

	updated, cmd := l.Update(foreignTick)

	assert.Nil(t, cmd)
	assert.Equal(t, l, updated)
}

func TestLoading_UpdateNoOpsOnUnrelatedMsg(t *testing.T) {
	l := NewLoading()

	type unknownMsg struct{}

	updated, cmd := l.Update(unknownMsg{})

	assert.Nil(t, cmd)
	assert.Equal(t, l, updated)
}
