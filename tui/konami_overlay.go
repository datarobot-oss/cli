package tui

// Easter egg: the Konami code (↑ ↑ ↓ ↓ ← → ← → B A) triggers the rocket animation.
// To remove: delete konami_overlay.go, konami.go, rocket.go, konami_test.go,
// and the wrapWithKonamiOverlay call in program.go.
// sequence_overlay.go can stay — it is general-purpose infrastructure.

import tea "github.com/charmbracelet/bubbletea"

// wrapWithKonamiOverlay wraps m in a sequenceOverlay that fires the rocket
// animation when the Konami code is entered. To swap the animation for
// something else, replace newRocketModel with a different overlayFactory.
// To add more sequences, pass additional overlayTrigger values.
func wrapWithKonamiOverlay(m tea.Model) tea.Model {
	return newSequenceOverlay(m, overlayTrigger{
		detector: &konamiDetector{},
		create: func(w, h int) tea.Model {
			return newRocketModel(w, h)
		},
	})
}
