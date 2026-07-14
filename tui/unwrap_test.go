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

	"github.com/stretchr/testify/assert"
)

// TestUnwrap_PeelsThroughInterruptibleModelAndSequenceOverlay guards the
// regression where commands like `dr start` type-assert a tui.Run() result
// down to their own concrete model (e.g. to read a field set during
// Update()). tui.Run() wraps every model as
// InterruptibleModel{sequenceOverlay{concreteModel}}, so a caller that only
// unwraps InterruptibleModel gets a *sequenceOverlay back, not their model
// — the assertion silently fails and the caller's "not ok" fallback path
// runs instead (e.g. `dr start` in an empty directory printing "Launching
// template setup..." and then just exiting without ever showing the
// wizard). tui.Unwrap must peel through both layers.
func TestUnwrap_PeelsThroughInterruptibleModelAndSequenceOverlay(t *testing.T) {
	wrapped := NewInterruptibleModel(wrapWithKonamiOverlay(noopModel{}))

	got := Unwrap(wrapped)

	assert.IsType(t, noopModel{}, got, "Unwrap should return the original concrete model, not a wrapper")
}

func TestUnwrap_PlainModelReturnsItself(t *testing.T) {
	got := Unwrap(noopModel{})

	assert.IsType(t, noopModel{}, got, "Unwrap on an unwrapped model should just return it")
}
