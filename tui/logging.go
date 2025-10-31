// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package tui

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

// DebugLogFile is the filename for TUI debug logs
const DebugLogFile = "dr-tui-debug.log"

// SetupDebugLogging configures debug logging for the TUI if debug mode is enabled.
// It should be called at the start of TUI programs when debug logging is needed.
// Returns a cleanup function that should be deferred to close the log file.
func SetupDebugLogging() (cleanup func(), err error) {
	f, err := tea.LogToFile(DebugLogFile, "debug")
	if err != nil {
		fmt.Println("fatal:", err)
		os.Exit(1)
	}

	return func() { f.Close() }, nil
}
