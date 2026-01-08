package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type ClearStatusMsg struct {
	MsgID int
}

func ClearStatusAfter(duration time.Duration, msgID int) tea.Cmd {
	return func() tea.Msg {
		<-time.After(duration)
		return ClearStatusMsg{msgID}
	}
}
