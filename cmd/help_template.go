// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package cmd

import (
	internalVersion "github.com/datarobot/cli/internal/version"
	"github.com/datarobot/cli/tui"
)

// Templates taken from (and combined and slightly altered): https://github.com/spf13/cobra/blob/main/command.go

var CustomHelpTemplate = `🚀 ` + tui.BaseTextStyle.Render(internalVersion.AppName) + ` - Build AI Applications Faster (version ` + tui.InfoStyle.Render(internalVersion.Version) + `)
{{with (or .Long .Short)}}{{. | trimTrailingWhitespaces}}

{{end}}{{if or .Runnable .HasSubCommands}}` + tui.BaseTextStyle.Render("Usage:") + `{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

` + tui.BaseTextStyle.Render("Aliases:") + `
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

` + tui.BaseTextStyle.Render("Examples:") + `
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}{{$cmds := .Commands}}{{if eq (len .Groups) 0}}

` + tui.BaseTextStyle.Render("Available Commands:") + `{{range $cmds}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{else}}{{range $group := .Groups}}

{{.Title}}{{range $cmds}}{{if (and (eq .GroupID $group.ID) (or .IsAvailableCommand (eq .Name "help")))}}
  ` + tui.SetAnsiForegroundColor(tui.GetAdaptiveColor(tui.DrPurple, tui.DrPurpleDark)) + `{{rpad .Name .NamePadding }}` + tui.ResetForegroundColor() + ` {{.Short}}{{end}}{{end}}{{end}}{{if not .AllChildCommandsHaveGroup}}

` + tui.BaseTextStyle.Render("Additional Commands:") + `{{range $cmds}}{{if (and (eq .GroupID "") (or .IsAvailableCommand (eq .Name "help")))}}
  ` + tui.SetAnsiForegroundColor(tui.GetAdaptiveColor(tui.DrPurple, tui.DrPurpleDark)) + `{{rpad .Name .NamePadding }}` + tui.ResetForegroundColor() + ` {{.Short}}{{end}}{{end}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

` + tui.BaseTextStyle.Render("Flags:") + `
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

` + tui.BaseTextStyle.Render("Global Flags:") + `
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

` + tui.BaseTextStyle.Render("Additional help topics:") + `{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}{{end}}
`
