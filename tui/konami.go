package tui

import tea "github.com/charmbracelet/bubbletea"

// konamiSequence is the Konami code: Up Down Left Right Left Right B
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
