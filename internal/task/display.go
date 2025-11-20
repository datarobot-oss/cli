// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package task

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/datarobot/cli/tui"
)

// PrintCategorizedTasks prints tasks grouped by category in a styled table format
func PrintCategorizedTasks(categories []*Category, showAll bool) error {
	if len(categories) == 0 {
		fmt.Println("No tasks found.")

		return nil
	}

	// Adaptive colors for light/dark terminals
	titleColor := tui.GetAdaptiveColor(tui.DrGreen, tui.DrGreenDark)
	taskColor := tui.GetAdaptiveColor(tui.DrPurple, tui.DrPurpleDark)
	aliasColor := tui.GetAdaptiveColor(tui.DrPurpleLight, tui.DrPurpleDarkLight)
	descColor := tui.GetAdaptiveColor(tui.DrGray, tui.DrGrayDark)
	borderColor := tui.GetAdaptiveColor(tui.DrPurpleLight, tui.DrPurpleDarkLight)
	tipBorderColor := tui.GetAdaptiveColor(tui.DrYellow, tui.DrYellowDark)

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(titleColor).
		MarginBottom(1)

	fmt.Println(titleStyle.Render("Available Tasks"))

	// Define table styles
	taskNameStyle := lipgloss.NewStyle().
		Foreground(taskColor).
		Padding(0, 1)

	aliasStyle := lipgloss.NewStyle().
		Foreground(aliasColor).
		Italic(true)

	descStyle := lipgloss.NewStyle().
		Foreground(descColor).
		Padding(0, 1)

	for _, category := range categories {
		// Print styled category header
		categoryStyle := GetCategoryStyle(category.Name)

		fmt.Println()
		fmt.Println(categoryStyle.Render(category.Name))

		// Create table for this category
		t := table.New().
			Border(lipgloss.RoundedBorder()).
			BorderStyle(lipgloss.NewStyle().Foreground(borderColor)).
			StyleFunc(func(_, col int) lipgloss.Style {
				// Note: Headers() are styled automatically by the table
				// We only need to style data rows based on column
				if col == 0 {
					return taskNameStyle
				}

				return descStyle
			}).
			Headers("TASK", "DESCRIPTION")

		// Add rows for each task
		for _, tsk := range category.Tasks {
			taskName := tsk.Name

			if len(tsk.Aliases) > 0 {
				taskName += " " + aliasStyle.Render("("+strings.Join(tsk.Aliases, ", ")+")")
			}

			desc := strings.ReplaceAll(tsk.Desc, "\n", " ")

			t.Row(taskName, desc)
		}

		fmt.Println(t.Render())
	}

	// Show tip if not showing all tasks
	if !showAll {
		tipStyle := lipgloss.NewStyle().
			Foreground(aliasColor).
			Italic(true).
			MarginTop(1).
			Padding(1, 2).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(tipBorderColor)

		fmt.Println()
		fmt.Println(tipStyle.Render("💡 Tip: Run 'dr task list --all' to see all available tasks."))
	}

	return nil
}
