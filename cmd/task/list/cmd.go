// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package list

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/charmbracelet/lipgloss"
	"github.com/datarobot/cli/internal/task"
	"github.com/datarobot/cli/tui"
	"github.com/spf13/cobra"
)

// Category represents a human-readable task category
type Category struct {
	Name     string
	Tasks    []task.Task
	Priority int // Lower numbers appear first
}

// categoryStyles defines the styling for different category types
var (
	quickStartStyle = lipgloss.NewStyle().
			Foreground(tui.DrGreen).
			Bold(true)

	buildingStyle = lipgloss.NewStyle().
			Foreground(tui.DrPurple).
			Bold(true)

	testingStyle = lipgloss.NewStyle().
			Foreground(tui.DrYellow).
			Bold(true)

	deploymentStyle = lipgloss.NewStyle().
			Foreground(tui.DrIndigo).
			Bold(true)

	otherStyle = lipgloss.NewStyle().
			Foreground(tui.DrPurpleLight).
			Bold(true)
)

// getCategoryStyle returns the appropriate style for a category name
func getCategoryStyle(categoryName string) lipgloss.Style {
	switch {
	case strings.Contains(categoryName, "Quick Start"):
		return quickStartStyle
	case strings.Contains(categoryName, "Building"):
		return buildingStyle
	case strings.Contains(categoryName, "Testing"):
		return testingStyle
	case strings.Contains(categoryName, "Deployment"):
		return deploymentStyle
	default:
		return otherStyle
	}
}

// getTaskCategory determines category based on task suffix
func getTaskCategory(suffix string) *Category {
	lowerSuffix := strings.ToLower(suffix)

	if strings.Contains(lowerSuffix, "dev") || strings.Contains(lowerSuffix, "install") {
		return &Category{Name: "🚀 Quick Start", Priority: 1}
	}

	if strings.Contains(lowerSuffix, "build") || strings.Contains(lowerSuffix, "docker") {
		return &Category{Name: "🏗️  Building", Priority: 3}
	}

	if strings.Contains(lowerSuffix, "test") || strings.Contains(lowerSuffix, "lint") || strings.Contains(lowerSuffix, "check") {
		return &Category{Name: "🧪 Testing & Quality", Priority: 4}
	}

	if strings.Contains(lowerSuffix, "deploy") || strings.Contains(lowerSuffix, "migrate") {
		return &Category{Name: "� Deployment", Priority: 5}
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

		return &Category{Name: "🚀 Quick Start", Priority: 1}
	}

	// Extract namespace and suffix
	parts := strings.SplitN(name, ":", 2)
	if len(parts) != 2 {
		return &Category{Name: "📦 Other", Priority: 99}
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

	categoryName := "📦 " + displayName

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

// printGroupedTasks prints tasks grouped by category
func printCategorizedTasks(categories []*Category, showAll bool) error {
	if len(categories) == 0 {
		fmt.Println("No tasks found.")

		return nil
	}

	fmt.Println("Available tasks:")

	w := tabwriter.NewWriter(os.Stdout, 0, 8, 6, ' ', 0)

	for _, category := range categories {
		// Print styled category header
		style := getCategoryStyle(category.Name)
		styledCategory := style.Render(category.Name)

		_, _ = fmt.Fprintf(w, "\n%s\n", styledCategory)

		for _, t := range category.Tasks {
			desc := strings.ReplaceAll(t.Desc, "\n", " ")

			_, _ = fmt.Fprint(w, "  ")
			_, _ = fmt.Fprint(w, t.Name)

			if len(t.Aliases) > 0 {
				_, _ = fmt.Fprintf(w, " (%s)", strings.Join(t.Aliases, ", "))
			}

			_, _ = fmt.Fprintf(w, " \t%s", desc)

			_, _ = fmt.Fprint(w, "\n")
		}
	}

	if err := w.Flush(); err != nil {
		return err
	}

	// Show tip if not showing all tasks
	if !showAll {
		tipStyle := lipgloss.NewStyle().Foreground(tui.DrPurpleLight).Italic(true)
		fmt.Println("\n" + tipStyle.Render("💡 Tip: Run 'dr task list --all' to see all available tasks"))
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
				_, _ = fmt.Fprintln(os.Stderr, "Error:", err)

				os.Exit(1)

				return
			}

			categories := groupTasksByCategory(tasks, showAll)

			if err = printCategorizedTasks(categories, showAll); err != nil {
				_, _ = fmt.Fprintln(os.Stderr, "Error:", err)

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
