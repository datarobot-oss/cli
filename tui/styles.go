// Copyright 2026 DataRobot, Inc. and its affiliates.
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

package tui

import "github.com/charmbracelet/lipgloss"

// Common style definitions using DataRobot branding
var (
	// Adaptive colors for light/dark terminals
	TitleColor  = GetAdaptiveColor(DrGreen, DrGreenDark)
	BorderColor = GetAdaptiveColor(DrPurpleLight, DrPurpleDarkLight)

	BaseTextStyle = lipgloss.NewStyle().Foreground(GetAdaptiveColor(DrPurple, DrPurpleDark))
	ErrorStyle    = lipgloss.NewStyle().Foreground(DrRed).Bold(true)
	SuccessStyle  = lipgloss.NewStyle().Foreground(GetAdaptiveColor(DrGreen, DrGreen)).Bold(true)
	InfoStyle     = lipgloss.NewStyle().Foreground(GetAdaptiveColor(DrPurpleLight, DrPurpleDarkLight)).Bold(true)
	DimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	HintStyle     = lipgloss.NewStyle().Foreground(GetAdaptiveColor(DrGray, DrGrayDark))
	TitleStyle    = BaseTextStyle.Foreground(TitleColor).Bold(true).MarginBottom(1)

	// Specific UI styles
	LogoStyle     = BaseTextStyle
	WelcomeStyle  = BaseTextStyle.Bold(true)
	SubTitleStyle = BaseTextStyle.Bold(true).
			Foreground(DrPurpleLight).
			BorderBottom(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(DrGreen)
	BoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(DrPurple).
			Padding(1, 2)
	NoteBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(BorderColor).
			Padding(0, 1)
	TableBorderStyle = lipgloss.NewStyle().Foreground(BorderColor)
	StatusBarStyle   = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(DrPurpleLight).
				Foreground(DrPurpleLight).
				Padding(0, 1)
)
