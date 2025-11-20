// Copyright 2025 DataRobot, Inc. and its affiliates.
// All rights reserved.
// DataRobot, Inc. Confidential.
// This is unpublished proprietary source code of DataRobot, Inc.
// and its affiliates.
// The copyright notice above does not evidence any actual or intended
// publication of such source code.

package task

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/datarobot/cli/tui"
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
	Tasks    []Task
	Priority int // Lower numbers appear first
}

// GetCategoryStyle returns the appropriate style for a category name with adaptive colors
func GetCategoryStyle(categoryName string) lipgloss.Style {
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
func categorizeTask(t Task, showAll bool) *Category {
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

// GroupTasksByCategory groups tasks into human-readable categories
func GroupTasksByCategory(tasks []Task, showAll bool) []*Category {
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
			cat.Tasks = []Task{t}
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
