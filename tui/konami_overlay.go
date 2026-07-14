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

// Easter egg: the Konami code (↑ ↑ ↓ ↓ ← → ← → B A) triggers Dax running
// across the screen to reveal the DataRobot logo. Setting the I_LOVE_DAX
// env var to any non-empty value shows it immediately, without needing the
// code. To remove: delete konami_overlay.go, konami.go, dax.go,
// konami_test.go, and the wrapWithKonamiOverlay call in program.go.
// sequence_overlay.go can stay — it is general-purpose infrastructure.

import (
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

// daxLoveEnvVar, when set to any non-empty value, shows the Dax overlay
// immediately on startup instead of waiting for the Konami code.
const daxLoveEnvVar = "I_LOVE_DAX"

// wrapWithKonamiOverlay wraps m in a sequenceOverlay that fires the Dax
// animation when the Konami code is entered. To swap the animation for
// something else, replace newDaxModel with a different overlayFactory.
// To add more sequences, pass additional overlayTrigger values.
func wrapWithKonamiOverlay(m tea.Model) tea.Model {
	overlay := newSequenceOverlay(m, overlayTrigger{
		detector: &konamiDetector{},
		create: func(w, h int) tea.Model {
			return newDaxModel(w, h)
		},
	})

	if os.Getenv(daxLoveEnvVar) != "" {
		overlay.activateNow(newDaxModel(defaultOverlayWidth, defaultOverlayHeight))
	}

	return overlay
}
