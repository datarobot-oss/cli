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

package hostpicker

import (
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// isRegionalIndicator reports whether r is one of the letters used to build flag
// emoji (U+1F1E6..U+1F1FF).
func isRegionalIndicator(r rune) bool {
	return r >= 0x1F1E6 && r <= 0x1F1FF
}

// TestRegionIcons_NoFlagEmoji is the guard against reintroducing country flags.
//
// Flags are pairs of regional-indicator letters. Windows ships no flag glyphs in
// Segoe UI Emoji and paints the pair as two letter boxes instead, so a flag is
// invisible or wrong in PowerShell, Windows Terminal, and the VS Code terminal
// regardless of the user's font. There is no capability check that fixes this;
// the only fix is not to use them.
func TestRegionIcons_NoFlagEmoji(t *testing.T) {
	for _, icon := range []string{USRegionIcon, EURegionIcon, JPRegionIcon, CustomRegionIcon} {
		for _, r := range icon {
			assert.False(t, isRegionalIndicator(r),
				"icon %q contains regional indicator %U; flags do not render on Windows", icon, r)
		}
	}
}

// TestRegionIcons_AreSingleCodepointAndWidthTwo pins the property that keeps the
// hand-aligned menu in internal/auth.printSetURLPrompt true.
//
// A flag measures as width 2 but paints 4 columns on Windows, so alignment breaks
// silently. A single-codepoint emoji measures and paints the same everywhere.
func TestRegionIcons_AreSingleCodepointAndWidthTwo(t *testing.T) {
	for _, icon := range []string{USRegionIcon, EURegionIcon, JPRegionIcon, CustomRegionIcon} {
		assert.Equal(t, 1, utf8.RuneCountInString(icon),
			"icon %q must be a single codepoint so its measured width matches what terminals paint", icon)
		assert.Equal(t, 2, lipgloss.Width(icon), "icon %q must measure two columns", icon)
	}
}

// TestRegionIcons_AreDistinct catches a copy-paste that would give two regions the
// same globe.
func TestRegionIcons_AreDistinct(t *testing.T) {
	seen := map[string]bool{}

	for _, icon := range []string{USRegionIcon, EURegionIcon, JPRegionIcon, CustomRegionIcon} {
		assert.False(t, seen[icon], "icon %q is used for more than one entry", icon)
		seen[icon] = true
	}
}

// TestNew_ItemTitlesUseTheRegionIcons ties the list items to the constants, so the
// checks above actually cover what the user sees.
func TestNew_ItemTitlesUseTheRegionIcons(t *testing.T) {
	items := New().list.Items()
	require.Len(t, items, 4)

	wantPrefixes := []string{USRegionIcon, EURegionIcon, JPRegionIcon, CustomRegionIcon}

	for i, li := range items {
		entry, ok := li.(item)
		require.True(t, ok)

		assert.True(t, len(entry.title) > 0 && entry.title[:len(wantPrefixes[i])] == wantPrefixes[i],
			"item %d title %q must start with %q", i, entry.title, wantPrefixes[i])

		for _, r := range entry.title {
			assert.False(t, isRegionalIndicator(r),
				"item %d title %q contains a flag codepoint", i, entry.title)
		}
	}
}

// compile-time assurance that the list package is the one New() builds against.
var _ = list.Item(item{})
