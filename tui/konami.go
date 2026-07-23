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

import tea "github.com/charmbracelet/bubbletea"

// konamiSequence is the Konami code: ↑ ↑ ↓ ↓ ← → ← → B A.
var konamiSequence = []string{
	"up", "up", "down", "down", "left", "right", "left", "right", "b", "a",
}

// konamiDetector tracks progress through the Konami code sequence.
type konamiDetector struct {
	pos int
}

// Feed advances the detector with a key. Returns true when the full sequence is complete.
// Resets to zero on any wrong key.
func (k *konamiDetector) Feed(msg tea.KeyMsg) bool {
	key := msg.String()

	if key == konamiSequence[k.pos] {
		k.pos++

		if k.pos == len(konamiSequence) {
			k.pos = 0

			return true
		}
	} else {
		k.pos = 0

		// Re-check from position 0 in case the wrong key starts a new sequence
		if key == konamiSequence[0] {
			k.pos = 1
		}
	}

	return false
}
