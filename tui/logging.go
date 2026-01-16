package tui

import "github.com/charmbracelet/log"

const DebugLogLevelWidth = 5

// DebugLogStyles customizes the log styles for debug logging in the TUI
var DebugLogStyles = func() *log.Styles {
	styles := log.DefaultStyles()
	for level, style := range styles.Levels {
		styles.Levels[level] = style.MaxWidth(DebugLogLevelWidth).PaddingRight(1)
	}

	return styles
}()
