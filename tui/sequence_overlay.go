package tui

import tea "github.com/charmbracelet/bubbletea"

// OverlayDoneMsg is the contract an overlay model must send when it finishes.
// sequenceOverlay listens for this to dismiss the overlay and resume the inner model.
type OverlayDoneMsg struct{}

// sequenceDetector detects a key sequence one keystroke at a time.
// Feed returns true exactly once when the complete sequence is entered.
type sequenceDetector interface {
	Feed(tea.KeyMsg) bool
}

// overlayFactory creates a new overlay model sized to the current terminal.
type overlayFactory func(width, height int) tea.Model

// overlayTrigger pairs a sequence detector with the overlay it should show.
type overlayTrigger struct {
	detector sequenceDetector
	create   overlayFactory
}

// sequenceOverlay wraps a tea.Model and watches for registered key sequences.
// When a sequence fires, the corresponding overlay is rendered on top of the
// inner model until it sends OverlayDoneMsg or any key is pressed.
//
// Background messages (non-key) are always forwarded to the inner model so
// async operations continue uninterrupted while an overlay is active.
//
// To add a new overlay, call newSequenceOverlay with additional overlayTrigger
// values — each with its own sequenceDetector and overlayFactory.
type sequenceOverlay struct {
	inner      tea.Model
	triggers   []overlayTrigger
	active     tea.Model
	termWidth  int
	termHeight int
}

func newSequenceOverlay(inner tea.Model, triggers ...overlayTrigger) *sequenceOverlay {
	return &sequenceOverlay{inner: inner, triggers: triggers}
}

func (m *sequenceOverlay) Init() tea.Cmd {
	return m.inner.Init()
}

func (m *sequenceOverlay) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if sizeMsg, ok := msg.(tea.WindowSizeMsg); ok {
		m.termWidth = sizeMsg.Width
		m.termHeight = sizeMsg.Height
	}

	if m.active != nil {
		return m.updateActive(msg)
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if cmd := m.checkTriggers(keyMsg); cmd != nil {
			return m, cmd
		}
	}

	updated, cmd := m.inner.Update(msg)
	m.inner = updated

	return m, cmd
}

// updateActive routes messages while an overlay is showing.
// Key presses are consumed here — they dismiss the overlay without reaching
// the inner model. All other messages flow through to both so background
// work (network calls, ticks) keeps running.
func (m *sequenceOverlay) updateActive(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(tea.KeyMsg); ok {
		m.active = nil

		return m, nil
	}

	innerUpdated, innerCmd := m.inner.Update(msg)
	m.inner = innerUpdated

	if _, ok := msg.(OverlayDoneMsg); ok {
		m.active = nil

		return m, innerCmd
	}

	activeUpdated, activeCmd := m.active.Update(msg)
	m.active = activeUpdated

	return m, tea.Batch(activeCmd, innerCmd)
}

// checkTriggers feeds the key to every registered detector and activates the
// first one that completes its sequence.
func (m *sequenceOverlay) checkTriggers(keyMsg tea.KeyMsg) tea.Cmd {
	for i := range m.triggers {
		if m.triggers[i].detector.Feed(keyMsg) {
			w := max(m.termWidth, 80)
			h := max(m.termHeight, 24)

			overlay := m.triggers[i].create(w, h)
			m.active = overlay

			return overlay.Init()
		}
	}

	return nil
}

func (m *sequenceOverlay) View() string {
	if m.active != nil {
		return m.active.View()
	}

	return m.inner.View()
}
