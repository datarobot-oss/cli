package tui

// Easter egg: entering the Konami code triggers a rocket animation.
// To remove: delete konami_overlay.go, konami.go, rocket.go, konami_test.go,
// and the wrapWithKonamiOverlay call in program.go.

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/datarobot/cli/internal/log"
)

type konamiOverlay struct {
	inner      tea.Model
	konami     konamiDetector
	rocket     *RocketModel
	termWidth  int
	termHeight int
}

func wrapWithKonamiOverlay(m tea.Model) tea.Model {
	return &konamiOverlay{inner: m}
}

func (m *konamiOverlay) Init() tea.Cmd {
	return m.inner.Init()
}

func (m *konamiOverlay) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if sizeMsg, ok := msg.(tea.WindowSizeMsg); ok {
		m.termWidth = sizeMsg.Width
		m.termHeight = sizeMsg.Height
	}

	if m.rocket != nil {
		return m.updateRocket(msg)
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if cmd := m.handleKonami(keyMsg); cmd != nil {
			return m, cmd
		}
	}

	updated, cmd := m.inner.Update(msg)
	m.inner = updated

	return m, cmd
}

func (m *konamiOverlay) updateRocket(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Key presses are consumed by the overlay and not forwarded to the inner
	// model. Any key dismisses the animation immediately.
	if _, ok := msg.(tea.KeyMsg); ok {
		m.rocket = nil

		return m, nil
	}

	// Forward non-key messages to the inner model so background operations
	// (e.g. network fetches, spinner ticks) continue during the animation.
	innerUpdated, innerCmd := m.inner.Update(msg)
	m.inner = innerUpdated

	if _, ok := msg.(RocketDoneMsg); ok {
		m.rocket = nil

		return m, innerCmd
	}

	rocketUpdated, rocketCmd := m.rocket.Update(msg)
	rocket := rocketUpdated.(RocketModel)
	m.rocket = &rocket

	return m, tea.Batch(rocketCmd, innerCmd)
}

func (m *konamiOverlay) handleKonami(keyMsg tea.KeyMsg) tea.Cmd {
	if !m.konami.Feed(keyMsg) {
		return nil
	}

	log.Info("Konami code activated!")

	w := max(m.termWidth, 80)
	h := max(m.termHeight, 24)

	rocket := newRocketModel(w, h)
	m.rocket = &rocket

	return rocket.Init()
}

func (m *konamiOverlay) View() string {
	if m.rocket != nil {
		return m.rocket.View()
	}

	return m.inner.View()
}
