package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	rocketTickInterval = 60 * time.Millisecond
	rocketMessageDelay = 1500 * time.Millisecond
)

type (
	rocketTickMsg struct{}
	rocketDoneMsg struct{}
)

// rocketPhase represents the current phase of the animation.
type rocketPhase int

const (
	rocketPhaseFlying rocketPhase = iota
	rocketPhaseMessage
)

// RocketModel is a Bubble Tea model that animates a rocket flying up the screen,
// then shows "Godspeed, Artemis II!" before signalling completion.
type RocketModel struct {
	width  int
	height int
	row    int
	phase  rocketPhase
	done   bool
}

// RocketDoneMsg is sent when the rocket animation finishes, so the parent can resume.
type RocketDoneMsg struct{}

func newRocketModel(width, height int) RocketModel {
	return RocketModel{
		width:  width,
		height: height,
		row:    height - 1,
		phase:  rocketPhaseFlying,
	}
}

func (m RocketModel) Init() tea.Cmd {
	return tickRocket()
}

func (m RocketModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case rocketTickMsg:
		if m.phase == rocketPhaseFlying {
			m.row--

			if m.row < -2 {
				m.phase = rocketPhaseMessage

				return m, showMessageThenDone()
			}

			return m, tickRocket()
		}

	case rocketDoneMsg:
		m.done = true

		return m, func() tea.Msg { return RocketDoneMsg{} }
	}

	return m, nil
}

func (m RocketModel) View() string {
	if m.phase == rocketPhaseMessage {
		return m.messageView()
	}

	return m.flyingView()
}

func (m RocketModel) flyingView() string {
	lines := make([]string, m.height)

	rocketCol := m.width/2 - 1

	for i := range lines {
		lines[i] = ""
	}

	placeAt := func(row int, content string) {
		if row >= 0 && row < m.height {
			pad := strings.Repeat(" ", rocketCol)

			lines[row] = pad + content
		}
	}

	placeAt(m.row, "🚀")
	placeAt(m.row+1, "✨")
	placeAt(m.row+2, " ·")

	return strings.Join(lines, "\n")
}

func (m RocketModel) messageView() string {
	msg1 := "The crew of Artemis II now bound for the moon."
	msg2 := "Humanity's next great voyage begins."

	style := lipgloss.NewStyle().
		Foreground(GetAdaptiveColor(DrGreen, DrGreenDark)).
		Bold(true)

	rendered := style.Render(msg1)
	rendered += "\n\n" + style.Render(msg2)

	pad := strings.Repeat(" ", max(0, (m.width-len(msg1))/2))
	verticalPad := strings.Repeat("\n", max(0, m.height/2-1))

	return verticalPad + pad + rendered
}

func tickRocket() tea.Cmd {
	return tea.Tick(rocketTickInterval, func(_ time.Time) tea.Msg {
		return rocketTickMsg{}
	})
}

func showMessageThenDone() tea.Cmd {
	return func() tea.Msg {
		<-time.After(rocketMessageDelay)

		return rocketDoneMsg{}
	}
}
