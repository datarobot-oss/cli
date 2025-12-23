// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

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
