package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// DataRobot brand colors, utilizing the Design System palette
const (
	DrPurple      = lipgloss.Color("#7770F9") // purple-60
	DrPurpleLight = lipgloss.Color("#B4B0FF") // purple-40
	DrIndigo      = lipgloss.Color("#5C41FF") // indigo-70
	DrRed         = lipgloss.Color("#9A3131") // red-80
	DrGreen       = lipgloss.Color("#81FBA5") // green-60
	DrYellow      = lipgloss.Color("#F6EB61") // yellow-60
	DrBlack       = lipgloss.Color("#0B0B0B") // black-90
)

// Light mode color variants (darker for visibility on light backgrounds)
const (
	DrPurpleDark      = lipgloss.Color("#5500DD") // Darker purple
	DrPurpleDarkLight = lipgloss.Color("#7755DD") // Darker purple-light
	DrIndigoDark      = lipgloss.Color("#4400FF") // Darker indigo
	DrGreenDark       = lipgloss.Color("#00AA00") // Darker green
	DrYellowDark      = lipgloss.Color("#AA8800") // Darker yellow
	DrGray            = lipgloss.Color("252")     // Light gray for dark backgrounds
	DrGrayDark        = lipgloss.Color("240")     // Dark gray for light backgrounds
)

// GetAdaptiveColor returns a color that works on both light and dark backgrounds
func GetAdaptiveColor(darkColor, lightColor lipgloss.Color) lipgloss.Color {
	if lipgloss.HasDarkBackground() {
		return darkColor
	}

	return lightColor
}

func SetAnsiForegroundColor(hexColor lipgloss.Color) string {
	hexString := strings.TrimPrefix(string(hexColor), "#")

	rVal, _ := strconv.ParseUint(hexString[0:2], 16, 8)
	gVal, _ := strconv.ParseUint(hexString[2:4], 16, 8)
	bVal, _ := strconv.ParseUint(hexString[4:6], 16, 8)

	return fmt.Sprintf("\033[38;2;%d;%d;%dm", rVal, gVal, bVal)
}

func ResetForegroundColor() string {
	return "\033[39m"
}
