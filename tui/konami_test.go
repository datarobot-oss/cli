package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func key(k string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}

func arrowKey(t tea.KeyType) tea.KeyMsg {
	return tea.KeyMsg{Type: t}
}

func feedSequence(d *konamiDetector, keys []tea.KeyMsg) bool {
	var result bool

	for _, k := range keys {
		result = d.Feed(k)
	}

	return result
}

func TestKonamiDetector_FullSequence(t *testing.T) {
	d := &konamiDetector{}

	sequence := []tea.KeyMsg{
		arrowKey(tea.KeyUp),
		arrowKey(tea.KeyUp),
		arrowKey(tea.KeyDown),
		arrowKey(tea.KeyDown),
		arrowKey(tea.KeyLeft),
		arrowKey(tea.KeyRight),
		arrowKey(tea.KeyLeft),
		arrowKey(tea.KeyRight),
		key("b"),
		key("a"),
	}

	triggered := feedSequence(d, sequence)

	assert.True(t, triggered, "full Konami sequence should trigger")
}

func TestKonamiDetector_WrongKeyResets(t *testing.T) {
	d := &konamiDetector{}

	// Feed up, then a wrong key
	d.Feed(arrowKey(tea.KeyUp))
	d.Feed(arrowKey(tea.KeyUp))

	result := d.Feed(key("x"))

	assert.False(t, result, "wrong key should not trigger")

	// Now the full sequence from scratch should still work
	sequence := []tea.KeyMsg{
		arrowKey(tea.KeyUp),
		arrowKey(tea.KeyUp),
		arrowKey(tea.KeyDown),
		arrowKey(tea.KeyDown),
		arrowKey(tea.KeyLeft),
		arrowKey(tea.KeyRight),
		arrowKey(tea.KeyLeft),
		arrowKey(tea.KeyRight),
		key("b"),
		key("a"),
	}

	triggered := feedSequence(d, sequence)

	assert.True(t, triggered, "fresh full sequence should trigger after reset")
}

func TestKonamiDetector_PartialSequenceNoTrigger(t *testing.T) {
	d := &konamiDetector{}

	// Feed only first 9 keys of the 10-key sequence
	sequence := []tea.KeyMsg{
		arrowKey(tea.KeyUp),
		arrowKey(tea.KeyUp),
		arrowKey(tea.KeyDown),
		arrowKey(tea.KeyDown),
		arrowKey(tea.KeyLeft),
		arrowKey(tea.KeyRight),
		arrowKey(tea.KeyLeft),
		arrowKey(tea.KeyRight),
		key("b"),
	}

	triggered := feedSequence(d, sequence)

	assert.False(t, triggered, "partial sequence should not trigger")
}

func TestKonamiDetector_ResetsAfterTrigger(t *testing.T) {
	d := &konamiDetector{}

	sequence := []tea.KeyMsg{
		arrowKey(tea.KeyUp),
		arrowKey(tea.KeyUp),
		arrowKey(tea.KeyDown),
		arrowKey(tea.KeyDown),
		arrowKey(tea.KeyLeft),
		arrowKey(tea.KeyRight),
		arrowKey(tea.KeyLeft),
		arrowKey(tea.KeyRight),
		key("b"),
		key("a"),
	}

	feedSequence(d, sequence)

	// After triggering, feeding a single 'a' should not trigger again
	result := d.Feed(key("a"))

	assert.False(t, result, "single key after trigger should not retrigger")
}
