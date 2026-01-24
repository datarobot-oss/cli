// Copyright 2025 DataRobot, Inc. and its affiliates.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package list

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/datarobot/cli/internal/task"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

// Category name constants
const (
	CategoryQuickStart      = "🚀 Quick Start"
	CategoryBuilding        = "🏗️ Building"
	CategoryTestingQuality  = "🧪 Testing & Quality"
	CategoryDeployment      = "🚀 Deployment"
	CategoryOther           = "📦 Other"
	CategoryNamespacePrefix = "❖ "
)

// Category represents a human-readable task category
type Category struct {
	Name     string
	Tasks    []task.Task
	Priority int // Lower numbers appear first
}

// getCategoryStyle returns the appropriate style for a category name with adaptive colors
func getCategoryStyle(categoryName string) lipgloss.Style {
	switch {
	case strings.Contains(categoryName, CategoryQuickStart):
		return lipgloss.NewStyle().
			Foreground(tui.GetAdaptiveColor(tui.DrGreen, tui.DrGreenDark)).
			Bold(true)
	case strings.Contains(categoryName, CategoryBuilding):
		return lipgloss.NewStyle().
			Foreground(tui.GetAdaptiveColor(tui.DrPurple, tui.DrPurpleDark)).
			Bold(true)
	case strings.Contains(categoryName, CategoryTestingQuality):
		return lipgloss.NewStyle().
			Foreground(tui.GetAdaptiveColor(tui.DrYellow, tui.DrYellowDark)).
			Bold(true)
	case strings.Contains(categoryName, CategoryDeployment):
		return lipgloss.NewStyle().
			Foreground(tui.GetAdaptiveColor(tui.DrIndigo, tui.DrIndigoDark)).
			Bold(true)
	default:
		return lipgloss.NewStyle().
			Foreground(tui.GetAdaptiveColor(tui.DrPurpleLight, tui.DrPurpleDarkLight)).
			Bold(true)
	}
}

// getTaskCategory determines category based on task suffix
func getTaskCategory(suffix string) *Category {
	lowerSuffix := strings.ToLower(suffix)

	if strings.Contains(lowerSuffix, "dev") || strings.Contains(lowerSuffix, "install") {
		return &Category{Name: CategoryQuickStart, Priority: 1}
	}

	if strings.Contains(lowerSuffix, "build") || strings.Contains(lowerSuffix, "docker") {
		return &Category{Name: CategoryBuilding, Priority: 3}
	}

	if strings.Contains(lowerSuffix, "test") || strings.Contains(lowerSuffix, "lint") || strings.Contains(lowerSuffix, "check") {
		return &Category{Name: CategoryTestingQuality, Priority: 4}
	}

	if strings.Contains(lowerSuffix, "deploy") || strings.Contains(lowerSuffix, "migrate") {
		return &Category{Name: CategoryDeployment, Priority: 5}
	}

	return nil
}

// isCommonTask checks if a task is commonly used
func isCommonTask(suffix string) bool {
	lowerSuffix := strings.ToLower(suffix)
	commonSuffixes := []string{"dev", "build", "test", "install", "deploy", "lint"}

	for _, common := range commonSuffixes {
		if strings.Contains(lowerSuffix, common) {
			return true
		}
	}

	return false
}

// categorizeTask determines the appropriate category for a task
func categorizeTask(t task.Task, showAll bool) *Category {
	name := t.Name

	// Root-level tasks
	if !strings.Contains(name, ":") {
		if cat := getTaskCategory(name); cat != nil {
			return cat
		}

		return &Category{Name: CategoryQuickStart, Priority: 1}
	}

	// Extract namespace and suffix
	parts := strings.SplitN(name, ":", 2)
	if len(parts) != 2 {
		return &Category{Name: CategoryOther, Priority: 99}
	}

	namespace := parts[0]
	suffix := parts[1]

	// Filter non-common tasks in default view
	if !showAll && !isCommonTask(suffix) {
		return nil
	}

	// Try to categorize by task type
	if cat := getTaskCategory(suffix); cat != nil {
		return cat
	}

	// Group by namespace for other tasks
	displayName := strings.ReplaceAll(namespace, "_", " ")
	if len(displayName) > 0 {
		displayName = strings.ToUpper(displayName[:1]) + displayName[1:]
	}

	categoryName := CategoryNamespacePrefix + displayName

	return &Category{Name: categoryName, Priority: 10}
}

// groupTasksByCategory groups tasks into human-readable categories
func groupTasksByCategory(tasks []task.Task, showAll bool) []*Category {
	categoryMap := make(map[string]*Category)

	var categories []*Category

	for _, t := range tasks {
		cat := categorizeTask(t, showAll)
		if cat == nil {
			continue // Skip tasks not shown in default view
		}

		if existing, found := categoryMap[cat.Name]; found {
			existing.Tasks = append(existing.Tasks, t)
		} else {
			cat.Tasks = []task.Task{t}
			categoryMap[cat.Name] = cat
			categories = append(categories, cat)
		}
	}

	// Sort categories by priority
	for i := 0; i < len(categories); i++ {
		for j := i + 1; j < len(categories); j++ {
			if categories[i].Priority > categories[j].Priority {
				categories[i], categories[j] = categories[j], categories[i]
			}
		}
	}

	return categories
}

// printCategorizedTasks prints tasks grouped by category in a styled table format
func printCategorizedTasks(categories []*Category, showAll bool) error {
	if len(categories) == 0 {
		fmt.Println("No tasks found.")

		return nil
	}

	// Adaptive colors for light/dark terminals
	titleColor := tui.GetAdaptiveColor(tui.DrGreen, tui.DrGreenDark)
	taskColor := tui.GetAdaptiveColor(tui.DrPurple, tui.DrPurpleDark)
	aliasColor := tui.GetAdaptiveColor(tui.DrPurpleLight, tui.DrPurpleDarkLight)
	descColor := tui.GetAdaptiveColor(tui.DrGray, tui.DrGrayDark)
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
		categoryStyle := getCategoryStyle(category.Name)

		fmt.Println()
		fmt.Println(categoryStyle.Render(category.Name))

		// Create table for this category
		t := table.New().
			Border(lipgloss.RoundedBorder()).
			BorderStyle(lipgloss.NewStyle().Foreground(tui.BorderColor)).
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

func Cmd() *cobra.Command {
	var dir string

	var showAll bool

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"l"},
		Short:   "List tasks",
		Run: func(_ *cobra.Command, _ []string) {
			binaryName := "task"
			discovery := task.NewTaskDiscovery("Taskfile.gen.yaml")

			rootTaskfile, err := discovery.Discover(dir, 2)
			if err != nil {
				task.ExitWithError(err)
				return
			}

			runner := task.NewTaskRunner(task.RunnerOpts{
				BinaryName: binaryName,
				Taskfile:   rootTaskfile,
				Dir:        dir,
			})

			if !runner.Installed() {
				_, _ = fmt.Fprintln(os.Stderr, `"`+binaryName+`" binary not found in PATH. Please install Task from https://taskfile.dev/installation/`)

				os.Exit(1)

				return
			}

			tasks, err := runner.ListTasks()
			if err != nil {
				_, _ = fmt.Fprintln(os.Stderr, "Error: ", err)

				os.Exit(1)

				return
			}

			categories := groupTasksByCategory(tasks, showAll)

			if err = printCategorizedTasks(categories, showAll); err != nil {
				_, _ = fmt.Fprintln(os.Stderr, "Error: ", err)

				os.Exit(1)

				return
			}
		},
	}

	cmd.Flags().StringVarP(&dir, "dir", "d", ".", "Directory to look for tasks.")
	cmd.Flags().BoolVarP(&showAll, "all", "a", false, "Show all tasks including less commonly used ones")

	// Register directory completion for the dir flag
	_ = cmd.RegisterFlagCompletionFunc("dir", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return nil, cobra.ShellCompDirectiveFilterDirs
	})

	return cmd
}
