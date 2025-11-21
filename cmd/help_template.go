// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package cmd

import (
	"github.com/charmbracelet/lipgloss"
	internalVersion "github.com/datarobot/cli/internal/version"
	"github.com/datarobot/cli/tui"
)

func getHelpHeader() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(tui.GetAdaptiveColor(tui.DrPurpleLight, tui.DrPurpleDark))

	sloganStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(tui.GetAdaptiveColor(tui.DrGreen, tui.DrGreenDark)).
		Italic(true)

	separatorStyle := lipgloss.NewStyle().
		Foreground(tui.GetAdaptiveColor(tui.DrPurple, tui.DrPurpleDark))

	title := titleStyle.Render(internalVersion.GetAppNameVersionText())
	separator := separatorStyle.Render("────────────────────────────────────────────")
	slogan := sloganStyle.Render("    🚀 Build AI Applications Faster")

	return lipgloss.JoinVertical(lipgloss.Left, title, separator, slogan, "")
}

func getSectionHeader(title string) string {
	style := lipgloss.NewStyle().
		Bold(true).
		Foreground(tui.GetAdaptiveColor(tui.DrYellow, tui.DrYellowDark))

	return style.Render(title)
}

func getCommandColor() string {
	return "\033[1m" + tui.SetAnsiForegroundColor(tui.GetAdaptiveColor(tui.DrPurple, tui.DrPurpleDark))
}

func resetCommandStyle() string {
	return tui.ResetForegroundColor() + "\033[0m"
}

// Templates taken from (and combined and slightly altered): https://github.com/spf13/cobra/blob/main/command.go

var CustomHelpTemplate = getHelpHeader() + `
{{with .Long}}{{. | trimTrailingWhitespaces}}

{{end}}{{if or .Runnable .HasSubCommands}}` + getSectionHeader("Usage:") + `{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

` + getSectionHeader("Aliases:") + `
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

` + getSectionHeader("Examples:") + `
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}{{$cmds := .Commands}}{{if eq (len .Groups) 0}}

` + getSectionHeader("Available Commands:") + `{{range $cmds}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  ` + getCommandColor() + `{{rpad .Name .NamePadding }}` + resetCommandStyle() + ` {{.Short}}{{end}}{{end}}{{else}}{{range $group := .Groups}}{{$hasCommands := false}}{{range $cmds}}{{if (and (eq .GroupID $group.ID) (or .IsAvailableCommand (eq .Name "help")))}}{{$hasCommands = true}}{{end}}{{end}}{{if $hasCommands}}

` + getSectionHeader("{{.Title}}") + `{{range $cmds}}{{if (and (eq .GroupID $group.ID) (or .IsAvailableCommand (eq .Name "help")))}}
  ` + getCommandColor() + `{{rpad .Name .NamePadding }}` + resetCommandStyle() + ` {{.Short}}{{end}}{{end}}{{end}}{{end}}{{if not .AllChildCommandsHaveGroup}}

` + getSectionHeader("Additional Commands:") + `{{range $cmds}}{{if (and (eq .GroupID "") (or .IsAvailableCommand (eq .Name "help")))}}
  ` + getCommandColor() + `{{rpad .Name .NamePadding }}` + resetCommandStyle() + ` {{.Short}}{{end}}{{end}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

` + getSectionHeader("Flags:") + `
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

` + getSectionHeader("Global Flags:") + `
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

` + getSectionHeader("Additional help topics:") + `{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more informatison about a command.{{end}}{{end}}
`
